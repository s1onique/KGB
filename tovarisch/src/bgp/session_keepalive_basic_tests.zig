// session_keepalive_basic_tests.zig — BGP KEEPALIVE scheduler basic tests
//
// ACT keepalive: Basic tests for established-state KEEPALIVE/hold timer lifecycle.
// Tests use FakeTransport + MockClock for deterministic timing.
//
// Test cases:
// 1. established session sends periodic keepalive
// 2. inbound message resets hold timer
// 3. missing inbound messages expires local hold timer

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

// === Test Case 1: established session sends periodic keepalive ===

test "established session sends periodic keepalive" {
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
    _ = try session.runOnce(&sess); // Send OPEN
    _ = try session.runOnce(&sess); // OPEN -> KEEPALIVE
    _ = try session.runOnce(&sess); // KEEPALIVE -> established

    try std.testing.expectEqual(session.SessionState.established, sess.status.state);

    // Initial keepalive counter after establishment
    const initial_keepalives = sess.status.keepalives_sent;

    // Advance time to keepalive interval (60 seconds = 60000 ms)
    session.MockClock.advance(60000);

    // Run once - should trigger KEEPALIVE transmission
    _ = try session.runOnce(&sess);

    // Verify KEEPALIVE was sent
    try std.testing.expectEqual(@as(u64, initial_keepalives + 1), sess.status.keepalives_sent);
}

test "second keepalive sent after another interval" {
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

    const initial_keepalives = sess.status.keepalives_sent;

    // Advance to first keepalive
    session.MockClock.advance(60000);
    _ = try session.runOnce(&sess);
    try std.testing.expectEqual(@as(u64, initial_keepalives + 1), sess.status.keepalives_sent);

    // Advance to second keepalive
    session.MockClock.advance(60000);
    _ = try session.runOnce(&sess);
    try std.testing.expectEqual(@as(u64, initial_keepalives + 2), sess.status.keepalives_sent);
}

test "keepalive interval is hold_time / 3 when smaller than configured" {
    // With hold_time=90, keepalive interval = 90/3 = 30 seconds = 30000 ms
    session.MockClock.reset();

    const peer_open = buildPeerOpen(65002, 90, .{ 10, 0, 0, 2 });
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
        .hold_time_seconds = 90,
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

    // Keepalive interval should be 90/3 = 30 seconds = 30000 ms
    try std.testing.expectEqual(@as(u16, 90), sess.negotiated_hold_time);

    const initial_keepalives = sess.status.keepalives_sent;

    // Advance 30 seconds (30000 ms) - should trigger keepalive
    session.MockClock.advance(30000);
    _ = try session.runOnce(&sess);
    try std.testing.expectEqual(@as(u64, initial_keepalives + 1), sess.status.keepalives_sent);
}

// === Test Case 2: inbound message resets hold timer ===

test "inbound KEEPALIVE resets hold timer" {
    session.MockClock.reset();

    const peer_open = buildPeerOpen(65002, 180, .{ 10, 0, 0, 2 });
    const peer_keepalive = buildPeerKeepalive();

    const responses = &.{
        session.PeerResponse{ .recv_bytes = &peer_open },
        session.PeerResponse{ .recv_bytes = &peer_keepalive },
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
    _ = try session.runOnce(&sess); // OPEN
    _ = try session.runOnce(&sess); // KEEPALIVE
    _ = try session.runOnce(&sess); // established

    // Advance near hold timeout (170 seconds)
    session.MockClock.advance(170000);
    _ = try session.runOnce(&sess);

    // Session should still be established (hold timer not expired)
    try std.testing.expectEqual(session.SessionState.established, sess.status.state);

    // Inject inbound KEEPALIVE to reset hold timer
    _ = try session.runOnce(&sess);

    // Advance less than full hold interval
    session.MockClock.advance(170000);
    _ = try session.runOnce(&sess);

    // Session should still be established
    try std.testing.expectEqual(session.SessionState.established, sess.status.state);
}

test "inbound UPDATE also resets hold timer" {
    session.MockClock.reset();

    const peer_open = buildPeerOpen(65002, 180, .{ 10, 0, 0, 2 });
    const peer_keepalive = buildPeerKeepalive();

    // Build peer UPDATE (minimal)
    var peer_update: [23]u8 = undefined;
    @memset(peer_update[0..16], 0xFF);
    peer_update[16] = 0;
    peer_update[17] = 23;
    peer_update[18] = 2; // UPDATE
    peer_update[19] = 0;
    peer_update[20] = 0;
    peer_update[21] = 0;
    peer_update[22] = 0;

    const responses = &.{
        session.PeerResponse{ .recv_bytes = &peer_open },
        session.PeerResponse{ .recv_bytes = &peer_keepalive },
        session.PeerResponse{ .recv_bytes = &peer_update },
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

    // Advance near hold timeout
    session.MockClock.advance(170000);
    _ = try session.runOnce(&sess);
    try std.testing.expectEqual(session.SessionState.established, sess.status.state);

    // Receive UPDATE which should reset hold timer
    _ = try session.runOnce(&sess);

    // Advance less than full hold interval
    session.MockClock.advance(170000);
    _ = try session.runOnce(&sess);

    // Session should still be established
    try std.testing.expectEqual(session.SessionState.established, sess.status.state);
}

// === Test Case 3: missing inbound messages expires local hold timer ===

test "missing inbound messages expires local hold timer" {
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

    // Advance beyond hold time (180 seconds = 180000 ms)
    session.MockClock.advance(180001);
    _ = try session.runOnce(&sess);

    // Session should be failed with hold timer error
    try std.testing.expectEqual(session.SessionState.failed, sess.status.state);
    try std.testing.expect(sess.status.last_error != null);
    try std.testing.expect(std.mem.containsAtLeast(u8, sess.status.last_error.?.message, 1, "hold timer"));
}

test "hold timer with negotiated hold time of 90 seconds" {
    session.MockClock.reset();

    const peer_open = buildPeerOpen(65002, 90, .{ 10, 0, 0, 2 });
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

    // negotiated_hold_time = min(180, 90) = 90
    try std.testing.expectEqual(@as(u16, 90), sess.negotiated_hold_time);

    // Advance just below hold time - should stay established
    session.MockClock.advance(89000);
    _ = try session.runOnce(&sess);
    try std.testing.expectEqual(session.SessionState.established, sess.status.state);

    // Advance past hold time - should fail
    session.MockClock.advance(2000);
    _ = try session.runOnce(&sess);
    try std.testing.expectEqual(session.SessionState.failed, sess.status.state);
}
