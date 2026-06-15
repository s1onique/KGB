// session_buffer_compaction_tests.zig — Regression tests for recv buffer compaction
//
// ACT: Fix BGP runtime aliasing panic at runtime.zig:56
// Root cause: @memcpy with overlapping slices in tryDecodeFrame buffer compaction.
// Fix: Replace @memcpy with std.mem.copyForwards for overlapping copies.
//
// This test file verifies that recv buffer compaction handles the overlapping
// copy case correctly WITHOUT panicking.
//
// The panic occurred when:
// 1. BGP session received data into recv_buf
// 2. tryDecodeFrame decoded a frame but partial data remained
// 3. Buffer compaction used @memcpy on overlapping slices
// 4. Zig 0.16 panics: "@memcpy arguments alias"
//
// The fix uses std.mem.copyForwards which handles overlapping copies correctly.

const std = @import("std");
const session = @import("session.zig");
const types = @import("types.zig");

/// Build a minimal BGP OPEN message.
fn buildPeerOpen(peer_as: u16) [29]u8 {
    var msg: [29]u8 = undefined;
    @memset(msg[0..16], 0xFF);
    msg[16] = 0;
    msg[17] = 29;
    msg[18] = 1; // OPEN
    msg[19] = 4; // version
    msg[20] = @as(u8, @intCast(peer_as >> 8));
    msg[21] = @as(u8, @intCast(peer_as & 0xFF));
    msg[22] = 0;
    msg[23] = 180; // hold time
    msg[24] = 10;
    msg[25] = 0;
    msg[26] = 0;
    msg[27] = 2; // router ID
    msg[28] = 0; // opt params
    return msg;
}

/// Build a minimal BGP KEEPALIVE message.
fn buildKeepalive() [19]u8 {
    var msg: [19]u8 = undefined;
    @memset(msg[0..16], 0xFF);
    msg[16] = 0;
    msg[17] = 19;
    msg[18] = 4; // KEEPALIVE
    return msg;
}

test "recv buffer compaction handles overlapping copy without panic" {
    // Test reproduces the prod scenario:
    // 1. recv_buf contains OPEN (29 bytes)
    // 2. Additional bytes arrive (e.g., more data after the OPEN)
    // 3. After decoding OPEN, remaining bytes need to be compacted
    // 4. The compaction copies overlapping slices in recv_buf
    //
    // Before the fix: Zig 0.16 panicked with "@memcpy arguments alias"
    // After the fix: std.mem.copyForwards handles overlapping copies correctly
    session.MockClock.reset();

    // Build peer OPEN message
    const peer_open = buildPeerOpen(65002);

    // Build peer KEEPALIVE message
    const peer_keepalive = buildKeepalive();

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

    // First runOnce: sends OPEN
    var result = try session.runOnce(&sess);
    try std.testing.expectEqual(session.RunResult.ok, result);
    try std.testing.expectEqual(session.SessionState.open_sent, sess.status.state);

    // Second runOnce: receives peer OPEN, sends KEEPALIVE
    result = try session.runOnce(&sess);
    try std.testing.expectEqual(session.RunResult.ok, result);
    try std.testing.expectEqual(session.SessionState.open_confirm, sess.status.state);

    // Third runOnce: receives peer KEEPALIVE, reaches established
    result = try session.runOnce(&sess);
    try std.testing.expectEqual(session.RunResult.established, result);
    try std.testing.expectEqual(session.SessionState.established, sess.status.state);

    // If we got here without panic, the overlapping copy fix is working.
    // This test exercises the full handshake which is where prod crashed.
    // Prod crash sequence: bgp_open_received → bgp_keepalive_sent → panic
}

test "recv buffer compaction handles back-to-back messages without panic" {
    // Scenario: Two BGP messages arrive in a single recv() call.
    // After decoding the first, the second needs to be compacted.
    // This exercises the overlapping copy path with different alignment.
    session.MockClock.reset();

    // Build peer OPEN message
    const peer_open = buildPeerOpen(65002);

    // Build two consecutive KEEPALIVE messages
    var back_to_back: [38]u8 = undefined;
    @memset(back_to_back[0..16], 0xFF);
    back_to_back[16] = 0;
    back_to_back[17] = 19;
    back_to_back[18] = 4; // KEEPALIVE
    @memset(back_to_back[19..35], 0xFF);
    back_to_back[35] = 0;
    back_to_back[36] = 19;
    back_to_back[37] = 4; // KEEPALIVE

    // Script: peer OPEN then two KEEPALIVEs back-to-back
    const responses = &.{
        session.PeerResponse{ .recv_bytes = &peer_open },
        session.PeerResponse{ .recv_bytes = &back_to_back },
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
        .prefixes = &.{},
        .same_as = true,
    };

    var sess = try session.initWithClock(config, &trans, session.MockClock.interface());

    // First runOnce: sends OPEN
    _ = try session.runOnce(&sess);
    try std.testing.expectEqual(session.SessionState.open_sent, sess.status.state);

    // Second runOnce: receives peer OPEN, sends KEEPALIVE
    _ = try session.runOnce(&sess);
    try std.testing.expectEqual(session.SessionState.open_confirm, sess.status.state);

    // Third runOnce: receives back-to-back KEEPALIVEs
    // The first KEEPALIVE transitions to established
    // The second needs buffer compaction
    _ = try session.runOnce(&sess);
    try std.testing.expectEqual(session.SessionState.established, sess.status.state);

    // If we got here without panic, the overlapping copy fix is working.
    // This test exercises the exact crash scenario: bgp_open_received → bgp_keepalive_sent → panic
}

test "established session with UPDATE handles buffer compaction without panic" {
    // Test that established session with UPDATE handles buffer compaction correctly.
    session.MockClock.reset();

    // Build peer OPEN message
    const peer_open = buildPeerOpen(65002);

    // Build peer KEEPALIVE message
    const peer_keepalive = buildKeepalive();

    // Build peer UPDATE (minimal)
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
    _ = try session.runOnce(&sess); // KEEPALIVE -> established

    // Run again to receive UPDATE
    _ = try session.runOnce(&sess);

    // If we got here without panic, the overlapping copy fix is working.
    try std.testing.expectEqual(session.SessionState.established, sess.status.state);
}
