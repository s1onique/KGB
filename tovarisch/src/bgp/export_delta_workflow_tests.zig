// bgp/export_delta_workflow_tests.zig — End-to-end delta workflow tests
//
// ACT: Apply BGP export deltas after watched prefix reload (Phase 2)
//
// End-to-end tests combining delta computation with session application.

const std = @import("std");
const types = @import("types.zig");
const session = @import("session.zig");
const session_delta = @import("session_delta.zig");
const export_delta = @import("export_delta.zig");

// Helper to build a peer OPEN message
fn buildPeerOpen(peer_as: u16, router_id: [4]u8) [29]u8 {
    var peer_open: [29]u8 = undefined;
    @memset(peer_open[0..16], 0xFF);
    peer_open[16] = 0;
    peer_open[17] = 29;
    peer_open[18] = 1;
    peer_open[19] = 4;
    peer_open[20] = @as(u8, @intCast(peer_as / 256));
    peer_open[21] = @as(u8, @intCast(peer_as % 256));
    peer_open[22] = 0;
    peer_open[23] = 180;
    peer_open[24] = router_id[0];
    peer_open[25] = router_id[1];
    peer_open[26] = router_id[2];
    peer_open[27] = router_id[3];
    peer_open[28] = 0;
    return peer_open;
}

// Helper to build a peer KEEPALIVE message
fn buildPeerKeepalive() [19]u8 {
    var peer_keepalive: [19]u8 = undefined;
    @memset(peer_keepalive[0..16], 0xFF);
    peer_keepalive[16] = 0;
    peer_keepalive[17] = 19;
    peer_keepalive[18] = 4;
    return peer_keepalive;
}

// ============================================================================
// End-to-End Workflow Tests
// ============================================================================

test "computeDelta + applyDelta workflow" {
    session.MockClock.reset();
    const allocator = std.testing.allocator;

    // Current exported prefixes
    const current = &.{
        types.Ipv4Prefix.init("10.0.0.0/8"),
        types.Ipv4Prefix.init("192.168.0.0/16"),
    };

    // New candidate prefixes
    const candidate = &.{
        types.Ipv4Prefix.init("10.0.0.0/8"),
        types.Ipv4Prefix.init("172.16.0.0/12"),
    };

    // Compute delta
    const delta = try export_delta.computeDelta(allocator, current, candidate);
    defer {
        allocator.free(delta.added);
        allocator.free(delta.removed);
    }

    // Verify delta
    try std.testing.expect(delta.added.len == 1);
    try std.testing.expect(delta.removed.len == 1);
    try std.testing.expect(delta.unchanged_count == 1);

    // Create session and apply delta
    const local_as: u16 = 65001;
    const peer_as: u16 = 65002;

    var fake = try session.FakeTransport.init(allocator, &.{
        session.PeerResponse{ .recv_bytes = &buildPeerOpen(peer_as, .{ 10, 0, 0, 2 }) },
        session.PeerResponse{ .recv_bytes = &buildPeerKeepalive() },
    });
    defer fake.deinit();
    const trans = fake.toTransport();

    const config = session.SessionConfig{
        .peer_address = .{ 10, 0, 0, 2 },
        .peer_port = 179,
        .local_address = .{ 10, 0, 0, 1 },
        .local_as = local_as,
        .peer_as = peer_as,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{},
        .same_as = false,
    };
    var sess = try session.initWithClock(config, &trans, session.MockClock.interface());

    // Transition to established
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);

    // Apply the computed delta
    const result = try session_delta.applyDelta(&sess, delta.removed, delta.added);

    // Verify results
    try std.testing.expect(result.withdrawals_sent == 1);
    try std.testing.expect(result.announcements_sent == 1);
    try std.testing.expect(result.withdrawn_prefixes == 1);
    try std.testing.expect(result.announced_prefixes == 1);
}

test "identical prefix sets produce no updates" {
    session.MockClock.reset();
    const allocator = std.testing.allocator;

    // Same current and candidate
    const prefixes = &.{
        types.Ipv4Prefix.init("10.0.0.0/8"),
        types.Ipv4Prefix.init("172.16.0.0/12"),
        types.Ipv4Prefix.init("192.168.0.0/16"),
    };

    // Compute delta
    const delta = try export_delta.computeDelta(allocator, prefixes, prefixes);
    defer {
        allocator.free(delta.added);
        allocator.free(delta.removed);
    }

    // Verify delta is empty
    try std.testing.expect(delta.added.len == 0);
    try std.testing.expect(delta.removed.len == 0);
    try std.testing.expect(delta.unchanged_count == 3);

    // Create session
    const local_as: u16 = 65001;
    const peer_as: u16 = 65002;

    var fake = try session.FakeTransport.init(allocator, &.{
        session.PeerResponse{ .recv_bytes = &buildPeerOpen(peer_as, .{ 10, 0, 0, 2 }) },
        session.PeerResponse{ .recv_bytes = &buildPeerKeepalive() },
    });
    defer fake.deinit();
    const trans = fake.toTransport();

    const config = session.SessionConfig{
        .peer_address = .{ 10, 0, 0, 2 },
        .peer_port = 179,
        .local_address = .{ 10, 0, 0, 1 },
        .local_as = local_as,
        .peer_as = peer_as,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{},
        .same_as = false,
    };
    var sess = try session.initWithClock(config, &trans, session.MockClock.interface());

    // Transition to established
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);

    // Apply empty delta
    const result = try session_delta.applyDelta(&sess, delta.removed, delta.added);

    // Verify no UPDATEs sent
    try std.testing.expect(result.withdrawals_sent == 0);
    try std.testing.expect(result.announcements_sent == 0);
}
