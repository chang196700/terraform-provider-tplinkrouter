package catalog

// SecuritySpecs describes the firewall and behavior-control pages shipped by the
// device. Ordered rule tables have exactly one authoritative Terraform resource.
func SecuritySpecs() []Spec {
	return []Spec{
		{
			Name: "arp_defense", Description: "ARP defense and gratuitous-ARP settings.", Category: "Security", Module: "arp_defense", Section: "global", Mode: Settings, Source: "/userrpm/imb.htm",
			Fields: []Field{netBool01("enabled", "enable", "Enable ARP defense."), netBool01("send_gratuitous_arp", "garp", "Send gratuitous ARP announcements when an ARP attack is detected."), netBool01("require_ip_mac_binding", "imb_pass", "Allow only packets matching an IP/MAC binding."), Integer("announcement_interval_ms", "interval", "Gratuitous ARP announcement interval in milliseconds."), Strings("interfaces", "interface", "Interfaces on which ARP defense applies.")},
		},
		{
			Name: "ip_mac_binding", Description: "An IP/MAC binding used by ARP defense.", Category: "Security", Module: "ip_mac_bind", Table: "user_bind", Mode: Entry, IDFields: []string{"ip", "mac", "interface"}, Source: "/userrpm/imb.htm",
			Fields: []Field{netIP(RequiredText("ip_address", "ip", "Bound IPv4 address."), "ipv4"), netIP(RequiredText("mac_address", "mac", "Bound client MAC address."), "mac"), RequiredText("interface", "interface", "Interface on which the binding applies."), Text("description", "description", "Binding description."), netBool01("enabled", "enable", "Enable this binding."), ReadText("unique_name", "uniq_name", "Router-generated binding identity."), ReadText("hostname", "hostname", "Discovered client hostname.")},
		},
		{
			Name: "mac_filter_settings", Description: "MAC filtering policy and interfaces.", Category: "Security", Module: "mac_filter", Section: "global", Mode: Settings, Source: "/userrpm/macFiltering.htm",
			Fields: []Field{netBool01("enabled", "enable", "Enable MAC filtering."), netEnum(Text("mode", "filter_mode", "Allow listed MACs or deny listed MACs."), "white", "black"), Strings("interfaces", "interfaces", "Interfaces on which MAC filtering applies.")},
		},
		{
			Name: "mac_filter", Description: "A named MAC filtering entry.", Category: "Security", Module: "mac_filter", Table: "mac_filter_list", Mode: Entry, NameField: "name", Source: "/userrpm/macFiltering.htm",
			Fields: []Field{netName(), netIP(RequiredText("mac_address", "mac", "MAC address to match."), "mac")},
		},
		{
			Name: "attack_defense", Description: "Malformed-packet protection and attack logging.", Category: "Security", Module: "dos_defense", Section: "global", Mode: Settings, Source: "/userrpm/attack-defense.htm",
			Fields: []Field{
				netBool01("log_enabled", "enable", "Log detected attacks."), netBool01("ip_fragments", "ip_frag", "Reject fragmented IP packets."),
				netBool01("tcp_no_flags", "tcp_noflag", "Protect against TCP packets without flags."), netBool01("ping_of_death", "ping_death", "Enable ping-of-death protection."),
				netBool01("large_ping", "ping_large", "Reject oversized ping packets."), netBool01("winnuke", "tcp_winnuke", "Enable WinNuke protection."),
				netBool01("teardrop", "teardrop", "Enable teardrop protection."), netBool01("land", "land", "Enable LAND attack protection."),
				netBool01("tcp_fin_syn", "tcp_fin_syn", "Reject TCP packets with FIN and SYN set."), netBool01("tcp_fin_without_ack", "tcp_fin_noack", "Reject TCP FIN without ACK."),
				netBool01("ip_options", "ip_option", "Enable IP option checking."), netBool01("ip_security_option", "ipopt_secure", "Check IP security options."),
				netBool01("loose_source_route", "ipopt_loose_route", "Reject loose source routing."), netBool01("strict_source_route", "ipopt_strict_route", "Reject strict source routing."),
				netBool01("record_route", "ipopt_record_route", "Check the IP record-route option."), netBool01("stream_id", "ipopt_stream", "Check the IP stream-ID option."),
				netBool01("timestamp_option", "ipopt_timestamp", "Check the IP timestamp option."), netBool01("noop_option", "ipopt_noop", "Check the IP no-operation option."),
			},
		},
		{
			Name: "flood_defense", Description: "TCP, UDP and ICMP flood detection switches and thresholds.", Category: "Security", Module: "flood_defense", Section: "global", Mode: Settings, Source: "/userrpm/attack-defense.htm",
			Fields: []Field{
				netBool01("tcp_connection_enabled", "tcp_conn_en", "Enable TCP connection flood protection."), securitySection(Integer("tcp_connection_limit", "tcp_conn_lim", "TCP connection flood threshold."), "threshold"),
				netBool01("udp_connection_enabled", "udp_conn_en", "Enable UDP connection flood protection."), securitySection(Integer("udp_connection_limit", "udp_conn_lim", "UDP connection flood threshold."), "threshold"),
				netBool01("icmp_connection_enabled", "icmp_conn_en", "Enable ICMP connection flood protection."), securitySection(Integer("icmp_connection_limit", "icmp_conn_lim", "ICMP connection flood threshold."), "threshold"),
				netBool01("tcp_source_enabled", "tcp_src_en", "Enable per-source TCP flood protection."), securitySection(Integer("tcp_source_limit", "tcp_src_lim", "Per-source TCP flood threshold."), "threshold"),
				netBool01("udp_source_enabled", "udp_src_en", "Enable per-source UDP flood protection."), securitySection(Integer("udp_source_limit", "udp_src_lim", "Per-source UDP flood threshold."), "threshold"),
				netBool01("icmp_source_enabled", "icmp_src_en", "Enable per-source ICMP flood protection."), securitySection(Integer("icmp_source_limit", "icmp_src_lim", "Per-source ICMP flood threshold."), "threshold"),
			},
			Notes: "The device stores enable switches in global and threshold numbers in threshold; the provider reads and writes both sections.",
		},
		{
			Name: "network_scan_defense", Description: "IP scan and port scan protection.", Category: "Security", Module: "netscan_defense", Section: "global", Mode: Settings, Source: "/userrpm/attack-defense.htm",
			Fields: []Field{netBool01("ip_scan_enabled", "ipscan_def", "Detect IP scans."), Integer("ip_scan_rate", "ipscan_rate", "IP scan rate threshold."), Integer("ip_scan_ageing_time", "ipscan_ageing_time", "IP scan aging time."), netBool01("port_scan_enabled", "portscan_def", "Detect port scans."), Integer("port_scan_rate", "portscan_rate", "Port scan rate threshold."), Integer("port_scan_ageing_time", "portscan_ageing_time", "Port scan aging time.")},
		},
		{
			Name: "access_control", Description: "The authoritative ordered user access-control ruleset.", Category: "Security", Module: "access_ctl", Table: "rule_acl_inner", Mode: Ruleset, OrderField: "position", NameField: "name", Source: "/userrpm/access_control.htm",
			Fields:         []Field{RequiredText("name", "name", "Rule name."), netEnum(RequiredText("policy", "policy", "Action for matching traffic."), "ACCEPT", "DROP"), netRequired(Strings("services", "service", "Matched service objects.")), netRequired(Strings("interfaces", "zone", "Interfaces on which the rule applies.")), Text("source_group", "src", "Source IP group."), Text("destination_group", "dest", "Destination IP group."), Text("time_object", "time", "Applicable time object.")},
			CreateDefaults: map[string]any{"user": "1"}, Protected: map[string][]string{"user": {"0", "2", "sdwan", "sdwan_global"}}, ProtectUpdates: true,
			Notes: "List order is precedence; the UI's position is one based. Preserve firmware, SSL VPN and SD-WAN generated rules identified by user=0,2,sdwan,sdwan_global.",
		},
		{
			Name: "ipv6_firewall", Description: "IPv6 firewall enable switch and protected interfaces.", Category: "Security", Module: "ipv6_firewall", Section: "basic", Mode: Settings, Source: "/userrpm/ipv6_firewall.htm",
			Fields: []Field{netBool("enabled", "ipv6_firewall_state", "Enable the IPv6 firewall."), Strings("interfaces", "use_if", "Interfaces protected by the IPv6 firewall.")},
		},
		securityToggle("application_control_settings", "Application-control global switch.", "app_restrict", "global", "appdist"),
		{
			Name: "application_control", Description: "The authoritative application-control ruleset.", Category: "Behavior control", Module: "app_restrict", Table: "app_restrict_rule", Mode: Ruleset, Source: "/userrpm/appdist.htm",
			Fields:    []Field{RequiredText("ip_group", "addr", "IP group to which the rule applies."), Strings("blocked_applications", "drop_app_groups", "Application group identifiers to block."), Strings("logged_applications", "log_app_groups", "Application group identifiers to log."), Text("time_object", "timeobj", "Applicable time object."), Text("comment", "comment", "Rule description."), netBool("enabled", "enable", "Enable the rule.")},
			Protected: map[string][]string{"user": {"0"}}, ProtectUpdates: true,
			Notes: "Preserves firmware default rules marked user=0. Application identifiers come from the device application library, not display labels.",
		},
		securityToggle("qq_filter_settings", "QQ allow/deny-list global switch.", "bwlist_qq", "global", "bwlist_qq"),
		{
			Name: "qq_filter", Description: "The authoritative ordered QQ account filtering ruleset.", Category: "Behavior control", Module: "bwlist_qq", Table: "qq_rule", Mode: Ruleset, OrderField: "insert_index", Source: "/userrpm/bwlist_qq.htm",
			Fields: []Field{RequiredText("ip_group", "addr", "IP group to match."), netEnum(RequiredText("mode", "rule_type", "Account list behavior."), "white_list", "black_list"), netRequired(Strings("accounts", "qq_num", "QQ account numbers.")), netBool("log", "record_log", "Record matching traffic."), Text("time_object", "timeobj", "Applicable time object."), Text("comment", "comment", "Rule description."), netBool("enabled", "enable", "Enable the rule.")},
		},
		{
			Name: "website_group", Description: "A named website group used by website filters.", Category: "Objects", Module: "websort", Table: "websort_list", Mode: Entry, NameField: "textname", Source: "/userrpm/web_group.htm",
			Fields: []Field{netImmutable(RequiredText("name", "textname", "Website group name.")), netRequired(Strings("members", "members", "Website domains or patterns.")), Text("comment", "comment", "Group description."), netReadInt("references", "refer", "Number of references."), ReadText("configuration", "config", "Firmware group configuration marker.")},
		},
		securityToggle("website_filter_settings", "Website filtering global switch.", "webfilter", "settings", "web_filter"),
		{
			Name: "website_filter", Description: "The authoritative ordered website-group filtering ruleset.", Category: "Behavior control", Module: "webfilter", Table: "webfilter_list", Mode: Ruleset, OrderField: "insert_index", Source: "/userrpm/web_filter.htm",
			Fields: []Field{netRequired(Strings("ip_groups", "group", "IP groups to match.")), netBool("allow", "allow", "Allow matching websites when true; deny them when false."), netRequired(Strings("website_groups", "website", "Website groups to match.")), Boolean("log", "action", "Log matching requests.", "log", "log_off"), Text("redirect", "redirect", "Redirect destination for blocked requests."), Text("time_object", "time", "Applicable time object."), Text("comment", "comment", "Rule description."), netBool("enabled", "enable", "Enable the rule.")},
		},
		securityToggle("url_filter_settings", "URL filtering global switch.", "url_filter", "settings", "url_filtering"),
		{
			Name: "url_filter", Description: "The authoritative ordered URL or keyword filtering ruleset.", Category: "Behavior control", Module: "url_filter", Table: "url_list", Mode: Ruleset, OrderField: "insert_index", Source: "/userrpm/url_filtering.htm",
			Fields: []Field{RequiredText("ip_group", "usergroupname", "IP group to match."), netEnum(RequiredText("policy", "allow", "Action for matching URLs."), "accept", "drop"), netEnum(RequiredText("match_type", "filter_type", "URL matching method."), "keyword", "url"), netRequired(Strings("content", "filter_content", "Keywords or URLs to match.")), Boolean("log", "action", "Log matching requests.", "log", "log_off"), Text("redirect", "redirect", "Redirect destination for blocked requests."), Text("time_object", "time", "Applicable time object."), Text("comment", "comment", "Rule description."), netBool("enabled", "enable", "Enable the rule.")},
		},
		securityToggle("web_security_settings", "Web submission and download filtering global switch.", "web_security", "global", "web_security"),
		{
			Name: "web_security", Description: "The authoritative web submission and download filtering ruleset.", Category: "Behavior control", Module: "web_security", Table: "security_rule", Mode: Ruleset, Source: "/userrpm/web_security.htm",
			Fields: []Field{RequiredText("ip_group", "addr", "IP group to match."), netBool("block_submissions", "websubmit", "Apply web submission protection."), Strings("file_extensions", "fileext", "File extensions to block."), Text("time_object", "timeobj", "Applicable time object."), Text("comment", "comment", "Rule description."), netBool("enabled", "enable", "Enable the rule.")},
		},
		{
			Name: "behavior_audit", Description: "Behavior-audit log forwarding.", Category: "Behavior control", Module: "behave_audit", Section: "setting", Mode: Settings, Source: "/userrpm/action_check.htm",
			Fields: []Field{netBool("send_logs", "log_send", "Forward behavior-audit logs."), Text("server", "send_server", "Log receiver address.")},
		},
	}
}

func securitySection(f Field, section string) Field { f.Section = section; return f }
func securityToggle(name, description, module, section, page string) Spec {
	return Spec{Name: name, Description: description, Category: "Behavior control", Module: module, Section: section, Mode: Settings, Source: "/userrpm/" + page + ".htm", Fields: []Field{netBool("enabled", "enable", "Enable this feature.")}}
}
