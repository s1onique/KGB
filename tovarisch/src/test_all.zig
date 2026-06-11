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
const _runtime_telemetry = @import("runtime/telemetry.zig");
const _runtime_heartbeat_log = @import("runtime/heartbeat_log.zig");

// BFD multihop module tests
const _bfd_packet = @import("bfd/packet.zig");
const _bfd_config = @import("bfd/config.zig");
const _bfd_clock = @import("bfd/clock.zig");
const _bfd_session = @import("bfd/session.zig");
const _bfd_packet_tests = @import("bfd/packet_tests.zig");
const _bfd_session_tests = @import("bfd/session_tests.zig");
const _bfd_smoke_test = @import("bfd/smoke_test.zig");
const _bfd_transport = @import("bfd/transport.zig");
const _bfd_transport_tests = @import("bfd/transport_tests.zig");
const _bfd_runtime = @import("bfd/runtime.zig");
const _bfd_runtime_tests = @import("bfd/runtime_tests.zig");
const _bfd_status = @import("bfd/status.zig");

// Force test discovery for all imported modules
// This ensures the test binary actually runs the tests from these modules
test {
    std.testing.refAllDecls(@import("net/private_ip.zig"));
}

test {
    std.testing.refAllDecls(@import("net/rates.zig"));
}

test {
    std.testing.refAllDecls(@import("net/interface_sampler.zig"));
}

test {
    std.testing.refAllDecls(@import("net/interface_sampler_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("net/linux_stats.zig"));
}

test {
    std.testing.refAllDecls(@import("net/linux_stats_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("net/linux_interfaces.zig"));
}

test {
    std.testing.refAllDecls(@import("net/linux_interfaces_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("net/linux_interface_stats.zig"));
}

test {
    std.testing.refAllDecls(@import("net/linux_interface_stats_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("net/interface_filter.zig"));
}

test {
    std.testing.refAllDecls(@import("net/interface_filter_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("net/linux_addr.zig"));
}

test {
    std.testing.refAllDecls(@import("net/linux_addr_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("net/linux_addr_parse.zig"));
}

test {
    std.testing.refAllDecls(@import("net/wg_show_parser.zig"));
}

test {
    std.testing.refAllDecls(@import("net/wg_show_parser_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("net/wg_show_collector.zig"));
}

test {
    std.testing.refAllDecls(@import("net/wg_show_collector_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("net/private_interface_stats.zig"));
}

test {
    std.testing.refAllDecls(@import("net/private_interface_stats_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("metrics.zig"));
}

test {
    std.testing.refAllDecls(@import("metrics_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("metrics_conversion_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("metrics_dto.zig"));
}

test {
    std.testing.refAllDecls(@import("metrics_dto_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("metrics_fallback_dto_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("metrics_state.zig"));
}

test {
    std.testing.refAllDecls(@import("metrics_state_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("metrics_tunnel_contract_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("cli.zig"));
}

test {
    std.testing.refAllDecls(@import("status.zig"));
}

test {
    std.testing.refAllDecls(@import("status_checks.zig"));
}

test {
    std.testing.refAllDecls(@import("status_ownership_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("http/response.zig"));
}

test {
    std.testing.refAllDecls(@import("http/routes.zig"));
}

test {
    std.testing.refAllDecls(@import("http/routes_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("http/server.zig"));
}

test {
    std.testing.refAllDecls(@import("runtime/telemetry.zig"));
}

test {
    std.testing.refAllDecls(@import("runtime/heartbeat_log.zig"));
}

test {
    std.testing.refAllDecls(@import("logging.zig"));
}

// BFD module tests
test {
    std.testing.refAllDecls(@import("bfd/packet.zig"));
}

test {
    std.testing.refAllDecls(@import("bfd/config.zig"));
}

test {
    std.testing.refAllDecls(@import("bfd/clock.zig"));
}

test {
    std.testing.refAllDecls(@import("bfd/session.zig"));
}

test {
    std.testing.refAllDecls(@import("bfd/packet_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("bfd/session_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("bfd/smoke_test.zig"));
}

test {
    std.testing.refAllDecls(@import("bfd/transport.zig"));
}

test {
    std.testing.refAllDecls(@import("bfd/transport_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("bfd/runtime.zig"));
}

test {
    std.testing.refAllDecls(@import("bfd/runtime_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("bfd/status.zig"));
}

// WireGuard config module tests
test {
    std.testing.refAllDecls(@import("config.zig"));
}

test {
    std.testing.refAllDecls(@import("wg/config.zig"));
}

test {
    std.testing.refAllDecls(@import("wg/generate.zig"));
}

test {
    std.testing.refAllDecls(@import("cli/wg_args.zig"));
}

test {
    std.testing.refAllDecls(@import("cli/bfd_serve.zig"));
}

test {
    std.testing.refAllDecls(@import("cli_serve_config_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("bfd_serve_config_tests.zig"));
}

// BGP protocol module tests (ACT 1: pure encoding/parsing only)
test {
    std.testing.refAllDecls(@import("bgp/types.zig"));
}

test {
    std.testing.refAllDecls(@import("bgp/message.zig"));
}

test {
    std.testing.refAllDecls(@import("bgp/message_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("bgp/validation.zig"));
}

test {
    std.testing.refAllDecls(@import("bgp/prefix_file.zig"));
}

// BGP session module tests (ACT 2: TCP session state machine)
test {
    std.testing.refAllDecls(@import("bgp/frame_decode.zig"));
}

test {
    std.testing.refAllDecls(@import("bgp/session_status.zig"));
}

test {
    std.testing.refAllDecls(@import("bgp/transport.zig"));
}

test {
    std.testing.refAllDecls(@import("bgp/session.zig"));
}

test {
    std.testing.refAllDecls(@import("bgp/session_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("bgp/session_handshake_tests.zig"));
}

// BGP TCP transport tests (ACT 3: real TCP transport)
test {
    std.testing.refAllDecls(@import("bgp/tcp_transport.zig"));
}

test {
    std.testing.refAllDecls(@import("bgp/tcp_transport_helpers.zig"));
}

test {
    std.testing.refAllDecls(@import("bgp/tcp_transport_tests.zig"));
}

// BGP send failure propagation tests
test {
    std.testing.refAllDecls(@import("bgp/send_failure_tests.zig"));
}

// BGP runtime integration tests (ACT 4: serve runtime wiring)
test {
    std.testing.refAllDecls(@import("bgp/config_parse.zig"));
}

test {
    std.testing.refAllDecls(@import("bgp/serve_integration.zig"));
}

test {
    std.testing.refAllDecls(@import("bgp/serve_integration_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("bgp/transport_ownership_tests.zig"));
}

test {
    std.testing.refAllDecls(@import("cli/bgp_serve.zig"));
}

// BGP status module tests (ACT 5: status exposure)
test {
    std.testing.refAllDecls(@import("bgp/status.zig"));
}

// BGP status integration tests
test {
    std.testing.refAllDecls(@import("status_bgp_tests.zig"));
}
