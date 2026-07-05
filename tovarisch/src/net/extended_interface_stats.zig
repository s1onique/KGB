// extended_interface_stats.zig — Extended Linux interface statistics
//
// ACT-TOVARISCH-ZIG-HULK16: Migrate to canonical linux_read.zig boundary
//
// Extended interface statistics including error/drop counters and deltas.
//
// Reads from sysfs via linux_read.zig boundary:
//   /sys/class/net/<iface>/operstate
//   /sys/class/net/<iface>/carrier
//   /sys/class/net/<iface>/mtu
//   /sys/class/net/<iface>/tx_queue_len
//   /sys/class/net/<iface>/statistics/rx_bytes
//   /sys/class/net/<iface>/statistics/tx_bytes
//   /sys/class/net/<iface>/statistics/rx_packets
//   /sys/class/net/<iface>/statistics/tx_packets
//   /sys/class/net/<iface>/statistics/rx_errors
//   /sys/class/net/<iface>/statistics/tx_errors
//   /sys/class/net/<iface>/statistics/rx_dropped
//   /sys/class/net/<iface>/statistics/tx_dropped

const std = @import("std");
const linux_stats = @import("linux_stats.zig");
const linux_read = @import("linux_read.zig");

// ============================================================================
// Constants
// ============================================================================

/// Maximum bytes for reading interface field files (operstate, carrier, mtu, etc.)
const INTERFACE_FIELD_MAX_BYTES: usize = 64;

// ============================================================================
// Types
// ============================================================================

/// Extended interface statistics.
pub const ExtendedInterfaceStats = struct {
    /// Interface name.
    name: []const u8,
    /// Operational state (up, down, unknown).
    operstate: []const u8,
    /// Carrier status (true if present, null if unavailable).
    carrier: ?bool = null,
    /// MTU value.
    mtu: ?u32 = null,
    /// Transmit queue length.
    tx_queue_len: ?u32 = null,
    /// Basic stats from linux_stats.
    basic: linux_stats.InterfaceStats,
    /// Error/drop counters.
    errors: InterfaceErrors,
    /// Delta values from previous sample (null if no previous sample).
    deltas: ?InterfaceDeltas = null,
};

/// Interface error/drop counters.
pub const InterfaceErrors = struct {
    /// Receive errors.
    rx_errors: u64,
    /// Transmit errors.
    tx_errors: u64,
    /// Receive dropped packets.
    rx_dropped: u64,
    /// Transmit dropped packets.
    tx_dropped: u64,
};

/// Delta values since previous sample.
pub const InterfaceDeltas = struct {
    /// Bytes received since last sample.
    rx_bytes_delta: i64,
    /// Bytes transmitted since last sample.
    tx_bytes_delta: i64,
    /// Packets received since last sample.
    rx_packets_delta: i64,
    /// Packets transmitted since last sample.
    tx_packets_delta: i64,
    /// Receive errors since last sample.
    rx_errors_delta: i64,
    /// Transmit errors since last sample.
    tx_errors_delta: i64,
    /// Receive dropped since last sample.
    rx_dropped_delta: i64,
    /// Transmit dropped since last sample.
    tx_dropped_delta: i64,
};

/// Previous sample for delta calculation.
pub const PreviousSample = struct {
    rx_bytes: u64,
    tx_bytes: u64,
    rx_packets: u64,
    tx_packets: u64,
    rx_errors: u64,
    tx_errors: u64,
    rx_dropped: u64,
    tx_dropped: u64,
};

/// Read errors.
pub const ReadError = error{
    InterfaceNotFound,
    StatisticsDirMissing,
    StatFileMissing,
    StatFileUnreadable,
    InvalidStatContents,
    OutOfMemory,
    UnsupportedPlatform,
} || linux_stats.ParseError;

// ============================================================================
// Reading via linux_read.zig
// ============================================================================

