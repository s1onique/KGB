// safe_command.zig — Bounded safe command runner for network diagnostics
//
// ACT: Add tovarisch WireGuard and XRay TCP underlay diagnostics
// Safe command execution without shell, with bounded output.
//
// Safety properties:
// - Fixed argv only, no shell interpolation
// - Bounded stdout/stderr capture
// - Explicit command allowlist
// - No user-controlled paths

const std = @import("std");

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
};

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
};

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

        var argv = std.ArrayListUnmanaged([*:0]const u8){ .items = &.{}, .capacity = 0 };
        errdefer argv.deinit(allocator);

        try argv.append(allocator, exe_path);
        for (args) |arg| {
            try argv.append(allocator, arg);
        }
        // Null-terminate argv
        const null_sentinel: [*:0]const u8 = "";
        try argv.append(allocator, null_sentinel);

        const empty_env: [*:null]const ?[*:0]const u8 = &.{};
        _ = std.c.execve(exe_path, @ptrCast(argv.items.ptr), empty_env);
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

// ============================================================================
// Tests
// ============================================================================

test "CommandConfig has sensible defaults" {
    const cfg = CommandConfig{};
    try std.testing.expectEqual(@as(usize, 8192), cfg.max_stdout_size);
    try std.testing.expectEqual(@as(usize, 1024), cfg.max_stderr_size);
}

test "CommandResult can be constructed" {
    const allocator = std.testing.allocator;
    const stdout_bytes = "test output";
    const stdout_buf = try allocator.dupe(u8, stdout_bytes);
    defer allocator.free(stdout_buf);
    
    const stderr_bytes = "";
    const stderr_buf = try allocator.dupe(u8, stderr_bytes);
    defer allocator.free(stderr_buf);
    
    const result = CommandResult{
        .exit_code = 0,
        .stdout = stdout_buf,
        .stderr = stderr_buf,
    };
    try std.testing.expectEqual(@as(c_int, 0), result.exit_code);
    try std.testing.expectEqualStrings("test output", result.stdout);
}
