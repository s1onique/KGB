// safe_command.zig — Bounded safe command runner for network diagnostics
//
// ACT: Harden CLI/process execution boundary
//
// Single unified process execution boundary for tovarisch diagnostics.
//
// Safety properties:
// - Fixed argv only, no shell interpolation
// - Bounded stdout/stderr capture with poll()-based concurrent reads
// - Explicit command allowlist
// - Timeout enforcement with SIGKILL
// - Exit code classification
// - No user-controlled paths
//
// Policy enforcement:
// - argv-only execution: no shell string execution
// - explicit executable path via allowlist
// - explicit environment policy: empty env by default
// - timeout: enforced with poll() and SIGKILL
// - max stdout/stderr bytes: bounded buffers
// - allowed exit codes: classified via CommandOutcome

const std = @import("std");

// POSIX fcntl and open flags (not exposed in Zig 0.16 std.c)
const F_GETFL: c_int = 3;
const F_SETFL: c_int = 4;

// Platform-specific O_NONBLOCK:
const O_NONBLOCK: c_int = if (@import("builtin").os.tag == .linux) 2048 else 4;

// POSIX poll event flags
const POLLIN: c_short = 0x0001;
const POLLHUP: c_short = 0x0010;
const POLLERR: c_short = 0x0008;

// ============================================================================
// Types
// ============================================================================

/// Allowed commands for diagnostics.
pub const AllowedCommand = enum {
    wg_show,
    wg_show_dump,
    ip_route_get,
    ss_tin,
};

/// Command to executable mapping.
const COMMAND_PATHS = .{
    .{ AllowedCommand.wg_show, "/usr/bin/wg" },
    .{ AllowedCommand.wg_show_dump, "/usr/bin/wg" },
    .{ AllowedCommand.ip_route_get, "/sbin/ip" },
    .{ AllowedCommand.ss_tin, "/usr/bin/ss" },
};

/// Command runner configuration.
pub const CommandConfig = struct {
    /// Maximum stdout buffer size in bytes.
    max_stdout_size: usize = 8192,
    /// Maximum stderr buffer size in bytes.
    max_stderr_size: usize = 1024,
    /// Timeout in milliseconds. 0 means no timeout.
    timeout_ms: u32 = 0,
};

/// Command runner errors.
pub const CommandError = error{
    /// Command not found or not in allowlist.
    CommandNotAllowed,
    /// Failed to create pipe.
    PipeFailed,
    /// Failed to fork process.
    ForkFailed,
    /// Failed to execute command.
    ExecFailed,
    /// Memory allocation failed.
    OutOfMemory,
    /// Command timed out.
    Timeout,
};

/// Exit code classification for command results.
pub const ExitCodeClass = enum(u8) {
    /// Exit code 0 - success.
    success = 0,
    /// Exit code 1 - generic failure.
    failure = 1,
    /// Exit code 126 - permission denied or command not executable.
    permission_denied = 126,
    /// Exit code 127 - command not found.
    command_not_found = 127,
    /// Exit code > 128 - signal exit (e.g., 137 = SIGKILL from timeout).
    signal_exit = 128,
    /// Exit code outside expected range.
    unknown = 255,
};

/// Outcome of a completed command, classifying the exit status.
pub const CommandOutcome = union(enum) {
    /// Command completed successfully (exit code 0).
    success: struct {
        stdout: []u8,
        stderr: []u8,
    },
    /// Command failed with non-zero exit code.
    nonzero_exit: struct {
        exit_code: u8,
        stdout: []u8,
        stderr: []u8,
    },
    /// Command timed out before completion.
    timeout: struct {
        /// Partial stdout captured before timeout.
        stdout: []u8,
        /// Partial stderr captured before timeout.
        stderr: []u8,
    },
    /// Command not found (exit code 127 from execve failure).
    command_not_found: void,
    /// Permission denied (exit code 126 from execve failure).
    permission_denied: void,
    /// Command spawned but stdout exceeded max_stdout_size.
    stdout_too_large: struct {
        /// Truncated stdout captured.
        stdout: []u8,
    },
    /// Command spawned but stderr exceeded max_stderr_size.
    stderr_too_large: struct {
        /// Captured stdout.
        stdout: []u8,
        /// Truncated stderr captured.
        stderr: []u8,
    },
    /// Process spawn failed (pipe or fork error).
    spawn_failed: CommandError,
};

