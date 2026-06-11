// session_bird_tests.zig — BIRD Initial Packet Regression Tests
//
// Tests for the BFD startup deadlock fix where BIRD sends initial packets
// with YourDiscr=0 (because it doesn't know our discriminator yet).
//
// Production packet that caused the deadlock:
//   - BFDv1 multihop
//   - State Down
//   - My Discriminator 0xb31d4bb8
//   - Your Discriminator 0
//   - Desired min Tx 1000ms
//   - Required min Rx 1500ms

const std = @import("std");
const packet = @import("packet.zig");
const config = @import("config.zig");
const clock = @import("clock.zig");
const session = @import("session.zig");

test "BIRD initial packet: Down, MyDiscr≠0, YourDiscr=0 learns remote_discr" {
    clock.MockClock.reset();
    clock.MockClock.setTime(0);
    const mock_clock = clock.MockClock.interface();

    const cfg = config.BfdConfig{
        .mode = .multihop,
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 1000,
        .multiplier = 3,
    };

    var sess = session.init(cfg, mock_clock);
    _ = session.processEvent(&sess, .start);
    try std.testing.expect(sess.local_discr != 0);

    // BIRD initial packet: State=Down, MyDiscr=nonzero, YourDiscr=0
    // This is the production packet that caused the deadlock:
    // - State Down
    // - My Discriminator 0xb31d4bb8 (nonzero)
    // - Your Discriminator 0
    const bird_initial_pkt = packet.ControlPacket{
        .state = .down,
        .my_discr = 0xb31d4bb8,
        .your_discr = 0, // BIRD doesn't know our discriminator yet
        .detect_mult = 3,
        .desired_min_tx_interval = 1_000_000, // 1000ms
        .required_min_rx_interval = 1_500_000, // 1500ms
    };
    _ = session.processEvent(&sess, .{ .packetReceived = bird_initial_pkt });

    // Remote discriminator should be learned from the packet
    try std.testing.expectEqual(@as(u32, 0xb31d4bb8), sess.remote_discr);
    // State should remain Down (Down + Down = Down per RFC 5880)
    try std.testing.expectEqual(session.SessionState.down, sess.state);
    // Packet should NOT be counted as dropped
    try std.testing.expectEqual(@as(u64, 0), sess.stats.packets_dropped);
}

test "BIRD initial packet enables transmit when Down + remote_discr learned" {
    clock.MockClock.reset();
    clock.MockClock.setTime(0);
    const mock_clock = clock.MockClock.interface();

    const cfg = config.BfdConfig{
        .mode = .multihop,
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 1000,
        .multiplier = 3,
    };

    var sess = session.init(cfg, mock_clock);
    _ = session.processEvent(&sess, .start);
    sess.local_discr = 1;

    // Before BIRD packet: Down state, packets_sent=0, remote_discr=0.
    // Fresh session SHOULD transmit (to initiate BFD).
    try std.testing.expect(session.isTransmitDue(&sess));

    // Receive BIRD initial packet
    const bird_pkt = packet.ControlPacket{
        .state = .down,
        .my_discr = 0xDEADBEEF,
        .your_discr = 0,
        .detect_mult = 3,
        .required_min_rx_interval = 1_500_000,
    };
    _ = session.processEvent(&sess, .{ .packetReceived = bird_pkt });

    // After BIRD packet: Down state, remote_discr learned, SHOULD transmit
    try std.testing.expectEqual(@as(u32, 0xDEADBEEF), sess.remote_discr);
    try std.testing.expect(session.isTransmitDue(&sess));
}

