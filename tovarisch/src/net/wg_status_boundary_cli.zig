// wg_status_boundary_cli.zig — CLI backend for WireGuard status boundary
//
// Part of wg_status_boundary.zig (split to satisfy LLM-friendliness limits).
// Contains only the CLI backend implementation.

const std = @import("std");
const wg = @import("wg_status_boundary.zig");

// POSIX fcntl and open flags (not exposed in Zig 0.16 std.c)
const F_GETFL: c_int = 3;
const F_SETFL: c_int = 4;
const O_NONBLOCK: c_int = 4;

// POSIX poll event flags (not exposed in Zig 0.16 std.c)
const POLLIN: c_short = 0x0001;
const POLLHUP: c_short = 0x0010;
const POLLERR: c_short = 0x0008;


// ============================================================================
// CLI Backend Implementation
// ============================================================================

/// CLI backend using `wg show dump` command.
/// Uses machine-readable tab-separated dump format to avoid human parsing issues.
///
/// Safety properties:
///   - Fixed argv only, no shell interpolation
///   - Bounded stdout/stderr capture (8KB stdout, 1KB stderr)
///   - Bounded timeout with SIGKILL enforcement
///   - Explicit command allowlist (only /usr/bin/wg, /usr/sbin/wg, /sbin/wg)
pub const CliBackend = struct {
    /// Default timeout for wg show command (5 seconds).
    pub const DEFAULT_TIMEOUT_SECS: u64 = 5;

    /// Maximum stdout buffer size (8KB) - sufficient for dump output.
    pub const MAX_STDOUT_SIZE: usize = 8192;

    /// Maximum stderr buffer size (1KB).
    pub const MAX_STDERR_SIZE: usize = 1024;

    /// Allowed paths to the WireGuard `wg` command.
    const WG_PATHS = [_][*:0]const u8{
        "/usr/bin/wg",
        "/usr/sbin/wg",
        "/sbin/wg",
    };

    /// Initialize CLI backend with defaults.
    pub fn init() CliBackend {
        return CliBackend{};
    }

    /// Convert to generic backend trait.
    /// Phase 1: Uses fixed DEFAULT_TIMEOUT_SECS (5 seconds) for all CLI operations.
    /// Note: Zig inner functions cannot capture outer scope variables, so custom
    /// timeout is not supported through the backend trait. This is a Phase 1 limitation.
    pub fn asBackend(self: *const CliBackend) wg.WireGuardStatusBackend {
        _ = self;
        return wg.WireGuardStatusBackend{
            .wireguardStatusFn = struct {
                fn f(allocator: std.mem.Allocator) wg.StatusError!wg.WireGuardStatusResult {
                    return cliWireguardStatus(allocator);
                }
            }.f,
            .backendKindFn = struct {
                fn f() wg.BackendKind {
                    return .cli;
                }
            }.f,
        };
    }
};

/// Standalone wireguardStatus implementation for CLI backend.
/// Uses DEFAULT_TIMEOUT_SECS.
fn cliWireguardStatus(allocator: std.mem.Allocator) wg.StatusError!wg.WireGuardStatusResult {
    const wg_path = findWgCommand() orelse return error.backend_missing;

    const cmd_result = runWgShowDump(allocator, wg_path, CliBackend.DEFAULT_TIMEOUT_SECS) catch |err| {
        return mapCollectorError(err);
    };
    errdefer allocator.free(cmd_result.stdout);
    errdefer allocator.free(cmd_result.stderr);

    if (cmd_result.exit_code == 127) return error.backend_missing;
    if (cmd_result.exit_code == 126) return error.permission_denied;

    if (cmd_result.timed_out) return error.timeout;

    if (cmd_result.exit_code != 0) {
        return error.command_failed;
    }

    if (cmd_result.stdout_truncated) {
        return error.command_failed;
    }

    const status = wg.parseWgDumpOutput(cmd_result.stdout) catch |parse_err| {
        _ = parse_err;
        return error.malformed_output;
    };

    return wg.WireGuardStatusResult.withDiagnostic(status, .cli, cmd_result.stderr);
}

/// Maps legacy collector errors to structured StatusError.
fn mapCollectorError(err: anytype) wg.StatusError {
    return switch (err) {
        error.PipeFailed => error.command_failed,
        error.ForkFailed => error.command_failed,
        error.OutOfMemory => error.out_of_memory,
    };
}

/// Internal: find first available wg command path.
fn findWgCommand() ?[*:0]const u8 {
    for (CliBackend.WG_PATHS) |path| {
        if (std.c.access(path, std.c.X_OK) == 0) {
            return path;
        }
    }
    return null;
}

/// Internal: result type for runWgShowDump.
const WgDumpResult = struct {
    exit_code: c_int,
    stdout: []u8,
    stderr: []u8,
    stdout_truncated: bool,
    stderr_truncated: bool,
    timed_out: bool,
};

