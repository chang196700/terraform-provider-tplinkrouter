package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/chang196700/terraform-provider-tplinkrouter/internal/client"
	"strconv"
	"strings"
)

// advancedFields keeps the wire schema explicit while sharing the repetitive
// field construction. Each group has one Terraform type; it is not a JSON API.
func advancedFields(texts, integers, switches, bits, lists, secrets string) []Field {
	var fields []Field
	groups := []string{texts, integers, switches, bits, lists, secrets}
	for group, names := range groups {
		for _, wire := range strings.Fields(names) {
			name := strings.ReplaceAll(wire, "-", "_")
			if name[0] >= '0' && name[0] <= '9' {
				name = "radio_" + name
			}
			description := "Router " + strings.ReplaceAll(wire, "_", " ") + " setting."
			var field Field
			switch group {
			case 0:
				field = Text(name, wire, description)
			case 1:
				field = Integer(name, wire, description)
			case 2:
				field = Boolean(name, wire, description, "on", "off")
			case 3:
				field = Boolean(name, wire, description, "1", "0")
			case 4:
				field = Strings(name, wire, description)
			case 5:
				field = Secret(name+"_wo", wire, "Write-only "+strings.ReplaceAll(wire, "_", " ")+". Increment its version to rotate it.")
			}
			fields = append(fields, field)
		}
	}
	return fields
}

func advancedEntry(name, category, module, table, key, source string, fields []Field) Spec {
	for i := range fields {
		if fields[i].Wire == key {
			fields[i].Required = true
		}
	}
	return Spec{Name: name, Category: category, Description: "Manage " + strings.ReplaceAll(name, "_", " ") + " configuration.", Module: module, Table: table, Mode: Entry, NameField: key, Fields: fields, Source: source, Query: map[string]any{"para": map[string]any{"start": 0, "end": 99}}}
}

func advancedSettings(name, category, module, section, source string, fields []Field) Spec {
	return Spec{Name: name, Category: category, Description: "Manage " + strings.ReplaceAll(name, "_", " ") + ". Destroy releases management without resetting the device.", Module: module, Section: section, Mode: Settings, Fields: fields, Source: source}
}

func advancedIdentity(spec Spec, wire string) Spec {
	spec.IDField, spec.FilterField = wire, wire
	return spec
}

func advancedHardware(name, module, table, identity, source string, fields []Field) Spec {
	field := RequiredText(identity, identity, "Stable hardware identifier returned by the router. Hardware must already exist.")
	field.RequiresReplace = true
	spec := advancedEntry(name, "wireless", module, table, identity, source, append([]Field{field}, fields...))
	spec.Mode = Existing
	spec.OmitWriteFields = []string{identity}
	for i := range spec.Fields {
		if spec.Fields[i].Kind == Bool {
			spec.Fields[i].Decode = func(value any) (any, error) {
				if fmt.Sprint(value) == "-1" {
					return nil, nil
				}
				return value, nil
			}
		}
	}
	return advancedIdentity(spec, identity)
}

func advancedState(name, category, module, table, source string, fields []Field) Spec {
	spec := advancedEntry(name, category, module, table, "", source, fields)
	spec.Description = "Read " + strings.ReplaceAll(name, "_", " ") + " from the router."
	spec.DataSourceOnly = true
	return spec
}

func resolvePortal(ctx context.Context, c *client.Client, s Spec) (Spec, error) {
	r, err := c.Query(ctx, map[string]any{"authentication": map[string]any{"name": "portal_para"}})
	if err != nil {
		return s, err
	}
	module, ok := r["authentication"].(map[string]any)
	if !ok {
		return s, fmt.Errorf("portal mode response lacks authentication module")
	}
	settings, ok := module["portal_para"].(map[string]any)
	if !ok {
		return s, fmt.Errorf("portal mode response lacks portal_para section")
	}
	mode, _ := settings["auth_mode"].(string)
	if mode == "ssid" {
		s.IDField, s.FilterField = "serv_id", "serv_id"
	} else if mode == "vlan" || mode == "interface" {
		s.IDField, s.FilterField = "portal_id", "portal_id"
	} else {
		return s, fmt.Errorf("unsupported portal authentication mode %q", mode)
	}
	return s, nil
}

