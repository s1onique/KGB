// wg_status_boundary_cli.zig — CLI backend for WireGuard status boundary
//
// Part of wg_status_boundary.zig (split to satisfy LLM-friendliness limits).
// Contains only the CLI backend implementation.
//
// Phase 1 CLI backend uses configured interface identity via `wg show <iface> dump`.
// Phase 2 generic netlink remains future work.

const std = @import("std");
const wg = @import("wg_status_boundary.zig");
const config_parse_helpers = @import("../config_parse_helpers.zig");

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
// Interface Name Configuration
// ============================================================================

/// Default WireGuard interface name when not explicitly configured.
/// Documented single source of truth for this default value.
/// This is a compile-time constant; Zig inner functions cannot capture
/// instance fields, so interface name must be fixed at compile time.
///
/// Phase 1: Only DEFAULT_WG_INTERFACE is supported.
/// Runtime configurable interface name is future work.
pub const DEFAULT_WG_INTERFACE: [:0]const u8 = "wg-kgb0";

/// Validates an interface name for safety before passing to wg command.
///
/// Rejects:
///   - empty strings
///   - whitespace characters
///   - forward slashes (path traversal attempt)
///   - shell metacharacters
///   - names exceeding Linux interface name limits (IFNAMSIZ-1 = 15 bytes)
pub fn isValidInterfaceName(name: []const u8) bool {
    return config_parse_helpers.isValidInterfaceName(name);
}


// ============================================================================
// CLI Backend Implementation
// ============================================================================

/// CLI backend using `wg show <interface> dump` command.
/// Uses machine-readable tab-separated dump format to avoid human parsing issues.
///
/// Safety properties:
///   - Fixed argv only, no shell interpolation
///   - Bounded stdout/stderr capture (8KB stdout, 1KB stderr)
///   - Bounded timeout with SIGKILL enforcement
///   - Explicit command allowlist (only /usr/bin/wg, /usr/sbin/wg, /sbin/wg)
///   - Validated interface name before execve
///
/// Note: Interface name is a compile-time constant (DEFAULT_WG_INTERFACE).
/// Zig inner functions cannot capture instance fields, so we use a fixed
/// interface name rather than per-instance configuration.
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

    /// Override path for testing (null = use default WG_PATHS).
    /// This allows tests to inject a fake wg command without modifying PATH.
    test_only_wg_path: ?[*:0]const u8 = null,

    /// Initialize CLI backend with defaults (uses DEFAULT_WG_INTERFACE).
    pub fn init() CliBackend {
        return CliBackend{};
    }

    /// Initialize CLI backend with a test-only wg path override.
    /// Only for use in tests - allows injecting a fake wg command.
    pub fn initWithTestWgPath(wg_path: [*:0]const u8) CliBackend {
        return CliBackend{ .test_only_wg_path = wg_path };
    }

    /// Convert to generic backend trait.
    /// Uses DEFAULT_WG_INTERFACE for the wg show command.
    ///
    /// Phase 1: Uses compile-time constant interface name.
    /// Note: Zig inner functions cannot capture outer scope variables,
    /// so interface_name must be a compile-time constant.
    pub fn asBackend(self: *const CliBackend) wg.WireGuardStatusBackend {
        _ = self;
        return wg.WireGuardStatusBackend{
            .wireguardStatusFn = struct {
                fn f(allocator: std.mem.Allocator, _: ?*anyopaque) wg.StatusError!wg.WireGuardStatusResult {
                    // For production, use default path lookup
                    return cliWireguardStatus(allocator, null);
                }
            }.f,
            .backendKindFn = struct {
                fn f(_: ?*anyopaque) wg.BackendKind {
                    return .cli;
                }
            }.f,
        };
    }

    /// Test-only: WireGuard status with explicit wg command path.
    /// This exercises the real CLI path (fork/execve) but with a fake command.
    /// Returns the result directly (not wrapped in backend trait) for testing.
    pub fn wireguardStatusWithPath(self: *CliBackend, allocator: std.mem.Allocator) wg.StatusError!wg.WireGuardStatusResult {
        return cliWireguardStatus(allocator, self.test_only_wg_path);
    }
};

