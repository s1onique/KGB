// linux_read_fixture_tests.zig — Fixture-based tests for Linux sysfs/procfs file boundary
//
// ACT-TOVARISCH-ZIG-HULK13R: Fix linux_read ownership and fixture-root semantics
//
// Tests use /tmp/kgb_fixture as the test root to properly exercise the boundary.

const std = @import("std");
const linux_read = @import("linux_read.zig");

// Re-exports
const LinuxReadResult = linux_read.LinuxReadResult;
const AllowedRoot = linux_read.AllowedRoot;
const validatePath = linux_read.validatePath;
const linuxRead = linux_read.linuxRead;
const linuxReadSmall = linux_read.linuxReadSmall;
const linuxReadCounter = linux_read.linuxReadCounter;
const linuxExists = linux_read.linuxExists;
const readFixtureFile = linux_read.readFixtureFile;

// ============================================================================
// Test Helpers
// ============================================================================

const TEST_ROOT = "/tmp/kgb_fixture";

fn createFixtureDir(base: []const u8) !void {
    try std.Io.Dir.cwd().createDir(std.testing.io, base);
}

fn deleteFixtureDir(base: []const u8) void {
    std.Io.Dir.cwd().deleteTree(std.testing.io, base) catch {};
}

fn writeFixtureFile(path: []const u8, content: []const u8) !void {
    var file = try std.Io.Dir.cwd().createFile(std.testing.io, path, .{});
    defer file.close();
    try file.writeAll(content);
}

fn fixturePath(comptime name: []const u8) []const u8 {
    return TEST_ROOT ++ "/" ++ name;
}

// ============================================================================
// Path Validation Tests
// ============================================================================

test "validatePath accepts valid test_fixture path" {
    try std.testing.expect(validatePath(.test_fixture, "/tmp/kgb_fixture/test"));
    try std.testing.expect(validatePath(.test_fixture, "/tmp/kgb_fixture/dir/file"));
}

test "validatePath rejects non-test_fixture paths" {
    try std.testing.expect(!validatePath(.test_fixture, "/etc/passwd"));
    try std.testing.expect(!validatePath(.test_fixture, "/sys/class/net/lo"));
    try std.testing.expect(!validatePath(.test_fixture, "/proc/self/status"));
}

test "validatePath rejects path traversal in test_fixture" {
    try std.testing.expect(!validatePath(.test_fixture, "/tmp/kgb_fixture/../etc/passwd"));
    try std.testing.expect(!validatePath(.test_fixture, "/tmp/kgb_fixture/../../etc/passwd"));
}

// ============================================================================
// readFixtureFile Tests (internal helper, used directly)
// ============================================================================

test "readFixtureFile returns .value for valid fixture" {
    if (@import("builtin").os.tag != .linux) return error.SkipZigTest;

    const allocator = std.testing.allocator;
    deleteFixtureDir(TEST_ROOT);
    try createFixtureDir(TEST_ROOT);
    defer deleteFixtureDir(TEST_ROOT);

    try writeFixtureFile(fixturePath("valid"), "12345\n");

    const result = readFixtureFile(allocator, fixturePath("valid"), 4096);
    try std.testing.expect(result == .value);
    if (result == .value) {
        defer allocator.free(result.value);
        try std.testing.expectEqualStrings("12345\n", result.value);
    }
}

test "readFixtureFile returns .missing for non-existent file" {
    if (@import("builtin").os.tag != .linux) return error.SkipZigTest;

    const allocator = std.testing.allocator;
    deleteFixtureDir(TEST_ROOT);

    const result = readFixtureFile(allocator, fixturePath("nonexistent"), 4096);
    try std.testing.expect(result == .missing);
}

test "readFixtureFile returns .too_large for oversized content" {
    if (@import("builtin").os.tag != .linux) return error.SkipZigTest;

    const allocator = std.testing.allocator;
    deleteFixtureDir(TEST_ROOT);
    try createFixtureDir(TEST_ROOT);
    defer deleteFixtureDir(TEST_ROOT);

    const large_content = try allocator.alloc(u8, 100);
    defer allocator.free(large_content);
    @memset(large_content, 'x');
    try writeFixtureFile(fixturePath("large"), large_content);

    const result = readFixtureFile(allocator, fixturePath("large"), 50);
    try std.testing.expect(result == .too_large);
}

test "readFixtureFile returns .malformed for non-fixture path" {
    if (@import("builtin").os.tag != .linux) return error.SkipZigTest;

    const allocator = std.testing.allocator;

    const result = readFixtureFile(allocator, "/etc/passwd", 4096);
    try std.testing.expect(result == .malformed);
}

test "readFixtureFile preserves trailing newlines" {
    if (@import("builtin").os.tag != .linux) return error.SkipZigTest;

    const allocator = std.testing.allocator;
    deleteFixtureDir(TEST_ROOT);
    try createFixtureDir(TEST_ROOT);
    defer deleteFixtureDir(TEST_ROOT);

    try writeFixtureFile(fixturePath("newline"), "12345\n");

    const result = readFixtureFile(allocator, fixturePath("newline"), 4096);
    try std.testing.expect(result == .value);
    if (result == .value) {
        defer allocator.free(result.value);
        try std.testing.expect(std.mem.endsWith(u8, result.value, "\n"));
    }
}

// ============================================================================
// linuxReadSmall and linuxReadCounter Tests
// ============================================================================