/// Internal: run `wg show dump` and capture output with timeout enforcement.
///
/// Uses nonblocking reads with poll() to avoid pipe deadlock between stdout/stderr.
fn runWgShowDump(allocator: std.mem.Allocator, wg_path: [*:0]const u8, timeout_secs: u64) !WgDumpResult {
    var stdout_pipe: [2]c_int = undefined;
    var stderr_pipe: [2]c_int = undefined;

    if (std.c.pipe(&stdout_pipe) != 0) return error.PipeFailed;
    if (std.c.pipe(&stderr_pipe) != 0) {
        _ = std.c.close(stdout_pipe[0]);
        _ = std.c.close(stdout_pipe[1]);
        return error.PipeFailed;
    }

    // Set stdout pipe to nonblocking before fork
    const stdout_flags: c_int = std.c.fcntl(stdout_pipe[0], F_GETFL, @as(c_int, 0));
    _ = std.c.fcntl(stdout_pipe[0], F_SETFL, stdout_flags | O_NONBLOCK);

    // Set stderr pipe to nonblocking before fork
    const stderr_flags: c_int = std.c.fcntl(stderr_pipe[0], F_GETFL, @as(c_int, 0));
    _ = std.c.fcntl(stderr_pipe[0], F_SETFL, stderr_flags | O_NONBLOCK);

    const pid = std.c.fork();
    if (pid < 0) {
        _ = std.c.close(stdout_pipe[0]);
        _ = std.c.close(stdout_pipe[1]);
        _ = std.c.close(stderr_pipe[0]);
        _ = std.c.close(stderr_pipe[1]);
        return error.ForkFailed;
    }

    if (pid == 0) {
        // Child process
        _ = std.c.close(stdout_pipe[0]);
        _ = std.c.close(stderr_pipe[0]);

        _ = std.c.dup2(stdout_pipe[1], 1);
        _ = std.c.close(stdout_pipe[1]);

        _ = std.c.dup2(stderr_pipe[1], 2);
        // FIX: Close stderr_pipe[1] AFTER dup2, not fd 2
        _ = std.c.close(stderr_pipe[1]);

        // Use wg show dump for machine-readable output
        const argv: [4]?[*:0]const u8 = .{ wg_path, "show", "dump", null };
        const argv_null: [*:null]const ?[*:0]const u8 = @ptrCast(&argv);
        const empty_env: [*:null]const ?[*:0]const u8 = &.{};
        _ = std.c.execve(wg_path, argv_null, empty_env);

        std.c._exit(127);
    }

    // Parent process
    _ = std.c.close(stdout_pipe[1]);
    _ = std.c.close(stderr_pipe[1]);

    // Allocate buffers
    var stdout_buf = try allocator.alloc(u8, CliBackend.MAX_STDOUT_SIZE);
    var stderr_buf = try allocator.alloc(u8, CliBackend.MAX_STDERR_SIZE);
    var stdout_len: usize = 0;
    var stderr_len: usize = 0;
    var stdout_truncated = false;
    var stderr_truncated = false;
    var timed_out = false;

    // Use poll() to avoid deadlock on concurrent stdout/stderr
    // Track remaining time ourselves since poll() doesn't have a "remaining time" feature
    var poll_fds: [2]std.c.pollfd = .{
        .{ .fd = stdout_pipe[0], .events = POLLIN, .revents = 0 },
        .{ .fd = stderr_pipe[0], .events = POLLIN, .revents = 0 },
    };

    var remaining_ms: i32 = @intCast(timeout_secs * 1000);
    const poll_interval_ms: i32 = 100; // Check every 100ms for responsive timeout

    // Read until both pipes are closed or timeout expires
    while (true) {
        // Check if we have all the data we need
        if (stdout_truncated and stderr_truncated) break;

        // Calculate actual poll timeout (don't exceed remaining time or our interval)
        const poll_ms: i32 = @min(remaining_ms, poll_interval_ms);

        const poll_result = std.c.poll(&poll_fds, 2, poll_ms);

        // Subtract elapsed time from remaining
        remaining_ms -= poll_ms;

        if (poll_result < 0) {
            // Poll interrupted (e.g., EINTR), check again
            continue;
        }

        // Check for timeout after subtracting elapsed time
        if (remaining_ms <= 0) {
            timed_out = true;
            break;
        }

        // Read stdout if ready
        if (poll_fds[0].revents & POLLIN != 0) {
            if (!stdout_truncated and stdout_len < CliBackend.MAX_STDOUT_SIZE) {
                const n = std.c.read(stdout_pipe[0], stdout_buf.ptr + stdout_len, CliBackend.MAX_STDOUT_SIZE - stdout_len);
                if (n > 0) {
                    stdout_len += @intCast(n);
                } else if (n == 0 or (n < 0 and std.c.errno(n) != .AGAIN)) {
                    poll_fds[0].fd = -1; // EOF or error, don't poll again
                }
            }
            if (stdout_len >= CliBackend.MAX_STDOUT_SIZE) stdout_truncated = true;
        }

        // Read stderr if ready
        if (poll_fds[1].revents & POLLIN != 0) {
            if (!stderr_truncated and stderr_len < CliBackend.MAX_STDERR_SIZE) {
                const n = std.c.read(stderr_pipe[0], stderr_buf.ptr + stderr_len, CliBackend.MAX_STDERR_SIZE - stderr_len);
                if (n > 0) {
                    stderr_len += @intCast(n);
                } else if (n == 0 or (n < 0 and std.c.errno(n) != .AGAIN)) {
                    poll_fds[1].fd = -1; // EOF or error, don't poll again
                }
            }
            if (stderr_len >= CliBackend.MAX_STDERR_SIZE) stderr_truncated = true;
        }

        // Check for hangup on both pipes
        if ((poll_fds[0].revents & (POLLHUP | POLLERR) != 0) and
            (poll_fds[1].revents & (POLLHUP | POLLERR) != 0)) {
            break;
        }

        // Check if both pipes are done
        if (poll_fds[0].fd == -1 and poll_fds[1].fd == -1) {
            break;
        }
    }

    // Drain any remaining stdout if truncated
    if (stdout_truncated) {
        var drain: [256]u8 = undefined;
        while (true) {
            const n = std.c.read(stdout_pipe[0], &drain, drain.len);
            if (n <= 0) break;
        }
    }
    _ = std.c.close(stdout_pipe[0]);
    _ = std.c.close(stderr_pipe[0]);

    // If timed out, kill the child process
    if (timed_out) {
        _ = std.c.kill(pid, .KILL);
    }

    // Wait for child to finish
    var status: c_int = undefined;
    _ = std.c.waitpid(pid, &status, 0);

    return WgDumpResult{
        .exit_code = if ((status & 0x7f) == 0) (status >> 8) & 0xff else -1,
        .stdout = stdout_buf[0..stdout_len],
        .stderr = stderr_buf[0..stderr_len],
        .stdout_truncated = stdout_truncated,
        .stderr_truncated = stderr_truncated,
        .timed_out = timed_out,
    };
}

