package catalog

// The fields below follow TL-ER6220T 1.2.2 Build 240829's management pages.
// Keep logical interfaces separate from VLAN switching membership: the web UI
// combines those two independent APIs and hard-codes a model-specific parent port.
func NetworkSpecs() []Spec {
	specs := []Spec{
		{
			Name: "interface", Description: "A logical Ethernet, PPPoE or existing physical interface.", Category: "Network",
			Module: "network", Table: "if_info", Mode: Entry, IDField: "if_name", NameField: "if_name", FilterField: "if_name", FilterList: true,
			Source: "/userrpm/interface.htm", Protected: map[string][]string{"if_type": {"physical"}},
			Notes: "Import physical interfaces before editing them. Physical interfaces cannot be deleted. Changes to addressing, management access or the parent interface can interrupt the provider connection. Interface names are immutable.",
			Fields: []Field{
				netImmutable(RequiredText("name", "if_name", "Logical interface name.")),
				netImmutable(netEnum(RequiredText("type", "if_type", "Interface type."), "physical", "ethernet", "pppoe", "bridge", "bond")),
				Strings("parent_interfaces", "bindif", "Parent Ethernet or physical interface names."),
				netRange(Integer("vlan_id", "vlanid", "VLAN identifier for an Ethernet subinterface."), 1, 4094),
				netBool01("untagged", "untag", "Send traffic without an 802.1Q tag."),
				netEnum(Text("protocol", "proto", "IPv4 addressing protocol; IPv6 has separate configuration fields."), "none", "static", "dhcp", "pppoe"),
				netIP(Text("ip_address", "ip", "Static IPv4 address."), "ipv4"),
				netIP(Text("netmask", "netmask", "IPv4 subnet mask."), "ipv4"),
				netIP(Text("gateway", "gateway", "IPv4 default gateway."), "ipv4"),
				Text("hostname", "hostname", "DHCP hostname."), Text("username", "username", "PPPoE username."),
				Secret("password_wo", "password", "PPPoE password; never retained in Terraform state."),
				netEnum(Text("connection_mode", "linktype", "PPPoE connection mode."), "auto", "manual", "time"),
				Text("time_object", "timeobj", "Time object controlling scheduled connections."),
				Text("service_name", "service", "PPPoE service name."),
				netRange(Integer("mtu", "mtu", "IPv4 MTU."), 576, 1500),
				netIP(Text("primary_dns", "pri_dns", "Preferred IPv4 DNS server."), "ipv4"),
				netIP(Text("secondary_dns", "snd_dns", "Secondary IPv4 DNS server."), "ipv4"),
				netBool01("use_custom_dns", "is_user_dns", "Use configured IPv4 DNS servers."),
				Integer("upstream_bandwidth_kbytes_per_second", "up_speed", "Interface upstream bandwidth in kilobytes per second; multiply by eight for the web UI's Kbps value."),
				Integer("downstream_bandwidth_kbytes_per_second", "down_speed", "Interface downstream bandwidth in kilobytes per second; multiply by eight for the web UI's Kbps value."),
				netIP(Text("mac_address", "mac", "Configured interface MAC address."), "mac"),
				netBool("management_enabled", "mngt_enable", "Permit management through this interface."),
				Text("comment", "comment", "Interface description."),
				netBool("ipv6_enabled", "ipv6_enable", "Enable IPv6 on the interface."),
				netBool("ipv6_share_ipv4_session", "pppoe_sharev4", "Share the IPv4 PPPoE session with IPv6."),
				Text("ipv6_username", "username6", "Username for an independent IPv6 PPPoE session."),
				Secret("ipv6_password_wo", "password6", "Password for an independent IPv6 PPPoE session."),
				netEnum(Text("ipv6_interface_id_mode", "ip6ifaceid", "IPv6 interface identifier mode."), "eui64", "manual"),
				Text("prefix_interface", "prefix_if", "Interface providing a delegated IPv6 prefix."),
				Text("ipv6_prefix", "prefix", "IPv6 prefix in the format accepted by the router."),
				netIP(Text("ipv6_address", "ip6addr", "Static IPv6 address."), "ipv6"),
				netRange(Integer("ipv6_prefix_length", "prefixlen", "IPv6 prefix length."), 0, 128),
				netIP(Text("ipv6_gateway", "ip6gw", "IPv6 gateway."), "ipv6"),
				netEnum(Text("ipv6_address_mode", "ip_config", "IPv6 address assignment mode."), "auto", "dhcpv6", "slaac", "static"),
				netEnum(Text("ipv6_delegation_mode", "pd_mode", "DHCPv6 prefix delegation mode."), "prefix", "non_temp"),
				netBool01("use_peer_ipv6_dns", "peerdns6", "Obtain IPv6 DNS servers from the upstream peer."),
				netIP(Text("primary_ipv6_dns", "pri_dns6", "Preferred IPv6 DNS server."), "ipv6"),
				netIP(Text("secondary_ipv6_dns", "snd_dns6", "Secondary IPv6 DNS server."), "ipv6"),
				Text("ipv6_service_name", "service6", "IPv6 PPPoE service name."),
				netRange(Integer("ipv6_mtu", "mtu6", "IPv6 MTU."), 1280, 1500),
				ReadText("link_status", "link_status", "Current link state."), ReadText("system_flag", "issys", "Firmware system-interface marker."),
			},
		},
		{
			Name: "bridge", Description: "A logical interface bridge.", Category: "Network", Module: "network", Table: "bridge", Mode: Entry,
			IDField: "if_name", NameField: "if_name", FilterField: "if_name", FilterList: true, Source: "/userrpm/bridge.htm",
			Fields: []Field{netImmutable(RequiredText("name", "if_name", "Bridge interface name.")), netRequired(Strings("interfaces", "bindif", "Member interface names.")), Text("configuration_interface", "configif", "Interface whose address configuration the bridge uses."), netBool01("spanning_tree", "stp", "Enable spanning tree.")},
			Notes:  "Manage bridge addressing through the interface resource. Changing membership can interrupt the management connection.",
		},
		{
			Name: "vlan", Description: "VLAN switching membership; configure routed addressing with a separate interface resource.", Category: "Network", Module: "vlan", Table: "vlan", Mode: Entry,
			NameField: "vlan_id", Source: "/userrpm/vlan_setting.htm", Protected: map[string][]string{"vlan_id": {"1"}},
			Fields: []Field{netImmutable(netRequired(netRange(Integer("vlan_id", "vlan_id", "VLAN identifier."), 1, 4094))), RequiredText("name", "name", "VLAN name."), Text("note", "note", "VLAN description."), Strings("tagged_ports", "tagged_ports", "Tagged member ports."), Strings("untagged_ports", "untagged_ports", "Untagged member ports.")},
			Notes:  "Available only when the device exposes the vlan.vlan table. VLAN 1 cannot be deleted. A routed VLAN interface is a separate interface resource; create the VLAN before its interface and destroy the interface first.",
		},
		{
			Name: "ipv6_bridge", Description: "IPv6 bridge settings.", Category: "Network", Module: "network", Section: "bridge_v6", Mode: Settings, Source: "/userrpm/ipv6_bridge.htm",
			Fields: []Field{netBool("enabled", "enable", "Enable IPv6 bridging."), Text("wan_interface", "ipv6_WAN", "WAN interface."), Text("lan_interface", "ipv6_LAN", "LAN interface.")},
		},
		{
			Name: "port", Description: "Flow control and link negotiation for an existing physical port.", Category: "Network", Module: "port", Table: "port_basic", Mode: Existing, Source: "/userrpm/port_flowControl.htm",
			Query:  map[string]any{"filter": []any{map[string]any{"type": "ALL"}}},
			Fields: []Field{netBool("flow_control", "flowcontrol_enable", "Enable Ethernet flow control."), netEnum(Text("speed", "cfg_speed", "Configured port speed."), "auto", "10M", "100M", "1000M"), netEnum(Text("duplex", "cfg_duplex", "Configured duplex."), "auto", "Half", "Full")},
			Notes:  "Import a discovered port section name. Hardware is never created or deleted. Available speeds depend on the port capabilities.",
		},
		{
			Name: "port_vlan", Description: "The default VLAN of an existing switch port.", Category: "Network", Module: "port", Table: "port_basic", WriteModule: "vlan", Mode: Existing, Source: "/userrpm/port_setting.htm",
			Fields: []Field{netRange(Integer("pvid", "pvid", "Default VLAN identifier."), 1, 4094)}, Notes: "Reads port.port_basic but writes the corresponding section in vlan. Only supported on devices with switch-port VLAN configuration.",
		},
		{
			Name: "optical_port", Description: "Configured speed of an existing optical interface.", Category: "Network", Module: "network", Table: "optical_port", Mode: Existing, Source: "/userrpm/auto_optical_port.htm",
			IDField: "if_name", FilterField: "if_name", FilterList: true,
			Fields: []Field{Text("speed", "speed", "Configured rate supported by this optical port."), ReadText("duplex", "duplex", "Reported duplex mode."), ReadText("link_status", "link_status", "Reported link status."), ReadText("current_speed", "cur_speed", "Current negotiated speed.")},
		},
		{
			Name: "ip_address", Description: "An address object containing a range or subnet.", Category: "Objects", Module: "ipgroup", Table: "rule_ipscope", Mode: Entry, NameField: "name", Source: "/userrpm/ipgroup_address.htm",
			Fields:         []Field{netName(), netEnum(RequiredText("type", "type", "Address representation."), "range", "mask"), RequiredText("scope", "scope", "IPv4 range start-end, or network/prefix length for mask objects."), netComment("comment", "comment", 50), ReadText("flag", "flag", "Firmware object ownership flag.")},
			CreateDefaults: map[string]any{"flag": "user"}, Protected: map[string][]string{"flag": {"system", "inner", "sdwan*"}}, ProtectUpdates: true,
		},
		{
			Name: "ip_group", Description: "An address group referencing address objects.", Category: "Objects", Module: "ipgroup", Table: "rule_ipgroup", Mode: Entry, NameField: "name", Source: "/userrpm/ipgroup_group.htm",
			Fields:         []Field{netName(), netRequired(Strings("addresses", "rule_scope", "Names of address objects in this group.")), netComment("comment", "comment", 50), ReadText("flag", "flag", "Firmware object ownership flag."), netReadInt("references", "ref", "Number of references from other objects.")},
			CreateDefaults: map[string]any{"flag": "user", "ref": "0"}, Protected: map[string][]string{"flag": {"system", "inner", "sdwan*"}}, ProtectUpdates: true,
		},
		{
			Name: "ip_pool", Description: "An IPv4 address pool.", Category: "Objects", Module: "ippool", Table: "ippool", Mode: Entry, NameField: "name", Source: "/userrpm/ippool.htm",
			Fields: []Field{netName(), netIP(RequiredText("start_ip", "start_ip", "First address in the pool."), "ipv4"), netIP(RequiredText("end_ip", "end_ip", "Last address in the pool."), "ipv4"), netReadInt("references", "ref", "Number of references from other objects.")}, CreateDefaults: map[string]any{"ref": "0"},
		},
		{
			Name: "service", Description: "An IP protocol and port-range service object.", Category: "Objects", Module: "service", Table: "service", Mode: Entry, NameField: "name", Source: "/userrpm/service.htm",
			Fields:         []Field{netName(), netEnum(RequiredText("protocol", "proto", "Service protocol."), "tcp", "udp", "tcp-udp", "icmp", "other"), Text("source_ports", "sport", "Source range, for example 1-65535."), Text("destination_ports", "dport", "Destination range, for example 443-443."), Text("icmp_type", "type", "ICMP type accepted by the router."), Text("icmp_code", "code", "ICMP code accepted by the router."), netRange(Integer("protocol_number", "other", "IP protocol number when protocol is other."), 0, 255), netComment("comment", "comment", 50), ReadText("flag", "flag", "Firmware object ownership flag."), netReadInt("references", "ref", "Number of references.")},
			CreateDefaults: map[string]any{"flag": "user", "ref": "0"}, Protected: map[string][]string{"flag": {"system"}}, ProtectUpdates: true,
		},
		{
			Name: "time_object", Description: "A reusable weekly time schedule.", Category: "Objects", Module: "time_mngt", Table: "time_obj", Mode: Entry, NameField: "name", Source: "/userrpm/time_mngt.htm",
			Fields:         []Field{netName(), netEnum(RequiredText("mode", "mode", "Calendar hourly bitmap or manual time intervals."), "calendar", "manual"), Strings("time_intervals", "time_section", "Manual intervals encoded as HHMM,HHMM, for example 0900,1730."), netRange(Integer("weekdays", "weekday", "Manual weekday bitmap; Monday is bit 0 and Sunday bit 6."), 1, 127), netRange(Integer("monday", "mon", "Calendar bitmap of the 24 hours of Monday."), 0, 16777215), netRange(Integer("tuesday", "tue", "Calendar bitmap of the 24 hours of Tuesday."), 0, 16777215), netRange(Integer("wednesday", "wed", "Calendar bitmap of the 24 hours of Wednesday."), 0, 16777215), netRange(Integer("thursday", "thu", "Calendar bitmap of the 24 hours of Thursday."), 0, 16777215), netRange(Integer("friday", "fri", "Calendar bitmap of the 24 hours of Friday."), 0, 16777215), netRange(Integer("saturday", "sat", "Calendar bitmap of the 24 hours of Saturday."), 0, 16777215), netRange(Integer("sunday", "sun", "Calendar bitmap of the 24 hours of Sunday."), 0, 16777215), netComment("comment", "comment", 50), ReadText("flag", "flag", "Firmware object ownership flag.")},
			CreateDefaults: map[string]any{"flag": "user"}, Protected: map[string][]string{"flag": {"system"}}, ProtectUpdates: true,
		},
		{
			Name: "napt", Description: "A network address and port translation rule.", Category: "NAT", Module: "nat", Table: "rule_napt", Mode: Entry, NameField: "name", Source: "/userrpm/napt.htm",
			Fields: []Field{netName(), netIPVersion(), netRequired(Strings("interfaces", "if", "Outgoing interface names.")), RequiredText("source", "ip", "Source network in the format accepted by the selected IP version."), Text("mask", "mask", "Source network mask when required by the firmware."), Text("address_pool", "nat_pool", "Optional source NAT address range as start~end; --- disables the pool."), netBool("enabled", "enable", "Enable this rule."), netComment("comment", "comment", 50), ReadText("system_rule", "sysrule", "Firmware system-rule marker.")},
			Notes:  "The page includes a sysrule field but does not define its values. The provider reports this marker without guessing which values indicate built-in rules.",
		},
		{
			Name: "one_to_one_nat", Description: "A one-to-one NAT rule.", Category: "NAT", Module: "nat", Table: "rule_onenat", Mode: Entry, NameField: "name", Source: "/userrpm/one_nat.htm",
			Fields: []Field{netName(), netIPVersion(), RequiredText("interface", "if", "Outgoing interface."), netIP(RequiredText("internal_ip", "internal_ip", "Internal host address."), "ip"), netIP(RequiredText("external_ip", "external_ip", "External address."), "ip"), netBool("dmz", "dmz", "Enable the rule's DMZ option."), netBool("enabled", "enable", "Enable this rule."), netComment("comment", "comment", 50)},
		},
		{
			Name: "port_forward", Description: "A virtual-server port-forwarding rule.", Category: "NAT", Module: "firewall", Table: "redirect", Mode: Entry, NameField: "name", Source: "/userrpm/virtual_server.htm",
			// These hidden editor inputs are always initialized to 65536. The page
			// sends the actual ranges in src_dport/dest_port and never recomputes them.
			CreateDefaults: map[string]any{"src_dport_start": "65536", "src_dport_end": "65536", "dest_port_start": "65536", "dest_port_end": "65536"},
			Fields:         []Field{netName(), netIPVersion(), netRequired(Strings("interfaces", "if", "Interfaces accepting external connections.")), netEnum(RequiredText("protocol", "proto", "Transport protocol; ALL means TCP and UDP."), "ALL", "TCP", "UDP"), RequiredText("external_ports", "src_dport", "At most five comma-separated ports or inclusive port ranges."), RequiredText("internal_ports", "dest_port", "Matching internal ports or port ranges."), netIP(RequiredText("internal_ip", "dest_ip", "Internal server address."), "ip"), netSeparated(Strings("loopback_networks", "loopback_ipaddr", "Networks allowed to use NAT loopback."), ","), netBool("enabled", "enable", "Enable this port forwarding rule.")},
			Notes:          "Internal and external port lists must have matching ranges. External ranges cannot overlap another active rule on the same interface and protocol.",
		},
		{
			Name: "dmz", Description: "A DMZ host rule.", Category: "NAT", Module: "nat", Table: "rule_dmz", Mode: Entry, NameField: "name", Source: "/userrpm/nat_dmz.htm",
			Fields: []Field{netName(), netIPVersion(), netRequired(Strings("interfaces", "if", "Outgoing interface names.")), netIP(RequiredText("ip_address", "ip", "DMZ host address."), "ip"), netBool("enabled", "enable", "Enable this rule.")},
		},
		{
			Name: "alg", Description: "Application-level gateway settings.", Category: "NAT", Module: "nat", Section: "alg_glb", Mode: Settings, Source: "/userrpm/alg.htm",
			Fields: []Field{netBool("ftp", "ftp", "FTP ALG."), netBool("tftp", "tftp", "TFTP ALG."), netBool("h323", "h323", "H.323 ALG."), netBool("pptp", "pptp", "PPTP ALG."), netBool("sip", "sip", "SIP ALG."), netBool("ipsec", "ipsec", "IPsec ALG.")},
		},
		{
			Name: "static_route", Description: "An IPv4 static route with one or more next hops.", Category: "Routing", Module: "network", Table: "user_route", Mode: Entry, NameField: "name", Source: "/userrpm/static_routing.htm",
			Fields: []Field{netName(), netIP(RequiredText("destination", "target", "Destination network address, with host bits cleared."), "ipv4"), netIP(RequiredText("netmask", "netmask", "Destination subnet mask."), "ipv4"), netRequired(Strings("gateways", "gateway_list", "Next-hop IPv4 addresses; --- represents no explicit gateway for a point-to-point interface.")), netRequired(Strings("interfaces", "if_list", "Parallel outgoing interface list; --- means route lookup by gateway.")), netRange(Integer("metric", "metric", "Route metric; smaller values have higher priority."), 0, 15), netComment("note", "note", 50), netBool("enabled", "enable", "Enable the route."), ReadText("reachability", "reachability", "Current reachability reported by the router.")},
			Notes:  "gateways and interfaces are parallel lists. Ethernet next hops require an address; PPPoE and VPN point-to-point next hops may use ---.",
		},
		{
			Name: "ipv6_static_route", Description: "An IPv6 static route.", Category: "Routing", Module: "network", Table: "user_route_ipv6", Mode: Entry, NameField: "name", Source: "/userrpm/static6_routing.htm",
			Fields: []Field{netName(), netIP(RequiredText("destination", "target", "Destination IPv6 network address."), "ipv6"), netRequired(netRange(Integer("prefix_length", "netmask", "Destination prefix length."), 0, 128)), netIP(RequiredText("gateway", "gateway", "IPv6 next-hop address."), "ipv6"), RequiredText("interface", "if", "Outgoing interface."), netRange(Integer("metric", "metric", "Route metric."), 1, 1024), netComment("note", "note", 50), netBool("enabled", "enable", "Enable the route."), ReadText("reachability", "reachability", "Current reachability.")},
		},
		{
			Name: "policy_routes", Description: "The authoritative ordered policy-routing ruleset.", Category: "Routing", Module: "policy_route", Table: "policy_rule", Mode: Ruleset, OrderField: "index", NameField: "name", Source: "/userrpm/policy_routing.htm",
			Fields: []Field{RequiredText("name", "name", "Rule name."), Text("service", "service_type", "Service object, or ALL."), Text("source_group", "src_ipgroup", "Source IP group."), Text("destination_group", "dst_ipgroup", "Destination IP group."), netRequired(Strings("interfaces", "if", "Outgoing interfaces.")), Text("time_object", "timeobj", "Applicable time object."), netBool("forced", "forced", "Apply the rule even when no outgoing interface is online."), netComment("comment", "comment", 50), netBool("enabled", "enable", "Enable the rule.")},
			Notes:  "List order defines precedence. The web UI sends a one-based para.index for insertion or repositioning.",
		},
		{
			Name: "qos_settings", Description: "Bandwidth-control global settings.", Category: "Traffic control", Module: "qos", Section: "setting", Mode: Settings, Source: "/userrpm/qos.htm",
			Fields: []Field{netBool("enabled", "qos_enable", "Enable bandwidth control."), netBool("threshold_enabled", "threshold_enable", "Only enforce limits after the configured threshold is reached."), netRange(Integer("threshold_percent", "qos_threshold", "Bandwidth threshold percentage."), 1, 100)},
		},
		{
			Name: "qos_rules", Description: "The authoritative ordered bandwidth-control ruleset.", Category: "Traffic control", Module: "qos", Table: "rule", Mode: Ruleset, OrderField: "position", NameField: "name", Source: "/userrpm/qos.htm",
			Fields: []Field{RequiredText("name", "name", "Rule name."), Strings("source_interfaces", "if_ping", "Source interfaces."), Strings("destination_interfaces", "if_pong", "Destination interfaces."), Text("ip_group", "ip_group", "Applicable IP group."), Integer("maximum_rate_kbps", "rate_max", "Maximum rate in Kbps."), Integer("reverse_maximum_rate_kbps", "rate_max_mate", "Reverse-direction maximum rate in Kbps when supported."), netEnum(Text("mode", "mode", "Share the limit or apply it per host."), "share", "priv"), Text("time_object", "time", "Applicable time object."), netEnum(Text("ip_type", "ip_type", "IP address matching direction."), "src", "dst"), netComment("comment", "comment", 50), netBool("enabled", "enable", "Enable the rule.")},
		},
		{
			Name: "session_limit_settings", Description: "Global connection-count limiting.", Category: "Traffic control", Module: "session_limits", Section: "global", Mode: Settings, Source: "/userrpm/session_limits.htm", Fields: []Field{netBool("enabled", "status", "Enable session limits.")},
		},
		{
			Name: "session_limit", Description: "A connection-count limit for an IP group.", Category: "Traffic control", Module: "session_limits", Table: "limit_rule", Mode: Entry, NameField: "name", Source: "/userrpm/session_limits.htm",
			Fields: []Field{netName(), RequiredText("ip_group", "ipgroup", "IP group to limit."), netRequired(Integer("maximum_connections", "max_conn", "Maximum concurrent connections.")), netBool("enabled", "status", "Enable this limit.")},
		},
		{
			Name: "load_balance", Description: "WAN load-balancing mode and participating interfaces.", Category: "Routing", Module: "load_balance", Section: "basic", Mode: Settings, Source: "/userrpm/balance_basic.htm",
			Fields: []Field{netEnum(Text("mode", "balance_mode", "Load-balancing mode."), "default_balance", "load_balance", "dns_balance"), Strings("interfaces", "use_if", "Participating WAN interfaces.")},
		},
		{
			Name: "load_balance_enabled", Description: "Global WAN load-balancing switch.", Category: "Routing", Module: "mwan3", Section: "global", Mode: Settings, Source: "/userrpm/balance_basic.htm", Fields: []Field{netBool("enabled", "state", "Enable WAN load balancing.")},
		},
		{
			Name: "special_route_settings", Description: "Application-aware load-balancing special settings.", Category: "Routing", Module: "mwan3", Section: "special_rule", Mode: Settings, Source: "/userrpm/balance_basic.htm", Fields: []Field{netBool("enabled", "state", "Enable special load-balancing rules.")},
		},
		{
			Name: "interface_weight", Description: "The load-balancing weight of an existing interface.", Category: "Routing", Module: "mwan3", Table: "if", Mode: Existing, Source: "/userrpm/balance_basic.htm", Fields: []Field{Integer("weight", "weight", "Interface weight.")},
		},
		{
			Name: "isp_routing_settings", Description: "Global ISP routing settings.", Category: "Routing", Module: "isp_route", Section: "global", Mode: Settings, Source: "/userrpm/isp_routing.htm", Fields: []Field{netBool("enabled", "state", "Enable ISP routing.")},
		},
		{
			Name: "isp_route", Description: "An interface-to-ISP routing rule.", Category: "Routing", Module: "isp_route", Table: "rule", Mode: Entry, IDFields: []string{"if", "isp_name"}, Source: "/userrpm/isp_routing.htm",
			Fields: []Field{RequiredText("interface", "if", "Outgoing interface."), RequiredText("isp", "isp_name", "ISP database name."), netBool("enabled", "state", "Enable the rule.")},
		},
		{
			Name: "line_backup", Description: "An interface failover or scheduled backup rule.", Category: "Routing", Module: "line_backup", Table: "rule", Mode: Entry, IDFields: []string{"master_if", "slave_if"}, Source: "/userrpm/line_backup.htm",
			Fields: []Field{RequiredText("primary_interface", "master_if", "Primary interface."), RequiredText("backup_interface", "slave_if", "Backup interface."), netEnum(RequiredText("mode", "mode", "Backup trigger mode."), "timer", "fault"), Text("time_object", "timeobj", "Time object for timer mode."), netBool("enabled", "state", "Enable the rule.")},
		},
		{
			Name: "online_detection", Description: "Online detection settings for an existing interface.", Category: "Routing", Module: "online_check", Table: "if", Mode: Existing, Source: "/userrpm/online.htm",
			Fields: []Field{netEnum(Text("mode", "mode", "Online detection mode."), "auto", "manual", "always_on"), Text("gateway", "gateway", "Ping target used for manual detection."), Text("dns", "dns", "DNS target used for manual detection."), netBool("enabled", "state", "Enable online detection.")},
		},
	}
	specs = append(specs, netDHCP(false)...)
	specs = append(specs, netDHCP(true)...)
	specs = append(specs, Spec{
		Name: "slaac", Description: "An IPv6 router-advertisement and SLAAC server.", Category: "DHCP", Module: "slaac", Table: "slaac_list", Mode: Entry, NameField: "interface", Source: "/userrpm/slaac.htm",
		Fields: []Field{RequiredText("interface", "interface", "Serving interface."), Text("prefix", "prefix", "Advertised IPv6 prefix; empty uses the interface prefix."), netEnum(Text("dns_type", "dns_type", "DNS advertisement mechanism."), "DHCPv6", "RDNSS"), netIP(Text("primary_dns", "pri_dns", "Preferred IPv6 DNS server."), "ipv6"), netIP(Text("secondary_dns", "snd_dns", "Secondary IPv6 DNS server."), "ipv6"), netBool("enabled", "enable", "Enable the SLAAC server.")},
	})
	return specs
}

