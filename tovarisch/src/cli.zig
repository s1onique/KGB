const std = @import("std");
const status = @import("status.zig");
const http = @import("http/server.zig");

pub const ExitCode = enum(u8) {
    ok = 0,
    usage = 2,
    serve_error = 3,
};

pub fn run(args: []const []const u8, stdout: anytype, stderr: anytype) ExitCode {
    if (args.len == 1) {
        printUsage(stderr);
        return .usage;
    }

    const command = args[1];

    if (std.mem.eql(u8, command, "--help") or std.mem.eql(u8, command, "-h")) {
        printUsage(stdout);
        return .ok;
    }

    if (std.mem.eql(u8, command, "--version")) {
        stdout.print("tovarisch {s}\n", .{status.version}) catch return .usage;
        return .ok;
    }

    if (std.mem.eql(u8, command, "check")) {
        stdout.writeAll("tovarisch check: ok\n") catch return .usage;
        return .ok;
    }

    if (std.mem.eql(u8, command, "serve")) {
        return serveCommand(args[2..], stdout, stderr);
    }

    if (std.mem.eql(u8, command, "status")) {
        return statusCommand(args[2..], stdout, stderr);
    }

    stderr.print("unknown command: {s}\n\n", .{command}) catch {};
    printUsage(stderr);
    return .usage;
}

/// Result of parsing serve command arguments.
pub const ServeParseResult = union(enum) {
    ok: http.Config,
    usage,
};

/// Parse serve command arguments without starting the daemon.
/// Returns the parsed config or usage error.
pub fn parseServeArgs(args: []const []const u8, stderr: anytype) ServeParseResult {
    var config = http.defaultConfig();

    var i: usize = 0;
    while (i < args.len) : (i += 1) {
        const arg = args[i];

        if (std.mem.eql(u8, arg, "--listen") and i + 1 < args.len) {
            const addr = args[i + 1];
            if (std.mem.indexOfScalar(u8, addr, ':')) |colon_idx| {
                const host = addr[0..colon_idx];
                const port_str = addr[colon_idx + 1 ..];
                config.port = std.fmt.parseInt(u16, port_str, 10) catch {
                    stderr.writeAll("invalid port in --listen address\n") catch {};
                    return .usage;
                };
                config.address = host;
            } else {
                config.address = addr;
            }
            i += 1;
        } else if (std.mem.eql(u8, arg, "--listen-private")) {
            config.address = "127.0.0.1";
        } else if (std.mem.eql(u8, arg, "--listen-all-public-dangerous")) {
            config.address = "0.0.0.0";
        } else if (std.mem.eql(u8, arg, "--listen-all")) {
            stderr.writeAll("error: --listen-all is deprecated; use --listen-all-public-dangerous\n") catch {};
            return .usage;
        } else {
            stderr.print("unknown serve option: {s}\n", .{arg}) catch {};
            return .usage;
        }
    }

    return .{ .ok = config };
}

fn serveCommand(args: []const []const u8, stdout: anytype, stderr: anytype) ExitCode {
    const parsed = parseServeArgs(args, stderr);

    switch (parsed) {
        .usage => return .usage,
        .ok => |config| {
            stdout.print("Starting tovarisch HTTP service on port {d}...\n", .{config.port}) catch {};
            stdout.writeAll("Press Ctrl+C to stop.\n") catch {};

            http.serve(config) catch {
                stderr.writeAll("error: failed to start HTTP server\n") catch {};
                return .serve_error;
            };

            return .ok;
        },
    }
}

fn statusCommand(args: []const []const u8, stdout: anytype, stderr: anytype) ExitCode {
    if (args.len != 1 or !std.mem.eql(u8, args[0], "--json")) {
        stderr.writeAll("usage: tovarisch status --json\n") catch {};
        return .usage;
    }

    status.renderPayload(stdout) catch return .usage;
    stdout.writeByte('\n') catch return .usage;

    return .ok;
}

fn printUsage(writer: anytype) void {
    writer.writeAll(
        \\usage:
        \\  tovarisch --version
        \\  tovarisch check
        \\  tovarisch serve [--listen ADDR:PORT] [--listen-private] [--listen-all-public-dangerous]
        \\  tovarisch status --json
        \\
    ) catch {};
}

