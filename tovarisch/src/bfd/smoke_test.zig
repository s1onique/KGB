// smoke_test.zig — BFD BIRD-compatible peer modeling test
//
// This test simulates a complete BFD multihop session between two peers
// without requiring BIRD to be installed. It models the BIRD config:
//
//   protocol bfd {
//       multihop {
//           interval 800 ms;
//           multiplier 3;
//       };
//   }
//
// Test simulates:
// - Two peers exchanging BFD control packets
// - Handshake from Down through Init to Up state
// - Detection timeout behavior when packets stop
// - Session recovery when packets resume

const std = @import("std");
const packet = @import("packet.zig");
const config = @import("config.zig");
const clock = @import("clock.zig");
const session = @import("session.zig");

/// Simulated BIRD peer (responder side)
const BIRDPeer = struct {
    discr: u32 = 0,
    state: packet.State = .down,
    interval_ms: u32 = 800,
    multiplier: u8 = 3,
};

// Simulates a full BIRD-compatible BFD multihop session.
// This is an integration-style test that doesn't require BIRD or network access.
test "BIRD multihop session simulation" {
    clock.MockClock.reset();
    const mock_clock = clock.MockClock.interface();

    // Simulate BIRD config on both sides:
    // interval 800 ms; multiplier 3
    const cfg_local = config.BfdConfig{
        .mode = .multihop,
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 800,
        .multiplier = 3,
        .role = .initiator,
    };

    var local_sess = session.init(cfg_local, mock_clock);

    // BIRD peer (simulated responder)
    var bird_peer = BIRDPeer{
        .discr = 0xBEEF,
        .state = .down,
        .interval_ms = 800,
        .multiplier = 3,
    };

    // Start local session
    _ = session.processEvent(&local_sess, .start);
    try std.testing.expectEqual(session.SessionState.down, local_sess.state);

    // === Round 1: Local sends initial Down packet ===
    const tx1 = session.buildTransmitPacket(&local_sess);
    try std.testing.expectEqual(session.SessionState.down, tx1.state);
    try std.testing.expectEqual(@as(u32, 0), tx1.your_discr); // No peer yet

    // Simulate BIRD peer receiving and responding with Init
    // (In real scenario, BIRD would send Init packet)
    bird_peer.state = .init;
    const bird_response = packet.ControlPacket{
        .state = bird_peer.state,
        .my_discr = bird_peer.discr,
        .your_discr = local_sess.local_discr,
        .detect_mult = bird_peer.multiplier,
        .required_min_rx_interval = config.msToUs(bird_peer.interval_ms),
    };

    _ = session.processEvent(&local_sess, .{ .packetReceived = bird_response });
    try std.testing.expectEqual(session.SessionState.init, local_sess.state);
    try std.testing.expectEqual(bird_peer.discr, local_sess.remote_discr);

    // === Round 2: BIRD sends Up (polite - both sides agree) ===
    bird_peer.state = .up;
    const bird_up = packet.ControlPacket{
        .state = bird_peer.state,
        .my_discr = bird_peer.discr,
        .your_discr = local_sess.local_discr,
        .detect_mult = bird_peer.multiplier,
        .required_min_rx_interval = config.msToUs(bird_peer.interval_ms),
    };

    _ = session.processEvent(&local_sess, .{ .packetReceived = bird_up });
    try std.testing.expectEqual(session.SessionState.up, local_sess.state);

    // Verify detection timeout is correct
    // Detection = interval × multiplier = 800 × 3 = 2400ms
    try std.testing.expectEqual(@as(u32, 2400), local_sess.detection_timeout_ms);

    // === Simulate path failure: no packets for 2.5 seconds ===
    clock.MockClock.advance(2500);

    // Detection should be expired
    try std.testing.expect(session.isDetectionExpired(&local_sess));

    // Process detection timeout
    _ = session.processEvent(&local_sess, .detectionTimeout);
    try std.testing.expectEqual(session.SessionState.down, local_sess.state);
    try std.testing.expectEqual(@as(u64, 1), local_sess.stats.detection_timeouts);

    // === Session recovery: BIRD sends Init again ===
    bird_peer.state = .init;
    const recovery_init = packet.ControlPacket{
        .state = bird_peer.state,
        .my_discr = bird_peer.discr,
        .your_discr = local_sess.local_discr,
        .detect_mult = bird_peer.multiplier,
        .required_min_rx_interval = config.msToUs(bird_peer.interval_ms),
    };

    _ = session.processEvent(&local_sess, .{ .packetReceived = recovery_init });
    try std.testing.expectEqual(session.SessionState.init, local_sess.state);

    // BIRD completes handshake with Up
    bird_peer.state = .up;
    const recovery_up = packet.ControlPacket{
        .state = bird_peer.state,
        .my_discr = bird_peer.discr,
        .your_discr = local_sess.local_discr,
        .detect_mult = bird_peer.multiplier,
        .required_min_rx_interval = config.msToUs(bird_peer.interval_ms),
    };

    _ = session.processEvent(&local_sess, .{ .packetReceived = recovery_up });
    try std.testing.expectEqual(session.SessionState.up, local_sess.state);

    // Final stats check
    try std.testing.expect(local_sess.stats.packets_received > 0);
    try std.testing.expect(local_sess.stats.packets_sent > 0);
    try std.testing.expect(local_sess.stats.state_changes >= 3); // Down->Init, Init->Down, Down->Init, Init->Up
}

