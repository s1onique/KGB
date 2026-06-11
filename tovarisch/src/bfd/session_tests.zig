// session_tests.zig — Extended BFD session tests
//
// Tests for BIRD-compatible multihop session behavior.

const std = @import("std");
const packet = @import("packet.zig");
const config = @import("config.zig");
const clock = @import("clock.zig");
const session = @import("session.zig");

test "BIRD config simulation: interval 800ms multiplier 3" {
    clock.MockClock.reset();
    const mock_clock = clock.MockClock.interface();

    // Simulate BIRD config:
    // protocol bfd {
    //     multihop {
    //         interval 800 ms;
    //         multiplier 3;
    //     };
    // }
    const cfg = config.BfdConfig{
        .mode = .multihop,
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 800,
        .multiplier = 3,
    };

    var sess = session.init(cfg, mock_clock);

    // Start the session
    _ = session.processEvent(&sess, .start);
    try std.testing.expectEqual(session.SessionState.down, sess.state);
    try std.testing.expect(sess.local_discr != 0);

    // Verify detection timeout matches BIRD config
    // Detection timeout = interval_ms × multiplier = 800 × 3 = 2400
    try std.testing.expectEqual(@as(u32, 2400), sess.detection_timeout_ms);
}

test "detection timeout calculation matches BIRD config" {
    // Verify the calculation: remote interval 800000 us, detect_mult 3
    const timeout = config.calculateDetectionTimeout(800_000, 3);
    try std.testing.expectEqual(@as(u32, 2400), timeout);

    // Also verify our session's default timeout matches
    const cfg = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 800,
        .multiplier = 3,
    };
    try std.testing.expectEqual(@as(u32, 2400), config.defaultDetectionTimeout(cfg));
}

test "complete BFD handshake simulation" {
    clock.MockClock.reset();
    const mock_clock = clock.MockClock.interface();

    const cfg = config.BfdConfig{
        .mode = .multihop,
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 800,
        .multiplier = 3,
    };

    var sess = session.init(cfg, mock_clock);

    // Start session (local is initiator)
    _ = session.processEvent(&sess, .start);
    try std.testing.expectEqual(session.SessionState.down, sess.state);

    // Build and check first transmit packet
    const tx1 = session.buildTransmitPacket(&sess);
    try std.testing.expectEqual(session.SessionState.down, tx1.state);
    try std.testing.expectEqual(@as(u32, 0), tx1.your_discr); // No remote yet

    // Simulate receiving Init from peer
    const init_pkt = packet.ControlPacket{
        .state = .init,
        .my_discr = 0xBEEF,
        .your_discr = sess.local_discr,
        .detect_mult = 3,
        .required_min_rx_interval = 800_000,
    };
    _ = session.processEvent(&sess, .{ .packetReceived = init_pkt });
    try std.testing.expectEqual(session.SessionState.init, sess.state);
    try std.testing.expectEqual(@as(u32, 0xBEEF), sess.remote_discr);

    // Simulate receiving Up from peer
    const up_pkt = packet.ControlPacket{
        .state = .up,
        .my_discr = 0xBEEF,
        .your_discr = sess.local_discr,
        .detect_mult = 3,
        .required_min_rx_interval = 800_000,
    };
    _ = session.processEvent(&sess, .{ .packetReceived = up_pkt });
    try std.testing.expectEqual(session.SessionState.up, sess.state);

    // Detection timeout should be recalculated based on remote's requirements
    try std.testing.expectEqual(@as(u32, 2400), sess.detection_timeout_ms);
}

test "session goes Down when detection timer expires" {
    clock.MockClock.reset();
    const mock_clock = clock.MockClock.interface();

    const cfg = config.BfdConfig{
        .mode = .multihop,
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 800,
        .multiplier = 3,
    };

    var sess = session.init(cfg, mock_clock);
    sess.state = .up;
    sess.remote_discr = 0xBEEF;
    sess.last_packet_recv_time = 0;
    sess.detection_timeout_ms = 2400;

    // Advance time past detection timeout
    clock.MockClock.advance(2500);

    // Detection should be expired
    try std.testing.expect(session.isDetectionExpired(&sess));

    // Process detection timeout event
    _ = session.processEvent(&sess, .detectionTimeout);
    try std.testing.expectEqual(session.SessionState.down, sess.state);
    try std.testing.expectEqual(@as(u64, 1), sess.stats.detection_timeouts);
}

test "session in AdminDown does not transmit" {
    clock.MockClock.reset();
    const mock_clock = clock.MockClock.interface();

    const cfg = config.BfdConfig{
        .mode = .multihop,
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 800,
        .multiplier = 3,
    };

    var sess = session.init(cfg, mock_clock);
    _ = session.processEvent(&sess, .stop);
    try std.testing.expectEqual(session.SessionState.admin_down, sess.state);

    // Should not be due to transmit
    try std.testing.expect(!session.isTransmitDue(&sess));
}

test "discriminator learning" {
    clock.MockClock.reset();
    const mock_clock = clock.MockClock.interface();

    const cfg = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
    };

    var sess = session.init(cfg, mock_clock);
    try std.testing.expectEqual(@as(u32, 0), sess.remote_discr);

    // First packet with remote discriminator
    const pkt1 = packet.ControlPacket{
        .state = .down,
        .my_discr = 0x1234,
        .your_discr = sess.local_discr,
    };
    _ = session.processEvent(&sess, .{ .packetReceived = pkt1 });
    try std.testing.expectEqual(@as(u32, 0x1234), sess.remote_discr);

    // Subsequent packet with different my_discr should not update
    // (we already learned the discriminator)
    const pkt2 = packet.ControlPacket{
        .state = .down,
        .my_discr = 0x5678, // Different!
        .your_discr = sess.local_discr,
    };
    _ = session.processEvent(&sess, .{ .packetReceived = pkt2 });
    try std.testing.expectEqual(@as(u32, 0x1234), sess.remote_discr); // Still original
}

