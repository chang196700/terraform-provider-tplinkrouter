package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"reflect"
	"strconv"
	"strings"

	"github.com/chang196700/terraform-provider-tplinkrouter/internal/catalog"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func wireString(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	default:
		return fmt.Sprint(v)
	}
}
func wireInt(v any) int64 { n, _ := strconv.ParseInt(wireString(v), 10, 64); return n }
func wireName(f catalog.Field) string {
	if f.Wire != "" {
		return f.Wire
	}
	return f.Name
}

func nullValue(ctx context.Context, t attr.Type) attr.Value {
	v, _ := t.ValueFromTerraform(ctx, tftypes.NewValue(t.TerraformType(ctx), nil))
	return v
}

func fromWire(ctx context.Context, f catalog.Field, raw any, secrets bool) (attr.Value, error) {
	t := fieldType(f, secrets)
	if raw == nil {
		return nullValue(ctx, t), nil
	}
	if f.Decode != nil {
		var err error
		raw, err = f.Decode(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: cannot decode device value", f.Name)
		}
		if raw == nil {
			return nullValue(ctx, t), nil
		}
	}
	switch f.Kind {
	case catalog.Bool:
		s := wireString(raw)
		if s == f.TrueValue && f.TrueValue != "" {
			return types.BoolValue(true), nil
		}
		if s == f.FalseValue && f.FalseValue != "" {
			return types.BoolValue(false), nil
		}
		switch strings.ToLower(s) {
		case "true", "on", "1", "enable", "enabled", "yes", "bind":
			return types.BoolValue(true), nil
		case "false", "off", "0", "disable", "disabled", "no", "unbind":
			return types.BoolValue(false), nil
		case "":
			return types.BoolNull(), nil
		}
		return nil, fmt.Errorf("%s: unrecognized boolean returned by device", f.Name)
	case catalog.Int:
		if wireString(raw) == "" {
			return types.Int64Null(), nil
		}
		n, err := strconv.ParseInt(wireString(raw), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s: non-integer returned by device", f.Name)
		}
		return types.Int64Value(n), nil
	case catalog.StringList, catalog.StringSet:
		var vs []attr.Value
		switch x := raw.(type) {
		case []any:
			for _, v := range x {
				vs = append(vs, types.StringValue(wireString(v)))
			}
		case []string:
			for _, v := range x {
				vs = append(vs, types.StringValue(v))
			}
		case string:
			if x != "" {
				if f.Separator != "" {
					for _, v := range strings.Split(x, f.Separator) {
						vs = append(vs, types.StringValue(v))
					}
				} else {
					vs = append(vs, types.StringValue(x))
				}
			}
		default:
			return nil, fmt.Errorf("%s: device returned neither a string nor a collection", f.Name)
		}
		if f.Kind == catalog.StringSet {
			v, d := types.SetValue(types.StringType, vs)
			return v, diagnosticError(d)
		}
		v, d := types.ListValue(types.StringType, vs)
		return v, diagnosticError(d)
	case catalog.Object:
		m, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: device returned a non-object", f.Name)
		}
		vals, err := fieldsFromWire(ctx, f.Fields, m, nil, secrets)
		if err != nil {
			return nil, err
		}
		v, d := types.ObjectValue(fieldTypes(f.Fields, secrets), vals)
		return v, diagnosticError(d)
	case catalog.ObjectList, catalog.ObjectSet:
		items, ok := raw.([]any)
		if !ok {
			if s, yes := raw.(string); yes && s == "" {
				items = []any{}
			} else {
				return nil, fmt.Errorf("%s: device returned a non-list", f.Name)
			}
		}
		var vals []attr.Value
		child := f
		child.Kind = catalog.Object
		child.Decode = nil
		for _, item := range items {
			v, err := fromWire(ctx, child, item, secrets)
			if err != nil {
				return nil, err
			}
			vals = append(vals, v)
		}
		ot := types.ObjectType{AttrTypes: fieldTypes(f.Fields, secrets)}
		if f.Kind == catalog.ObjectSet {
			v, d := types.SetValue(ot, vals)
			return v, diagnosticError(d)
		}
		v, d := types.ListValue(ot, vals)
		return v, diagnosticError(d)
	default:
		return types.StringValue(wireString(raw)), nil
	}
}

func diagnosticError(d diag.Diagnostics) error {
	if !d.HasError() {
		return nil
	}
	return fmt.Errorf("value conversion failed: %s", d.Errors()[0].Summary())
}

// fieldsFromWire preserves prior spelling only when the device value is semantically equal.
// Changed device values are returned unchanged so Terraform can detect real drift.
func fieldsFromWire(ctx context.Context, fields []catalog.Field, raw map[string]any, prior map[string]attr.Value, secrets bool) (map[string]attr.Value, error) {
	out := map[string]attr.Value{}
	for _, f := range fields {
		if f.WriteOnly {
			v := prior[f.Name+"_version"]
			if v == nil || v.IsUnknown() {
				v = types.Int64Null()
			}
			out[f.Name+"_version"] = v
			// The write-only value is accepted from configuration but is never
			// returned by the device or stored in Terraform state.
			if secrets {
				out[f.Name] = types.StringNull()
			}
			continue
		}
		key := wireName(f)
		if f.Section != "" {
			key = f.Section + "\x1f" + key
		}
		v, err := fromWire(ctx, f, raw[key], secrets)
		if err != nil {
			return nil, err
		}
		if old := prior[f.Name]; old != nil && !old.IsNull() && !old.IsUnknown() && equivalent(f, old, v) {
			v = old
		}
		out[f.Name] = v
	}
	return out, nil
}

