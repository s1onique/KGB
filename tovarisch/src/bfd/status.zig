// status.zig — BFD status reporting for tovarisch
//
// Provides BFD status snapshot derivation for integration with the main status system.
// The BFD status module is stateless: it transforms an optional runtime pointer
// into a contract-valid snapshot. Daemon owns the runtime, not the status module.

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

/// BFD status snapshot containing the current state derived from runtime.
/// This is the canonical representation for status reporting.
pub const StatusSnapshot = struct {
    /// Number of configured peers
    peer_count: usize,
    /// Number of peers in Up state
    up_count: usize,
    /// Whether runtime has any peers configured
    has_peers: bool,
};

/// Derive a status snapshot from an optional BFD runtime.
/// Returns empty/no-runtime fallback snapshot when runtime is null.
/// This is the key invariant: no-runtime/no-peers remains a valid status, not an error.
pub fn snapshotFromRuntime(rt: ?*const BfdRuntime) StatusSnapshot {
    if (rt == null or !rt.?.hasPeers()) {
        return .{
            .peer_count = 0,
            .up_count = 0,
            .has_peers = false,
        };
    }

    return .{
        .peer_count = rt.?.peerCount(),
        .up_count = rt.?.upCount(),
        .has_peers = rt.?.hasPeers(),
    };
}

/// Maximum buffer size for BFD detail formatting.
/// Format: "9999/9999 bfd sessions up" = 24 chars max.
pub const BFD_DETAIL_BUF_SIZE: usize = 64;

/// Build a StatusCheck from a StatusSnapshot using caller-provided buffer.
/// This is the allocation-free variant - callers pass a buffer for dynamic details.
/// 
/// The returned StatusCheck.detail points to either:
/// - A static string for non-partial cases
/// - The caller's buffer for partial BFD cases
pub fn buildStatusCheckInto(
    snapshot: StatusSnapshot,
    detail_buf: *[BFD_DETAIL_BUF_SIZE]u8,
) StatusCheck {
    if (!snapshot.has_peers or snapshot.peer_count == 0) {
        return .{
            .name = "bfd",
            .status = .warn,
            .detail = "bfd not configured",
        };
    }

    if (snapshot.up_count == snapshot.peer_count) {
        return .{
            .name = "bfd",
            .status = .ok,
            .detail = "bfd sessions up",
        };
    }

    // Some peers up, some not - format into caller's buffer (no allocation)
    const detail = std.fmt.bufPrint(
        detail_buf,
        "{d}/{d} bfd sessions up",
        .{ snapshot.up_count, snapshot.peer_count },
    ) catch {
        // Buffer too small (should never happen with 64 bytes)
        return .{
            .name = "bfd",
            .status = .warn,
            .detail = "bfd partially up",
        };
    };

    return .{
        .name = "bfd",
        .status = .warn,
        .detail = detail,
    };
}

/// Owned test runtime helper for proper memory ownership.
/// This ensures the fake transport instance used by the runtime is heap-allocated
/// and stable across function boundaries.
const TestRuntime = struct {
    rt: BfdRuntime,
    fake: *transport.FakeTransport,
    ctx: *transport.TransportContext,

    pub fn deinit(self: *TestRuntime) void {
        std.testing.allocator.destroy(self.ctx);
        std.testing.allocator.destroy(self.fake);
        std.testing.allocator.destroy(self);
    }
};

/// Create a fresh runtime for testing with proper memory ownership.
pub fn createTestRuntime() !*TestRuntime {
    clock.MockClock.reset();
    const mock_clock = clock.MockClock.interface();

    // Allocate fake transport on heap
    var fake = try std.testing.allocator.create(transport.FakeTransport);
    fake.* = transport.FakeTransport.init(&.{});
    fake.reset();

    // Allocate context that references the same fake instance
    var ctx = try std.testing.allocator.create(transport.TransportContext);
    ctx.* = transport.TransportContext.initFake(fake);

    const rt = BfdRuntime.initWithContext(
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

// --- Tests ---

test "snapshotFromRuntime returns empty when runtime is null" {
    const snapshot = snapshotFromRuntime(null);
    try std.testing.expectEqual(@as(usize, 0), snapshot.peer_count);
    try std.testing.expectEqual(@as(usize, 0), snapshot.up_count);
    try std.testing.expect(!snapshot.has_peers);
}

test "snapshotFromRuntime returns empty when runtime has no peers" {
    var result = try createTestRuntime();
    defer result.deinit();

    const snapshot = snapshotFromRuntime(&result.rt);
    try std.testing.expectEqual(@as(usize, 0), snapshot.peer_count);
    try std.testing.expectEqual(@as(usize, 0), snapshot.up_count);
    try std.testing.expect(!snapshot.has_peers);
}

test "snapshotFromRuntime returns correct counts with peers" {
    var result = try createTestRuntime();
    defer result.deinit();

    var rt = result.rt;
    try addTestPeer(&rt, "10.0.0.1", "10.0.0.2");
    rt.startAll();

    // Bring peer to Up state
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

    const snapshot = snapshotFromRuntime(&rt);
    try std.testing.expectEqual(@as(usize, 1), snapshot.peer_count);
    try std.testing.expectEqual(@as(usize, 1), snapshot.up_count);
    try std.testing.expect(snapshot.has_peers);
}

test "buildStatusCheckInto returns warn for no peers" {
    const snapshot: StatusSnapshot = .{
        .peer_count = 0,
        .up_count = 0,
        .has_peers = false,
    };
    var detail_buf: [BFD_DETAIL_BUF_SIZE]u8 = undefined;
    const check = buildStatusCheckInto(snapshot, &detail_buf);
    try std.testing.expectEqualStrings("bfd", check.name);
    try std.testing.expect(check.status == .warn);
    try std.testing.expectEqualStrings("bfd not configured", check.detail);
}

test "buildStatusCheckInto returns ok when all peers up" {
    const snapshot: StatusSnapshot = .{
        .peer_count = 2,
        .up_count = 2,
        .has_peers = true,
    };
    var detail_buf: [BFD_DETAIL_BUF_SIZE]u8 = undefined;
    const check = buildStatusCheckInto(snapshot, &detail_buf);
    try std.testing.expectEqualStrings("bfd", check.name);
    try std.testing.expect(check.status == .ok);
    try std.testing.expectEqualStrings("bfd sessions up", check.detail);
}

test "buildStatusCheckInto returns warn when some peers down" {
    const snapshot: StatusSnapshot = .{
        .peer_count = 2,
        .up_count = 1,
        .has_peers = true,
    };
    var detail_buf: [BFD_DETAIL_BUF_SIZE]u8 = undefined;
    const check = buildStatusCheckInto(snapshot, &detail_buf);
    try std.testing.expectEqualStrings("bfd", check.name);
    try std.testing.expect(check.status == .warn);
    try std.testing.expect(std.mem.indexOf(u8, check.detail, "1/2 bfd sessions up") != null);
}

test "createTestRuntime and addTestPeer work together" {
    var result = try createTestRuntime();
    defer result.deinit();

    var rt = result.rt;
    try addTestPeer(&rt, "10.0.0.1", "10.0.0.2");
    
    try std.testing.expect(rt.hasPeers());
    try std.testing.expectEqual(@as(usize, 1), rt.peerCount());
}
