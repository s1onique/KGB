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

    // runOnce: send UPDATE (deferred from established transition)
    result = try session.runOnce(&sess);
    try std.testing.expectEqual(@as(u64, 1), sess.status.updates_sent);
    try std.testing.expectEqual(@as(u64, 3), sess.status.messages_sent);

    // nlri_sent_count should match configured prefix count
    try std.testing.expectEqual(@as(usize, 1), sess.status.nlri_sent_count);

    // configured_prefix_count should be set
    try std.testing.expectEqual(@as(usize, 1), sess.status.configured_prefix_count);
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

    var fake = try session.FakeTransport.init(std.testing.allocator, responses);
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

    var sess = try session.initWithClock(config, &trans, session.MockClock.interface());

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
    _ = try session.runOnce(&sess); // Send OPEN
    _ = try session.runOnce(&sess); // OPEN -> KEEPALIVE
    _ = try session.runOnce(&sess); // KEEPALIVE -> UPDATE -> established

    // Run once more to receive peer UPDATE
    _ = try session.runOnce(&sess);

    // Verify session still established
    try std.testing.expectEqual(session.SessionState.established, sess.status.state);

    // Verify configured_prefix_count matches
    try std.testing.expectEqual(@as(usize, 1), sess.status.configured_prefix_count);

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

    // Run once more to receive peer NOTIFICATION
    const result = try session.runOnce(&sess);

    try std.testing.expectEqual(session.RunResult.failed, result);
    try std.testing.expectEqual(session.SessionState.failed, sess.status.state);
    try std.testing.expectEqual(@as(u8, 6), sess.status.last_notification_code);
    try std.testing.expectEqual(@as(u8, 0), sess.status.last_notification_subcode);
}

// === UPDATE Batching Tests ===

test "UPDATE batching sends multiple UPDATEs for large prefix sets" {
    // Build peer OPEN message
    var peer_open: [29]u8 = undefined;
    @memset(peer_open[0..16], 0xFF);
    peer_open[16] = 0;
    peer_open[17] = 29;
    peer_open[18] = 1; // OPEN
    peer_open[19] = 4; // version
    peer_open[20] = 0xFD; // peer AS = 65002
    peer_open[21] = 0xEA;
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

    // Create more prefixes than fit in one UPDATE to force batching
    // MAX_PREFIXES_PER_UPDATE is ~811, use 1000 to force at least 2 batches
    const prefix_count = 1000;
    var prefixes: [prefix_count]types.Ipv4Prefix = undefined;
    for (0..prefix_count) |i| {
        // Generate prefix 10.x.y.0/24 pattern (use two octets for variety)
        prefixes[i] = types.Ipv4Prefix{
            .addr = .{ 10, @as(u8, @intCast(i / 256)), @as(u8, @intCast(i % 256)), 0 },
            .len = 24,
        };
    }

    var fake = try session.FakeTransport.init(std.testing.allocator, &.{
        session.PeerResponse{ .recv_bytes = &peer_open },
        session.PeerResponse{ .recv_bytes = &peer_keepalive },
    });
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
        .prefixes = &prefixes,
        .same_as = true,
    };

    var sess = try session.initWithClock(config, &trans, session.MockClock.interface());

    // Complete handshake (3 runOnce calls)
    _ = try session.runOnce(&sess); // Send OPEN
    _ = try session.runOnce(&sess); // OPEN received, send KEEPALIVE
    _ = try session.runOnce(&sess); // KEEPALIVE received, become established

    // After handshake, export is not complete - we'll send batches on subsequent runs
    try std.testing.expectEqual(session.SessionState.established, sess.status.state);
    try std.testing.expect(!sess.export_complete);

    // First runOnce after established: send first UPDATE batch
    _ = try session.runOnce(&sess);
    try std.testing.expectEqual(@as(u64, 1), sess.status.updates_sent);
    try std.testing.expect(sess.export_batch_index > 0);
    try std.testing.expect(!sess.export_complete);

    // Continue sending batches until complete
    var iterations: usize = 0;
    while (!sess.export_complete and iterations < 10) {
        _ = try session.runOnce(&sess);
        iterations += 1;
    }

    // Verify batching worked - should have sent all prefixes
    try std.testing.expect(sess.export_complete);
    // updates_sent = 1 (handshake batch) + iterations (export batches)
    try std.testing.expectEqual(@as(u64, @as(u64, @intCast(iterations + 1))), sess.status.updates_sent);
    try std.testing.expectEqual(@as(usize, prefix_count), sess.nlri_sent_count);
    try std.testing.expectEqual(@as(usize, prefix_count), sess.status.configured_prefix_count);
}

test "UPDATE batching max prefixes per batch constant is reasonable" {
    // Verify our batching constant is sensible
    // Each prefix needs: 1 byte length + up to 4 bytes address
    // Path attributes: ORIGIN(4) + AS_PATH(~7) + NEXT_HOP(7) = 18 bytes
    // Total per prefix: 1-5 bytes
    // 4096 - 19(header) - 2(withdrawn) - 18(attrs) = 4057 bytes for NLRI
    // With /32 prefixes (5 bytes each), we can fit ~811 prefixes
    try std.testing.expect(session.MAX_PREFIXES_PER_UPDATE >= 700);
    try std.testing.expect(session.MAX_PREFIXES_PER_UPDATE <= 1000);
}
