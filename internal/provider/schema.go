package provider

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"unicode/utf8"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-tplink-router/internal/catalog"
)

func fieldTypes(fields []catalog.Field, includeSecrets bool) map[string]attr.Type {
	out := map[string]attr.Type{}
	for _, f := range fields {
		if f.WriteOnly {
			// Terraform write-only attributes must never be materialized in state,
			// but their version counters are ordinary state values used to detect
			// an intentional secret rotation.
			if includeSecrets {
				out[f.Name] = fieldType(f, includeSecrets)
			}
			out[f.Name+"_version"] = types.Int64Type
			continue
		}
		out[f.Name] = fieldType(f, includeSecrets)
	}
	return out
}
func fieldType(f catalog.Field, secrets bool) attr.Type {
	switch f.Kind {
	case catalog.Bool:
		return types.BoolType
	case catalog.Int:
		return types.Int64Type
	case catalog.StringList:
		return types.ListType{ElemType: types.StringType}
	case catalog.StringSet:
		return types.SetType{ElemType: types.StringType}
	case catalog.Object:
		return types.ObjectType{AttrTypes: fieldTypes(f.Fields, secrets)}
	case catalog.ObjectList:
		return types.ListType{ElemType: types.ObjectType{AttrTypes: fieldTypes(f.Fields, secrets)}}
	case catalog.ObjectSet:
		return types.SetType{ElemType: types.ObjectType{AttrTypes: fieldTypes(f.Fields, secrets)}}
	default:
		return types.StringType
	}
}

func resourceAttributes(fields []catalog.Field) map[string]schema.Attribute {
	out := map[string]schema.Attribute{}
	for _, f := range fields {
		required := f.Required && !f.ReadOnly
		optional := !required && !f.ReadOnly
		computed := !required && !f.WriteOnly
		desc := f.Description
		if desc == "" {
			desc = f.Name
		}
		switch f.Kind {
		case catalog.Bool:
			a := schema.BoolAttribute{Required: required, Optional: optional, Computed: computed, Sensitive: f.Sensitive, Description: desc}
			if f.RequiresReplace {
				a.PlanModifiers = []planmodifier.Bool{boolplanmodifier.RequiresReplace()}
			}
			if v, ok := f.Default.(bool); ok {
				a.Default = booldefault.StaticBool(v)
			}
			out[f.Name] = a
		case catalog.Int:
			a := schema.Int64Attribute{Required: required, Optional: optional, Computed: computed, Sensitive: f.Sensitive, Description: desc, Validators: []validator.Int64{integerValidator{field: f}}}
			if f.RequiresReplace {
				a.PlanModifiers = []planmodifier.Int64{int64planmodifier.RequiresReplace()}
			}
			if f.Default != nil {
				a.Default = int64default.StaticInt64(wireInt(f.Default))
			}
			out[f.Name] = a
		case catalog.StringList:
			out[f.Name] = schema.ListAttribute{Required: required, Optional: optional, Computed: computed, Sensitive: f.Sensitive, Description: desc, ElementType: types.StringType}
		case catalog.StringSet:
			out[f.Name] = schema.SetAttribute{Required: required, Optional: optional, Computed: computed, Sensitive: f.Sensitive, Description: desc, ElementType: types.StringType}
		case catalog.Object:
			out[f.Name] = schema.SingleNestedAttribute{Required: required, Optional: optional, Computed: computed, Sensitive: f.Sensitive, Description: desc, Attributes: resourceAttributes(f.Fields)}
		case catalog.ObjectList:
			out[f.Name] = schema.ListNestedAttribute{Required: required, Optional: optional, Computed: computed, Sensitive: f.Sensitive, Description: desc, NestedObject: schema.NestedAttributeObject{Attributes: resourceAttributes(f.Fields)}}
		case catalog.ObjectSet:
			out[f.Name] = schema.SetNestedAttribute{Required: required, Optional: optional, Computed: computed, Sensitive: f.Sensitive, Description: desc, NestedObject: schema.NestedAttributeObject{Attributes: resourceAttributes(f.Fields)}}
		default:
			a := schema.StringAttribute{Required: required, Optional: optional, Computed: computed, Sensitive: f.Sensitive, WriteOnly: f.WriteOnly, Description: desc, Validators: []validator.String{stringValidator{field: f}}}
			if f.RequiresReplace {
				a.PlanModifiers = []planmodifier.String{stringplanmodifier.RequiresReplace()}
			}
			if v, ok := f.Default.(string); ok {
				a.Default = stringdefault.StaticString(v)
			}
			out[f.Name] = a
		}
		if f.WriteOnly {
			out[f.Name+"_version"] = schema.Int64Attribute{Optional: true, Description: "Increase this value to intentionally set " + f.Name + ". Omit both attributes to keep the current secret."}
		}
	}
	return out
}

func resourceSchema(s catalog.Spec) schema.Schema {
	attrs := resourceAttributes(s.Fields)
	if s.Mode == catalog.Ruleset {
		attrs = map[string]schema.Attribute{"rules": schema.ListNestedAttribute{Required: true, Description: "Complete ordered list of user rules. Omitted user rules are deleted on apply; built-in rules are preserved.", NestedObject: schema.NestedAttributeObject{Attributes: attrs}}}
	}
	attrs["id"] = schema.StringAttribute{Computed: true, Description: "Stable device identity. Import existing resources using this ID.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}}
	return schema.Schema{Description: s.Description + " " + s.Notes, Attributes: attrs}
}

type stringValidator struct{ field catalog.Field }

func (v stringValidator) Description(context.Context) string {
	return "Validate the router field's supported values and format."
}
func (v stringValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }
func (v stringValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	s := req.ConfigValue.ValueString()
	f := v.field
	invalid := ""
	if f.MaxLength > 0 && utf8.RuneCountInString(s) > f.MaxLength {
		invalid = fmt.Sprintf("Must contain at most %d characters.", f.MaxLength)
	}
	if len(f.Enum) > 0 {
		ok := false
		for _, x := range f.Enum {
			if s == x {
				ok = true
			}
		}
		if !ok {
			invalid = "Must be one of: " + strings.Join(f.Enum, ", ")
		}
	}
	if s != "" {
		switch f.Format {
		case "ip", "ipv4", "ipv6":
			ip, err := netip.ParseAddr(s)
			if err != nil || (f.Format == "ipv4" && !ip.Is4()) || (f.Format == "ipv6" && !ip.Is6()) {
				invalid = "Must be a valid " + f.Format + " address."
			}
		case "cidr":
			if _, err := netip.ParsePrefix(s); err != nil {
				invalid = "Must be a valid CIDR prefix."
			}
		case "mac":
			if _, err := net.ParseMAC(s); err != nil {
				invalid = "Must be a valid MAC address."
			}
		}
	}
	if invalid != "" {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid router configuration", invalid)
	}
}

type integerValidator struct{ field catalog.Field }

func (v integerValidator) Description(context.Context) string {
	return "Validate the integer range supported by the router."
}
func (v integerValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }
func (v integerValidator) ValidateInt64(_ context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	n := req.ConfigValue.ValueInt64()
	f := v.field
	if f.Max > f.Min && (n < f.Min || n > f.Max) {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid router configuration", fmt.Sprintf("Must be between %d and %d.", f.Min, f.Max))
	}
}