func portalSpec() Spec {
	f := advancedFields
	s := advancedEntry("portal", "authentication", "authentication", "portal_auth_list", "", "multi_portal.htm; remote_portal.htm; cmcc_portal.htm; cloud_portal.htm", f(
		"serv_id vlan_id interface jump_page_name default_page_type success_url fail_url note web_auth_server_type web_auth_server_group web_remind_type web_remind_page_title web_remind_page_content wechat_ssid wechat_shop_id wechat_app_id wechat_qrcode wechat_qrcode_pic_url wechat_token_server sms_channel_type sms_http_url sms_http_mode sms_http_code_type sms_http_template sms_username sms_template_id sms_signname sms_app_id remote_url remote_auth_server_type remote_auth_server_group cmcc_url cmcc_auth_server_type cmcc_auth_server_group cmcc_ac_name",
		"onekey_free_time web_free_time web_remind_time web_remind_interval wechat_free_time sms_free_time sms_auth_expiry remote_free_time remote_temp_free_time cmcc_free_time",
		"enable onekey_enable web_enable web_nosense_auth web_remind_enable wechat_enable wechat_force_concern sms_enable escape_enable remote_enable remote_nosense_auth cmcc_enable cmcc_nosense_auth cloud_portal_enable", "", "", "wechat_secret_key wechat_app_secret sms_password"))
	var filters []any
	for _, key := range []string{"web_enable", "onekey_enable", "wechat_enable", "sms_enable", "remote_enable", "cmcc_enable", "cloud_portal_enable"} {
		filters = append(filters, map[string]any{key: "on"}, map[string]any{key: "off"})
	}
	s.Query = map[string]any{"filter": filters, "para": map[string]any{"start": 0, "end": 99}}
	s.Resolve = resolvePortal
	s.Notes = "Import identity is serv_id in SSID authentication mode and portal_id otherwise. The router selects mode through portal_settings.auth_mode; changing global mode changes the meaning of existing portal entries."
	return s
}

// queryAdvancedTable follows the same inclusive pagination used by store.js.
// It is used where the UI joins parent and child tables to represent bindings.
func queryAdvancedTable(ctx context.Context, c *client.Client, module, table string, filter []any) ([]map[string]any, error) {
	var rows []map[string]any
	seen := map[string]bool{}
	for page := 0; page < 10000; page++ {
		query := map[string]any{"table": table, "para": map[string]any{"start": len(rows), "end": len(rows) + 99}}
		if filter != nil {
			query["filter"] = filter
		}
		response, err := c.Query(ctx, map[string]any{module: query})
		if err != nil {
			return nil, err
		}
		m, ok := response[module].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("binding query omitted module %s", module)
		}
		items, ok := m[table].([]any)
		if !ok {
			return nil, fmt.Errorf("binding query omitted table %s.%s", module, table)
		}
		added := 0
		for _, item := range items {
			entry, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("malformed row in %s.%s", module, table)
			}
			row := entry
			section := ""
			if len(entry) == 1 {
				for key, value := range entry {
					if object, yes := value.(map[string]any); yes {
						row = object
						section = key
					}
				}
			}
			if section == "" {
				section, _ = row[".name"].(string)
			}
			if section == "" {
				return nil, fmt.Errorf("binding query omitted stable section name for %s.%s", module, table)
			}
			if seen[section] {
				continue
			}
			seen[section] = true
			row[".name"] = section
			rows = append(rows, row)
			added++
		}
		count := len(rows)
		if counts, yes := m["count"].(map[string]any); yes {
			if raw, exists := counts[table]; exists {
				var err error
				count, err = strconv.Atoi(fmt.Sprint(raw))
				if err != nil {
					return nil, fmt.Errorf("invalid table count for %s.%s", module, table)
				}
			}
		}
		if len(rows) >= count {
			return rows, nil
		}
		if added == 0 {
			return nil, fmt.Errorf("binding pagination did not advance for %s.%s", module, table)
		}
	}
	return nil, fmt.Errorf("binding table exceeded pagination limit")
}

func bindingScalarList(value any) (any, error) {
	if list, ok := value.([]any); ok {
		if len(list) != 1 {
			return nil, fmt.Errorf("binding identity must contain one value")
		}
		return list, nil
	}
	if list, ok := value.([]string); ok {
		if len(list) != 1 {
			return nil, fmt.Errorf("binding identity must contain one value")
		}
		return []any{list[0]}, nil
	}
	return []any{value}, nil
}

