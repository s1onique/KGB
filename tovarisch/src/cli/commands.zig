const std = @import("std");
const cli_args = @import("args.zig");
const usage = @import("usage.zig");
const status = @import("../status.zig");
const http = @import("../http/server.zig");
const logging = @import("../logging.zig");

pub const ExitCode = enum(u8) {
    ok = 0,
    usage = 2,
    serve_error = 3,
};

/// Run the CLI with the given arguments.
pub fn run(argv: []const []const u8, stdout: anytype, stderr: anytype) ExitCode {
    if (argv.len == 1) {
        printUsage(stderr);
        return .usage;
    }

    const command = argv[1];

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

    if (std.mem.eql(u8, command, "thread-smoke")) {
        return threadSmokeCommand(stdout, stderr);
    }

    if (std.mem.eql(u8, command, "serve")) {
        return serveCommand(argv[2..], stdout, stderr);
    }

    if (std.mem.eql(u8, command, "status")) {
        return statusCommand(argv[2..], stdout, stderr);
    }

    stderr.print("unknown command: {s}\n\n", .{command}) catch {};
    printUsage(stderr);
    return .usage;
}

fn printUsage(writer: anytype) void {
    usage.printUsage(writer) catch {};
}

fn serveCommand(serve_args: []const []const u8, stdout: anytype, stderr: anytype) ExitCode {
    const parsed = cli_args.parseServeArgs(serve_args, stderr);

    switch (parsed) {
        .usage => return .usage,
        .ok => |config| {
            // Use serveForever for daemon-style blocking - the process stays alive
            // until interrupted by a signal. This is the correct CLI behavior.
            // Structured JSON logs go to stdout.
            http.serveForever(config, stdout) catch |err| {
                // Log error as structured JSON to stderr using logging module
                var log_buf = logging.BufferedWriter.init();
                logging.emit(.server_error, &log_buf, &.{
                    .{ .name = "error", .value = logging.FieldValue{ .string = @errorName(err) } },
                }) catch {};
                stderr.writeAll(log_buf.slice()) catch {};
                return .serve_error;
            };

            return .ok;
        },
    }
}

fn statusCommand(status_args: []const []const u8, stdout: anytype, stderr: anytype) ExitCode {
    if (status_args.len != 1 or !std.mem.eql(u8, status_args[0], "--json")) {
        stderr.writeAll("usage: tovarisch status --json\n") catch {};
        return .usage;
    }

    status.renderPayload(stdout) catch return .usage;
    stdout.writeByte('\n') catch return .usage;

    return .ok;
}

/// Diagnostic command to isolate std.Thread.spawn crash on Linux target.
///
/// This command spawns no-op threads to verify whether std.Thread.spawn
/// itself is stable on the current build/runtime target.
///
/// Variants tested:
/// 1. spawn + join: thread completes and is joined (bounded work pattern)
/// 2. spawn + detach: thread is detached (daemon-lifetime pattern)
///
/// Exit codes:
/// - 0: all variants passed
/// - 1: thread smoke test failed
///
/// This is NOT run automatically by systemd. Operators should run it manually
/// on the target system to diagnose threading issues before re-enabling
/// threaded heartbeat.
fn threadSmokeCommand(stdout: anytype, stderr: anytype) ExitCode {
    // Variant 1: spawn + join (bounded work pattern)
    stdout.writeAll("thread-smoke: variant 1 (spawn+join)... ") catch return .usage;

    const spawn_result = std.Thread.spawn(.{}, noopThread, .{});
    if (spawn_result) |thread| {
        thread.join();
        stdout.writeAll("ok\n") catch return .usage;
    } else |err| {
        stdout.writeAll("FAILED\n") catch {};
        stderr.print("thread-smoke: spawn+join failed: {s}\n", .{@errorName(err)}) catch {};
        return .usage;
    }

    // Variant 2: spawn + detach (daemon-lifetime pattern)
    stdout.writeAll("thread-smoke: variant 2 (spawn+detach, default stack)... ") catch return .usage;

    const detach_result = std.Thread.spawn(.{}, noopSleepThread, .{});
    if (detach_result) |thread| {
        thread.detach();
        stdout.writeAll("ok\n") catch return .usage;
    } else |err| {
        stdout.writeAll("FAILED\n") catch {};
        stderr.print("thread-smoke: spawn+detach failed: {s}\n", .{@errorName(err)}) catch {};
        return .usage;
    }

    // Variant 3: spawn + detach with explicit 64 KiB stack (regression test for R-009)
    // This mirrors the config that crashed on Linux/glibc release target.
    stdout.writeAll("thread-smoke: variant 3 (spawn+detach, 64KiB stack)... ") catch return .usage;

    const small_stack_result = std.Thread.spawn(
        .{ .stack_size = 65536 },
        noopSleepThread,
        .{},
    );
    if (small_stack_result) |thread| {
        thread.detach();
        stdout.writeAll("ok\n") catch return .usage;
    } else |err| {
        stdout.writeAll("FAILED\n") catch {};
        stderr.print("thread-smoke: variant 3 (64KiB stack) failed: {s}\n", .{@errorName(err)}) catch {};
        return .usage;
    }

    stdout.writeAll("thread-smoke: all variants passed\n") catch return .usage;
    return .ok;
}

/// No-op thread function for smoke test variant 1.
fn noopThread() void {
    // Intentionally empty - verifies thread spawns without crash
}

/// No-op thread that sleeps briefly then exits for smoke test variant 2.
fn noopSleepThread() void {
    // Sleep for 100ms then exit - enough time to verify detach works
    var ts: std.c.timespec = .{ .sec = 0, .nsec = 100_000_000 };
    _ = std.c.nanosleep(&ts, null);
}

// --- Tests ---

// VoidWriter: for tests that only need exit codes (no output)
const VoidWriter = struct {
    const Self = @This();

    pub fn writeAll(_: Self, _: []const u8) error{}!void {}
    pub fn write(_: Self, _: []const u8) error{}!void {}
    pub fn print(_: Self, _: []const u8, _: anytype) error{}!void {}
    pub fn writeByte(_: Self, _: u8) error{}!void {}
    pub fn flush(_: Self) error{}!void {}
};

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

// --- Output behavior tests ---
// These tests verify CLI output content using CaptureWriter.

// CaptureWriter: collects bytes written for test assertions.
// Uses a fixed buffer to avoid allocator complexity in tests.
const CaptureWriter = struct {
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

    pub fn print(self: *Self, comptime fmt: []const u8, print_args: anytype) !void {
        if (self.len >= BufSize) return error.BufferOverflow;
        const written = std.fmt.bufPrint(self.buf[self.len..], fmt, print_args) catch return error.BufferOverflow;
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

    /// No-op flush for test writers. Required for writeLogRecord compatibility.
    pub fn flush(_: *Self) error{}!void {}
};

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
// Use parseServeArgs tests in args.zig for argument parsing coverage.
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
