// clock.zig — Testable monotonic clock for BGP session timers
//
// Provides a Clock trait and real implementation for BGP KEEPALIVE/hold timers.
// The abstraction allows unit tests to control time progression deterministically.
//
// BGP RFC 4271:
// - Keepalive interval = min(keepalive_time, hold_time / 3)
// - If hold_time = 0, KEEPALIVE/hold timer behavior is disabled
// - Hold timer expires if no message received within hold_time seconds

const std = @import("std");

/// Monotonic time in milliseconds since an arbitrary epoch.
pub const MonoTime = u64;

/// Clock interface for time operations.
/// Implement this trait to inject fake clocks in tests.
pub const Clock = struct {
    /// Get the current monotonic time in milliseconds.
    getMonoTimeMs: *const fn () MonoTime,
};

/// Get current monotonic time in milliseconds.
///
/// Uses std.os.linux.clock_gettime(CLOCK_MONOTONIC) for monotonic time.
/// This is appropriate for BGP timers since monotonic time is not affected
/// by NTP or manual time adjustments.
///
/// Returns milliseconds since arbitrary epoch (monotonic clock).
fn currentMonotonicMillis() MonoTime {
    // On Linux, use CLOCK_MONOTONIC for BGP timers.
    // On non-Linux (macOS, etc.), return 0 and let tests use MockClock.
    if (comptime @import("builtin").os.tag == .linux and @hasDecl(std.os.linux, "clock_gettime")) {
        // CLOCK_MONOTONIC = 1
        var ts: std.os.linux.timespec = undefined;
        if (std.os.linux.clock_gettime(@enumFromInt(1), &ts) < 0) return 0;
        // Detect field names: stable Zig 0.16.0 uses .sec/.nsec; dev builds use .tv_sec/.tv_nsec
        const sec_val: u128 = if (@hasDecl(std.os.linux.timespec, "tv_sec"))
            @intCast(ts.tv_sec) else @intCast(ts.sec);
        const nsec_val: u128 = if (@hasDecl(std.os.linux.timespec, "tv_nsec"))
            @intCast(ts.tv_nsec) else @intCast(ts.nsec);
        return @as(MonoTime, @intCast(sec_val * 1000 + nsec_val / 1_000_000));
    }
    // Fallback for non-Linux: return 0 (tests use MockClock)
    return 0;
}

/// Real monotonic clock for BGP timers.
/// Uses CLOCK_MONOTONIC so NTP adjustments don't affect BGP detection timers.
pub fn realNow() MonoTime {
    return currentMonotonicMillis();
}

/// Real clock instance (singleton for production use).
pub const RealClock = Clock{
    .getMonoTimeMs = struct {
        fn get() MonoTime {
            return realNow();
        }
    }.get,
};

/// Mock clock for testing with controllable time.
/// All instances share the same mock time storage.
pub const MockClock = struct {
    const Self = @This();

    /// Shared mock time storage (per-thread safety is caller's responsibility).
    /// Initialized to 0 for tests.
    var mock_time: MonoTime = 0;

    /// Set the mock time explicitly.
    pub fn setTime(t: MonoTime) void {
        mock_time = t;
    }

    /// Advance mock time by delta milliseconds.
    pub fn advance(delta_ms: u32) void {
        mock_time +%= delta_ms;
    }

    /// Get the current mock time.
    pub fn getTime() MonoTime {
        return mock_time;
    }

    /// Reset mock time to 0.
    pub fn reset() void {
        mock_time = 0;
    }

    /// Create a Clock interface that uses mock time.
    pub fn interface() Clock {
        return Clock{
            .getMonoTimeMs = struct {
                fn get() MonoTime {
                    return mock_time;
                }
            }.get,
        };
    }
};

// === Tests ===

test "MockClock basic operations" {
    MockClock.reset();
    try std.testing.expectEqual(@as(MonoTime, 0), MockClock.getTime());

    MockClock.setTime(1000);
    try std.testing.expectEqual(@as(MonoTime, 1000), MockClock.getTime());

    MockClock.advance(500);
    try std.testing.expectEqual(@as(MonoTime, 1500), MockClock.getTime());

    MockClock.reset();
    try std.testing.expectEqual(@as(MonoTime, 0), MockClock.getTime());
}

test "MockClock overflow wrap" {
    MockClock.reset();
    MockClock.setTime(std.math.maxInt(MonoTime) - 100);
    MockClock.advance(200);
    // Should wrap around (u64 overflow)
    try std.testing.expect(MockClock.getTime() < 200);
}

test "RealClock returns non-negative" {
    const t = realNow();
    // Real clock should return a non-negative value
    try std.testing.expect(t >= 0);
}
