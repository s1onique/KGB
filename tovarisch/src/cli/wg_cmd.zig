// cli/wg_cmd.zig — WireGuard command handler
//
// Handles the `tovarisch wg generate` subcommand.

const std = @import("std");
const wg_args = @import("wg_args.zig");
const config = @import("../config.zig");
const wg_config = @import("../wg/config.zig");
const wg_generate = @import("../wg/generate.zig");

/// Execute the wg subcommand.
/// @param stdout - used for help and success messages
/// @param stderr - used for error messages
pub fn wgCommand(args: []const []const u8, stdout: anytype, stderr: anytype, allocator: std.mem.Allocator) u8 {
    const result = wg_args.parseWgArgs(args, stderr);

    switch (result) {
        .help => {
            stdout.writeAll("usage: tovarisch wg generate --config <path>\n") catch return 1;
            stdout.writeAll("\nGenerates WireGuard server and client configs from [wg] and [wg.peer.*] sections.\n") catch return 1;
            stdout.writeAll("Server config is written to {output_dir}/{interface}.conf\n") catch return 1;
            stdout.writeAll("Client configs are written to paths specified in each [wg.peer.*] section.\n") catch return 1;
            stdout.writeAll("Generated config files are written with strict permissions (0600).\n") catch return 1;
            stdout.writeAll("Private keys are never logged or printed to output.\n") catch return 1;
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

            // Parse all peer configs
            const peers = wg_config.parseAllPeerConfigs(&raw, allocator) catch |e| {
                stderr.print("error: failed to parse peer configs: {s}\n", .{@errorName(e)}) catch {};
                return 1;
            };
            defer allocator.free(peers);

            // Generate server config
            const server_result = wg_generate.generateServerConfig(wg_cfg, peers, allocator) catch |e| {
                stderr.print("error: failed to generate server config: {s}\n", .{@errorName(e)}) catch {};
                return 1;
            };
            defer server_result.deinit(allocator);

            // Output server config result
            stdout.print("Server config generated: {s}\n", .{server_result.output_path}) catch {};
            stdout.print("  Interface: {s}\n", .{server_result.interface}) catch {};
            stdout.print("  Address: {s}\n", .{server_result.address}) catch {};
            stdout.print("  ListenPort: {d}\n", .{server_result.listen_port}) catch {};
            stdout.print("  Peers: {d}\n", .{server_result.peer_count}) catch {};

            // Generate client configs for enabled peers
            // Client generation is fatal - any failure exits non-zero
            var client_count: usize = 0;
            for (peers) |*p| {
                if (!p.enabled) continue;

                // Skip if no client_output_file specified
                if (p.client_output_file.len == 0) continue;

                const client_result = wg_generate.generateClientConfig(wg_cfg, p, allocator) catch |e| {
                    stderr.print("error: failed to generate client config for {s}: {s}\n", .{ p.name, @errorName(e) }) catch {};
                    return 1; // Fatal error - exit non-zero
                };
                defer client_result.deinit(allocator);

                stdout.print("Client config generated: {s}\n", .{client_result.output_path}) catch {};
                stdout.print("  Peer: {s}\n", .{client_result.peer_name}) catch {};
                client_count += 1;
            }

            if (client_count == 0 and server_result.peer_count == 0) {
                stdout.writeAll("\nNo enabled peers found. Add [wg.peer.<name>] sections with enabled = true.\n") catch {};
            } else if (client_count == 0) {
                stdout.writeAll("\nNo client configs generated. Ensure enabled peers have client_output_file set.\n") catch {};
            }

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
    const w = VoidWriter{};
    const result = wgCommand(&.{"--help"}, w, w, std.heap.page_allocator);
    try std.testing.expectEqual(@as(u8, 0), result);
}

test "wgCommand returns 0 for -h" {
    const w = VoidWriter{};
    const result = wgCommand(&.{"-h"}, w, w, std.heap.page_allocator);
    try std.testing.expectEqual(@as(u8, 0), result);
}

test "wgCommand returns 1 for no args" {
    const w = VoidWriter{};
    const result = wgCommand(&.{}, w, w, std.heap.page_allocator);
    try std.testing.expectEqual(@as(u8, 1), result);
}

test "wgCommand returns 1 for unknown subcommand" {
    const w = VoidWriter{};
    const result = wgCommand(&.{"unknown"}, w, w, std.heap.page_allocator);
    try std.testing.expectEqual(@as(u8, 1), result);
}

test "wgCommand returns 1 when --config is missing" {
    const w = VoidWriter{};
    const result = wgCommand(&.{"generate"}, w, w, std.heap.page_allocator);
    try std.testing.expectEqual(@as(u8, 1), result);
}

test "wgCommand returns 1 for non-existent config file" {
    const w = VoidWriter{};
    const result = wgCommand(&.{ "generate", "--config", "/nonexistent/path.conf" }, w, w, std.heap.page_allocator);
    try std.testing.expectEqual(@as(u8, 1), result);
}
