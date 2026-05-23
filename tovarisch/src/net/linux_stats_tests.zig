// linux_stats_tests.zig — Tests for Linux sysfs interface statistics parser and reader
//
// ACT 5a: Pure parser tests
// ACT 5b: Fixture-based readInterfaceStats tests
// ACT 5c: Live sysfs smoke test (Linux-only)
//
// This file is imported by test_all.zig and refAllDecls forces test discovery.

const std = @import("std");
const linux_stats = @import("linux_stats.zig");

// Re-export for convenience
const parseCounter = linux_stats.parseCounter;
const statsFromCounters = linux_stats.statsFromCounters;
const readInterfaceStats = linux_stats.readInterfaceStats;
const ReadError = linux_stats.ReadError;
const ParseError = linux_stats.ParseError;
const InterfaceStats = linux_stats.InterfaceStats;

// Internal helpers from linux_stats.zig (test-only access)
const makeDir = linux_stats.makeDir;
const deleteTree = linux_stats.deleteTree;
const writeFile = linux_stats.writeFile;

// ============================================================================
// Tests (ACT 5a: Pure parser)
// ============================================================================

test "parseCounter: plain integer" {
    try std.testing.expectEqual(@as(u64, 123), try parseCounter("123"));
}

test "parseCounter: integer with newline" {
    try std.testing.expectEqual(@as(u64, 123), try parseCounter("123\n"));
}

test "parseCounter: rejects empty input" {
    try std.testing.expectError(error.EmptyCounter, parseCounter(""));
}

test "parseCounter: rejects negative input" {
    try std.testing.expectError(error.NegativeCounter, parseCounter("-1\n"));
}

test "parseCounter: rejects non-numeric input" {
    try std.testing.expectError(error.InvalidCounter, parseCounter("abc\n"));
}

test "parseCounter: handles large u64 values" {
    try std.testing.expectEqual(@as(u64, 18446744073709551615), try parseCounter("18446744073709551615"));
}

test "parseCounter: handles u64 max plus one (overflow)" {
    try std.testing.expectError(error.CounterOverflow, parseCounter("18446744073709551616"));
}

test "statsFromCounters: builds InterfaceStats from valid counters" {
    const stats = try statsFromCounters("100", "200", "50", "75");
    try std.testing.expectEqual(@as(u64, 100), stats.rx_bytes);
    try std.testing.expectEqual(@as(u64, 200), stats.tx_bytes);
    try std.testing.expectEqual(@as(u64, 50), stats.rx_packets);
    try std.testing.expectEqual(@as(u64, 75), stats.tx_packets);
}

// ============================================================================
// Tests (ACT 5b: Fixture-based sysfs reading)
// ============================================================================

test "readInterfaceStats: reads valid stats from fixture" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_stats_test";

    makeDir(base) catch {};
    defer deleteTree(base) catch {};

    {
        var buf: [256]u8 = undefined;
        const path = std.fmt.bufPrint(&buf, "{s}/eth0", .{base}) catch unreachable;
        makeDir(path) catch {};
    }
    {
        var buf: [256]u8 = undefined;
        const path = std.fmt.bufPrint(&buf, "{s}/eth0/statistics", .{base}) catch unreachable;
        makeDir(path) catch {};
    }

    {
        var buf: [256]u8 = undefined;
        const path = std.fmt.bufPrint(&buf, "{s}/eth0/statistics/rx_bytes", .{base}) catch unreachable;
        writeFile(path, "100\n") catch {};
    }
    {
        var buf: [256]u8 = undefined;
        const path = std.fmt.bufPrint(&buf, "{s}/eth0/statistics/tx_bytes", .{base}) catch unreachable;
        writeFile(path, "200\n") catch {};
    }
    {
        var buf: [256]u8 = undefined;
        const path = std.fmt.bufPrint(&buf, "{s}/eth0/statistics/rx_packets", .{base}) catch unreachable;
        writeFile(path, "10\n") catch {};
    }
    {
        var buf: [256]u8 = undefined;
        const path = std.fmt.bufPrint(&buf, "{s}/eth0/statistics/tx_packets", .{base}) catch unreachable;
        writeFile(path, "20\n") catch {};
    }

    const stats = try readInterfaceStats(allocator, base, "eth0");
    try std.testing.expectEqual(@as(u64, 100), stats.rx_bytes);
    try std.testing.expectEqual(@as(u64, 200), stats.tx_bytes);
    try std.testing.expectEqual(@as(u64, 10), stats.rx_packets);
    try std.testing.expectEqual(@as(u64, 20), stats.tx_packets);
}

test "readInterfaceStats: returns error when interface directory missing" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_stats_test2";

    makeDir(base) catch {};
    defer deleteTree(base) catch {};

    try std.testing.expectError(error.InterfaceNotFound, readInterfaceStats(allocator, base, "eth99"));
}

test "readInterfaceStats: returns error when statistics directory missing" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_stats_test3";

    makeDir(base) catch {};
    {
        var buf: [256]u8 = undefined;
        const path = std.fmt.bufPrint(&buf, "{s}/eth0", .{base}) catch unreachable;
        makeDir(path) catch {};
    }
    defer deleteTree(base) catch {};

    try std.testing.expectError(error.StatisticsDirMissing, readInterfaceStats(allocator, base, "eth0"));
}

