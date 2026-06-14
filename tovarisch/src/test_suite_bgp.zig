// test_suite_bgp.zig — BGP test suite
//
// Tests: BGP protocol encoding/decoding, session state machine, TCP transport.
// Note: Some tests use loopback sockets for TCP transport tests.

const std = @import("std");

// BGP protocol modules (encoding/decoding only)
const _bgp_types = @import("bgp/types.zig");
const _bgp_message = @import("bgp/message.zig");
const _bgp_message_tests = @import("bgp/message_tests.zig");
const _bgp_message_wire_format_tests = @import("bgp/message_wire_format_tests.zig");
const _bgp_validation = @import("bgp/validation.zig");
const _bgp_prefix_file = @import("bgp/prefix_file.zig");

// BGP session modules (state machine with FakeTransport)
const _bgp_frame_decode = @import("bgp/frame_decode.zig");
const _bgp_session_status = @import("bgp/session_status.zig");
const _bgp_transport = @import("bgp/transport.zig");
const _bgp_clock = @import("bgp/clock.zig");
const _bgp_notification_decode = @import("bgp/notification_decode.zig");
const _bgp_session = @import("bgp/session.zig");
const _bgp_session_tests = @import("bgp/session_tests.zig");
const _bgp_session_handshake_tests = @import("bgp/session_handshake_tests.zig");
const _bgp_session_keepalive_basic_tests = @import("bgp/session_keepalive_basic_tests.zig");
const _bgp_session_keepalive_notification_tests = @import("bgp/session_keepalive_notification_tests.zig");
const _bgp_session_keepalive_advanced_tests = @import("bgp/session_keepalive_advanced_tests.zig");

// BGP TCP transport (uses loopback sockets - potential hang source on Linux)
const _bgp_tcp_transport = @import("bgp/tcp_transport.zig");
const _bgp_tcp_transport_tests = @import("bgp/tcp_transport_tests.zig");

// BGP serve integration (config parsing, no runtime sockets)
const _bgp_config_parse = @import("bgp/config_parse.zig");
const _bgp_serve_integration = @import("bgp/serve_integration.zig");
const _bgp_serve_integration_tests = @import("bgp/serve_integration_tests.zig");
const _bgp_serve_lifetime_tests = @import("bgp/serve_lifetime_tests.zig");
const _bgp_transport_ownership_tests = @import("bgp/transport_ownership_tests.zig");
const _bgp_serve_export_integration = @import("bgp/serve_export_integration.zig");
const _bgp_serve_export_integration_tests = @import("bgp/serve_export_integration_tests.zig");
const _bgp_prefix_file_loader = @import("bgp/prefix_file_loader.zig");
const _bgp_prefix_file_integration_tests = @import("bgp/prefix_file_integration_tests.zig");

// BGP status
const _bgp_status = @import("bgp/status.zig");

// BGP hold timer expiry recovery tests (this ACT)
const _bgp_reconnect_hold_timer_tests = @import("bgp/reconnect_hold_timer_tests.zig");
const _bgp_reconnect_recovery_tests = @import("bgp/reconnect_recovery_tests.zig");

// BGP export delta tests (ACT: Apply BGP export deltas after watched prefix reload)
const _bgp_export_delta = @import("bgp/export_delta.zig");
const _bgp_session_delta = @import("bgp/session_delta.zig");
const _bgp_session_delta_tests = @import("bgp/session_delta_tests.zig");
const _bgp_export_delta_workflow_tests = @import("bgp/export_delta_workflow_tests.zig");
const _bgp_export_reload_apply = @import("bgp/export_reload_apply.zig");
const _bgp_export_reload_apply_tests = @import("bgp/export_reload_apply_tests.zig");
const _bgp_export_reload_apply_integration_tests = @import("bgp/export_reload_apply_integration_tests.zig");

// (New BGP modules owned by test_suite_bgp_tcp.zig)

// Force test discovery
test { std.testing.refAllDecls(@import("bgp/types.zig")); }
test { std.testing.refAllDecls(@import("bgp/message.zig")); }
test { std.testing.refAllDecls(@import("bgp/message_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/message_wire_format_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/validation.zig")); }
test { std.testing.refAllDecls(@import("bgp/prefix_file.zig")); }
test { std.testing.refAllDecls(@import("bgp/frame_decode.zig")); }
test { std.testing.refAllDecls(@import("bgp/session_status.zig")); }
test { std.testing.refAllDecls(@import("bgp/transport.zig")); }
test { std.testing.refAllDecls(@import("bgp/session.zig")); }
test { std.testing.refAllDecls(@import("bgp/session_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/session_handshake_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/session_keepalive_basic_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/session_keepalive_notification_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/session_keepalive_advanced_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/tcp_transport.zig")); }
test { std.testing.refAllDecls(@import("bgp/tcp_transport_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/config_parse.zig")); }
test { std.testing.refAllDecls(@import("bgp/serve_integration.zig")); }
test { std.testing.refAllDecls(@import("bgp/serve_integration_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/serve_lifetime_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/transport_ownership_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/serve_export_integration.zig")); }
test { std.testing.refAllDecls(@import("bgp/serve_export_integration_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/prefix_file_loader.zig")); }
test { std.testing.refAllDecls(@import("bgp/prefix_file_integration_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/status.zig")); }

// BGP hold timer expiry recovery tests (this ACT)
test { std.testing.refAllDecls(@import("bgp/reconnect_hold_timer_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/reconnect_recovery_tests.zig")); }

// BGP export delta tests (ACT: Apply BGP export deltas after watched prefix reload)
test { std.testing.refAllDecls(@import("bgp/export_delta.zig")); }
test { std.testing.refAllDecls(@import("bgp/session_delta.zig")); }
test { std.testing.refAllDecls(@import("bgp/session_delta_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/export_delta_workflow_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/export_reload_apply.zig")); }
test { std.testing.refAllDecls(@import("bgp/export_reload_apply_tests.zig")); }
test { std.testing.refAllDecls(@import("bgp/export_reload_apply_integration_tests.zig")); }
