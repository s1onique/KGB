// session_keepalive_notification_tests.zig — BGP NOTIFICATION and zero-hold tests
//
// ACT keepalive: Tests for NOTIFICATION decoding and zero hold time behavior.
// Tests use FakeTransport + MockClock for deterministic timing.
//
// Test cases:
// 4. peer notification is decoded
// 5. zero hold time disables keepalive scheduler
// 6. status established zero prefixes remains ok

const std = @import("std");
const session = @import("session.zig");
const types = @import("types.zig");

// Helper to build a peer OPEN message with specified hold time.
fn buildPeerOpen(peer_as: u16, hold_time: u16, router_id: [4]u8) [29]u8 {
    var buf: [29]u8 = undefined;
    @memset(buf[0..16], 0xFF);
    buf[16] = 0;
    buf[17] = 29;
    buf[18] = 1; // OPEN
    buf[19] = 4; // version
    buf[20] = @as(u8, @intCast(peer_as / 256));
    buf[21] = @as(u8, @intCast(peer_as % 256));
    buf[22] = @as(u8, @intCast(hold_time / 256));
    buf[23] = @as(u8, @intCast(hold_time % 256));
    buf[24] = router_id[0];
    buf[25] = router_id[1];
    buf[26] = router_id[2];
    buf[27] = router_id[3];
    buf[28] = 0; // opt params
    return buf;
}

/// Helper to build a peer KEEPALIVE message.
fn buildPeerKeepalive() [19]u8 {
    var buf: [19]u8 = undefined;
    @memset(buf[0..16], 0xFF);
    buf[16] = 0;
    buf[17] = 19;
    buf[18] = 4; // KEEPALIVE
    return buf;
}

// === Test Case 4: peer notification is decoded ===

test "peer NOTIFICATION is decoded with human-readable detail" {
    session.MockClock.reset();

    // Build peer NOTIFICATION for Hold Timer Expired (code 4, subcode 0)
    var peer_notif: [21]u8 = undefined;
    @memset(peer_notif[0..16], 0xFF);
    peer_notif[16] = 0;
    peer_notif[17] = 21;
    peer_notif[18] = 3; // NOTIFICATION
    peer_notif[19] = 4; // Hold Timer Expired
    peer_notif[20] = 0; // no subcode

    const peer_open = buildPeerOpen(65002, 180, .{ 10, 0, 0, 2 });
    const peer_keepalive = buildPeerKeepalive();

    const responses = &.{
        session.PeerResponse{ .recv_bytes = &peer_open },
        session.PeerResponse{ .recv_bytes = &peer_keepalive },
        session.PeerResponse{ .recv_bytes = &peer_notif },
    };

    var fake = try session.FakeTransport.init(std.testing.allocator, responses);
    defer fake.deinit();
    const trans = fake.toTransport();

    const config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = null,
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{types.Ipv4Prefix.init("10.0.0.0/8")},
        .same_as = true,
    };

    var sess = try session.initWithClock(config, &trans, session.MockClock.interface());

    // Complete handshake
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);

    // Receive peer NOTIFICATION
    _ = try session.runOnce(&sess);

    // Verify NOTIFICATION code and subcode are stored
    try std.testing.expectEqual(@as(u8, 4), sess.status.last_notification_code);
    try std.testing.expectEqual(@as(u8, 0), sess.status.last_notification_subcode);

    // Verify error message includes human-readable detail
    try std.testing.expect(sess.status.last_error != null);
    try std.testing.expect(std.mem.containsAtLeast(u8, sess.status.last_error.?.message, 1, "Hold Timer Expired"));
}

test "peer Cease NOTIFICATION is decoded" {
    session.MockClock.reset();

    // Build peer Cease NOTIFICATION (code 6, subcode 0)
    var peer_notif: [21]u8 = undefined;
    @memset(peer_notif[0..16], 0xFF);
    peer_notif[16] = 0;
    peer_notif[17] = 21;
    peer_notif[18] = 3; // NOTIFICATION
    peer_notif[19] = 6; // Cease
    peer_notif[20] = 0; // no subcode

    const peer_open = buildPeerOpen(65002, 180, .{ 10, 0, 0, 2 });
    const peer_keepalive = buildPeerKeepalive();

    const responses = &.{
        session.PeerResponse{ .recv_bytes = &peer_open },
        session.PeerResponse{ .recv_bytes = &peer_keepalive },
        session.PeerResponse{ .recv_bytes = &peer_notif },
    };

    var fake = try session.FakeTransport.init(std.testing.allocator, responses);
    defer fake.deinit();
    const trans = fake.toTransport();

    const config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = null,
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{types.Ipv4Prefix.init("10.0.0.0/8")},
        .same_as = true,
    };

    var sess = try session.initWithClock(config, &trans, session.MockClock.interface());

    // Complete handshake
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);

    // Receive peer NOTIFICATION
    _ = try session.runOnce(&sess);

    // Verify NOTIFICATION is decoded
    try std.testing.expectEqual(session.SessionState.failed, sess.status.state);
    try std.testing.expectEqual(@as(u8, 6), sess.status.last_notification_code);
    try std.testing.expect(sess.status.last_error != null);
    try std.testing.expect(std.mem.containsAtLeast(u8, sess.status.last_error.?.message, 1, "Cease"));
}

// === Test Case 5: zero hold time disables keepalive scheduler ===

