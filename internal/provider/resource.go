package provider

import (
	"context"
	"fmt"

	"github.com/chang196700/terraform-provider-tplinkrouter/internal/catalog"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type routerResource struct {
	spec catalog.Spec
	conn *connection
}

func (r *routerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.spec.Name
}
func (r *routerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema(r.spec)
}
func (r *routerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.conn = configureConnection(req.ProviderData, &resp.Diagnostics)
}

type objectGetter interface {
	Get(context.Context, interface{}) diag.Diagnostics
}

func attributes(ctx context.Context, source objectGetter, diags *diag.Diagnostics) map[string]attr.Value {
	var v types.Object
	diags.Append(source.Get(ctx, &v)...)
	return v.Attributes()
}

func (r *routerResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	cfg := attributes(ctx, req.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.spec.Mode == catalog.Ruleset {
		if rules, ok := cfg["rules"].(types.List); ok && !rules.IsNull() && !rules.IsUnknown() {
			for i, v := range rules.Elements() {
				if o, ok := v.(types.Object); ok && !o.IsUnknown() {
					validateSecrets(r.spec.Fields, o.Attributes(), path.Root("rules").AtListIndex(i), &resp.Diagnostics)
				}
			}
		}
	} else {
		validateSecrets(r.spec.Fields, cfg, path.Empty(), &resp.Diagnostics)
	}
}
func validateSecrets(fields []catalog.Field, cfg map[string]attr.Value, base path.Path, diags *diag.Diagnostics) {
	for _, f := range fields {
		if !f.WriteOnly {
			continue
		}
		v := cfg[f.Name]
		n := cfg[f.Name+"_version"]
		hasValue := v != nil && !v.IsNull()
		hasVersion := n != nil && !n.IsNull()
		if hasValue != hasVersion {
			diags.AddAttributeError(base.AtName(f.Name), "Secret version required", "Configure both "+f.Name+" and "+f.Name+"_version to set a secret, or omit both to preserve it.")
		}
		if version, ok := n.(types.Int64); ok && !version.IsNull() && !version.IsUnknown() && version.ValueInt64() < 1 {
			diags.AddAttributeError(base.AtName(f.Name+"_version"), "Invalid secret version", "Secret versions must be positive integers.")
		}
	}
}

func (r *routerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid import ID", "An existing device identity is required.")
		return
	}
	if r.spec.Mode == catalog.Settings && req.ID != "settings" {
		resp.Diagnostics.AddError("Invalid import ID", "Import this settings resource using ID settings.")
		return
	}
	if r.spec.Mode == catalog.Ruleset && req.ID != "ruleset" {
		resp.Diagnostics.AddError("Invalid import ID", "Import this complete ordered table using ID ruleset.")
		return
	}
	if _, err := splitIdentity(r.spec, req.ID); err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(req.ID))...)
}

