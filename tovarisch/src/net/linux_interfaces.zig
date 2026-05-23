// linux_interfaces.zig — Linux sysfs interface enumeration
//
// ACT 5c: Enumerate interface names from injectable sysfs root.
// Does NOT filter private interfaces, does NOT read statistics,
// and does NOT wire /metrics.json.
//
// Scope:
// - List interface names under an injectable /sys/class/net-style root
// - Return allocator-owned copies of interface names
// - Skip "." and ".."
// - Do not inspect addresses or classify private/public interfaces

const std = @import("std");

// ============================================================================
// Errors
// ============================================================================

pub const ListError = error{
    RootDirMissing,
    RootDirUnreadable,
    OutOfMemory,
};

// ============================================================================
// C String Helper
// ============================================================================

/// Converts a Zig slice to a null-terminated C string.
/// C filesystem APIs require null-terminated paths.
fn toCString(path: []const u8, buf: *[4096]u8) error{PathTooLong}![*:0]const u8 {
    if (path.len >= buf.len) return error.PathTooLong;
    @memcpy(buf[0..path.len], path);
    buf[path.len] = 0;
    return @as([*:0]const u8, @ptrCast(buf));
}

// ============================================================================
// Interface Enumeration
// ============================================================================

/// Lists network interface names under a sysfs-style root directory.
///
/// The function opens `sysfs_root`, iterates directory entries, skips "." and
/// "..", and returns interface names as allocator-owned copies.
///
/// Does NOT read statistics files or classify interfaces.
///
/// Returns an error if the root directory is missing or unreadable.
pub fn listInterfaces(allocator: std.mem.Allocator, sysfs_root: []const u8) ListError![][]const u8 {
    var path_buf: [4096]u8 = undefined;
    const c_path = toCString(sysfs_root, &path_buf) catch return error.RootDirUnreadable;

    // Check if directory exists using access() with F_OK
    if (std.c.access(c_path, std.c.F_OK) != 0) {
        return error.RootDirMissing;
    }

    const dir = std.c.opendir(c_path) orelse return error.RootDirUnreadable;
    defer _ = std.c.closedir(dir);

    var names = std.ArrayList([]const u8).empty;
    errdefer {
        for (names.items) |name| allocator.free(name);
        names.deinit(allocator);
    }

    while (true) {
        const entry = std.c.readdir(dir) orelse break;

        // std.c.dirent.name is a fixed-size array on all platforms.
        // Find null terminator to get actual name length.
        const name_ptr: [*]const u8 = @ptrCast(&entry.name);
        const name_len = std.mem.indexOfScalar(u8, name_ptr[0..256], 0) orelse 0;
        if (name_len == 0) continue;

        // Skip "." and ".."
        if (name_len == 1 and name_ptr[0] == '.') continue;
        if (name_len == 2 and name_ptr[0] == '.' and name_ptr[1] == '.') continue;

        // Copy immediately - readdir() reuses internal storage
        const name_copy = try allocator.dupe(u8, name_ptr[0..name_len]);
        errdefer allocator.free(name_copy);
        try names.append(allocator, name_copy);
    }

    return try names.toOwnedSlice(allocator);
}

/// Frees an interface name list returned by listInterfaces().
pub fn freeInterfaceList(allocator: std.mem.Allocator, names: [][]const u8) void {
    for (names) |name| allocator.free(name);
    allocator.free(names);
}