/// Read extended stats for a specific interface.
pub fn readExtendedInterfaceStats(
    allocator: std.mem.Allocator,
    sysfs_root: []const u8,
    iface: []const u8,
) ReadError!ExtendedInterfaceStats {
    // Build paths
    var iface_dir_buf: [4096]u8 = undefined;
    const iface_dir = std.fmt.bufPrint(&iface_dir_buf, "{s}/{s}", .{ sysfs_root, iface }) catch return error.StatFileUnreadable;

    // Validate and check interface exists
    if (!linux_read.validatePath(.sysfs_net, iface_dir)) {
        return error.InterfaceNotFound;
    }
    if (!linux_read.linuxExists(iface_dir, .sysfs_net)) {
        return error.InterfaceNotFound;
    }

    var stats_dir_buf: [4096]u8 = undefined;
    const stats_dir = std.fmt.bufPrint(&stats_dir_buf, "{s}/statistics", .{iface_dir}) catch return error.StatFileUnreadable;

    if (!linux_read.linuxExists(stats_dir, .sysfs_net)) {
        return error.StatisticsDirMissing;
    }

    // Read basic stats via linux_stats (now uses linux_read.zig internally)
    const basic = linux_stats.readInterfaceStats(allocator, sysfs_root, iface, .sysfs_net) catch |e| return e;

    // Read extended fields via linux_read.zig
    const operstate = readInterfaceField(allocator, iface_dir, "operstate") catch |e| return e;
    const carrier = readOptionalBool(allocator, iface_dir, "carrier");
    const mtu = readOptionalU32(allocator, iface_dir, "mtu");
    const tx_queue_len = readOptionalU32(allocator, iface_dir, "tx_queue_len");

    // Read error counters via linux_read.zig (NOT using page_allocator)
    const rx_errors = readStatCounterOrZero(allocator, stats_dir, "rx_errors");
    const tx_errors = readStatCounterOrZero(allocator, stats_dir, "tx_errors");
    const rx_dropped = readStatCounterOrZero(allocator, stats_dir, "rx_dropped");
    const tx_dropped = readStatCounterOrZero(allocator, stats_dir, "tx_dropped");

    return ExtendedInterfaceStats{
        .name = iface,
        .operstate = operstate,
        .carrier = carrier,
        .mtu = mtu,
        .tx_queue_len = tx_queue_len,
        .basic = basic,
        .errors = InterfaceErrors{
            .rx_errors = rx_errors,
            .tx_errors = tx_errors,
            .rx_dropped = rx_dropped,
            .tx_dropped = tx_dropped,
        },
        .deltas = null,
    };
}

/// Read extended stats and calculate deltas from previous sample.
pub fn readExtendedInterfaceStatsWithDeltas(
    allocator: std.mem.Allocator,
    sysfs_root: []const u8,
    iface: []const u8,
    previous: ?PreviousSample,
) ReadError!ExtendedInterfaceStats {
    var stats = try readExtendedInterfaceStats(allocator, sysfs_root, iface);

    if (previous) |prev| {
        stats.deltas = InterfaceDeltas{
            .rx_bytes_delta = @as(i64, @intCast(stats.basic.rx_bytes)) - @as(i64, @intCast(prev.rx_bytes)),
            .tx_bytes_delta = @as(i64, @intCast(stats.basic.tx_bytes)) - @as(i64, @intCast(prev.tx_bytes)),
            .rx_packets_delta = @as(i64, @intCast(stats.basic.rx_packets)) - @as(i64, @intCast(prev.rx_packets)),
            .tx_packets_delta = @as(i64, @intCast(stats.basic.tx_packets)) - @as(i64, @intCast(prev.tx_packets)),
            .rx_errors_delta = @as(i64, @intCast(stats.errors.rx_errors)) - @as(i64, @intCast(prev.rx_errors)),
            .tx_errors_delta = @as(i64, @intCast(stats.errors.tx_errors)) - @as(i64, @intCast(prev.tx_errors)),
            .rx_dropped_delta = @as(i64, @intCast(stats.errors.rx_dropped)) - @as(i64, @intCast(prev.rx_dropped)),
            .tx_dropped_delta = @as(i64, @intCast(stats.errors.tx_dropped)) - @as(i64, @intCast(prev.tx_dropped)),
        };
    }

    return stats;
}

/// Read an interface file field via linux_read.zig boundary.
/// Caller owns the returned slice and must free it.
fn readInterfaceField(allocator: std.mem.Allocator, iface_dir: []const u8, field: []const u8) ReadError![]u8 {
    var path_buf: [4096]u8 = undefined;
    const path = std.fmt.bufPrint(&path_buf, "{s}/{s}", .{ iface_dir, field }) catch return error.StatFileUnreadable;

    // Validate path
    if (!linux_read.validatePath(.sysfs_net, path)) {
        return error.StatFileUnreadable;
    }

    // Read via linux_read.zig boundary
    const result = linux_read.linuxReadSmall(allocator, path, .sysfs_net);

    switch (result) {
        .value => |content| {
            // HULK16R3: Use defer to free content on all exits (including dupe failure)
            defer allocator.free(content);
            const trimmed = std.mem.trim(u8, content, " \t\r\n");
            return allocator.dupe(u8, trimmed) catch return error.OutOfMemory;
        },
        .missing => return error.StatFileMissing,
        .permission_denied => return error.StatFileUnreadable,
        .unsupported_platform => return error.StatFileUnreadable,
        .too_large, .malformed, .io_error => return error.StatFileUnreadable,
    }
}

