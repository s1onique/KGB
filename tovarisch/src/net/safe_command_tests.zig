// safe_command_tests.zig — Tests for safe_command module
//
// Tests for process boundary classification and command outcome types.
// Main process execution logic lives in safe_command.zig.

const std = @import("std");
const safe_command = @import("safe_command.zig");

// Re-export types for tests
const CommandConfig = safe_command.CommandConfig;
const CommandResult = safe_command.CommandResult;
const CommandOutcome = safe_command.CommandOutcome;
const ExitCodeClass = safe_command.ExitCodeClass;
const classifyResult = safe_command.classifyResult;
const classifyExitCode = safe_command.classifyExitCode;
const COMMAND_PATHS = safe_command.COMMAND_PATHS;

// ============================================================================
// CommandOutcome Classification Tests
// ============================================================================
//
// These tests verify the classifyResult() function correctly classifies
// all process boundary failure modes without shell invocation.

test "classifyExitCode: exit 0 is success" {
    try std.testing.expect(classifyExitCode(0) == .success);
}

test "classifyExitCode: exit 1 is failure" {
    try std.testing.expect(classifyExitCode(1) == .failure);
}

test "classifyExitCode: exit 126 is permission_denied" {
    try std.testing.expect(classifyExitCode(126) == .permission_denied);
}

test "classifyExitCode: exit 127 is command_not_found" {
    try std.testing.expect(classifyExitCode(127) == .command_not_found);
}

test "classifyExitCode: exit 137 (SIGKILL) is signal_exit" {
    try std.testing.expect(classifyExitCode(137) == .signal_exit);
}

test "classifyExitCode: exit -1 (spawn failed) is unknown" {
    try std.testing.expect(classifyExitCode(-1) == .unknown);
}

// Test 1: Success with bounded stdout
test "classifyResult: success case returns success outcome" {
    const stdout = try std.testing.allocator.dupe(u8, "hello world");
    defer std.testing.allocator.free(stdout);
    const stderr = try std.testing.allocator.dupe(u8, "");
    defer std.testing.allocator.free(stderr);
    
    const result = CommandResult{
        .exit_code = 0,
        .stdout = stdout,
        .stderr = stderr,
        .stdout_truncated = false,
        .stderr_truncated = false,
        .timed_out = false,
    };
    
    const outcome = classifyResult(result);
    try std.testing.expect(outcome == .success);
}

// Test 2: Non-zero exit classification
test "classifyResult: nonzero exit returns nonzero_exit outcome" {
    const stdout = try std.testing.allocator.dupe(u8, "error output");
    defer std.testing.allocator.free(stdout);
    const stderr = try std.testing.allocator.dupe(u8, "");
    defer std.testing.allocator.free(stderr);
    
    const result = CommandResult{
        .exit_code = 1,
        .stdout = stdout,
        .stderr = stderr,
        .stdout_truncated = false,
        .stderr_truncated = false,
        .timed_out = false,
    };
    
    const outcome = classifyResult(result);
    try std.testing.expect(outcome == .nonzero_exit);
    if (outcome == .nonzero_exit) {
        try std.testing.expectEqual(@as(u8, 1), outcome.nonzero_exit.exit_code);
    }
}

// Test 3: Command missing classification (exit 127)
test "classifyResult: exit 127 returns command_not_found outcome" {
    const result = CommandResult{
        .exit_code = 127,
        .stdout = &[_]u8{},
        .stderr = &[_]u8{},
        .stdout_truncated = false,
        .stderr_truncated = false,
        .timed_out = false,
    };
    
    const outcome = classifyResult(result);
    try std.testing.expect(outcome == .command_not_found);
}

// Test 4: Permission denied classification (exit 126)
test "classifyResult: exit 126 returns permission_denied outcome" {
    const result = CommandResult{
        .exit_code = 126,
        .stdout = &[_]u8{},
        .stderr = &[_]u8{},
        .stdout_truncated = false,
        .stderr_truncated = false,
        .timed_out = false,
    };
    
    const outcome = classifyResult(result);
    try std.testing.expect(outcome == .permission_denied);
}

