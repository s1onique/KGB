// cli_serve_config_tests.zig — Tests for serve config loading and CLI error handling
//
// Tests the BFD config loader state machine and CLI error diagnostics:
// - loadConfigAndBfd() state mapping (no_config, disabled, configured, failed)
// - serveCommand error path diagnostics with non-empty stderr
// - Only actual load/parse/build failure maps to failed/serve_error

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
// BfdLoadResult state mapping tests
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
// Invariant: only actual load/parse/build failure maps to failed
// Disabled or absent BFD config is NOT an error
// ============================================================================

test "Invariant: no_config is not an error path" {
    var stderr = CaptureWriter.init();
    const result = bfd_serve.loadConfigAndBfd(null, &stderr);
    try std.testing.expect(result == .no_config);
    try std.testing.expect(stderr.len == 0);
}
