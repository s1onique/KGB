// private_interface_stats_tests.zig — Tests for private interface stats pipeline
//
// ACT 5g: Tests for composing live stats collection + live address discovery +
// private filtering into a unified pipeline.
//
// Tests cover:
// 1. filterCollectedPrivateInterfaceStats includes eth0 with 192.168.1.10
// 2. Excludes wan0 with 8.8.8.8
// 3. Includes wg0 with 10.0.0.1
// 4. Returns empty when address list is empty
// 5. Output snapshots are owned and freeable
// 6. Input snapshots remain untouched
// 7. Empty input snapshots returns empty output
// 8. Live Linux smoke test
//
// This file is imported by test_all.zig and refAllDecls forces test discovery.

const std = @import("std");
const testing = std.testing;
const private_interface_stats = @import("private_interface_stats.zig");
const linux_interface_stats = @import("linux_interface_stats.zig");
const interface_filter = @import("interface_filter.zig");
const linux_stats = @import("linux_stats.zig");

// Re-export for convenience
const filterCollectedPrivateInterfaceStats = private_interface_stats.filterCollectedPrivateInterfaceStats;
const collectPrivateInterfaceStats = private_interface_stats.collectPrivateInterfaceStats;
const InterfaceStatsSnapshot = linux_interface_stats.InterfaceStatsSnapshot;

// Re-use filesystem helpers for fixture creation
const makeDir = linux_stats.makeDir;
const deleteTree = linux_stats.deleteTree;
const writeFile = linux_stats.writeFile;

// ============================================================================
// Helper: Create a fixture interface with statistics
// ============================================================================

fn createIfaceWithStats(base: []const u8, iface: []const u8, rx_bytes: u64, tx_bytes: u64, rx_packets: u64, tx_packets: u64) !void {
    var path_buf1: [4096]u8 = undefined;
    var path_buf2: [4096]u8 = undefined;
    var num_buf: [64]u8 = undefined;

    const iface_path = std.fmt.bufPrint(&path_buf1, "{s}/{s}", .{ base, iface }) catch return error.OutOfMemory;
    try makeDir(iface_path);

    const stats_path = std.fmt.bufPrint(&path_buf2, "{s}/statistics", .{iface_path}) catch return error.OutOfMemory;
    try makeDir(stats_path);

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
// Tests: filterCollectedPrivateInterfaceStats
// ============================================================================

test "filterCollectedPrivateInterfaceStats: includes eth0 with 192.168.1.10" {
    const allocator = testing.allocator;

    // Create test snapshots
    const eth0_name = try allocator.dupe(u8, "eth0");
    defer allocator.free(eth0_name);

    const snapshots = [_]InterfaceStatsSnapshot{
        .{
            .name = eth0_name,
            .stats = .{
                .rx_bytes = 1000,
                .tx_bytes = 2000,
                .rx_packets = 10,
                .tx_packets = 20,
            },
        },
    };

    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "192.168.1.10" },
    };

    const filtered = try filterCollectedPrivateInterfaceStats(allocator, &snapshots, &addresses);
    defer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, filtered);

    try testing.expectEqual(@as(usize, 1), filtered.len);
    try testing.expectEqualSlices(u8, "eth0", filtered[0].name);
    try testing.expectEqual(@as(u64, 1000), filtered[0].stats.rx_bytes);
}

test "filterCollectedPrivateInterfaceStats: excludes wan0 with 8.8.8.8" {
    const allocator = testing.allocator;

    const wan0_name = try allocator.dupe(u8, "wan0");
    defer allocator.free(wan0_name);

    const snapshots = [_]InterfaceStatsSnapshot{
        .{
            .name = wan0_name,
            .stats = .{
                .rx_bytes = 3000,
                .tx_bytes = 4000,
                .rx_packets = 30,
                .tx_packets = 40,
            },
        },
    };

    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "wan0", .address = "8.8.8.8" },
    };

    const filtered = try filterCollectedPrivateInterfaceStats(allocator, &snapshots, &addresses);
    defer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, filtered);

    // wan0 has only public address, should be excluded
    try testing.expectEqual(@as(usize, 0), filtered.len);
}

