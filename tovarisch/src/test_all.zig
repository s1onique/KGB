// test_all.zig — Aggregate test root for tovarisch
//
// This file forces Zig's test discovery to find tests in all modules.
// Use this as the root_source_file for test steps to ensure every module's
// tests are included in the test binary.
//
// Pattern: import every module with tests, then refAllDecls to force linking.
const std = @import("std");

// Import all source modules to ensure they are compiled and their tests discovered
const _cli = @import("cli.zig");
const _status = @import("status.zig");
const _status_checks = @import("status_checks.zig");
const _status_ownership_tests = @import("status_ownership_tests.zig");
const _status_query = @import("status_query.zig");
const _status_response = @import("status_response.zig");
const _status_response_test = @import("status_response_test.zig");
// Route contract table (ACT-TOVARISCH-ZIG-HULK02)
const _status_route_contract = @import("http/status_route_contract.zig");
const _status_route_contract_test = @import("http/status_route_contract_test.zig");
const _logging = @import("logging.zig");
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
const _net_private_interface_stats = @import("net/private_interface_stats.zig");
const _net_private_interface_stats_tests = @import("net/private_interface_stats_tests.zig");
const _metrics = @import("metrics.zig");
const _metrics_tests = @import("metrics_tests.zig");
const _metrics_conversion_tests = @import("metrics_conversion_tests.zig");
const _metrics_dto = @import("metrics_dto.zig");
const _metrics_dto_tests = @import("metrics_dto_tests.zig");
const _metrics_fallback_dto_tests = @import("metrics_fallback_dto_tests.zig");
const _metrics_state = @import("metrics_state.zig");
const _metrics_state_tests = @import("metrics_state_tests.zig");
const _metrics_tunnel_contract_tests = @import("metrics_tunnel_contract_tests.zig");
const _http_response = @import("http/response.zig");
const _http_routes = @import("http/routes.zig");
const _http_routes_tests = @import("http/routes_tests.zig");
const _http_server = @import("http/server.zig");
const _http_heartbeat = @import("http/heartbeat.zig");
const _heartbeat_idle_memory_regression_tests = @import("http/heartbeat_idle_memory_regression_tests.zig");
const _idle_memory_attribution_tests = @import("http/idle_memory_attribution_tests.zig");
const _runtime_telemetry = @import("runtime/telemetry.zig");
const _runtime_heartbeat_log = @import("runtime/heartbeat_log.zig");
const _runtime_lab_events = @import("runtime/lab_events.zig");
const _runtime_lab_events_tests = @import("runtime/lab_events_tests.zig");

// BFD multihop module tests
const _bfd_packet = @import("bfd/packet.zig");
const _bfd_config = @import("bfd/config.zig");
const _bfd_clock = @import("bfd/clock.zig");
const _bfd_session = @import("bfd/session.zig");
const _bfd_packet_tests = @import("bfd/packet_tests.zig");
const _bfd_session_tests = @import("bfd/session_tests.zig");
const _bfd_session_bird_tests = @import("bfd/session_bird_tests.zig");
const _bfd_smoke_test = @import("bfd/smoke_test.zig");
const _bfd_transport = @import("bfd/transport.zig");
const _bfd_transport_tests = @import("bfd/transport_tests.zig");
const _bfd_runtime = @import("bfd/runtime.zig");
const _bfd_runtime_tests = @import("bfd/runtime_tests.zig");
const _bfd_status = @import("bfd/status.zig");
const _bfd_receive_startup_tests = @import("bfd/receive_startup_tests.zig");
const _bfd_receive_tests = @import("bfd/receive_tests.zig");

