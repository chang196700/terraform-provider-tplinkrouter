package catalog

import (
	"context"
	"fmt"

	"github.com/chang196700/terraform-provider-tplinkrouter/internal/client"
)

// readDNSGlobal uses the names form used by dns_proxy.htm. A singleton DNS
// setting is returned beside the rule tables, so treating `global` as a table
// (the generic settings query shape) produces an empty list on this firmware.
func readDNSGlobal(ctx context.Context, c *client.Client, s Spec) ([]map[string]any, error) {
	response, err := c.Query(ctx, map[string]any{"dns": map[string]any{
		"name": []string{"global", "rule", "dns_policy", "dns_map"},
	}})
	if err != nil {
		return nil, err
	}
	module, ok := response["dns"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("DNS response omitted dns module")
	}
	raw, ok := module["global"]
	if !ok {
		return nil, fmt.Errorf("DNS response omitted global settings")
	}
	// Firmware revisions have returned both an object and a one-element list
	// for the named singleton. Accept either wire representation.
	if row, ok := raw.(map[string]any); ok {
		row[".name"] = "global"
		return []map[string]any{row}, nil
	}
	if list, ok := raw.([]any); ok {
		if len(list) == 0 {
			// DNS is disabled on some firmware states and the named singleton is
			// represented by an empty list. Keep the singleton identity so the
			// data source remains readable with null settings.
			return []map[string]any{{".name": "global"}}, nil
		}
		for _, item := range list {
			if row, ok := item.(map[string]any); ok {
				row[".name"] = "global"
				return []map[string]any{row}, nil
			}
		}
	}
	return nil, fmt.Errorf("DNS global settings returned an unsupported shape")
}

