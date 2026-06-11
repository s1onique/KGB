// test_suite_bgp_integration.zig — BGP config parsing and serve integration tests
//
// Tests: config parsing, serve integration, status reporting.
// No runtime sockets — parse-only validation.

const std = @import("std");

// Force test discovery for BGP integration layer
test { std.testing.refAllDecls(@import("bgp/config_parse.zig")); }
test { std.testing.refAllDecls(@import("bgp/serve_integration.zig")); }
test { std.testing.refAllDecls(@import("bgp/serve_integration_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/serve_lifetime_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/status.zig")); }
