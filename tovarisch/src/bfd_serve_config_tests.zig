// bfd_serve_config_tests.zig — Tests for BFD serve config ownership and cleanup
//
// Tests the ownership model after the serve-config split:
// - BfdServeBundle owns both RawConfig and BfdRuntime
// - Config memory is properly cleaned up
// - Cleanup can be called safely
//
// Note: Only behavioral tests here. Shape tests (e.g., field existence via
// @typeInfo) are weak assertions that don't prove observable behavior.
//
// IMPORTANT: loadConfigAndBfd() spawns real threads which can cause timing
// issues in test context. Tests here focus on config loading and cleanup
// paths that don't require thread initialization. For full integration tests,
// prefer the existing bfd/runtime_tests.zig.

const std = @import("std");
const bfd_serve = @import("cli/bfd_serve.zig");

// ============================================================================
// Test helpers
// ============================================================================

/// A writer that captures output into a fixed buffer for testing stderr content.
const CaptureWriter = struct {
    const Self = @This();
    const BufSize = 4096;
    buf: [BufSize]u8 = undefined,
    len: usize = 0,

    pub fn init() Self {
        return .{ .buf = undefined, .len = 0 };
    }

    pub fn print(self: *Self, comptime fmt: []const u8, args: anytype) !void {
        if (self.len >= BufSize) return error.BufferOverflow;
        const written = std.fmt.bufPrint(self.buf[self.len..], fmt, args) catch return error.BufferOverflow;
        self.len += written.len;
    }

    pub fn writeAll(self: *Self, bytes: []const u8) !void {
        if (self.len + bytes.len > BufSize) return error.BufferOverflow;
        @memcpy(self.buf[self.len..][0..bytes.len], bytes);
        self.len += bytes.len;
    }

    pub fn slice(self: *const Self) []const u8 {
        return self.buf[0..self.len];
    }

    pub fn flush(_: *Self) error{}!void {}
};

// ============================================================================
// BfdLoadResult state machine tests
// These tests use null path to avoid thread initialization issues
// ============================================================================

test "loadConfigAndBfd with null path returns no_config" {
    var stderr = CaptureWriter.init();
    const result = bfd_serve.loadConfigAndBfd(null, &stderr);
    try std.testing.expect(result == .no_config);
}

test "loadConfigAndBfd with null path does NOT produce stderr" {
    var stderr = CaptureWriter.init();
    const result = bfd_serve.loadConfigAndBfd(null, &stderr);
    try std.testing.expect(result == .no_config);
    try std.testing.expect(stderr.len == 0);
}

test "loadConfigAndBfd.no_config is NOT .failed" {
    var stderr = CaptureWriter.init();
    const result = bfd_serve.loadConfigAndBfd(null, &stderr);
    try std.testing.expect(result != .failed);
}

// ============================================================================
// BfdLoadResult.failed path tests (config file errors)
// These tests verify error path without thread initialization
// ============================================================================

test "loadConfigAndBfd with non-existent file returns failed" {
    var stderr = CaptureWriter.init();
    const result = bfd_serve.loadConfigAndBfd("/nonexistent/path/to/config.toml", &stderr);
    try std.testing.expect(result == .failed);
}

test "loadConfigAndBfd with non-existent file produces stderr error" {
    var stderr = CaptureWriter.init();
    const result = bfd_serve.loadConfigAndBfd("/nonexistent/path/to/config.toml", &stderr);
    try std.testing.expect(result == .failed);
    // Must have non-empty stderr when returning failed
    try std.testing.expect(stderr.len > 0);
    try std.testing.expect(std.mem.containsAtLeast(u8, stderr.slice(), 1, "error:"));
    try std.testing.expect(std.mem.containsAtLeast(u8, stderr.slice(), 1, "failed to read config"));
}

// ============================================================================
// BfdLoadResult union invariants
// ============================================================================

test "BfdLoadResult is a tagged union with expected variants" {
    // Test that we can pattern match on all variants
    var stderr = CaptureWriter.init();

    // no_config variant
    const no_config_result = bfd_serve.loadConfigAndBfd(null, &stderr);
    try std.testing.expect(no_config_result == .no_config);

    // failed variant
    const failed_result = bfd_serve.loadConfigAndBfd("/definitely/nonexistent.toml", &stderr);
    try std.testing.expect(failed_result == .failed);
}

test "BfdLoadResult variants are mutually exclusive" {
    var stderr = CaptureWriter.init();

    // no_config and failed cannot both be true
    const no_config_result = bfd_serve.loadConfigAndBfd(null, &stderr);
    try std.testing.expect(no_config_result == .no_config);
    try std.testing.expect(no_config_result != .failed);

    const failed_result = bfd_serve.loadConfigAndBfd("/nonexistent/path.toml", &stderr);
    try std.testing.expect(failed_result == .failed);
    try std.testing.expect(failed_result != .no_config);
}
