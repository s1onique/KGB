// bgp/prefix_file_loader.zig — Runtime prefix file loading for BGP
//
// ACT 6: Add advertised_prefix_files support for BGP runtime config.
// Loads and parses BIRD-style prefix files.
//
// References: RFC 4271 (BGP-4)

const std = @import("std");
const config_parse = @import("config_parse.zig");
const prefix_file = @import("prefix_file.zig");
const types = @import("types.zig");

/// Load a prefix file's contents as a string.
/// Returns allocated string that caller must free.
pub fn loadPrefixFile(path: []const u8, allocator: std.mem.Allocator) ![]const u8 {
    // Use C stdlib to open and read the file
    var path_buf: [4096]u8 = undefined;
    const c_path = try toCString(path, &path_buf);

    const fd = std.c.open(c_path, @bitCast(@as(u32, 0)));
    if (fd < 0) {
        return error.FileNotFound;
    }
    defer _ = std.c.close(fd);

    // Read file into memory
    var content = std.ArrayList(u8).empty;
    errdefer content.deinit(allocator);

    var buf: [4096]u8 = undefined;
    while (true) {
        const bytes_read = std.c.read(fd, &buf, buf.len);
        if (bytes_read < 0) {
            return error.FileReadError;
        }
        if (bytes_read == 0) break;
        try content.appendSlice(allocator, buf[0..@as(usize, @intCast(bytes_read))]);
    }

    return try content.toOwnedSlice(allocator);
}

/// Convert a Zig slice to a null-terminated C string for C path APIs.
fn toCString(path: []const u8, buf: *[4096]u8) error{PathTooLong}![*:0]const u8 {
    if (path.len >= buf.len) return error.PathTooLong;
    // MemoryCopySafety: buf is a caller-provided [4096]u8 buffer. path is a
    // caller-provided slice. They are distinct memory regions; no aliasing possible.
    @memcpy(buf[0..path.len], path);
    buf[path.len] = 0;
    return @as([*:0]const u8, @ptrCast(buf));
}

/// Load and parse prefixes from one or more prefix files.
/// Returns allocated slice of Ipv4Prefix that caller must free.
/// On error, reports path-specific diagnostics to stderr if provided.
pub fn loadPrefixFilesFromConfig(
    advertised_prefix_files_raw: []const u8,
    allocator: std.mem.Allocator,
) ![]types.Ipv4Prefix {
    if (advertised_prefix_files_raw.len == 0) {
        return try allocator.alloc(types.Ipv4Prefix, 0);
    }

    const file_paths = try config_parse.parsePrefixList(advertised_prefix_files_raw, allocator);
    defer allocator.free(file_paths);

    var prefixes = std.ArrayList(types.Ipv4Prefix).empty;
    errdefer prefixes.deinit(allocator);

    for (file_paths) |file_path| {
        // Read file content as raw string
        const file_content = try loadPrefixFile(file_path, allocator);
        defer allocator.free(file_content);

        // Parse BIRD-style prefix file
        const parse_result = try prefix_file.parse(file_content, allocator);
        defer allocator.free(parse_result.prefixes);

        // Append parsed prefixes from this file
        for (parse_result.prefixes) |prefix| {
            try prefixes.append(allocator, prefix);
        }
    }

    return try prefixes.toOwnedSlice(allocator);
}
