// wg_status_boundary_cli_run.zig — CLI command execution for WireGuard status
//
// Part of wg_status_boundary_cli.zig (split for LLM-friendliness).
// Contains runWgShowDump and OwnedWgCommandResult.
//
// ACT-HULK29R-ZIG016-WG-PEERS-DIAGNOSTIC-INTEGRATION

const std = @import("std");

// POSIX fcntl and open flags (not exposed in Zig 0.16 std.c)
const F_GETFL: c_int = 3;
const F_SETFL: c_int = 4;

// Platform-specific O_NONBLOCK:
// - Linux: octal 04000 = decimal 2048
// - macOS/BSD: 0x0004 = decimal 4
const O_NONBLOCK: c_int = if (@import("builtin").os.tag == .linux) 2048 else 4;

// POSIX poll event flags (not exposed in Zig 0.16 std.c)
const POLLIN: c_short = 0x0001;
const POLLHUP: c_short = 0x0010;
const POLLERR: c_short = 0x0008;

// ============================================================================
// OwnedWgCommandResult — Explicit ownership for command output
// ============================================================================

/// Owned result type for WireGuard command output.
/// Provides explicit ownership contract with single deinit() method.
/// This makes memory ownership mechanically reviewable.
pub const OwnedWgCommandResult = struct {
    /// Full allocated stdout buffer (may be larger than used slice).
    stdout_storage: []u8,
    /// Full allocated stderr buffer (may be larger than used slice).
    stderr_storage: []u8,
    /// Actual stdout content (slice of stdout_storage).
    stdout: []const u8,
    /// Actual stderr content (slice of stderr_storage).
    stderr: []const u8,
    exit_code: c_int,
    stdout_truncated: bool,
    stderr_truncated: bool,
    timed_out: bool,

    /// Frees all owned allocations.
    /// Must be called on all return paths from the consumer.
    pub fn deinit(self: *OwnedWgCommandResult, allocator: std.mem.Allocator) void {
        allocator.free(self.stdout_storage);
        allocator.free(self.stderr_storage);
        self.* = undefined;
    }
};

/// Run `wg show <interface> dump` and capture output with timeout enforcement.
/// Uses nonblocking reads with poll() to avoid pipe deadlock between stdout/stderr.
/// Returns OwnedWgCommandResult with explicit ownership - caller must call deinit().
pub fn runWgShowDump(
    allocator: std.mem.Allocator,
    wg_path: [*:0]const u8,
    interface_name: []const u8,
    timeout_secs: u64,
) !OwnedWgCommandResult {
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
        _ = std.c.close(stderr_pipe[1]);

        // Use wg show <interface> dump for machine-readable per-interface output.
        // Null-terminate interface_name using a sentinel slice. bufPrint with \x00
        // writes exactly interface_name.len + 1 bytes; we assert that fits in buf
        // and then use [:0]T sentinel slice to get a [*:0]const u8 pointer safely.
        var iface_buf: [64]u8 = undefined;
        const iface_formatted = std.fmt.bufPrint(&iface_buf, "{s}\x00", .{interface_name}) catch {
            // Formatting failed (interface_name too long) - fail cleanly in child.
            std.c._exit(127);
        };
        // SAFETY: bufPrint guarantees iface_formatted contains a null terminator.
        // [:0]T sentinel slice makes this explicit in the type system.
        const iface_null_terminated: [*:0]const u8 = @ptrCast(iface_buf[0..iface_formatted.len - 1 :0].ptr);

        const argv: [5]?[*:0]const u8 = .{ wg_path, "show", iface_null_terminated, "dump", null };
        const argv_null: [*:null]const ?[*:0]const u8 = @ptrCast(&argv);
        const empty_env: [*:null]const ?[*:0]const u8 = &.{};
        _ = std.c.execve(wg_path, argv_null, empty_env);

        std.c._exit(127);
    }

    // Parent process
    _ = std.c.close(stdout_pipe[1]);
    _ = std.c.close(stderr_pipe[1]);

    // Allocate buffers with errdefer for cleanup on allocation failure.
    // If stderr_buf allocation fails after stdout_buf succeeds, errdefer
    // cleans up stdout_buf. Once runWgShowDump returns successfully, the
    // caller's defer handles cleanup - no conflict.
    const MAX_STDOUT: usize = 8192;
    const MAX_STDERR: usize = 1024;
    var stdout_buf = try allocator.alloc(u8, MAX_STDOUT);
    errdefer allocator.free(stdout_buf);

    var stderr_buf = try allocator.alloc(u8, MAX_STDERR);
    errdefer allocator.free(stderr_buf);
    var stdout_len: usize = 0;
    var stderr_len: usize = 0;
    var stdout_truncated = false;
    var stderr_truncated = false;
    var timed_out = false;

    // Use poll() to avoid deadlock on concurrent stdout/stderr
    var poll_fds: [2]std.c.pollfd = .{
        .{ .fd = stdout_pipe[0], .events = POLLIN, .revents = 0 },
        .{ .fd = stderr_pipe[0], .events = POLLIN, .revents = 0 },
    };

    var remaining_ms: i32 = @intCast(timeout_secs * 1000);
    const poll_interval_ms: i32 = 100;

    // Read until both pipes are closed or timeout expires
    while (true) {
        if (stdout_truncated and stderr_truncated) break;

        const poll_ms: i32 = @min(remaining_ms, poll_interval_ms);
        const poll_result = std.c.poll(&poll_fds, 2, poll_ms);
        remaining_ms -= poll_ms;

        if (poll_result < 0) {
            continue;
        }

        if (remaining_ms <= 0) {
            timed_out = true;
            break;
        }

        // Read stdout if ready
        if (poll_fds[0].revents & POLLIN != 0) {
            if (!stdout_truncated and stdout_len < MAX_STDOUT) {
                const n = std.c.read(stdout_pipe[0], stdout_buf.ptr + stdout_len, MAX_STDOUT - stdout_len);
                if (n > 0) {
                    stdout_len += @intCast(n);
                } else if (n == 0 or (n < 0 and std.c.errno(n) != .AGAIN)) {
                    poll_fds[0].fd = -1;
                }
            }
            if (stdout_len >= MAX_STDOUT) stdout_truncated = true;
        }

        // Read stderr if ready
        if (poll_fds[1].revents & POLLIN != 0) {
            if (!stderr_truncated and stderr_len < MAX_STDERR) {
                const n = std.c.read(stderr_pipe[0], stderr_buf.ptr + stderr_len, MAX_STDERR - stderr_len);
                if (n > 0) {
                    stderr_len += @intCast(n);
                } else if (n == 0 or (n < 0 and std.c.errno(n) != .AGAIN)) {
                    poll_fds[1].fd = -1;
                }
            }
            if (stderr_len >= MAX_STDERR) stderr_truncated = true;
        }

        // Check for hangup on both pipes
        if ((poll_fds[0].revents & (POLLHUP | POLLERR) != 0) and
            (poll_fds[1].revents & (POLLHUP | POLLERR) != 0)) {
            break;
        }

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

    return OwnedWgCommandResult{
        .stdout_storage = stdout_buf,
        .stderr_storage = stderr_buf,
        .stdout = stdout_buf[0..stdout_len],
        .stderr = stderr_buf[0..stderr_len],
        .exit_code = if ((status & 0x7f) == 0) (status >> 8) & 0xff else -1,
        .stdout_truncated = stdout_truncated,
        .stderr_truncated = stderr_truncated,
        .timed_out = timed_out,
    };
}
