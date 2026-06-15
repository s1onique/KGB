// runtime_tests.zig — BFD runtime tests
//
// Tests for BFD multihop runtime layer.

const std = @import("std");
const packet = @import("packet.zig");
const config = @import("config.zig");
const clock = @import("clock.zig");
const session = @import("session.zig");
const transport = @import("transport.zig");
const runtime = @import("runtime.zig");

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

/// Create a test runtime with proper memory ownership.
/// Returns a TestRuntime with heap-allocated fake transport.
fn createTestRuntime() !*TestRuntime {
    clock.MockClock.reset();
    const mock_clock = clock.MockClock.interface();

    // Allocate fake transport on heap
    var fake = try std.testing.allocator.create(transport.FakeTransport);
    fake.* = transport.FakeTransport.init(&.{});
    fake.reset();

    // Allocate context that references the same fake instance
    var ctx = try std.testing.allocator.create(transport.TransportContext);
    ctx.* = transport.TransportContext.initFake(fake);

    const rt = runtime.BfdRuntime.initWithContext(
        ctx.toTransport(),
        mock_clock,
        ctx,
    );

    const result = try std.testing.allocator.create(TestRuntime);
    result.* = .{
        .rt = rt,
        .fake = fake,
        .ctx = ctx,
    };
    return result;
}

test "BfdRuntime init and peer add" {
    var result = try createTestRuntime();
    defer result.deinit();

    var rt = result.rt;

    const cfg = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 800,
        .multiplier = 3,
    };

    try rt.addPeer(cfg);
    try std.testing.expectEqual(@as(usize, 1), rt.peerCount());
    try std.testing.expect(rt.hasPeers());
}

test "BfdRuntime session starts Down" {
    var result = try createTestRuntime();
    defer result.deinit();

    var rt = result.rt;

    const cfg = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
    };

    try rt.addPeer(cfg);
    rt.startAll();

    const sess = rt.getSession("10.0.0.2").?;
    try std.testing.expectEqual(session.SessionState.down, sess.state);
}

test "BfdRuntime first tick sends Down packet" {
    var result = try createTestRuntime();
    defer result.deinit();

    var rt = result.rt;
    const fake = result.fake;
    fake.reset();

    const cfg = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 800,
        .multiplier = 3,
    };

    try rt.addPeer(cfg);
    rt.startAll();

    // Tick should trigger initial packet send
    try rt.tick();

    // Verify a packet was sent
    try std.testing.expectEqual(@as(usize, 1), fake.captured_count);
}

test "BfdRuntime receives packet and updates session" {
    var result = try createTestRuntime();
    defer result.deinit();

    var rt = result.rt;

    const cfg = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 800,
        .multiplier = 3,
    };

    try rt.addPeer(cfg);
    rt.startAll();

    // Build a BFD Init packet from peer
    var buf: [packet.CONTROL_PACKET_LEN]u8 = undefined;
    const pkt = packet.ControlPacket{
        .state = .init,
        .diag = .no_diagnostic,
        .detect_mult = 3,
        .my_discr = 0xBEEF,
        .your_discr = 0,
        .desired_min_tx_interval = 800_000,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(pkt, &buf);

    // Receive the packet
    try rt.receivePacket("10.0.0.2", &buf);

    // Session should be in Init state
    const sess = rt.getSession("10.0.0.2").?;
    try std.testing.expectEqual(session.SessionState.init, sess.state);
    try std.testing.expectEqual(@as(u32, 0xBEEF), sess.remote_discr);
}

test "BfdRuntime upCount tracks session state" {
    var result = try createTestRuntime();
    defer result.deinit();

    var rt = result.rt;

    const cfg = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 800,
        .multiplier = 3,
    };

    try rt.addPeer(cfg);
    rt.startAll();

    // Initially no peers up
    try std.testing.expectEqual(@as(usize, 0), rt.upCount());

    // Receive Init packet
    var init_buf: [packet.CONTROL_PACKET_LEN]u8 = undefined;
    const init_pkt = packet.ControlPacket{
        .state = .init,
        .my_discr = 0xBEEF,
        .your_discr = 0,
        .detect_mult = 3,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(init_pkt, &init_buf);
    try rt.receivePacket("10.0.0.2", &init_buf);

    // Still not Up (need Init + Up)
    try std.testing.expectEqual(@as(usize, 0), rt.upCount());

    // Get local discriminator for Up packet
    const sess = rt.getSession("10.0.0.2").?;
    const local_discr = sess.local_discr;

    // Receive Up packet
    var up_buf: [packet.CONTROL_PACKET_LEN]u8 = undefined;
    const up_pkt = packet.ControlPacket{
        .state = .up,
        .my_discr = 0xBEEF,
        .your_discr = local_discr,
        .detect_mult = 3,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(up_pkt, &up_buf);
    try rt.receivePacket("10.0.0.2", &up_buf);

    // Now session should be Up
    try std.testing.expectEqual(@as(usize, 1), rt.upCount());
}

test "BfdRuntime drops packets with wrong Your Discriminator" {
    var result = try createTestRuntime();
    defer result.deinit();

    var rt = result.rt;

    const cfg = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 800,
        .multiplier = 3,
    };

    try rt.addPeer(cfg);
    rt.startAll();

    // Get session for initial stats
    const sess = rt.getSession("10.0.0.2").?;
    const initial_drops = sess.stats.packets_dropped;

    // Send packet with wrong Your Discriminator
    var buf: [packet.CONTROL_PACKET_LEN]u8 = undefined;
    const pkt = packet.ControlPacket{
        .state = .up,
        .my_discr = 0xBEEF,
        .your_discr = 0xFFFFFFFF, // Wrong discriminator
        .detect_mult = 3,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(pkt, &buf);
    try rt.receivePacket("10.0.0.2", &buf);

    // Packet should be dropped
    const sess2 = rt.getSession("10.0.0.2").?;
    try std.testing.expectEqual(@as(u64, initial_drops + 1), sess2.stats.packets_dropped);
}

test "BfdRuntime peerCount returns correct value" {
    var result = try createTestRuntime();
    defer result.deinit();

    var rt = result.rt;

    // Initially no peers
    try std.testing.expectEqual(@as(usize, 0), rt.peerCount());

    // Add first peer
    const cfg1 = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
    };
    try rt.addPeer(cfg1);
    try std.testing.expectEqual(@as(usize, 1), rt.peerCount());

    // Add second peer
    const cfg2 = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.3",
    };
    try rt.addPeer(cfg2);
    try std.testing.expectEqual(@as(usize, 2), rt.peerCount());
}

