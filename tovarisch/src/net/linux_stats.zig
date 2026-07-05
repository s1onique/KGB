// linux_stats.zig — Linux sysfs interface statistics parser and reader
//
// ACT 5a: Pure parsing helpers (InterfaceStats, parseCounter, statsFromCounters).
// ACT 5b: Live sysfs reading (readInterfaceStats) with test fixtures.
// ACT 5c: Live sysfs smoke test in linux_stats_tests.zig.
//
// HULK16: Production Linux sysfs reads use linux_read.zig boundary for
// path validation and max-byte caps. Test infrastructure unchanged.

const std = @import("std");
const linux_read = @import("linux_read.zig");

// ============================================================================
// Constants
// ============================================================================

/// Maximum bytes for a single counter value (e.g., rx_bytes)
/// Counter files typically contain a single integer < 20 digits
const COUNTER_MAX_BYTES: usize = 32;

/// Max path length for C string conversion
const MAX_PATH_BUF: usize = 4096;

// ============================================================================
// Pure Parser (ACT 5a)
// ============================================================================

pub const ParseError = error{
    EmptyCounter,
    InvalidCounter,
    NegativeCounter,
    CounterOverflow,
};

pub const InterfaceStats = struct {
    rx_bytes: u64,
    tx_bytes: u64,
    rx_packets: u64,
    tx_packets: u64,
};

pub fn parseCounter(bytes: []const u8) ParseError!u64 {
    const trimmed = std.mem.trim(u8, bytes, " \t\r\n");
    if (trimmed.len == 0) return error.EmptyCounter;
    if (trimmed[0] == '-') return error.NegativeCounter;
    for (trimmed) |c| {
        if (c < '0' or c > '9') return error.InvalidCounter;
    }
    return std.fmt.parseInt(u64, trimmed, 10) catch |e| {
        if (e == error.Overflow) return error.CounterOverflow;
        return error.InvalidCounter;
    };
}

pub const ReadError = error{
    InterfaceNotFound,
    StatisticsDirMissing,
    StatFileMissing,
    StatFileUnreadable,
    InvalidStatContents,
    OutOfMemory,
} || ParseError;

// ============================================================================
// Filesystem Helpers
// ============================================================================

/// Converts a Zig slice to a null-terminated C string.
/// std.fmt.bufPrint() returns a slice without null-terminator,
/// but C filesystem APIs require null-terminated paths.
fn toCString(path: []const u8, buf: *[4096]u8) error{PathTooLong}![*:0]const u8 {
    if (path.len >= buf.len) return error.PathTooLong;
    // MemoryCopySafety: buf is a fixed [4096]u8 buffer. path is a caller-provided
    // slice. They are distinct memory regions; no aliasing.
    @memcpy(buf[0..path.len], path);
    buf[path.len] = 0;
    return @as([*:0]const u8, @ptrCast(buf));
}

/// Check if a file or directory exists.
/// Exported for test access (linux_stats_tests.zig).
pub fn fileExists(path: []const u8) bool {
    var path_buf: [4096]u8 = undefined;
    const c_path = toCString(path, &path_buf) catch return false;

    if (@import("builtin").os.tag == .linux) {
        const flags = std.os.linux.O{ .ACCMODE = std.posix.ACCMODE.RDONLY };
        const fd = std.c.open(c_path, flags, @as(c_uint, 0));
        if (fd >= 0) {
            _ = std.c.close(fd);
            return true;
        }
        return false;
    }
    // macOS - use access() with F_OK to check existence
    return std.c.access(c_path, std.c.F_OK) == 0;
}

fn openForWrite(path: []const u8) ReadError!usize {
    var path_buf: [4096]u8 = undefined;
    const c_path = toCString(path, &path_buf) catch return error.StatFileUnreadable;

    if (@import("builtin").os.tag == .linux) {
        const flags = std.os.linux.O{
            .ACCMODE = std.posix.ACCMODE.WRONLY,
            .CREAT = true,
            .TRUNC = true,
        };
        const fd = std.c.open(c_path, flags, @as(c_uint, 0o644));
        if (fd < 0) return error.StatFileUnreadable;
        return @as(usize, @intCast(fd));
    }
    // macOS - fopen requires null-terminated path
    const file = std.c.fopen(c_path, "w");
    if (file) |f| {
        return @intFromPtr(f);
    }
    return error.StatFileUnreadable;
}

