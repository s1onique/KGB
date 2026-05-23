// rates.zig — Pure interface traffic rate/delta calculation
//
// ACT 1: Pure rate calculation foundation.
//
// This module calculates traffic deltas and rates from two interface counter
// samples. It is intentionally pure: no wall-clock access, no OS/sysfs access.
//
// Rate calculation is the first building block for eventual /metrics.json rate
// exposure. This ACT does NOT wire into HTTP or change the metrics contract.
//
// Usage:
//   const rate = rates.calculateRate(previous_sample, current_sample);
//   if (rate) |r| {
//       // r.rx_bytes_per_second, r.tx_bytes_per_second, etc.
//   }
//
// Non-goals:
// - No /metrics.json wiring
// - No counter collection (collection belongs in linux_stats.zig)
// - No HTTP server integration
// - No IPv6 support
//
// Counter reset handling:
// - If any current counter is less than its previous, treat as counter reset
//   and return null (no rate can be computed).
// - This handles interface restart, reboot, or kernel counter reset.

const std = @import("std");

// ============================================================================
// Data Structures
// ============================================================================

/// A snapshot of interface counter values at a specific point in time.
/// Used as input for rate calculation.
pub const InterfaceCounterSample = struct {
    name: []const u8,
    rx_bytes: u64,
    tx_bytes: u64,
    rx_packets: u64,
    tx_packets: u64,
    /// Timestamp when this sample was taken, in milliseconds since epoch.
    /// Used to calculate elapsed time for rate computation.
    sampled_at_ms: i64,
};

/// Computed traffic rates derived from two counter samples.
/// All deltas and rates are non-negative integers.
/// Returns null when rate cannot be computed (first sample, counter reset, etc.).
pub const InterfaceRate = struct {
    /// The elapsed time between samples, in seconds.
    window_seconds: u64,
    /// Delta values (current - previous)
    rx_bytes_delta: u64,
    tx_bytes_delta: u64,
    rx_packets_delta: u64,
    tx_packets_delta: u64,
    /// Per-second rates (delta / window_seconds)
    /// Integer division; may be zero for very small deltas.
    rx_bytes_per_second: u64,
    tx_bytes_per_second: u64,
    rx_packets_per_second: u64,
    tx_packets_per_second: u64,
};

// ============================================================================
// Rate Calculation
// ============================================================================

/// Calculates traffic rates from two interface counter samples.
///
/// Returns null when rate cannot be computed:
/// - `previous` is null (first sample has no rate)
/// - `current.sampled_at_ms <= previous.sampled_at_ms` (non-positive elapsed time)
/// - Elapsed time is less than 1000ms (sub-second samples have no rate)
/// - Any current counter is less than its previous (counter reset detected)
///
/// Uses integer arithmetic only. Per-second rates use actual elapsed time,
/// not an assumed interval. Rates are only computed when at least one whole
/// second has elapsed.
///
/// Behavior:
/// - If previous is null, return null.
/// - If elapsed time is zero or negative, return null.
/// - If elapsed time is less than 1000ms, return null (no rate can be derived).
/// - If any counter decreased, return null (counter reset).
/// - Otherwise, calculate deltas and integer per-second rates.
/// - window_seconds is the integer elapsed seconds (>= 1).
/// - Per-second values use integer division (may be zero for small deltas).
pub fn calculateRate(
    previous: ?InterfaceCounterSample,
    current: InterfaceCounterSample,
) ?InterfaceRate {
    // First sample has no previous - no rate can be computed
    const prev = previous orelse return null;

    // Non-positive elapsed time is invalid
    const elapsed_ms = current.sampled_at_ms - prev.sampled_at_ms;
    if (elapsed_ms <= 0) return null;

    // Sub-second elapsed time - cannot derive a meaningful rate
    // A raw delta over 500ms is NOT "bytes per second" without explicit scaling
    if (elapsed_ms < 1000) return null;

    // Detect counter reset: any counter going backwards indicates reset
    if (current.rx_bytes < prev.rx_bytes) return null;
    if (current.tx_bytes < prev.tx_bytes) return null;
    if (current.rx_packets < prev.rx_packets) return null;
    if (current.tx_packets < prev.tx_packets) return null;

    // Calculate deltas
    const rx_bytes_delta = current.rx_bytes - prev.rx_bytes;
    const tx_bytes_delta = current.tx_bytes - prev.tx_bytes;
    const rx_packets_delta = current.rx_packets - prev.rx_packets;
    const tx_packets_delta = current.tx_packets - prev.tx_packets;

    // Calculate elapsed time in whole seconds (>= 1 here)
    const window_seconds: u64 = @intCast(@divTrunc(elapsed_ms, 1000));

    // Calculate per-second rates
    const rx_bytes_per_second = @divTrunc(rx_bytes_delta, window_seconds);
    const tx_bytes_per_second = @divTrunc(tx_bytes_delta, window_seconds);
    const rx_packets_per_second = @divTrunc(rx_packets_delta, window_seconds);
    const tx_packets_per_second = @divTrunc(tx_packets_delta, window_seconds);

    return InterfaceRate{
        .window_seconds = window_seconds,
        .rx_bytes_delta = rx_bytes_delta,
        .tx_bytes_delta = tx_bytes_delta,
        .rx_packets_delta = rx_packets_delta,
        .tx_packets_delta = tx_packets_delta,
        .rx_bytes_per_second = rx_bytes_per_second,
        .tx_bytes_per_second = tx_bytes_per_second,
        .rx_packets_per_second = rx_packets_per_second,
        .tx_packets_per_second = tx_packets_per_second,
    };
}

