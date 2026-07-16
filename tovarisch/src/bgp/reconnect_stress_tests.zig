// bgp/reconnect_stress_tests.zig — Test infrastructure for bounded memory reconnect ACT
//
// Provides test data types used by future reconnect-proof tests.
// The actual 10,000-generation stress harness requires integration with the
// real session closeForReconnect lifecycle (separate ACT).

const std = @import("std");
const session = @import("session.zig");
const types = @import("types.zig");
const clock = @import("clock.zig");
const allocation_tracker = @import("../runtime/allocation_tracker.zig");

/// Number of reconnect generations for stress test (declared here for tests below)
pub const STRESS_TEST_GENERATIONS: u64 = 10_000;

/// Mock clock for deterministic testing
pub const MockClockForStress = struct {
    current_time_ms: u64 = 0,

    pub fn getMonoTimeMs(self: *const MockClockForStress) u64 {
        return self.current_time_ms;
    }

    pub fn advance(self: *MockClockForStress, delta_ms: u64) void {
        self.current_time_ms += delta_ms;
    }
};

/// A fake transport that always fails on send/recv.
pub const AlwaysFailingTransport = struct {
    closed: bool = false,
    send_count: u64 = 0,
    recv_count: u64 = 0,

    pub fn reset(self: *AlwaysFailingTransport) void {
        self.closed = false;
        self.send_count = 0;
        self.recv_count = 0;
    }

    pub fn send(self: *AlwaysFailingTransport, data: []const u8) session.TransportError!void {
        _ = data;
        self.send_count += 1;
        return session.TransportError.ConnectionReset;
    }

    pub fn recv(self: *AlwaysFailingTransport) []const u8 {
        self.recv_count += 1;
        return &[_]u8{};
    }

    pub fn close(self: *AlwaysFailingTransport) void {
        self.closed = true;
    }

    pub fn toTransport(self: *AlwaysFailingTransport) session.Transport {
        return session.Transport{
            .sendFn = struct {
                fn send(ctx: *anyopaque, data: []const u8) session.TransportError!void {
                    const fake: *AlwaysFailingTransport = @ptrCast(@alignCast(ctx));
                    return fake.send(data);
                }
            }.send,
            .recvFn = struct {
                fn recv(ctx: *anyopaque) []const u8 {
                    const fake: *AlwaysFailingTransport = @ptrCast(@alignCast(ctx));
                    return fake.recv();
                }
            }.recv,
            .closeFn = struct {
                fn close(ctx: *anyopaque) void {
                    const fake: *AlwaysFailingTransport = @ptrCast(@alignCast(ctx));
                    fake.close();
                }
            }.close,
            .isClosedFn = struct {
                fn isClosed(ctx: *anyopaque) bool {
                    const fake: *AlwaysFailingTransport = @ptrCast(@alignCast(ctx));
                    return fake.closed;
                }
            }.isClosed,
            .ctx = @ptrCast(self),
        };
    }
};

/// Creates a minimal session config for testing.
pub fn makeTestSessionConfig() session.SessionConfig {
    return .{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = .{ 127, 0, 0, 1 },
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{types.Ipv4Prefix.init("10.0.0.0/8")},
        .same_as = true,
    };
}

/// Results from the reconnect stress test.
pub const ReconnectStressResults = struct {
    /// Total generations executed
    generations: u64,
    /// Baseline live bytes after warmup
    baseline_live_bytes: u64,
    /// Final live bytes after all generations
    final_live_bytes: u64,
    /// Peak live bytes observed
    peak_live_bytes: u64,
    /// Peak active timers observed
    peak_active_timers: u32,
    /// Peak active sockets observed
    peak_active_sockets: u32,
    /// Total reconnect count
    total_reconnect_count: u64,
    /// Whether allocations leaked
    allocations_leaked: bool,
    /// Whether resources returned to baseline
    resources_at_baseline: bool,
};

// ============================================================================
// Tests for the test infrastructure itself (no production lifecycle)
// ============================================================================

test "MockClockForStress advances time deterministically" {
    var mock_clock = MockClockForStress{};
    try std.testing.expectEqual(@as(u64, 0), mock_clock.current_time_ms);

    mock_clock.advance(1000);
    try std.testing.expectEqual(@as(u64, 1000), mock_clock.current_time_ms);

    mock_clock.advance(500);
    try std.testing.expectEqual(@as(u64, 1500), mock_clock.current_time_ms);
}

test "AlwaysFailingTransport fails immediately on send" {
    var fake = AlwaysFailingTransport{};
    const result = fake.send(&[_]u8{ 1, 2, 3 });
    try std.testing.expectError(session.TransportError.ConnectionReset, result);
    try std.testing.expectEqual(@as(u64, 1), fake.send_count);
}

test "AlwaysFailingTransport resets state" {
    var fake = AlwaysFailingTransport{};
    const result = fake.send(&[_]u8{ 1 });
    try std.testing.expectError(session.TransportError.ConnectionReset, result);
    try std.testing.expectEqual(@as(u64, 1), fake.send_count);

    fake.reset();
    try std.testing.expectEqual(@as(u64, 0), fake.send_count);
    try std.testing.expect(!fake.closed);
}

test "AlwaysFailingTransport closes properly" {
    var fake = AlwaysFailingTransport{};
    try std.testing.expect(!fake.closed);
    fake.close();
    try std.testing.expect(fake.closed);
}

test "ReconnectStressResults struct has expected fields" {
    const results = ReconnectStressResults{
        .generations = 0,
        .baseline_live_bytes = 0,
        .final_live_bytes = 0,
        .peak_live_bytes = 0,
        .peak_active_timers = 0,
        .peak_active_sockets = 0,
        .total_reconnect_count = 0,
        .allocations_leaked = false,
        .resources_at_baseline = true,
    };

    try std.testing.expectEqual(@as(u64, 0), results.generations);
    try std.testing.expect(!results.allocations_leaked);
    try std.testing.expect(results.resources_at_baseline);
}

test "makeTestSessionConfig creates valid config" {
    const cfg = makeTestSessionConfig();
    try std.testing.expectEqual(@as(u16, 65001), cfg.local_as);
    try std.testing.expectEqual(@as(u16, 65002), cfg.peer_as);
    try std.testing.expectEqual(@as(u32, 180), cfg.hold_time_seconds);
}

test "STRESS_TEST_GENERATIONS constant is exactly 10,000" {
    try std.testing.expectEqual(@as(u64, 10_000), STRESS_TEST_GENERATIONS);
}

test "BoundedResourceCounters: tryReserve + release slot pair" {
    var c = allocation_tracker.BoundedResourceCounters{};
    c.error_history_capacity = 2;
    try std.testing.expect(c.tryReserveErrorHistorySlot());
    try std.testing.expect(c.tryReserveErrorHistorySlot());
    try std.testing.expect(!c.tryReserveErrorHistorySlot());
    c.releaseErrorHistorySlot();
    try std.testing.expect(c.tryReserveErrorHistorySlot());
    try c.validateGenerationComplete(0);
}

test "ReconnectMemoryState: failure-atomic generation transition" {
    var state = allocation_tracker.ReconnectMemoryState{};
    try state.finishGeneration(0);
    try std.testing.expectEqual(@as(u64, 1), state.generation);
}
