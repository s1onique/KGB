// receive_tests.zig — Tests for BFD UDP receive socket.
// Uses FakeTransport to test the receive logic without actual network.
// NOTE: These tests use the packet decoder and runtime integration, not the actual UDP socket.

const std = @import("std");
const testing = std.testing;
const packet = @import("packet.zig");
const runtime = @import("runtime.zig");
const transport = @import("transport.zig");
const clock = @import("clock.zig");
const config = @import("config.zig");
const receive = @import("receive.zig");
const session = @import("session.zig");

// ============================================================================
// Test helpers
// ============================================================================

/// Owned test runtime helper for proper memory ownership.
/// This ensures the fake transport instance used by the runtime is heap-allocated
/// and stable across function boundaries.
const TestRuntime = struct {
    rt: runtime.BfdRuntime,
    fake: *transport.FakeTransport,
    ctx: *transport.TransportContext,

    fn deinit(self: *TestRuntime) void {
        std.testing.allocator.destroy(self.ctx);
        std.testing.allocator.destroy(self.fake);
        std.testing.allocator.destroy(self);
    }
};

/// Create a test runtime with one peer configured.
/// Returns a TestRuntime with stable pointers so that fake.reset() and
/// fake.captured_count in tests observe the same instance the runtime uses.
fn createTestRuntimeWithPeer() !*TestRuntime {
    clock.MockClock.reset();
    const mock_clock = clock.MockClock.interface();

    // Allocate fake transport on heap so we have a stable pointer
    var fake = try std.testing.allocator.create(transport.FakeTransport);
    fake.* = transport.FakeTransport.init(&.{ "10.149.149.10" });
    fake.reset();

    // Allocate context that references the same fake instance
    var ctx = try std.testing.allocator.create(transport.TransportContext);
    ctx.* = transport.TransportContext.initFake(fake);

    var rt = runtime.BfdRuntime.initWithContext(
        ctx.toTransport(),
        mock_clock,
        ctx,
    );

    const cfg = config.BfdConfig{
        .mode = .multihop,
        .local_addr = "10.149.149.1",
        .peer_addr = "10.149.149.10",
        .interval_ms = 800,
        .multiplier = 3,
    };
    try rt.addPeer(cfg);
    rt.startAll();

    const result = try std.testing.allocator.create(TestRuntime);
    result.* = .{
        .rt = rt,
        .fake = fake,
        .ctx = ctx,
    };
    return result;
}

// ============================================================================
// Packet decode tests
// ============================================================================

test "packet decode extracts Your Discriminator" {
    // Simulate a BIRD packet with Your Discriminator = 0 (new session)
    // Format: version(3) + diag(5) | state(2) + flags(6) | detect_mult | length | my_discr | your_discr | ...
    var raw: [packet.CONTROL_PACKET_LEN]u8 = undefined;
    raw[0] = 0x20; // Version 1, diag 0
    raw[1] = 0x40; // State Down (1), flags 0
    raw[2] = 3; // detect_mult
    raw[3] = packet.CONTROL_PACKET_LEN; // length
    std.mem.writeInt(u32, raw[4..8], 0xDEADBEEF, .big); // my_discr = 0xDEADBEEF (BIRD's discriminator)
    std.mem.writeInt(u32, raw[8..12], 0, .big); // your_discr = 0 (we don't know our discriminator yet)
    std.mem.writeInt(u32, raw[12..16], 800_000, .big); // desired_min_tx_interval
    std.mem.writeInt(u32, raw[16..20], 800_000, .big); // required_min_rx_interval
    std.mem.writeInt(u32, raw[20..24], 0, .big); // required_min_echo_rx_interval

    const pkt = try packet.decode(&raw);

    try testing.expectEqual(@as(u32, 0xDEADBEEF), pkt.my_discr);
    try testing.expectEqual(@as(u32, 0), pkt.your_discr);
    try testing.expectEqual(packet.State.down, pkt.state);
}

