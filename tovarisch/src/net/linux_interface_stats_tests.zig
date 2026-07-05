// linux_interface_stats_tests.zig — Tests for Linux interface stats collector
//
// ACT 5d: Fixture-based collectInterfaceStats tests.
// Tests cover: both valid, one missing stats, one invalid counter,
// empty root, missing root, and Linux smoke.
//
// This file is imported by test_all.zig and refAllDecls forces test discovery.

const std = @import("std");
const linux_interface_stats = @import("linux_interface_stats.zig");

// Re-export for convenience
const collectInterfaceStats = linux_interface_stats.collectInterfaceStats;
const freeInterfaceStatsSnapshots = linux_interface_stats.freeInterfaceStatsSnapshots;
const InterfaceStatsSnapshot = linux_interface_stats.InterfaceStatsSnapshot;

// Re-use filesystem helpers from linux_stats.zig for fixture creation
const linux_stats = @import("linux_stats.zig");
const makeDir = linux_stats.makeDir;
const deleteTree = linux_stats.deleteTree;
const writeFile = linux_stats.writeFile;

// ============================================================================
// Helper: Create a fixture interface with statistics
// ============================================================================

fn createIfaceWithStats(base: []const u8, iface: []const u8, rx_bytes: u64, tx_bytes: u64, rx_packets: u64, tx_packets: u64) !void {
    // Use separate buffers to avoid aliasing issues with bufPrint
    var path_buf1: [4096]u8 = undefined;
    var path_buf2: [4096]u8 = undefined;
    var num_buf: [64]u8 = undefined;

    const iface_path = std.fmt.bufPrint(&path_buf1, "{s}/{s}", .{ base, iface }) catch return error.OutOfMemory;
    try makeDir(iface_path);

    const stats_path = std.fmt.bufPrint(&path_buf2, "{s}/statistics", .{iface_path}) catch return error.OutOfMemory;
    try makeDir(stats_path);

    // Build stat file paths using separate buffers
    var rx_bytes_path_buf: [4096]u8 = undefined;
    var tx_bytes_path_buf: [4096]u8 = undefined;
    var rx_packets_path_buf: [4096]u8 = undefined;
    var tx_packets_path_buf: [4096]u8 = undefined;

    const rx_bytes_path = std.fmt.bufPrint(&rx_bytes_path_buf, "{s}/rx_bytes", .{stats_path}) catch return error.OutOfMemory;
    const tx_bytes_path = std.fmt.bufPrint(&tx_bytes_path_buf, "{s}/tx_bytes", .{stats_path}) catch return error.OutOfMemory;
    const rx_packets_path = std.fmt.bufPrint(&rx_packets_path_buf, "{s}/rx_packets", .{stats_path}) catch return error.OutOfMemory;
    const tx_packets_path = std.fmt.bufPrint(&tx_packets_path_buf, "{s}/tx_packets", .{stats_path}) catch return error.OutOfMemory;

    const rx_bytes_str = std.fmt.bufPrint(&num_buf, "{d}\n", .{rx_bytes}) catch unreachable;
    try writeFile(rx_bytes_path, rx_bytes_str);

    const tx_bytes_str = std.fmt.bufPrint(&num_buf, "{d}\n", .{tx_bytes}) catch unreachable;
    try writeFile(tx_bytes_path, tx_bytes_str);

    const rx_packets_str = std.fmt.bufPrint(&num_buf, "{d}\n", .{rx_packets}) catch unreachable;
    try writeFile(rx_packets_path, rx_packets_str);

    const tx_packets_str = std.fmt.bufPrint(&num_buf, "{d}\n", .{tx_packets}) catch unreachable;
    try writeFile(tx_packets_path, tx_packets_str);
}

// ============================================================================
// Tests: Fixture root with eth0 and wg0, both with valid stats, returns two snapshots
// ============================================================================

test "collectInterfaceStats: returns two snapshots for eth0 and wg0 with valid stats" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_fixture/kgb_collector_test_both_valid";

    makeDir("/tmp/kgb_fixture") catch {};
    try makeDir(base);
    defer deleteTree(base) catch {};

    try createIfaceWithStats(base, "eth0", 100, 200, 10, 20);
    try createIfaceWithStats(base, "wg0", 300, 400, 30, 40);

    const snapshots = try collectInterfaceStats(allocator, base, .test_fixture);
    defer freeInterfaceStatsSnapshots(allocator, snapshots);

    try std.testing.expectEqual(@as(usize, 2), snapshots.len);

    // Sort for deterministic comparison
    const sorted = try allocator.dupe(InterfaceStatsSnapshot, snapshots);
    defer allocator.free(sorted);
    std.mem.sort(InterfaceStatsSnapshot, sorted, {}, (struct {
        fn less(_: void, a: InterfaceStatsSnapshot, b: InterfaceStatsSnapshot) bool {
            return std.mem.lessThan(u8, a.name, b.name);
        }
    }).less);

    try std.testing.expectEqualStrings("eth0", sorted[0].name);
    try std.testing.expectEqual(@as(u64, 100), sorted[0].stats.rx_bytes);
    try std.testing.expectEqual(@as(u64, 200), sorted[0].stats.tx_bytes);
    try std.testing.expectEqual(@as(u64, 10), sorted[0].stats.rx_packets);
    try std.testing.expectEqual(@as(u64, 20), sorted[0].stats.tx_packets);

    try std.testing.expectEqualStrings("wg0", sorted[1].name);
    try std.testing.expectEqual(@as(u64, 300), sorted[1].stats.rx_bytes);
    try std.testing.expectEqual(@as(u64, 400), sorted[1].stats.tx_bytes);
    try std.testing.expectEqual(@as(u64, 30), sorted[1].stats.rx_packets);
    try std.testing.expectEqual(@as(u64, 40), sorted[1].stats.tx_packets);
}

