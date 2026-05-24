// status_tests.zig — Unit tests for status module
//
// Tests the state_dir check behavior using isolated filesystem paths.
// These tests are deterministic and do not depend on repository state.

const std = @import("std");
const status = @import("status.zig");

// --- State Directory Check Tests ---
// Deterministic tests using isolated test paths.

test "getStateDirCheckForPath returns warn for missing path" {
    // Use an isolated path that definitely does not exist
    const check = status.getStateDirCheckForPath("/tmp/tovarisch_test_nonexistent_12345");
    try std.testing.expectEqualStrings("state_dir", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("state directory not found", check.detail);
}

test "getStateDirCheckForPath returns ok for existing directory" {
    // Create a temporary directory for testing
    const test_dir = "/tmp/tovarisch_test_dir_12345";
    var path_buf: [4096]u8 = undefined;
    const c_path = status.toCString(test_dir, &path_buf) orelse return error.SkipZigTest;
    _ = std.c.mkdir(c_path, 0o755);

    defer _ = std.c.rmdir(c_path);

    const check = status.getStateDirCheckForPath(test_dir);
    try std.testing.expectEqualStrings("state_dir", check.name);
    try std.testing.expectEqual(status.CheckStatus.ok, check.status);
    try std.testing.expectEqualStrings("state directory ready", check.detail);
}

test "getStateDirCheckForPath returns warn for path that is a file" {
    // Create a temporary file for testing
    const test_file = "/tmp/tovarisch_test_file_12345";
    var path_buf: [4096]u8 = undefined;
    const c_path = status.toCString(test_file, &path_buf) orelse return error.SkipZigTest;
    const fd = std.c.open(c_path, std.c.O{ .ACCMODE = std.posix.ACCMODE.WRONLY, .CREAT = true }, @as(c_uint, 0o644));
    if (fd < 0) return error.SkipZigTest;
    defer _ = std.c.close(fd);

    defer _ = std.c.unlink(c_path);

    // When a file exists where a directory is expected, opendir fails with ENOTDIR.
    // Current behavior returns warn (honest limitation without stat()).
    const check = status.getStateDirCheckForPath(test_file);
    try std.testing.expectEqualStrings("state_dir", check.name);
    try std.testing.expect(check.status == .warn or check.status == .@"error");
    try std.testing.expect(check.status == .warn); // Current honest behavior
}

test "top-level status derives warn when state_dir is warn" {
    const checks = [_]status.Check{
        .{
            .name = "process",
            .status = .ok,
            .detail = "running",
        },
        .{
            .name = "binary",
            .status = .ok,
            .detail = "tovarisch",
        },
        .{
            .name = "config",
            .status = .warn,
            .detail = "not configured yet",
        },
        .{
            .name = "state_dir",
            .status = .warn,
            .detail = "state directory not found",
        },
        .{
            .name = "http",
            .status = .ok,
            .detail = "http service route available",
        },
    };
    try std.testing.expectEqual(status.CheckStatus.warn, status.deriveStatus(&checks));
}

test "top-level status derives error when state_dir is error" {
    const checks = [_]status.Check{
        .{
            .name = "process",
            .status = .ok,
            .detail = "running",
        },
        .{
            .name = "binary",
            .status = .ok,
            .detail = "tovarisch",
        },
        .{
            .name = "config",
            .status = .warn,
            .detail = "not configured yet",
        },
        .{
            .name = "state_dir",
            .status = .@"error",
            .detail = "state path is not a directory",
        },
        .{
            .name = "http",
            .status = .ok,
            .detail = "http service route available",
        },
    };
    // error takes priority over warn
    try std.testing.expectEqual(status.CheckStatus.@"error", status.deriveStatus(&checks));
}

test "DEFAULT_STATE_DIR constant is correct" {
    try std.testing.expectEqualStrings(".tovarisch/state", status.DEFAULT_STATE_DIR);
}