test "packet dropped with mismatch your_discr" {
    clock.MockClock.reset();
    const mock_clock = clock.MockClock.interface();

    const cfg = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
    };

    var sess = session.init(cfg, mock_clock);
    sess.local_discr = 100;

    // Packet for different session
    const wrong_pkt = packet.ControlPacket{
        .state = .up,
        .my_discr = 200,
        .your_discr = 999, // Not for us
    };
    _ = session.processEvent(&sess, .{ .packetReceived = wrong_pkt });

    try std.testing.expectEqual(@as(u64, 1), sess.stats.packets_dropped);
    try std.testing.expectEqual(session.SessionState.down, sess.state); // Unchanged
}

test "transmit timing for periodic packets" {
    clock.MockClock.reset();
    clock.MockClock.setTime(0);
    const mock_clock = clock.MockClock.interface();

    const cfg = config.BfdConfig{
        .mode = .multihop,
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 800,
        .multiplier = 3,
    };

    var sess = session.init(cfg, mock_clock);
    sess.state = .up;
    sess.next_transmit_time = 0;

    // First transmit should be due immediately (or now)
    try std.testing.expect(session.isTransmitDue(&sess));

    // Process transmit timeout
    _ = session.processEvent(&sess, .transmitTimeout);

    // Next transmit should be scheduled 800ms later
    try std.testing.expectEqual(@as(clock.MonoTime, 800), sess.next_transmit_time);

    // Advance time to 799ms - NOT due yet
    clock.MockClock.setTime(799);
    try std.testing.expect(!session.isTransmitDue(&sess));

    // Advance to 800ms - due
    clock.MockClock.setTime(800);
    try std.testing.expect(session.isTransmitDue(&sess));
}

test "statistics increment correctly" {
    clock.MockClock.reset();
    const mock_clock = clock.MockClock.interface();

    const cfg = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
    };

    var sess = session.init(cfg, mock_clock);

    // Start session
    _ = session.processEvent(&sess, .start);
    try std.testing.expectEqual(@as(u64, 1), sess.stats.state_changes);

    // Build packets
    _ = session.buildTransmitPacket(&sess);
    _ = session.buildTransmitPacket(&sess);
    try std.testing.expectEqual(@as(u64, 2), sess.stats.packets_sent);

    // Receive packets
    const pkt = packet.ControlPacket{
        .state = .down,
        .my_discr = 0x1234,
        .your_discr = sess.local_discr,
    };
    _ = session.processEvent(&sess, .{ .packetReceived = pkt });
    _ = session.processEvent(&sess, .{ .packetReceived = pkt });
    try std.testing.expectEqual(@as(u64, 2), sess.stats.packets_received);
}

test "session with custom local discriminator" {
    clock.MockClock.reset();
    const mock_clock = clock.MockClock.interface();

    const cfg = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .local_discr = 0xDEADBEEF,
    };

    var sess = session.init(cfg, mock_clock);
    _ = session.processEvent(&sess, .start);

    try std.testing.expectEqual(@as(u32, 0xDEADBEEF), sess.local_discr);

    // Build packet and verify discriminator
    const tx_pkt = session.buildTransmitPacket(&sess);
    try std.testing.expectEqual(@as(u32, 0xDEADBEEF), tx_pkt.my_discr);
}

test "up after Init with Init from peer" {
    clock.MockClock.reset();
    const mock_clock = clock.MockClock.interface();

    const cfg = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
    };

    var sess = session.init(cfg, mock_clock);
    _ = session.processEvent(&sess, .start);
    sess.local_discr = 1;

    // Init from peer
    const init_pkt = packet.ControlPacket{
        .state = .init,
        .my_discr = 2,
        .your_discr = 1,
        .detect_mult = 3,
        .required_min_rx_interval = 800_000,
    };
    _ = session.processEvent(&sess, .{ .packetReceived = init_pkt });
    try std.testing.expectEqual(session.SessionState.init, sess.state);

    // Response Init from peer brings us to Up
    const init2_pkt = packet.ControlPacket{
        .state = .init,
        .my_discr = 2,
        .your_discr = 1,
        .detect_mult = 3,
        .required_min_rx_interval = 800_000,
    };
    _ = session.processEvent(&sess, .{ .packetReceived = init2_pkt });
    try std.testing.expectEqual(session.SessionState.up, sess.state);
}

test "wrong nonzero YourDiscr is still dropped" {
    clock.MockClock.reset();
    const mock_clock = clock.MockClock.interface();

    const cfg = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .local_discr = 100,
    };

    var sess = session.init(cfg, mock_clock);
    try std.testing.expectEqual(@as(u32, 100), sess.local_discr);

    // Packet with wrong nonzero YourDiscr should be dropped
    const wrong_pkt = packet.ControlPacket{
        .state = .up,
        .my_discr = 200,
        .your_discr = 999, // Not 0, not our discriminator
        .detect_mult = 3,
    };
    _ = session.processEvent(&sess, .{ .packetReceived = wrong_pkt });

    // Should be counted as dropped
    try std.testing.expectEqual(@as(u64, 1), sess.stats.packets_dropped);
    // State should remain Down (packet was dropped before state transition)
    try std.testing.expectEqual(session.SessionState.down, sess.state);
    // Note: remote_discr is learned BEFORE your_discr check, so it IS set.
    // This is correct - we learn peers from my_discr regardless of addressing.
    try std.testing.expectEqual(@as(u32, 200), sess.remote_discr);
}