test "zero hold time disables keepalive scheduler" {
    session.MockClock.reset();

    const peer_open = buildPeerOpen(65002, 0, .{ 10, 0, 0, 2 });
    const peer_keepalive = buildPeerKeepalive();

    const responses = &.{
        session.PeerResponse{ .recv_bytes = &peer_open },
        session.PeerResponse{ .recv_bytes = &peer_keepalive },
    };

    var fake = try session.FakeTransport.init(std.testing.allocator, responses);
    defer fake.deinit();
    const trans = fake.toTransport();

    const config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = .{ 127, 0, 0, 1 },
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 0,
        .keepalive_seconds = 0,
        .connect_timeout_ms = 5000,
        .prefixes = &.{types.Ipv4Prefix.init("10.0.0.0/8")},
        .same_as = true,
    };

    var sess = try session.initWithClock(config, &trans, session.MockClock.interface());

    // Complete handshake
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);

    // negotiated_hold_time should be 0
    try std.testing.expectEqual(@as(u16, 0), sess.negotiated_hold_time);

    // keepalive_interval should be 0 (disabled)
    try std.testing.expectEqual(@as(u32, 0), sess.keepalive_interval_ms);

    // Initial keepalive counter
    const initial_keepalives = sess.status.keepalives_sent;

    // Advance a large amount of time
    session.MockClock.advance(1000000); // 1000 seconds

    // Run once - should NOT send KEEPALIVE
    _ = try session.runOnce(&sess);

    // No KEEPALIVE should be sent
    try std.testing.expectEqual(initial_keepalives, sess.status.keepalives_sent);

    // Session should still be established
    try std.testing.expectEqual(session.SessionState.established, sess.status.state);
}

test "zero hold time from peer disables keepalive" {
    session.MockClock.reset();

    const peer_open = buildPeerOpen(65002, 0, .{ 10, 0, 0, 2 });
    const peer_keepalive = buildPeerKeepalive();

    const responses = &.{
        session.PeerResponse{ .recv_bytes = &peer_open },
        session.PeerResponse{ .recv_bytes = &peer_keepalive },
    };

    var fake = try session.FakeTransport.init(std.testing.allocator, responses);
    defer fake.deinit();
    const trans = fake.toTransport();

    const config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = .{ 127, 0, 0, 1 },
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{types.Ipv4Prefix.init("10.0.0.0/8")},
        .same_as = true,
    };

    var sess = try session.initWithClock(config, &trans, session.MockClock.interface());

    // Complete handshake
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);

    // negotiated_hold_time = min(180, 0) = 0
    try std.testing.expectEqual(@as(u16, 0), sess.negotiated_hold_time);
    try std.testing.expectEqual(@as(u32, 0), sess.keepalive_interval_ms);

    // Session should still be established (no hold timer behavior)
    try std.testing.expectEqual(session.SessionState.established, sess.status.state);
}

// === Test Case 6: status established zero prefixes remains ok ===

test "status established zero prefixes remains ok" {
    session.MockClock.reset();

    const peer_open = buildPeerOpen(65002, 180, .{ 10, 0, 0, 2 });
    const peer_keepalive = buildPeerKeepalive();

    const responses = &.{
        session.PeerResponse{ .recv_bytes = &peer_open },
        session.PeerResponse{ .recv_bytes = &peer_keepalive },
    };

    var fake = try session.FakeTransport.init(std.testing.allocator, responses);
    defer fake.deinit();
    const trans = fake.toTransport();

    const config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = null,
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{}, // Zero prefixes
        .same_as = true,
    };

    var sess = try session.initWithClock(config, &trans, session.MockClock.interface());

    // Complete handshake with zero prefixes
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);
    const result = try session.runOnce(&sess);

    try std.testing.expectEqual(session.RunResult.established, result);
    try std.testing.expectEqual(session.SessionState.established, sess.status.state);
    try std.testing.expectEqual(@as(usize, 0), sess.status.advertised_prefix_count);

    // Session should remain established even with zero prefixes
    _ = try session.runOnce(&sess);
    try std.testing.expectEqual(session.SessionState.established, sess.status.state);
}

// === Additional tests ===

test "keepalive counter increments correctly" {
    session.MockClock.reset();

    const peer_open = buildPeerOpen(65002, 180, .{ 10, 0, 0, 2 });
    const peer_keepalive = buildPeerKeepalive();

    const responses = &.{
        session.PeerResponse{ .recv_bytes = &peer_open },
        session.PeerResponse{ .recv_bytes = &peer_keepalive },
    };

    var fake = try session.FakeTransport.init(std.testing.allocator, responses);
    defer fake.deinit();
    const trans = fake.toTransport();

    const config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = .{ 127, 0, 0, 1 },
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{types.Ipv4Prefix.init("10.0.0.0/8")},
        .same_as = true,
    };

    var sess = try session.initWithClock(config, &trans, session.MockClock.interface());

    // Complete handshake
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);

    // Initial counter should be 1 (sent during handshake)
    try std.testing.expectEqual(@as(u64, 1), sess.status.keepalives_sent);

    // Advance and send periodic keepalives
    session.MockClock.advance(60000);
    _ = try session.runOnce(&sess);
    try std.testing.expectEqual(@as(u64, 2), sess.status.keepalives_sent);

    session.MockClock.advance(60000);
    _ = try session.runOnce(&sess);
    try std.testing.expectEqual(@as(u64, 3), sess.status.keepalives_sent);
}
