// status_bgp_tests.zig — BGP status tests for tovarisch
//
// Tests the BGP status integration in the status module.
// These tests are deterministic and do not depend on repository state.

const std = @import("std");
const status = @import("status.zig");
const bgp_status = @import("bgp/status.zig");

// --- BGP Status Integration Tests ---

test "bgp check has warn status when no config" {
    const checks = status.getLocalChecks();
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
    const check = status.getBgpCheck(.{ .configured = .{ .advertised_prefix_count = 2 } }, &scratch);
    try std.testing.expectEqualStrings("bgp", check.name);
    try std.testing.expect(check.status == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, check.detail, 1, "2 advertised prefixes"));
}

test "getBgpCheck returns warn for configured with zero prefixes" {
    var scratch: [64]u8 = undefined;
    const check = status.getBgpCheck(.{ .configured = .{ .advertised_prefix_count = 0 } }, &scratch);
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
    const state = bgp_status.BgpStatusState{ .configured = .{ .advertised_prefix_count = 3 } };
    const check = bgp_status.buildBgpCheckInto(state, &scratch);
    try std.testing.expect(check.status == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, check.detail, 1, "3 advertised prefixes"));
}

test "BgpStatusState.configured with zero prefixes maps to warn status" {
    var scratch: [64]u8 = undefined;
    const state = bgp_status.BgpStatusState{ .configured = .{ .advertised_prefix_count = 0 } };
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
    const state = bgp_status.BgpStatusState{ .configured = .{ .advertised_prefix_count = 5 } };
    const check = bgp_status.buildBgpCheckInto(state, &detail_buf);
    
    try std.testing.expect(check.status == .ok);
    // Verify the detail points to our buffer (not heap-allocated)
    try std.testing.expect(@intFromPtr(check.detail.ptr) == @intFromPtr(&detail_buf[0]));
}

test "deriveStatusStateFromBundle returns no_config for null" {
    const state = bgp_status.deriveStatusStateFromBundle(null);
    try std.testing.expect(state == .no_config);
}
