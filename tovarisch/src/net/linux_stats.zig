// linux_stats.zig — Pure sysfs interface statistics parsing
//
// This module provides pure parsing helpers for Linux network interface
// statistics exposed under /sys/class/net/<iface>/statistics/*.
//
// This is a pure parser slice (ACT 5a). It does NOT read /sys/class/net yet.
// It only introduces the data model and parsing helpers needed to safely
// read Linux sysfs counters later (ACT 5b).

const std = @import("std");

/// Errors that can occur during counter parsing.
pub const ParseError = error{
    /// Input is empty after trimming whitespace.
    EmptyCounter,
    /// Input contains non-numeric characters.
    InvalidCounter,
    /// Input represents a negative number.
    NegativeCounter,
    /// Input exceeds u64 maximum (18446744073709551615).
    CounterOverflow,
};

/// Network interface traffic statistics.
///
/// Fields map to Linux kernel ABI counters under:
/// /sys/class/net/<iface>/statistics/{rx_bytes,tx_bytes,rx_packets,tx_packets}
///
/// NOTE: These are cumulative counters since interface/device lifetime.
/// They are NOT instantaneous bandwidth. Bandwidth/utilization requires
/// computing deltas between samples over time — this is deferred to ACT 5b+.
pub const InterfaceStats = struct {
    rx_bytes: u64,
    tx_bytes: u64,
    rx_packets: u64,
    tx_packets: u64,
};

/// Parse a counter value from sysfs stat file contents.
///
/// Accepts:
/// - Plain integer: "123"
/// - Integer with trailing newline: "123\n"
/// - Integer with surrounding whitespace: "  123\n"
///
/// Rejects:
/// - Empty input (after trim)
/// - Negative values (leading '-')
/// - Non-numeric characters
/// - Values exceeding u64 max
pub fn parseCounter(bytes: []const u8) ParseError!u64 {
    // Trim ASCII whitespace: space, tab, carriage return, newline
    const trimmed = std.mem.trim(u8, bytes, " \t\r\n");

    if (trimmed.len == 0) {
        return error.EmptyCounter;
    }

    // Explicitly reject negative input
    if (trimmed[0] == '-') {
        return error.NegativeCounter;
    }

    // Check for non-digit characters before parsing
    for (trimmed) |c| {
        if (c < '0' or c > '9') {
            return error.InvalidCounter;
        }
    }

    // Parse the integer; std.fmt.parseInt returns overflow as error
    return std.fmt.parseInt(u64, trimmed, 10) catch |e| {
        if (e == error.Overflow) {
            return error.CounterOverflow;
        }
        return error.InvalidCounter;
    };
}

/// Construct InterfaceStats from four raw counter strings.
///
/// This is a pure helper for testing and for future sysfs collection.
/// Returns error if any counter fails to parse.
pub fn statsFromCounters(
    rx_bytes: []const u8,
    tx_bytes: []const u8,
    rx_packets: []const u8,
    tx_packets: []const u8,
) ParseError!InterfaceStats {
    return InterfaceStats{
        .rx_bytes = try parseCounter(rx_bytes),
        .tx_bytes = try parseCounter(tx_bytes),
        .rx_packets = try parseCounter(rx_packets),
        .tx_packets = try parseCounter(tx_packets),
    };
}

// ============================================================================
// Tests
// ============================================================================

test "parseCounter: plain integer" {
    try std.testing.expectEqual(@as(u64, 123), try parseCounter("123"));
}

test "parseCounter: integer with newline" {
    try std.testing.expectEqual(@as(u64, 123), try parseCounter("123\n"));
}

test "parseCounter: integer with surrounding whitespace" {
    try std.testing.expectEqual(@as(u64, 123), try parseCounter("  123\n"));
}

test "parseCounter: integer with tab and newline" {
    try std.testing.expectEqual(@as(u64, 789), try parseCounter("\t789\n"));
}

test "parseCounter: integer with carriage return" {
    try std.testing.expectEqual(@as(u64, 999), try parseCounter("999\r\n"));
}

test "parseCounter: rejects empty input" {
    try std.testing.expectError(error.EmptyCounter, parseCounter(""));
}

test "parseCounter: rejects whitespace-only input" {
    try std.testing.expectError(error.EmptyCounter, parseCounter("   \n\t "));
}

test "parseCounter: rejects negative input" {
    try std.testing.expectError(error.NegativeCounter, parseCounter("-1\n"));
}

test "parseCounter: rejects non-numeric input" {
    try std.testing.expectError(error.InvalidCounter, parseCounter("abc\n"));
}

test "parseCounter: rejects mixed input" {
    try std.testing.expectError(error.InvalidCounter, parseCounter("123abc\n"));
}

test "parseCounter: rejects leading non-digit" {
    try std.testing.expectError(error.InvalidCounter, parseCounter("x123"));
}

test "parseCounter: rejects embedded non-digit" {
    try std.testing.expectError(error.InvalidCounter, parseCounter("12 3"));
}

test "statsFromCounters: builds InterfaceStats from valid counters" {
    const stats = try statsFromCounters("100", "200", "50", "75");
    try std.testing.expectEqual(@as(u64, 100), stats.rx_bytes);
    try std.testing.expectEqual(@as(u64, 200), stats.tx_bytes);
    try std.testing.expectEqual(@as(u64, 50), stats.rx_packets);
    try std.testing.expectEqual(@as(u64, 75), stats.tx_packets);
}

test "statsFromCounters: fails if any counter is invalid" {
    try std.testing.expectError(
        error.InvalidCounter,
        statsFromCounters("100", "abc", "50", "75"),
    );
}

test "statsFromCounters: fails on negative rx_bytes" {
    try std.testing.expectError(
        error.NegativeCounter,
        statsFromCounters("-1", "200", "50", "75"),
    );
}

test "parseCounter: handles large u64 values" {
    // Max u64: 18446744073709551615
    try std.testing.expectEqual(@as(u64, 18446744073709551615), try parseCounter("18446744073709551615"));
}

test "parseCounter: handles u64 max minus one" {
    try std.testing.expectEqual(@as(u64, 18446744073709551614), try parseCounter("18446744073709551614"));
}

test "parseCounter: handles u64 max plus one (overflow)" {
    try std.testing.expectError(error.CounterOverflow, parseCounter("18446744073709551616"));
}

test "parseCounter: handles large overflow values" {
    try std.testing.expectError(error.CounterOverflow, parseCounter("99999999999999999999"));
}

test "parseCounter: handles zero" {
    try std.testing.expectEqual(@as(u64, 0), try parseCounter("0"));
}

test "parseCounter: handles zero with whitespace" {
    try std.testing.expectEqual(@as(u64, 0), try parseCounter("  0\n"));
}

test "parseCounter: large but valid counter" {
    // A large realistic counter value
    try std.testing.expectEqual(@as(u64, 1073741824), try parseCounter("1073741824"));
}

test "InterfaceStats: struct has correct field types" {
    const stats = InterfaceStats{
        .rx_bytes = 100,
        .tx_bytes = 200,
        .rx_packets = 50,
        .tx_packets = 75,
    };
    try std.testing.expectEqual(@as(u64, 100), stats.rx_bytes);
    try std.testing.expectEqual(@as(u64, 200), stats.tx_bytes);
    try std.testing.expectEqual(@as(u64, 50), stats.rx_packets);
    try std.testing.expectEqual(@as(u64, 75), stats.tx_packets);
}