// Force test discovery for all imported modules
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
test { std.testing.refAllDecls(@import("net/private_interface_stats.zig")); }
test { std.testing.refAllDecls(@import("net/private_interface_stats_tests.zig")); }
test { std.testing.refAllDecls(@import("metrics.zig")); }
test { std.testing.refAllDecls(@import("metrics_tests.zig")); }
test { std.testing.refAllDecls(@import("metrics_conversion_tests.zig")); }
test { std.testing.refAllDecls(@import("metrics_dto.zig")); }
test { std.testing.refAllDecls(@import("metrics_dto_tests.zig")); }
test { std.testing.refAllDecls(@import("metrics_fallback_dto_tests.zig")); }
test { std.testing.refAllDecls(@import("metrics_state.zig")); }
test { std.testing.refAllDecls(@import("metrics_state_tests.zig")); }
test { std.testing.refAllDecls(@import("metrics_tunnel_contract_tests.zig")); }
test { std.testing.refAllDecls(@import("cli.zig")); }
test { std.testing.refAllDecls(@import("status.zig")); }
test { std.testing.refAllDecls(@import("status_checks.zig")); }
test { std.testing.refAllDecls(@import("status_ownership_tests.zig")); }
test { std.testing.refAllDecls(@import("status_query.zig")); }
test { std.testing.refAllDecls(@import("status_response.zig")); }
test { std.testing.refAllDecls(@import("status_response_test.zig")); }
// Route contract table tests (ACT-TOVARISCH-ZIG-HULK02)
test { std.testing.refAllDecls(@import("http/status_route_contract.zig")); }
test { std.testing.refAllDecls(@import("http/status_route_contract_test.zig")); }
test { std.testing.refAllDecls(@import("http/response.zig")); }
test { std.testing.refAllDecls(@import("http/routes.zig")); }
test { std.testing.refAllDecls(@import("http/routes_tests.zig")); }
test { std.testing.refAllDecls(@import("http/server.zig")); }
test { std.testing.refAllDecls(@import("http/heartbeat.zig")); }
test { std.testing.refAllDecls(@import("runtime/telemetry.zig")); }
test { std.testing.refAllDecls(@import("runtime/heartbeat_log.zig")); }
test { std.testing.refAllDecls(@import("runtime/lab_events.zig")); }
test { std.testing.refAllDecls(@import("runtime/lab_events_tests.zig")); }
test { std.testing.refAllDecls(@import("logging.zig")); }

// WireGuard status boundary tests
test { std.testing.refAllDecls(@import("net/wg_status_boundary.zig")); }
test { std.testing.refAllDecls(@import("net/wg_status_boundary_cli.zig")); }
test { std.testing.refAllDecls(@import("net/wg_status_boundary_netlink.zig")); }
test { std.testing.refAllDecls(@import("net/wg_status_boundary_netlink_tests.zig")); }
test { std.testing.refAllDecls(@import("net/wg_status_boundary_netlink_runtime_tests.zig")); }

// BFD module tests
test { std.testing.refAllDecls(@import("bfd/packet.zig")); }
test { std.testing.refAllDecls(@import("bfd/config.zig")); }
test { std.testing.refAllDecls(@import("bfd/clock.zig")); }
test { std.testing.refAllDecls(@import("bfd/session.zig")); }
test { std.testing.refAllDecls(@import("bfd/packet_tests.zig")); }
test { std.testing.refAllDecls(@import("bfd/session_tests.zig")); }
test { std.testing.refAllDecls(@import("bfd/session_bird_tests.zig")); }
test { std.testing.refAllDecls(@import("bfd/smoke_test.zig")); }
test { std.testing.refAllDecls(@import("bfd/transport.zig")); }
test { std.testing.refAllDecls(@import("bfd/transport_tests.zig")); }
test { std.testing.refAllDecls(@import("bfd/runtime.zig")); }
test { std.testing.refAllDecls(@import("bfd/runtime_tests.zig")); }
test { std.testing.refAllDecls(@import("bfd/status.zig")); }
test { std.testing.refAllDecls(@import("bfd/receive_startup_tests.zig")); }
test { std.testing.refAllDecls(@import("bfd/receive_tests.zig")); }
// BFD snapshot budget/state hardening (ACT-TOVARISCH-ZIG-HULK14)
test { std.testing.refAllDecls(@import("bfd/snapshot.zig")); }
test { std.testing.refAllDecls(@import("bfd/snapshot_tests.zig")); }

