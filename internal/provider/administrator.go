package provider

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"unicode/utf8"

	"github.com/chang196700/terraform-provider-tplinkrouter/internal/catalog"
	"github.com/chang196700/terraform-provider-tplinkrouter/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var adminSpec = catalog.Spec{Name: "administrator", Mode: catalog.Settings, Description: "Manage the currently authenticated administrator. Destroy only releases management. After rotation, update the provider credential source before the next Terraform command; the provider never edits env_file.", Fields: []catalog.Field{
	catalog.Text("username", "username", "Administrator login name. Defaults to the currently authenticated account."),
	catalog.Secret("password_wo", "password", "New administrator password, 5 to 31 characters. Never stored in state."),
}}

type administratorResource struct{ conn *connection }

func (r *administratorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_administrator"
}
func (r *administratorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema(adminSpec)
}
func (r *administratorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.conn = configureConnection(req.ProviderData, &resp.Diagnostics)
}
func (r *administratorResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	cfg := attributes(ctx, req.Config, &resp.Diagnostics)
	validateSecrets(adminSpec.Fields, cfg, path.Empty(), &resp.Diagnostics)
	if p, ok := cfg["password_wo"].(types.String); ok && !p.IsNull() && !p.IsUnknown() {
		n := utf8.RuneCountInString(p.ValueString())
		if n < 5 || n > 31 {
			resp.Diagnostics.AddAttributeError(path.Root("password_wo"), "Invalid administrator password", "The router requires 5 to 31 characters.")
		}
	}
	if u, ok := cfg["username"].(types.String); ok && !u.IsNull() && !u.IsUnknown() && u.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(path.Root("username"), "Invalid administrator username", "Username must not be empty.")
	}
}
func (r *administratorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID != "settings" {
		resp.Diagnostics.AddError("Invalid import ID", "Import the current administrator using ID settings.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue("settings"))...)
}
func (r *administratorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	prior := attributes(ctx, req.State, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.conn == nil {
		resp.Diagnostics.AddError("Unconfigured provider", "Router client is unavailable.")
		return
	}
	err := r.conn.withOperation(ctx, func() error {
		if err := r.conn.discover(ctx); err != nil {
			return err
		}
		if err := r.conn.client.Login(ctx); err != nil {
			return err
		}
		u, _ := r.conn.client.Credentials()
		return setRowState(ctx, adminSpec, routerRow{ID: "settings", Data: map[string]any{"username": u}}, prior, &resp.State, &resp.Diagnostics)
	})
	if err != nil {
		resp.Diagnostics.AddError("Cannot read administrator", err.Error())
	}
}
func (r *administratorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.change(ctx, req.Plan, req.Config, nil, &resp.State, &resp.Diagnostics)
}
func (r *administratorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	prior := attributes(ctx, req.State, &resp.Diagnostics)
	r.change(ctx, req.Plan, req.Config, prior, &resp.State, &resp.Diagnostics)
}
func (r *administratorResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {
}

func (r *administratorResource) change(ctx context.Context, planSource, configSource objectGetter, prior map[string]attr.Value, state *tfsdk.State, diags *diag.Diagnostics) {
	plan := attributes(ctx, planSource, diags)
	cfg := attributes(ctx, configSource, diags)
	if diags.HasError() {
		return
	}
	if r.conn == nil {
		diags.AddError("Unconfigured provider", "Router client is unavailable.")
		return
	}
	err := r.conn.withOperation(ctx, func() error {
		if err := r.conn.discover(ctx); err != nil {
			return err
		}
		oldUser, oldPassword := r.conn.client.Credentials()
		newUser := knownString(plan, "username")
		if newUser == "" {
			newUser = oldUser
		}
		newPassword := oldPassword
		para, err := fieldsToWire(adminSpec.Fields, plan, cfg, prior, prior == nil)
		if err != nil {
			return err
		}
		if v, ok := para["password"]; ok {
			newPassword = wireString(v)
		}
		changed := newUser != oldUser || newPassword != oldPassword
		if changed {
			_, mutationErr := r.conn.client.Mutate(ctx, map[string]any{"method": "set", "administration": map[string]any{"modify_login_pwd": map[string]any{"old_acc": oldUser, "old_pwd": client.EncodePassword(oldPassword), "new_acc": newUser, "new_pwd": client.EncodePassword(newPassword)}}})
			if mutationErr != nil {
				var apiErr *client.APIError
				if errors.As(mutationErr, &apiErr) || errors.Is(mutationErr, client.ErrReadOnly) {
					return mutationErr
				}
			}
			if err := r.conn.client.RotateCredentials(ctx, newUser, newPassword); err != nil {
				return fmt.Errorf("administrator update was submitted but a new session could not be verified; verify the credential source before retrying: %w", err)
			}
			if mutationErr != nil {
				diags.AddWarning("Administrator credentials verified", "The original response was lost, but a new session with the requested credentials succeeded. The change was not repeated.")
			}
		}
		if err := setRowState(ctx, adminSpec, routerRow{ID: "settings", Data: map[string]any{"username": newUser}}, plan, state, diags); err != nil {
			return err
		}
		if changed {
			diags.AddWarning("Update your credential source", "Administrator credentials changed. Update env_file or the provider's credential environment variables before the next Terraform command.")
		}
		return (&routerResource{conn: r.conn}).save(ctx, changed)
	})
	if err != nil {
		diags.AddError("Cannot update administrator", err.Error())
	}
}