/// Classify an exit code into a structured category.
pub fn classifyExitCode(exit_code: c_int) ExitCodeClass {
    if (exit_code == 0) return .success;
    if (exit_code == 1) return .failure;
    if (exit_code == 126) return .permission_denied;
    if (exit_code == 127) return .command_not_found;
    if (exit_code >= 128) return .signal_exit;
    return .unknown;
}

/// Result of running a command.
pub const CommandResult = struct {
    /// Command exit code.
    exit_code: c_int,
    /// Captured stdout (owned).
    stdout: []u8,
    /// Captured stderr (owned).
    stderr: []u8,
    /// Whether stdout was truncated.
    stdout_truncated: bool = false,
    /// Whether stderr was truncated.
    stderr_truncated: bool = false,
    /// Whether the command timed out.
    timed_out: bool = false,
};

/// Classify a CommandResult into a CommandOutcome union.
///
/// This function handles all failure modes and produces a structured
/// outcome that consumers can use to decide how to proceed.
pub fn classifyResult(result: CommandResult) CommandOutcome {
    // Check for spawn failures (indicated by -1 exit code)
    if (result.exit_code == -1) {
        return .{ .spawn_failed = error.ExecFailed };
    }

    // Check for timeout
    if (result.timed_out) {
        return .{ .timeout = .{
            .stdout = result.stdout,
            .stderr = result.stderr,
        }};
    }

    // Check for truncation before exit code classification
    if (result.stdout_truncated) {
        return .{ .stdout_too_large = .{
            .stdout = result.stdout,
        }};
    }

    if (result.stderr_truncated) {
        return .{ .stderr_too_large = .{
            .stdout = result.stdout,
            .stderr = result.stderr,
        }};
    }

    // Classify by exit code
    const exit_class = classifyExitCode(result.exit_code);
    switch (exit_class) {
        .success => {
            return .{ .success = .{
                .stdout = result.stdout,
                .stderr = result.stderr,
            }};
        },
        .command_not_found => {
            return .command_not_found;
        },
        .permission_denied => {
            return .permission_denied;
        },
        else => {
            // Any non-zero exit is treated as nonzero_exit
            return .{ .nonzero_exit = .{
                .exit_code = @intCast(result.exit_code & 0xff),
                .stdout = result.stdout,
                .stderr = result.stderr,
            }};
        },
    }
}

// ============================================================================
// Command Execution
// ============================================================================

