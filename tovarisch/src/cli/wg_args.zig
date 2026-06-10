// cli/wg_args.zig — Argument parsing for wg subcommand
//
// Parses arguments for the `tovarisch wg generate` command.

const std = @import("std");
const config = @import("../config.zig");

/// Errors that can occur during wg argument parsing.
pub const WgArgError = error{
    /// Invalid arguments passed to wg command.
    InvalidArguments,
    /// Config file not found.
    ConfigFileNotFound,
    /// Config parsing failed.
    ConfigParseError,
    /// Generation failed.
    GenerationError,
    /// Out of memory.
    OutOfMemory,
};

/// Parse result for the wg subcommand.
pub const WgParseResult = union(enum) {
    /// Help was requested.
    help,
    /// Generation was requested.
    generate: GenerateRequest,
    /// Invalid usage.
    usage,
};

/// Request to generate WireGuard config.
pub const GenerateRequest = struct {
    /// Path to the config file.
    config_path: []const u8,
};

/// Parse wg subcommand arguments.
/// Returns WgParseResult indicating what action to take.
pub fn parseWgArgs(args: []const []const u8, stderr: anytype) WgParseResult {
    if (args.len == 0) {
        stderr.writeAll("error: missing subcommand for 'wg'\n") catch {};
        stderr.writeAll("usage: tovarisch wg generate --config <path>\n") catch {};
        return .usage;
    }

    const subcmd = args[0];

    if (std.mem.eql(u8, subcmd, "--help") or std.mem.eql(u8, subcmd, "-h")) {
        return .help;
    }

    if (std.mem.eql(u8, subcmd, "generate")) {
        return parseGenerateArgs(args[1..], stderr);
    }

    stderr.print("error: unknown wg subcommand: {s}\n", .{subcmd}) catch {};
    stderr.writeAll("usage: tovarisch wg generate --config <path>\n") catch {};
    return .usage;
}

/// Parse arguments for the `wg generate` subcommand.
fn parseGenerateArgs(args: []const []const u8, stderr: anytype) WgParseResult {
    var config_path: ?[]const u8 = null;

    var i: usize = 0;
    while (i < args.len) : (i += 1) {
        const arg = args[i];

        if (std.mem.eql(u8, arg, "--config") and i + 1 < args.len) {
            config_path = args[i + 1];
            i += 1;
        } else if (std.mem.eql(u8, arg, "--help") or std.mem.eql(u8, arg, "-h")) {
            return .help;
        } else {
            stderr.print("error: unknown option: {s}\n", .{arg}) catch {};
            stderr.writeAll("usage: tovarisch wg generate --config <path>\n") catch {};
            return .usage;
        }
    }

    if (config_path == null) {
        stderr.writeAll("error: --config is required\n") catch {};
        stderr.writeAll("usage: tovarisch wg generate --config <path>\n") catch {};
        return .usage;
    }

    return .{
        .generate = .{
            .config_path = config_path.?,
        },
    };
}

/// Read and parse a tovarisch config file.
pub fn readConfig(path: []const u8, allocator: std.mem.Allocator) WgArgError!config.RawConfig {
    var raw = config.RawConfig{};
    errdefer raw.deinit(allocator);

    var path_buf: [4096]u8 = undefined;
    const c_path = toCString(path, &path_buf) catch {
        return WgArgError.ConfigFileNotFound;
    };

    const fd = std.c.open(c_path, @bitCast(@as(u32, 0)));
    if (fd < 0) {
        return WgArgError.ConfigFileNotFound;
    }
    defer _ = std.c.close(fd);

    // Read the entire file into memory
    var content = std.ArrayList(u8).empty;
    defer content.deinit(allocator);

    var buf: [4096]u8 = undefined;
    while (true) {
        const bytes_read = std.c.read(fd, &buf, buf.len);
        if (bytes_read < 0) {
            return WgArgError.ConfigParseError;
        }
        if (bytes_read == 0) break;
        try content.appendSlice(allocator, buf[0..@as(usize, @intCast(bytes_read))]);
    }

    try parseIniContent(content.items, &raw, allocator);
    return raw;
}

/// Convert a Zig slice to a null-terminated C string for C path APIs.
fn toCString(path: []const u8, buf: *[4096]u8) error{PathTooLong}![*:0]const u8 {
    if (path.len >= buf.len) return error.PathTooLong;
    @memcpy(buf[0..path.len], path);
    buf[path.len] = 0;
    return @as([*:0]const u8, @ptrCast(buf));
}

