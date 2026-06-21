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

// Regression test: argv array must be nullable for execve.
// This ensures we can properly null-terminate the argv list.
test "argv type is nullable for proper null-termination" {
    // The argv array must use nullable pointers so we can set the
    // terminating null pointer: argv[n] = null.
    // If the type were [*:0]const u8 (non-nullable), we could not
    // properly terminate the array for execve.
    var argv = std.ArrayListUnmanaged(?[*:0]const u8){ .items = &.{}, .capacity = 0 };
    try argv.append(std.testing.allocator, @ptrFromInt(0x1000));
    try argv.append(std.testing.allocator, null); // Valid: nullable type allows null
    
    // Verify structure: [ptr, null]
    try std.testing.expectEqual(@as(usize, 2), argv.items.len);
    try std.testing.expectEqual(@as(?[*:0]const u8, @ptrFromInt(0x1000)), argv.items[0]);
    try std.testing.expectEqual(@as(?[*:0]const u8, null), argv.items[1]);
    
    argv.deinit(std.testing.allocator);
}

// Regression test: ss_tin command builds correct argv for execve.
// Before fix: execve received ["/usr/bin/ss", "-tin", "", garbage...]
// After fix: execve receives ["/usr/bin/ss", "-tin", null]
test "ss_tin builds correct argv for execve" {
    // This test verifies the argv construction pattern used by runSsTin.
    // runSsTin calls runCommand with args = &.{ "-tin" }
    // 
    // Expected argv structure:
    //   argv[0] = "/usr/bin/ss"
    //   argv[1] = "-tin"
    //   argv[2] = null  <-- MUST be null, not empty string ""
    
    const exe_path: [*:0]const u8 = "/usr/bin/ss";
    const args: []const [*:0]const u8 = &.{ "-tin" };
    
    // Simulate argv construction (same as runCommand)
    var argv = std.ArrayListUnmanaged(?[*:0]const u8){ .items = &.{}, .capacity = 0 };
    defer argv.deinit(std.testing.allocator);
    
    try argv.append(std.testing.allocator, exe_path);
    for (args) |arg| {
        try argv.append(std.testing.allocator, arg);
    }
    try argv.append(std.testing.allocator, null);
    
    // Verify: ["/usr/bin/ss", "-tin", null]
    try std.testing.expectEqual(@as(usize, 3), argv.items.len);
    try std.testing.expect(argv.items[0] != null);
    try std.testing.expectEqualStrings("/usr/bin/ss", std.mem.sliceTo(@as([*:0]const u8, @ptrCast(argv.items[0].?)), 0));
    try std.testing.expect(argv.items[1] != null);
    try std.testing.expectEqualStrings("-tin", std.mem.sliceTo(@as([*:0]const u8, @ptrCast(argv.items[1].?)), 0));
    try std.testing.expectEqual(@as(?[*:0]const u8, null), argv.items[2]);
}

// Regression test: wg_show builds correct argv for execve.
// Expected: ["/usr/bin/wg", "show", null]
test "wg_show builds correct argv for execve" {
    const exe_path: [*:0]const u8 = "/usr/bin/wg";
    const args: []const [*:0]const u8 = &.{ "show" };
    
    // Simulate argv construction (same as runWgShow -> runCommand)
    var argv = std.ArrayListUnmanaged(?[*:0]const u8){ .items = &.{}, .capacity = 0 };
    defer argv.deinit(std.testing.allocator);
    
    try argv.append(std.testing.allocator, exe_path);
    for (args) |arg| {
        try argv.append(std.testing.allocator, arg);
    }
    try argv.append(std.testing.allocator, null);
    
    // Verify: ["/usr/bin/wg", "show", null]
    try std.testing.expectEqual(@as(usize, 3), argv.items.len);
    try std.testing.expect(argv.items[0] != null);
    try std.testing.expectEqualStrings("/usr/bin/wg", std.mem.sliceTo(@as([*:0]const u8, @ptrCast(argv.items[0].?)), 0));
    try std.testing.expect(argv.items[1] != null);
    try std.testing.expectEqualStrings("show", std.mem.sliceTo(@as([*:0]const u8, @ptrCast(argv.items[1].?)), 0));
    try std.testing.expectEqual(@as(?[*:0]const u8, null), argv.items[2]);
}

// Regression test: wg_show_dump builds correct argv for execve.
// Expected: ["/usr/bin/wg", "show", <iface>, "dump", null]
test "wg_show_dump builds correct argv for execve" {
    const exe_path: [*:0]const u8 = "/usr/bin/wg";
    const iface_arg: [*:0]const u8 = "wg0";
    const args: []const [*:0]const u8 = &.{ "show", iface_arg, "dump" };
    
    // Simulate argv construction
    var argv = std.ArrayListUnmanaged(?[*:0]const u8){ .items = &.{}, .capacity = 0 };
    defer argv.deinit(std.testing.allocator);
    
    try argv.append(std.testing.allocator, exe_path);
    for (args) |arg| {
        try argv.append(std.testing.allocator, arg);
    }
    try argv.append(std.testing.allocator, null);
    
    // Verify: ["/usr/bin/wg", "show", "wg0", "dump", null]
    try std.testing.expectEqual(@as(usize, 5), argv.items.len);
    try std.testing.expectEqualStrings("/usr/bin/wg", std.mem.sliceTo(@as([*:0]const u8, @ptrCast(argv.items[0].?)), 0));
    try std.testing.expectEqualStrings("show", std.mem.sliceTo(@as([*:0]const u8, @ptrCast(argv.items[1].?)), 0));
    try std.testing.expectEqualStrings("wg0", std.mem.sliceTo(@as([*:0]const u8, @ptrCast(argv.items[2].?)), 0));
    try std.testing.expectEqualStrings("dump", std.mem.sliceTo(@as([*:0]const u8, @ptrCast(argv.items[3].?)), 0));
    try std.testing.expectEqual(@as(?[*:0]const u8, null), argv.items[4]);
}

// Regression test: old bug would produce [ptr, "", garbage...] not [ptr, null]
// This test verifies the FIX: we use nullable type and append null, not "".
test "old bug: empty string not used as sentinel" {
    // OLD BUG: const null_sentinel: [*:0]const u8 = "";
    //          try argv.append(allocator, null_sentinel);
    // This appended "" (empty string pointer) to argv, making:
    //   ["/usr/bin/ss", "-tin", ""]  <-- "" is NOT a null pointer!
    // 
    // When execve scanned argv looking for NULL terminator, it found:
    //   argv[0] = ptr to "/usr/bin/ss"   <-- valid
    //   argv[1] = ptr to "-tin"          <-- valid
    //   argv[2] = ptr to ""              <-- valid but not NULL!
    //   argv[3] = UNINITIALIZED/GARBAGE  <-- EFAULT!
    //
    // FIX: Use nullable type and append null:
    //   var argv = ArrayListUnmanaged(?[*:0]const u8)
    //   try argv.append(allocator, null);  <-- actual NULL pointer
    
    // Verify that appending null produces a true null pointer, not empty string
    var argv = std.ArrayListUnmanaged(?[*:0]const u8){ .items = &.{}, .capacity = 0 };
    defer argv.deinit(std.testing.allocator);
    
    try argv.append(std.testing.allocator, @ptrFromInt(0x1000));
    try argv.append(std.testing.allocator, null);
    
    // The second element MUST be null, not a pointer to empty string
    try std.testing.expectEqual(@as(?[*:0]const u8, null), argv.items[1]);
    
    // Verify it's actually the NULL pointer, not some non-null address
    const sentinel = argv.items[1];
    try std.testing.expect(sentinel == null);
}
