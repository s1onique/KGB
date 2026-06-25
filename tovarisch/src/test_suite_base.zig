// test_suite_base.zig — Base test suite for core modules
//
// Tests: config, metrics, status, net utilities, logging.
// These tests are CPU-bound and have no socket/thread dependencies.

const std = @import("std");

// Core modules
const _config = @import("config.zig");
const _config_server_tests = @import("config_server_tests.zig");
const _config_vpn_masquerade_tests = @import("config_vpn_masquerade_tests.zig");
const _logging = @import("logging.zig");
const _status = @import("status.zig");
const _status_checks = @import("status_checks.zig");
const _status_ownership_tests = @import("status_ownership_tests.zig");

// Metrics modules
const _metrics = @import("metrics.zig");
const _metrics_tests = @import("metrics_tests.zig");
const _metrics_conversion_tests = @import("metrics_conversion_tests.zig");
const _metrics_dto = @import("metrics_dto.zig");
const _metrics_dto_tests = @import("metrics_dto_tests.zig");
const _metrics_fallback_dto_tests = @import("metrics_fallback_dto_tests.zig");
const _metrics_state = @import("metrics_state.zig");
const _metrics_state_tests = @import("metrics_state_tests.zig");
const _metrics_tunnel_contract_tests = @import("metrics_tunnel_contract_tests.zig");

// Net utility modules
const _net_private_ip = @import("net/private_ip.zig");
const _net_rates = @import("net/rates.zig");
const _net_interface_sampler = @import("net/interface_sampler.zig");
const _net_interface_sampler_tests = @import("net/interface_sampler_tests.zig");
const _net_linux_stats = @import("net/linux_stats.zig");
const _net_linux_stats_tests = @import("net/linux_stats_tests.zig");
const _net_linux_interfaces = @import("net/linux_interfaces.zig");
const _net_linux_interfaces_tests = @import("net/linux_interfaces_tests.zig");
const _net_linux_interface_stats = @import("net/linux_interface_stats.zig");
const _net_linux_interface_stats_tests = @import("net/linux_interface_stats_tests.zig");
const _net_interface_filter = @import("net/interface_filter.zig");
const _net_interface_filter_tests = @import("net/interface_filter_tests.zig");
const _net_linux_addr = @import("net/linux_addr.zig");
const _net_linux_addr_tests = @import("net/linux_addr_tests.zig");
const _net_linux_addr_parse = @import("net/linux_addr_parse.zig");
const _net_wg_show_parser = @import("net/wg_show_parser.zig");
const _net_wg_show_parser_tests = @import("net/wg_show_parser_tests.zig");
const _net_wg_show_collector = @import("net/wg_show_collector.zig");
const _net_wg_show_collector_tests = @import("net/wg_show_collector_tests.zig");
const _net_wg_status_boundary = @import("net/wg_status_boundary.zig");
const _net_wg_status_boundary_cli = @import("net/wg_status_boundary_cli.zig");
const _net_wg_status_boundary_netlink = @import("net/wg_status_boundary_netlink.zig");
const _net_wg_status_boundary_netlink_tests = @import("net/wg_status_boundary_netlink_tests.zig");
const _net_wg_status_boundary_netlink_runtime_tests = @import("net/wg_status_boundary_netlink_runtime_tests.zig");
const _net_private_interface_stats = @import("net/private_interface_stats.zig");
const _net_private_interface_stats_tests = @import("net/private_interface_stats_tests.zig");
const _net_iptables = @import("net/iptables.zig");
const _status_vpn_masquerade = @import("status_vpn_masquerade.zig");

// Network diagnostics (ACT: Add tovarisch WireGuard and XRay TCP underlay diagnostics)
const _net_network_diag_config = @import("net/network_diag_config.zig");
const _net_wg_dump_parser = @import("net/wg_dump_parser.zig");
const _net_diag_event_ring = @import("net/diag_event_ring.zig");
const _net_safe_command = @import("net/safe_command.zig");
const _net_ss_parser = @import("net/ss_parser.zig");
const _net_route_diag = @import("net/route_diag.zig");
const _net_extended_interface_stats = @import("net/extended_interface_stats.zig");
const _net_wg_dump_collector = @import("net/wg_dump_collector.zig");

// Lab events (idle staircase memory lab)
const _runtime_lab_events = @import("runtime/lab_events.zig");
const _runtime_lab_events_tests = @import("runtime/lab_events_tests.zig");

const _status_network_diag = @import("status_network_diag.zig");
const _status_network_diag_types = @import("status_network_diag_types.zig");
const _status_network_diag_events = @import("status_network_diag_events.zig");
const _status_network_diag_tcp = @import("status_network_diag_tcp.zig");
const _status_network_diag_tests = @import("status_network_diag_tests.zig");
const _status_network_diag_ownership_tests = @import("status_network_diag_ownership_tests.zig");
const _net_ss_parser_tests = @import("net/ss_parser_tests.zig");