// ============================================================================
// Tests
// ============================================================================

test "first sample returns null" {
    const current = InterfaceCounterSample{
        .name = "wg0",
        .rx_bytes = 1000,
        .tx_bytes = 2000,
        .rx_packets = 10,
        .tx_packets = 20,
        .sampled_at_ms = 1000,
    };

    const rate = calculateRate(null, current);
    try std.testing.expect(rate == null);
}

test "normal byte and packet delta" {
    const previous = InterfaceCounterSample{
        .name = "wg0",
        .rx_bytes = 1000,
        .tx_bytes = 2000,
        .rx_packets = 10,
        .tx_packets = 20,
        .sampled_at_ms = 0,
    };
    const current = InterfaceCounterSample{
        .name = "wg0",
        .rx_bytes = 31000,  // +30000
        .tx_bytes = 62000,  // +60000
        .rx_packets = 310,   // +300
        .tx_packets = 620,   // +600
        .sampled_at_ms = 30000,  // 30 seconds
    };

    const rate = calculateRate(previous, current);
    try std.testing.expect(rate != null);

    const r = rate.?;
    try std.testing.expectEqual(@as(u64, 30), r.window_seconds);
    try std.testing.expectEqual(@as(u64, 30000), r.rx_bytes_delta);
    try std.testing.expectEqual(@as(u64, 60000), r.tx_bytes_delta);
    try std.testing.expectEqual(@as(u64, 300), r.rx_packets_delta);
    try std.testing.expectEqual(@as(u64, 600), r.tx_packets_delta);
    try std.testing.expectEqual(@as(u64, 1000), r.rx_bytes_per_second);
    try std.testing.expectEqual(@as(u64, 2000), r.tx_bytes_per_second);
    try std.testing.expectEqual(@as(u64, 10), r.rx_packets_per_second);
    try std.testing.expectEqual(@as(u64, 20), r.tx_packets_per_second);
}

test "uses actual elapsed time" {
    const previous = InterfaceCounterSample{
        .name = "wg0",
        .rx_bytes = 0,
        .tx_bytes = 0,
        .rx_packets = 0,
        .tx_packets = 0,
        .sampled_at_ms = 0,
    };
    const current = InterfaceCounterSample{
        .name = "wg0",
        .rx_bytes = 60000,
        .tx_bytes = 0,
        .rx_packets = 0,
        .tx_packets = 0,
        .sampled_at_ms = 60000,  // 60 seconds
    };

    const rate = calculateRate(previous, current);
    try std.testing.expect(rate != null);

    const r = rate.?;
    try std.testing.expectEqual(@as(u64, 60), r.window_seconds);
    try std.testing.expectEqual(@as(u64, 1000), r.rx_bytes_per_second);  // 60000 / 60
}

test "zero elapsed time returns null" {
    const previous = InterfaceCounterSample{
        .name = "wg0",
        .rx_bytes = 1000,
        .tx_bytes = 2000,
        .rx_packets = 10,
        .tx_packets = 20,
        .sampled_at_ms = 5000,
    };
    const current = InterfaceCounterSample{
        .name = "wg0",
        .rx_bytes = 2000,
        .tx_bytes = 3000,
        .rx_packets = 20,
        .tx_packets = 30,
        .sampled_at_ms = 5000,  // Same timestamp - zero elapsed
    };

    const rate = calculateRate(previous, current);
    try std.testing.expect(rate == null);
}

test "negative elapsed time returns null" {
    const previous = InterfaceCounterSample{
        .name = "wg0",
        .rx_bytes = 1000,
        .tx_bytes = 2000,
        .rx_packets = 10,
        .tx_packets = 20,
        .sampled_at_ms = 5000,
    };
    const current = InterfaceCounterSample{
        .name = "wg0",
        .rx_bytes = 2000,
        .tx_bytes = 3000,
        .rx_packets = 20,
        .tx_packets = 30,
        .sampled_at_ms = 4000,  // Earlier timestamp - negative elapsed
    };

    const rate = calculateRate(previous, current);
    try std.testing.expect(rate == null);
}

