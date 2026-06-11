// session_keepalive_advanced_tests.zig — BGP KEEPALIVE advanced tests
//
// Tests for session ownership, wire shape, and edge cases.
// Tests use FakeTransport + MockClock for deterministic timing.

const std = @import("std");
const session = @import("session.zig");
const types = @import("types.zig");
const message = @import("message.zig");

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

test "each session owns its own keepalive state" {
    // Verify that each session has its own pending_keepalive and interval.
    // This prevents cross-session contamination of timer state.
    session.MockClock.reset();

    const peer_open = buildPeerOpen(65002, 180, .{ 10, 0, 0, 2 });
    const peer_keepalive = buildPeerKeepalive();

    const responses1 = &.{
        session.PeerResponse{ .recv_bytes = &peer_open },
        session.PeerResponse{ .recv_bytes = &peer_keepalive },
    };
    const responses2 = &.{
        session.PeerResponse{ .recv_bytes = &peer_open },
        session.PeerResponse{ .recv_bytes = &peer_keepalive },
    };

    var fake1 = try session.FakeTransport.init(std.testing.allocator, responses1);
    defer fake1.deinit();
    const trans1 = fake1.toTransport();

    var fake2 = try session.FakeTransport.init(std.testing.allocator, responses2);
    defer fake2.deinit();
    const trans2 = fake2.toTransport();

    const config1 = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = .{ 127, 0, 0, 1 },
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{},
        .same_as = true,
    };

    const config2 = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 2 },
        .peer_port = 179,
        .local_address = .{ 127, 0, 0, 2 },
        .local_as = 65003,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 3 },
        .hold_time_seconds = 120,
        .keepalive_seconds = 40,
        .connect_timeout_ms = 5000,
        .prefixes = &.{},
        .same_as = true,
    };

    var sess1 = try session.initWithClock(config1, &trans1, session.MockClock.interface());
    var sess2 = try session.initWithClock(config2, &trans2, session.MockClock.interface());

    // Complete handshake for both sessions
    _ = try session.runOnce(&sess1);
    _ = try session.runOnce(&sess1);
    _ = try session.runOnce(&sess1);
    try std.testing.expectEqual(session.SessionState.established, sess1.status.state);

    _ = try session.runOnce(&sess2);
    _ = try session.runOnce(&sess2);
    _ = try session.runOnce(&sess2);
    try std.testing.expectEqual(session.SessionState.established, sess2.status.state);

    // Verify each session has its own pending_keepalive flag
    try std.testing.expect(sess1.pending_keepalive);
    try std.testing.expect(sess2.pending_keepalive);

    // Verify each session has its own keepalive interval
    // Session 1: min(60, 180/3) = min(60, 60) = 60 seconds = 60000 ms
    try std.testing.expectEqual(@as(u32, 60000), sess1.keepalive_interval_ms);
    // Session 2: min(40, 120/3) = min(40, 40) = 40 seconds = 40000 ms
    try std.testing.expectEqual(@as(u32, 40000), sess2.keepalive_interval_ms);

    // Verify each session has its own negotiated hold time
    try std.testing.expectEqual(@as(u16, 180), sess1.negotiated_hold_time);
    try std.testing.expectEqual(@as(u16, 120), sess2.negotiated_hold_time);
}

test "KEEPALIVE message encoding produces correct wire format" {
    // Verify that message.encodeKeepalive produces a properly formatted BGP KEEPALIVE.
    // BGP KEEPALIVE format: 16 bytes 0xFF marker, 2 bytes length (19), 1 byte type (4).
    var buf: [4096]u8 = undefined;
    const len = message.encodeKeepalive(&buf);
    try std.testing.expectEqual(@as(usize, 19), len);

    // Check 16-byte marker (all 0xFF)
    try std.testing.expectEqual(@as(u8, 0xFF), buf[0]);
    try std.testing.expectEqual(@as(u8, 0xFF), buf[1]);
    try std.testing.expectEqual(@as(u8, 0xFF), buf[2]);
    try std.testing.expectEqual(@as(u8, 0xFF), buf[3]);
    try std.testing.expectEqual(@as(u8, 0xFF), buf[4]);
    try std.testing.expectEqual(@as(u8, 0xFF), buf[5]);
    try std.testing.expectEqual(@as(u8, 0xFF), buf[6]);
    try std.testing.expectEqual(@as(u8, 0xFF), buf[7]);
    try std.testing.expectEqual(@as(u8, 0xFF), buf[8]);
    try std.testing.expectEqual(@as(u8, 0xFF), buf[9]);
    try std.testing.expectEqual(@as(u8, 0xFF), buf[10]);
    try std.testing.expectEqual(@as(u8, 0xFF), buf[11]);
    try std.testing.expectEqual(@as(u8, 0xFF), buf[12]);
    try std.testing.expectEqual(@as(u8, 0xFF), buf[13]);
    try std.testing.expectEqual(@as(u8, 0xFF), buf[14]);
    try std.testing.expectEqual(@as(u8, 0xFF), buf[15]);

    // Check length field (bytes 16-17)
    try std.testing.expectEqual(@as(u8, 0), buf[16]); // high byte
    try std.testing.expectEqual(@as(u8, 19), buf[17]); // low byte = 19

    // Check message type (byte 18)
    try std.testing.expectEqual(@as(u8, 4), buf[18]); // KEEPALIVE
}

test "periodic scheduler sends KEEPALIVE bytes on fake transport" {
    // Regression test: periodic KEEPALIVE must update send_pos so flushSend sends bytes.
    // This was a bug where encodeKeepalive return value was discarded.
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
        .prefixes = &.{},
        .same_as = true,
    };

    var sess = try session.initWithClock(config, &trans, session.MockClock.interface());

    // Complete handshake to reach Established state
    _ = try session.runOnce(&sess); // idle -> open_sent (local OPEN)
    _ = try session.runOnce(&sess); // open_sent -> open_confirm (peer OPEN)
    _ = try session.runOnce(&sess); // open_confirm -> established (peer KEEPALIVE)

    try std.testing.expectEqual(session.SessionState.established, sess.status.state);

    // Record bytes sent before periodic keepalive
    const sent_before = fake.getAllSent().len;
    try std.testing.expect(sent_before > 0); // OPEN + KEEPALIVE sent during handshake

    // Advance mock clock to keepalive interval (60 seconds = 60000 ms)
    session.MockClock.advance(60000);

    // Call runOnce - this should trigger periodic KEEPALIVE
    _ = try session.runOnce(&sess);

    // Verify KEEPALIVE bytes were sent
    const sent_after = fake.getAllSent().len;
    const new_bytes = sent_after - sent_before;

    // Should have sent exactly 19 bytes (KEEPALIVE message)
    try std.testing.expectEqual(@as(usize, 19), new_bytes);

    // Verify the last 19 bytes are a BGP KEEPALIVE
    const all_sent = fake.getAllSent();
    const last_19 = all_sent[all_sent.len - 19 ..];
    try std.testing.expectEqual(@as(u8, 0xFF), last_19[0]);
    try std.testing.expectEqual(@as(u8, 0xFF), last_19[15]);
    try std.testing.expectEqual(@as(u8, 19), last_19[17]); // length
    try std.testing.expectEqual(@as(u8, 4), last_19[18]); // type = KEEPALIVE
}