/// Close a file descriptor.
pub fn closeFile(fd: usize) void {
    if (@import("builtin").os.tag == .linux) {
        _ = std.c.close(@as(c_int, @intCast(fd)));
    } else {
        const file = @as(*std.c.FILE, @ptrFromInt(fd));
        _ = std.c.fclose(file);
    }
}

fn writeToFd(fd: usize, contents: []const u8) !void {
    if (@import("builtin").os.tag == .linux) {
        const written = std.c.write(@as(c_int, @intCast(fd)), contents.ptr, contents.len);
        if (written < 0) return error.StatFileUnreadable;
    } else {
        const file = @as(*std.c.FILE, @ptrFromInt(fd));
        _ = std.c.fwrite(contents.ptr, 1, contents.len, file);
    }
}

/// Create a directory.
/// Exported for test access (linux_stats_tests.zig).
pub fn makeDir(path: []const u8) !void {
    var path_buf: [4096]u8 = undefined;
    const c_path = try toCString(path, &path_buf);
    _ = std.c.mkdir(c_path, 0o755);
}

/// Delete an empty directory.
/// Exported for test access (linux_stats_tests.zig).
pub fn deleteTree(path: []const u8) !void {
    var path_buf: [4096]u8 = undefined;
    const c_path = try toCString(path, &path_buf);
    _ = std.c.rmdir(c_path);
}

/// Write contents to a file.
/// Exported for test access (linux_stats_tests.zig).
pub fn writeFile(path: []const u8, contents: []const u8) !void {
    const fd = try openForWrite(path);
    defer closeFile(fd);
    try writeToFd(fd, contents);
}

// ============================================================================
// Read Interface Stats (HULK16: via linux_read.zig boundary)
// ============================================================================

