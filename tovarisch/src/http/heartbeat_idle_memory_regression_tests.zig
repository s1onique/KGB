// heartbeat_idle_memory_regression_tests.zig — Memory regression tests for heartbeat
//
// Regression coverage for explicit heartbeat tunnel-summary ownership and
// deterministic memory cleanup. Ensures collectTunnelSummaryWithStats()
// and freeTunnelSummarySnapshots() are used correctly in repeated heartbeat cycles.

const std = @import("std");
const heartbeat = @import("heartbeat.zig");
const linux_interface_stats = @import("../net/linux_interface_stats.zig");
const linux_stats = @import("../net/linux_stats.zig");

const makeDir = linux_stats.makeDir;
const deleteTree = linux_stats.deleteTree;
const writeFile = linux_stats.writeFile;

/// Helper: Create a fixture interface with statistics
fn createIfaceWithStats(base: []const u8, iface: []const u8, rx_bytes: u64, tx_bytes: u64, rx_packets: u64, tx_packets: u64) !void {
    var path_buf1: [4096]u8 = undefined;
    var path_buf2: [4096]u8 = undefined;
    var num_buf: [64]u8 = undefined;

    const iface_path = try std.fmt.bufPrint(&path_buf1, "{s}/{s}", .{ base, iface });
    try makeDir(iface_path);

    const stats_path = try std.fmt.bufPrint(&path_buf2, "{s}/statistics", .{iface_path});
    try makeDir(stats_path);

    var rx_bytes_path_buf: [4096]u8 = undefined;
    var tx_bytes_path_buf: [4096]u8 = undefined;
    var rx_packets_path_buf: [4096]u8 = undefined;
    var tx_packets_path_buf: [4096]u8 = undefined;

    const rx_bytes_path = try std.fmt.bufPrint(&rx_bytes_path_buf, "{s}/rx_bytes", .{stats_path});
    const tx_bytes_path = try std.fmt.bufPrint(&tx_bytes_path_buf, "{s}/tx_bytes", .{stats_path});
    const rx_packets_path = try std.fmt.bufPrint(&rx_packets_path_buf, "{s}/rx_packets", .{stats_path});
    const tx_packets_path = try std.fmt.bufPrint(&tx_packets_path_buf, "{s}/tx_packets", .{stats_path});

    const rx_bytes_str = try std.fmt.bufPrint(&num_buf, "{d}\n", .{rx_bytes});
    try writeFile(rx_bytes_path, rx_bytes_str);

    const tx_bytes_str = try std.fmt.bufPrint(&num_buf, "{d}\n", .{tx_bytes});
    try writeFile(tx_bytes_path, tx_bytes_str);

    const rx_packets_str = try std.fmt.bufPrint(&num_buf, "{d}\n", .{rx_packets});
    try writeFile(rx_packets_path, rx_packets_str);

    const tx_packets_str = try std.fmt.bufPrint(&num_buf, "{d}\n", .{tx_packets});
    try writeFile(tx_packets_path, tx_packets_str);
}

// ============================================================================
// Memory Regression Tests
// ============================================================================

test "collectTunnelSummaryWithStats: repeated calls with freeing do NOT leak memory" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_heartbeat_memory_leak_test";

    try makeDir(base);
    defer deleteTree(base) catch {};

    try createIfaceWithStats(base, "wg0", 1000, 2000, 10, 20);
    try createIfaceWithStats(base, "wg1", 3000, 4000, 30, 40);
    try createIfaceWithStats(base, "eth0", 500, 600, 5, 6);

    // Warmup cycles
    for (0..3) |_| {
        const result = heartbeat.collectTunnelSummaryWithStats(allocator, base, .test_fixture);
        try std.testing.expect(result.stats.len > 0);
        heartbeat.freeTunnelSummarySnapshots(allocator, result);
    }

    // Simulate heartbeat cycles (30 seconds * 40 cycles = 20 minutes)
    for (0..40) |_| {
        const result = heartbeat.collectTunnelSummaryWithStats(allocator, base, .test_fixture);
        try std.testing.expect(result.summary.count >= 2);
        heartbeat.freeTunnelSummarySnapshots(allocator, result);
    }
}

test "collectTunnelSummary: single-shot use is safe" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_heartbeat_singleshot_test";

    try makeDir(base);
    defer deleteTree(base) catch {};

    try createIfaceWithStats(base, "wg0", 1000, 2000, 10, 20);

    // Use collectTunnelSummaryWithStats with test_fixture root for deterministic testing
    const result = heartbeat.collectTunnelSummaryWithStats(allocator, base, .test_fixture);
    defer heartbeat.freeTunnelSummarySnapshots(allocator, result);
    try std.testing.expectEqual(@as(u32, 1), result.summary.count);
    try std.testing.expectEqual(@as(u64, 1000), result.summary.rx_bytes);
    try std.testing.expectEqual(@as(u64, 2000), result.summary.tx_bytes);
}

test "freeTunnelSummarySnapshots: handles empty stats slice" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_heartbeat_empty_test";

    deleteTree(base) catch {};

    const result = heartbeat.collectTunnelSummaryWithStats(allocator, base, .test_fixture);
    try std.testing.expectEqual(@as(usize, 0), result.stats.len);
    try std.testing.expectEqual(@as(u32, 0), result.summary.count);
    heartbeat.freeTunnelSummarySnapshots(allocator, result);
}

test "collectTunnelSummaryWithStats: returns correct tunnel summary" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_heartbeat_summary_test";

    try makeDir(base);
    defer deleteTree(base) catch {};

    try createIfaceWithStats(base, "wg0", 100, 200, 10, 20);
    try createIfaceWithStats(base, "wg1", 300, 400, 30, 40);
    try createIfaceWithStats(base, "eth0", 5000, 6000, 50, 60);

    const result = heartbeat.collectTunnelSummaryWithStats(allocator, base, .test_fixture);
    defer heartbeat.freeTunnelSummarySnapshots(allocator, result);

    try std.testing.expectEqual(@as(u32, 2), result.summary.count);
    try std.testing.expectEqual(@as(u64, 100 + 300), result.summary.rx_bytes);
    try std.testing.expectEqual(@as(u64, 200 + 400), result.summary.tx_bytes);
    try std.testing.expectEqual(@as(usize, 3), result.stats.len);
}

test "heartbeat loop simulation: no memory growth after many cycles" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_heartbeat_loop_simulation";

    try makeDir(base);
    defer deleteTree(base) catch {};

    try createIfaceWithStats(base, "wg0", 1000, 2000, 10, 20);
    try createIfaceWithStats(base, "wg1", 3000, 4000, 30, 40);

    var previous_summary: heartbeat.TunnelSummary = undefined;
    var first_cycle = true;

    for (0..100) |_| {
        const result = heartbeat.collectTunnelSummaryWithStats(allocator, base, .test_fixture);

        if (first_cycle) {
            previous_summary = result.summary;
            first_cycle = false;
        } else {
            try std.testing.expectEqual(previous_summary.count, result.summary.count);
            try std.testing.expectEqual(previous_summary.rx_bytes, result.summary.rx_bytes);
            try std.testing.expectEqual(previous_summary.tx_bytes, result.summary.tx_bytes);
        }

        heartbeat.freeTunnelSummarySnapshots(allocator, result);
    }
}