// ============================================================================
// Runtime packet feed tests
// ============================================================================

test "runtime receives packet and learns peer discriminator" {
    var result = try createTestRuntimeWithPeer();
    defer result.deinit();

    var rt = result.rt;

    // Get the session and check initial state
    const sess = rt.getSession("10.149.149.10").?;
    try testing.expectEqual(@as(u32, 0), sess.remote_discr);
    try testing.expectEqual(packet.State.down, sess.state);

    // Simulate BIRD packet with Your Discriminator = 0
    var buf: [packet.CONTROL_PACKET_LEN]u8 = undefined;

    const pkt = packet.ControlPacket{
        .state = .init,
        .diag = .no_diagnostic,
        .detect_mult = 3,
        .my_discr = 0xBEEF, // BIRD's discriminator
        .your_discr = 0, // We don't know our discriminator yet
        .desired_min_tx_interval = 800_000,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(pkt, &buf);

    try rt.receivePacket("10.149.149.10", &buf);

    // After receiving the packet, peer discriminator should be learned
    const sess2 = rt.getSession("10.149.149.10").?;
    try testing.expectEqual(@as(u32, 0xBEEF), sess2.remote_discr);
    try testing.expectEqual(packet.State.init, sess2.state);
}

test "runtime packet feed updates session state to Up" {
    var result = try createTestRuntimeWithPeer();
    defer result.deinit();

    var rt = result.rt;

    const sess = rt.getSession("10.149.149.10").?;
    const local_discr = sess.local_discr;

    // Send Init packet
    var init_buf: [packet.CONTROL_PACKET_LEN]u8 = undefined;
    const init_pkt = packet.ControlPacket{
        .state = .init,
        .diag = .no_diagnostic,
        .detect_mult = 3,
        .my_discr = 0xBEEF,
        .your_discr = 0, // New session
        .desired_min_tx_interval = 800_000,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(init_pkt, &init_buf);
    try rt.receivePacket("10.149.149.10", &init_buf);

    // Verify we're in Init state
    {
        const s = rt.getSession("10.149.149.10").?;
        try testing.expectEqual(packet.State.init, s.state);
    }

    // Send Up packet
    var up_buf: [packet.CONTROL_PACKET_LEN]u8 = undefined;
    const up_pkt = packet.ControlPacket{
        .state = .up,
        .diag = .no_diagnostic,
        .detect_mult = 3,
        .my_discr = 0xBEEF,
        .your_discr = local_discr, // Now we know our discriminator
        .desired_min_tx_interval = 800_000,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(up_pkt, &up_buf);
    try rt.receivePacket("10.149.149.10", &up_buf);

    // Verify we're in Up state
    const sess2 = rt.getSession("10.149.149.10").?;
    try testing.expectEqual(packet.State.up, sess2.state);
    try testing.expectEqual(@as(usize, 1), rt.upCount());
}

test "runtime drops packets with wrong Your Discriminator" {
    var result = try createTestRuntimeWithPeer();
    defer result.deinit();

    var rt = result.rt;

    // Get initial stats
    const sess = rt.getSession("10.149.149.10").?;
    const initial_drops = sess.stats.packets_dropped;
    const initial_rcvd = sess.stats.packets_received;

    // Send packet with wrong Your Discriminator (not our local_discr)
    var buf: [packet.CONTROL_PACKET_LEN]u8 = undefined;
    const wrong_pkt = packet.ControlPacket{
        .state = .up,
        .diag = .no_diagnostic,
        .detect_mult = 3,
        .my_discr = 0xBEEF,
        .your_discr = 0xFFFFFFFF, // Wrong discriminator
        .desired_min_tx_interval = 800_000,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(wrong_pkt, &buf);
    try rt.receivePacket("10.149.149.10", &buf);

    // Packet should be dropped
    const sess2 = rt.getSession("10.149.149.10").?;
    try testing.expectEqual(@as(u64, initial_drops + 1), sess2.stats.packets_dropped);
    try testing.expectEqual(@as(u64, initial_rcvd + 1), sess2.stats.packets_received); // Still counted as received
}

// ============================================================================
// Transmit path tests (verify runtime sends responses)
// ============================================================================

test "runtime builds correct transmit packet" {
    var result = try createTestRuntimeWithPeer();
    defer result.deinit();

    var rt = result.rt;

    // Start session and receive Init from BIRD
    const sess = rt.getSession("10.149.149.10").?;
    const local_discr = sess.local_discr;

    // Send Init packet from BIRD
    var init_buf: [packet.CONTROL_PACKET_LEN]u8 = undefined;
    const init_pkt = packet.ControlPacket{
        .state = .init,
        .my_discr = 0xBEEF,
        .your_discr = 0, // BIRD doesn't know our discriminator yet
        .detect_mult = 3,
        .desired_min_tx_interval = 800_000,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(init_pkt, &init_buf);
    try rt.receivePacket("10.149.149.10", &init_buf);

    // Now receive Up packet from BIRD (this should transition us to Up)
    var up_buf: [packet.CONTROL_PACKET_LEN]u8 = undefined;
    const up_pkt = packet.ControlPacket{
        .state = .up,
        .my_discr = 0xBEEF,
        .your_discr = local_discr, // BIRD learned our discriminator
        .detect_mult = 3,
        .desired_min_tx_interval = 800_000,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(up_pkt, &up_buf);
    try rt.receivePacket("10.149.149.10", &up_buf);

    // Session should now be Up
    const sess2 = rt.getSession("10.149.149.10").?;
    try testing.expectEqual(packet.State.up, sess2.state);
    try testing.expectEqual(@as(u32, 0xBEEF), sess2.remote_discr);

    // Verify the transmit packet would have correct discriminators
    const tx_pkt = session.buildTransmitPacket(sess2);
    try testing.expectEqual(local_discr, tx_pkt.my_discr);
    try testing.expectEqual(@as(u32, 0xBEEF), tx_pkt.your_discr);
}

test "session packet stats are tracked correctly" {
    var result = try createTestRuntimeWithPeer();
    defer result.deinit();

    var rt = result.rt;

    const sess = rt.getSession("10.149.149.10").?;

    // Initially no packets
    try testing.expectEqual(@as(u64, 0), sess.stats.packets_sent);
    try testing.expectEqual(@as(u64, 0), sess.stats.packets_received);
    try testing.expectEqual(@as(u64, 0), sess.stats.packets_dropped);

    // Send Init packet
    var buf: [packet.CONTROL_PACKET_LEN]u8 = undefined;
    const pkt = packet.ControlPacket{
        .state = .init,
        .my_discr = 0xBEEF,
        .your_discr = 0,
        .detect_mult = 3,
        .desired_min_tx_interval = 800_000,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(pkt, &buf);
    try rt.receivePacket("10.149.149.10", &buf);

    const sess2 = rt.getSession("10.149.149.10").?;
    try testing.expectEqual(@as(u64, 1), sess2.stats.packets_received);
    try testing.expectEqual(@as(u64, 0), sess2.stats.packets_dropped);
}

// ============================================================================
// Status integration tests
// ============================================================================

test "runtime upCount reflects session state" {
    var result = try createTestRuntimeWithPeer();
    defer result.deinit();

    var rt = result.rt;

    // Initially session is Down
    try testing.expectEqual(@as(usize, 0), rt.upCount());
    try testing.expectEqual(@as(usize, 1), rt.peerCount());

    // Bring session Up
    const sess = rt.getSession("10.149.149.10").?;
    const local_discr = sess.local_discr;

    // Init packet
    var init_buf: [packet.CONTROL_PACKET_LEN]u8 = undefined;
    const init_pkt = packet.ControlPacket{
        .state = .init,
        .my_discr = 0xBEEF,
        .your_discr = 0,
        .detect_mult = 3,
        .desired_min_tx_interval = 800_000,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(init_pkt, &init_buf);
    try rt.receivePacket("10.149.149.10", &init_buf);

    // Up packet
    var up_buf: [packet.CONTROL_PACKET_LEN]u8 = undefined;
    const up_pkt = packet.ControlPacket{
        .state = .up,
        .my_discr = 0xBEEF,
        .your_discr = local_discr,
        .detect_mult = 3,
        .desired_min_tx_interval = 800_000,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(up_pkt, &up_buf);
    try rt.receivePacket("10.149.149.10", &up_buf);

    // Now session should be Up
    try testing.expectEqual(@as(usize, 1), rt.upCount());
}

// ============================================================================
// Discriminator learning tests
// ============================================================================

test "session learns remote discriminator from Your_Discr_0 packet" {
    var result = try createTestRuntimeWithPeer();
    defer result.deinit();

    var rt = result.rt;

    const sess = rt.getSession("10.149.149.10").?;
    try testing.expectEqual(@as(u32, 0), sess.remote_discr);

    // BIRD sends first packet with Your Discriminator = 0
    // and includes its discriminator in My Discriminator field
    var buf: [packet.CONTROL_PACKET_LEN]u8 = undefined;
    const pkt = packet.ControlPacket{
        .state = .down,
        .my_discr = 0xCAFE, // BIRD's discriminator
        .your_discr = 0, // Unknown (session new)
        .detect_mult = 3,
        .desired_min_tx_interval = 800_000,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(pkt, &buf);
    try rt.receivePacket("10.149.149.10", &buf);

    // Session should have learned BIRD's discriminator
    const sess2 = rt.getSession("10.149.149.10").?;
    try testing.expectEqual(@as(u32, 0xCAFE), sess2.remote_discr);
}

test "session calculates detection timeout from peer packet" {
    var result = try createTestRuntimeWithPeer();
    defer result.deinit();

    var rt = result.rt;

    const sess = rt.getSession("10.149.149.10").?;
    // Default detection timeout based on config
    try testing.expectEqual(@as(u32, 2400), sess.detection_timeout_ms); // 800ms * 3

    // Send packet with specific RX interval
    var buf: [packet.CONTROL_PACKET_LEN]u8 = undefined;
    const pkt = packet.ControlPacket{
        .state = .init,
        .my_discr = 0xCAFE,
        .your_discr = 0,
        .detect_mult = 5, // Override multiplier
        .desired_min_tx_interval = 500_000, // 500ms
        .required_min_rx_interval = 500_000,
    };
    _ = packet.encode(pkt, &buf);
    try rt.receivePacket("10.149.149.10", &buf);

    // Detection timeout should be updated
    const sess2 = rt.getSession("10.149.149.10").?;
    try testing.expectEqual(@as(u32, 2500), sess2.detection_timeout_ms); // 500ms * 5
}

// ============================================================================
// BFD receive loop EAGAIN bounding tests
// ============================================================================

test "DEFAULT_POLL_TIMEOUT_MS is bounded (not zero)" {
    // Regression test: ensure the poll timeout is not zero to prevent CPU spin.
    // A timeout of 0 would cause busy-spin on EAGAIN.
    // The default should be a small positive value (e.g., 50ms).
    try testing.expect(receive.DEFAULT_POLL_TIMEOUT_MS > 0);
}

test "poll timeout constant is reasonable for BFD" {
    // BFD typically operates at 800ms+ intervals.
    // A 50ms poll timeout = 20 polls/second max, which is 1/16th of min BFD interval.
    // This is responsive enough without causing CPU spin.
    const timeout_ms = receive.DEFAULT_POLL_TIMEOUT_MS;
    try testing.expect(timeout_ms >= 10); // At least 10ms for any responsiveness
    try testing.expect(timeout_ms <= 100); // At most 100ms (10 polls/second) for BFD
}