// Force test discovery
test { std.testing.refAllDecls(@import("config.zig")); }
test { std.testing.refAllDecls(@import("config_server_tests.zig")); }
test { std.testing.refAllDecls(@import("config_vpn_masquerade_tests.zig")); }
test { std.testing.refAllDecls(@import("logging.zig")); }
test { std.testing.refAllDecls(@import("status.zig")); }
test { std.testing.refAllDecls(@import("status_checks.zig")); }
test { std.testing.refAllDecls(@import("status_ownership_tests.zig")); }
test { std.testing.refAllDecls(@import("metrics.zig")); }
test { std.testing.refAllDecls(@import("metrics_tests.zig")); }
test { std.testing.refAllDecls(@import("metrics_conversion_tests.zig")); }
test { std.testing.refAllDecls(@import("metrics_dto.zig")); }
test { std.testing.refAllDecls(@import("metrics_dto_tests.zig")); }
test { std.testing.refAllDecls(@import("metrics_fallback_dto_tests.zig")); }
test { std.testing.refAllDecls(@import("metrics_state.zig")); }
test { std.testing.refAllDecls(@import("metrics_state_tests.zig")); }
test { std.testing.refAllDecls(@import("metrics_tunnel_contract_tests.zig")); }
test { std.testing.refAllDecls(@import("net/private_ip.zig")); }
test { std.testing.refAllDecls(@import("net/rates.zig")); }
test { std.testing.refAllDecls(@import("net/interface_sampler.zig")); }
test { std.testing.refAllDecls(@import("net/interface_sampler_tests.zig")); }
test { std.testing.refAllDecls(@import("net/linux_stats.zig")); }
test { std.testing.refAllDecls(@import("net/linux_stats_tests.zig")); }
test { std.testing.refAllDecls(@import("net/linux_interfaces.zig")); }
test { std.testing.refAllDecls(@import("net/linux_interfaces_tests.zig")); }
test { std.testing.refAllDecls(@import("net/linux_interface_stats.zig")); }
test { std.testing.refAllDecls(@import("net/linux_interface_stats_tests.zig")); }
test { std.testing.refAllDecls(@import("net/interface_filter.zig")); }
test { std.testing.refAllDecls(@import("net/interface_filter_tests.zig")); }
test { std.testing.refAllDecls(@import("net/linux_addr.zig")); }
test { std.testing.refAllDecls(@import("net/linux_addr_tests.zig")); }
test { std.testing.refAllDecls(@import("net/linux_addr_parse.zig")); }
test { std.testing.refAllDecls(@import("net/wg_show_parser.zig")); }
test { std.testing.refAllDecls(@import("net/wg_show_parser_tests.zig")); }
test { std.testing.refAllDecls(@import("net/wg_show_collector.zig")); }
test { std.testing.refAllDecls(@import("net/wg_show_collector_tests.zig")); }
test { std.testing.refAllDecls(@import("net/wg_status_boundary.zig")); }
test { std.testing.refAllDecls(@import("net/wg_status_boundary_cli.zig")); }
test { std.testing.refAllDecls(@import("net/wg_status_boundary_netlink.zig")); }
test { std.testing.refAllDecls(@import("net/wg_status_boundary_netlink_tests.zig")); }
test { std.testing.refAllDecls(@import("net/private_interface_stats.zig")); }
test { std.testing.refAllDecls(@import("net/private_interface_stats_tests.zig")); }
test { std.testing.refAllDecls(@import("net/iptables.zig")); }
test { std.testing.refAllDecls(@import("status_vpn_masquerade.zig")); }
test { std.testing.refAllDecls(@import("net/network_diag_config.zig")); }
test { std.testing.refAllDecls(@import("net/wg_dump_parser.zig")); }
test { std.testing.refAllDecls(@import("net/diag_event_ring.zig")); }
test { std.testing.refAllDecls(@import("net/safe_command.zig")); }
test { std.testing.refAllDecls(@import("net/ss_parser.zig")); }
test { std.testing.refAllDecls(@import("net/ss_parser_tests.zig")); }
test { std.testing.refAllDecls(@import("status_network_diag.zig")); }
test { std.testing.refAllDecls(@import("status_network_diag_types.zig")); }
test { std.testing.refAllDecls(@import("status_network_diag_events.zig")); }
test { std.testing.refAllDecls(@import("status_network_diag_tcp.zig")); }
test { std.testing.refAllDecls(@import("status_network_diag_tests.zig")); }
test { std.testing.refAllDecls(@import("status_network_diag_ownership_tests.zig")); }
test { std.testing.refAllDecls(@import("net/route_diag.zig")); }
test { std.testing.refAllDecls(@import("net/extended_interface_stats.zig")); }
test { std.testing.refAllDecls(@import("net/wg_dump_collector.zig")); }
test { std.testing.refAllDecls(@import("runtime/lab_events.zig")); }
test { std.testing.refAllDecls(@import("runtime/lab_events_tests.zig")); }
test { std.testing.refAllDecls(@import("net/wg_status_boundary_netlink_runtime_tests.zig")); }
