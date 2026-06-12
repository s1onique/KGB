// test_suite_bgp_protocol.zig — BGP protocol encoding/decoding tests
//
// Tests: types, message encoding/decoding, validation, prefix file parsing,
// frame decoding. No socket/transport dependencies.

const std = @import("std");

// Force test discovery for BGP protocol layer
test { std.testing.refAllDecls(@import("bgp/types.zig")); }
test { std.testing.refAllDecls(@import("bgp/message.zig")); }
test { std.testing.refAllDecls(@import("bgp/message_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/validation.zig")); }
test { std.testing.refAllDecls(@import("bgp/prefix_file.zig")); }
test { std.testing.refAllDecls(@import("bgp/prefix_file_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/frame_decode.zig")); }
