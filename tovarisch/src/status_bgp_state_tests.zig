// status_bgp_state_tests.zig — BgpStatusState unit tests
//
// Tests for BgpStatusState enum variants and buildBgpCheckInto function.

const std = @import("std");
const bgp_status = @import("bgp/status.zig");

test "BgpStatusState.no_config maps to warn status" {
    var scratch: [64]u8 = undefined;
    const check = bgp_status.buildBgpCheckInto(.no_config, &scratch);
    try std.testing.expect(check.status == .warn);
    try std.testing.expectEqualStrings("BGP not configured", check.detail);
}

test "BgpStatusState.not_configured maps to warn status" {
    var scratch: [64]u8 = undefined;
    const check = bgp_status.buildBgpCheckInto(.not_configured, &scratch);
    try std.testing.expect(check.status == .warn);
    try std.testing.expectEqualStrings("BGP not configured", check.detail);
}

test "BgpStatusState.disabled maps to ok status" {
    var scratch: [64]u8 = undefined;
    const check = bgp_status.buildBgpCheckInto(.disabled, &scratch);
    try std.testing.expect(check.status == .ok);
    try std.testing.expectEqualStrings("BGP disabled by config", check.detail);
}

test "BgpStatusState.configured with prefixes maps to ok status" {
    var scratch: [64]u8 = undefined;
    const state = bgp_status.BgpStatusState{
        .configured = .{
            .advertised_prefix_count = 3,
            .fsm_state = "established",
            .peer_address = .{ 10, 0, 0, 2 },
            .peer_as = 65002,
            .local_as = 65001,
            .last_error = null,
            .messages_sent = 5,
            .messages_received = 4,
            .keepalives_sent = 2,
            .keepalives_received = 2,
            .passive_listener_state = .disabled,
            .passive_listener_error = null,
        },
    };
    const check = bgp_status.buildBgpCheckInto(state, &scratch);
    try std.testing.expect(check.status == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, check.detail, 1, "3 advertised prefixes"));
}

test "BgpStatusState.configured with zero prefixes maps to warn status" {
    var scratch: [64]u8 = undefined;
    const state = bgp_status.BgpStatusState{
        .configured = .{
            .advertised_prefix_count = 0,
            .fsm_state = "idle",
            .peer_address = .{ 10, 0, 0, 2 },
            .peer_as = 65002,
            .local_as = 65001,
            .last_error = null,
            .messages_sent = 0,
            .messages_received = 0,
            .keepalives_sent = 0,
            .keepalives_received = 0,
            .passive_listener_state = .disabled,
            .passive_listener_error = null,
        },
    };
    const check = bgp_status.buildBgpCheckInto(state, &scratch);
    try std.testing.expect(check.status == .warn);
    try std.testing.expectEqualStrings("BGP configured with no advertised prefixes", check.detail);
}

test "BgpStatusState.failed maps to error status" {
    var scratch: [64]u8 = undefined;
    const state = bgp_status.BgpStatusState{ .failed = .{ .message = "bad config" } };
    const check = bgp_status.buildBgpCheckInto(state, &scratch);
    try std.testing.expect(check.status == .@"error");
    try std.testing.expectEqualStrings("bad config", check.detail);
}

test "BgpStatusState.runtime_failed maps to error status" {
    var scratch: [64]u8 = undefined;
    const state = bgp_status.BgpStatusState{ .runtime_failed = .{ .message = "session dropped" } };
    const check = bgp_status.buildBgpCheckInto(state, &scratch);
    try std.testing.expect(check.status == .@"error");
    try std.testing.expectEqualStrings("session dropped", check.detail);
}

test "buildBgpCheckInto uses caller's buffer" {
    var detail_buf: [64]u8 = undefined;
    const state = bgp_status.BgpStatusState{
        .configured = .{
            .advertised_prefix_count = 5,
            .fsm_state = "open_sent",
            .peer_address = .{ 10, 0, 0, 2 },
            .peer_as = 65002,
            .local_as = 65001,
            .last_error = null,
            .messages_sent = 1,
            .messages_received = 0,
            .keepalives_sent = 0,
            .keepalives_received = 0,
            .passive_listener_state = .disabled,
            .passive_listener_error = null,
        },
    };
    const check = bgp_status.buildBgpCheckInto(state, &detail_buf);

    try std.testing.expect(check.status == .ok);
    try std.testing.expect(@intFromPtr(check.detail.ptr) == @intFromPtr(&detail_buf[0]));
}

test "deriveStatusStateFromBundle returns no_config for null" {
    const state = bgp_status.deriveStatusStateFromBundle(null);
    try std.testing.expect(state == .no_config);
}

test "BgpStatusState.failed renders as error status" {
    var scratch: [64]u8 = undefined;
    const check = bgp_status.buildBgpCheckInto(.{ .failed = .{ .message = "BGP load failed" } }, &scratch);
    try std.testing.expect(check.status == .@"error");
    try std.testing.expectEqualStrings("BGP load failed", check.detail);
}

test "BgpStatusState.failed message is preserved verbatim" {
    var scratch: [64]u8 = undefined;
    const check = bgp_status.buildBgpCheckInto(.{ .failed = .{ .message = "BGP connect failed" } }, &scratch);
    try std.testing.expect(check.status == .@"error");
    try std.testing.expectEqualStrings("BGP connect failed", check.detail);
}

test "BgpStatusState.failed is distinguishable from no_config" {
    var scratch: [64]u8 = undefined;

    const no_config_check = bgp_status.buildBgpCheckInto(.no_config, &scratch);
    try std.testing.expect(no_config_check.status == .warn);
    try std.testing.expectEqualStrings("BGP not configured", no_config_check.detail);

    const failed_check = bgp_status.buildBgpCheckInto(.{ .failed = .{ .message = "BGP load failed" } }, &scratch);
    try std.testing.expect(failed_check.status == .@"error");
    try std.testing.expectEqualStrings("BGP load failed", failed_check.detail);

    try std.testing.expect(no_config_check.status != failed_check.status);
}

test "BgpStatusState.failed is distinguishable from disabled" {
    var scratch: [64]u8 = undefined;

    const disabled_check = bgp_status.buildBgpCheckInto(.disabled, &scratch);
    try std.testing.expect(disabled_check.status == .ok);
    try std.testing.expectEqualStrings("BGP disabled by config", disabled_check.detail);

    const failed_check = bgp_status.buildBgpCheckInto(.{ .failed = .{ .message = "invalid local_address" } }, &scratch);
    try std.testing.expect(failed_check.status == .@"error");
    try std.testing.expect(failed_check.status != disabled_check.status);
}