func netDHCP(ipv6 bool) []Spec {
	module, prefix, page, addressFormat := "dhcpd", "dhcp", "dhcp", "ipv4"
	if ipv6 {
		module, prefix, page, addressFormat = "dhcpd6", "dhcpv6", "dhcp6", "ipv6"
	}
	server := []Field{RequiredText("interface", "interface", "Serving interface."), Text("address_pool", "ippool", "Address pool name when an object pool is used."), netIP(Text("pool_start", "pool_start", "First lease address."), addressFormat), netIP(Text("pool_end", "pool_end", "Last lease address."), addressFormat), netRange(Integer("lease_time_seconds", "lease_time", "Lease duration in seconds; the web UI displays minutes."), 120, 172800), netIP(Text("primary_dns", "pri_dns", "Preferred DNS server."), addressFormat), netIP(Text("secondary_dns", "snd_dns", "Secondary DNS server."), addressFormat), netBool("enabled", "enable", "Enable this DHCP server.")}
	if ipv6 {
		server = append(server, Text("option52", "option52", "DHCPv6 option 52 CAPWAP controller address."), Text("option16", "option16", "DHCPv6 option 16 vendor class."))
	} else {
		server = append(server, netIP(Text("gateway", "gateway", "Advertised default gateway."), "ipv4"), netIP(Text("netmask", "netmask", "Advertised subnet mask."), "ipv4"), Text("domain", "domain", "Advertised domain name."), Text("option138", "option138", "DHCP option 138 CAPWAP controller address."), Text("option60", "option60", "DHCP option 60 vendor class."))
	}
	return []Spec{
		{Name: prefix + "_settings", Description: "DHCP allocation target settings.", Category: "DHCP", Module: module, Section: "settings", Mode: Settings, Source: "/userrpm/" + page + "_server.htm", Fields: []Field{netEnum(Text("allocation_target", "alloc_target", "Allocate to all clients or only APs."), "for_all", "for_ap")}},
		{Name: prefix + "_server", Description: "A DHCP server address pool and options.", Category: "DHCP", Module: module, Table: "dhcpd_list", Mode: Entry, NameField: "interface", Source: "/userrpm/" + page + "_server.htm", Fields: server},
		{Name: prefix + "_reservation", Description: "A DHCP address reservation for a MAC address.", Category: "DHCP", Module: module, Table: "dhcp_static", Mode: Entry, IDField: "dhcp_static_id", FilterField: "dhcp_static_id", Source: "/userrpm/" + page + "_static.htm", Fields: []Field{netIP(RequiredText("mac_address", "mac", "Client MAC address."), "mac"), netIP(RequiredText("ip_address", "ip", "Reserved address."), addressFormat), Text("name", "name", "Reservation name reported by the firmware."), netComment("note", "note", 32), netBool("enabled", "enable", "Enable the reservation.")}, Notes: "The stable identity is dhcp_static_id. Updates and deletes use the documented ID filter instead of a table index."},
	}
}

func netName() Field { return netImmutable(RequiredText("name", "name", "Unique object name.")) }
func netBool(name, wire, description string) Field {
	return Boolean(name, wire, description, "on", "off")
}
func netBool01(name, wire, description string) Field {
	return Boolean(name, wire, description, "1", "0")
}
func netEnum(f Field, values ...string) Field      { f.Enum = values; return f }
func netRequired(f Field) Field                    { f.Required = true; return f }
func netImmutable(f Field) Field                   { f.RequiresReplace = true; return f }
func netRange(f Field, min, max int64) Field       { f.Min, f.Max = min, max; return f }
func netIP(f Field, format string) Field           { f.Format = format; return f }
func netSeparated(f Field, separator string) Field { f.Separator = separator; return f }
func netComment(name, wire string, maximum int) Field {
	f := Text(name, wire, "Optional description.")
	f.MaxLength = maximum
	return f
}
func netReadInt(name, wire, description string) Field {
	f := Integer(name, wire, description)
	f.ReadOnly = true
	return f
}
func netIPVersion() Field {
	return netEnum(Text("ip_version", "ip_proto", "IP version of the rule."), "IPv4", "IPv6")
}
