// test_suite_bgp_integration.zig — BGP config parsing and serve integration tests
//
// Tests: config parsing, serve integration, status reporting, reconnect lifecycle,
// and passive listener tests (real socket behavior).

const std = @import("std");

// Import modules for split test inventory (required pattern)
// Note: config_parse, serve_integration, serve_integration_tests, serve_lifetime_tests,
// status are already in test_suite_bgp.zig - only add new ACT 5 modules here
const _bgp_reconnect_lifecycle = @import("bgp/reconnect_lifecycle.zig");
const _bgp_backoff_tests = @import("bgp/backoff_tests.zig");
const _bgp_lifecycle_tests = @import("bgp/lifecycle_tests.zig");
const _bgp_runtime_tests = @import("bgp/runtime_tests.zig");

// Passive listener tests (real socket behavior tests)
const _passive_listener_config_tests = @import("bgp/passive_listener_config_tests.zig");
const _passive_listener_integration_tests = @import("bgp/passive_listener_integration_tests.zig");
const _passive_listener_serve_integration_tests = @import("bgp/passive_listener_serve_integration_tests.zig");

// Same-AS/iBGP AS_PATH regression tests
const _same_as_regression_tests = @import("bgp/same_as_regression_tests.zig");

// Session config builder tests (extracted from serve_integration.zig)
const _session_config_builder = @import("bgp/session_config_builder.zig");

// BGP reconnect regression tests (ACT: BGP post-glitch reconnect/reset diagnostics)
const _bgp_reconnect_regression_tests = @import("bgp/bgp_reconnect_regression_tests.zig");

// Prefix file watcher tests (ACT: inotify watcher for BGP prefix files)
const _prefix_watch_tests = @import("bgp/prefix_watch_tests.zig");

// Force test discovery for BGP integration layer
test { std.testing.refAllDecls(@import("bgp/reconnect_lifecycle.zig")); }
test { std.testing.refAllDecls(@import("bgp/backoff_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/lifecycle_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/runtime_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/passive_listener_config_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/passive_listener_integration_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/passive_listener_serve_integration_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/same_as_regression_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/session_config_builder.zig")); }
test { std.testing.refAllDecls(@import("bgp/prefix_watch_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/bgp_reconnect_regression_tests.zig")); }
