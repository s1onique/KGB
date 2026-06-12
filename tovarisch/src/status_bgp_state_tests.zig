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

// REGRESSION: statusStateFromLoadResult preserves BgpLoadResult tag.
// This verifies the fix for the collapse bug where .failed, .not_configured,
// .disabled, and .no_config were all collapsed to null bundle pointer.

test "REGRESSION: statusStateFromLoadResult(.no_config) returns no_config" {
    const result = bgp_status.statusStateFromLoadResult(.{ .no_config = {} });
    try std.testing.expect(result == .no_config);
}

test "REGRESSION: statusStateFromLoadResult(.not_configured) returns not_configured" {
    const result = bgp_status.statusStateFromLoadResult(.{ .not_configured = {} });
    try std.testing.expect(result == .not_configured);
}

test "REGRESSION: statusStateFromLoadResult(.disabled) returns disabled" {
    const result = bgp_status.statusStateFromLoadResult(.{ .disabled = {} });
    try std.testing.expect(result == .disabled);
}

test "REGRESSION: statusStateFromLoadResult(.failed) preserves message" {
    const bgp_serve = @import("bgp/serve_integration.zig");
    const load_failure = bgp_serve.LoadFailure{ .message = "missing peer_address" };
    const result = bgp_status.statusStateFromLoadResult(.{ .failed = load_failure });
    try std.testing.expect(result == .failed);
    try std.testing.expectEqualStrings("missing peer_address", result.failed.message);
}

test "REGRESSION: .failed renders as error status with load error detail" {
    var scratch: [64]u8 = undefined;
    const bgp_serve = @import("bgp/serve_integration.zig");
    const load_failure = bgp_serve.LoadFailure{ .message = "connection refused" };
    const state = bgp_status.statusStateFromLoadResult(.{ .failed = load_failure });
    const check = bgp_status.buildBgpCheckInto(state, &scratch);
    try std.testing.expect(check.status == .@"error");
    try std.testing.expectEqualStrings("connection refused", check.detail);
}

test "REGRESSION: .no_config and .not_configured both warn but are distinct tags" {
    var scratch: [64]u8 = undefined;
    const no_config_state = bgp_status.statusStateFromLoadResult(.{ .no_config = {} });
    const no_config_check = bgp_status.buildBgpCheckInto(no_config_state, &scratch);
    try std.testing.expect(no_config_check.status == .warn);
    try std.testing.expectEqualStrings("BGP not configured", no_config_check.detail);
    const not_configured_state = bgp_status.statusStateFromLoadResult(.{ .not_configured = {} });
    const not_configured_check = bgp_status.buildBgpCheckInto(not_configured_state, &scratch);
    try std.testing.expect(not_configured_check.status == .warn);
    try std.testing.expectEqualStrings("BGP not configured", not_configured_check.detail);
    // Both warn but render from different union tags
    try std.testing.expect(@as(u32, @intFromEnum(@as(bgp_status.BgpStatusState, no_config_state))) != @as(u32, @intFromEnum(@as(bgp_status.BgpStatusState, not_configured_state))));
}

// REGRESSION: Established FSM outranks zero-prefix warning.
// This verifies the fix for the regression where an established BGP session
// with zero prefixes was incorrectly reported as warn instead of ok.
test "REGRESSION: BgpStatusState.configured with established FSM and zero prefixes is ok" {
    var scratch: [64]u8 = undefined;
    const state = bgp_status.BgpStatusState{
        .configured = .{
            .advertised_prefix_count = 0,
            .fsm_state = "established",
            .peer_address = .{ 10, 0, 0, 2 },
            .peer_as = 65002,
            .local_as = 65001,
            .last_error = null,
            .messages_sent = 100,
            .messages_received = 98,
            .keepalives_sent = 50,
            .keepalives_received = 50,
            .passive_listener_state = .disabled,
            .passive_listener_error = null,
        },
    };
    const check = bgp_status.buildBgpCheckInto(state, &scratch);
    try std.testing.expect(check.status == .ok);
    try std.testing.expectEqualStrings("BGP established", check.detail);
}

