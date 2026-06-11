const std = @import("std");
const cli_args = @import("args.zig");
const usage = @import("usage.zig");
const wg_cmd = @import("wg_cmd.zig");
const status = @import("../status.zig");
const build_info = @import("../build_info.zig");
const http = @import("../http/server.zig");
const logging = @import("../logging.zig");
const bfd_serve = @import("bfd_serve.zig");
const bfd_status = @import("../bfd/status.zig");
const bgp_serve = @import("bgp_serve.zig");
const bgp_status = @import("../bgp/status.zig");
const test_helpers = @import("commands_test_helpers.zig");

pub const ExitCode = enum(u8) {
    ok = 0,
    usage = 2,
    serve_error = 3,
};

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
        return serveCommand(argv[2..], stdout, stderr);
    }

    if (std.mem.eql(u8, command, "status")) {
        return statusCommand(argv[2..], stdout, stderr);
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

fn serveCommand(serve_args: []const []const u8, stdout: anytype, stderr: anytype) ExitCode {
    const parsed = cli_args.parseServeArgs(serve_args, stderr);

    switch (parsed) {
        .usage => return .usage,
        .ok => |serve_config| {
            // Load BFD configuration (unchanged from previous implementation)
            const bfd_result = bfd_serve.loadConfigAndBfd(serve_config.config_path, stderr);

            // Only fail on actual errors, not on "no config" or "disabled"
            switch (bfd_result) {
                .failed => return .serve_error,
                else => {},
            }

            // Get BFD bundle pointer for cleanup
            const bfd_bundle: ?*bfd_serve.BfdServeBundle = switch (bfd_result) {
                .configured => |bundle| bundle,
                else => null,
            };

            // Startup assertion: verify BFD bundle is properly initialized before serve
            if (bfd_bundle) |bundle| {
                if (bundle.loop_state == null) {
                    stderr.writeAll("FATAL: BFD bundle initialized but loop_state is null\n") catch {};
                    return .serve_error;
                }
                if (bundle.transmit_loop_state == null) {
                    stderr.writeAll("FATAL: BFD bundle initialized but transmit_loop_state is null\n") catch {};
                    return .serve_error;
                }
                if (bundle.receive_thread == null) {
                    stderr.writeAll("FATAL: BFD bundle initialized but receive_thread is null\n") catch {};
                    return .serve_error;
                }
                if (bundle.transmit_thread == null) {
                    stderr.writeAll("FATAL: BFD bundle initialized but transmit_thread is null\n") catch {};
                    return .serve_error;
                }
                if (!bundle.bfd_active) {
                    stderr.writeAll("FATAL: BFD bundle initialized but bfd_active is false\n") catch {};
                    return .serve_error;
                }
                if (bundle.stop_signal.load()) {
                    stderr.writeAll("FATAL: BFD bundle initialized but stop_signal is already set\n") catch {};
                    return .serve_error;
                }
            }

            // Load BGP configuration (ACT 4: BGP wiring)
            // CRITICAL: When BGP is disabled, this creates ZERO sockets.

            // Log BGP load start for observability
            var bgp_log_buf = logging.BufferedWriter.init();
            logging.logBgpLoadStarted(&bgp_log_buf) catch {};
            stderr.writeAll(bgp_log_buf.slice()) catch {};

            const bgp_result = bgp_serve.loadConfigAndBgp(serve_config.config_path, stderr);

            // Log BGP load result for observability
            bgp_log_buf.reset();
            const result_tag: []const u8 = switch (bgp_result) {
                .configured => "configured",
                .disabled => "disabled",
                .no_config => "no_config",
                .failed => |load_err| load_err.message,
            };
            logging.logBgpLoadResult(&bgp_log_buf, result_tag, "") catch {};
            stderr.writeAll(bgp_log_buf.slice()) catch {};

            // Preserve BgpLoadResult tag for status derivation.
            // BGP failures must be visible as ".failed", not silently collapsed to ".no_config".
            const bgp_bundle: ?*bgp_serve.BgpServeBundle = switch (bgp_result) {
                .configured => |bundle| bundle,
                else => null,
            };

            // Clean up bundle on any exit
            defer if (bgp_bundle) |bundle| bgp_serve.cleanupBgpBundle(bundle);

            // Derive BGP status preserving the load result tag.
            // This prevents ".failed" from collapsing into ".no_config".
            const bgp_state: bgp_status.BgpStatusState = switch (bgp_result) {
                .configured => |bundle| bgp_status.deriveStatusStateFromBundle(bundle),
                .disabled => bgp_status.BgpStatusState.disabled,
                .no_config => bgp_status.BgpStatusState.no_config,
                .failed => |load_err| bgp_status.BgpStatusState{
                    .failed = .{ .message = load_err.message },
                },
            };

            // Extract optional BFD runtime pointer for HTTP server
            const bfd_rt: ?*const bfd_status.BfdRuntime = if (bfd_bundle) |bundle|
                &bundle.runtime
            else
                null;

            // Derive config check state from config_path.
            const config_check: status.ConfigCheckState = if (serve_config.config_path) |path|
                .{ .loaded = .{ .path = path } }
            else
                .no_config;

            // Clean up BFD bundle on any exit
            defer if (bfd_bundle) |bundle| bfd_serve.cleanupBfdBundle(bundle);

            http.serveForeverWithContext(serve_config.http_config, .{
                .bfd_runtime = bfd_rt,
                .config_check = config_check,
                .bgp_state = bgp_state,
            }, stdout) catch |err| {
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
