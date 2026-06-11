// test_suite_bgp_tcp.zig — BGP TCP transport loopback tests
//
// Tests: TCP transport with loopback sockets.
// Bounded accept/recv behavior prevents indefinite blocking on Linux CI.

const std = @import("std");

// New BGP TCP transport modules added in this ACT
const _tcp_transport_helpers = @import("bgp/tcp_transport_helpers.zig");
const _send_failure_tests = @import("bgp/send_failure_tests.zig");

// Force test discovery for new modules
test { std.testing.refAllDecls(@import("bgp/tcp_transport_helpers.zig")); }
test { std.testing.refAllDecls(@import("bgp/send_failure_tests.zig")); }