/// Parse INI content into RawConfig.
pub fn parseIniContent(content: []const u8, raw: *config.RawConfig, allocator: std.mem.Allocator) WgArgError!void {
    var current_section: ?*std.StringArrayHashMapUnmanaged([]const u8) = null;

    var line_iter = std.mem.splitScalar(u8, content, '\n');
    while (line_iter.next()) |line| {
        const trimmed = std.mem.trim(u8, line, " \t\r\n");

        // Skip empty lines and comments
        if (trimmed.len == 0 or trimmed[0] == '#' or trimmed[0] == ';') {
            continue;
        }

        // Section header: [section_name]
        if (trimmed[0] == '[') {
            const end_idx = std.mem.indexOfScalar(u8, trimmed, ']') orelse continue;
            const name = trimmed[1..end_idx];
            const owned_name = try allocator.dupe(u8, name);
            errdefer allocator.free(owned_name);

            try raw.put(allocator, owned_name, .{});
            current_section = raw.getPtr(owned_name);
            continue;
        }

        // Key=value pair
        if (current_section) |section| {
            if (std.mem.indexOfScalar(u8, trimmed, '=')) |eq_idx| {
                const key = std.mem.trim(u8, trimmed[0..eq_idx], " \t");
                const raw_value = trimmed[eq_idx + 1 ..];

                // Trim the value and remove surrounding quotes if present
                var value = std.mem.trim(u8, raw_value, " \t");
                if (value.len >= 2 and value[0] == '"' and value[value.len - 1] == '"') {
                    value = value[1 .. value.len - 1];
                }

                const owned_key = try allocator.dupe(u8, key);
                errdefer allocator.free(owned_key);
                const owned_value = try allocator.dupe(u8, value);
                errdefer allocator.free(owned_value);

                try section.put(allocator, owned_key, owned_value);
            }
        }
    }
}

// --- Tests ---

const VoidWriter = struct {
    const Self = @This();
    pub fn writeAll(_: Self, _: []const u8) error{}!void {}
    pub fn write(_: Self, _: []const u8) error{}!void {}
    pub fn print(_: Self, _: []const u8, _: anytype) error{}!void {}
    pub fn writeByte(_: Self, _: u8) error{}!void {}
    pub fn flush(_: Self) error{}!void {}
};

test "parseWgArgs requires subcommand" {
    const w = VoidWriter{};
    const result = parseWgArgs(&.{}, w);
    try std.testing.expect(result == .usage);
}

test "parseWgArgs accepts --help" {
    const w = VoidWriter{};
    const result = parseWgArgs(&.{"--help"}, w);
    try std.testing.expect(result == .help);
}

test "parseWgArgs accepts -h" {
    const w = VoidWriter{};
    const result = parseWgArgs(&.{"-h"}, w);
    try std.testing.expect(result == .help);
}

test "parseWgArgs accepts generate subcommand" {
    const w = VoidWriter{};
    const result = parseWgArgs(&.{ "generate", "--config", "/path/to/config" }, w);
    try std.testing.expect(result == .generate);
    try std.testing.expectEqualStrings("/path/to/config", result.generate.config_path);
}

test "parseWgArgs rejects unknown subcommand" {
    const w = VoidWriter{};
    const result = parseWgArgs(&.{"unknown"}, w);
    try std.testing.expect(result == .usage);
}

test "parseGenerateArgs requires --config" {
    const w = VoidWriter{};
    const result = parseGenerateArgs(&.{}, w);
    try std.testing.expect(result == .usage);
}

test "parseGenerateArgs accepts --config with path" {
    const w = VoidWriter{};
    const result = parseGenerateArgs(&.{ "--config", "/etc/tovarisch.conf" }, w);
    try std.testing.expect(result == .generate);
    try std.testing.expectEqualStrings("/etc/tovarisch.conf", result.generate.config_path);
}

test "parseIniContent parses simple config" {
    const content = "[wg]\nenabled = true\ninterface = wg-kgb0";
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);

    try parseIniContent(content, &raw, std.heap.page_allocator);

    try std.testing.expect(raw.contains("wg"));
    const wg_section = raw.get("wg").?;
    try std.testing.expectEqualStrings("true", wg_section.get("enabled").?);
    try std.testing.expectEqualStrings("wg-kgb0", wg_section.get("interface").?);
}

test "parseIniContent skips comments" {
    const content = "# comment\n; another comment\n[section]\nkey = value";
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);

    try parseIniContent(content, &raw, std.heap.page_allocator);

    try std.testing.expect(raw.contains("section"));
    try std.testing.expectEqualStrings("value", raw.get("section").?.get("key").?);
}

test "parseIniContent handles quoted values" {
    const content = "[section]\nkey = \"quoted value\"";
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);

    try parseIniContent(content, &raw, std.heap.page_allocator);

    try std.testing.expectEqualStrings("quoted value", raw.get("section").?.get("key").?);
}