test "rx byte counter reset returns null" {
    const previous = InterfaceCounterSample{
        .name = "wg0",
        .rx_bytes = 100000,
        .tx_bytes = 100000,
        .rx_packets = 1000,
        .tx_packets = 1000,
        .sampled_at_ms = 0,
    };
    const current = InterfaceCounterSample{
        .name = "wg0",
        .rx_bytes = 500,  // Counter reset - less than previous
        .tx_bytes = 200000,
        .rx_packets = 2000,
        .tx_packets = 2000,
        .sampled_at_ms = 30000,
    };

    const rate = calculateRate(previous, current);
    try std.testing.expect(rate == null);
}

test "tx byte counter reset returns null" {
    const previous = InterfaceCounterSample{
        .name = "wg0",
        .rx_bytes = 100000,
        .tx_bytes = 100000,
        .rx_packets = 1000,
        .tx_packets = 1000,
        .sampled_at_ms = 0,
    };
    const current = InterfaceCounterSample{
        .name = "wg0",
        .rx_bytes = 200000,
        .tx_bytes = 500,  // Counter reset - less than previous
        .rx_packets = 2000,
        .tx_packets = 2000,
        .sampled_at_ms = 30000,
    };

    const rate = calculateRate(previous, current);
    try std.testing.expect(rate == null);
}

test "rx packet counter reset returns null" {
    const previous = InterfaceCounterSample{
        .name = "wg0",
        .rx_bytes = 100000,
        .tx_bytes = 100000,
        .rx_packets = 1000,
        .tx_packets = 1000,
        .sampled_at_ms = 0,
    };
    const current = InterfaceCounterSample{
        .name = "wg0",
        .rx_bytes = 200000,
        .tx_bytes = 200000,
        .rx_packets = 50,  // Counter reset - less than previous
        .tx_packets = 2000,
        .sampled_at_ms = 30000,
    };

    const rate = calculateRate(previous, current);
    try std.testing.expect(rate == null);
}

test "tx packet counter reset returns null" {
    const previous = InterfaceCounterSample{
        .name = "wg0",
        .rx_bytes = 100000,
        .tx_bytes = 100000,
        .rx_packets = 1000,
        .tx_packets = 1000,
        .sampled_at_ms = 0,
    };
    const current = InterfaceCounterSample{
        .name = "wg0",
        .rx_bytes = 200000,
        .tx_bytes = 200000,
        .rx_packets = 2000,
        .tx_packets = 50,  // Counter reset - less than previous
        .sampled_at_ms = 30000,
    };

    const rate = calculateRate(previous, current);
    try std.testing.expect(rate == null);
}

test "sub-second elapsed time returns null" {
    const previous = InterfaceCounterSample{
        .name = "wg0",
        .rx_bytes = 0,
        .tx_bytes = 0,
        .rx_packets = 0,
        .tx_packets = 0,
        .sampled_at_ms = 0,
    };
    const current = InterfaceCounterSample{
        .name = "wg0",
        .rx_bytes = 1500,
        .tx_bytes = 500,
        .rx_packets = 5,
        .tx_packets = 2,
        .sampled_at_ms = 500,  // 500ms - sub-second, rate unavailable
    };

    // Sub-second elapsed time should return null - not a fake per-second rate
    const rate = calculateRate(previous, current);
    try std.testing.expect(rate == null);
}

test "large counters work correctly" {
    const previous = InterfaceCounterSample{
        .name = "wg0",
        .rx_bytes = 0xFFFFFFFF,
        .tx_bytes = 0xFFFFFFFF,
        .rx_packets = 100,
        .tx_packets = 100,
        .sampled_at_ms = 1000,
    };
    const current = InterfaceCounterSample{
        .name = "wg0",
        .rx_bytes = 0xFFFFFFFF + 1000000,
        .tx_bytes = 0xFFFFFFFF + 2000000,
        .rx_packets = 1100,
        .tx_packets = 1100,
        .sampled_at_ms = 61000,  // 60 seconds
    };

    const rate = calculateRate(previous, current);
    try std.testing.expect(rate != null);

    const r = rate.?;
    try std.testing.expectEqual(@as(u64, 60), r.window_seconds);
    try std.testing.expectEqual(@as(u64, 1000000), r.rx_bytes_delta);
    try std.testing.expectEqual(@as(u64, 2000000), r.tx_bytes_delta);
    try std.testing.expectEqual(@as(u64, 16666), r.rx_bytes_per_second);  // 1000000 / 60
    try std.testing.expectEqual(@as(u64, 33333), r.tx_bytes_per_second);  // 2000000 / 60
}
