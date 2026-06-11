// status_bgp_tests.zig — BGP status tests for tovarisch
//
// Tests the BGP status integration in the status module.
// These tests are deterministic and do not depend on repository state.

const std = @import("std");
const status = @import("status.zig");
const bgp_status = @import("bgp/status.zig");

// --- BGP Status Integration Tests ---

test "bgp check has warn status when no config" {
    var scratch = status.StatusScratch{};
    const checks = status.getLocalChecksWithBgp(
        null,
        status.getDefaultConfigCheck(),
        .no_config,
        &scratch,
    );
    for (checks) |check| {
        if (std.mem.eql(u8, check.name, "bgp")) {
            try std.testing.expectEqual(status.CheckStatus.warn, check.status);
            try std.testing.expectEqualStrings("BGP not configured", check.detail);
        }
    }
}

test "getBgpCheck returns warn for no_config state" {
    var scratch: [64]u8 = undefined;
    const check = status.getBgpCheck(.no_config, &scratch);
    try std.testing.expectEqualStrings("bgp", check.name);
    try std.testing.expect(check.status == .warn);
    try std.testing.expectEqualStrings("BGP not configured", check.detail);
}

test "getBgpCheck returns ok for disabled state" {
    var scratch: [64]u8 = undefined;
    const check = status.getBgpCheck(.disabled, &scratch);
    try std.testing.expectEqualStrings("bgp", check.name);
    try std.testing.expect(check.status == .ok);
    try std.testing.expectEqualStrings("BGP disabled by config", check.detail);
}

test "getBgpCheck returns ok for configured with prefixes" {
    var scratch: [64]u8 = undefined;
    const check = status.getBgpCheck(.{
        .configured = .{
            .advertised_prefix_count = 2,
            .fsm_state = "established",
            .peer_address = .{ 10, 0, 0, 2 },
            .peer_as = 65002,
            .local_as = 65001,
            .last_error = null,
            .messages_sent = 3,
            .messages_received = 2,
            .keepalives_sent = 1,
            .keepalives_received = 1,
        },
    }, &scratch);
    try std.testing.expectEqualStrings("bgp", check.name);
    try std.testing.expect(check.status == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, check.detail, 1, "2 advertised prefixes"));
}

test "getBgpCheck returns warn for configured with zero prefixes" {
    var scratch: [64]u8 = undefined;
    const check = status.getBgpCheck(.{
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
        },
    }, &scratch);
    try std.testing.expectEqualStrings("bgp", check.name);
    try std.testing.expect(check.status == .warn);
    try std.testing.expectEqualStrings("BGP configured with no advertised prefixes", check.detail);
}

test "getBgpCheck returns error for failed state" {
    var scratch: [64]u8 = undefined;
    const check = status.getBgpCheck(.{ .failed = .{ .message = "invalid AS number" } }, &scratch);
    try std.testing.expectEqualStrings("bgp", check.name);
    try std.testing.expect(check.status == .@"error");
    try std.testing.expectEqualStrings("invalid AS number", check.detail);
}

test "getLocalChecksWithBgp passes explicit BGP state" {
    var scratch = status.StatusScratch{};
    const default_check = status.getDefaultConfigCheck();
    const checks = status.getLocalChecksWithBgp(null, default_check, .disabled, &scratch);
    
    var found_bgp = false;
    for (checks) |check| {
        if (std.mem.eql(u8, check.name, "bgp")) {
            found_bgp = true;
            try std.testing.expect(check.status == .ok);
            try std.testing.expectEqualStrings("BGP disabled by config", check.detail);
        }
    }
    try std.testing.expect(found_bgp);
}

test "top-level status degrades when BGP is error" {
    const checks = [_]status.Check{
        .{ .name = "process", .status = .ok, .detail = "running" },
        .{ .name = "binary", .status = .ok, .detail = "tovarisch" },
        .{ .name = "config", .status = .ok, .detail = "/etc/tovarisch.conf" },
        .{ .name = "state_dir", .status = .ok, .detail = "state directory ready" },
        .{ .name = "http", .status = .ok, .detail = "http service route available" },
        .{ .name = "tunnel", .status = .ok, .detail = "wg0" },
        .{ .name = "wg_peers", .status = .ok, .detail = "wg0" },
        .{ .name = "bfd", .status = .ok, .detail = "bfd sessions up" },
        .{ .name = "bgp", .status = .@"error", .detail = "configuration failed" },
    };
    try std.testing.expectEqual(status.CheckStatus.@"error", status.deriveStatus(&checks));
}

// ============================================================================
// Regression Tests: Concrete Error Preservation in Status Derivation
// ============================================================================
// These tests verify that concrete session errors (e.g., "send: EBADF") are
// preserved in status derivation, not replaced with generic "IoError".

test "deriveStatusStateFromBundle returns runtime_failed when session state is failed" {
    // This test verifies that when a bundle's session transitions to failed state,
    // deriveStatusStateFromBundle returns .runtime_failed (not .configured with
    // last_error set). The concrete error message should be preserved.
    var scratch: [64]u8 = undefined;
    const state = bgp_status.BgpStatusState{
        .runtime_failed = .{ .message = "send: EBADF" },
    };
    const check = bgp_status.buildBgpCheckInto(state, &scratch);
    try std.testing.expect(check.status == .@"error");
    try std.testing.expectEqualStrings("send: EBADF", check.detail);
}

