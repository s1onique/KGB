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
                printGenerationError(stderr, "server config", e) catch {};
                stderr.flush() catch {};
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
                    printGenerationError(stderr, p.name, e) catch {};
                    stderr.flush() catch {};
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

/// Print a human-friendly diagnostic for WireGuard config generation errors.
/// Maps machine error names to actionable messages.
fn printGenerationError(stderr: anytype, target: []const u8, err: anyerror) !void {
    // Map specific errors to friendly messages
    if (err == wg_generate.GenerateError.InvalidPublicKey) {
        try stderr.print("error: invalid WireGuard public key for {s}: expected 44-character base64 key\n", .{target});
        return;
    }
    if (err == wg_generate.GenerateError.InvalidPrivateKey) {
        try stderr.print("error: invalid WireGuard private key for {s}: expected 44-character base64 key\n", .{target});
        return;
    }
    // Default: print the error name for other generation errors
    try stderr.print("error: failed to generate {s}: {s}\n", .{ target, @errorName(err) });
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

// CaptureWriter: collects bytes written for test assertions.
const CaptureWriter = struct {
    const Self = @This();
    const BufSize = 4096;

    buf: [BufSize]u8 = undefined,
    len: usize = 0,

    pub fn init() Self {
        return .{ .buf = undefined, .len = 0 };
    }

    pub fn print(self: *Self, comptime fmt: []const u8, print_args: anytype) !void {
        if (self.len >= BufSize) return error.BufferOverflow;
        const written = std.fmt.bufPrint(self.buf[self.len..], fmt, print_args) catch return error.BufferOverflow;
        self.len += written.len;
    }

    pub fn writeAll(self: *Self, bytes: []const u8) !void {
        if (self.len + bytes.len > BufSize) return error.BufferOverflow;
        // MemoryCopySafety: self.buf is a fixed [4096]u8 buffer. bytes is a
        // caller-provided slice. They are distinct memory regions; no aliasing.
        @memcpy(self.buf[self.len..][0..bytes.len], bytes);
        self.len += bytes.len;
    }

    pub fn writeByte(self: *Self, byte: u8) !void {
        if (self.len >= BufSize) return error.BufferOverflow;
        self.buf[self.len] = byte;
        self.len += 1;
    }

    pub fn slice(self: *const Self) []const u8 {
        return self.buf[0..self.len];
    }

    pub fn flush(_: *Self) error{}!void {}
};

test "wgCommand returns 1 for non-existent config file" {
    const w = VoidWriter{};
    const result = wgCommand(&.{ "generate", "--config", "/nonexistent/path.conf" }, w, w, std.heap.page_allocator);
    try std.testing.expectEqual(@as(u8, 1), result);
}

test "wgCommand prints error to stderr on non-existent config" {
    var stdout = CaptureWriter.init();
    var stderr = CaptureWriter.init();
    const result = wgCommand(&.{ "generate", "--config", "/nonexistent/path.conf" }, &stdout, &stderr, std.heap.page_allocator);
    try std.testing.expectEqual(@as(u8, 1), result);
    // Must NOT have empty stderr when returning nonzero
    try std.testing.expect(stderr.len > 0);
    try std.testing.expect(std.mem.containsAtLeast(u8, stderr.slice(), 1, "error:"));
}

test "wgCommand never exits nonzero with empty stderr" {
    // Regression: ensure all error paths produce non-empty stderr before returning nonzero.
    // This test uses invalid config path which triggers error during config read.
    var stdout = CaptureWriter.init();
    var stderr = CaptureWriter.init();
    const exit_code = wgCommand(&.{ "generate", "--config", "/nonexistent/path.conf" }, &stdout, &stderr, std.heap.page_allocator);

    // If exit code is nonzero, stderr MUST have content
    if (exit_code != 0) {
        try std.testing.expect(stderr.len > 0);
    }
}

test "wgCommand with invalid public key file produces stderr diagnostic" {
    // Use a unique temp directory for this test
    const tmp_dir = "/tmp/tovarisch-wg-test-invalid-key-unique";
    const tmp_config = "/tmp/tovarisch-wg-test-invalid-key-unique/config.toml";
    const tmp_priv_key = "/tmp/tovarisch-wg-test-invalid-key-unique/server.key";
    const tmp_pub_key = "/tmp/tovarisch-wg-test-invalid-key-unique/peer.pub";

    // Use separate buffers for each path since we need them simultaneously
    var dir_buf: [256]u8 = undefined;
    var config_buf: [256]u8 = undefined;
    var priv_buf: [256]u8 = undefined;
    var pub_buf: [256]u8 = undefined;

    const c_dir = initCPath(&dir_buf, tmp_dir);
    const c_config = initCPath(&config_buf, tmp_config);
    const c_priv = initCPath(&priv_buf, tmp_priv_key);
    const c_pub = initCPath(&pub_buf, tmp_pub_key);

    // Setup: create directory, files, config
    _ = std.c.mkdir(c_dir, 0o700);

    const open_flags = std.c.O{ .ACCMODE = std.posix.ACCMODE.WRONLY, .CREAT = true, .TRUNC = true };
    const valid_priv_key = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA";
    const invalid_pub_key = "short";
    const config_content = "[wg]\nenabled = true\ninterface = \"wg0\"\naddress = \"10.0.0.1/24\"\nlisten_port = 51820\nprivate_key_file = \"" ++ tmp_priv_key ++ "\"\npublic_key_file = \"" ++ tmp_pub_key ++ "\"\noutput_dir = \"" ++ tmp_dir ++ "\"\n\n[wg.peer.phone]\nenabled = true\naddress = \"10.0.0.2/32\"\nprivate_key_file = \"" ++ tmp_priv_key ++ "\"\npublic_key_file = \"" ++ tmp_pub_key ++ "\"\nallowed_ips = \"10.0.0.2/32\"\nclient_output_file = \"" ++ tmp_dir ++ "/phone.conf\"\n";

    // Write private key
    const priv_fd = std.c.open(c_priv, open_flags, @as(c_uint, 0o600));
    if (priv_fd >= 0) {
        _ = std.c.write(priv_fd, valid_priv_key.ptr, valid_priv_key.len);
        _ = std.c.close(priv_fd);
    }

    // Write invalid public key
    const pub_fd = std.c.open(c_pub, open_flags, @as(c_uint, 0o600));
    if (pub_fd >= 0) {
        _ = std.c.write(pub_fd, invalid_pub_key.ptr, invalid_pub_key.len);
        _ = std.c.close(pub_fd);
    }

    // Write config
    const conf_fd = std.c.open(c_config, open_flags, @as(c_uint, 0o600));
    if (conf_fd >= 0) {
        _ = std.c.write(conf_fd, config_content.ptr, config_content.len);
        _ = std.c.close(conf_fd);
    }

    // Run wgCommand
    var stdout = CaptureWriter.init();
    var stderr = CaptureWriter.init();
    const exit_code = wgCommand(&.{ "generate", "--config", tmp_config }, &stdout, &stderr, std.heap.page_allocator);

    // Cleanup
    _ = std.c.unlink(c_config);
    _ = std.c.unlink(c_priv);
    _ = std.c.unlink(c_pub);
    _ = std.c.rmdir(c_dir);

    // Must return error
    try std.testing.expect(exit_code != 0);
    // Must produce stderr with error message
    try std.testing.expect(stderr.len > 0);
    try std.testing.expect(std.mem.containsAtLeast(u8, stderr.slice(), 1, "error:"));
    // The error should mention key validation failure with friendly message
    try std.testing.expect(std.mem.containsAtLeast(u8, stderr.slice(), 1, "invalid WireGuard public key"));
}

/// Helper to convert a Zig string to a null-terminated C string in a buffer.
fn initCPath(buf: *[256]u8, path: []const u8) [*:0]const u8 {
    // MemoryCopySafety: buf is a fixed [256]u8 buffer. path is a caller-provided
    // slice. They are distinct memory regions; no aliasing.
    @memcpy(buf[0..path.len], path);
    buf[path.len] = 0;
    return @ptrCast(buf);
}
