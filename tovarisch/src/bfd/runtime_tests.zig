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

/// Helper to create a test runtime with proper memory ownership
fn createTestRuntime() runtime.BfdRuntime {
    clock.MockClock.reset();
    const mock_clock = clock.MockClock.interface();
    var fake = transport.FakeTransport.init(&.{});
    fake.reset();
    const result = transport.makeFakeTransportInterface(fake);
    return runtime.BfdRuntime.initWithContext(result.trans, mock_clock, result.ctx);
}

test "BfdRuntime init and peer add" {
    var rt = createTestRuntime();

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
    var rt = createTestRuntime();

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
    var rt = createTestRuntime();

    const cfg = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 800,
        .multiplier = 3,
    };

    try rt.addPeer(cfg);
    rt.startAll();

    // First tick should send a packet
    try rt.tick();
}

test "BfdRuntime receiving Init advances to Init" {
    var rt = createTestRuntime();

    const cfg = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 800,
        .multiplier = 3,
    };

    try rt.addPeer(cfg);
    rt.startAll();

    const sess = rt.getSession("10.0.0.2").?;
    const local_discr = sess.local_discr;

    // Build Init packet from peer
    var init_buf: [24]u8 = undefined;
    const init_pkt = packet.ControlPacket{
        .state = .init,
        .my_discr = 0xBEEF,
        .your_discr = local_discr,
        .detect_mult = 3,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(init_pkt, &init_buf);

    try rt.receivePacket("10.0.0.2", &init_buf);
    try std.testing.expectEqual(session.SessionState.init, sess.state);
}

test "BfdRuntime receiving Up advances to Up" {
    var rt = createTestRuntime();

    const cfg = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 800,
        .multiplier = 3,
    };

    try rt.addPeer(cfg);
    rt.startAll();

    const sess = rt.getSession("10.0.0.2").?;
    const local_discr = sess.local_discr;

    // Receive Init
    var init_buf: [24]u8 = undefined;
    const init_pkt = packet.ControlPacket{
        .state = .init,
        .my_discr = 0xBEEF,
        .your_discr = local_discr,
        .detect_mult = 3,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(init_pkt, &init_buf);
    try rt.receivePacket("10.0.0.2", &init_buf);

    // Receive Up
    var up_buf: [24]u8 = undefined;
    const up_pkt = packet.ControlPacket{
        .state = .up,
        .my_discr = 0xBEEF,
        .your_discr = local_discr,
        .detect_mult = 3,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(up_pkt, &up_buf);
    try rt.receivePacket("10.0.0.2", &up_buf);

    try std.testing.expectEqual(session.SessionState.up, sess.state);
}

test "BfdRuntime detection timeout causes state Down" {
    var rt = createTestRuntime();

    const cfg = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 800,
        .multiplier = 3,
    };

    try rt.addPeer(cfg);
    rt.startAll();

    const sess = rt.getSession("10.0.0.2").?;
    const local_discr = sess.local_discr;

    // Bring session to Up
    var init_buf: [24]u8 = undefined;
    const init_pkt = packet.ControlPacket{
        .state = .init,
        .my_discr = 0xBEEF,
        .your_discr = local_discr,
        .detect_mult = 3,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(init_pkt, &init_buf);
    try rt.receivePacket("10.0.0.2", &init_buf);

    var up_buf: [24]u8 = undefined;
    const up_pkt = packet.ControlPacket{
        .state = .up,
        .my_discr = 0xBEEF,
        .your_discr = local_discr,
        .detect_mult = 3,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(up_pkt, &up_buf);
    try rt.receivePacket("10.0.0.2", &up_buf);

    try std.testing.expectEqual(session.SessionState.up, sess.state);

    // Advance time past detection timeout (2400ms)
    clock.MockClock.advance(2500);

    // Tick should process detection timeout
    try rt.tick();

    try std.testing.expectEqual(session.SessionState.down, sess.state);
}

test "BfdRuntime malformed packet returns InvalidPacket error" {
    var rt = createTestRuntime();

    const cfg = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 800,
        .multiplier = 3,
    };

    try rt.addPeer(cfg);
    rt.startAll();

    const sess = rt.getSession("10.0.0.2").?;

    // Send malformed packet (too short)
    const short_buf = [_]u8{0x20, 0x40, 0x03};
    try std.testing.expectError(runtime.RuntimeError.InvalidPacket, rt.receivePacket("10.0.0.2", &short_buf));

    // Session should be unchanged
    try std.testing.expectEqual(session.SessionState.down, sess.state);
}

test "BfdRuntime unknown peer is rejected" {
    var rt = createTestRuntime();

    const cfg = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 800,
        .multiplier = 3,
    };

    try rt.addPeer(cfg);
    rt.startAll();

    // Try to receive from unknown peer
    var buf: [24]u8 = undefined;
    try std.testing.expectError(runtime.RuntimeError.SessionNotFound, rt.receivePacket("192.168.1.1", &buf));
}

test "BfdRuntime upCount and peerCount" {
    var rt = createTestRuntime();

    const cfg = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 800,
        .multiplier = 3,
    };

    try rt.addPeer(cfg);
    rt.startAll();

    try std.testing.expectEqual(@as(usize, 1), rt.peerCount());
    try std.testing.expectEqual(@as(usize, 0), rt.upCount());

    // Bring to Up
    const sess = rt.getSession("10.0.0.2").?;
    const local_discr = sess.local_discr;

    var init_buf: [24]u8 = undefined;
    const init_pkt = packet.ControlPacket{
        .state = .init,
        .my_discr = 0xBEEF,
        .your_discr = local_discr,
        .detect_mult = 3,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(init_pkt, &init_buf);
    try rt.receivePacket("10.0.0.2", &init_buf);

    var up_buf: [24]u8 = undefined;
    const up_pkt = packet.ControlPacket{
        .state = .up,
        .my_discr = 0xBEEF,
        .your_discr = local_discr,
        .detect_mult = 3,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(up_pkt, &up_buf);
    try rt.receivePacket("10.0.0.2", &up_buf);

    try std.testing.expectEqual(@as(usize, 1), rt.peerCount());
    try std.testing.expectEqual(@as(usize, 1), rt.upCount());
}