func readWirelessBindings(ctx context.Context, c *client.Client, s Spec) ([]map[string]any, error) {
	services, err := queryAdvancedTable(ctx, c, "apmng_wserv", "wlan_serv", nil)
	if err != nil {
		return nil, err
	}
	var rows []map[string]any
	for _, service := range services {
		id, ok := service["serv_id"]
		if !ok {
			return nil, fmt.Errorf("wireless service omitted serv_id")
		}
		bindings, err := queryAdvancedTable(ctx, c, s.Module, s.Table, []any{map[string]any{"serv_id": id, "group_id": []any{"-1"}}})
		if err != nil {
			return nil, err
		}
		for _, row := range bindings {
			if fmt.Sprint(row["bind_status"]) == "1" {
				row["serv_id"] = id
				rows = append(rows, row)
			}
		}
	}
	return rows, nil
}

func readProtectionBindings(ctx context.Context, c *client.Client, s Spec) ([]map[string]any, error) {
	aps, err := queryAdvancedTable(ctx, c, s.Module, "ap_list", nil)
	if err != nil {
		return nil, err
	}
	var rows []map[string]any
	for _, ap := range aps {
		id, ok := ap["ap_id"]
		if !ok {
			return nil, fmt.Errorf("AP protection response omitted ap_id")
		}
		bindings, err := queryAdvancedTable(ctx, c, s.Module, s.Table, []any{map[string]any{"ap_id": []any{id}}})
		if err != nil {
			return nil, err
		}
		for _, row := range bindings {
			if fmt.Sprint(row["bind_status"]) == "bind" {
				row["ap_id"] = id
				rows = append(rows, row)
			}
		}
	}
	return rows, nil
}

func readAPStatus(ctx context.Context, c *client.Client, s Spec) ([]map[string]any, error) {
	groups, err := queryAdvancedTable(ctx, c, "apmng_set", "group_list", nil)
	if err != nil {
		return nil, err
	}
	var rows []map[string]any
	for _, group := range groups {
		id, ok := group["group_id"]
		if !ok {
			return nil, fmt.Errorf("AP group response omitted group_id")
		}
		members, err := queryAdvancedTable(ctx, c, s.Module, s.Table, []any{map[string]any{"group_id": id}})
		if err != nil {
			return nil, err
		}
		rows = append(rows, members...)
	}
	return rows, nil
}

func advancedBindings() []Spec {
	wireless := Spec{Name: "wireless_binding", Category: "wireless", Description: "Bind one wireless service to one existing radio. Destroy unbinds the service.", Module: "apmng_wserv", Table: "radio_bind_list", Mode: Entry, IDFields: []string{"serv_id", "radio_id"}, FilterFields: []string{"serv_id"}, CreateViaSet: true, CreateDefaults: map[string]any{"bind_status": "1"}, DeleteValues: map[string]any{"bind_status": "0"}, DeleteIdentityFields: []string{"radio_id"}, ReadRows: readWirelessBindings, Source: "apmng_wserv.htm", Fields: []Field{
		RequiredText("serv_id", "serv_id", "Wireless service identifier."), RequiredText("radio_id", "radio_id", "Radio identifier."), Integer("vlan_id", "vlan_id", "VLAN identifier, or zero for no tag."),
	}}
	wireless.WriteDefaults = map[string]any{"bind_status": "1"}
	wireless.OmitWriteFields = []string{"serv_id"}
	for i := range wireless.Fields {
		field := &wireless.Fields[i]
		if field.Wire == "radio_id" || field.Wire == "vlan_id" {
			field.Encode = bindingScalarList
		}
		if field.Wire != "vlan_id" {
			field.RequiresReplace = true
		}
	}
	var specs = []Spec{wireless}
	for _, kind := range []string{"dhcp", "arp"} {
		module, table, identity, source := "apmng_dhcp_defense", "dhcp_filter", "svr_id", "apmng_dhcp_defend.htm"
		if kind == "arp" {
			module, table, identity, source = "apmng_arpdefend", "usr_bind", "arp_id", "apmng_arpdefend_ap.htm"
		}
		spec := Spec{Name: "ap_" + kind + "_protection_binding", Category: "wireless", Description: "Bind one existing " + kind + " protection entry to an AP. Destroy removes only this binding.", Module: module, Table: table, Mode: Entry, IDFields: []string{"ap_id", identity}, FilterFields: []string{"ap_id", identity}, FilterListFields: []string{"ap_id"}, CreateViaSet: true, CreateDefaults: map[string]any{"bind_status": "bind"}, DeleteValues: map[string]any{"bind_status": "unbind"}, ReadRows: readProtectionBindings, Source: source, Fields: []Field{RequiredText("ap_id", "ap_id", "Existing AP identifier."), RequiredText(identity, identity, "Existing protection entry identifier.")}}
		for i := range spec.Fields {
			spec.Fields[i].RequiresReplace = true
		}
		spec.WriteDefaults = map[string]any{"bind_status": "bind"}
		spec.OmitWriteFields = []string{"ap_id", identity}
		specs = append(specs, spec)
	}
	return specs
}