// CaptureWriter: collects bytes written for test assertions.
// Uses a fixed buffer to avoid allocator complexity in tests.
// Buffer size is generous for the CLI's small output.
pub const CaptureWriter = struct {
    const Self = @This();
    const BufSize = 4096;

    buf: [BufSize]u8 = undefined,
    len: usize = 0,

    pub fn init() Self {
        return .{
            .buf = undefined,
            .len = 0,
        };
    }

    pub fn print(self: *Self, comptime fmt: []const u8, args: anytype) !void {
        if (self.len >= BufSize) return error.BufferOverflow;
        const written = std.fmt.bufPrint(self.buf[self.len..], fmt, args) catch return error.BufferOverflow;
        self.len += written.len;
    }

    pub fn writeAll(self: *Self, bytes: []const u8) !void {
        if (self.len + bytes.len > BufSize) return error.BufferOverflow;
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
};

// VoidWriter: for tests that only need exit codes (no output)
pub const VoidWriter = struct {
    const Self = @This();

    pub fn writeAll(_: Self, _: []const u8) error{}!void {}
    pub fn write(_: Self, _: []const u8) error{}!void {}
    pub fn print(_: Self, _: []const u8, _: anytype) error{}!void {}
    pub fn writeByte(_: Self, _: u8) error{}!void {}
    pub fn flush(_: Self) error{}!void {}
};

// --- Tests ---

test "help command returns ok" {
    const w = VoidWriter{};
    try std.testing.expect(run(&.{ "tovarisch", "--help" }, w, w) == .ok);
}

test "-h short flag returns ok" {
    const w = VoidWriter{};
    try std.testing.expect(run(&.{ "tovarisch", "-h" }, w, w) == .ok);
}

test "version command returns ok" {
    const w = VoidWriter{};
    try std.testing.expect(run(&.{ "tovarisch", "--version" }, w, w) == .ok);
}

test "check command returns ok" {
    const w = VoidWriter{};
    try std.testing.expect(run(&.{ "tovarisch", "check" }, w, w) == .ok);
}

test "unknown command returns usage" {
    const w = VoidWriter{};
    try std.testing.expect(run(&.{ "tovarisch", "badcmd" }, w, w) == .usage);
}

test "no args returns usage" {
    const w = VoidWriter{};
    try std.testing.expect(run(&.{"tovarisch"}, w, w) == .usage);
}

test "status without --json returns usage" {
    const w = VoidWriter{};
    try std.testing.expect(run(&.{ "tovarisch", "status" }, w, w) == .usage);
}

test "status --json returns ok" {
    const w = VoidWriter{};
    try std.testing.expect(run(&.{ "tovarisch", "status", "--json" }, w, w) == .ok);
}

// --- serve argument parsing tests (non-blocking) ---
// Note: We test parseServeArgs directly to avoid blocking daemon tests.

test "parseServeArgs defaults to loopback port 8317" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{}, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("127.0.0.1", parsed.ok.address);
    try std.testing.expectEqual(@as(u16, 8317), parsed.ok.port);
}

test "parseServeArgs with --listen sets address and port" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{ "--listen", "127.0.0.1:9999" }, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("127.0.0.1", parsed.ok.address);
    try std.testing.expectEqual(@as(u16, 9999), parsed.ok.port);
}

test "parseServeArgs with --listen-private sets loopback" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{"--listen-private"}, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("127.0.0.1", parsed.ok.address);
}

test "parseServeArgs with --listen-all-public-dangerous sets 0.0.0.0" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{"--listen-all-public-dangerous"}, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("0.0.0.0", parsed.ok.address);
}

test "parseServeArgs with deprecated --listen-all returns usage" {
    const w = VoidWriter{};
    try std.testing.expect(parseServeArgs(&.{"--listen-all"}, w) == .usage);
}

test "parseServeArgs with unknown option returns usage" {
    const w = VoidWriter{};
    try std.testing.expect(parseServeArgs(&.{"--unknown"}, w) == .usage);
}

// --- Output behavior tests ---
// These tests verify CLI output content using CaptureWriter.

test "--help output contains usage" {
    var cw = CaptureWriter.init();
    const code = run(&.{ "tovarisch", "--help" }, &cw, &cw);
    try std.testing.expect(code == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, cw.slice(), 1, "usage:"));
}

test "--help output contains tovarisch --version" {
    var cw = CaptureWriter.init();
    const code = run(&.{ "tovarisch", "--help" }, &cw, &cw);
    try std.testing.expect(code == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, cw.slice(), 1, "tovarisch --version"));
}

test "--help output contains tovarisch check" {
    var cw = CaptureWriter.init();
    const code = run(&.{ "tovarisch", "--help" }, &cw, &cw);
    try std.testing.expect(code == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, cw.slice(), 1, "tovarisch check"));
}

test "--help output contains tovarisch serve" {
    var cw = CaptureWriter.init();
    const code = run(&.{ "tovarisch", "--help" }, &cw, &cw);
    try std.testing.expect(code == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, cw.slice(), 1, "tovarisch serve"));
}

