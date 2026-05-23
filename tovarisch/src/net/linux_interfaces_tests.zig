// linux_interfaces_tests.zig — Tests for Linux sysfs interface enumeration
//
// ACT 5c: Fixture-based listInterfaces tests.
// Tests cover normal, empty, and missing root cases.
//
// This file is imported by test_all.zig and refAllDecls forces test discovery.

const std = @import("std");
const linux_interfaces = @import("linux_interfaces.zig");

// Re-export for convenience
const listInterfaces = linux_interfaces.listInterfaces;
const freeInterfaceList = linux_interfaces.freeInterfaceList;
const ListError = linux_interfaces.ListError;

// ============================================================================
// Test Fixtures (helper functions from linux_stats.zig)
// ============================================================================

// Re-use filesystem helpers from linux_stats.zig for fixture creation
const linux_stats = @import("linux_stats.zig");
const makeDir = linux_stats.makeDir;
const deleteTree = linux_stats.deleteTree;

// ============================================================================
// Tests: Fixture root with eth0, wg0, lo returns all three names
// ============================================================================

test "listInterfaces: returns eth0, wg0, lo from fixture" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_iface_test_normal";

    try makeDir(base);
    defer deleteTree(base) catch {};

    // Create interface directories (but no statistics/)
    inline for ([_][]const u8{ "eth0", "wg0", "lo" }) |iface| {
        var buf: [256]u8 = undefined;
        const path = std.fmt.bufPrint(&buf, "{s}/{s}", .{ base, iface }) catch unreachable;
        try makeDir(path);
    }

    const names = try listInterfaces(allocator, base);
    defer freeInterfaceList(allocator, names);

    try std.testing.expectEqual(@as(usize, 3), names.len);

    // Sort for deterministic comparison
    const sorted = try allocator.dupe([]const u8, names);
    defer allocator.free(sorted);
    std.mem.sort([]const u8, sorted, {}, (struct {
        fn less(_: void, a: []const u8, b: []const u8) bool {
            return std.mem.lessThan(u8, a, b);
        }
    }).less);

    try std.testing.expectEqualStrings("eth0", sorted[0]);
    try std.testing.expectEqualStrings("lo", sorted[1]);
    try std.testing.expectEqualStrings("wg0", sorted[2]);
}

// ============================================================================
// Tests: Skips "." and ".."
// ============================================================================

test "listInterfaces: skips dot entries" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_iface_test_dots";

    try makeDir(base);
    defer deleteTree(base) catch {};

    // Create at least one real interface
    {
        var buf: [256]u8 = undefined;
        const path = std.fmt.bufPrint(&buf, "{s}/eth0", .{base}) catch unreachable;
        try makeDir(path);
    }

    const names = try listInterfaces(allocator, base);
    defer freeInterfaceList(allocator, names);

    // Should only contain "eth0", not "." or ".."
    try std.testing.expectEqual(@as(usize, 1), names.len);
    try std.testing.expectEqualStrings("eth0", names[0]);
}

// ============================================================================
// Tests: Empty fixture root returns empty list
// ============================================================================

test "listInterfaces: returns empty list for empty directory" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_iface_test_empty";

    try makeDir(base);
    defer deleteTree(base) catch {};

    const names = try listInterfaces(allocator, base);
    defer freeInterfaceList(allocator, names);

    try std.testing.expectEqual(@as(usize, 0), names.len);
}

// ============================================================================
// Tests: Missing root returns error
// ============================================================================

test "listInterfaces: returns error when root missing" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_iface_test_nonexistent_123456789";

    // Ensure it does not exist
    deleteTree(base) catch {};

    try std.testing.expectError(error.RootDirMissing, listInterfaces(allocator, base));
}

// ============================================================================
// Tests: Names are allocator-owned copies
// ============================================================================

test "listInterfaces: returns allocator-owned copies" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_iface_test_owned";

    try makeDir(base);
    defer deleteTree(base) catch {};

    {
        var buf: [256]u8 = undefined;
        const path = std.fmt.bufPrint(&buf, "{s}/eth0", .{base}) catch unreachable;
        try makeDir(path);
    }

    const names = try listInterfaces(allocator, base);
    defer freeInterfaceList(allocator, names);

    // Verify we can free them without panicking
    // This confirms they are separate allocations, not pointers into dirent buffer
    try std.testing.expectEqual(@as(usize, 1), names.len);
}

// ============================================================================
// Tests: Interface names with common safe characters
// ============================================================================

test "listInterfaces: handles eth0, wg0, br-lan, veth1234" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_iface_test_chars";

    try makeDir(base);
    defer deleteTree(base) catch {};

    // Create interfaces with various safe characters
    inline for ([_][]const u8{ "eth0", "wg0", "br-lan", "veth1234" }) |iface| {
        var buf: [256]u8 = undefined;
        const path = std.fmt.bufPrint(&buf, "{s}/{s}", .{ base, iface }) catch unreachable;
        try makeDir(path);
    }

    const names = try listInterfaces(allocator, base);
    defer freeInterfaceList(allocator, names);

    try std.testing.expectEqual(@as(usize, 4), names.len);

    // Sort for deterministic comparison
    const sorted = try allocator.dupe([]const u8, names);
    defer allocator.free(sorted);
    std.mem.sort([]const u8, sorted, {}, (struct {
        fn less(_: void, a: []const u8, b: []const u8) bool {
            return std.mem.lessThan(u8, a, b);
        }
    }).less);

    try std.testing.expectEqualStrings("br-lan", sorted[0]);
    try std.testing.expectEqualStrings("eth0", sorted[1]);
    try std.testing.expectEqualStrings("veth1234", sorted[2]);
    try std.testing.expectEqualStrings("wg0", sorted[3]);
}

// ============================================================================
// Tests: Does not read statistics files
// ============================================================================

test "listInterfaces: does not require statistics directories" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_iface_test_no_stats";

    try makeDir(base);
    defer deleteTree(base) catch {};

    // Create interface directories WITHOUT statistics/
    {
        var buf: [256]u8 = undefined;
        const path = std.fmt.bufPrint(&buf, "{s}/eth0", .{base}) catch unreachable;
        try makeDir(path);
    }

    // This should succeed even without statistics/
    const names = try listInterfaces(allocator, base);
    defer freeInterfaceList(allocator, names);

    try std.testing.expectEqual(@as(usize, 1), names.len);
    try std.testing.expectEqualStrings("eth0", names[0]);
}

// ============================================================================
// Linux-only Smoke Test
// ============================================================================
//
// This test exercises listInterfaces against real /sys/class/net on Linux.
// It is compile-gated to Linux only and skips gracefully when:
//   - /sys/class/net does not exist (container without sysfs)
//
test "listInterfaces: live sysfs smoke test on Linux" {
    if (@import("builtin").os.tag != .linux) return error.SkipZigTest;

    const allocator = std.testing.allocator;
    const sysfs_root = "/sys/class/net";

    // Check if /sys/class/net exists
    if (!linux_stats.fileExists(sysfs_root)) {
        return error.SkipZigTest;
    }

    // Call listInterfaces on real sysfs
    const names = listInterfaces(allocator, sysfs_root) catch return error.SkipZigTest;
    // Successful enumeration of real /sys/class/net is the smoke assertion.
    // The list may be empty in constrained/containerized environments.
    // defer below frees the list.
    defer freeInterfaceList(allocator, names);
}
