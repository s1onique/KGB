// test_suite_bfd.zig — BFD test suite
//
// Tests: BFD protocol packet encoding/decoding, session state machine, runtime.
// Uses FakeTransport for isolation - no real sockets or threads.

const std = @import("std");

// BFD modules
const _bfd_packet = @import("bfd/packet.zig");
const _bfd_config = @import("bfd/config.zig");
const _bfd_clock = @import("bfd/clock.zig");
const _bfd_session = @import("bfd/session.zig");
const _bfd_packet_tests = @import("bfd/packet_tests.zig");
const _bfd_session_tests = @import("bfd/session_tests.zig");
const _bfd_session_bird_tests = @import("bfd/session_bird_tests.zig");
const _bfd_smoke_test = @import("bfd/smoke_test.zig");
const _bfd_transport = @import("bfd/transport.zig");
const _bfd_transport_tests = @import("bfd/transport_tests.zig");
const _bfd_runtime = @import("bfd/runtime.zig");
const _bfd_runtime_tests = @import("bfd/runtime_tests.zig");
const _bfd_status = @import("bfd/status.zig");
const _bfd_receive_startup_tests = @import("bfd/receive_startup_tests.zig");
const _bfd_receive_tests = @import("bfd/receive_tests.zig");

// Force test discovery
test { std.testing.refAllDecls(@import("bfd/packet.zig")); }
test { std.testing.refAllDecls(@import("bfd/config.zig")); }
test { std.testing.refAllDecls(@import("bfd/clock.zig")); }
test { std.testing.refAllDecls(@import("bfd/session.zig")); }
test { std.testing.refAllDecls(@import("bfd/packet_tests.zig")); }
test { std.testing.refAllDecls(@import("bfd/session_tests.zig")); }
test { std.testing.refAllDecls(@import("bfd/session_bird_tests.zig")); }
test { std.testing.refAllDecls(@import("bfd/smoke_test.zig")); }
test { std.testing.refAllDecls(@import("bfd/transport.zig")); }
test { std.testing.refAllDecls(@import("bfd/transport_tests.zig")); }
test { std.testing.refAllDecls(@import("bfd/runtime.zig")); }
test { std.testing.refAllDecls(@import("bfd/runtime_tests.zig")); }
test { std.testing.refAllDecls(@import("bfd/status.zig")); }
test { std.testing.refAllDecls(@import("bfd/receive_startup_tests.zig")); }
test { std.testing.refAllDecls(@import("bfd/receive_tests.zig")); }
