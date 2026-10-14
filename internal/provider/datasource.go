package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-tplink-router/internal/catalog"
)

func dataAttributes(fields []catalog.Field) map[string]schema.Attribute {
	out := map[string]schema.Attribute{}
	for _, f := range fields {
		if f.WriteOnly {
			continue
		}
		d := f.Description
		switch f.Kind {
		case catalog.Bool:
			out[f.Name] = schema.BoolAttribute{Computed: true, Sensitive: f.Sensitive, Description: d}
		case catalog.Int:
			out[f.Name] = schema.Int64Attribute{Computed: true, Sensitive: f.Sensitive, Description: d}
		case catalog.StringList:
			out[f.Name] = schema.ListAttribute{Computed: true, Sensitive: f.Sensitive, Description: d, ElementType: types.StringType}
		case catalog.StringSet:
			out[f.Name] = schema.SetAttribute{Computed: true, Sensitive: f.Sensitive, Description: d, ElementType: types.StringType}
		case catalog.Object:
			out[f.Name] = schema.SingleNestedAttribute{Computed: true, Sensitive: f.Sensitive, Description: d, Attributes: dataAttributes(f.Fields)}
		case catalog.ObjectList:
			out[f.Name] = schema.ListNestedAttribute{Computed: true, Sensitive: f.Sensitive, Description: d, NestedObject: schema.NestedAttributeObject{Attributes: dataAttributes(f.Fields)}}
		case catalog.ObjectSet:
			out[f.Name] = schema.SetNestedAttribute{Computed: true, Sensitive: f.Sensitive, Description: d, NestedObject: schema.NestedAttributeObject{Attributes: dataAttributes(f.Fields)}}
		default:
			out[f.Name] = schema.StringAttribute{Computed: true, Sensitive: f.Sensitive, Description: d}
		}
	}
	return out
}

type routerDataSource struct {
	spec catalog.Spec
	conn *connection
}

func (d *routerDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.spec.Name
}
func (d *routerDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := dataAttributes(d.spec.Fields)
	if d.spec.Mode != catalog.Settings {
		attrs["id"] = schema.StringAttribute{Computed: true, Description: "Stable row identity used when importing this object."}
		attrs = map[string]schema.Attribute{"items": schema.ListNestedAttribute{Computed: true, Description: "Current device rows, including predefined rows. Secret values are not returned.", NestedObject: schema.NestedAttributeObject{Attributes: attrs}}, "row_count": schema.Int64Attribute{Computed: true, Description: "Number of returned rows."}}
	}
	attrs["id"] = schema.StringAttribute{Computed: true, Description: "Data-source identity."}
	resp.Schema = schema.Schema{Description: "Read " + d.spec.Description + " " + d.spec.Notes, Attributes: attrs}
}
func (d *routerDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.conn = configureConnection(req.ProviderData, &resp.Diagnostics)
}
func (d *routerDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.conn == nil {
		resp.Diagnostics.AddError("Unconfigured provider", "Router client is unavailable.")
		return
	}
	err := d.conn.withOperation(ctx, func() error {
		s, err := resolveSpec(ctx, d.conn, d.spec)
		if err != nil {
			return err
		}
		if s.Mode == catalog.Settings {
			row, err := readSettings(ctx, d.conn, s)
			if err != nil {
				return err
			}
			vals, err := fieldsFromWire(ctx, s.Fields, row.Data, nil, false)
			if err != nil {
				return err
			}
			vals["id"] = types.StringValue("settings")
			ts := fieldTypes(s.Fields, false)
			ts["id"] = types.StringType
			obj, ds := types.ObjectValue(ts, vals)
			resp.Diagnostics.Append(ds...)
			if !resp.Diagnostics.HasError() {
				resp.Diagnostics.Append(resp.State.Set(ctx, obj)...)
			}
			return diagnosticError(resp.Diagnostics)
		}
		rows, err := readRows(ctx, d.conn, s)
		if err != nil {
			return err
		}
		ts := fieldTypes(s.Fields, false)
		ts["id"] = types.StringType
		ot := types.ObjectType{AttrTypes: ts}
		items := []attr.Value{}
		for _, row := range rows {
			vals, err := fieldsFromWire(ctx, s.Fields, row.Data, nil, false)
			if err != nil {
				return err
			}
			vals["id"] = types.StringValue(row.ID)
			obj, ds := types.ObjectValue(ts, vals)
			resp.Diagnostics.Append(ds...)
			items = append(items, obj)
		}
		list, ds := types.ListValue(ot, items)
		resp.Diagnostics.Append(ds...)
		obj, ds := types.ObjectValue(map[string]attr.Type{"id": types.StringType, "row_count": types.Int64Type, "items": types.ListType{ElemType: ot}}, map[string]attr.Value{"id": types.StringValue(d.conn.client.Endpoint() + "#" + s.Module + "." + s.Table), "row_count": types.Int64Value(int64(len(rows))), "items": list})
		resp.Diagnostics.Append(ds...)
		if !resp.Diagnostics.HasError() {
			resp.Diagnostics.Append(resp.State.Set(ctx, obj)...)
		}
		return diagnosticError(resp.Diagnostics)
	})
	if err != nil {
		resp.Diagnostics.AddError("Cannot read router data", fmt.Sprintf("%s: %s", d.spec.Name, err))
	}
}

