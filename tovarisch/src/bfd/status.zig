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

/// Build a StatusCheck from a StatusSnapshot.
/// This function is pure and stateless - it only transforms input.
pub fn buildStatusCheck(snapshot: StatusSnapshot) StatusCheck {
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

    // Some peers up, some not
    const detail = std.fmt.allocPrint(std.heap.page_allocator, "{d}/{d} bfd sessions up", .{
        snapshot.up_count,
        snapshot.peer_count,
    }) catch return .{
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

// --- Tests ---

test "snapshotFromRuntime returns empty when runtime is null" {
    const snapshot = snapshotFromRuntime(null);
    try std.testing.expectEqual(@as(usize, 0), snapshot.peer_count);
    try std.testing.expectEqual(@as(usize, 0), snapshot.up_count);
    try std.testing.expect(!snapshot.has_peers);
}

test "snapshotFromRuntime returns empty when runtime has no peers" {
    var rt = createTestRuntime();
    const snapshot = snapshotFromRuntime(&rt);
    try std.testing.expectEqual(@as(usize, 0), snapshot.peer_count);
    try std.testing.expectEqual(@as(usize, 0), snapshot.up_count);
    try std.testing.expect(!snapshot.has_peers);
}

test "snapshotFromRuntime returns correct counts with peers" {
    var rt = createTestRuntime();
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

test "buildStatusCheck returns warn for no peers" {
    const snapshot: StatusSnapshot = .{
        .peer_count = 0,
        .up_count = 0,
        .has_peers = false,
    };
    const check = buildStatusCheck(snapshot);
    try std.testing.expectEqualStrings("bfd", check.name);
    try std.testing.expect(check.status == .warn);
    try std.testing.expectEqualStrings("bfd not configured", check.detail);
}

test "buildStatusCheck returns ok when all peers up" {
    const snapshot: StatusSnapshot = .{
        .peer_count = 2,
        .up_count = 2,
        .has_peers = true,
    };
    const check = buildStatusCheck(snapshot);
    try std.testing.expectEqualStrings("bfd", check.name);
    try std.testing.expect(check.status == .ok);
    try std.testing.expectEqualStrings("bfd sessions up", check.detail);
}

test "buildStatusCheck returns warn when some peers down" {
    const snapshot: StatusSnapshot = .{
        .peer_count = 2,
        .up_count = 1,
        .has_peers = true,
    };
    const check = buildStatusCheck(snapshot);
    try std.testing.expectEqualStrings("bfd", check.name);
    try std.testing.expect(check.status == .warn);
    try std.testing.expect(std.mem.indexOf(u8, check.detail, "1/2 bfd sessions up") != null);
}

test "createTestRuntime and addTestPeer work together" {
    var rt = createTestRuntime();
    try addTestPeer(&rt, "10.0.0.1", "10.0.0.2");
    
    try std.testing.expect(rt.hasPeers());
    try std.testing.expectEqual(@as(usize, 1), rt.peerCount());
}
