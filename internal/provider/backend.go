package provider

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/chang196700/terraform-provider-tplinkrouter/internal/catalog"
)

type routerRow struct {
	ID, Section string
	Data        map[string]any
}

func resolveSpec(ctx context.Context, c *connection, s catalog.Spec) (catalog.Spec, error) {
	if err := c.check(ctx, s); err != nil {
		return s, err
	}
	if s.Resolve != nil {
		return s.Resolve(ctx, c.client, s)
	}
	return s, nil
}

func rowIdentity(s catalog.Spec, section string, data map[string]any) (string, error) {
	if len(s.IDFields) > 0 {
		parts := make([]string, len(s.IDFields))
		for i, key := range s.IDFields {
			v, ok := data[key]
			if list, yes := v.([]any); yes && len(list) == 1 {
				v = list[0]
			}
			if list, yes := v.([]string); yes && len(list) == 1 {
				v = list[0]
			}
			if !ok || wireString(v) == "" {
				return "", fmt.Errorf("missing composite identity field %s", key)
			}
			parts[i] = url.PathEscape(wireString(v))
		}
		return strings.Join(parts, "/"), nil
	}
	if s.IDField != "" {
		if v, ok := data[s.IDField]; ok && wireString(v) != "" {
			return wireString(v), nil
		}
		return "", fmt.Errorf("device omitted identity field %s", s.IDField)
	}
	if section != "" {
		return section, nil
	}
	if n := wireString(data[".name"]); n != "" {
		return n, nil
	}
	return "", fmt.Errorf("device omitted row section identity")
}

func splitIdentity(s catalog.Spec, id string) (map[string]any, error) {
	out := map[string]any{}
	if len(s.IDFields) > 0 {
		parts := strings.Split(id, "/")
		if len(parts) != len(s.IDFields) {
			return nil, fmt.Errorf("import ID must contain %d slash-separated URL-escaped components", len(s.IDFields))
		}
		for i, key := range s.IDFields {
			v, err := url.PathUnescape(parts[i])
			if err != nil || v == "" {
				return nil, fmt.Errorf("invalid component in composite import ID")
			}
			out[key] = v
		}
	} else if s.IDField != "" {
		out[s.IDField] = id
	}
	return out, nil
}

func readRows(ctx context.Context, c *connection, s catalog.Spec) ([]routerRow, error) {
	if s.ReadRows != nil {
		raw, err := s.ReadRows(ctx, c.client, s)
		if err != nil {
			return nil, err
		}
		var rows []routerRow
		for _, data := range raw {
			section := wireString(data[".name"])
			id, err := rowIdentity(s, section, data)
			if err != nil {
				return nil, err
			}
			rows = append(rows, routerRow{ID: id, Section: section, Data: data})
		}
		return rows, nil
	}
	query := mergeMaps(s.Query, map[string]any{"table": s.Table})
	if len(s.Filter) > 0 {
		if _, ok := query["filter"]; !ok {
			query["filter"] = []any{s.Filter}
		}
	}
	var rows []routerRow
	seen := map[string]bool{}
	for page := 0; page < 10000; page++ {
		response, err := c.client.Query(ctx, map[string]any{s.Module: query})
		if err != nil {
			return nil, err
		}
		module, ok := response[s.Module].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("device response omitted module %s", s.Module)
		}
		raw, present := module[s.Table]
		if !present {
			return nil, fmt.Errorf("device response omitted table %s.%s", s.Module, s.Table)
		}
		items, ok := raw.([]any)
		if raw == nil {
			items = []any{}
			ok = true
		}
		if !ok {
			return nil, fmt.Errorf("device returned a non-list for %s.%s", s.Module, s.Table)
		}
		added := 0
		for _, item := range items {
			entry, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("device returned a malformed row in %s", s.Table)
			}
			section := ""
			data := entry
			if len(entry) == 1 {
				for key, value := range entry {
					if fields, yes := value.(map[string]any); yes {
						section = key
						data = fields
					}
				}
			}
			id, err := rowIdentity(s, section, data)
			if err != nil {
				if s.DataSourceOnly {
					id = fmt.Sprintf("row-%d", len(rows))
				} else {
					return nil, fmt.Errorf("%s: %w", s.Name, err)
				}
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			rows = append(rows, routerRow{ID: id, Section: section, Data: data})
			added++
		}
		count := int64(len(rows))
		if counts, ok := module["count"].(map[string]any); ok {
			if v, exists := counts[s.Table]; exists {
				count = wireInt(v)
			}
		}
		if int64(len(rows)) >= count {
			break
		}
		if added == 0 {
			return nil, fmt.Errorf("device pagination did not advance for %s; refusing to use an incomplete table", s.Table)
		}
		para, _ := query["para"].(map[string]any)
		query["para"] = mergeMaps(para, map[string]any{"start": len(rows), "end": len(rows) + 99})
		if page == 9999 {
			return nil, fmt.Errorf("device table exceeded pagination limit")
		}
	}
	if s.Mode == catalog.Ruleset {
		// Firmware's .index is the physical order; rule position fields are not always returned.
		sort.SliceStable(rows, func(i, j int) bool {
			a, aok := rows[i].Data[".index"]
			b, bok := rows[j].Data[".index"]
			return aok && bok && wireInt(a) < wireInt(b)
		})
	}
	return rows, nil
}