/// Read an optional boolean field via linux_read.zig boundary.
/// Returns null if field cannot be read.
/// MemoryOwnership: caller provides allocator for reading
fn readOptionalBool(allocator: std.mem.Allocator, iface_dir: []const u8, field: []const u8) ?bool {
    var path_buf: [4096]u8 = undefined;
    const path = std.fmt.bufPrint(&path_buf, "{s}/{s}", .{ iface_dir, field }) catch return null;

    // Validate path
    if (!linux_read.validatePath(.sysfs_net, path)) {
        return null;
    }

    // Use linuxExists for existence check
    if (!linux_read.linuxExists(path, .sysfs_net)) {
        return null;
    }

    // Read via linux_read.zig boundary
    const result = linux_read.linuxReadSmall(allocator, path, .sysfs_net);
    defer if (result == .value) allocator.free(result.value);

    if (result != .value) {
        return null;
    }

    const content = result.value;
    const trimmed = std.mem.trim(u8, content, " \t\r\n");

    return std.mem.eql(u8, trimmed, "1");
}

/// Read an optional u32 field via linux_read.zig boundary.
/// Returns null if field cannot be read.
/// MemoryOwnership: caller provides allocator for reading
fn readOptionalU32(allocator: std.mem.Allocator, iface_dir: []const u8, field: []const u8) ?u32 {
    var path_buf: [4096]u8 = undefined;
    const path = std.fmt.bufPrint(&path_buf, "{s}/{s}", .{ iface_dir, field }) catch return null;

    // Validate path
    if (!linux_read.validatePath(.sysfs_net, path)) {
        return null;
    }

    // Use linuxExists for existence check
    if (!linux_read.linuxExists(path, .sysfs_net)) {
        return null;
    }

    // Read via linux_read.zig boundary
    const result = linux_read.linuxReadSmall(allocator, path, .sysfs_net);
    defer if (result == .value) allocator.free(result.value);

    if (result != .value) {
        return null;
    }

    const content = result.value;
    const trimmed = std.mem.trim(u8, content, " \t\r\n");

    return std.fmt.parseInt(u32, trimmed, 10) catch null;
}

/// Read a statistic counter from sysfs statistics directory via linux_read.zig boundary.
/// Returns 0 on error (structured absence, not panic).
fn readStatCounterOrZero(allocator: std.mem.Allocator, stats_dir: []const u8, stat: []const u8) u64 {
    var path_buf: [4096]u8 = undefined;
    const path = std.fmt.bufPrint(&path_buf, "{s}/{s}", .{ stats_dir, stat }) catch return 0;

    // Validate path
    if (!linux_read.validatePath(.sysfs_net, path)) {
        return 0;
    }

    // Read via linux_read.zig boundary
    const result = linux_read.linuxReadCounter(allocator, path, .sysfs_net);

    switch (result) {
        .value => |content| {
            defer allocator.free(content);
            return linux_stats.parseCounter(content) catch 0;
        },
        .missing, .permission_denied, .unsupported_platform, .too_large, .malformed, .io_error => {
            return 0;
        },
    }
}

// ============================================================================
// Tests
// ============================================================================

test "InterfaceErrors can be constructed" {
    const err = InterfaceErrors{
        .rx_errors = 0,
        .tx_errors = 0,
        .rx_dropped = 0,
        .tx_dropped = 0,
    };
    try std.testing.expectEqual(@as(u64, 0), err.rx_errors);
}

test "InterfaceDeltas can be constructed" {
    const delta = InterfaceDeltas{
        .rx_bytes_delta = 100,
        .tx_bytes_delta = 50,
        .rx_packets_delta = 10,
        .tx_packets_delta = 5,
        .rx_errors_delta = 0,
        .tx_errors_delta = 0,
        .rx_dropped_delta = 0,
        .tx_dropped_delta = 0,
    };
    try std.testing.expectEqual(@as(i64, 100), delta.rx_bytes_delta);
}

test "PreviousSample can be constructed" {
    const sample = PreviousSample{
        .rx_bytes = 1000,
        .tx_bytes = 500,
        .rx_packets = 100,
        .tx_packets = 50,
        .rx_errors = 0,
        .tx_errors = 0,
        .rx_dropped = 0,
        .tx_dropped = 0,
    };
    try std.testing.expectEqual(@as(u64, 1000), sample.rx_bytes);
}

test "INTERFACE_FIELD_MAX_BYTES is reasonable" {
    try std.testing.expect(INTERFACE_FIELD_MAX_BYTES >= 32);
    try std.testing.expect(INTERFACE_FIELD_MAX_BYTES <= 128);
}