/// Read interface statistics from sysfs.
/// 
/// MemoryOwnership: caller provides allocator; returned InterfaceStats is pass-by-value
/// (no heap allocation for the struct itself). Counter values are parsed from owned
/// allocations that are freed via errdefer before return.
/// 
/// Deinit: None required - InterfaceStats is pass-by-value.
/// 
/// Root parameter allows tests to use .test_fixture while production uses .sysfs_net.
pub fn readInterfaceStats(
    allocator: std.mem.Allocator,
    sysfs_root: []const u8,
    iface: []const u8,
    root: linux_read.AllowedRoot,
) ReadError!InterfaceStats {
    var iface_dir_buf: [4096]u8 = undefined;
    const iface_dir = std.fmt.bufPrint(&iface_dir_buf, "{s}/{s}", .{ sysfs_root, iface }) catch return error.StatFileUnreadable;

    // Validate and check interface exists via linux_read boundary
    if (!linux_read.validatePath(root, iface_dir)) return error.InterfaceNotFound;
    if (!linux_read.linuxExists(iface_dir, root)) return error.InterfaceNotFound;

    var stats_dir_buf: [4096]u8 = undefined;
    const stats_dir = std.fmt.bufPrint(&stats_dir_buf, "{s}/statistics", .{iface_dir}) catch return error.StatFileUnreadable;

    if (!linux_read.linuxExists(stats_dir, root)) return error.StatisticsDirMissing;

    var rx_bytes_buf: [4096]u8 = undefined;
    var tx_bytes_buf: [4096]u8 = undefined;
    var rx_packets_buf: [4096]u8 = undefined;
    var tx_packets_buf: [4096]u8 = undefined;

    const rx_bytes_path = std.fmt.bufPrint(&rx_bytes_buf, "{s}/rx_bytes", .{stats_dir}) catch return error.StatFileUnreadable;
    const tx_bytes_path = std.fmt.bufPrint(&tx_bytes_buf, "{s}/tx_bytes", .{stats_dir}) catch return error.StatFileUnreadable;
    const rx_packets_path = std.fmt.bufPrint(&rx_packets_buf, "{s}/rx_packets", .{stats_dir}) catch return error.StatFileUnreadable;
    const tx_packets_path = std.fmt.bufPrint(&tx_packets_buf, "{s}/tx_packets", .{stats_dir}) catch return error.StatFileUnreadable;

    // Read via linux_read.linuxReadCounter() boundary (HULK16)
    // We must free any .value allocations on early return, but NOT on success.
    const rx_bytes_result = linux_read.linuxReadCounter(allocator, rx_bytes_path, root);
    const tx_bytes_result = linux_read.linuxReadCounter(allocator, tx_bytes_path, root);
    const rx_packets_result = linux_read.linuxReadCounter(allocator, rx_packets_path, root);
    const tx_packets_result = linux_read.linuxReadCounter(allocator, tx_packets_path, root);

    // Convert LinuxReadResult to parseable content, mapping errors appropriately
    // On error return, free all successfully-read allocations.
    const rx_bytes_content = switch (rx_bytes_result) {
        .value => |v| v,
        .missing => {
            // Free tx/rx_packets/tx if they were read successfully
            if (tx_bytes_result == .value) allocator.free(tx_bytes_result.value);
            if (rx_packets_result == .value) allocator.free(rx_packets_result.value);
            if (tx_packets_result == .value) allocator.free(tx_packets_result.value);
            return error.StatFileMissing;
        },
        else => {
            if (tx_bytes_result == .value) allocator.free(tx_bytes_result.value);
            if (rx_packets_result == .value) allocator.free(rx_packets_result.value);
            if (tx_packets_result == .value) allocator.free(tx_packets_result.value);
            return error.StatFileUnreadable;
        },
    };
    const tx_bytes_content = switch (tx_bytes_result) {
        .value => |v| v,
        .missing => {
            allocator.free(rx_bytes_content);
            if (rx_packets_result == .value) allocator.free(rx_packets_result.value);
            if (tx_packets_result == .value) allocator.free(tx_packets_result.value);
            return error.StatFileMissing;
        },
        else => {
            allocator.free(rx_bytes_content);
            if (rx_packets_result == .value) allocator.free(rx_packets_result.value);
            if (tx_packets_result == .value) allocator.free(tx_packets_result.value);
            return error.StatFileUnreadable;
        },
    };
    const rx_packets_content = switch (rx_packets_result) {
        .value => |v| v,
        .missing => {
            allocator.free(rx_bytes_content);
            allocator.free(tx_bytes_content);
            if (tx_packets_result == .value) allocator.free(tx_packets_result.value);
            return error.StatFileMissing;
        },
        else => {
            allocator.free(rx_bytes_content);
            allocator.free(tx_bytes_content);
            if (tx_packets_result == .value) allocator.free(tx_packets_result.value);
            return error.StatFileUnreadable;
        },
    };
    const tx_packets_content = switch (tx_packets_result) {
        .value => |v| v,
        .missing => {
            allocator.free(rx_bytes_content);
            allocator.free(tx_bytes_content);
            allocator.free(rx_packets_content);
            return error.StatFileMissing;
        },
        else => {
            allocator.free(rx_bytes_content);
            allocator.free(tx_bytes_content);
            allocator.free(rx_packets_content);
            return error.StatFileUnreadable;
        },
    };

    // Parse counter values. On success, we need to free the allocations.
    const rx_parsed = parseCounter(rx_bytes_content) catch |e| {
        allocator.free(rx_bytes_content);
        allocator.free(tx_bytes_content);
        allocator.free(rx_packets_content);
        allocator.free(tx_packets_content);
        return e;
    };
    const tx_parsed = parseCounter(tx_bytes_content) catch |e| {
        allocator.free(rx_bytes_content);
        allocator.free(tx_bytes_content);
        allocator.free(rx_packets_content);
        allocator.free(tx_packets_content);
        return e;
    };
    const rx_packets_parsed = parseCounter(rx_packets_content) catch |e| {
        allocator.free(rx_bytes_content);
        allocator.free(tx_bytes_content);
        allocator.free(rx_packets_content);
        allocator.free(tx_packets_content);
        return e;
    };
    const tx_packets_parsed = parseCounter(tx_packets_content) catch |e| {
        allocator.free(rx_bytes_content);
        allocator.free(tx_bytes_content);
        allocator.free(rx_packets_content);
        allocator.free(tx_packets_content);
        return e;
    };

    // Free all allocations now that parsing is complete
    allocator.free(rx_bytes_content);
    allocator.free(tx_bytes_content);
    allocator.free(rx_packets_content);
    allocator.free(tx_packets_content);

    return InterfaceStats{
        .rx_bytes = rx_parsed,
        .tx_bytes = tx_parsed,
        .rx_packets = rx_packets_parsed,
        .tx_packets = tx_packets_parsed,
    };
}

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