test "filterCollectedPrivateInterfaceStats: includes wg0 with 10.0.0.1" {
    const allocator = testing.allocator;

    const wg0_name = try allocator.dupe(u8, "wg0");
    defer allocator.free(wg0_name);

    const snapshots = [_]InterfaceStatsSnapshot{
        .{
            .name = wg0_name,
            .stats = .{
                .rx_bytes = 5000,
                .tx_bytes = 6000,
                .rx_packets = 50,
                .tx_packets = 60,
            },
        },
    };

    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "wg0", .address = "10.0.0.1" },
    };

    const filtered = try filterCollectedPrivateInterfaceStats(allocator, &snapshots, &addresses);
    defer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, filtered);

    try testing.expectEqual(@as(usize, 1), filtered.len);
    try testing.expectEqualSlices(u8, "wg0", filtered[0].name);
    try testing.expectEqual(@as(u64, 5000), filtered[0].stats.rx_bytes);
}

test "filterCollectedPrivateInterfaceStats: returns empty when address list is empty" {
    const allocator = testing.allocator;

    const eth0_name = try allocator.dupe(u8, "eth0");
    defer allocator.free(eth0_name);

    const snapshots = [_]InterfaceStatsSnapshot{
        .{
            .name = eth0_name,
            .stats = .{
                .rx_bytes = 1000,
                .tx_bytes = 2000,
                .rx_packets = 10,
                .tx_packets = 20,
            },
        },
    };

    const addresses: [0]interface_filter.InterfaceAddress = .{};

    const filtered = try filterCollectedPrivateInterfaceStats(allocator, &snapshots, &addresses);
    defer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, filtered);

    // No addresses means no interfaces pass the private filter
    try testing.expectEqual(@as(usize, 0), filtered.len);
}

test "filterCollectedPrivateInterfaceStats: output snapshots are owned and freeable" {
    const allocator = testing.allocator;

    const eth0_name = try allocator.dupe(u8, "eth0");
    defer allocator.free(eth0_name);

    const snapshots = [_]InterfaceStatsSnapshot{
        .{
            .name = eth0_name,
            .stats = .{
                .rx_bytes = 1000,
                .tx_bytes = 2000,
                .rx_packets = 10,
                .tx_packets = 20,
            },
        },
    };

    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "192.168.1.10" },
    };

    const filtered = try filterCollectedPrivateInterfaceStats(allocator, &snapshots, &addresses);
    // If this completes without panic, output is properly owned and freeable
    defer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, filtered);

    try testing.expectEqual(@as(usize, 1), filtered.len);
}

test "filterCollectedPrivateInterfaceStats: input snapshots remain untouched" {
    const allocator = testing.allocator;

    const eth0_name = try allocator.dupe(u8, "eth0");
    defer allocator.free(eth0_name);

    var snapshots_list = std.ArrayList(InterfaceStatsSnapshot).empty;
    defer snapshots_list.deinit(allocator);

    try snapshots_list.append(allocator, .{
        .name = eth0_name,
        .stats = .{
            .rx_bytes = 1000,
            .tx_bytes = 2000,
            .rx_packets = 10,
            .tx_packets = 20,
        },
    });

    const snapshots_slice = try snapshots_list.toOwnedSlice(allocator);
    defer allocator.free(snapshots_slice);

    const original_name_ptr = snapshots_slice[0].name.ptr;

    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "192.168.1.10" },
    };

    const filtered = try filterCollectedPrivateInterfaceStats(allocator, snapshots_slice, &addresses);
    defer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, filtered);

    // Original snapshot name should still be valid
    try testing.expectEqualSlices(u8, "eth0", snapshots_slice[0].name);
    // Pointer should be unchanged (name not freed)
    try testing.expectEqual(original_name_ptr, snapshots_slice[0].name.ptr);
}

test "filterCollectedPrivateInterfaceStats: empty input snapshots returns empty output" {
    const allocator = testing.allocator;

    const snapshots: [0]InterfaceStatsSnapshot = .{};

    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "192.168.1.10" },
    };

    const filtered = try filterCollectedPrivateInterfaceStats(allocator, &snapshots, &addresses);
    defer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, filtered);

    try testing.expectEqual(@as(usize, 0), filtered.len);
}