/// Standalone wireguardStatus implementation for CLI backend.
/// Uses DEFAULT_WG_INTERFACE and DEFAULT_TIMEOUT_SECS.
/// 
/// The test_path_override parameter allows tests to inject a fake wg command
/// without modifying PATH. Pass null for production use.
fn cliWireguardStatus(allocator: std.mem.Allocator, test_path_override: ?[*:0]const u8) wg.StatusError!wg.WireGuardStatusResult {
    // Validate interface name before execve (defense in depth)
    if (!isValidInterfaceName(DEFAULT_WG_INTERFACE)) {
        return error.interface_missing;
    }

    const wg_path = test_path_override orelse (findWgCommand() orelse return error.backend_missing);

    const cmd_result = runWgShowDump(allocator, wg_path, CliBackend.DEFAULT_TIMEOUT_SECS) catch |err| {
        return mapCollectorError(err);
    };
    // MemoryOwnership: Free stdout/stderr buffers on ALL return paths.
    // Critical fix for per-request RSS leak (~18KB/request) observed during /status hammering.
    // Defer handles success path and error paths where we control the return.
    defer {
        allocator.free(cmd_result.stdout);
        allocator.free(cmd_result.stderr);
    }

    if (cmd_result.exit_code == 127) return error.backend_missing;
    if (cmd_result.exit_code == 126) return error.permission_denied;

    if (cmd_result.timed_out) return error.timeout;

    if (cmd_result.exit_code != 0) {
        // wg show <iface> returns exit 1 if interface doesn't exist
        if (cmd_result.exit_code == 1) {
            return error.interface_missing;
        }
        return error.command_failed;
    }

    if (cmd_result.stdout_truncated) {
        return error.command_failed;
    }

    // Parse output with explicit interface name (not invented from output)
    const status = wg.parseWgDumpOutput(cmd_result.stdout, DEFAULT_WG_INTERFACE) catch |parse_err| {
        _ = parse_err;
        return error.malformed_output;
    };

    // MemoryOwnership: On success, we return without stderr diagnostic to avoid
    // dangling pointer issues (defer would free stderr while result borrows it).
    // Stderr diagnostics are primarily useful for error conditions, not success.
    // The result uses empty diagnostic on success, which is fine for the current
    // status check use case where stderr isn't exposed.
    return wg.WireGuardStatusResult.ok(status, .cli);
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

/// Internal: run `wg show <interface> dump` and capture output with timeout enforcement.
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
        _ = std.c.close(stderr_pipe[1]);

        // Use wg show <interface> dump for machine-readable per-interface output
        // Uses DEFAULT_WG_INTERFACE as the single source of truth
        const argv: [5]?[*:0]const u8 = .{ wg_path, "show", DEFAULT_WG_INTERFACE.ptr, "dump", null };
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
    var stdout_buf = try allocator.alloc(u8, CliBackend.MAX_STDOUT_SIZE);
    errdefer allocator.free(stdout_buf);

    var stderr_buf = try allocator.alloc(u8, CliBackend.MAX_STDERR_SIZE);
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
            if (!stdout_truncated and stdout_len < CliBackend.MAX_STDOUT_SIZE) {
                const n = std.c.read(stdout_pipe[0], stdout_buf.ptr + stdout_len, CliBackend.MAX_STDOUT_SIZE - stdout_len);
                if (n > 0) {
                    stdout_len += @intCast(n);
                } else if (n == 0 or (n < 0 and std.c.errno(n) != .AGAIN)) {
                    poll_fds[0].fd = -1;
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
                    poll_fds[1].fd = -1;
                }
            }
            if (stderr_len >= CliBackend.MAX_STDERR_SIZE) stderr_truncated = true;
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

    return WgDumpResult{
        .exit_code = if ((status & 0x7f) == 0) (status >> 8) & 0xff else -1,
        .stdout = stdout_buf[0..stdout_len],
        .stderr = stderr_buf[0..stderr_len],
        .stdout_truncated = stdout_truncated,
        .stderr_truncated = stderr_truncated,
        .timed_out = timed_out,
    };
}