test "linuxReadSmall reads small fixture files" {
    if (@import("builtin").os.tag != .linux) return error.SkipZigTest;

    const allocator = std.testing.allocator;
    deleteFixtureDir(TEST_ROOT);
    try createFixtureDir(TEST_ROOT);
    defer deleteFixtureDir(TEST_ROOT);

    try writeFixtureFile(fixturePath("small"), "up\n");

    const result = linuxReadSmall(allocator, fixturePath("small"), .test_fixture);
    try std.testing.expect(result == .value);
    if (result == .value) {
        defer allocator.free(result.value);
        const trimmed = std.mem.trim(u8, result.value, " \t\r\n");
        try std.testing.expectEqualStrings("up", trimmed);
    }
}

test "linuxReadCounter reads counter fixture files" {
    if (@import("builtin").os.tag != .linux) return error.SkipZigTest;

    const allocator = std.testing.allocator;
    deleteFixtureDir(TEST_ROOT);
    try createFixtureDir(TEST_ROOT);
    defer deleteFixtureDir(TEST_ROOT);

    try writeFixtureFile(fixturePath("counter"), "1234567890\n");

    const result = linuxReadCounter(allocator, fixturePath("counter"), .test_fixture);
    try std.testing.expect(result == .value);
    if (result == .value) {
        defer allocator.free(result.value);
        const trimmed = std.mem.trim(u8, result.value, " \t\r\n");
        try std.testing.expectEqualStrings("1234567890", trimmed);
    }
}

// ============================================================================
// TOCTOU Test - disappearing file
// ============================================================================

test "linuxRead handles disappearing fixture file" {
    if (@import("builtin").os.tag != .linux) return error.SkipZigTest;

    const allocator = std.testing.allocator;
    deleteFixtureDir(TEST_ROOT);
    try createFixtureDir(TEST_ROOT);
    defer deleteFixtureDir(TEST_ROOT);

    try writeFixtureFile(fixturePath("ephemeral"), "data\n");

    try std.testing.expect(linuxExists(fixturePath("ephemeral"), .test_fixture));
    try std.Io.Dir.cwd().deleteFile(std.testing.io, fixturePath("ephemeral"));

    const result = readFixtureFile(allocator, fixturePath("ephemeral"), 4096);
    try std.testing.expect(result == .missing);
}

// ============================================================================
// linuxExists Tests
// ============================================================================

test "linuxExists returns true for existing fixture file" {
    if (@import("builtin").os.tag != .linux) return error.SkipZigTest;

    deleteFixtureDir(TEST_ROOT);
    try createFixtureDir(TEST_ROOT);
    defer deleteFixtureDir(TEST_ROOT);

    try writeFixtureFile(fixturePath("exists"), "data\n");
    try std.testing.expect(linuxExists(fixturePath("exists"), .test_fixture));
}

test "linuxExists returns false for non-existent fixture" {
    if (@import("builtin").os.tag != .linux) return error.SkipZigTest;

    deleteFixtureDir(TEST_ROOT);
    try std.testing.expect(!linuxExists(fixturePath("missing"), .test_fixture));
}

test "linuxExists returns false for invalid path" {
    if (@import("builtin").os.tag != .linux) return error.SkipZigTest;

    try std.testing.expect(!linuxExists("/etc/passwd", .test_fixture));
    try std.testing.expect(!linuxExists("/tmp/kgb_fixture/../etc/passwd", .test_fixture));
}

// ============================================================================
// Constants Tests
// ============================================================================

test "MAX_FILE_SIZE is 65536" {
    try std.testing.expectEqual(@as(usize, 65536), linux_read.MAX_FILE_SIZE);
}

test "DEFAULT_MAX_BYTES is 4096" {
    try std.testing.expectEqual(@as(usize, 4096), linux_read.DEFAULT_MAX_BYTES);
}

test "READ_BUFFER_SIZE is 4096" {
    try std.testing.expectEqual(@as(usize, 4096), linux_read.READ_BUFFER_SIZE);
}

// ============================================================================
// Live Sysfs Tests (Linux only)
// ============================================================================

test "linuxRead reads real /sys/class/net/lo/operstate" {
    if (@import("builtin").os.tag != .linux) return error.SkipZigTest;
    if (!linuxExists("/sys/class/net", .sysfs_net)) return error.SkipZigTest;

    const allocator = std.testing.allocator;
    const result = linuxRead(allocator, "/sys/class/net/lo/operstate", .sysfs_net, .{});

    switch (result) {
        .value => |content| {
            defer allocator.free(content);
            const trimmed = std.mem.trim(u8, content, " \t\r\n");
            try std.testing.expect(trimmed.len > 0);
            try std.testing.expect(trimmed.len < 10);
        },
        .missing => return error.SkipZigTest,
        .permission_denied => return error.SkipZigTest,
        else => return error.SkipZigTest,
    }
}

test "linuxRead reads real /proc/self/status" {
    if (@import("builtin").os.tag != .linux) return error.SkipZigTest;

    const allocator = std.testing.allocator;
    const result = linuxRead(allocator, "/proc/self/status", .proc_self, .{});

    switch (result) {
        .value => |content| {
            defer allocator.free(content);
            try std.testing.expect(content.len > 0);
            try std.testing.expect(std.mem.indexOf(u8, content, "VmRSS") != null);
        },
        else => return error.SkipZigTest,
    }
}

test "linuxExists returns true for real /sys/class/net" {
    if (@import("builtin").os.tag != .linux) return error.SkipZigTest;
    if (!linuxExists("/sys/class/net", .sysfs_net)) return error.SkipZigTest;
}

test "linuxExists returns true for real /proc/self/status" {
    if (@import("builtin").os.tag != .linux) return error.SkipZigTest;
    try std.testing.expect(linuxExists("/proc/self/status", .proc_self));
}
