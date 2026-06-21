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
const _runtime_telemetry = @import("runtime/telemetry.zig");
const _runtime_heartbeat_log = @import("runtime/heartbeat_log.zig");

// Status + network diagnostics wiring tests
const _status_network_diag_wiring_tests = @import("status_network_diag_wiring_tests.zig");

// Force test discovery
test { std.testing.refAllDecls(@import("http/response.zig")); }
test { std.testing.refAllDecls(@import("http/routes.zig")); }
test { std.testing.refAllDecls(@import("http/routes_tests.zig")); }
test { std.testing.refAllDecls(@import("http/server.zig")); }
test { std.testing.refAllDecls(@import("runtime/telemetry.zig")); }
test { std.testing.refAllDecls(@import("runtime/heartbeat_log.zig")); }
test { std.testing.refAllDecls(@import("status_network_diag_wiring_tests.zig")); }