// The template manager stores metadata separately from the local editor page.
// Reading the editor table requires a camelCase selector; updates use the
// snake_case selector below. Only pages already linked to a local template are
// managed because creating that linkage is a separate browser workflow.
func readPortalPages(ctx context.Context, c *client.Client, s Spec) ([]map[string]any, error) {
	templates, err := queryAdvancedTable(ctx, c, "authentication", "jump_page_list", nil)
	if err != nil {
		return nil, err
	}
	var rows []map[string]any
	seen := map[string]bool{}
	for _, template := range templates {
		if fmt.Sprint(template["jump_page_mode"]) != "local" {
			continue
		}
		id := fmt.Sprint(template["portalPageId"])
		if id == "" || id == "<nil>" {
			return nil, fmt.Errorf("local portal template response omitted portalPageId")
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		pages, err := queryAdvancedTable(ctx, c, s.Module, s.Table, []any{map[string]any{"portalPageId": id}})
		if err != nil {
			return nil, err
		}
		for _, page := range pages {
			for wire, read := range map[string]string{"portal_page_id": "portalPageId", "template_id": "templateId", "portal_page_name": "portalPageName", "portal_page_desc": "portalPageDesc"} {
				page[wire] = page[read]
			}
			rows = append(rows, page)
		}
	}
	return rows, nil
}

func portalPageSpec() Spec {
	config := []Field{
		Text("auth_title", "auth_title", "Authentication page title, at most 20 characters."),
		Text("auth_welcome", "auth_welcome", "Authentication welcome text, at most 26 characters."),
		Text("auth_copyright", "auth_copyright", "Copyright text, at most 50 characters."),
		Boolean("show_user_protocol", "showUserProtocol", "Show the user agreement.", "true", "false"),
		Text("agreement", "agreement", "User agreement text, at most 2500 characters."),
		Text("auth_back_img", "auth_back_img", "Existing authentication background image filename; file upload is not performed."),
		Text("auth_logo_img", "auth_logo_img", "Existing authentication logo filename; file upload is not performed."),
		Boolean("show_self_link", "show_self_link", "Show the custom link.", "true", "false"),
		Text("self_link_text", "self_link_text", "Custom link text, at most 20 characters."),
		Text("self_link_address", "self_link_address", "Custom link destination, at most 120 characters."),
		Text("success_title", "success_title", "Success page title, at most 20 characters."),
		Text("success_welcome", "success_welcome", "Success welcome text, at most 26 characters."),
		Text("success_back_img", "success_back_img", "Existing success background image filename; file upload is not performed."),
		Text("success_logo_img", "success_logo_img", "Existing success logo filename; file upload is not performed."),
		Boolean("front_page_enable", "front_page_enable", "Enable the pre-authentication page.", "on", "off"),
		Text("front_page_text", "front_page_text", "Pre-authentication page text, at most 20 characters."),
		Text("front_jump_way", "front_jump_way", "Pre-authentication page transition: manual or countdown."),
		Integer("front_jump_time", "front_jump_time", "Countdown duration in seconds."),
		Integer("front_img_swip_time", "front_img_swip_time", "Background image rotation interval in seconds; zero disables rotation."),
		Strings("front_page_back_img", "front_page_back_img", "Existing pre-authentication image filenames; file upload is not performed."),
	}
	limits := map[string]int{"auth_title": 20, "auth_welcome": 26, "auth_copyright": 50, "agreement": 2500, "self_link_text": 20, "self_link_address": 120, "success_title": 20, "success_welcome": 26, "front_page_text": 20}
	for i := range config {
		field := &config[i]
		field.MaxLength = limits[field.Wire]
		switch field.Wire {
		case "front_jump_way":
			field.Enum = []string{"manual", "countdown"}
		case "front_jump_time":
			field.Min, field.Max = 1, 60
		case "front_img_swip_time":
			field.Min, field.Max = 0, 10
		case "front_page_back_img":
			field.Decode = func(v any) (any, error) {
				if v == nil || v == "" {
					return []any{}, nil
				}
				if raw, ok := v.(string); ok {
					var images []any
					if err := json.Unmarshal([]byte(raw), &images); err != nil {
						return nil, fmt.Errorf("decode portal image filenames: %w", err)
					}
					return images, nil
				}
				return v, nil
			}
			field.Encode = func(v any) (any, error) {
				raw, err := json.Marshal(v)
				return string(raw), err
			}
		}
	}
	id := RequiredText("portal_page_id", "portal_page_id", "Existing local page identifier from the portal_templates data source.")
	id.RequiresReplace = true
	return Spec{
		Name: "portal_page", Category: "authentication", Description: "Edit an existing local portal page. Destroy releases management without deleting the page.",
		Module: "authentication", Table: "portal_page_list", Mode: Existing, IDField: "portal_page_id", FilterField: "portal_page_id",
		ReadRows: readPortalPages, OmitWriteFields: []string{"portal_page_id"}, WriteDefaults: map[string]any{"site_id": ""},
		PreserveWriteFields: []string{"template_id", "portal_page_name", "portal_page_desc"},
		Source:              "template.htm; local_template.htm; /web-static/js/cloud/cwm-device.js",
		Notes:               "Supports existing local pages only. The template manager creates metadata and editor content in separate stages; complete template creation, template deletion, cloud-hosted templates, and image uploads are not implemented. Omitted or unknown fields in the stored page configuration are preserved when updating known fields.",
		Fields: []Field{id, ReadText("template_id", "template_id", "Built-in template identifier; changing templates is not supported."),
			Text("portal_page_name", "portal_page_name", "Editor page name; separate from the template manager's display name."),
			Text("portal_page_desc", "portal_page_desc", "Editor page description."),
			{Name: "config", Wire: "config", Kind: Object, Description: "Typed local page appearance and text settings.", Fields: config, Decode: func(v any) (any, error) {
				if raw, ok := v.(string); ok {
					var result map[string]any
					if err := json.Unmarshal([]byte(raw), &result); err != nil {
						return nil, fmt.Errorf("decode local portal configuration: %w", err)
					}
					return result, nil
				}
				return v, nil
			}},
		},
	}
}

// AdvancedSpecs is derived from the authenticated VPN, authentication and AP
// management pages of TL-ER6220T 1.2.2 Build 240829 and the SSL VPN JS bundles.
func AdvancedSpecs() []Spec {
	f := advancedFields
	specs := []Spec{
		advancedSettings("ipsec_settings", "vpn", "vpn", "ipsec_global", "ipsec_tunnel.htm", f("", "", "route_enable", "", "", "")),
		advancedEntry("ipsec_tunnel", "vpn", "vpn", "ipsec_connection", "name", "ipsec_tunnel.htm", f(
			"name remote_peer ipsec_vpn_mode l3_proto bindif xfrmi_addr local_network remote_network ike_proposal_1 ike_proposal_2 ike_proposal_3 ike_proposal_4 exchange_mode connection_type local_id_type local_id_value remote_id_type remote_id_value mode ph2_proposal_1 ph2_proposal_2 ph2_proposal_3 ph2_proposal_4 pfs",
			"ike_lifetime dpd_interval xfrmi_mtu ph2_lifetime active_interval silent_interval", "enable auto_route dpd_enable workaround_enable", "", "", "psk")),
		advancedEntry("l2tp_server", "vpn", "vpn", "lns", "bindif", "l2tp_server.htm", f("bindif ipsecenc", "mtu", "enable", "", "", "presharekey")),
		advancedEntry("l2tp_client", "vpn", "vpn", "lac", "tunnelname", "l2tp_client.htm", f("tunnelname username server_ip outif ipsecenc workmode remotesubnet", "uplink_bandwidth downlink_bandwidth mtu", "enable", "", "", "password presharekey")),
		advancedEntry("pptp_server", "vpn", "vpn", "pns", "bindif", "pptp_server.htm", f("bindif mppeencryption", "mtu", "enable", "", "", "")),
		advancedEntry("pptp_client", "vpn", "vpn", "pac", "tunnelname", "pptp_client.htm", f("tunnelname username server_ip outif mppeencryption workmode remotesubnet", "uplink_bandwidth downlink_bandwidth mtu", "enable", "", "", "password")),
		advancedEntry("gre_tunnel", "vpn", "vpn", "gre", "tunnelname", "gre_tunnel.htm", f("tunnelname outif remote_peer gre_if_addr ipsecenc remotesubnet gre_key", "mtu keepalive_interval keepalive_retries uplink_bandwidth downlink_bandwidth", "enable duplicate_addr keepalive_enable gre_csm_enable", "", "", "presharekey")),
		advancedEntry("vpn_user", "vpn", "vpn", "user", "username", "vpn_user.htm", f("username type localip ippool dns netmode remotesubnet", "maxsessions", "", "", "", "password")),
		advancedSettings("ssl_vpn_server", "vpn", "ssl_vpn", "sslvpn_server_config", "sslvpnserver_global.htm", f("ippool first_dns_server second_dns_server auth_type", "port username_error_time username_expire_time ip_error_time ip_expire_time idle_time", "enable username_lock_enable ip_lock_enable vpn_redirect mult_auth_enable auth_security_enable", "", "service_if", "")),
		advancedSettings("ssl_vpn_radius", "vpn", "ssl_vpn", "sslvpn_remote_server", "radius_auth.htm", f("remote_radius_server_group", "", "", "", "", "")),
		advancedSettings("ssl_vpn_sms", "vpn", "ssl_vpn", "sslvpn_sms_portal", "sslvpn_sms_portal.htm", f("sms_channel_type sms_http_url sms_http_mode sms_http_code_type sms_http_template sms_username sms_template_id sms_signname sms_app_id", "sms_auth_expiry", "sms_enable", "", "", "sms_password")),
		advancedEntry("ssl_vpn_resource", "vpn", "ssl_vpn", "resource", "name", "resource_tunnel.htm", f("name type scope url service_type service_other proto sport dport logo_img", "icmp_type icmp_code", "", "", "group", "")),
		advancedEntry("ssl_vpn_resource_group", "vpn", "ssl_vpn", "resource_group", "name", "resource_tunnel.htm", f("name", "", "", "", "member_list", "")),
		advancedIdentity(advancedEntry("ssl_vpn_access_rule", "vpn", "ssl_vpn", "sslvpn_access_rule", "name", "resource-user-binding.htm", f("name", "", "", "", "user_list usergroup_list resource_list resource_group_list", "")), "entry_id"),
		advancedIdentity(advancedEntry("user_directory", "authentication", "usergroup", "user_directory", "name", "usergroup_user.htm", f("name comment user_source server_type server_addr base_dn account_username attr_password attr_username attr_user_filter attr_display_name attr_comment attr_organization attr_group attr_account_status attr_expired_time attr_telephone attr_email org_filter org_name org_comment group_filter group_name group_comment", "auth_port", "", "", "", "account_password")), "entry_id"),
		advancedIdentity(advancedEntry("user_group", "authentication", "usergroup", "user_group", "name", "usergroup_user.htm", f("name parent_directory_list parent_group_list", "", "", "", "", "")), "entry_id"),
		advancedIdentity(advancedEntry("ssl_vpn_user", "authentication", "usergroup", "user_userlist", "username", "usergroup_user.htm", f("username email telephone expire_time parent_group_list", "", "enable never_expire_enable", "", "", "password")), "entry_id"),
		advancedIdentity(advancedEntry("user_strategy", "authentication", "usergroup", "user_strategy", "name", "strategy-manage.htm", f("name comment", "pc_max_online_device mobile_max_online_device pc_max_online_time mobile_max_online_time", "", "", "user_list usergroup_list", "")), "entry_id"),
		advancedIdentity(advancedEntry("radius_server", "authentication", "authentication", "radius_list", "server_name", "radius.htm", f("server_name server_addr nas_ip nas_identifier radius_auth_type", "auth_port account_port retry_count retry_interval", "", "", "", "share_key")), "entry_id"),
		advancedIdentity(advancedEntry("authentication_server_group", "authentication", "authentication", "server_group_list", "group_name", "auth_server_group.htm", f("group_name server_type main_server backup_server note", "recovery_time", "", "", "", "")), "entry_id"),
		advancedIdentity(advancedEntry("ldap_server", "authentication", "authentication", "ldap_list", "server_name", "ldap.htm", f("server_name server_addr base_dn admin_dn user_filter user_passwd_filter", "auth_port", "status", "", "", "admin_passwd")), "entry_id"),
		advancedIdentity(advancedEntry("authentication_user", "authentication", "authentication", "userlist", "username", "usermngr_user.htm", f("username flag user_type expire_time allow_auth_time mac_bind_type mac name telephone note", "free_time allow_user up_limit down_limit", "enable", "", "", "password")), "entry_id"),
		advancedSettings("portal_settings", "authentication", "authentication", "portal_para", "portal_para.htm", f("auth_mode escape_strategy", "portal_port portal_https_port age_time cmcc_port cmcc_portal_port escape_interval", "age_enable https_enable ipoe_enable escape_enable", "", "", "")),
		advancedEntry("portal_free_rule", "authentication", "authentication", "free_auth_list", "name", "free_strategy.htm", f("name match_type url src_ip_set dst_ip_set src_mac src_vlan src_port dst_port protocol note flag version", "", "enable", "", "", "")),
		advancedSettings("ap_controller_settings", "wireless", "apmng_set", "settings", "apmng_set.htm", f("reboot_day reboot_time", "", "auto_reboot auto_firm apmngr_status", "", "", "")),
		advancedIdentity(advancedEntry("ap_group", "wireless", "apmng_set", "group_list", "name", "apmng_set.htm", f("name", "", "", "", "", "")), "group_id"),
		advancedHardware("ap", "apmng_set", "ap_list", "entry_id", "apmng_set.htm", f("entry_name group_id username led_close_date led_close_time led_open_date led_open_time", "ap_keep_alive client_keep_alive client_idle phy_wire_vlan_1 phy_wire_vlan_2 phy_wire_vlan_3 phy_wire_vlan_4", "offline_selfmanage led_wifi_sync led_timer_enable", "led ap_trunk fit_fat_button_status", "", "password")),
		advancedHardware("radio", "apmng_rf", "rf_entry", "rf_id", "apmng_rdmngr.htm", f("mode bandwidth channel power antenna manage_frame_rate", "max_usr frag_thr bcn_intvl rts_thr dtim_prd rssi_restrict_val rssi_kick_val rf_check_circle rf_chnl_occup_threshold rf_chnl_tolerance reject_limit reject_diff", "rf_state wmm brd_probe short_gi rssi_restrict rssi_kick rf_online_switch airtime reject_enable", "rf_chnl_dcs", "", "")),
		advancedIdentity(advancedEntry("wireless_service", "wireless", "apmng_wserv", "wlan_serv", "ssid", "apmng_wserv.htm", f("ssid ssid_code_type desc encryption auth cipher radius_ip bw_ctrl_mode default_bind_freq", "radius_port radius_acct_port key_update_intv up_limit down_limit default_bind_vlan", "enable auto_bind", "isolate ssidbrd bw_ctrl_enable", "", "radius_pwd key")), "serv_id"),
		advancedSettings("wireless_roaming", "wireless", "apmng_roam", "roam", "apmng_roaming.htm", f("2g_roaming_threshold_type 5g_roaming_threshold_type", "2g_rssi_trigger_threshold 2g_low_rssi_threshold 2g_rate_trigger_threshold 2g_low_rate_threshold 5g_rssi_trigger_threshold 5g_low_rssi_threshold 5g_rate_trigger_threshold 5g_low_rate_threshold threshold_check_interval roaming_dif blacklist_restrict_time stats_interval", "11k_switch 11v_switch 11r_switch 2g_low_rssi_kick 2g_low_rate_kick 5g_low_rssi_kick 5g_low_rate_kick stats_switch", "", "", "")),
		advancedSettings("radio_management", "wireless", "apmng_rrm", "settings", "apmng_rrm.htm", f("bw_2g bw_5g autorrm_date autorrm_time", "pwr_threshold pwr_max_pwr pwr_min_pwr", "chnl_enable pwr_enable autorrm_enable", "", "chnl_2g_set chnl_5g1_set", "")),
		advancedSettings("mesh_settings", "wireless", "apmng_mesh", "settings", "apmng_mesh_advanced_setting.htm; apmng_mesh_dev_list.htm", f("backhaul_ssid", "", "enable re_auto_switch", "", "", "backhaul_key")),
		advancedIdentity(advancedEntry("ap_dhcp_server", "wireless", "apmng_dhcp_defense", "dhcp_filter", "name", "apmng_dhcp_server_list.htm", f("name addr_type ip", "", "", "", "", "")), "svr_id"),
		advancedIdentity(advancedEntry("ap_arp_binding", "wireless", "apmng_arpdefend", "usr_bind", "ip", "apmng_arpdefend_arp.htm", f("description ip mac", "", "", "", "", "")), "arp_id"),
		advancedHardware("ap_dhcp_protection", "apmng_dhcp_defense", "ap_list", "ap_id", "apmng_dhcp_defend.htm", f("", "", "enable", "", "", "")),
		advancedHardware("ap_arp_protection", "apmng_arpdefend", "ap_list", "ap_id", "apmng_arpdefend_ap.htm", f("", "", "enable", "", "", "")),
		advancedSettings("ap_audit", "wireless", "apmng_audit", "settings", "apmng_audit.htm", f("server_type", "sniffing_report_interval sniffing_report_max", "enable", "", "", "")),
		advancedSettings("ap_report", "wireless", "apmng_report", "settings", "apmng_report.htm", f("protocol", "interval max_load", "state", "", "apgroup content", "")),
		advancedSettings("ap_log_settings", "wireless", "apmng_set", "aplog_setting", "dia_info.htm", f("ac_log_level remote_log_level", "", "ac_log_switch remote_server_switch", "", "", "")),
		advancedState("ipsec_security_associations", "vpn", "vpn", "ipsec_sa", "ipsec_sa.htm", f("name xfrmi spi direction tunnel flows encapsule ah_auth_alg esp_auth_alg encapsule_alg", "", "", "", "", "")),
		advancedState("l2tp_tunnels", "vpn", "vpn", "l2tp_tunnel_info", "l2tp_tunnel.htm", f("name isserver username localip remoteip ourtid peertid ourcid peercid localpppip remotepppip remotehostname ifname connection dns", "", "", "", "", "")),
		advancedState("pptp_tunnels", "vpn", "vpn", "pptp_tunnel_info", "pptp_tunnel.htm", f("isserver username ifname localpppip remoteip remotepppip dns", "", "", "", "", "")),
		advancedState("gre_tunnels", "vpn", "vpn", "gre_status", "gre_status.htm", f("tunnelname outif remote_peer keepavlive_state", "", "", "", "", "")),
		advancedState("ssl_vpn_clients", "vpn", "ssl_vpn", "client_list", "sslvpn_running.htm", f("username telephone virtual_ip online_timestamp remote_ip", "rx_traffic rx_rate tx_traffix tx_rate port", "", "", "", "")),
		advancedState("portal_templates", "authentication", "authentication", "jump_page_list", "template.htm", append(f("jump_page_mode jump_page_name note", "", "", "", "", ""), ReadText("portal_page_id", "portalPageId", "Identifier of the page backing this portal template."))),
		advancedState("authentication_sessions", "authentication", "authentication", "auth_session_list", "sessmngr.htm", f("auth_type auth_server_type username mac ip vlan_id serv_id ssid interface auth_time remain_time", "", "", "", "", "")),
		advancedState("wireless_clients", "wireless", "apmng_client", "client_list", "apmng_wstation.htm", f("entry_id mac ap_name freq_unit freq_name ip ipv6 ssid encode vlan_id connect_date connect_time rssi", "", "", "", "", "")),
		advancedState("ap_status", "wireless", "apmng_status", "ap_list", "apmng_status.htm", f("entry_id entry_name model_name mac ip link_status led_status online_time", "", "", "", "", "")),
	}
	specs = append(specs, portalSpec(), portalPageSpec())
	specs = append(specs, advancedBindings()...)
	for _, tunnel := range []string{"l2tp_server", "l2tp_client", "pptp_server", "pptp_client"} {
		protocol := strings.Split(tunnel, "_")[0]
		specs = append(specs, advancedSettings(tunnel+"_settings", "vpn", "vpn", tunnel+"_global", tunnel+".htm", f("", "lcpechointerval "+protocol+"hellointerval", "", "", "", "")))
	}
	for i := range specs {
		s := &specs[i]
		switch s.Name {
		case "ssl_vpn_resource", "ssl_vpn_resource_group":
			s.Protected = map[string][]string{"issys": {"1"}}
			s.CreateDefaults = map[string]any{"issys": "0"}
		case "user_directory", "user_group", "ssl_vpn_user", "user_strategy", "ssl_vpn_access_rule":
			s.Protected = map[string][]string{"is_sys": {"1"}}
			s.CreateDefaults = map[string]any{"is_sys": "0"}
		case "ap_group":
			s.Protected = map[string][]string{"group_id": {"0"}}
		case "portal_free_rule":
			s.FilterField = "key"
		case "authentication_user":
			s.Filter = map[string]any{"flag": ""}
		case "ap_status":
			s.ReadRows = readAPStatus
		case "wireless_roaming":
			for j := range s.Fields {
				if s.Fields[j].Wire == "2g_roaming_threshold_type" || s.Fields[j].Wire == "5g_roaming_threshold_type" {
					s.Fields[j].Kind = StringList
				}
			}
		}
	}
	return specs
}

