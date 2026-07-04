// linux_read.zig — Linux sysfs/procfs file boundary helper
//
// ACT-TOVARISCH-ZIG-HULK13: Harden Linux sysfs/procfs read boundary
//
// Canonical boundary for all Linux runtime file reads from diagnostic collectors.
// This module is only valid on Linux; returns unsupported_platform on other platforms.

const std = @import("std");
const builtin = @import("builtin");

// ============================================================================
// Types
// ============================================================================

pub const LinuxReadResult = union(enum) {
    value: []const u8,
    missing,
    permission_denied,
    unsupported_platform,
    too_large,
    malformed,
    io_error,

    pub fn hasValue(self: LinuxReadResult) bool {
        return self == .value;
    }

    pub fn valueOrNull(self: LinuxReadResult) ?[]const u8 {
        if (self == .value) return self.value;
        return null;
    }

    pub fn free(self: LinuxReadResult, allocator: std.mem.Allocator) void {
        if (self == .value) allocator.free(self.value);
    }
};

pub const ReadConfig = struct {
    max_bytes: usize = 4096,
};

pub const AllowedRoot = enum {
    sysfs_net,
    proc_self,
    test_fixture,
};

pub const MAX_FILE_SIZE: usize = 65536;
pub const DEFAULT_MAX_BYTES: usize = 4096;
pub const READ_BUFFER_SIZE: usize = 4096;

// ============================================================================
// Path Validation
// ============================================================================

pub fn validatePath(root: AllowedRoot, path: []const u8) bool {
    const expected_prefix = rootPrefix(root);
    if (path.len < expected_prefix.len) return false;
    if (!std.mem.startsWith(u8, path, expected_prefix)) return false;
    if (path.len == expected_prefix.len) return false;
    if (path[expected_prefix.len] != '/') return false;

    var i: usize = expected_prefix.len + 1;
    while (i < path.len) : (i += 1) {
        if (path[i] == '.' and i + 1 < path.len and path[i + 1] == '.') {
            if (i + 2 >= path.len or path[i + 2] == '/') {
                return false;
            }
        }
    }
    return true;
}

fn rootPrefix(root: AllowedRoot) []const u8 {
    return switch (root) {
        .sysfs_net => "/sys/class/net",
        .proc_self => "/proc/self",
        .test_fixture => "/tmp/kgb_fixture",
    };
}

// ============================================================================
// Core Read Functions
// ============================================================================

pub fn linuxRead(
    allocator: std.mem.Allocator,
    path: []const u8,
    root: AllowedRoot,
    config: ReadConfig,
) LinuxReadResult {
    if (builtin.os.tag != .linux) {
        return .unsupported_platform;
    }
    if (!validatePath(root, path)) {
        return .malformed;
    }
    if (config.max_bytes == 0 or config.max_bytes > MAX_FILE_SIZE) {
        return .malformed;
    }
    return linuxReadLinux(allocator, path, config.max_bytes);
}

pub fn linuxReadSmall(
    allocator: std.mem.Allocator,
    path: []const u8,
    root: AllowedRoot,
) LinuxReadResult {
    return linuxRead(allocator, path, root, .{ .max_bytes = 32 });
}

pub fn linuxReadCounter(
    allocator: std.mem.Allocator,
    path: []const u8,
    root: AllowedRoot,
) LinuxReadResult {
    return linuxRead(allocator, path, root, .{ .max_bytes = 32 });
}

pub fn linuxExists(path: []const u8, root: AllowedRoot) bool {
    if (builtin.os.tag != .linux) return false;
    if (!validatePath(root, path)) return false;

    var path_buf: [4096]u8 = undefined;
    const c_path = toCString(path, &path_buf) catch return false;

    const flags = std.os.linux.O{ .ACCMODE = std.posix.ACCMODE.RDONLY };
    const fd = std.c.open(c_path, flags, @as(c_uint, 0));
    if (fd >= 0) {
        _ = std.c.close(fd);
        return true;
    }
    return false;
}

// ============================================================================
// Linux-only Implementation
// ============================================================================

fn linuxReadLinux(allocator: std.mem.Allocator, path: []const u8, max_bytes: usize) LinuxReadResult {
    var path_buf: [4096]u8 = undefined;
    const c_path = toCString(path, &path_buf) catch return .io_error;

    const flags = std.os.linux.O{ .ACCMODE = std.posix.ACCMODE.RDONLY };
    const fd = std.c.open(c_path, flags, @as(c_uint, 0));
    if (fd < 0) {
        const err = std.c.getErrno(fd);
        switch (err) {
            .NOENT => return .missing,
            .ACCES => return .permission_denied,
            else => return .io_error,
        }
    }
    defer _ = std.c.close(fd);

    const stat = std.c.fstat(fd) catch return .io_error;
    if (stat.size > 0 and @as(u64, @intCast(stat.size)) > max_bytes) {
        return .too_large;
    }

    var buf = allocator.alloc(u8, max_bytes) catch return .io_error;
    var owns_buf = true;
    defer if (owns_buf) allocator.free(buf);

    const bytes_read = std.c.read(fd, buf.ptr, max_bytes);
    if (bytes_read < 0) {
        return .io_error;
    }

    const n = @as(usize, @intCast(bytes_read));

    if (n >= max_bytes) {
        var extra: [1]u8 = undefined;
        const extra_read = std.c.read(fd, &extra, 1);
        if (extra_read > 0) {
            return .too_large;
        }
    }

    const exact_buf = allocator.realloc(buf, n) catch {
        // realloc failed, dupe the content as fallback
        const copy = allocator.dupe(u8, buf[0..n]) catch return .io_error;
        owns_buf = false;
        return .{ .value = copy };
    };

    owns_buf = false;
    return .{ .value = exact_buf };
}