test "--help output contains --listen-all-public-dangerous" {
    var cw = CaptureWriter.init();
    const code = run(&.{ "tovarisch", "--help" }, &cw, &cw);
    try std.testing.expect(code == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, cw.slice(), 1, "--listen-all-public-dangerous"));
}

test "--help output does NOT contain deprecated --listen-all" {
    var cw = CaptureWriter.init();
    const code = run(&.{ "tovarisch", "--help" }, &cw, &cw);
    try std.testing.expect(code == .ok);
    // The deprecated --listen-all should NOT appear in help
    try std.testing.expect(!std.mem.containsAtLeast(u8, cw.slice(), 1, "--listen-all]"));
}

test "--help output contains tovarisch status --json" {
    var cw = CaptureWriter.init();
    const code = run(&.{ "tovarisch", "--help" }, &cw, &cw);
    try std.testing.expect(code == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, cw.slice(), 1, "tovarisch status --json"));
}

test "-h short flag contains usage" {
    var cw = CaptureWriter.init();
    const code = run(&.{ "tovarisch", "-h" }, &cw, &cw);
    try std.testing.expect(code == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, cw.slice(), 1, "usage:"));
}

test "--version output contains tovarisch" {
    var cw = CaptureWriter.init();
    const code = run(&.{ "tovarisch", "--version" }, &cw, &cw);
    try std.testing.expect(code == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, cw.slice(), 1, "tovarisch"));
}

test "--version output contains 0.1.1" {
    var cw = CaptureWriter.init();
    const code = run(&.{ "tovarisch", "--version" }, &cw, &cw);
    try std.testing.expect(code == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, cw.slice(), 1, "0.1.1"));
}

test "check output contains tovarisch check: ok" {
    var cw = CaptureWriter.init();
    const code = run(&.{ "tovarisch", "check" }, &cw, &cw);
    try std.testing.expect(code == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, cw.slice(), 1, "tovarisch check: ok"));
}

// Note: Tests for "serve" output are removed because serveCommand blocks forever.
// Use parseServeArgs tests above for argument parsing coverage.
// Daemon smoke tests should be run manually or via integration harness.

test "status --json output contains service:tovarisch" {
    var cw = CaptureWriter.init();
    const code = run(&.{ "tovarisch", "status", "--json" }, &cw, &cw);
    try std.testing.expect(code == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, cw.slice(), 1, "\"service\":\"tovarisch\""));
}

test "status --json output contains name:process" {
    var cw = CaptureWriter.init();
    const code = run(&.{ "tovarisch", "status", "--json" }, &cw, &cw);
    try std.testing.expect(code == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, cw.slice(), 1, "\"name\":\"process\""));
}

test "status --json output contains name:binary" {
    var cw = CaptureWriter.init();
    const code = run(&.{ "tovarisch", "status", "--json" }, &cw, &cw);
    try std.testing.expect(code == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, cw.slice(), 1, "\"name\":\"binary\""));
}

test "status --json output contains name:config" {
    var cw = CaptureWriter.init();
    const code = run(&.{ "tovarisch", "status", "--json" }, &cw, &cw);
    try std.testing.expect(code == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, cw.slice(), 1, "\"name\":\"config\""));
}

test "status --json output contains name:state_dir" {
    var cw = CaptureWriter.init();
    const code = run(&.{ "tovarisch", "status", "--json" }, &cw, &cw);
    try std.testing.expect(code == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, cw.slice(), 1, "\"name\":\"state_dir\""));
}

test "CLI exit codes match expected behavior" {
    const w = VoidWriter{};

    // Success exit codes (skip "serve" - blocks forever)
    try std.testing.expect(run(&.{ "tovarisch", "--help" }, w, w) == .ok);
    try std.testing.expect(run(&.{ "tovarisch", "-h" }, w, w) == .ok);
    try std.testing.expect(run(&.{ "tovarisch", "--version" }, w, w) == .ok);
    try std.testing.expect(run(&.{ "tovarisch", "check" }, w, w) == .ok);
    try std.testing.expect(run(&.{ "tovarisch", "status", "--json" }, w, w) == .ok);
    // Note: "serve" is skipped because it blocks forever; use parseServeArgs tests instead.

    // Usage exit codes
    try std.testing.expect(run(&.{"tovarisch"}, w, w) == .usage);
    try std.testing.expect(run(&.{ "tovarisch", "badcmd" }, w, w) == .usage);
    try std.testing.expect(run(&.{ "tovarisch", "status" }, w, w) == .usage);
    try std.testing.expect(run(&.{ "tovarisch", "serve", "--unknown" }, w, w) == .usage);
}