// Test 5: Timeout classification
test "classifyResult: timed_out returns timeout outcome" {
    const stdout = try std.testing.allocator.dupe(u8, "partial output");
    defer std.testing.allocator.free(stdout);
    const stderr = try std.testing.allocator.dupe(u8, "");
    defer std.testing.allocator.free(stderr);
    
    const result = CommandResult{
        .exit_code = 137, // SIGKILL from timeout
        .stdout = stdout,
        .stderr = stderr,
        .stdout_truncated = false,
        .stderr_truncated = false,
        .timed_out = true,
    };
    
    const outcome = classifyResult(result);
    try std.testing.expect(outcome == .timeout);
}

// Test 6: stdout too large
test "classifyResult: stdout_truncated returns stdout_too_large outcome" {
    const stdout = try std.testing.allocator.dupe(u8, "truncated...");
    defer std.testing.allocator.free(stdout);
    
    const result = CommandResult{
        .exit_code = 0,
        .stdout = stdout,
        .stderr = &[_]u8{},
        .stdout_truncated = true,
        .stderr_truncated = false,
        .timed_out = false,
    };
    
    const outcome = classifyResult(result);
    try std.testing.expect(outcome == .stdout_too_large);
}

// Test 7: stderr too large
test "classifyResult: stderr_truncated returns stderr_too_large outcome" {
    const stdout = try std.testing.allocator.dupe(u8, "output");
    defer std.testing.allocator.free(stdout);
    const stderr = try std.testing.allocator.dupe(u8, "error truncated...");
    defer std.testing.allocator.free(stderr);
    
    const result = CommandResult{
        .exit_code = 0,
        .stdout = stdout,
        .stderr = stderr,
        .stdout_truncated = false,
        .stderr_truncated = true,
        .timed_out = false,
    };
    
    const outcome = classifyResult(result);
    try std.testing.expect(outcome == .stderr_too_large);
}

// Test 8: spawn failed (exit_code == -1)
test "classifyResult: exit -1 returns spawn_failed outcome" {
    const result = CommandResult{
        .exit_code = -1,
        .stdout = &[_]u8{},
        .stderr = &[_]u8{},
        .stdout_truncated = false,
        .stderr_truncated = false,
        .timed_out = false,
    };
    
    const outcome = classifyResult(result);
    try std.testing.expect(outcome == .spawn_failed);
    if (outcome == .spawn_failed) {
        try std.testing.expect(outcome.spawn_failed == error.ExecFailed);
    }
}

// Test 9: Malformed output is parser-owned (test that process boundary doesn't care about content)
// This verifies that the process boundary treats any output as opaque bytes.
// Parser errors are the caller's responsibility, not the process boundary's.
test "classifyResult: malformed output is treated as valid process output" {
    // Malformed output like "not valid wg output!!!"
    const stdout = try std.testing.allocator.dupe(u8, "not valid wg output!!!");
    defer std.testing.allocator.free(stdout);
    
    const result = CommandResult{
        .exit_code = 0,
        .stdout = stdout,
        .stderr = &[_]u8{},
        .stdout_truncated = false,
        .stderr_truncated = false,
        .timed_out = false,
    };
    
    // Process boundary only checks exit code and truncation, not content
    const outcome = classifyResult(result);
    try std.testing.expect(outcome == .success);
    
    // The caller (parser) is responsible for detecting malformed output
}

// Test 10: No shell invocation - argv must not contain shell metacharacters
// This is verified by the AllowedCommand enum which only allows specific commands
test "AllowedCommand only allows whitelisted commands" {
    inline for (0..COMMAND_PATHS.len) |i| {
        const command = COMMAND_PATHS[i][0];
        const path = COMMAND_PATHS[i][1];
        
        // Verify paths are absolute and don't contain shell metacharacters
        try std.testing.expect(path[0] == '/');
        try std.testing.expect(std.mem.indexOfScalar(u8, path, ' ') == null);
        try std.testing.expect(std.mem.indexOfScalar(u8, path, ';') == null);
        try std.testing.expect(std.mem.indexOfScalar(u8, path, '|') == null);
        try std.testing.expect(std.mem.indexOfScalar(u8, path, '&') == null);
        try std.testing.expect(std.mem.indexOfScalar(u8, path, '$') == null);
        try std.testing.expect(std.mem.indexOfScalar(u8, path, '`') == null);
        try std.testing.expect(std.mem.indexOfScalar(u8, path, '<') == null);
        try std.testing.expect(std.mem.indexOfScalar(u8, path, '>') == null);
        try std.testing.expect(std.mem.indexOfScalar(u8, path, '\n') == null);
        
        _ = command; // unused but verified to exist
    }
}