pub fn readFixtureFile(
    allocator: std.mem.Allocator,
    path: []const u8,
    max_bytes: usize,
) LinuxReadResult {
    if (builtin.os.tag != .linux) {
        return .unsupported_platform;
    }
    if (!std.mem.startsWith(u8, path, "/tmp/kgb_fixture")) {
        return .malformed;
    }

    var path_buf: [4096]u8 = undefined;
    const c_path = toCString(path, &path_buf) catch return .io_error;

    const flags = std.os.linux.O{ .ACCMODE = std.posix.ACCMODE.RDONLY };
    const fd = std.c.open(c_path, flags, @as(c_uint, 0));
    if (fd < 0) {
        const err = std.c.getErrno(fd);
        switch (err) {
            .NOENT => return .missing,
            .ACCES => return .permission_denied,
            else => return .io_error,
        }
    }
    defer _ = std.c.close(fd);

    var buf = allocator.alloc(u8, max_bytes) catch return .io_error;
    var owns_buf = true;
    defer if (owns_buf) allocator.free(buf);

    const bytes_read = std.c.read(fd, buf.ptr, max_bytes);
    if (bytes_read < 0) {
        return .io_error;
    }

    const n = @as(usize, @intCast(bytes_read));

    if (n >= max_bytes) {
        var extra: [1]u8 = undefined;
        const extra_read = std.c.read(fd, &extra, 1);
        if (extra_read > 0) {
            return .too_large;
        }
    }

    const exact_buf = allocator.realloc(buf, n) catch {
        // realloc failed, dupe the content as fallback
        const copy = allocator.dupe(u8, buf[0..n]) catch return .io_error;
        owns_buf = false;
        return .{ .value = copy };
    };

    owns_buf = false;
    return .{ .value = exact_buf };
}

// MemoryCopySafety: path and buf are distinct memory regions; no aliasing.
fn toCString(path: []const u8, buf: *[4096]u8) error{PathTooLong}![*:0]const u8 {
    if (path.len >= buf.len) return error.PathTooLong;
    @memcpy(buf[0..path.len], path);
    buf[path.len] = 0;
    return @as([*:0]const u8, @ptrCast(buf));
}

pub fn trimAndClone(allocator: std.mem.Allocator, content: []const u8) []u8 {
    const trimmed = std.mem.trim(u8, content, " \t\r\n");
    return allocator.dupe(u8, trimmed) catch @panic("OOM");
}

// ============================================================================
// Tests
// ============================================================================

test "LinuxReadResult hasValue returns true for value" {
    const result: LinuxReadResult = .{ .value = "test" };
    try std.testing.expect(result.hasValue());
}

test "LinuxReadResult hasValue returns false for missing" {
    const result: LinuxReadResult = .missing;
    try std.testing.expect(!result.hasValue());
}

test "LinuxReadResult valueOrNull returns content for value" {
    const result: LinuxReadResult = .{ .value = "test" };
    try std.testing.expect(std.mem.eql(u8, result.valueOrNull().?, "test"));
}

test "LinuxReadResult valueOrNull returns null for missing" {
    const result: LinuxReadResult = .missing;
    try std.testing.expect(result.valueOrNull() == null);
}

test "validatePath accepts valid sysfs_net path" {
    try std.testing.expect(validatePath(.sysfs_net, "/sys/class/net/lo"));
    try std.testing.expect(validatePath(.sysfs_net, "/sys/class/net/lo/statistics"));
}

test "validatePath rejects invalid sysfs_net path" {
    try std.testing.expect(!validatePath(.sysfs_net, "/etc/passwd"));
    try std.testing.expect(!validatePath(.sysfs_net, "/sys/class/../etc/passwd"));
    try std.testing.expect(!validatePath(.sysfs_net, "/sys"));
}

test "validatePath accepts valid proc_self path" {
    try std.testing.expect(validatePath(.proc_self, "/proc/self/status"));
}

test "validatePath rejects invalid proc_self path" {
    try std.testing.expect(!validatePath(.proc_self, "/etc/passwd"));
    try std.testing.expect(!validatePath(.proc_self, "/proc/123/status"));
}

test "ReadConfig default max_bytes" {
    const config: ReadConfig = .{};
    try std.testing.expectEqual(@as(usize, 4096), config.max_bytes);
}

test "trimAndClone removes whitespace" {
    const allocator = std.testing.allocator;
    const result = trimAndClone(allocator, "  hello \n");
    defer allocator.free(result);
    try std.testing.expectEqualStrings("hello", result);
}

test "trimAndClone handles empty after trim" {
    const allocator = std.testing.allocator;
    const result = trimAndClone(allocator, "   \n\t  ");
    defer allocator.free(result);
    try std.testing.expectEqualStrings("", result);
}