func equivalent(f catalog.Field, a, b attr.Value) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a.Equal(b) {
		return true
	}
	x, ok := a.(types.String)
	if !ok || a.IsNull() || a.IsUnknown() || b.IsNull() || b.IsUnknown() {
		return false
	}
	y, ok := b.(types.String)
	if !ok {
		return false
	}
	s, t := x.ValueString(), y.ValueString()
	switch f.Format {
	case "mac":
		p, e1 := net.ParseMAC(s)
		q, e2 := net.ParseMAC(t)
		return e1 == nil && e2 == nil && reflect.DeepEqual(p, q)
	case "ip", "ipv4", "ipv6":
		p, e1 := netip.ParseAddr(s)
		q, e2 := netip.ParseAddr(t)
		return e1 == nil && e2 == nil && p == q
	case "cidr":
		p, e1 := netip.ParsePrefix(s)
		q, e2 := netip.ParsePrefix(t)
		return e1 == nil && e2 == nil && p.Masked() == q.Masked()
	}
	return false
}

func valueToWire(f catalog.Field, v attr.Value) (any, error) {
	if v == nil || v.IsNull() {
		return nil, nil
	}
	if v.IsUnknown() {
		return nil, fmt.Errorf("%s is still unknown at apply time", f.Name)
	}
	var raw any
	switch x := v.(type) {
	case types.String:
		raw = x.ValueString()
	case types.Bool:
		if x.ValueBool() {
			raw = f.TrueValue
			if raw == "" {
				raw = "1"
			}
		} else {
			raw = f.FalseValue
			if raw == "" {
				raw = "0"
			}
		}
	case types.Int64:
		raw = strconv.FormatInt(x.ValueInt64(), 10)
	case types.List:
		var list []any
		for _, e := range x.Elements() {
			child := catalog.Field{Name: f.Name, Kind: catalog.String}
			if f.Kind == catalog.ObjectList {
				child.Kind = catalog.Object
				child.Fields = f.Fields
			}
			v, err := valueToWire(child, e)
			if err != nil {
				return nil, err
			}
			list = append(list, v)
		}
		if list == nil {
			list = []any{}
		}
		raw = list
	case types.Set:
		var list []any
		for _, e := range x.Elements() {
			child := catalog.Field{Name: f.Name, Kind: catalog.String}
			if f.Kind == catalog.ObjectSet {
				child.Kind = catalog.Object
				child.Fields = f.Fields
			}
			v, err := valueToWire(child, e)
			if err != nil {
				return nil, err
			}
			list = append(list, v)
		}
		if list == nil {
			list = []any{}
		}
		raw = list
	case types.Object:
		m := map[string]any{}
		for _, ch := range f.Fields {
			if ch.ReadOnly || ch.WriteOnly {
				continue
			}
			av := x.Attributes()[ch.Name]
			if av == nil || av.IsNull() || av.IsUnknown() {
				continue
			}
			p, err := valueToWire(ch, av)
			if err != nil {
				return nil, err
			}
			m[wireName(ch)] = p
		}
		raw = m
	default:
		return nil, fmt.Errorf("unsupported Terraform type for %s", f.Name)
	}
	if f.Separator != "" {
		if list, ok := raw.([]any); ok {
			parts := make([]string, len(list))
			for i, v := range list {
				parts[i] = wireString(v)
			}
			raw = strings.Join(parts, f.Separator)
		}
	}
	if f.Encode != nil {
		return f.Encode(raw)
	}
	return raw, nil
}

func fieldsToWire(fields []catalog.Field, plan, config, prior map[string]attr.Value, creating bool) (map[string]any, error) {
	out := map[string]any{}
	for _, f := range fields {
		if f.ReadOnly {
			continue
		}
		v := plan[f.Name]
		if f.WriteOnly {
			v = config[f.Name]
			version := plan[f.Name+"_version"]
			old := prior[f.Name+"_version"]
			changed := creating || (version != nil && !version.IsNull() && !version.IsUnknown() && (old == nil || !version.Equal(old)))
			if !changed {
				continue
			}
			if v == nil || v.IsNull() {
				if version != nil && !version.IsNull() {
					return nil, fmt.Errorf("%s is required when its version changes", f.Name)
				}
				continue
			}
		} else {
			configured := config[f.Name]
			if (configured == nil || configured.IsNull()) && f.Default == nil {
				continue
			}
		}
		if v == nil || v.IsNull() {
			continue
		}
		raw, err := valueToWire(f, v)
		if err != nil {
			return nil, err
		}
		out[wireName(f)] = raw
	}
	return out, nil
}

func mergeMaps(base, overlay map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}

func knownString(m map[string]attr.Value, key string) string {
	v, ok := m[key].(types.String)
	if !ok || v.IsNull() || v.IsUnknown() {
		return ""
	}
	return v.ValueString()
}

