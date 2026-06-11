// test_suite_bgp_integration.zig — BGP config parsing and serve integration tests
//
// Tests: config parsing, serve integration, status reporting, reconnect lifecycle.
// No runtime sockets — parse-only validation.

const std = @import("std");

// Import modules for split test inventory (required pattern)
// Note: config_parse, serve_integration, serve_integration_tests, serve_lifetime_tests,
// status are already in test_suite_bgp.zig - only add new ACT 5 modules here
const _bgp_reconnect_lifecycle = @import("bgp/reconnect_lifecycle.zig");
const _bgp_backoff_tests = @import("bgp/backoff_tests.zig");
const _bgp_lifecycle_tests = @import("bgp/lifecycle_tests.zig");

// Force test discovery for BGP integration layer
test { std.testing.refAllDecls(@import("bgp/reconnect_lifecycle.zig")); }
test { std.testing.refAllDecls(@import("bgp/backoff_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/lifecycle_tests.zig")); }