// WireGuard tests
test { std.testing.refAllDecls(@import("config.zig")); }
test { std.testing.refAllDecls(@import("config_server_tests.zig")); }
test { std.testing.refAllDecls(@import("config_vpn_masquerade_tests.zig")); }
test { std.testing.refAllDecls(@import("wg/config.zig")); }
test { std.testing.refAllDecls(@import("wg/generate.zig")); }
test { std.testing.refAllDecls(@import("cli/wg_args.zig")); }
test { std.testing.refAllDecls(@import("cli/bfd_serve.zig")); }
test { std.testing.refAllDecls(@import("cli_serve_config_tests.zig")); }
test { std.testing.refAllDecls(@import("bfd_serve_config_tests.zig")); }

// CLI tests
test { std.testing.refAllDecls(@import("cli/args_explicit_listen_tests.zig")); }

// VPN masquerade tests (ACT: Add config-controlled VPN masquerade rule with rule watcher)
test { std.testing.refAllDecls(@import("net/iptables.zig")); }
test { std.testing.refAllDecls(@import("status_vpn_masquerade.zig")); }

// BGP protocol module tests
test { std.testing.refAllDecls(@import("bgp/types.zig")); }
test { std.testing.refAllDecls(@import("bgp/message.zig")); }
test { std.testing.refAllDecls(@import("bgp/message_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/message_wire_format_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/validation.zig")); }
test { std.testing.refAllDecls(@import("bgp/prefix_file.zig")); }

// BGP session module tests
test { std.testing.refAllDecls(@import("bgp/frame_decode.zig")); }
test { std.testing.refAllDecls(@import("bgp/session_status.zig")); }
test { std.testing.refAllDecls(@import("bgp/transport.zig")); }
test { std.testing.refAllDecls(@import("bgp/clock.zig")); }
test { std.testing.refAllDecls(@import("bgp/notification_decode.zig")); }
test { std.testing.refAllDecls(@import("bgp/session.zig")); }
test { std.testing.refAllDecls(@import("bgp/session_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/session_handshake_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/session_buffer_compaction_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/session_keepalive_basic_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/session_keepalive_notification_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/session_keepalive_advanced_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/session_update_capture_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/update_frame_decode_tests.zig")); }

// BGP TCP transport tests
test { std.testing.refAllDecls(@import("bgp/tcp_transport.zig")); }
test { std.testing.refAllDecls(@import("bgp/tcp_transport_helpers.zig")); }
test { std.testing.refAllDecls(@import("bgp/tcp_transport_tests.zig")); }

// BGP send failure propagation tests
test { std.testing.refAllDecls(@import("bgp/send_failure_tests.zig")); }

// BGP runtime integration tests
test { std.testing.refAllDecls(@import("bgp/config_parse.zig")); }
test { std.testing.refAllDecls(@import("bgp/serve_integration.zig")); }
test { std.testing.refAllDecls(@import("bgp/serve_integration_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/serve_lifetime_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/transport_ownership_tests.zig")); }

// BGP prefix file tests
test { std.testing.refAllDecls(@import("bgp/prefix_file_loader.zig")); }
test { std.testing.refAllDecls(@import("bgp/prefix_file_integration_tests.zig")); }
test { std.testing.refAllDecls(@import("cli/bgp_serve.zig")); }

// BGP prefix file watcher tests (ACT: inotify watcher for BGP prefix files)
test { std.testing.refAllDecls(@import("bgp/prefix_watch_tests.zig")); }

// BGP export delta tests (ACT: Apply BGP export deltas after watched prefix reload)
test { std.testing.refAllDecls(@import("bgp/export_delta.zig")); }
test { std.testing.refAllDecls(@import("bgp/session_delta.zig")); }
test { std.testing.refAllDecls(@import("bgp/session_delta_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/export_delta_workflow_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/export_reload_apply.zig")); }
test { std.testing.refAllDecls(@import("bgp/export_reload_apply_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/export_reload_apply_integration_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/serve_export_integration.zig")); }
test { std.testing.refAllDecls(@import("bgp/serve_export_integration_tests.zig")); }

