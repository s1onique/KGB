// metrics_conversion_tests.zig — Tests for snapshot-to-sampled conversion
//
// ACT 4a: Tests for InterfaceStatsSnapshot -> SampledInterface conversion.
//
// Tests cover:
// 1. Converting one snapshot with rate = null
// 2. Converting two snapshots preserving order
// 3. Name duplication (not borrowed) — proves lifetime via mutation
// 4. Empty snapshots returns empty slice
//
// This file is imported by test_all.zig and refAllDecls forces test discovery.

const std = @import("std");
const testing = std.testing;
const metrics = @import("metrics.zig");

// Re-use types and helpers
const InterfaceStatsSnapshot = metrics.InterfaceStatsSnapshot;
const sampledInterfacesFromSnapshots = metrics.sampledInterfacesFromSnapshots;
const freeSampledInterfaces = metrics.freeSampledInterfaces;

// ============================================================================
// Tests: Conversion from snapshots to sampled interfaces (ACT 4a)
// ============================================================================

test "sampledInterfacesFromSnapshots: converts one snapshot with rate null" {
    const name = try testing.allocator.dupe(u8, "eth0");
    defer testing.allocator.free(name);
    const snapshots = [_]InterfaceStatsSnapshot{
        .{
            .name = name,
            .stats = .{
                .rx_bytes = 1000,
                .tx_bytes = 2000,
                .rx_packets = 10,
                .tx_packets = 20,
            },
        },
    };

    const sampled = try sampledInterfacesFromSnapshots(testing.allocator, &snapshots);
    defer freeSampledInterfaces(testing.allocator, sampled);

    try testing.expectEqual(@as(usize, 1), sampled.len);
    try testing.expectEqualStrings("eth0", sampled[0].sample.name);
    try testing.expectEqual(@as(u64, 1000), sampled[0].sample.rx_bytes);
    try testing.expectEqual(@as(u64, 2000), sampled[0].sample.tx_bytes);
    try testing.expectEqual(@as(u64, 10), sampled[0].sample.rx_packets);
    try testing.expectEqual(@as(u64, 20), sampled[0].sample.tx_packets);
    try testing.expectEqual(@as(i64, 0), sampled[0].sample.sampled_at_ms);
    try testing.expect(sampled[0].rate == null);
}

test "sampledInterfacesFromSnapshots: converts two snapshots preserving order" {
    const eth0_name = try testing.allocator.dupe(u8, "eth0");
    defer testing.allocator.free(eth0_name);
    const wg0_name = try testing.allocator.dupe(u8, "wg0");
    defer testing.allocator.free(wg0_name);
    const snapshots = [_]InterfaceStatsSnapshot{
        .{
            .name = eth0_name,
            .stats = .{ .rx_bytes = 1000, .tx_bytes = 2000, .rx_packets = 10, .tx_packets = 20 },
        },
        .{
            .name = wg0_name,
            .stats = .{ .rx_bytes = 3000, .tx_bytes = 4000, .rx_packets = 30, .tx_packets = 40 },
        },
    };

    const sampled = try sampledInterfacesFromSnapshots(testing.allocator, &snapshots);
    defer freeSampledInterfaces(testing.allocator, sampled);

    try testing.expectEqual(@as(usize, 2), sampled.len);
    try testing.expectEqualStrings("eth0", sampled[0].sample.name);
    try testing.expectEqualStrings("wg0", sampled[1].sample.name);
    try testing.expectEqual(@as(u64, 1000), sampled[0].sample.rx_bytes);
    try testing.expectEqual(@as(u64, 3000), sampled[1].sample.rx_bytes);
}

test "sampledInterfacesFromSnapshots: names are duplicated not borrowed (proved via mutation)" {
    // Create a mutable buffer on the heap for the source name
    const name_buf = try testing.allocator.alloc(u8, 4);
    defer testing.allocator.free(name_buf);
    @memcpy(name_buf, "eth0");

    // Point a slice at the buffer
    const name_slice: []u8 = name_buf[0..4];

    const snapshots = [_]InterfaceStatsSnapshot{
        .{
            .name = name_slice,
            .stats = .{ .rx_bytes = 100, .tx_bytes = 200, .rx_packets = 1, .tx_packets = 2 },
        },
    };

    // Convert snapshots to sampled interfaces
    const sampled = try sampledInterfacesFromSnapshots(testing.allocator, &snapshots);
    defer freeSampledInterfaces(testing.allocator, sampled);

    // Verify the sampled name is the original value before mutation
    try testing.expectEqualStrings("eth0", sampled[0].sample.name);

    // Mutate the ORIGINAL buffer (simulate the source being freed/changed)
    @memcpy(name_buf, "XXXX");

    // The sampled name should still be "eth0" — it was duplicated, not borrowed
    // This proves the sampled result owns its own copy of the name
    try testing.expectEqualStrings("eth0", sampled[0].sample.name);
}

test "sampledInterfacesFromSnapshots: empty snapshots returns empty slice" {
    const snapshots: [0]InterfaceStatsSnapshot = .{};
    const sampled = try sampledInterfacesFromSnapshots(testing.allocator, &snapshots);
    defer freeSampledInterfaces(testing.allocator, sampled);

    try testing.expectEqual(@as(usize, 0), sampled.len);
}