test "readInterfaceStats: returns error when counter file missing" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_stats_test4";

    makeDir(base) catch {};
    {
        var buf: [256]u8 = undefined;
        const path = std.fmt.bufPrint(&buf, "{s}/eth0", .{base}) catch unreachable;
        makeDir(path) catch {};
    }
    {
        var buf: [256]u8 = undefined;
        const path = std.fmt.bufPrint(&buf, "{s}/eth0/statistics", .{base}) catch unreachable;
        makeDir(path) catch {};
    }
    defer deleteTree(base) catch {};

    {
        var buf: [256]u8 = undefined;
        const path = std.fmt.bufPrint(&buf, "{s}/eth0/statistics/tx_bytes", .{base}) catch unreachable;
        writeFile(path, "200\n") catch {};
    }
    {
        var buf: [256]u8 = undefined;
        const path = std.fmt.bufPrint(&buf, "{s}/eth0/statistics/rx_packets", .{base}) catch unreachable;
        writeFile(path, "10\n") catch {};
    }
    {
        var buf: [256]u8 = undefined;
        const path = std.fmt.bufPrint(&buf, "{s}/eth0/statistics/tx_packets", .{base}) catch unreachable;
        writeFile(path, "20\n") catch {};
    }

    try std.testing.expectError(error.StatFileMissing, readInterfaceStats(allocator, base, "eth0"));
}

test "readInterfaceStats: returns error on invalid counter contents" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_stats_test5";

    makeDir(base) catch {};
    {
        var buf: [256]u8 = undefined;
        const path = std.fmt.bufPrint(&buf, "{s}/eth0", .{base}) catch unreachable;
        makeDir(path) catch {};
    }
    {
        var buf: [256]u8 = undefined;
        const path = std.fmt.bufPrint(&buf, "{s}/eth0/statistics", .{base}) catch unreachable;
        makeDir(path) catch {};
    }
    defer deleteTree(base) catch {};

    {
        var buf: [256]u8 = undefined;
        const path = std.fmt.bufPrint(&buf, "{s}/eth0/statistics/rx_bytes", .{base}) catch unreachable;
        writeFile(path, "abc\n") catch {};
    }
    {
        var buf: [256]u8 = undefined;
        const path = std.fmt.bufPrint(&buf, "{s}/eth0/statistics/tx_bytes", .{base}) catch unreachable;
        writeFile(path, "200\n") catch {};
    }
    {
        var buf: [256]u8 = undefined;
        const path = std.fmt.bufPrint(&buf, "{s}/eth0/statistics/rx_packets", .{base}) catch unreachable;
        writeFile(path, "10\n") catch {};
    }
    {
        var buf: [256]u8 = undefined;
        const path = std.fmt.bufPrint(&buf, "{s}/eth0/statistics/tx_packets", .{base}) catch unreachable;
        writeFile(path, "20\n") catch {};
    }

    try std.testing.expectError(error.InvalidCounter, readInterfaceStats(allocator, base, "eth0"));
}

// ============================================================================
// Linux-only Smoke Test (ACT 5c: Live sysfs reader exercise)
// ============================================================================
//
// This test exercises the Zig sysfs reader against real /sys/class/net on Linux.
// It is compile-gated to Linux only and skips gracefully when:
//   - /sys/class/net does not exist (container without sysfs)
//   - No interfaces have statistics directories
//   - Interfaces exist but are unreadable (permission denied)
//
// Coverage impact: Exercises Linux syscalls (open, read, close) for live sysfs.
test "readInterfaceStats: live sysfs smoke test on Linux" {
    if (@import("builtin").os.tag != .linux) return error.SkipZigTest;

    const allocator = std.testing.allocator;

    // Check if /sys/class/net exists
    const sysfs_root = "/sys/class/net";
    if (!linux_stats.fileExists(sysfs_root)) {
        // Skip in container without sysfs — not a failure
        return error.SkipZigTest;
    }

    // Use opendir/readdir to find interfaces with statistics directories
    const dir = std.c.opendir(sysfs_root);
    if (dir == null) {
        // Cannot open /sys/class/net — skip
        return error.SkipZigTest;
    }
    defer _ = std.c.closedir(dir);

    var found_iface: [64]u8 = undefined;
    var found_len: usize = 0;
    var found = false;

    while (true) {
        const entry = std.c.readdir(dir);
        if (entry == null) break;

        // Get null-terminated name from dirent
        const name_ptr = @as([*:0]const u8, @ptrFromInt(@intFromPtr(entry) + @offsetOf(std.c.dirent, "d_name")));
        const name = std.mem.sliceTo(name_ptr, 0);

        // Skip . and ..
        if (name.len == 0 or (name.len == 1 and name[0] == '.')) continue;
        if (name.len == 2 and name[0] == '.' and name[1] == '.') continue;

        // Build path to statistics directory
        var stats_path_buf: [4096]u8 = undefined;
        const stats_path = std.fmt.bufPrint(&stats_path_buf, "{s}/{s}/statistics", .{ sysfs_root, name }) catch continue;

        if (linux_stats.fileExists(stats_path)) {
            // Found an interface with statistics — copy name
            if (name.len < found_iface.len) {
                @memcpy(found_iface[0..name.len], name);
                found_len = name.len;
                found = true;
                break; // Use first matching interface
            }
        }
    }

    if (!found) {
        // No interfaces with statistics found — skip
        return error.SkipZigTest;
    }

    const iface_name = found_iface[0..found_len];

    // Attempt to read stats from real sysfs.
    // Successful parse from real sysfs is the smoke assertion.
    _ = try readInterfaceStats(allocator, sysfs_root, iface_name);
}
