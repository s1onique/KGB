// status.zig — BFD status reporting for tovarisch
//
// Provides BFD status check that integrates with the main status system.
// Reports per-peer state and overall BFD health.

const std = @import("std");
const packet = @import("packet.zig");
const session = @import("session.zig");
const runtime = @import("runtime.zig");
const transport = @import("transport.zig");
const clock = @import("clock.zig");
const config = @import("config.zig");

/// Re-export for external use
pub const BfdRuntime = runtime.BfdRuntime;
pub const PeerStatus = runtime.PeerStatus;
pub const RuntimeError = runtime.RuntimeError;

/// BFD status check result
pub const StatusCheck = struct {
    name: []const u8,
    status: CheckStatus,
    detail: []const u8,
};

/// BFD check status enum
pub const CheckStatus = enum {
    ok,
    warn,
    @"error",
    unknown,
};

/// Module-level runtime instance for status reporting.
/// This is the runtime that status checks query.
var module_runtime: ?*BfdRuntime = null;

/// Initialize the BFD runtime for status reporting.
/// Must be called before getStatusCheck() returns runtime-aware status.
pub fn setRuntime(rt: *BfdRuntime) void {
    module_runtime = rt;
}

/// Clear the BFD runtime for status reporting.
/// Use this to reset state between tests.
pub fn clearRuntime() void {
    module_runtime = null;
}

/// Get the current BFD status check.
/// Uses module_runtime if available, otherwise returns "not configured".
pub fn getStatusCheck() StatusCheck {
    const rt = module_runtime orelse {
        return .{
            .name = "bfd",
            .status = .warn,
            .detail = "bfd not configured",
        };
    };

    if (!rt.hasPeers()) {
        return .{
            .name = "bfd",
            .status = .warn,
            .detail = "bfd not configured",
        };
    }

    const total = rt.peerCount();
    const up = rt.upCount();

    if (up == total) {
        return .{
            .name = "bfd",
            .status = .ok,
            .detail = "bfd sessions up",
        };
    }

    // Some peers up, some not
    const detail = std.fmt.allocPrint(std.heap.page_allocator, "{d}/{d} bfd sessions up", .{up, total}) 
        catch return .{
            .name = "bfd",
            .status = .warn,
            .detail = "bfd partially up",
        };

    return .{
        .name = "bfd",
        .status = .warn,
        .detail = detail,
    };
}

/// Create a fresh runtime for testing.
pub fn createTestRuntime() BfdRuntime {
    clock.MockClock.reset();
    const mock_clock = clock.MockClock.interface();
    var fake = transport.FakeTransport.init(&.{});
    fake.reset();
    const result = transport.makeFakeTransportInterface(fake);
    return BfdRuntime.initWithContext(result.trans, mock_clock, result.ctx);
}

/// Add a test peer with default BIRD-style config.
pub fn addTestPeer(rt: *BfdRuntime, local_addr: []const u8, peer_addr: []const u8) !void {
    const cfg = config.BfdConfig{
        .local_addr = local_addr,
        .peer_addr = peer_addr,
        .interval_ms = 800,
        .multiplier = 3,
    };
    try rt.addPeer(cfg);
}

test "getStatusCheck returns warn when no runtime" {
    module_runtime = null;
    const check = getStatusCheck();
    try std.testing.expectEqualStrings("bfd", check.name);
    try std.testing.expectEqualStrings("bfd not configured", check.detail);
}

test "getStatusCheck returns warn when no peers" {
    var rt = createTestRuntime();
    setRuntime(&rt);
    
    const check = getStatusCheck();
    try std.testing.expectEqualStrings("bfd not configured", check.detail);
}

test "getStatusCheck returns ok when all peers up" {
    var rt = createTestRuntime();
    try addTestPeer(&rt, "10.0.0.1", "10.0.0.2");
    rt.startAll();
    setRuntime(&rt);

    // Bring to Up state
    const sess = rt.getSession("10.0.0.2").?;
    const local_discr = sess.local_discr;

    var init_buf: [24]u8 = undefined;
    const init_pkt = session.ControlPacket{
        .state = .init,
        .my_discr = 0xBEEF,
        .your_discr = local_discr,
        .detect_mult = 3,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(init_pkt, &init_buf);
    try rt.receivePacket("10.0.0.2", &init_buf);

    var up_buf: [24]u8 = undefined;
    const up_pkt = session.ControlPacket{
        .state = .up,
        .my_discr = 0xBEEF,
        .your_discr = local_discr,
        .detect_mult = 3,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(up_pkt, &up_buf);
    try rt.receivePacket("10.0.0.2", &up_buf);

    const check = getStatusCheck();
    try std.testing.expectEqualStrings("bfd sessions up", check.detail);
}

test "getStatusCheck returns warn when some peers down" {
    var rt = createTestRuntime();
    try addTestPeer(&rt, "10.0.0.1", "10.0.0.2");
    try addTestPeer(&rt, "10.0.0.1", "10.0.0.3");
    rt.startAll();
    setRuntime(&rt);

    // Bring first peer to Up
    const sess1 = rt.getSession("10.0.0.2").?;
    const local_discr = sess1.local_discr;

    var init_buf: [24]u8 = undefined;
    const init_pkt = session.ControlPacket{
        .state = .init,
        .my_discr = 0xBEEF,
        .your_discr = local_discr,
        .detect_mult = 3,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(init_pkt, &init_buf);
    try rt.receivePacket("10.0.0.2", &init_buf);

    var up_buf: [24]u8 = undefined;
    const up_pkt = session.ControlPacket{
        .state = .up,
        .my_discr = 0xBEEF,
        .your_discr = local_discr,
        .detect_mult = 3,
        .required_min_rx_interval = 800_000,
    };
    _ = packet.encode(up_pkt, &up_buf);
    try rt.receivePacket("10.0.0.2", &up_buf);

    // Second peer stays Down
    const check = getStatusCheck();
    try std.testing.expect(std.mem.indexOf(u8, check.detail, "1/2 bfd sessions up") != null);
}

test "createTestRuntime and addTestPeer work together" {
    var rt = createTestRuntime();
    try addTestPeer(&rt, "10.0.0.1", "10.0.0.2");
    
    try std.testing.expect(rt.hasPeers());
    try std.testing.expectEqual(@as(usize, 1), rt.peerCount());
}