// ============================================================================
// Fake Backend for Tests
// ============================================================================

/// Fake backend for deterministic unit testing.
pub const FakeBackend = struct {
    /// Pre-configured status to return (null = return err).
    status: ?wg.WireGuardStatus = null,
    /// Pre-configured error to return (null = return status).
    err: ?wg.StatusError = null,
    /// Backend kind for this fake.
    kind: wg.BackendKind = .fake,

    /// Initialize fake backend with no preset (returns no_interface by default).
    pub fn init() FakeBackend {
        return FakeBackend{};
    }

    /// Initialize with a specific status.
    pub fn initWithStatus(status: wg.WireGuardStatus) FakeBackend {
        return FakeBackend{ .status = status };
    }

    /// Initialize with a specific error.
    pub fn initWithError(err: wg.StatusError) FakeBackend {
        return FakeBackend{ .err = err };
    }

    /// Set the status to return.
    pub fn setStatus(self: *FakeBackend, status: wg.WireGuardStatus) void {
        self.status = status;
        self.err = null;
    }

    /// Set the error to return.
    pub fn setError(self: *FakeBackend, err: wg.StatusError) void {
        self.err = err;
        self.status = null;
    }

    /// Set the backend kind.
    pub fn setKind(self: *FakeBackend, kind: wg.BackendKind) void {
        self.kind = kind;
    }

    /// Convert to generic backend trait.
    pub fn asBackend(self: *const FakeBackend) wg.WireGuardStatusBackend {
        return wg.WireGuardStatusBackend{
            .wireguardStatusFn = struct {
                fn f(allocator: std.mem.Allocator) wg.StatusError!wg.WireGuardStatusResult {
                    _ = allocator;
                    return fakeWireguardStatusImpl(self);
                }
            }.f,
            .backendKindFn = struct {
                fn f() wg.BackendKind {
                    return self.kind;
                }
            }.f,
        };
    }

    fn fakeWireguardStatusImpl(self: *const FakeBackend) wg.StatusError!wg.WireGuardStatusResult {
        if (self.err) |e| {
            return e;
        }
        if (self.status) |s| {
            return wg.WireGuardStatusResult.ok(s, self.kind);
        }
        return wg.WireGuardStatusResult.ok(wg.WireGuardStatus.noInterface(), self.kind);
    }
};