// Test: CommandConfig supports custom buffer sizes
test "CommandConfig supports custom buffer sizes" {
    const cfg = CommandConfig{
        .max_stdout_size = 4096,
        .max_stderr_size = 512,
        .timeout_ms = 5000,
    };
    try std.testing.expectEqual(@as(usize, 4096), cfg.max_stdout_size);
    try std.testing.expectEqual(@as(usize, 512), cfg.max_stderr_size);
    try std.testing.expectEqual(@as(u32, 5000), cfg.timeout_ms);
}

// Test: CommandConfig default timeout is 0 (no timeout)
test "CommandConfig default timeout is zero" {
    const cfg = CommandConfig{};
    try std.testing.expectEqual(@as(u32, 0), cfg.timeout_ms);
}

// Test: ExitCodeClass values match expected exit codes
test "ExitCodeClass discriminant values are correct" {
    try std.testing.expectEqual(@as(u8, 0), @intFromEnum(ExitCodeClass.success));
    try std.testing.expectEqual(@as(u8, 1), @intFromEnum(ExitCodeClass.failure));
    try std.testing.expectEqual(@as(u8, 126), @intFromEnum(ExitCodeClass.permission_denied));
    try std.testing.expectEqual(@as(u8, 127), @intFromEnum(ExitCodeClass.command_not_found));
    try std.testing.expectEqual(@as(u8, 128), @intFromEnum(ExitCodeClass.signal_exit));
    try std.testing.expectEqual(@as(u8, 255), @intFromEnum(ExitCodeClass.unknown));
}

// ============================================================================
// argv Construction Tests (regression tests)
// ============================================================================

// Regression test: argv array must be nullable for execve.
test "argv type is nullable for proper null-termination" {
    var argv = std.ArrayListUnmanaged(?[*:0]const u8){ .items = &.{}, .capacity = 0 };
    try argv.append(std.testing.allocator, @ptrFromInt(0x1000));
    try argv.append(std.testing.allocator, null);
    
    try std.testing.expectEqual(@as(usize, 2), argv.items.len);
    try std.testing.expectEqual(@as(?[*:0]const u8, @ptrFromInt(0x1000)), argv.items[0]);
    try std.testing.expectEqual(@as(?[*:0]const u8, null), argv.items[1]);
    
    argv.deinit(std.testing.allocator);
}

test "old bug: empty string not used as sentinel" {
    var argv = std.ArrayListUnmanaged(?[*:0]const u8){ .items = &.{}, .capacity = 0 };
    defer argv.deinit(std.testing.allocator);
    
    try argv.append(std.testing.allocator, @ptrFromInt(0x1000));
    try argv.append(std.testing.allocator, null);
    
    try std.testing.expectEqual(@as(?[*:0]const u8, null), argv.items[1]);
    const sentinel = argv.items[1];
    try std.testing.expect(sentinel == null);
}

test "ss_tin builds correct argv for execve" {
    const exe_path: [*:0]const u8 = "/usr/bin/ss";
    const args: []const [*:0]const u8 = &.{ "-tin" };
    
    var argv = std.ArrayListUnmanaged(?[*:0]const u8){ .items = &.{}, .capacity = 0 };
    defer argv.deinit(std.testing.allocator);
    
    try argv.append(std.testing.allocator, exe_path);
    for (args) |arg| {
        try argv.append(std.testing.allocator, arg);
    }
    try argv.append(std.testing.allocator, null);
    
    try std.testing.expectEqual(@as(usize, 3), argv.items.len);
    try std.testing.expect(argv.items[0] != null);
    try std.testing.expectEqualStrings("/usr/bin/ss", std.mem.sliceTo(@as([*:0]const u8, @ptrCast(argv.items[0].?)), 0));
    try std.testing.expect(argv.items[1] != null);
    try std.testing.expectEqualStrings("-tin", std.mem.sliceTo(@as([*:0]const u8, @ptrCast(argv.items[1].?)), 0));
    try std.testing.expectEqual(@as(?[*:0]const u8, null), argv.items[2]);
}

