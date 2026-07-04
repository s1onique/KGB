// test_suite_http.zig — HTTP server test suite
//
// Tests: HTTP response formatting, routes, server initialization.
// No real server sockets in tests - ServerState is initialized and immediately deinited.

const std = @import("std");

// HTTP modules
const _http_response = @import("http/response.zig");
const _http_routes = @import("http/routes.zig");
const _http_routes_tests = @import("http/routes_tests.zig");
const _http_server = @import("http/server.zig");
const _http_heartbeat = @import("http/heartbeat.zig");
const _status_route_contract = @import("http/status_route_contract.zig");
const _status_route_contract_test = @import("http/status_route_contract_test.zig");
const _runtime_telemetry = @import("runtime/telemetry.zig");
const _runtime_heartbeat_log = @import("runtime/heartbeat_log.zig");

// Status + network diagnostics wiring tests
const _status_network_diag_wiring_tests = @import("status_network_diag_wiring_tests.zig");

// Memory regression tests (ACT: Attribute and fix tovarisch idle/background staircase memory growth)
const _heartbeat_idle_memory_regression_tests = @import("http/heartbeat_idle_memory_regression_tests.zig");
const _idle_memory_attribution_tests = @import("http/idle_memory_attribution_tests.zig");

// Force test discovery
test { std.testing.refAllDecls(@import("http/response.zig")); }
test { std.testing.refAllDecls(@import("http/routes.zig")); }
test { std.testing.refAllDecls(@import("http/routes_tests.zig")); }
test { std.testing.refAllDecls(@import("http/server.zig")); }
test { std.testing.refAllDecls(@import("http/heartbeat.zig")); }
test { std.testing.refAllDecls(@import("runtime/telemetry.zig")); }
test { std.testing.refAllDecls(@import("runtime/heartbeat_log.zig")); }
test { std.testing.refAllDecls(@import("status_network_diag_wiring_tests.zig")); }
test { std.testing.refAllDecls(@import("http/heartbeat_idle_memory_regression_tests.zig")); }
// Route contract table tests (ACT-TOVARISCH-ZIG-HULK02)
test { std.testing.refAllDecls(@import("http/status_route_contract.zig")); }
test { std.testing.refAllDecls(@import("http/status_route_contract_test.zig")); }
// Active route proof tests (ACT-TOVARISCH-ZIG-HULK06)
test { std.testing.refAllDecls(@import("http/status_route_active_tests.zig")); }
// Handler-level fd/socket proof tests (ACT-TOVARISCH-ZIG-HULK09)
test { std.testing.refAllDecls(@import("http/status_handler_fd_tests.zig")); }
