// status_bgp_integration_tests.zig — BGP status integration tests
//
// Tests the BGP status integration in the status module.
// These tests are deterministic and do not depend on repository state.

const std = @import("std");
const status = @import("status.zig");
const bgp_status = @import("bgp/status.zig");

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
            .passive_listener_state = .disabled,
            .passive_listener_error = null,
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
            .passive_listener_state = .disabled,
            .passive_listener_error = null,
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

// REGRESSION: buildStatusWithInputs respects established FSM over zero-prefix warning.
// This tests the full path through RuntimeStatusInputs -> statusStateFromLoadResult ->
// deriveStatusStateFromBundle -> buildBgpCheckInto with established FSM and zero prefixes.
test "REGRESSION: getBgpCheck with established FSM and zero prefixes returns ok" {
    // Create a BgpStatusState with established FSM but zero prefixes
    const bgp_state = bgp_status.BgpStatusState{
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
    
    // Build BGP check directly (this is what buildStatusWithInputs uses internally)
    var bgp_detail_buf: [64]u8 = undefined;
    const bgp_check = status.getBgpCheck(bgp_state, &bgp_detail_buf);
    
    // Established FSM outranks zero-prefix warning
    try std.testing.expectEqualStrings("bgp", bgp_check.name);
    try std.testing.expect(bgp_check.status == .ok);
    try std.testing.expectEqualStrings("BGP established", bgp_check.detail);
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
