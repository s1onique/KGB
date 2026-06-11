// test_suite_bgp_session.zig — BGP session state machine tests
//
// Tests: session status, transport abstraction, session state machine,
// handshake flow with FakeTransport, KEEPALIVE scheduler with MockClock.
// No real sockets.

const std = @import("std");

// Force test discovery for BGP session layer
test { std.testing.refAllDecls(@import("bgp/session_status.zig")); }
test { std.testing.refAllDecls(@import("bgp/transport.zig")); }
test { std.testing.refAllDecls(@import("bgp/clock.zig")); }
test { std.testing.refAllDecls(@import("bgp/notification_decode.zig")); }
test { std.testing.refAllDecls(@import("bgp/session.zig")); }
test { std.testing.refAllDecls(@import("bgp/session_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/session_handshake_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/session_keepalive_basic_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/session_keepalive_notification_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/session_keepalive_advanced_tests.zig")); }