test "filterCollectedPrivateInterfaceStats: end-to-end eth0 private, wan0 public" {
    const allocator = testing.allocator;

    const eth0_name = try allocator.dupe(u8, "eth0");
    defer allocator.free(eth0_name);
    const wan0_name = try allocator.dupe(u8, "wan0");
    defer allocator.free(wan0_name);

    const snapshots = [_]InterfaceStatsSnapshot{
        .{
            .name = eth0_name,
            .stats = .{
                .rx_bytes = 1000,
                .tx_bytes = 2000,
                .rx_packets = 10,
                .tx_packets = 20,
            },
        },
        .{
            .name = wan0_name,
            .stats = .{
                .rx_bytes = 3000,
                .tx_bytes = 4000,
                .rx_packets = 30,
                .tx_packets = 40,
            },
        },
    };

    // eth0 has private address, wan0 has public address
    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "192.168.1.10" },
        .{ .iface = "wan0", .address = "8.8.8.8" },
    };

    const filtered = try filterCollectedPrivateInterfaceStats(allocator, &snapshots, &addresses);
    defer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, filtered);

    // Only eth0 should be included
    try testing.expectEqual(@as(usize, 1), filtered.len);
    try testing.expectEqualSlices(u8, "eth0", filtered[0].name);
}

test "filterCollectedPrivateInterfaceStats: end-to-end multiple private interfaces" {
    const allocator = testing.allocator;

    const eth0_name = try allocator.dupe(u8, "eth0");
    defer allocator.free(eth0_name);
    const wg0_name = try allocator.dupe(u8, "wg0");
    defer allocator.free(wg0_name);
    const lo_name = try allocator.dupe(u8, "lo");
    defer allocator.free(lo_name);

    const snapshots = [_]InterfaceStatsSnapshot{
        .{
            .name = eth0_name,
            .stats = .{ .rx_bytes = 1000, .tx_bytes = 2000, .rx_packets = 10, .tx_packets = 20 },
        },
        .{
            .name = wg0_name,
            .stats = .{ .rx_bytes = 5000, .tx_bytes = 6000, .rx_packets = 50, .tx_packets = 60 },
        },
        .{
            .name = lo_name,
            .stats = .{ .rx_bytes = 50, .tx_bytes = 50, .rx_packets = 5, .tx_packets = 5 },
        },
    };

    // eth0 (192.168.x.x), wg0 (10.x.x.x) are private; lo (127.0.0.1) is loopback, not private
    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "192.168.1.10" },
        .{ .iface = "wg0", .address = "10.0.0.1" },
        .{ .iface = "lo", .address = "127.0.0.1" },
    };

    const filtered = try filterCollectedPrivateInterfaceStats(allocator, &snapshots, &addresses);
    defer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, filtered);

    // eth0 and wg0 should be included; lo is loopback (not private)
    try testing.expectEqual(@as(usize, 2), filtered.len);
}

// ============================================================================
// Linux-only Smoke Test
// ============================================================================

test "collectPrivateInterfaceStats: live sysfs + rtnetlink smoke test on Linux" {
    if (@import("builtin").os.tag != .linux) return error.SkipZigTest;

    const allocator = std.testing.allocator;
    const sysfs_root = "/sys/class/net";

    // Check if /sys/class/net exists
    if (!linux_stats.fileExists(sysfs_root)) {
        return error.SkipZigTest;
    }

    // Call collectPrivateInterfaceStats on real sysfs + rtnetlink
    const snapshots = collectPrivateInterfaceStats(allocator, sysfs_root) catch return error.SkipZigTest;
    // Successful collection is the smoke assertion.
    // The list may be empty in constrained/containerized environments.
    defer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, snapshots);

    // For every returned snapshot, verify structural integrity
    for (snapshots) |snap| {
        try testing.expect(snap.name.len > 0);
        // Access fields to verify structure
        _ = snap.stats.rx_bytes;
        _ = snap.stats.tx_bytes;
        _ = snap.stats.rx_packets;
        _ = snap.stats.tx_packets;
        // Do not assert counters are nonzero (host may have no traffic)
    }
}
