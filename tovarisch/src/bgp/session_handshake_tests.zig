// session_handshake_tests.zig — BGP session handshake tests
//
// ACT 2: Tests proving full BGP OPEN/KEEPALIVE/UPDATE handshake flow.
// Extracted from session_tests.zig to satisfy LLM-friendliness line limits.

const std = @import("std");
const session = @import("session.zig");
const types = @import("types.zig");

// === Full Handshake Tests ===

test "complete BGP handshake reaches established" {
    // Build peer OPEN message (29 bytes)
    var peer_open: [29]u8 = undefined;
    @memset(peer_open[0..16], 0xFF);
    peer_open[16] = 0;
    peer_open[17] = 29;
    peer_open[18] = 1; // OPEN
    peer_open[19] = 4; // version
    peer_open[20] = 0xFD; // peer AS = 65002 (big-endian: high byte)
    peer_open[21] = 0xEA; // peer AS = 65002 (big-endian: low byte)
    peer_open[22] = 0;
    peer_open[23] = 180; // hold time
    peer_open[24] = 10;
    peer_open[25] = 0;
    peer_open[26] = 0;
    peer_open[27] = 2; // router ID
    peer_open[28] = 0; // opt params

    // Build peer KEEPALIVE (19 bytes)
    var peer_keepalive: [19]u8 = undefined;
    @memset(peer_keepalive[0..16], 0xFF);
    peer_keepalive[16] = 0;
    peer_keepalive[17] = 19;
    peer_keepalive[18] = 4; // KEEPALIVE

    // Script: peer OPEN then peer KEEPALIVE
    const responses = &.{
        session.PeerResponse{ .recv_bytes = &peer_open },
        session.PeerResponse{ .recv_bytes = &peer_keepalive },
    };

    var fake = session.FakeTransport.init(std.testing.allocator, responses);
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

    var sess = try session.init(config, &trans);

    // runOnce from idle: sends OPEN
    var result = try session.runOnce(&sess);
    try std.testing.expectEqual(session.RunResult.ok, result);
    try std.testing.expectEqual(session.SessionState.open_sent, sess.status.state);
    try std.testing.expectEqual(@as(u64, 1), sess.status.messages_sent);

    // runOnce: receive peer OPEN, send KEEPALIVE
    result = try session.runOnce(&sess);
    try std.testing.expectEqual(session.RunResult.ok, result);
    try std.testing.expectEqual(session.SessionState.open_confirm, sess.status.state);
    try std.testing.expectEqual(@as(u64, 2), sess.status.messages_sent);
    try std.testing.expectEqual(@as(u64, 1), sess.status.keepalives_sent);

    // runOnce: receive peer KEEPALIVE, send UPDATE, reach established
    result = try session.runOnce(&sess);
    try std.testing.expectEqual(session.RunResult.established, result);
    try std.testing.expectEqual(session.SessionState.established, sess.status.state);
    try std.testing.expectEqual(@as(u64, 1), sess.status.updates_sent);
    try std.testing.expectEqual(@as(u64, 3), sess.status.messages_sent);

    // advertised_prefix_count should be set
    try std.testing.expectEqual(@as(usize, 1), sess.status.advertised_prefix_count);
}

test "session validates peer AS on OPEN" {
    // Build peer OPEN with wrong AS (65022 instead of 65002)
    var peer_open: [29]u8 = undefined;
    @memset(peer_open[0..16], 0xFF);
    peer_open[16] = 0;
    peer_open[17] = 29;
    peer_open[18] = 1; // OPEN
    peer_open[19] = 4; // version
    peer_open[20] = 0xFE; // peer AS = 65022
    peer_open[21] = 0xEE;
    peer_open[22] = 0;
    peer_open[23] = 180;
    peer_open[24] = 10;
    peer_open[25] = 0;
    peer_open[26] = 0;
    peer_open[27] = 2;
    peer_open[28] = 0;

    const responses = &.{
        session.PeerResponse{ .recv_bytes = &peer_open },
    };

    var fake = session.FakeTransport.init(std.testing.allocator, responses);
    defer fake.deinit();
    const trans = fake.toTransport();

    const config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = null,
        .local_as = 65001,
        .peer_as = 65002, // Expects 65002
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{types.Ipv4Prefix.init("10.0.0.0/8")},
        .same_as = true,
    };

    var sess = try session.init(config, &trans);

    // Send OPEN
    _ = try session.runOnce(&sess);
    try std.testing.expectEqual(session.SessionState.open_sent, sess.status.state);

    // Receive peer OPEN with wrong AS -> should fail
    _ = try session.runOnce(&sess);
    try std.testing.expectEqual(session.SessionState.failed, sess.status.state);
    try std.testing.expect(sess.status.last_error != null);
    try std.testing.expectEqualStrings("peer AS mismatch", sess.status.last_error.?.message);
}

