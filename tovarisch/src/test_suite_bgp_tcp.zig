// test_suite_bgp_tcp.zig — BGP TCP transport loopback tests
//
// Tests: TCP transport with loopback sockets.
// POTENTIAL HANG SOURCE: loopback listener thread may block in accept/read/recv.
// Linux CI should watch this suite closely.

const std = @import("std");

// Force test discovery for BGP TCP transport layer
test { std.testing.refAllDecls(@import("bgp/tcp_transport.zig")); }
test { std.testing.refAllDecls(@import("bgp/tcp_transport_tests.zig")); }