// SystemSpecs describes persistent services separately from operational actions.
// Reboot, restore, firmware upload, diagnostics and cloud-account binding are not
// Terraform resources: their management pages expose one-shot operations.
func SystemSpecs() []Spec {
	f := advancedFields
	specs := []Spec{
		advancedSettings("pppoe_server", "services", "pppoe_server", "setting", "pppoeserver_global.htm", f("first_dns_server second_dns_server", "max_sessions max_noreply_lcp idle_time", "enable force_enable user_isolation", "", "service_if auth_type", "")),
		advancedEntry("pppoe_user", "services", "pppoe_server", "user", "username", "pppoeserver_user.htm", f("username ip_pool expire_time bandwidth_mode note mac_bind_type bind_mac", "max_sessions uplink_bandwidth downlink_bandwidth timing_time", "enable adv_enable", "", "", "password")),
		advancedEntry("pppoe_exception", "services", "pppoe_server", "exceptip", "start_ip", "pppoeserver_exceptip.htm", f("start_ip end_ip note", "", "enable", "", "", "")),
		advancedSettings("dns_settings", "services", "dns", "global", "dns_proxy.htm", f("", "dns_ttl", "", "", "", "")),
		advancedEntry("dns_proxy_rule", "services", "dns", "rule", "name", "dns_proxy.htm", f("name out_if service_if", "", "", "", "", "")),
		advancedEntry("dns_policy", "services", "dns", "dns_policy", "domain", "dns_policy.htm", f("domain userif", "", "enable", "", "", "")),
		advancedEntry("dns_mapping", "services", "dns", "dns_map", "domain", "dns_map.htm", f("domain ip", "", "enable", "", "", "")),
		advancedEntry("ddns_oray", "services", "phddns", "phddns", "interface", "phddns.htm", f("interface username", "", "enable", "", "", "password")),
		advancedEntry("ddns_comexe", "services", "cmxddns", "cmxddns", "interface", "cmxddns.htm", f("interface username", "", "enable", "", "", "password")),
		advancedEntry("ddns_3322", "services", "dyn3322ddns", "dyn3322ddns", "interface", "dyn3322ddns.htm", f("interface username domain", "", "enable", "", "", "password")),
		advancedSettings("upnp_settings", "services", "upnpd", "config", "upnp.htm", f("", "", "enable_upnp", "", "internal_iface external_iface", "")),
		advancedSettings("ftp_server", "services", "ftp", "ftpd", "storage_share.htm", f("server_if", "", "", "enable", "", "")),
		advancedSettings("storage_credentials", "services", "nas", "security_1", "storage_share.htm", f("username", "", "", "anonymous", "", "password")),
		advancedSettings("vrrp_settings", "high_availability", "keepalived", "setting", "keepalived.htm", f("vrrp_interface", "", "vrrp_interface_enable", "", "", "")),
		advancedEntry("vrrp_rule", "high_availability", "keepalived", "rule", "name", "keepalived.htm", f("name state effect_interface monitor_interface mode virtual_ipaddress detect_ip", "virtual_router_id priority advert_int", "enable", "", "", "")),
		// These are the fields returned by the firmware's trp_data singleton.
		// Peer/heartbeat controls shown by newer UI builds are not present on this
		// model and are intentionally not invented here.
		advancedSettings("high_availability", "high_availability", "ha_mngt", "trp_data", "ha.htm", append(f("mode role slot_info static_route_backup", "", "enable preempt", "", "", ""),
			Integer("cluster_id", "cluster_id", "HA cluster identifier."), Integer("fault_detect_group_num", "fault_detect_group_num", "Fault detection group number."),
			Integer("hello_timeout", "hello_timeout", "HA heartbeat timeout."), Integer("peer_ip_num", "peer_ip_num", "Configured peer address count."))),
		advancedEntry("ha_virtual_router", "high_availability", "ha_mngt", "vrrp", "name", "ha.htm", f("name interface virtual_ipaddress virtual_ipaddress_mask virtual_mac", "priority vrid", "", "", "", "")),
		advancedEntry("nqa_probe", "high_availability", "nqa", "check_case", "name", "nqa_case.htm", f("name dst type interface domain", "probe_interval timeout retry dst_port src_port packet_interval packet_num delay lose jitter udp_jitter", "nqa_enable", "", "", "")),
		advancedEntry("nqa_suite", "high_availability", "nqa", "check_suit", "name", "nqa_suit.htm", f("name", "threshold", "", "", "case_id_list", "")),
		advancedSettings("management_settings", "system", "system", "service_port", "account_config.htm", f("", "http_port https_port web_timeout max_trytimes lock_time", "http_port_switch", "", "", "")),
		advancedEntry("remote_management_rule", "system", "system", "remote_rule", "ipaddr", "remote_mngt.htm", f("ipaddr", "", "", "enabled", "", "")),
		advancedSettings("device_settings", "system", "system", "sys", "system_mgt.htm", f("dev_alias", "", "", "", "", "")),
		advancedSettings("route_metrics", "system", "system", "metric", "metric.htm", f("", "static dhcp pppoe l2tp pptp", "", "", "", "")),
		advancedSettings("connection_tracking", "system", "system", "sys", "xt_socket2.htm", f("internal_iface", "", "", "", "", "")),
		advancedSettings("telnet_settings", "system", "diagnose", "settings", "telnet_mngt.htm", f("", "telnet_port", "enable", "", "", "")),
		advancedSettings("fault_diagnostics_settings", "system", "system", "fault_diagnose", "dia_info.htm", f("", "", "enable", "", "", "")),
		advancedSettings("time_settings", "system", "system", "ntp", "time_setting.htm", f("pri_ntp snd_ntp", "timezone", "", "", "", "")),
		advancedSettings("syslog_settings", "system", "system", "syslog", "system_log.htm", f("level module send_server", "", "level_switch module_switch log_send", "", "", "")),
		advancedSettings("private_cloud_server", "system", "cloud_config", "tums_info", "cloud_management.htm", f("server comment", "port", "", "", "", "")),
		advancedSettings("ucl_settings", "services", "ucl", "global", "ucl.htm", f("", "", "enable", "", "", "")),
		advancedSettings("ucl_authentication", "services", "ucl_auth", "settings", "ucl_auth.htm", f("", "", "enable", "", "", "")),
		advancedState("ucl_policies", "services", "ucl", "ucl_policy_list", "ucl.htm", f("policy_name comment obj_time_id band_width", "", "", "", "obj_user_list access_list", "")),
		advancedState("pppoe_sessions", "services", "pppoe_server", "session", "pppoeserver_session.htm", f("pid username state ip mac connected_time note", "", "", "", "", "")),
		advancedState("upnp_leases", "services", "upnpd", "upnp_lease", "upnp.htm", f("desc proto client ext_port inner_port enable", "", "", "", "if", "")),
		advancedState("storage_mounts", "services", "nas", "mount_info", "storage_share.htm", f("devname address", "", "", "", "", "")),
		advancedState("tplink_ddns_domains", "services", "ddns", "ddns_list", "tpddns.htm", f("domain bind_status is_cur_host", "", "", "", "", "")),
	}
	for i := range specs {
		s := &specs[i]
		if s.Name == "dns_settings" {
			s.ReadRows = readDNSGlobal
		}
		if s.Name == "time_settings" {
			mode := Text("type", "type", "Time synchronization mode: auto or cloud where supported. Manual clock changes are operational actions.")
			mode.Section = "time_type"
			s.Fields = append(s.Fields, mode)
		}
	}
	cloud := advancedSettings("cloud_status", "system", "cloud_config", "conf_mngt", "cloud_management.htm", f("mngt_switch", "", "", "", "", ""))
	cloud.DataSourceOnly = true
	cloud.Notes = "Cloud-management mode changes can replace AP and authentication configuration; account binding, restore and mode-transition workflows are deliberately excluded. Private-cloud endpoint settings are separately managed."
	ha := advancedSettings("high_availability_status", "high_availability", "ha_mngt", "ha_state", "ha_status.htm", f("work_mode work_role heartbeat_interface preempt consistency", "", "", "", "", ""))
	ha.DataSourceOnly = true
	specs = append(specs, cloud, ha)
	return specs
}