test "incoming UPDATE is ignored (import-nothing)" {
    // Build peer OPEN
    var peer_open: [29]u8 = undefined;
    @memset(peer_open[0..16], 0xFF);
    peer_open[16] = 0;
    peer_open[17] = 29;
    peer_open[18] = 1;
    peer_open[19] = 4;
    peer_open[20] = 0xFD; // peer AS = 65002 (big-endian)
    peer_open[21] = 0xEA;
    peer_open[22] = 0;
    peer_open[23] = 180;
    peer_open[24] = 10;
    peer_open[25] = 0;
    peer_open[26] = 0;
    peer_open[27] = 2;
    peer_open[28] = 0;

    // Build peer KEEPALIVE
    var peer_keepalive: [19]u8 = undefined;
    @memset(peer_keepalive[0..16], 0xFF);
    peer_keepalive[16] = 0;
    peer_keepalive[17] = 19;
    peer_keepalive[18] = 4;

    // Build peer UPDATE (minimal: just header + withdrawn_len = 0 + attrs_len = 0)
    var peer_update: [23]u8 = undefined;
    @memset(peer_update[0..16], 0xFF);
    peer_update[16] = 0;
    peer_update[17] = 23;
    peer_update[18] = 2; // UPDATE
    peer_update[19] = 0; // withdrawn len
    peer_update[20] = 0;
    peer_update[21] = 0; // attrs len
    peer_update[22] = 0;

    const responses = &.{
        session.PeerResponse{ .recv_bytes = &peer_open },
        session.PeerResponse{ .recv_bytes = &peer_keepalive },
        session.PeerResponse{ .recv_bytes = &peer_update },
    };

    var fake = session.FakeTransport.init(std.testing.allocator, responses);
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

    var sess = try session.init(config, &trans);

    // Complete handshake
    _ = try session.runOnce(&sess); // Send OPEN
    _ = try session.runOnce(&sess); // OPEN -> KEEPALIVE
    _ = try session.runOnce(&sess); // KEEPALIVE -> UPDATE -> established

    // Run once more to receive peer UPDATE
    _ = try session.runOnce(&sess);

    // Verify session still established
    try std.testing.expectEqual(session.SessionState.established, sess.status.state);

    // Verify UPDATE was received but NOT imported
    try std.testing.expectEqual(@as(usize, 1), sess.status.advertised_prefix_count);

    // Verify messages_received includes the UPDATE
    try std.testing.expect(sess.status.messages_received >= 3);
}

test "incoming NOTIFICATION transitions to failed" {
    // Build peer OPEN
    var peer_open: [29]u8 = undefined;
    @memset(peer_open[0..16], 0xFF);
    peer_open[16] = 0;
    peer_open[17] = 29;
    peer_open[18] = 1;
    peer_open[19] = 4;
    peer_open[20] = 0xFD; // peer AS = 65002 (big-endian)
    peer_open[21] = 0xEA;
    peer_open[22] = 0;
    peer_open[23] = 180;
    peer_open[24] = 10;
    peer_open[25] = 0;
    peer_open[26] = 0;
    peer_open[27] = 2;
    peer_open[28] = 0;

    // Build peer KEEPALIVE
    var peer_keepalive: [19]u8 = undefined;
    @memset(peer_keepalive[0..16], 0xFF);
    peer_keepalive[16] = 0;
    peer_keepalive[17] = 19;
    peer_keepalive[18] = 4;

    // Build peer NOTIFICATION (Cease = 6, no subcode)
    var peer_notif: [21]u8 = undefined;
    @memset(peer_notif[0..16], 0xFF);
    peer_notif[16] = 0;
    peer_notif[17] = 21;
    peer_notif[18] = 3; // NOTIFICATION
    peer_notif[19] = 6; // Cease
    peer_notif[20] = 0; // no subcode

    const responses = &.{
        session.PeerResponse{ .recv_bytes = &peer_open },
        session.PeerResponse{ .recv_bytes = &peer_keepalive },
        session.PeerResponse{ .recv_bytes = &peer_notif },
    };

    var fake = session.FakeTransport.init(std.testing.allocator, responses);
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

    var sess = try session.init(config, &trans);

    // Complete handshake
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);

    // Run once more to receive peer NOTIFICATION
    const result = try session.runOnce(&sess);

    try std.testing.expectEqual(session.RunResult.failed, result);
    try std.testing.expectEqual(session.SessionState.failed, sess.status.state);
    try std.testing.expectEqual(@as(u8, 6), sess.status.last_notification_code);
    try std.testing.expectEqual(@as(u8, 0), sess.status.last_notification_subcode);
}
