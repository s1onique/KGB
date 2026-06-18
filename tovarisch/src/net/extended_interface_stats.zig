// extended_interface_stats.zig — Extended Linux interface statistics
//
// ACT: Add tovarisch WireGuard and XRay TCP underlay diagnostics
// Extended interface statistics including error/drop counters and deltas.
//
// Reads from sysfs:
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
} || linux_stats.ParseError;

// ============================================================================
// Reading
// ============================================================================

/// Read extended stats for a specific interface.
pub fn readExtendedInterfaceStats(
    allocator: std.mem.Allocator,
    sysfs_root: []const u8,
    iface: []const u8,
) ReadError!ExtendedInterfaceStats {
    var iface_dir_buf: [4096]u8 = undefined;
    const iface_dir = std.fmt.bufPrint(&iface_dir_buf, "{s}/{s}", .{ sysfs_root, iface }) catch return error.StatFileUnreadable;

    if (!linux_stats.fileExists(iface_dir)) return error.InterfaceNotFound;

    var stats_dir_buf: [4096]u8 = undefined;
    const stats_dir = std.fmt.bufPrint(&stats_dir_buf, "{s}/statistics", .{iface_dir}) catch return error.StatFileUnreadable;

    if (!linux_stats.fileExists(stats_dir)) return error.StatisticsDirMissing;

    // Read basic stats
    const basic = try linux_stats.readInterfaceStats(allocator, sysfs_root, iface);

    // Read extended fields
    const operstate = try readInterfaceField(allocator, iface_dir, "operstate");
    const carrier = readOptionalBool(iface_dir, "carrier");
    const mtu = readOptionalU32(iface_dir, "mtu");
    const tx_queue_len = readOptionalU32(iface_dir, "tx_queue_len");

    // Read error counters
    const rx_errors = try readStatCounter(stats_dir, "rx_errors");
    const tx_errors = try readStatCounter(stats_dir, "tx_errors");
    const rx_dropped = try readStatCounter(stats_dir, "rx_dropped");
    const tx_dropped = try readStatCounter(stats_dir, "tx_dropped");

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

/// Read an interface file field.
/// Caller owns the returned slice and must free it.
fn readInterfaceField(allocator: std.mem.Allocator, iface_dir: []const u8, field: []const u8) ReadError![]u8 {
    var path_buf: [4096]u8 = undefined;
    const path = std.fmt.bufPrint(&path_buf, "{s}/{s}", .{ iface_dir, field }) catch return error.StatFileUnreadable;

    const content = try linux_stats.readFile(allocator, path);
    const trimmed = std.mem.trim(u8, content, " \t\r\n");
    // Duplicate the trimmed slice so we can free the original content
    const owned = try allocator.dupe(u8, trimmed);
    allocator.free(content);

    return owned;
}

/// Read an optional boolean field.
fn readOptionalBool(iface_dir: []const u8, field: []const u8) ?bool {
    var path_buf: [4096]u8 = undefined;
    const path = std.fmt.bufPrint(&path_buf, "{s}/{s}", .{ iface_dir, field }) catch return null;

    var file_buf: [32]u8 = undefined;
    const fd = linux_stats.openForRead(path) catch return null;
    defer linux_stats.closeFile(fd);

    const n = linux_stats.readFromFd(fd, &file_buf) catch return null;
    if (n == 0) return null;

    const content = file_buf[0..n];
    const trimmed = std.mem.trim(u8, content, " \t\r\n");

    return std.mem.eql(u8, trimmed, "1");
}

/// Read an optional u32 field.
fn readOptionalU32(iface_dir: []const u8, field: []const u8) ?u32 {
    var path_buf: [4096]u8 = undefined;
    const path = std.fmt.bufPrint(&path_buf, "{s}/{s}", .{ iface_dir, field }) catch return null;

    var file_buf: [32]u8 = undefined;
    const fd = linux_stats.openForRead(path) catch return null;
    defer linux_stats.closeFile(fd);

    const n = linux_stats.readFromFd(fd, &file_buf) catch return null;
    if (n == 0) return null;

    const content = file_buf[0..n];
    const trimmed = std.mem.trim(u8, content, " \t\r\n");

    return std.fmt.parseInt(u32, trimmed, 10) catch null;
}

/// Read a statistic counter from sysfs statistics directory.
fn readStatCounter(stats_dir: []const u8, stat: []const u8) ReadError!u64 {
    var path_buf: [4096]u8 = undefined;
    const path = std.fmt.bufPrint(&path_buf, "{s}/{s}", .{ stats_dir, stat }) catch return error.StatFileUnreadable;

    const content = linux_stats.readFile(std.heap.page_allocator, path) catch return error.StatFileMissing;
    defer std.heap.page_allocator.free(content);

    return linux_stats.parseCounter(content);
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
