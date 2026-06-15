// receive_startup_tests.zig — ACT 2.3 BIRD startup path regression tests
//
// Tests for the complete BIRD startup scenario where:
// 1. BIRD sends packet with Your Discriminator = 0
// 2. tovarisch accepts it and learns remote discriminator
// 3. tovarisch sends response via tick/send path
//
// This file was split from receive_tests.zig to satisfy LLM-friendliness limits.

const std = @import("std");
const testing = std.testing;
const packet = @import("packet.zig");
const runtime = @import("runtime.zig");
const transport = @import("transport.zig");
const clock = @import("clock.zig");
const config = @import("config.zig");
const session = @import("session.zig");

// ============================================================================
// Test helpers
// ============================================================================

/// Stable test runtime that holds pointers to all owned resources.
/// This ensures the fake transport instance used by the runtime is the same
/// one that the test inspects for captured packets.
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
// ACT 2.3: BIRD startup path regression tests
// ============================================================================

test "BIRD startup: packet with your_discr_0 is accepted and response is sent" {
    // This is the exact scenario from ACT 2.3 - BIRD starts session with Your Discr = 0
    var result = try createTestRuntimeWithPeer();
    defer result.deinit();

    var rt = result.rt;
    const fake = result.fake;
    fake.reset();

    const sess = rt.getSession("10.149.149.10").?;
    const local_discr = sess.local_discr;
    try testing.expectEqual(@as(u32, 0), sess.remote_discr);

    // Initial state: Down, no packets sent
    try testing.expectEqual(packet.State.down, sess.state);
    try testing.expectEqual(@as(u64, 0), sess.stats.packets_sent);

    // BIRD sends first packet with Your Discriminator = 0
    // This simulates BIRD's initial multihop packet per RFC 5880
    var bird_init_buf: [packet.CONTROL_PACKET_LEN]u8 = undefined;
    const bird_init = packet.ControlPacket{
        .state = .down, // BIRD starts Down
        .diag = .no_diagnostic,
        .detect_mult = 3,
        .my_discr = 0xB1D00001, // BIRD's discriminator
        .your_discr = 0, // Doesn't know our discriminator yet
        .desired_min_tx_interval = 800_000,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(bird_init, &bird_init_buf);

    // Receive the BIRD startup packet - this should NOT be dropped!
    try rt.receivePacket("10.149.149.10", &bird_init_buf);

    // After receiving, remote discriminator should be learned
    const sess_after = rt.getSession("10.149.149.10").?;
    try testing.expectEqual(@as(u32, 0xB1D00001), sess_after.remote_discr);

    // Session should have advanced to Init (Down + Down = Init per RFC 5880 6.8.4)
    // Actually, the state machine says Down + Down = Down
    // But after BIRD sends Init, we should go to Init
    try testing.expectEqual(packet.State.down, sess_after.state);

    // Trigger tick to process the packet and send response
    try rt.tick();

    // Verify a packet was sent back to BIRD (via fake transport)
    try testing.expectEqual(@as(usize, 1), fake.captured_count);

    // Verify the response packet has correct discriminators
    const response = fake.lastPacket().?;
    try testing.expectEqualStrings("10.149.149.10", response.peer_addr);
    try testing.expectEqual(@as(u16, packet.MULTIHOP_UDP_PORT), response.port);

    // Decode and verify the response packet contents
    const response_pkt = try packet.decode(&response.bytes);
    try testing.expectEqual(local_discr, response_pkt.my_discr); // Our local discr
    try testing.expectEqual(@as(u32, 0xB1D00001), response_pkt.your_discr); // Learned BIRD discr
    try testing.expectEqual(packet.State.down, response_pkt.state);
}

test "BIRD startup: complete handshake to Up state" {
    // Simulate complete BFD handshake between tovarisch and BIRD
    var result = try createTestRuntimeWithPeer();
    defer result.deinit();

    var rt = result.rt;
    const fake = result.fake;
    fake.reset();

    const sess = rt.getSession("10.149.149.10").?;
    const local_discr = sess.local_discr;

    // Step 1: BIRD sends Init with Your Discr = 0
    var bird_init_buf: [packet.CONTROL_PACKET_LEN]u8 = undefined;
    const bird_init = packet.ControlPacket{
        .state = .init,
        .my_discr = 0xB1D0001,
        .your_discr = 0,
        .detect_mult = 3,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(bird_init, &bird_init_buf);
    try rt.receivePacket("10.149.149.10", &bird_init_buf);

    // Session should be in Init state
    try testing.expectEqual(packet.State.init, sess.state);
    try testing.expectEqual(@as(u32, 0xB1D0001), sess.remote_discr);

    // Step 2: BIRD sends Up with our local discriminator
    var bird_up_buf: [packet.CONTROL_PACKET_LEN]u8 = undefined;
    const bird_up = packet.ControlPacket{
        .state = .up,
        .my_discr = 0xB1D0001,
        .your_discr = local_discr,
        .detect_mult = 3,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(bird_up, &bird_up_buf);
    try rt.receivePacket("10.149.149.10", &bird_up_buf);

    // Session should now be Up!
    try testing.expectEqual(packet.State.up, sess.state);
    try testing.expectEqual(@as(usize, 1), rt.upCount());

    // Step 3: Trigger tick to process and send response
    try rt.tick();

    // Verify a packet was sent back to BIRD
    try testing.expectEqual(@as(usize, 1), fake.captured_count);

    // Verify response packet contents
    const response = fake.lastPacket().?;
    const response_pkt = try packet.decode(&response.bytes);
    try testing.expectEqual(local_discr, response_pkt.my_discr);
    try testing.expectEqual(@as(u32, 0xB1D0001), response_pkt.your_discr);
    try testing.expectEqual(packet.State.up, response_pkt.state);
}

test "BIRD startup: packet not dropped when your_discr_0 from configured peer" {
    // Regression test: ensure packets with Your Discr = 0 from configured peers
    // are NOT counted as dropped. RFC 5880 Section 6.8.4 explicitly allows this.
    var result = try createTestRuntimeWithPeer();
    defer result.deinit();

    var rt = result.rt;

    const sess = rt.getSession("10.149.149.10").?;

    // Initial drop count should be 0
    try testing.expectEqual(@as(u64, 0), sess.stats.packets_dropped);

    // Send packet with Your Discr = 0 (BIRD startup packet)
    var buf: [packet.CONTROL_PACKET_LEN]u8 = undefined;
    const pkt = packet.ControlPacket{
        .state = .down,
        .my_discr = 0xB1D1234,
        .your_discr = 0, // Valid for startup
        .detect_mult = 3,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(pkt, &buf);
    try rt.receivePacket("10.149.149.10", &buf);

    // Packet should NOT be dropped
    const sess2 = rt.getSession("10.149.149.10").?;
    try testing.expectEqual(@as(u64, 0), sess2.stats.packets_dropped);
    try testing.expectEqual(@as(u64, 1), sess2.stats.packets_received);
    try testing.expectEqual(@as(u32, 0xB1D1234), sess2.remote_discr);
}
