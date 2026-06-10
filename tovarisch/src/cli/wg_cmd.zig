// cli/wg_cmd.zig — WireGuard command handler
//
// Handles the `tovarisch wg generate` subcommand.

const std = @import("std");
const wg_args = @import("wg_args.zig");
const config = @import("../config.zig");
const wg_generate = @import("../wg/generate.zig");

/// Execute the wg subcommand.
pub fn wgCommand(args: []const []const u8, stderr: anytype, allocator: std.mem.Allocator) u8 {
    const result = wg_args.parseWgArgs(args, stderr);

    switch (result) {
        .help => {
            stderr.writeAll("usage: tovarisch wg generate --config <path>\n") catch return 1;
            stderr.writeAll("\nGenerates WireGuard server config from [wg] section in tovarisch.conf.\n") catch return 1;
            stderr.writeAll("The generated config file is written with strict permissions (0600).\n") catch return 1;
            stderr.writeAll("Private keys are read from the path specified in private_key_file.\n") catch return 1;
            return 0;
        },
        .usage => {
            return 1;
        },
        .generate => |req| {
            // Read and parse config
            var raw = wg_args.readConfig(req.config_path, allocator) catch |e| {
                stderr.print("error: failed to read config: {s}\n", .{@errorName(e)}) catch {};
                return 1;
            };
            defer raw.deinit(allocator);

            // Parse wg config
            const wg_cfg = config.parseWgConfig(&raw) catch |e| {
                stderr.print("error: failed to parse wg config: {s}\n", .{@errorName(e)}) catch {};
                return 1;
            };

            // Check if enabled
            if (!wg_cfg.enabled) {
                stderr.writeAll("error: WireGuard config generation is disabled\n") catch {};
                stderr.writeAll("Set enabled = true in [wg] section\n") catch {};
                return 1;
            }

            // Generate the config
            const gen_result = wg_generate.generateConfig(wg_cfg, allocator) catch |e| {
                stderr.print("error: failed to generate config: {s}\n", .{@errorName(e)}) catch {};
                return 1;
            };
            defer gen_result.deinit(allocator);

            // Success output
            stderr.print("Generated WireGuard config at: {s}\n", .{gen_result.output_path}) catch {};
            stderr.print("Interface: {s}\n", .{gen_result.interface}) catch {};
            stderr.print("Address: {s}\n", .{gen_result.address}) catch {};
            stderr.print("ListenPort: {d}\n", .{gen_result.listen_port}) catch {};
            return 0;
        },
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

test "wgCommand returns 0 for --help" {
    const result = wgCommand(&.{"--help"}, VoidWriter{}, std.heap.page_allocator);
    try std.testing.expectEqual(@as(u8, 0), result);
}

test "wgCommand returns 0 for -h" {
    const result = wgCommand(&.{"-h"}, VoidWriter{}, std.heap.page_allocator);
    try std.testing.expectEqual(@as(u8, 0), result);
}

test "wgCommand returns 1 for no args" {
    const result = wgCommand(&.{}, VoidWriter{}, std.heap.page_allocator);
    try std.testing.expectEqual(@as(u8, 1), result);
}

test "wgCommand returns 1 for unknown subcommand" {
    const result = wgCommand(&.{"unknown"}, VoidWriter{}, std.heap.page_allocator);
    try std.testing.expectEqual(@as(u8, 1), result);
}

test "wgCommand returns 1 when --config is missing" {
    const result = wgCommand(&.{"generate"}, VoidWriter{}, std.heap.page_allocator);
    try std.testing.expectEqual(@as(u8, 1), result);
}

test "wgCommand returns 1 for non-existent config file" {
    const result = wgCommand(&.{ "generate", "--config", "/nonexistent/path.conf" }, VoidWriter{}, std.heap.page_allocator);
    try std.testing.expectEqual(@as(u8, 1), result);
}
