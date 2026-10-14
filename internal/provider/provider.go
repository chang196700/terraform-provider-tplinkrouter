package provider

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/chang196700/terraform-provider-tplinkrouter/internal/catalog"
	"github.com/chang196700/terraform-provider-tplinkrouter/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type routerProvider struct{ version string }

type providerModel struct {
	Model    types.String `tfsdk:"model"`
	Endpoint types.String `tfsdk:"endpoint"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
	EnvFile  types.String `tfsdk:"env_file"`
	Timeout  types.String `tfsdk:"timeout"`
	Insecure types.Bool   `tfsdk:"insecure_skip_verify"`
	Legacy   types.Bool   `tfsdk:"allow_legacy_tls"`
	AutoSave types.Bool   `tfsdk:"auto_save"`
	ReadOnly types.Bool   `tfsdk:"read_only"`
}

type connection struct {
	client                    *client.Client
	autoSave                  bool
	expectedModel             string
	expectedHW                string
	mu                        sync.Mutex
	discovered                bool
	model, hardware, firmware string
	capabilities              map[string]bool
}

func New(version string) func() provider.Provider {
	return func() provider.Provider { return &routerProvider{version: version} }
}
func (p *routerProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "tplinkrouter"
	resp.Version = p.version
}
func (p *routerProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Manage a TP-LINK TL-ER6220T router through its local SLP API.", Attributes: map[string]schema.Attribute{
		"model":                schema.StringAttribute{Required: true, Description: "Router model identifier. The only supported value is tl-er6220t (TP-LINK TL-ER6220T v1.0)."},
		"endpoint":             schema.StringAttribute{Optional: true, Description: "Router base URL (HTTP or HTTPS). Defaults to https://192.168.1.1. HTTPS never falls back to HTTP."},
		"username":             schema.StringAttribute{Optional: true, Description: "Administrator username. Overrides TPLINK_ROUTER_USERNAME and USERNAME in env_file."},
		"password":             schema.StringAttribute{Optional: true, Sensitive: true, Description: "Administrator password. Overrides TPLINK_ROUTER_PASSWORD and PASSWORD in env_file."},
		"env_file":             schema.StringAttribute{Optional: true, Description: "Explicit path to a dotenv file containing USERNAME and PASSWORD. No file is loaded implicitly."},
		"timeout":              schema.StringAttribute{Optional: true, Description: "Per-request timeout, as a Go duration. Defaults to 30s."},
		"insecure_skip_verify": schema.BoolAttribute{Optional: true, Description: "Explicitly disable HTTPS certificate verification. Default false."},
		"allow_legacy_tls":     schema.BoolAttribute{Optional: true, Description: "Explicitly enable the router's legacy RSA AES-GCM TLS 1.2 cipher suites. Does not enable TLS 1.0/1.1. Default false."},
		"auto_save":            schema.BoolAttribute{Optional: true, Description: "Save all pending router configuration after each confirmed mutation. This also persists other unsaved changes on the device. Default false; reads never save."},
		"read_only":            schema.BoolAttribute{Optional: true, Description: "Reject every configuration mutation, including save. Use for inspection/import/plan validation. Default false."},
	}}
}

func (p *routerProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for name, v := range map[string]types.String{"model": cfg.Model, "endpoint": cfg.Endpoint, "username": cfg.Username, "password": cfg.Password, "env_file": cfg.EnvFile, "timeout": cfg.Timeout} {
		if v.IsUnknown() {
			resp.Diagnostics.AddError("Unknown provider configuration", name+" must be known before the provider can connect.")
		}
	}
	for name, v := range map[string]types.Bool{"insecure_skip_verify": cfg.Insecure, "allow_legacy_tls": cfg.Legacy, "auto_save": cfg.AutoSave, "read_only": cfg.ReadOnly} {
		if v.IsUnknown() {
			resp.Diagnostics.AddError("Unknown provider configuration", name+" must be known before the provider can connect.")
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}
	model := strings.ToLower(strings.TrimSpace(cfg.Model.ValueString()))
	if model != "tl-er6220t" {
		resp.Diagnostics.AddError("Unsupported router model", "model must be tl-er6220t, the identifier for a TP-LINK TL-ER6220T v1.0 router.")
		return
	}
	env := map[string]string{}
	if !cfg.EnvFile.IsNull() {
		var err error
		env, err = client.LoadEnvFile(cfg.EnvFile.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Cannot load credentials", "Unable to read or parse env_file. Check the path and dotenv syntax.")
			return
		}
	}
	choose := func(v types.String, envKey, fileKey string) string {
		if !v.IsNull() {
			return v.ValueString()
		}
		if s := os.Getenv(envKey); s != "" {
			return s
		}
		return env[fileKey]
	}
	endpoint := cfg.Endpoint.ValueString()
	if cfg.Endpoint.IsNull() {
		endpoint = "https://192.168.1.1"
	}
	timeout := 30 * time.Second
	if !cfg.Timeout.IsNull() {
		var err error
		timeout, err = time.ParseDuration(cfg.Timeout.ValueString())
		if err != nil || timeout <= 0 {
			resp.Diagnostics.AddError("Invalid timeout", "timeout must be a positive duration such as 30s.")
			return
		}
	}
	c, err := client.New(client.Config{Endpoint: endpoint, Username: choose(cfg.Username, "TPLINK_ROUTER_USERNAME", "USERNAME"), Password: choose(cfg.Password, "TPLINK_ROUTER_PASSWORD", "PASSWORD"), Timeout: timeout, Insecure: cfg.Insecure.ValueBool(), AllowLegacyTLS: cfg.Legacy.ValueBool(), ReadOnly: cfg.ReadOnly.ValueBool()})
	if err != nil {
		resp.Diagnostics.AddError("Invalid provider configuration", err.Error())
		return
	}
	conn := &connection{client: c, autoSave: cfg.AutoSave.ValueBool(), expectedModel: "TL-ER6220T", expectedHW: "1.0"}
	resp.ResourceData = conn
	resp.DataSourceData = conn
}

func (p *routerProvider) Resources(_ context.Context) []func() resource.Resource {
	factories := []func() resource.Resource{func() resource.Resource { return &administratorResource{} }}
	for _, s := range catalog.All() {
		if s.DataSourceOnly {
			continue
		}
		spec := s
		factories = append(factories, func() resource.Resource { return &routerResource{spec: spec} })
	}
	return factories
}
func (p *routerProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	factories := []func() datasource.DataSource{func() datasource.DataSource { return &deviceDataSource{} }}
	for _, s := range catalog.All() {
		spec := s
		factories = append(factories, func() datasource.DataSource { return &routerDataSource{spec: spec} })
	}
	return factories
}

func configureConnection(data any, diags *diag.Diagnostics) *connection {
	if data == nil {
		return nil
	}
	c, ok := data.(*connection)
	if !ok {
		diags.AddError("Provider configuration error", "Unexpected provider client type.")
		return nil
	}
	return c
}

func (c *connection) discover(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.discovered {
		return nil
	}
	info, err := c.client.Query(ctx, map[string]any{"device_info": map[string]any{"name": "info"}})
	if err != nil {
		return err
	}
	d, _ := info["device_info"].(map[string]any)
	v, _ := d["info"].(map[string]any)
	c.model = wireString(v["device_model"])
	c.hardware = wireString(v["hw_version"])
	c.firmware = wireString(v["sw_version"])
	if !strings.EqualFold(c.model, c.expectedModel) {
		return fmt.Errorf("unsupported device model %q; provider model tl-er6220t requires TL-ER6220T", c.model)
	}
	if c.hardware != c.expectedHW {
		return fmt.Errorf("unsupported hardware version %q; provider model tl-er6220t requires TL-ER6220T v1.0", c.hardware)
	}
	features, err := c.client.Query(ctx, map[string]any{"function": map[string]any{"name": "new_module_spec"}})
	if err != nil {
		return err
	}
	d, _ = features["function"].(map[string]any)
	v, _ = d["new_module_spec"].(map[string]any)
	c.capabilities = map[string]bool{}
	if list, ok := v["new_module_list"].([]any); ok {
		for _, name := range list {
			c.capabilities[wireString(name)] = true
		}
	}
	c.discovered = true
	return nil
}
func (c *connection) check(ctx context.Context, s catalog.Spec) error {
	if c == nil {
		return fmt.Errorf("provider client is not configured")
	}
	if err := c.discover(ctx); err != nil {
		return err
	}
	if s.Capability != "" && !c.capabilities[s.Capability] {
		return fmt.Errorf("%s requires device capability %s, which this firmware does not expose", s.Name, s.Capability)
	}
	return nil
}

// Each public operation ends its own authenticated session, so short-lived
// Terraform plugin processes do not leave administrative sessions behind.
func (c *connection) withOperation(ctx context.Context, operation func() error) error {
	return c.client.WithLock(ctx, func() error {
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = c.client.Logout(cleanup)
		}()
		return operation()
	})
}

