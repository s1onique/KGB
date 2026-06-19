// routes_status_tests.zig — Tests for HTTP status endpoint
//
// Tests cover:
// - Status handler response structure
// - BFD runtime wiring

const std = @import("std");
const status = @import("../status.zig");
const server = @import("server.zig");
const bfd_status = @import("../bfd/status.zig");

// --- Status handler response tests ---

test "status handler response contains status payload" {
    var scratch = status.StatusScratch{};
    const s = status.buildStatus(&scratch);
    try std.testing.expectEqualStrings("tovarisch", s.service);
    try std.testing.expect(s.checks.len > 0);

    var buf: [8192]u8 = undefined;
    var len: usize = 0;

    const writer = struct {
        buf: *[8192]u8,
        len: *usize,

        pub fn print(self: @This(), comptime fmt: []const u8, args: anytype) !void {
            if (self.len.* >= 8192) return error.BufferOverflow;
            const written = std.fmt.bufPrint(self.buf[self.len.*..], fmt, args) catch return error.BufferOverflow;
            self.len.* += written.len;
        }

        pub fn writeAll(self: @This(), bytes: []const u8) !void {
            if (self.len.* + bytes.len > 8192) return error.BufferOverflow;
            for (bytes, 0..) |byte, i| {
                self.buf[self.len.* + i] = byte;
            }
            self.len.* += bytes.len;
        }
    }{ .buf = &buf, .len = &len };

    try status.renderPayload(writer);
    const json = buf[0..len];

    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"service\":\"tovarisch\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"checks\":["));
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"name\":\"http\""));
}

test "status handler includes http check in output" {
    var buf: [8192]u8 = undefined;
    var len: usize = 0;

    const writer = struct {
        buf: *[8192]u8,
        len: *usize,

        pub fn print(self: @This(), comptime fmt: []const u8, args: anytype) !void {
            if (self.len.* >= 8192) return error.BufferOverflow;
            const written = std.fmt.bufPrint(self.buf[self.len.*..], fmt, args) catch return error.BufferOverflow;
            self.len.* += written.len;
        }

        pub fn writeAll(self: @This(), bytes: []const u8) !void {
            if (self.len.* + bytes.len > 8192) return error.BufferOverflow;
            for (bytes, 0..) |byte, i| {
                self.buf[self.len.* + i] = byte;
            }
            self.len.* += bytes.len;
        }
    }{ .buf = &buf, .len = &len };

    try status.renderPayload(writer);
    const json = buf[0..len];

    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"name\":\"http\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"status\":\"ok\""));
}

// --- BFD runtime wiring tests ---

test "serve status endpoint reflects configured BFD runtime" {
    var runtime = try bfd_status.createTestRuntime();
    defer runtime.deinit();
    try bfd_status.addTestPeer(&runtime.rt, "10.0.0.1", "10.0.0.2");

    var serve_ctx = server.ServeContext.init(std.testing.allocator);
    defer serve_ctx.deinit();
    serve_ctx.bfd_runtime = &runtime.rt;

    var buf: [4096]u8 = undefined;
    var len: usize = 0;

    const writer = struct {
        buf: *[4096]u8,
        len: *usize,

        pub fn print(self: @This(), comptime fmt: []const u8, args: anytype) !void {
            if (self.len.* >= 4096) return error.BufferOverflow;
            const written = std.fmt.bufPrint(self.buf[self.len.*..], fmt, args) catch return error.BufferOverflow;
            self.len.* += written.len;
        }

        pub fn writeAll(self: @This(), bytes: []const u8) !void {
            if (self.len.* + bytes.len > 4096) return error.BufferOverflow;
            @memcpy(self.buf[self.len.*..][0..bytes.len], bytes);
            self.len.* += bytes.len;
        }
    }{ .buf = &buf, .len = &len };

    try status.renderPayloadWithBfd(writer, serve_ctx.bfd_runtime);
    const json = buf[0..len];

    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"name\":\"bfd\""));
    try std.testing.expect(!std.mem.containsAtLeast(u8, json, 1, "bfd not configured"));
}

test "serve status endpoint with null BFD shows not configured" {
    var buf: [4096]u8 = undefined;
    var len: usize = 0;

    const writer = struct {
        buf: *[4096]u8,
        len: *usize,

        pub fn print(self: @This(), comptime fmt: []const u8, args: anytype) !void {
            if (self.len.* >= 4096) return error.BufferOverflow;
            const written = std.fmt.bufPrint(self.buf[self.len.*..], fmt, args) catch return error.BufferOverflow;
            self.len.* += written.len;
        }

        pub fn writeAll(self: @This(), bytes: []const u8) !void {
            if (self.len.* + bytes.len > 4096) return error.BufferOverflow;
            @memcpy(self.buf[self.len.*..][0..bytes.len], bytes);
            self.len.* += bytes.len;
        }
    }{ .buf = &buf, .len = &len };

    try status.renderPayloadWithBfd(writer, null);
    const json = buf[0..len];

    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"name\":\"bfd\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "bfd not configured"));
}