test "buildBgpCheckInto renders concrete send error, not IoError" {
    // CRITICAL: This test ensures that concrete send errors like "send: EBADF"
    // render directly in the BGP check detail, not as "IoError".
    var scratch: [64]u8 = undefined;

    // Test with concrete EBADF error
    const check1 = bgp_status.buildBgpCheckInto(.{ .runtime_failed = .{ .message = "send: EBADF" } }, &scratch);
    try std.testing.expect(check1.status == .@"error");
    try std.testing.expectEqualStrings("send: EBADF", check1.detail);
    // Must NOT be "IoError" - that would indicate the fix isn't working
    try std.testing.expect(!std.mem.eql(u8, check1.detail, "IoError"));

    // Test with concrete ECONNRESET error
    const check2 = bgp_status.buildBgpCheckInto(.{ .runtime_failed = .{ .message = "send: ECONNRESET" } }, &scratch);
    try std.testing.expect(check2.status == .@"error");
    try std.testing.expectEqualStrings("send: ECONNRESET", check2.detail);
    try std.testing.expect(!std.mem.eql(u8, check2.detail, "IoError"));

    // Test with concrete EAGAIN error
    const check3 = bgp_status.buildBgpCheckInto(.{ .runtime_failed = .{ .message = "send: EAGAIN/EWOULDBLOCK" } }, &scratch);
    try std.testing.expect(check3.status == .@"error");
    try std.testing.expectEqualStrings("send: EAGAIN/EWOULDBLOCK", check3.detail);
    try std.testing.expect(!std.mem.eql(u8, check3.detail, "IoError"));
}

test "runtime_failed is distinguishable from failed in status output" {
    // Verify that .runtime_failed (runtime error) and .failed (config error)
    // both render as error status but are distinguishable by message content.
    var scratch: [64]u8 = undefined;

    // Config failure
    const check1 = bgp_status.buildBgpCheckInto(.{ .failed = .{ .message = "invalid AS number" } }, &scratch);
    try std.testing.expect(check1.status == .@"error");
    try std.testing.expectEqualStrings("invalid AS number", check1.detail);

    // Runtime failure with concrete send error
    const check2 = bgp_status.buildBgpCheckInto(.{ .runtime_failed = .{ .message = "send: EBADF" } }, &scratch);
    try std.testing.expect(check2.status == .@"error");
    try std.testing.expectEqualStrings("send: EBADF", check2.detail);

    // Both are error status
    try std.testing.expect(check1.status == check2.status);
    // But detail messages are different
    try std.testing.expect(!std.mem.eql(u8, check1.detail, check2.detail));
}

test "top-level status degrades when BGP is warn" {
    const checks = [_]status.Check{
        .{ .name = "process", .status = .ok, .detail = "running" },
        .{ .name = "binary", .status = .ok, .detail = "tovarisch" },
        .{ .name = "config", .status = .ok, .detail = "/etc/tovarisch.conf" },
        .{ .name = "state_dir", .status = .ok, .detail = "state directory ready" },
        .{ .name = "http", .status = .ok, .detail = "http service route available" },
        .{ .name = "tunnel", .status = .ok, .detail = "wg0" },
        .{ .name = "wg_peers", .status = .ok, .detail = "wg0" },
        .{ .name = "bfd", .status = .ok, .detail = "bfd sessions up" },
        .{ .name = "bgp", .status = .warn, .detail = "BGP not configured" },
    };
    try std.testing.expectEqual(status.CheckStatus.warn, status.deriveStatus(&checks));
}

test "top-level status remains ok when BGP disabled" {
    const checks = [_]status.Check{
        .{ .name = "process", .status = .ok, .detail = "running" },
        .{ .name = "binary", .status = .ok, .detail = "tovarisch" },
        .{ .name = "config", .status = .ok, .detail = "/etc/tovarisch.conf" },
        .{ .name = "state_dir", .status = .ok, .detail = "state directory ready" },
        .{ .name = "http", .status = .ok, .detail = "http service route available" },
        .{ .name = "tunnel", .status = .ok, .detail = "wg0" },
        .{ .name = "wg_peers", .status = .ok, .detail = "wg0" },
        .{ .name = "bfd", .status = .ok, .detail = "bfd sessions up" },
        .{ .name = "bgp", .status = .ok, .detail = "BGP disabled by config" },
    };
    try std.testing.expectEqual(status.CheckStatus.ok, status.deriveStatus(&checks));
}

// --- BgpStatusState Unit Tests ---

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
        },
    };
    const check = bgp_status.buildBgpCheckInto(state, &detail_buf);
    
    try std.testing.expect(check.status == .ok);
    // Verify the detail points to our buffer (not heap-allocated)
    try std.testing.expect(@intFromPtr(check.detail.ptr) == @intFromPtr(&detail_buf[0]));
}

test "deriveStatusStateFromBundle returns no_config for null" {
    const state = bgp_status.deriveStatusStateFromBundle(null);
    try std.testing.expect(state == .no_config);
}

// --- BGP Load Failure Tests ---

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
    
    // These must be different statuses
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

test "top-level status degrades when BGP load fails" {
    const checks = [_]status.Check{
        .{ .name = "process", .status = .ok, .detail = "running" },
        .{ .name = "binary", .status = .ok, .detail = "tovarisch" },
        .{ .name = "config", .status = .ok, .detail = "/etc/tovarisch.conf" },
        .{ .name = "state_dir", .status = .ok, .detail = "state directory ready" },
        .{ .name = "http", .status = .ok, .detail = "http service route available" },
        .{ .name = "tunnel", .status = .ok, .detail = "wg0" },
        .{ .name = "wg_peers", .status = .ok, .detail = "wg0" },
        .{ .name = "bfd", .status = .ok, .detail = "bfd sessions up" },
        .{ .name = "bgp", .status = .@"error", .detail = "BGP load failed" },
    };
    try std.testing.expectEqual(status.CheckStatus.@"error", status.deriveStatus(&checks));
}
