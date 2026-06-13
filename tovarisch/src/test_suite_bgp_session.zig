// test_suite_bgp_session.zig — BGP session state machine tests
//
// Tests: session status, transport abstraction, session state machine,
// handshake flow with FakeTransport, KEEPALIVE scheduler with MockClock.
// No real sockets.
//
// Note: This suite only includes modules NOT already in test_suite_bgp.zig

const std = @import("std");

// Import new test modules (for split test inventory)
const _session_update_capture_tests = @import("bgp/session_update_capture_tests.zig");
const _update_frame_decode_tests = @import("bgp/update_frame_decode_tests.zig");

// Force test discovery for new UPDATE capture tests
test { std.testing.refAllDecls(@import("bgp/session_update_capture_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/update_frame_decode_tests.zig")); }