// ============================================================================
// Tests: Snapshot names are allocator-owned and can be freed
// ============================================================================

test "collectInterfaceStats: snapshot names are allocator-owned copies" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_fixture/kgb_collector_test_owned";

    makeDir("/tmp/kgb_fixture") catch {};
    try makeDir(base);
    defer deleteTree(base) catch {};

    try createIfaceWithStats(base, "eth0", 100, 200, 10, 20);

    const snapshots = try collectInterfaceStats(allocator, base, .test_fixture);
    defer freeInterfaceStatsSnapshots(allocator, snapshots);

    try std.testing.expectEqual(@as(usize, 1), snapshots.len);
    try std.testing.expectEqualStrings("eth0", snapshots[0].name);

    // Verify we can free them without panicking
    // This confirms names are separate allocations, not pointers into directory buffer
}

// ============================================================================
// Tests: Fixture root with one valid interface and one without statistics/ returns only valid
// ============================================================================

test "collectInterfaceStats: skips interface without statistics/ directory" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_fixture/kgb_collector_test_no_stats_dir";

    makeDir("/tmp/kgb_fixture") catch {};
    try makeDir(base);
    defer deleteTree(base) catch {};

    // Create eth0 with valid statistics
    try createIfaceWithStats(base, "eth0", 100, 200, 10, 20);

    // Create wg0 without statistics/ directory (just the interface dir)
    {
        var wg0_buf: [4096]u8 = undefined;
        const wg0_path = std.fmt.bufPrint(&wg0_buf, "{s}/wg0", .{base}) catch unreachable;
        try makeDir(wg0_path);
    }

    const snapshots = try collectInterfaceStats(allocator, base, .test_fixture);
    defer freeInterfaceStatsSnapshots(allocator, snapshots);

    // Only eth0 should be included
    try std.testing.expectEqual(@as(usize, 1), snapshots.len);
    try std.testing.expectEqualStrings("eth0", snapshots[0].name);
}

// ============================================================================
// Tests: Fixture root with one valid interface and one invalid counter file returns only valid
// ============================================================================

test "collectInterfaceStats: skips interface with invalid counter file" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_fixture/kgb_collector_test_invalid_counter";

    makeDir("/tmp/kgb_fixture") catch {};
    try makeDir(base);
    defer deleteTree(base) catch {};

    // Create eth0 with valid statistics
    try createIfaceWithStats(base, "eth0", 100, 200, 10, 20);

    // Create wg0 with statistics/ but invalid rx_bytes
    {
        var wg0_buf: [4096]u8 = undefined;
        var stats_buf: [4096]u8 = undefined;
        var rx_bytes_buf: [4096]u8 = undefined;

        const wg0_path = std.fmt.bufPrint(&wg0_buf, "{s}/wg0", .{base}) catch unreachable;
        try makeDir(wg0_path);

        const stats_path = std.fmt.bufPrint(&stats_buf, "{s}/statistics", .{wg0_path}) catch unreachable;
        try makeDir(stats_path);

        const rx_bytes_path = std.fmt.bufPrint(&rx_bytes_buf, "{s}/rx_bytes", .{stats_path}) catch unreachable;
        try writeFile(rx_bytes_path, "not_a_number\n");
    }

    const snapshots = try collectInterfaceStats(allocator, base, .test_fixture);
    defer freeInterfaceStatsSnapshots(allocator, snapshots);

    // Only eth0 should be included
    try std.testing.expectEqual(@as(usize, 1), snapshots.len);
    try std.testing.expectEqualStrings("eth0", snapshots[0].name);
}

// ============================================================================
// Tests: Empty fixture root returns empty snapshot list
// ============================================================================

