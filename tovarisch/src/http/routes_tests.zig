// routes_tests.zig — Tests for HTTP route handlers
//
// ACT 5h: Metrics handler integration tests extracted from routes.zig.
//
// Tests cover:
// - Metrics handler emits service field
// - Metrics handler emits metrics_version field
// - Metrics handler emits private_interfaces field
// - Metrics handler fallback emits status warn
// - Metrics handler fallback emits error metrics_unavailable
// - Metrics handler emits cumulative counter note
// - Metrics handler emits IPv4-only note

const std = @import("std");
const metrics = @import("../metrics.zig");

// ============================================================================
// Test Writer Helper
// ============================================================================

const TestWriter = struct {
    const Self = @This();
    const BufSize = 8192;

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

    pub fn slice(self: *const Self) []const u8 {
        return self.buf[0..self.len];
    }
};

// ============================================================================
// Metrics Handler Tests
// ============================================================================

test "metrics handler response contains service field" {
    var buf: [8192]u8 = undefined;
    var len: usize = 0;

    const writer = struct {
        buf: *[8192]u8,
        len: *usize,

        pub fn print(self: @This(), comptime fmt: []const u8, args: anytype) !void {
            if (self.len.* >= 8192) return error.BufferOverflow;
            const remaining = self.buf[self.len.*..];
            const written = std.fmt.bufPrint(remaining, fmt, args) catch return error.BufferOverflow;
            self.len.* += written.len;
        }

        pub fn writeAll(self: @This(), bytes: []const u8) !void {
            if (self.len.* + bytes.len > 8192) return error.BufferOverflow;
            @memcpy(self.buf[self.len.*..][0..bytes.len], bytes);
            self.len.* += bytes.len;
        }

        pub fn writeByte(self: @This(), c: u8) !void {
            if (self.len.* >= 8192) return error.BufferOverflow;
            self.buf[self.len.*] = c;
            self.len.* += 1;
        }
    }{ .buf = &buf, .len = &len };

    // Verify renderLiveMetricsPayload produces valid metrics JSON with service field.
    // This test does not start the blocking server.
    metrics.renderLiveMetricsPayload(std.heap.page_allocator, &writer) catch {
        // Fallback to verify fallback payload has service field
        len = 0;
        try metrics.renderMetricsFallbackPayload(&writer);
    };

    const json = buf[0..len];
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"service\":\"tovarisch\""));
}

test "metrics handler response contains metrics_version field" {
    var buf: [8192]u8 = undefined;
    var len: usize = 0;

    const writer = struct {
        buf: *[8192]u8,
        len: *usize,

        pub fn print(self: @This(), comptime fmt: []const u8, args: anytype) !void {
            if (self.len.* >= 8192) return error.BufferOverflow;
            const remaining = self.buf[self.len.*..];
            const written = std.fmt.bufPrint(remaining, fmt, args) catch return error.BufferOverflow;
            self.len.* += written.len;
        }

        pub fn writeAll(self: @This(), bytes: []const u8) !void {
            if (self.len.* + bytes.len > 8192) return error.BufferOverflow;
            @memcpy(self.buf[self.len.*..][0..bytes.len], bytes);
            self.len.* += bytes.len;
        }
    }{ .buf = &buf, .len = &len };

    // Test fallback payload
    try metrics.renderMetricsFallbackPayload(&writer);

    const json = buf[0..len];
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"metrics_version\":\"0.1\""));
}

test "metrics handler response contains private_interfaces field" {
    var buf: [8192]u8 = undefined;
    var len: usize = 0;

    const writer = struct {
        buf: *[8192]u8,
        len: *usize,

        pub fn print(self: @This(), comptime fmt: []const u8, args: anytype) !void {
            if (self.len.* >= 8192) return error.BufferOverflow;
            const remaining = self.buf[self.len.*..];
            const written = std.fmt.bufPrint(remaining, fmt, args) catch return error.BufferOverflow;
            self.len.* += written.len;
        }

        pub fn writeAll(self: @This(), bytes: []const u8) !void {
            if (self.len.* + bytes.len > 8192) return error.BufferOverflow;
            @memcpy(self.buf[self.len.*..][0..bytes.len], bytes);
            self.len.* += bytes.len;
        }
    }{ .buf = &buf, .len = &len };

    // Test fallback payload
    try metrics.renderMetricsFallbackPayload(&writer);

    const json = buf[0..len];
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"private_interfaces\""));
}