func (r *routerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	prior := attributes(ctx, req.State, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.conn == nil {
		resp.Diagnostics.AddError("Unconfigured provider", "Router client is unavailable.")
		return
	}
	err := r.conn.withOperation(ctx, func() error {
		s, err := resolveSpec(ctx, r.conn, r.spec)
		if err != nil {
			return err
		}
		return r.refresh(ctx, s, knownString(prior, "id"), prior, &resp.State, &resp.Diagnostics)
	})
	if err != nil {
		resp.Diagnostics.AddError("Cannot read router configuration", err.Error())
	}
}

func (r *routerResource) refresh(ctx context.Context, s catalog.Spec, id string, prior map[string]attr.Value, state *tfsdk.State, diags *diag.Diagnostics) error {
	if s.Mode == catalog.Settings {
		row, err := readSettings(ctx, r.conn, s)
		if err != nil {
			return err
		}
		return setRowState(ctx, s, row, prior, state, diags)
	}
	rows, err := readRows(ctx, r.conn, s)
	if err != nil {
		return err
	}
	if s.Mode == catalog.Ruleset {
		return setRulesState(ctx, s, userRows(s, rows), prior, state, diags)
	}
	row, ok := findRow(rows, id)
	if !ok {
		state.RemoveResource(ctx)
		return nil
	}
	return setRowState(ctx, s, row, prior, state, diags)
}

func setRowState(ctx context.Context, s catalog.Spec, row routerRow, prior map[string]attr.Value, state *tfsdk.State, diags *diag.Diagnostics) error {
	vals, err := fieldsFromWire(ctx, s.Fields, row.Data, prior, false)
	if err != nil {
		return err
	}
	vals["id"] = types.StringValue(row.ID)
	ts := fieldTypes(s.Fields, false)
	ts["id"] = types.StringType
	obj, d := types.ObjectValue(ts, vals)
	diags.Append(d...)
	if !d.HasError() {
		diags.Append(state.Set(ctx, obj)...)
	}
	return diagnosticError(*diags)
}

func (r *routerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	plan := attributes(ctx, req.Plan, &resp.Diagnostics)
	cfg := attributes(ctx, req.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, plan, cfg, nil, true, &resp.State, &resp.Diagnostics)
}
func (r *routerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	plan := attributes(ctx, req.Plan, &resp.Diagnostics)
	cfg := attributes(ctx, req.Config, &resp.Diagnostics)
	prior := attributes(ctx, req.State, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, plan, cfg, prior, false, &resp.State, &resp.Diagnostics)
}

func (r *routerResource) apply(ctx context.Context, plan, cfg, prior map[string]attr.Value, creating bool, state *tfsdk.State, diags *diag.Diagnostics) {
	if r.conn == nil {
		diags.AddError("Unconfigured provider", "Router client is unavailable.")
		return
	}
	err := r.conn.withOperation(ctx, func() error {
		s, err := resolveSpec(ctx, r.conn, r.spec)
		if err != nil {
			return err
		}
		if s.Mode == catalog.Ruleset {
			return r.applyRules(ctx, s, plan, cfg, prior, creating, state, diags)
		}
		para, err := fieldsToWire(s.Fields, plan, cfg, prior, creating)
		if err != nil {
			return err
		}
		if s.Mode == catalog.Settings {
			if _, err := readSettings(ctx, r.conn, s); err != nil {
				return err
			}
			mutationErr := setSettings(ctx, r.conn, s, para)
			row, readErr := readSettings(ctx, r.conn, s)
			if readErr == nil {
				readErr = setRowState(ctx, s, row, plan, state, diags)
			}
			if mutationErr != nil {
				return fmt.Errorf("configuration update failed; no write was retried: %w", mutationErr)
			}
			if readErr != nil {
				return readErr
			}
			if err := checkPlannedFields(s.Fields, plan, row.Data, ctx); err != nil {
				return err
			}
			return r.save(ctx, len(para) > 0)
		}
		rows, err := readRows(ctx, r.conn, s)
		if err != nil {
			return err
		}
		id := knownString(prior, "id")
		if creating {
			id = candidateID(s, para, rows)
		}
		old, exists := findRow(rows, id)
		if creating && s.Mode == catalog.Entry && exists {
			return fmt.Errorf("the object already exists; import it before managing it")
		}
		if !creating && !exists {
			return fmt.Errorf("the object disappeared during apply; refresh the plan before proceeding")
		}
		if creating && s.Mode == catalog.Existing && !exists {
			return fmt.Errorf("the requested hardware object was not discovered; import or select an existing device identity")
		}
		if exists && s.ProtectUpdates && isProtected(s, old) {
			return fmt.Errorf("this firmware-defined object is immutable")
		}
		var mutationErr error
		if creating && s.Mode == catalog.Entry {
			id, mutationErr = addRow(ctx, r.conn, s, para)
		} else {
			mutationErr = setRow(ctx, r.conn, s, old, para)
		}
		// A post-write read is mandatory, including ambiguous failures. Never replay add/set.
		after, readErr := readRows(ctx, r.conn, s)
		if readErr == nil {
			if len(s.IDFields) > 0 || s.IDField != "" {
				if newID, err := rowIdentity(s, "", mergeMaps(old.Data, para)); err == nil {
					id = newID
				}
			}
			row, found := findRow(after, id)
			if !found && creating {
				if key := candidateID(s, para, after); key != "" {
					row, found = findRow(after, key)
				}
			}
			if found {
				readErr = setRowState(ctx, s, row, plan, state, diags)
				if readErr == nil && mutationErr == nil {
					readErr = checkPlannedFields(s.Fields, plan, row.Data, ctx)
				}
			} else if mutationErr == nil {
				readErr = fmt.Errorf("device accepted the request but the object could not be found; no write was retried")
			}
		}
		if mutationErr != nil {
			return fmt.Errorf("configuration write failed; refreshed any identifiable object without retrying the write: %w", mutationErr)
		}
		if readErr != nil {
			return readErr
		}
		return r.save(ctx, creating || len(para) > 0)
	})
	if err != nil {
		diags.AddError("Cannot apply router configuration", err.Error())
	}
}

func candidateID(s catalog.Spec, para map[string]any, rows []routerRow) string {
	if s.IDField != "" || len(s.IDFields) > 0 {
		if id, err := rowIdentity(s, "", para); err == nil {
			if _, exists := findRow(rows, id); exists || s.Mode == catalog.Existing {
				return id
			}
		}
	}
	if s.NameField != "" {
		name, ok := para[s.NameField]
		if ok {
			for _, row := range rows {
				if wireString(row.Data[s.NameField]) == wireString(name) {
					return row.ID
				}
			}
		}
	}
	return ""
}

func checkPlannedFields(fields []catalog.Field, plan map[string]attr.Value, raw map[string]any, ctx context.Context) error {
	actual, err := fieldsFromWire(ctx, fields, raw, plan, false)
	if err != nil {
		return err
	}
	for _, f := range fields {
		if f.WriteOnly {
			continue
		}
		v := plan[f.Name]
		if v == nil || v.IsUnknown() || v.IsNull() {
			continue
		}
		if !equivalent(f, v, actual[f.Name]) {
			return fmt.Errorf("device did not retain the planned value of %s; actual configuration was refreshed into state", f.Name)
		}
	}
	return nil
}

func (r *routerResource) save(ctx context.Context, changed bool) error {
	if changed && r.conn.autoSave {
		if err := r.conn.client.Save(ctx); err != nil {
			return fmt.Errorf("running configuration changed, but persistence could not be confirmed: %w; inspect the next plan before retrying", err)
		}
	}
	return nil
}

func (r *routerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.spec.Mode == catalog.Settings || r.spec.Mode == catalog.Existing {
		return
	}
	prior := attributes(ctx, req.State, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.conn == nil {
		resp.Diagnostics.AddError("Unconfigured provider", "Router client is unavailable.")
		return
	}
	err := r.conn.withOperation(ctx, func() error {
		s, err := resolveSpec(ctx, r.conn, r.spec)
		if err != nil {
			return err
		}
		rows, err := readRows(ctx, r.conn, s)
		if err != nil {
			return err
		}
		targets := []routerRow{}
		if s.Mode == catalog.Ruleset {
			targets = userRows(s, rows)
		} else if row, ok := findRow(rows, knownString(prior, "id")); ok {
			targets = append(targets, row)
		}
		for _, row := range targets {
			if err := deleteRow(ctx, r.conn, s, row); err != nil {
				return err
			}
		}
		after, err := readRows(ctx, r.conn, s)
		if err != nil {
			return err
		}
		if s.Mode == catalog.Ruleset && len(userRows(s, after)) > 0 {
			return fmt.Errorf("device still contains user rules after deletion")
		}
		if s.Mode != catalog.Ruleset {
			if _, ok := findRow(after, knownString(prior, "id")); ok {
				return fmt.Errorf("device still contains the object after deletion")
			}
		}
		return r.save(ctx, len(targets) > 0)
	})
	if err != nil {
		resp.Diagnostics.AddError("Cannot delete router configuration", err.Error())
	}
}