// BGP status tests
test { std.testing.refAllDecls(@import("bgp/status.zig")); }

// BGP snapshot budget/state hardening (ACT-TOVARISCH-ZIG-HULK14)
test { std.testing.refAllDecls(@import("bgp/snapshot.zig")); }
test { std.testing.refAllDecls(@import("bgp/snapshot_tests.zig")); }
// BGP/BFD budget contract tests (ACT-TOVARISCH-ZIG-HULK17R2)
test { std.testing.refAllDecls(@import("bgp/budget_contract_tests.zig")); }

// BGP reconnect/backoff lifecycle tests
test { std.testing.refAllDecls(@import("bgp/reconnect_lifecycle.zig")); }
test { std.testing.refAllDecls(@import("bgp/backoff_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/lifecycle_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/runtime_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/bgp_reconnect_regression_tests.zig")); }

// BGP hold timer expiry recovery tests (this ACT)
test { std.testing.refAllDecls(@import("bgp/reconnect_hold_timer_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/reconnect_recovery_tests.zig")); }

// BGP status tests (split into focused files)
test { std.testing.refAllDecls(@import("status_bgp_integration_tests.zig")); }
test { std.testing.refAllDecls(@import("status_bgp_state_tests.zig")); }
test { std.testing.refAllDecls(@import("status_bgp_error_tests.zig")); }
test { std.testing.refAllDecls(@import("status_bgp_fsm_tests.zig")); }

// Passive listener tests (split into focused files)
test { std.testing.refAllDecls(@import("bgp/passive_listener_config_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/passive_listener_integration_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/passive_listener_serve_integration_tests.zig")); }

// Same-AS/iBGP AS_PATH regression tests
test { std.testing.refAllDecls(@import("bgp/same_as_regression_tests.zig")); }

// Session config builder tests
test { std.testing.refAllDecls(@import("bgp/session_config_builder.zig")); }

// ============================================================================
// Network Diagnostics (ACT: Add tovarisch WireGuard and XRay TCP underlay diagnostics)
// ============================================================================

// Network diagnostics config
test { std.testing.refAllDecls(@import("net/network_diag_config.zig")); }

// WireGuard dump parser
test { std.testing.refAllDecls(@import("net/wg_dump_parser.zig")); }

// Diagnostic event ring
test { std.testing.refAllDecls(@import("net/diag_event_ring.zig")); }

// Safe command runner
test { std.testing.refAllDecls(@import("net/safe_command.zig")); }

// Linux sysfs/procfs file boundary (ACT-TOVARISCH-ZIG-HULK13)
test { std.testing.refAllDecls(@import("net/linux_read.zig")); }
test { std.testing.refAllDecls(@import("net/linux_read_fixture_tests.zig")); }

// TCP underlay (ss) parser
test { std.testing.refAllDecls(@import("net/ss_parser.zig")); }
test { std.testing.refAllDecls(@import("net/ss_parser_tests.zig")); }

// Route diagnostics
test { std.testing.refAllDecls(@import("net/route_diag.zig")); }

// Extended interface stats
test { std.testing.refAllDecls(@import("net/extended_interface_stats.zig")); }

// WireGuard dump collector
test { std.testing.refAllDecls(@import("net/wg_dump_collector.zig")); }

// Status network diagnostics integration
test { std.testing.refAllDecls(@import("status_network_diag.zig")); }
test { std.testing.refAllDecls(@import("status_network_diag_types.zig")); }
test { std.testing.refAllDecls(@import("status_network_diag_events.zig")); }
test { std.testing.refAllDecls(@import("status_network_diag_tcp.zig")); }
test { std.testing.refAllDecls(@import("status_network_diag_tests.zig")); }
test { std.testing.refAllDecls(@import("status_network_diag_ownership_tests.zig")); }

// Network diagnostics config wiring regression tests
// ACT: Wire parsed tovarisch network diagnostics config into HTTP status path
test { std.testing.refAllDecls(@import("status_network_diag_wiring_tests.zig")); }
