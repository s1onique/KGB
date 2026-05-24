// metrics_fallback_dto_tests.zig — Tests for fallback payload rendering in metrics_dto
//
// ACT: Centralize metrics fallback JSON rendering in metrics_dto.
//
// Tests cover:
// 1. Fallback payload emits all required fields (service, version, metrics_version)
// 2. Fallback payload includes runtime telemetry (pid, rss_kib)
// 3. Fallback payload includes error status and detail
// 4. Fallback payload includes empty private_interfaces array
// 5. Fallback payload includes all notes
// 6. Fallback payload handles null rss_kib gracefully
//
// This file is imported by test_all.zig and refAllDecls forces test discovery.

const std = @import("std");
const metrics_dto = @import("metrics_dto.zig");
const telemetry = @import("runtime/telemetry.zig");

// Re-export for convenience
const renderFallbackPayload = metrics_dto.renderFallbackPayload;

// Test runtime telemetry - use known values for deterministic tests
const testRuntime = telemetry.RuntimeTelemetry{ .pid = 1234, .rss_kib = 1920 };

// ============================================================================
// Test Writer Helper
// ============================================================================

const TestWriter = struct {
    const Self = @This();
    const BufSize: usize = 8192;

    buf: [BufSize]u8 = undefined,
    len: usize = 0,

    pub fn init() Self {
        return .{ .buf = undefined, .len = 0 };
    }

    pub fn print(self: *Self, comptime fmt: []const u8, args: anytype) !void {
        if (self.len >= BufSize) return error.BufferOverflow;
        const remaining = self.buf[self.len..];
        const written = std.fmt.bufPrint(remaining, fmt, args) catch return error.BufferOverflow;
        self.len += written.len;
    }

    pub fn writeAll(self: *Self, bytes: []const u8) !void {
        if (self.len + bytes.len > BufSize) return error.BufferOverflow;
        @memcpy(self.buf[self.len..][0..bytes.len], bytes);
        self.len += bytes.len;
    }

    pub fn writeByte(self: *Self, c: u8) !void {
        if (self.len >= BufSize) return error.BufferOverflow;
        self.buf[self.len] = c;
        self.len += 1;
    }

    pub fn slice(self: *const Self) []const u8 {
        return self.buf[0..self.len];
    }
};

// ============================================================================
// Tests: Fallback Payload Rendering
// ============================================================================

test "renderFallbackPayload: emits service" {
    var w = TestWriter.init();
    try renderFallbackPayload(&w, testRuntime);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"service\":\"tovarisch\""));
}

test "renderFallbackPayload: emits version" {
    var w = TestWriter.init();
    try renderFallbackPayload(&w, testRuntime);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"version\":\"0.1.1\""));
}

test "renderFallbackPayload: emits metrics_version 0.3" {
    var w = TestWriter.init();
    try renderFallbackPayload(&w, testRuntime);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"metrics_version\":\"0.3\""));
}

test "renderFallbackPayload: emits status warn" {
    var w = TestWriter.init();
    try renderFallbackPayload(&w, testRuntime);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"status\":\"warn\""));
}

test "renderFallbackPayload: emits empty private_interfaces" {
    var w = TestWriter.init();
    try renderFallbackPayload(&w, testRuntime);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"private_interfaces\":[]"));
}

test "renderFallbackPayload: emits error metrics_unavailable" {
    var w = TestWriter.init();
    try renderFallbackPayload(&w, testRuntime);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"error\":\"metrics_unavailable\""));
}

test "renderFallbackPayload: emits detail private interface stats unavailable" {
    var w = TestWriter.init();
    try renderFallbackPayload(&w, testRuntime);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"detail\":\"private interface stats unavailable\""));
}

test "renderFallbackPayload: emits runtime block" {
    var w = TestWriter.init();
    try renderFallbackPayload(&w, testRuntime);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"runtime\":{"));
}

test "renderFallbackPayload: emits runtime pid" {
    var w = TestWriter.init();
    try renderFallbackPayload(&w, testRuntime);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"pid\":1234"));
}

test "renderFallbackPayload: emits runtime rss_kib" {
    var w = TestWriter.init();
    try renderFallbackPayload(&w, testRuntime);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"rss_kib\":1920"));
}

test "renderFallbackPayload: emits rate null note" {
    var w = TestWriter.init();
    try renderFallbackPayload(&w, testRuntime);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "rate is null until a previous sample exists"));
}

test "renderFallbackPayload: emits cumulative counter note" {
    var w = TestWriter.init();
    try renderFallbackPayload(&w, testRuntime);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "interface counters are cumulative"));
}

test "renderFallbackPayload: emits IPv4-only note" {
    var w = TestWriter.init();
    try renderFallbackPayload(&w, testRuntime);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "IPv4 private interfaces only"));
}

test "renderFallbackPayload: emits runtime RSS best-effort note" {
    var w = TestWriter.init();
    try renderFallbackPayload(&w, testRuntime);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "runtime RSS is best-effort platform telemetry"));
}

test "renderFallbackPayload: output is valid JSON structure" {
    var w = TestWriter.init();
    try renderFallbackPayload(&w, testRuntime);

    try std.testing.expect(std.mem.startsWith(u8, w.slice(), "{\"service\":"));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"status\":\"warn\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"error\":\"metrics_unavailable\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"private_interfaces\":[]"));
}

test "renderFallbackPayload: handles null rss_kib" {
    var w = TestWriter.init();
    const runtime_no_rss = telemetry.RuntimeTelemetry{ .pid = 5678, .rss_kib = null };
    try renderFallbackPayload(&w, runtime_no_rss);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"rss_kib\":null"));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"pid\":5678"));
}