// Test that session correctly calculates intervals for different BIRD configs
test "BIRD interval variants" {
    clock.MockClock.reset();
    const mock_clock = clock.MockClock.interface();

    // Test 1: Very fast BIRD config (100ms)
    {
        const cfg = config.BfdConfig{
            .local_addr = "10.0.0.1",
            .peer_addr = "10.0.0.2",
            .interval_ms = 100,
            .multiplier = 5,
        };
        const sess = session.init(cfg, mock_clock);
        try std.testing.expectEqual(@as(u32, 500), sess.detection_timeout_ms);
    }

    // Test 2: Slow BIRD config (10s)
    {
        const cfg = config.BfdConfig{
            .local_addr = "10.0.0.1",
            .peer_addr = "10.0.0.2",
            .interval_ms = 10000,
            .multiplier = 2,
        };
        const sess = session.init(cfg, mock_clock);
        try std.testing.expectEqual(@as(u32, 20000), sess.detection_timeout_ms);
    }

    // Test 3: BIRD defaults (if unspecified)
    {
        const cfg = config.BfdConfig{
            .local_addr = "10.0.0.1",
            .peer_addr = "10.0.0.2",
        };
        const sess = session.init(cfg, mock_clock);
        // Default: interval 800ms, multiplier 3
        try std.testing.expectEqual(@as(u32, 800), sess.config.interval_ms);
        try std.testing.expectEqual(@as(u8, 3), sess.config.multiplier);
        try std.testing.expectEqual(@as(u32, 2400), sess.detection_timeout_ms);
    }
}

// Test UDP port constants match RFC 5883
test "multihop UDP port is 4784" {
    try std.testing.expectEqual(@as(u16, 4784), packet.MULTIHOP_UDP_PORT);
    // Single-hop should use 3784 (not implemented in this ACT)
    try std.testing.expectEqual(@as(u16, 3784), packet.SINGLEHOP_UDP_PORT);
}

// Test that discriminator learning works correctly
test "discriminator exchange between peers" {
    clock.MockClock.reset();
    const mock_clock = clock.MockClock.interface();

    const cfg = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
    };

    var sess = session.init(cfg, mock_clock);
    _ = session.processEvent(&sess, .start);
    const local_discr = sess.local_discr;
    try std.testing.expect(local_discr != 0);

    // Simulate peer packet with our discriminator
    const peer_pkt = packet.ControlPacket{
        .state = .init,
        .my_discr = 0xCAFE,
        .your_discr = local_discr,
    };
    _ = session.processEvent(&sess, .{ .packetReceived = peer_pkt });

    // Verify we learned peer's discriminator
    try std.testing.expectEqual(@as(u32, 0xCAFE), sess.remote_discr);

    // Now our transmit packet should include peer's discriminator
    const tx_pkt = session.buildTransmitPacket(&sess);
    try std.testing.expectEqual(@as(u32, 0xCAFE), tx_pkt.your_discr);
    try std.testing.expectEqual(local_discr, tx_pkt.my_discr);
}

// Test packet roundtrip encoding matches RFC 5880 wire format
test "wire format compatible with BIRD" {
    var buf: [32]u8 = undefined;

    // Build a BFD control packet per RFC 5880 (24 bytes)
    // BIRD uses the same format for multihop BFD
    const bird_pkt = packet.ControlPacket{
        .state = .up,
        .diag = .no_diagnostic,
        .detect_mult = 3,
        .my_discr = 0x12345678,
        .your_discr = 0x87654321,
        .desired_min_tx_interval = 800000,
        .required_min_rx_interval = 800000,
        .required_min_echo_rx_interval = 0,
    };

    const written = packet.encode(bird_pkt, &buf);
    try std.testing.expectEqual(@as(usize, 24), written);

    // Decode and verify all fields match
    const decoded = try packet.decode(&buf);
    try std.testing.expectEqual(@intFromEnum(packet.State.up), @intFromEnum(decoded.state));
    try std.testing.expectEqual(@intFromEnum(packet.Diagnostic.no_diagnostic), @intFromEnum(decoded.diag));
    try std.testing.expectEqual(@as(u8, 3), decoded.detect_mult);
    try std.testing.expectEqual(@as(u32, 0x12345678), decoded.my_discr);
    try std.testing.expectEqual(@as(u32, 0x87654321), decoded.your_discr);
    try std.testing.expectEqual(@as(u32, 800000), decoded.desired_min_tx_interval);
    try std.testing.expectEqual(@as(u32, 800000), decoded.required_min_rx_interval);
    try std.testing.expectEqual(@as(u32, 0), decoded.required_min_echo_rx_interval);
}

// Test that state changes are tracked correctly
test "state change tracking" {
    clock.MockClock.reset();
    const mock_clock = clock.MockClock.interface();

    const cfg = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
    };

    var sess = session.init(cfg, mock_clock);
    _ = session.processEvent(&sess, .start);
    try std.testing.expectEqual(@as(u64, 1), sess.stats.state_changes);

    // Receive Init -> state change
    const init_pkt = packet.ControlPacket{
        .state = .init,
        .my_discr = 0x1,
        .your_discr = sess.local_discr,
    };
    _ = session.processEvent(&sess, .{ .packetReceived = init_pkt });
    try std.testing.expectEqual(@as(u64, 2), sess.stats.state_changes);

    // Receive Up -> state change
    const up_pkt = packet.ControlPacket{
        .state = .up,
        .my_discr = 0x1,
        .your_discr = sess.local_discr,
    };
    _ = session.processEvent(&sess, .{ .packetReceived = up_pkt });
    try std.testing.expectEqual(@as(u64, 3), sess.stats.state_changes);

    // Receive Down -> state change
    const down_pkt = packet.ControlPacket{
        .state = .down,
        .my_discr = 0x1,
        .your_discr = sess.local_discr,
    };
    _ = session.processEvent(&sess, .{ .packetReceived = down_pkt });
    try std.testing.expectEqual(@as(u64, 4), sess.stats.state_changes);
}