type deviceDataSource struct{ conn *connection }

func (d *deviceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_device"
}
func (d *deviceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.conn = configureConnection(req.ProviderData, &resp.Diagnostics)
}
func (d *deviceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	a := map[string]schema.Attribute{}
	for _, name := range []string{"id", "model", "hardware_version", "firmware_version", "configuration_status"} {
		a[name] = schema.StringAttribute{Computed: true, Description: name}
	}
	a["capabilities"] = schema.ListAttribute{Computed: true, ElementType: types.StringType, Description: "Feature modules advertised by the current firmware."}
	resp.Schema = schema.Schema{Description: "Read router identity, firmware, capabilities, and pending-save status.", Attributes: a}
}
func (d *deviceDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.conn == nil {
		resp.Diagnostics.AddError("Unconfigured provider", "Router client is unavailable.")
		return
	}
	err := d.conn.withOperation(ctx, func() error {
		if err := d.conn.discover(ctx); err != nil {
			return err
		}
		res, err := d.conn.client.Query(ctx, map[string]any{"device_info": map[string]any{"name": "info"}, "system": map[string]any{"name": "cfg_status"}})
		if err != nil {
			return err
		}
		m, _ := res["device_info"].(map[string]any)
		info, _ := m["info"].(map[string]any)
		m, _ = res["system"].(map[string]any)
		status, _ := m["cfg_status"].(map[string]any)
		caps := []string{}
		for cap := range d.conn.capabilities {
			caps = append(caps, cap)
		}
		sort.Strings(caps)
		list, ds := types.ListValueFrom(ctx, types.StringType, caps)
		resp.Diagnostics.Append(ds...)
		vals := map[string]attr.Value{"id": types.StringValue(d.conn.client.Endpoint()), "model": types.StringValue(wireString(info["device_model"])), "hardware_version": types.StringValue(wireString(info["hw_version"])), "firmware_version": types.StringValue(wireString(info["sw_version"])), "configuration_status": types.StringValue(wireString(status["status"])), "capabilities": list}
		ts := map[string]attr.Type{}
		for key, value := range vals {
			ts[key] = value.Type(ctx)
		}
		obj, ds := types.ObjectValue(ts, vals)
		resp.Diagnostics.Append(ds...)
		if !resp.Diagnostics.HasError() {
			resp.Diagnostics.Append(resp.State.Set(ctx, obj)...)
		}
		return diagnosticError(resp.Diagnostics)
	})
	if err != nil {
		resp.Diagnostics.AddError("Cannot read router information", err.Error())
	}
}
