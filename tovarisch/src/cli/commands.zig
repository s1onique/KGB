// cli/commands.zig — CLI command dispatcher
//
// Thin facade that delegates to command modules.
// Extracted from the monolithic file to satisfy LLM-friendliness line limits.

const std = @import("std");
const usage = @import("usage.zig");
const wg_cmd = @import("wg_cmd.zig");
const build_info = @import("../build_info.zig");
const test_helpers = @import("commands_test_helpers.zig");

// Extracted command modules
const command_model = @import("command_model.zig");
const status_command = @import("status_command.zig");
const daemon_command = @import("daemon_command.zig");

// Re-export ExitCode for callers that import commands.zig directly
pub const ExitCode = command_model.ExitCode;

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
        stdout.print("tovarisch {s}\n", .{build_info.version}) catch return .usage;
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
        return daemon_command.serveCommand(argv[2..], stdout, stderr);
    }

    if (std.mem.eql(u8, command, "status")) {
        return status_command.statusCommand(argv[2..], stdout, stderr);
    }

    if (std.mem.eql(u8, command, "wg")) {
        return wgCommand(argv[2..], stdout, stderr);
    }

    stderr.print("unknown command: {s}\n\n", .{command}) catch {};
    printUsage(stderr);
    return .usage;
}

fn printUsage(writer: anytype) void {
    usage.printUsage(writer) catch {};
}

fn wgCommand(wg_args_list: []const []const u8, stdout: anytype, stderr: anytype) ExitCode {
    const exit_code = wg_cmd.wgCommand(wg_args_list, stdout, stderr, std.heap.page_allocator);
    return if (exit_code == 0) .ok else .usage;
}

/// Spawns no-op threads to verify std.Thread.spawn stability.
fn threadSmokeCommand(stdout: anytype, stderr: anytype) ExitCode {
    // Variant 1: spawn + join
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

    // Variant 2: spawn + detach (default stack)
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

    // Variant 3: spawn + detach (64 KiB stack, regression test R-009)
    stdout.writeAll("thread-smoke: variant 3 (spawn+detach, 64KiB stack)... ") catch return .usage;
    const small_stack_result = std.Thread.spawn(.{ .stack_size = 65536 }, noopSleepThread, .{});
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

fn noopThread() void {}

fn noopSleepThread() void {
    var ts: std.c.timespec = .{ .sec = 0, .nsec = 100_000_000 };
    _ = std.c.nanosleep(&ts, null);
}

// Re-export test helpers for external use
const VoidWriter = test_helpers.VoidWriter;
const CaptureWriter = test_helpers.CaptureWriter;

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

test "--help output contains tovarisch serve" {
    var cw = CaptureWriter.init();
    const code = run(&.{ "tovarisch", "--help" }, &cw, &cw);
    try std.testing.expect(code == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, cw.slice(), 1, "tovarisch serve"));
}

test "--help output does NOT contain deprecated --listen-all" {
    var cw = CaptureWriter.init();
    const code = run(&.{ "tovarisch", "--help" }, &cw, &cw);
    try std.testing.expect(code == .ok);
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

test "--version output contains base_version prefix" {
    var cw = CaptureWriter.init();
    const code = run(&.{ "tovarisch", "--version" }, &cw, &cw);
    try std.testing.expect(code == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, cw.slice(), 1, build_info.base_version));
    try std.testing.expect(std.mem.containsAtLeast(u8, cw.slice(), 1, "+"));
}

test "check output contains tovarisch check: ok" {
    var cw = CaptureWriter.init();
    const code = run(&.{ "tovarisch", "check" }, &cw, &cw);
    try std.testing.expect(code == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, cw.slice(), 1, "tovarisch check: ok"));
}

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

test "CLI exit codes match expected behavior" {
    const w = VoidWriter{};
    try std.testing.expect(run(&.{ "tovarisch", "--help" }, w, w) == .ok);
    try std.testing.expect(run(&.{ "tovarisch", "-h" }, w, w) == .ok);
    try std.testing.expect(run(&.{ "tovarisch", "--version" }, w, w) == .ok);
    try std.testing.expect(run(&.{ "tovarisch", "check" }, w, w) == .ok);
    try std.testing.expect(run(&.{ "tovarisch", "status", "--json" }, w, w) == .ok);
    try std.testing.expect(run(&.{"tovarisch"}, w, w) == .usage);
    try std.testing.expect(run(&.{ "tovarisch", "badcmd" }, w, w) == .usage);
    try std.testing.expect(run(&.{ "tovarisch", "serve", "--unknown" }, w, w) == .usage);
}

test "wg command returns ok with --help" {
    const w = VoidWriter{};
    try std.testing.expect(run(&.{ "tovarisch", "wg", "--help" }, w, w) == .ok);
}

test "wg generate without args returns usage" {
    const w = VoidWriter{};
    try std.testing.expect(run(&.{ "tovarisch", "wg", "generate" }, w, w) == .usage);
}

test "--help output contains tovarisch wg generate" {
    var cw = CaptureWriter.init();
    const code = run(&.{ "tovarisch", "--help" }, &cw, &cw);
    try std.testing.expect(code == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, cw.slice(), 1, "tovarisch wg generate"));
}