func readSettings(ctx context.Context, c *connection, s catalog.Spec) (routerRow, error) {
	sections := []string{s.Section}
	seen := map[string]bool{s.Section: true}
	for _, f := range s.Fields {
		if f.Section != "" && !seen[f.Section] {
			sections = append(sections, f.Section)
			seen[f.Section] = true
		}
	}
	var names any = s.Section
	if len(sections) > 1 {
		names = sections
	}
	response, err := c.client.Query(ctx, map[string]any{s.Module: mergeMaps(s.Query, map[string]any{"name": names})})
	if err != nil {
		return routerRow{}, err
	}
	module, ok := response[s.Module].(map[string]any)
	if !ok {
		return routerRow{}, fmt.Errorf("device response omitted module %s", s.Module)
	}
	base, ok := module[s.Section].(map[string]any)
	if !ok {
		return routerRow{}, fmt.Errorf("device response omitted section %s.%s", s.Module, s.Section)
	}
	raw := mergeMaps(base, nil)
	for _, f := range s.Fields {
		if f.Section != "" {
			sec, ok := module[f.Section].(map[string]any)
			if !ok {
				return routerRow{}, fmt.Errorf("device response omitted section %s.%s", s.Module, f.Section)
			}
			raw[f.Section+"\x1f"+wireName(f)] = sec[wireName(f)]
		}
	}
	return routerRow{ID: "settings", Section: s.Section, Data: raw}, nil
}

func findRow(rows []routerRow, id string) (routerRow, bool) {
	if id == "" {
		return routerRow{}, false
	}
	for _, r := range rows {
		if r.ID == id || r.Section != "" && r.Section == id {
			return r, true
		}
	}
	return routerRow{}, false
}
func isProtected(s catalog.Spec, r routerRow) bool {
	for key, values := range s.Protected {
		actual := wireString(r.Data[key])
		for _, v := range values {
			if actual == v || (strings.HasSuffix(v, "*") && strings.HasPrefix(actual, strings.TrimSuffix(v, "*"))) {
				return true
			}
		}
	}
	return false
}
func userRows(s catalog.Spec, rows []routerRow) []routerRow {
	var out []routerRow
	for _, r := range rows {
		if !isProtected(s, r) {
			out = append(out, r)
		}
	}
	return out
}
func writer(s catalog.Spec) string {
	if s.WriteModule != "" {
		return s.WriteModule
	}
	return s.Module
}
func writeParameters(s catalog.Spec, p map[string]any) map[string]any {
	out := mergeMaps(s.WriteDefaults, p)
	for _, key := range s.OmitWriteFields {
		delete(out, key)
	}
	return out
}

func selector(s catalog.Spec, r routerRow) (map[string]any, error) {
	f := mergeMaps(s.Filter, nil)
	keys := s.FilterFields
	if len(keys) == 0 && s.FilterField != "" {
		keys = []string{s.FilterField}
	}
	parts, err := splitIdentity(s, r.ID)
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		v, ok := r.Data[key]
		if !ok {
			v, ok = parts[key]
		}
		if !ok && key == s.FilterField {
			v = r.ID
			ok = true
		}
		if !ok {
			return nil, fmt.Errorf("missing selector %s", key)
		}
		list := s.FilterList
		for _, k := range s.FilterListFields {
			if key == k {
				list = true
			}
		}
		if list {
			v = []any{v}
		}
		f[key] = v
	}
	return f, nil
}