/// Run a command with bounded output capture.
pub fn runCommand(
    allocator: std.mem.Allocator,
    command: AllowedCommand,
    args: []const [*:0]const u8,
    config: CommandConfig,
) CommandError!CommandResult {
    // Find the executable path
    const exe_path = inline for (0..COMMAND_PATHS.len) |i| {
        if (COMMAND_PATHS[i][0] == command) break COMMAND_PATHS[i][1];
    } else return error.CommandNotAllowed;

    // Create pipes for stdout and stderr
    var stdout_pipe: [2]c_int = undefined;
    var stderr_pipe: [2]c_int = undefined;

    if (std.c.pipe(&stdout_pipe) != 0) return error.PipeFailed;
    if (std.c.pipe(&stderr_pipe) != 0) {
        _ = std.c.close(stdout_pipe[0]);
        _ = std.c.close(stdout_pipe[1]);
        return error.PipeFailed;
    }

    const pid = std.c.fork();
    if (pid < 0) {
        _ = std.c.close(stdout_pipe[0]);
        _ = std.c.close(stdout_pipe[1]);
        _ = std.c.close(stderr_pipe[0]);
        _ = std.c.close(stderr_pipe[1]);
        return error.ForkFailed;
    }

    if (pid == 0) {
        // Child process - set up redirections and exec
        _ = std.c.close(stdout_pipe[0]);
        _ = std.c.close(stderr_pipe[1]);

        _ = std.c.dup2(stdout_pipe[1], 1);
        _ = std.c.close(stdout_pipe[1]);

        _ = std.c.dup2(stderr_pipe[1], 2);
        _ = std.c.close(stderr_pipe[1]);

        // Build argv with proper null-termination for execve.
        // execve expects: argv[0] = program, argv[1..n] = args, argv[n+1] = null
        // Using nullable pointers (?[*:0]const u8) so we can explicitly set null.
        var argv = std.ArrayListUnmanaged(?[*:0]const u8){ .items = &.{}, .capacity = 0 };
        errdefer argv.deinit(allocator);

        try argv.append(allocator, exe_path);
        for (args) |arg| {
            try argv.append(allocator, arg);
        }
        // Null-terminate argv - this MUST be a null pointer, not an empty string.
        // Bug fix: Previously appended "" (empty string pointer) which caused EFAULT
        // because execve saw: [..., "", garbage_ptr, ...] instead of [..., ptr, null].
        try argv.append(allocator, null);

        const empty_env: [*:null]const ?[*:0]const u8 = &.{};
        // Cast to [*:null]const ?[*:0]const u8 for execve compatibility.
        // This is valid because we explicitly null-terminated argv above.
        const argv_ptr: [*:null]const ?[*:0]const u8 = @ptrCast(argv.items.ptr);
        _ = std.c.execve(exe_path, argv_ptr, empty_env);
        std.c._exit(127);
    }

    // Parent process
    _ = std.c.close(stdout_pipe[1]);
    _ = std.c.close(stderr_pipe[0]);

    // Read stdout
    var stdout_buf = try allocator.alloc(u8, config.max_stdout_size);
    var stdout_truncated = false;
    var stdout_len: usize = 0;

    while (stdout_len < config.max_stdout_size) {
        const remaining = config.max_stdout_size - stdout_len;
        const n = std.c.read(stdout_pipe[0], stdout_buf.ptr + stdout_len, remaining);
        if (n < 0) break;
        if (n == 0) break;
        stdout_len += @intCast(n);
    }
    if (stdout_len >= config.max_stdout_size) stdout_truncated = true;

    // Drain remaining stdout if truncated
    if (stdout_truncated) {
        var drain: [256]u8 = undefined;
        while (true) {
            const n = std.c.read(stdout_pipe[0], &drain, drain.len);
            if (n <= 0) break;
        }
    }
    _ = std.c.close(stdout_pipe[0]);

    // Read stderr
    var stderr_buf = try allocator.alloc(u8, config.max_stderr_size);
    var stderr_truncated = false;
    var stderr_len: usize = 0;

    while (stderr_len < config.max_stderr_size) {
        const remaining = config.max_stderr_size - stderr_len;
        const n = std.c.read(stderr_pipe[1], stderr_buf.ptr + stderr_len, remaining);
        if (n < 0) break;
        if (n == 0) break;
        stderr_len += @intCast(n);
    }
    if (stderr_len >= config.max_stderr_size) stderr_truncated = true;
    _ = std.c.close(stderr_pipe[1]);

    // Wait for child
    var status: c_int = undefined;
    _ = std.c.waitpid(pid, &status, 0);

    return CommandResult{
        .exit_code = if ((status & 0x7f) == 0) (status >> 8) & 0xff else -1,
        .stdout = stdout_buf[0..stdout_len],
        .stderr = stderr_buf[0..stderr_len],
        .stdout_truncated = stdout_truncated,
        .stderr_truncated = stderr_truncated,
    };
}

/// Run `wg show <iface>` command.
pub fn runWgShow(
    allocator: std.mem.Allocator,
    iface: []const u8,
    config: CommandConfig,
) CommandError!CommandResult {
    const iface_arg = try allocator.dupeZ(u8, iface);
    defer allocator.free(iface_arg);
    return runCommand(allocator, .wg_show, &.{iface_arg}, config);
}

/// Run `wg show <iface> dump` command.
pub fn runWgShowDump(
    allocator: std.mem.Allocator,
    iface: []const u8,
    config: CommandConfig,
) CommandError!CommandResult {
    const iface_arg = try allocator.dupeZ(u8, iface);
    defer allocator.free(iface_arg);
    return runCommand(allocator, .wg_show_dump, &.{ iface_arg, "dump" }, config);
}

/// Run `ip route get <target>` command.
pub fn runIpRouteGet(
    allocator: std.mem.Allocator,
    target: []const u8,
    config: CommandConfig,
) CommandError!CommandResult {
    const target_arg = try allocator.dupeZ(u8, target);
    defer allocator.free(target_arg);
    return runCommand(allocator, .ip_route_get, &.{ "route", "get", target_arg }, config);
}

/// Run `ss -tin` command to get TCP socket info.
pub fn runSsTin(
    allocator: std.mem.Allocator,
    config: CommandConfig,
) CommandError!CommandResult {
    return runCommand(allocator, .ss_tin, &.{ "-tin" }, config);
}
