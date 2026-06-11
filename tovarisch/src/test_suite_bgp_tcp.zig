// test_suite_bgp_tcp.zig — BGP TCP transport loopback tests
//
// Tests: TCP transport with loopback sockets.
// Bounded accept/recv behavior prevents indefinite blocking on Linux CI.

const std = @import("std");

// Force test discovery for BGP TCP transport layer
test { std.testing.refAllDecls(@import("bgp/tcp_transport.zig")); }
test { std.testing.refAllDecls(@import("bgp/tcp_transport_tests.zig")); }