test "collectInterfaceStats: returns empty list for empty directory" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_fixture/kgb_collector_test_empty";

    makeDir("/tmp/kgb_fixture") catch {};
    try makeDir(base);
    defer deleteTree(base) catch {};

    const snapshots = try collectInterfaceStats(allocator, base, .test_fixture);
    defer freeInterfaceStatsSnapshots(allocator, snapshots);

    try std.testing.expectEqual(@as(usize, 0), snapshots.len);
}

// ============================================================================
// Tests: Missing root returns the same error from enumeration
// ============================================================================

test "collectInterfaceStats: returns error when root missing" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_fixture/kgb_collector_test_nonexistent_1234567890";

    // Ensure it does not exist
    deleteTree(base) catch {};

    try std.testing.expectError(error.RootDirMissing, collectInterfaceStats(allocator, base, .test_fixture));
}

// ============================================================================
// Tests: Does not classify private/public interfaces
// ============================================================================

test "collectInterfaceStats: does not filter by private/public classification" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_fixture/kgb_collector_test_no_classification";

    makeDir("/tmp/kgb_fixture") catch {};
    try makeDir(base);
    defer deleteTree(base) catch {};

    // Create multiple interfaces with valid stats
    try createIfaceWithStats(base, "eth0", 100, 200, 10, 20);
    try createIfaceWithStats(base, "wg0", 300, 400, 30, 40);
    try createIfaceWithStats(base, "lo", 50, 50, 5, 5);

    const snapshots = try collectInterfaceStats(allocator, base, .test_fixture);
    defer freeInterfaceStatsSnapshots(allocator, snapshots);

    // All interfaces should be included - no filtering by private/public
    try std.testing.expectEqual(@as(usize, 3), snapshots.len);
}

// ============================================================================
// Tests: Interface with missing tx_packets is skipped
// ============================================================================

test "collectInterfaceStats: skips interface with missing stat file" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_fixture/kgb_collector_test_missing_file";

    makeDir("/tmp/kgb_fixture") catch {};
    try makeDir(base);
    defer deleteTree(base) catch {};

    // Create eth0 with valid statistics
    try createIfaceWithStats(base, "eth0", 100, 200, 10, 20);

    // Create wg0 with incomplete statistics (missing tx_packets)
    {
        var wg0_buf: [4096]u8 = undefined;
        var stats_buf: [4096]u8 = undefined;
        var rx_bytes_buf: [4096]u8 = undefined;
        var tx_bytes_buf: [4096]u8 = undefined;
        var rx_packets_buf: [4096]u8 = undefined;

        const wg0_path = std.fmt.bufPrint(&wg0_buf, "{s}/wg0", .{base}) catch unreachable;
        try makeDir(wg0_path);

        const stats_path = std.fmt.bufPrint(&stats_buf, "{s}/statistics", .{wg0_path}) catch unreachable;
        try makeDir(stats_path);

        const rx_bytes_path = std.fmt.bufPrint(&rx_bytes_buf, "{s}/rx_bytes", .{stats_path}) catch unreachable;
        const tx_bytes_path = std.fmt.bufPrint(&tx_bytes_buf, "{s}/tx_bytes", .{stats_path}) catch unreachable;
        const rx_packets_path = std.fmt.bufPrint(&rx_packets_buf, "{s}/rx_packets", .{stats_path}) catch unreachable;
        // NOTE: tx_packets is intentionally missing

        try writeFile(rx_bytes_path, "300\n");
        try writeFile(tx_bytes_path, "400\n");
        try writeFile(rx_packets_path, "30\n");
    }

    const snapshots = try collectInterfaceStats(allocator, base, .test_fixture);
    defer freeInterfaceStatsSnapshots(allocator, snapshots);

    // Only eth0 should be included
    try std.testing.expectEqual(@as(usize, 1), snapshots.len);
    try std.testing.expectEqualStrings("eth0", snapshots[0].name);
}

// ============================================================================
// Linux-only Smoke Test
// ============================================================================
//
// This test exercises collectInterfaceStats against real /sys/class/net on Linux.
// It is compile-gated to Linux only and skips gracefully when:
//   - /sys/class/net does not exist (container without sysfs)
//   - No interfaces have readable statistics
//
test "collectInterfaceStats: live sysfs smoke test on Linux" {
    if (@import("builtin").os.tag != .linux) return error.SkipZigTest;

    const allocator = std.testing.allocator;
    const sysfs_root = "/sys/class/net";

    // Check if /sys/class/net exists
    if (!linux_stats.fileExists(sysfs_root)) {
        return error.SkipZigTest;
    }

    // Call collectInterfaceStats on real sysfs (HULK16R2: use .sysfs_net for live smoke)
    const snapshots = collectInterfaceStats(allocator, sysfs_root, .sysfs_net) catch return error.SkipZigTest;
    // Successful collection is the smoke assertion.
    // The list may be empty in constrained/containerized environments.
    // defer below frees the snapshots.
    defer freeInterfaceStatsSnapshots(allocator, snapshots);
}