test "metrics handler fallback emits status warn" {
    var buf: [8192]u8 = undefined;
    var len: usize = 0;

    const writer = struct {
        buf: *[8192]u8,
        len: *usize,

        pub fn print(self: @This(), comptime fmt: []const u8, args: anytype) !void {
            if (self.len.* >= 8192) return error.BufferOverflow;
            const remaining = self.buf[self.len.*..];
            const written = std.fmt.bufPrint(remaining, fmt, args) catch return error.BufferOverflow;
            self.len.* += written.len;
        }

        pub fn writeAll(self: @This(), bytes: []const u8) !void {
            if (self.len.* + bytes.len > 8192) return error.BufferOverflow;
            @memcpy(self.buf[self.len.*..][0..bytes.len], bytes);
            self.len.* += bytes.len;
        }
    }{ .buf = &buf, .len = &len };

    try metrics.renderMetricsFallbackPayload(&writer);

    const json = buf[0..len];
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"status\":\"warn\""));
}

test "metrics handler fallback emits error metrics_unavailable" {
    var buf: [8192]u8 = undefined;
    var len: usize = 0;

    const writer = struct {
        buf: *[8192]u8,
        len: *usize,

        pub fn print(self: @This(), comptime fmt: []const u8, args: anytype) !void {
            if (self.len.* >= 8192) return error.BufferOverflow;
            const remaining = self.buf[self.len.*..];
            const written = std.fmt.bufPrint(remaining, fmt, args) catch return error.BufferOverflow;
            self.len.* += written.len;
        }

        pub fn writeAll(self: @This(), bytes: []const u8) !void {
            if (self.len.* + bytes.len > 8192) return error.BufferOverflow;
            @memcpy(self.buf[self.len.*..][0..bytes.len], bytes);
            self.len.* += bytes.len;
        }
    }{ .buf = &buf, .len = &len };

    try metrics.renderMetricsFallbackPayload(&writer);

    const json = buf[0..len];
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"error\":\"metrics_unavailable\""));
}

test "metrics handler emits cumulative counter note" {
    var buf: [8192]u8 = undefined;
    var len: usize = 0;

    const writer = struct {
        buf: *[8192]u8,
        len: *usize,

        pub fn print(self: @This(), comptime fmt: []const u8, args: anytype) !void {
            if (self.len.* >= 8192) return error.BufferOverflow;
            const remaining = self.buf[self.len.*..];
            const written = std.fmt.bufPrint(remaining, fmt, args) catch return error.BufferOverflow;
            self.len.* += written.len;
        }

        pub fn writeAll(self: @This(), bytes: []const u8) !void {
            if (self.len.* + bytes.len > 8192) return error.BufferOverflow;
            @memcpy(self.buf[self.len.*..][0..bytes.len], bytes);
            self.len.* += bytes.len;
        }
    }{ .buf = &buf, .len = &len };

    try metrics.renderMetricsFallbackPayload(&writer);

    const json = buf[0..len];
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "interface counters are cumulative"));
}

test "metrics handler emits IPv4-only note" {
    var buf: [8192]u8 = undefined;
    var len: usize = 0;

    const writer = struct {
        buf: *[8192]u8,
        len: *usize,

        pub fn print(self: @This(), comptime fmt: []const u8, args: anytype) !void {
            if (self.len.* >= 8192) return error.BufferOverflow;
            const remaining = self.buf[self.len.*..];
            const written = std.fmt.bufPrint(remaining, fmt, args) catch return error.BufferOverflow;
            self.len.* += written.len;
        }

        pub fn writeAll(self: @This(), bytes: []const u8) !void {
            if (self.len.* + bytes.len > 8192) return error.BufferOverflow;
            @memcpy(self.buf[self.len.*..][0..bytes.len], bytes);
            self.len.* += bytes.len;
        }
    }{ .buf = &buf, .len = &len };

    try metrics.renderMetricsFallbackPayload(&writer);

    const json = buf[0..len];
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "IPv4 private interfaces only"));
}