func setRow(ctx context.Context, c *connection, s catalog.Spec, r routerRow, para map[string]any) error {
	if len(para) == 0 {
		return nil
	}
	para = mergeMaps(nil, para)
	for _, key := range s.PreserveWriteFields {
		if _, ok := para[key]; !ok {
			if value, found := r.Data[key]; found {
				para[key] = value
			}
		}
	}
	for _, f := range s.Fields {
		if f.Kind != catalog.Object {
			continue
		}
		key := wireName(f)
		proposed, changed := para[key]
		if !changed {
			continue
		}
		current := r.Data[key]
		if f.Decode != nil {
			var err error
			current, err = f.Decode(current)
			if err != nil {
				return fmt.Errorf("cannot decode current %s for an update", f.Name)
			}
		}
		base, ok := current.(map[string]any)
		overlay, other := proposed.(map[string]any)
		if ok && other {
			merged := deepMerge(base, overlay)
			if f.Encode != nil {
				var err error
				para[key], err = f.Encode(merged)
				if err != nil {
					return fmt.Errorf("cannot encode merged %s", f.Name)
				}
			} else {
				para[key] = merged
			}
		}
	}
	para = writeParameters(s, para)
	var body map[string]any
	if s.FilterField != "" || len(s.FilterFields) > 0 {
		f, err := selector(s, r)
		if err != nil {
			return err
		}
		body = map[string]any{"table": s.Table, "filter": []any{f}, "para": para}
	} else {
		section := r.Section
		if section == "" {
			section = r.ID
		}
		body = map[string]any{section: para}
	}
	_, err := c.client.Mutate(ctx, map[string]any{"method": "set", writer(s): body})
	return err
}

func deepMerge(base, overlay map[string]any) map[string]any {
	out := mergeMaps(base, nil)
	for k, v := range overlay {
		a, aok := out[k].(map[string]any)
		b, bok := v.(map[string]any)
		if aok && bok {
			out[k] = deepMerge(a, b)
		} else {
			out[k] = v
		}
	}
	return out
}

func addRow(ctx context.Context, c *connection, s catalog.Spec, para map[string]any) (string, error) {
	para = mergeMaps(s.CreateDefaults, para)
	if s.CreateViaSet {
		id, err := rowIdentity(s, "", para)
		if err != nil {
			return "", err
		}
		return id, setRow(ctx, c, s, routerRow{ID: id, Data: para}, para)
	}
	module := map[string]any{"table": s.Table, "para": writeParameters(s, para)}
	// The legacy SU store sends the requested insertion position twice for
	// tables that opt into isAppointIndex: the normal editor field remains in
	// para, while the actual insertion hint is a sibling of table and para.
	// Without the sibling, the firmware silently appends URL-filter and QQ-filter
	// entries regardless of their requested position.
	if s.OrderField == "insert_index" && (s.Module == "url_filter" || s.Module == "bwlist_qq") {
		if index, ok := para[s.OrderField]; ok {
			module[s.OrderField] = index
		}
	}
	response, err := c.client.Mutate(ctx, map[string]any{"method": "add", writer(s): module})
	if err != nil {
		return "", err
	}
	m, _ := response[writer(s)].(map[string]any)
	name := wireString(m["name"])
	if s.IDField != "" || len(s.IDFields) > 0 {
		if id, err := rowIdentity(s, "", para); err == nil {
			return id, nil
		}
	}
	return name, nil
}

func deleteRow(ctx context.Context, c *connection, s catalog.Spec, r routerRow) error {
	if isProtected(s, r) {
		return fmt.Errorf("the selected object is built into the firmware and cannot be deleted")
	}
	if s.DeleteValues != nil {
		para := mergeMaps(s.DeleteValues, nil)
		for _, key := range s.DeleteIdentityFields {
			v, ok := r.Data[key]
			if !ok {
				return fmt.Errorf("device omitted unbind identity %s", key)
			}
			for _, f := range s.Fields {
				if wireName(f) == key && f.Encode != nil {
					var err error
					v, err = f.Encode(v)
					if err != nil {
						return err
					}
				}
			}
			para[key] = v
		}
		return setRow(ctx, c, s, r, para)
	}
	var body map[string]any
	if s.FilterField != "" || len(s.FilterFields) > 0 {
		f, err := selector(s, r)
		if err != nil {
			return err
		}
		body = map[string]any{"table": s.Table, "filter": []any{f}}
	} else {
		name := r.Section
		if name == "" {
			name = r.ID
		}
		body = map[string]any{"name": name}
	}
	_, err := c.client.Mutate(ctx, map[string]any{"method": "delete", writer(s): body})
	return err
}

func setSettings(ctx context.Context, c *connection, s catalog.Spec, para map[string]any) error {
	body := map[string]any{}
	for _, f := range s.Fields {
		key := wireName(f)
		value, ok := para[key]
		if !ok {
			continue
		}
		section := f.Section
		if section == "" {
			section = s.Section
		}
		m, _ := body[section].(map[string]any)
		if m == nil {
			m = map[string]any{}
			body[section] = m
		}
		m[key] = value
	}
	if len(body) == 0 {
		return nil
	}
	_, err := c.client.Mutate(ctx, map[string]any{"method": "set", writer(s): body})
	return err
}