test "BfdRuntime getSession returns null for unknown peer" {
    var result = try createTestRuntime();
    defer result.deinit();

    var rt = result.rt;

    const sess = rt.getSession("10.0.0.99");
    try std.testing.expect(sess == null);
}

test "BfdRuntime addPeer fails when max peers reached" {
    var result = try createTestRuntime();
    defer result.deinit();

    var rt = result.rt;

    // Add max peers (MaxPeers = 16) with unique inline addresses
    try rt.addPeer(config.BfdConfig{ .local_addr = "10.0.0.1", .peer_addr = "10.0.0.2" });
    try rt.addPeer(config.BfdConfig{ .local_addr = "10.0.0.1", .peer_addr = "10.0.0.3" });
    try rt.addPeer(config.BfdConfig{ .local_addr = "10.0.0.1", .peer_addr = "10.0.0.4" });
    try rt.addPeer(config.BfdConfig{ .local_addr = "10.0.0.1", .peer_addr = "10.0.0.5" });
    try rt.addPeer(config.BfdConfig{ .local_addr = "10.0.0.1", .peer_addr = "10.0.0.6" });
    try rt.addPeer(config.BfdConfig{ .local_addr = "10.0.0.1", .peer_addr = "10.0.0.7" });
    try rt.addPeer(config.BfdConfig{ .local_addr = "10.0.0.1", .peer_addr = "10.0.0.8" });
    try rt.addPeer(config.BfdConfig{ .local_addr = "10.0.0.1", .peer_addr = "10.0.0.9" });
    try rt.addPeer(config.BfdConfig{ .local_addr = "10.0.0.1", .peer_addr = "10.0.0.10" });
    try rt.addPeer(config.BfdConfig{ .local_addr = "10.0.0.1", .peer_addr = "10.0.0.11" });
    try rt.addPeer(config.BfdConfig{ .local_addr = "10.0.0.1", .peer_addr = "10.0.0.12" });
    try rt.addPeer(config.BfdConfig{ .local_addr = "10.0.0.1", .peer_addr = "10.0.0.13" });
    try rt.addPeer(config.BfdConfig{ .local_addr = "10.0.0.1", .peer_addr = "10.0.0.14" });
    try rt.addPeer(config.BfdConfig{ .local_addr = "10.0.0.1", .peer_addr = "10.0.0.15" });
    try rt.addPeer(config.BfdConfig{ .local_addr = "10.0.0.1", .peer_addr = "10.0.0.16" });
    try rt.addPeer(config.BfdConfig{ .local_addr = "10.0.0.1", .peer_addr = "10.0.0.17" });

    // Adding 17th peer should fail
    const cfg17 = config.BfdConfig{ .local_addr = "10.0.0.1", .peer_addr = "10.0.0.18" };
    try std.testing.expectError(runtime.RuntimeError.TooManyPeers, rt.addPeer(cfg17));
}

test "BfdRuntime session stats are tracked" {
    var result = try createTestRuntime();
    defer result.deinit();

    var rt = result.rt;

    const cfg = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 800,
        .multiplier = 3,
    };

    try rt.addPeer(cfg);
    rt.startAll();

    const sess = rt.getSession("10.0.0.2").?;

    // Initial stats should be zero
    try std.testing.expectEqual(@as(u64, 0), sess.stats.packets_sent);
    try std.testing.expectEqual(@as(u64, 0), sess.stats.packets_received);
    try std.testing.expectEqual(@as(u64, 0), sess.stats.packets_dropped);

    // Send a packet
    try rt.tick();

    // Should have sent at least one packet
    const sess2 = rt.getSession("10.0.0.2").?;
    try std.testing.expect(sess2.stats.packets_sent > 0);
}