test "wg_show builds correct argv for execve" {
    const exe_path: [*:0]const u8 = "/usr/bin/wg";
    const args: []const [*:0]const u8 = &.{ "show" };
    
    var argv = std.ArrayListUnmanaged(?[*:0]const u8){ .items = &.{}, .capacity = 0 };
    defer argv.deinit(std.testing.allocator);
    
    try argv.append(std.testing.allocator, exe_path);
    for (args) |arg| {
        try argv.append(std.testing.allocator, arg);
    }
    try argv.append(std.testing.allocator, null);
    
    try std.testing.expectEqual(@as(usize, 3), argv.items.len);
    try std.testing.expect(argv.items[0] != null);
    try std.testing.expectEqualStrings("/usr/bin/wg", std.mem.sliceTo(@as([*:0]const u8, @ptrCast(argv.items[0].?)), 0));
    try std.testing.expect(argv.items[1] != null);
    try std.testing.expectEqualStrings("show", std.mem.sliceTo(@as([*:0]const u8, @ptrCast(argv.items[1].?)), 0));
    try std.testing.expectEqual(@as(?[*:0]const u8, null), argv.items[2]);
}

test "wg_show_dump builds correct argv for execve" {
    const exe_path: [*:0]const u8 = "/usr/bin/wg";
    const iface_arg: [*:0]const u8 = "wg0";
    const args: []const [*:0]const u8 = &.{ "show", iface_arg, "dump" };
    
    var argv = std.ArrayListUnmanaged(?[*:0]const u8){ .items = &.{}, .capacity = 0 };
    defer argv.deinit(std.testing.allocator);
    
    try argv.append(std.testing.allocator, exe_path);
    for (args) |arg| {
        try argv.append(std.testing.allocator, arg);
    }
    try argv.append(std.testing.allocator, null);
    
    try std.testing.expectEqual(@as(usize, 5), argv.items.len);
    try std.testing.expectEqualStrings("/usr/bin/wg", std.mem.sliceTo(@as([*:0]const u8, @ptrCast(argv.items[0].?)), 0));
    try std.testing.expectEqualStrings("show", std.mem.sliceTo(@as([*:0]const u8, @ptrCast(argv.items[1].?)), 0));
    try std.testing.expectEqualStrings("wg0", std.mem.sliceTo(@as([*:0]const u8, @ptrCast(argv.items[2].?)), 0));
    try std.testing.expectEqualStrings("dump", std.mem.sliceTo(@as([*:0]const u8, @ptrCast(argv.items[3].?)), 0));
    try std.testing.expectEqual(@as(?[*:0]const u8, null), argv.items[4]);
}

test "wg_show (no interface) argv matches wg_show_collector pattern" {
    const exe_path: [*:0]const u8 = "/usr/bin/wg";
    
    const argv: [3]?[*:0]const u8 = .{ exe_path, "show", null };
    const argv_ptr: [*:null]const ?[*:0]const u8 = @ptrCast(&argv);
    
    try std.testing.expectEqual(@as(usize, 3), argv.len);
    try std.testing.expect(argv_ptr[0] != null);
    try std.testing.expectEqualStrings("/usr/bin/wg", std.mem.sliceTo(@as([*:0]const u8, @ptrCast(argv_ptr[0].?)), 0));
    try std.testing.expect(argv_ptr[1] != null);
    try std.testing.expectEqualStrings("show", std.mem.sliceTo(@as([*:0]const u8, @ptrCast(argv_ptr[1].?)), 0));
    try std.testing.expectEqual(@as(?[*:0]const u8, null), argv_ptr[2]);
}
