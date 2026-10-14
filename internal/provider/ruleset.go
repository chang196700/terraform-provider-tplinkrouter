package provider

import (
	"context"
	"fmt"
	"reflect"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-tplink-router/internal/catalog"
)

func rowOrderNumber(row routerRow, fallback int) int {
	if n := wireInt(row.Data[".index"]); n >= 0 {
		if _, ok := row.Data[".index"]; ok {
			return int(n) + 1
		}
	}
	return fallback + 1
}

// orderedPosition maps Terraform's user-rule index onto the device's physical
// table position. Firmware-generated rows can occupy slots between user rules;
// preserving their slots avoids moving generated rules when a user rule is
// reordered.
func orderedPosition(s catalog.Spec, all []routerRow, desired int) int {
	var userSlots []int
	for i, row := range all {
		if !isProtected(s, row) {
			userSlots = append(userSlots, rowOrderNumber(row, i))
		}
	}
	if desired < len(userSlots) {
		return userSlots[desired]
	}
	if len(all) == 0 {
		return desired + 1
	}
	last := rowOrderNumber(all[len(all)-1], len(all)-1)
	return last + desired - len(userSlots) + 1
}

func wireValueEqual(a, b any) bool {
	if reflect.DeepEqual(a, b) {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	av, aok := a.([]any)
	bv, bok := b.([]any)
	if aok && bok {
		if len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !wireValueEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	}
	if as, aok := a.([]string); aok {
		converted := make([]any, len(as))
		for i := range as {
			converted[i] = as[i]
		}
		return wireValueEqual(converted, b)
	}
	if bs, bok := b.([]string); bok {
		converted := make([]any, len(bs))
		for i := range bs {
			converted[i] = bs[i]
		}
		return wireValueEqual(a, converted)
	}
	return wireString(a) == wireString(b)
}

func ruleNeedsUpdate(s catalog.Spec, row routerRow, para map[string]any, targetPosition int) bool {
	for key, desired := range writeParameters(s, para) {
		if key == s.OrderField {
			continue
		}
		if !wireValueEqual(row.Data[key], desired) {
			return true
		}
	}
	if s.OrderField == "" {
		return false
	}
	if physical := rowOrderNumber(row, -1); physical > 0 {
		return physical != targetPosition
	}
	if actual, ok := row.Data[s.OrderField]; ok {
		return wireInt(actual) != int64(targetPosition)
	}
	return false
}

func listObjects(v attr.Value) ([]map[string]attr.Value, error) {
	l, ok := v.(types.List)
	if !ok || l.IsUnknown() || l.IsNull() {
		return nil, fmt.Errorf("rules must be a known list")
	}
	var out []map[string]attr.Value
	for _, item := range l.Elements() {
		o, ok := item.(types.Object)
		if !ok || o.IsNull() || o.IsUnknown() {
			return nil, fmt.Errorf("each rule must be a known object")
		}
		out = append(out, o.Attributes())
	}
	return out, nil
}

func setRulesState(ctx context.Context, s catalog.Spec, rows []routerRow, prior map[string]attr.Value, state *tfsdk.State, diags *diag.Diagnostics) error {
	old, _ := listObjects(prior["rules"])
	ot := types.ObjectType{AttrTypes: fieldTypes(s.Fields, false)}
	values := []attr.Value{}
	for i, row := range rows {
		var prev map[string]attr.Value
		if i < len(old) {
			prev = old[i]
		}
		vals, err := fieldsFromWire(ctx, s.Fields, row.Data, prev, false)
		if err != nil {
			return err
		}
		obj, d := types.ObjectValue(ot.AttrTypes, vals)
		diags.Append(d...)
		values = append(values, obj)
	}
	list, d := types.ListValue(ot, values)
	diags.Append(d...)
	obj, d := types.ObjectValue(map[string]attr.Type{"id": types.StringType, "rules": types.ListType{ElemType: ot}}, map[string]attr.Value{"id": types.StringValue("ruleset"), "rules": list})
	diags.Append(d...)
	if !diags.HasError() {
		diags.Append(state.Set(ctx, obj)...)
	}
	return diagnosticError(*diags)
}

func writableRow(s catalog.Spec, row routerRow) map[string]any {
	out := map[string]any{}
	for _, f := range s.Fields {
		if f.ReadOnly || f.WriteOnly {
			continue
		}
		if v, ok := row.Data[wireName(f)]; ok {
			out[wireName(f)] = v
		}
	}
	return out
}

func (r *routerResource) applyRules(ctx context.Context, s catalog.Spec, plan, cfg, prior map[string]attr.Value, creating bool, state *tfsdk.State, diags *diag.Diagnostics) error {
	desired, err := listObjects(plan["rules"])
	if err != nil {
		return err
	}
	configured, err := listObjects(cfg["rules"])
	if err != nil {
		return err
	}
	previous, _ := listObjects(prior["rules"])
	if len(configured) != len(desired) {
		return fmt.Errorf("rules configuration and plan lengths do not match")
	}
	all, err := readRows(ctx, r.conn, s)
	if err != nil {
		return err
	}
	current := userRows(s, all)
	if creating && len(current) > 0 {
		return fmt.Errorf("this table already contains user rules; import the complete table with ID ruleset first")
	}
	parameters := make([]map[string]any, len(desired))
	keys := make([]string, len(desired))
	seen := map[string]bool{}
	for i, item := range desired {
		var old map[string]attr.Value
		if i < len(previous) {
			old = previous[i]
		}
		para, err := fieldsToWire(s.Fields, item, configured[i], old, creating)
		if err != nil {
			return err
		}
		parameters[i] = para
		if s.NameField != "" {
			keys[i] = wireString(para[s.NameField])
			if keys[i] == "" {
				for _, f := range s.Fields {
					if wireName(f) == s.NameField {
						keys[i] = knownString(item, f.Name)
					}
				}
			}
			if keys[i] == "" {
				return fmt.Errorf("every ordered rule needs a known %s", s.NameField)
			}
			if seen[keys[i]] {
				return fmt.Errorf("ordered rules must have unique %s values", s.NameField)
			}
			seen[keys[i]] = true
		}
	}
	oldKeys := make([]string, len(current))
	byKey := map[string]routerRow{}
	for i, row := range current {
		oldKeys[i] = wireString(row.Data[s.NameField])
		byKey[oldKeys[i]] = row
	}
	structural := len(current) != len(desired) || s.NameField != "" && !reflect.DeepEqual(keys, oldKeys)
	// If firmware has no move/position field, recreating only user rows is the only
	// supported way to change their order. Predefined rows are never deleted.
	rebuild := structural && s.OrderField == ""
	if rebuild {
		for i := range parameters {
			if old, ok := byKey[keys[i]]; ok && s.NameField != "" {
				parameters[i] = mergeMaps(writableRow(s, old), parameters[i])
			}
		}
	}
	changed := false
	writeErr := func() error {
		if rebuild {
			for _, row := range current {
				changed = true
				if err := deleteRow(ctx, r.conn, s, row); err != nil {
					return err
				}
			}
			for _, para := range parameters {
				changed = true
				if _, err := addRow(ctx, r.conn, s, para); err != nil {
					return err
				}
			}
			return nil
		}
		if s.NameField != "" {
			for _, row := range current {
				if !seen[wireString(row.Data[s.NameField])] {
					changed = true
					if err := deleteRow(ctx, r.conn, s, row); err != nil {
						return err
					}
				}
			}
		}
		for i, para := range parameters {
			targetPosition := 0
			if s.OrderField != "" {
				targetPosition = orderedPosition(s, all, i)
				para = mergeMaps(para, map[string]any{s.OrderField: fmt.Sprint(targetPosition)})
			}
			var old routerRow
			exists := false
			if s.NameField != "" {
				old, exists = byKey[keys[i]]
			} else if i < len(current) {
				old = current[i]
				exists = true
			}
			if exists {
				if !ruleNeedsUpdate(s, old, para, targetPosition) {
					continue
				}
				changed = true
				if err := setRow(ctx, r.conn, s, old, para); err != nil {
					return err
				}
			} else {
				changed = true
				if _, err := addRow(ctx, r.conn, s, para); err != nil {
					return err
				}
			}
		}
		return nil
	}()
	after, readErr := readRows(ctx, r.conn, s)
	if readErr == nil {
		users := userRows(s, after)
		readErr = setRulesState(ctx, s, users, plan, state, diags)
		if writeErr == nil && readErr == nil {
			if len(users) != len(desired) {
				readErr = fmt.Errorf("device returned an unexpected number of rules after apply")
			} else {
				for i, row := range users {
					if err := checkPlannedFields(s.Fields, desired[i], row.Data, ctx); err != nil {
						readErr = fmt.Errorf("rule %d: %w", i+1, err)
						break
					}
				}
			}
		}
	}
	if writeErr != nil {
		return fmt.Errorf("rule-table change stopped after a failed request; available actual state was refreshed and no request was replayed: %w", writeErr)
	}
	if readErr != nil {
		return readErr
	}
	return r.save(ctx, changed)
}