test "transmit packet has YourDiscr = remote MyDiscr after BIRD initial packet" {
    clock.MockClock.reset();
    clock.MockClock.setTime(0);
    const mock_clock = clock.MockClock.interface();

    const cfg = config.BfdConfig{
        .mode = .multihop,
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 1000,
        .multiplier = 3,
    };

    var sess = session.init(cfg, mock_clock);
    _ = session.processEvent(&sess, .start);
    sess.local_discr = 1;

    // Receive BIRD initial packet
    const bird_pkt = packet.ControlPacket{
        .state = .down,
        .my_discr = 0x12345678,
        .your_discr = 0,
        .detect_mult = 3,
        .required_min_rx_interval = 1_500_000,
    };
    _ = session.processEvent(&sess, .{ .packetReceived = bird_pkt });

    // Build transmit packet
    const tx_pkt = session.buildTransmitPacket(&sess);

    // YourDiscr should be set to remote's MyDiscr (0x12345678)
    try std.testing.expectEqual(@as(u32, 0x12345678), tx_pkt.your_discr);
    // MyDiscr should be our local discriminator (1)
    try std.testing.expectEqual(@as(u32, 1), tx_pkt.my_discr);
    // State should be Down in the tx packet
    try std.testing.expectEqual(session.SessionState.down, tx_pkt.state);
}

test "YourDiscr=0 is accepted even when local_discr is nonzero" {
    clock.MockClock.reset();
    const mock_clock = clock.MockClock.interface();

    const cfg = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .local_discr = 0xDEAD,
    };

    var sess = session.init(cfg, mock_clock);
    // Session has local_discr set
    try std.testing.expectEqual(@as(u32, 0xDEAD), sess.local_discr);

    // Packet with YourDiscr=0 (initial discovery) should be accepted
    const init_pkt = packet.ControlPacket{
        .state = .down,
        .my_discr = 0xBEEF,
        .your_discr = 0, // Initial discovery
        .detect_mult = 3,
    };
    _ = session.processEvent(&sess, .{ .packetReceived = init_pkt });

    // Should NOT be dropped
    try std.testing.expectEqual(@as(u64, 0), sess.stats.packets_dropped);
    // Remote discriminator should be learned
    try std.testing.expectEqual(@as(u32, 0xBEEF), sess.remote_discr);
}

test "complete BIRD handshake simulation: Down→Init→Up" {
    clock.MockClock.reset();
    clock.MockClock.setTime(0);
    const mock_clock = clock.MockClock.interface();

    const cfg = config.BfdConfig{
        .mode = .multihop,
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 1000,
        .multiplier = 3,
    };

    var sess = session.init(cfg, mock_clock);
    sess.local_discr = 1;
    _ = session.processEvent(&sess, .start);

    // Step 1: BIRD sends initial Down packet with YourDiscr=0
    const bird_down_pkt = packet.ControlPacket{
        .state = .down,
        .my_discr = 0xB31D4BB8,
        .your_discr = 0,
        .detect_mult = 3,
        .required_min_rx_interval = 1_500_000,
    };
    _ = session.processEvent(&sess, .{ .packetReceived = bird_down_pkt });
    try std.testing.expectEqual(@as(u32, 0xB31D4BB8), sess.remote_discr);
    try std.testing.expectEqual(session.SessionState.down, sess.state);
    try std.testing.expect(session.isTransmitDue(&sess));

    // Step 2: Build and verify our response packet
    const our_response = session.buildTransmitPacket(&sess);
    try std.testing.expectEqual(@as(u32, 0xB31D4BB8), our_response.your_discr);
    try std.testing.expectEqual(@as(u32, 1), our_response.my_discr);

    // Step 3: BIRD receives our response, now knows our discriminator,
    // and sends Init packet with YourDiscr=1
    const bird_init_pkt = packet.ControlPacket{
        .state = .init,
        .my_discr = 0xB31D4BB8,
        .your_discr = 1, // Now knows our local discriminator
        .detect_mult = 3,
        .required_min_rx_interval = 1_500_000,
    };
    _ = session.processEvent(&sess, .{ .packetReceived = bird_init_pkt });
    try std.testing.expectEqual(session.SessionState.init, sess.state);

    // Step 4: BIRD sends Up packet to complete handshake
    const bird_up_pkt = packet.ControlPacket{
        .state = .up,
        .my_discr = 0xB31D4BB8,
        .your_discr = 1,
        .detect_mult = 3,
        .required_min_rx_interval = 1_500_000,
    };
    _ = session.processEvent(&sess, .{ .packetReceived = bird_up_pkt });
    try std.testing.expectEqual(session.SessionState.up, sess.state);
}
