/// Structured JSON logging for tovarisch.
///
/// All runtime logs flow through this module.
/// Do NOT emit hand-written JSON logs in serve/runtime paths.

const std = @import("std");
const status = @import("status.zig");

/// Stable event identifiers.
/// Each event has a defined log level to ensure consistency.
pub const Event = enum(u8) {
    /// HTTP server has bound and is listening.
    http_server_listening,
    /// Accept loop has started.
    http_accept_loop_started,
    /// Non-transient error in accept loop (logged, loop continues).
    http_accept_loop_error,
    /// Periodic tunnel status (placeholder until tunnel inventory exists).
    tunnel_stats,
    /// Application starting.
    app_startup,
    /// Application shutting down.
    app_shutdown,
    /// Server-level error from CLI (bind failure, etc).
    server_error,
    /// UVB-76 signal ready for monitoring.
    uvb76_signal_ready,
};

/// Get the log level string for a given event.
fn levelFor(event: Event) []const u8 {
    return switch (event) {
        .http_server_listening,
        .http_accept_loop_started,
        .tunnel_stats,
        .app_startup,
        .app_shutdown,
        .uvb76_signal_ready => "info",

        .http_accept_loop_error,
        .server_error => "error",
    };
}

/// Field value types supported in log records.
pub const FieldValue = union(enum) {
    string: []const u8,
    integer: i64,
    boolean: bool,
    null,
};

/// BufferedWriter: writes JSON to a buffer with overflow check.
pub const BufferedWriter = struct {
    const Self = @This();
    const BufSize = 1024;

    buf: [BufSize]u8 = undefined,
    len: usize = 0,

    pub fn init() Self {
        return .{ .buf = undefined, .len = 0 };
    }

    pub fn writeAll(self: *Self, bytes: []const u8) !void {
        if (self.len + bytes.len > BufSize) return error.BufferOverflow;
        @memcpy(self.buf[self.len..][0..bytes.len], bytes);
        self.len += bytes.len;
    }

    pub fn writeByte(self: *Self, byte: u8) !void {
        if (self.len >= BufSize) return error.BufferOverflow;
        self.buf[self.len] = byte;
        self.len += 1;
    }

    pub fn slice(self: *const Self) []const u8 {
        return self.buf[0..self.len];
    }

    pub fn reset(self: *Self) void {
        self.len = 0;
    }

    /// Write a formatted integer (hex)
    pub fn writeHex(self: *Self, value: u8) !void {
        const hex_chars = "0123456789ABCDEF";
        const high = hex_chars[value >> 4];
        const low = hex_chars[value & 0x0F];
        try self.writeByte(high);
        try self.writeByte(low);
    }
};

/// Escape a string for JSON output.
fn escapeJsonString(s: []const u8, buf: *BufferedWriter) !void {
    try buf.writeAll("\"");
    for (s) |ch| {
        switch (ch) {
            '"' => try buf.writeAll("\\\""),
            '\\' => try buf.writeAll("\\\\"),
            '\n' => try buf.writeAll("\\n"),
            '\r' => try buf.writeAll("\\r"),
            '\t' => try buf.writeAll("\\t"),
            else => if (ch < 0x20) {
                try buf.writeAll("\\u00");
                try buf.writeHex(ch);
            } else {
                try buf.writeByte(ch);
            },
        }
    }
    try buf.writeAll("\"");
}

/// Write a field value as JSON.
fn writeFieldValue(buf: *BufferedWriter, value: FieldValue) !void {
    switch (value) {
        .string => |s| try escapeJsonString(s, buf),
        .integer => |n| {
            var num_buf: [32]u8 = undefined;
            const slice = std.fmt.bufPrint(&num_buf, "{d}", .{n}) catch return error.BufferOverflow;
            try buf.writeAll(slice);
        },
        .boolean => |b| try buf.writeAll(if (b) "true" else "false"),
        .null => try buf.writeAll("null"),
    }
}

/// Emit a structured log record.
/// All fields are required: level, event, service, version, fields.
/// Record ends with newline for NDJSON.
pub fn emit(comptime event: Event, writer: *BufferedWriter, fields: anytype) !void {
    try writer.writeAll("{\"level\":\"");
    try writer.writeAll(levelFor(event));
    try writer.writeAll("\",\"event\":\"");
    try writer.writeAll(@tagName(event));
    try writer.writeAll("\",\"service\":\"tovarisch\",\"version\":\"");
    try writer.writeAll(status.version);
    try writer.writeAll("\",\"fields\":{");

    inline for (fields, 0..) |field, i| {
        if (i > 0) try writer.writeAll(",");
        try writer.writeAll("\"");
        try writer.writeAll(field.name);
        try writer.writeAll("\":");
        try writeFieldValue(writer, field.value);
    }

    try writer.writeAll("}}\n"); // Newline for NDJSON
}

/// Log tunnel stats with placeholder.
pub fn logTunnelStats(writer: *BufferedWriter) !void {
    try emit(.tunnel_stats, writer, &.{
        .{ .name = "tunnels_total", .value = FieldValue{ .integer = 0 } },
        .{ .name = "detail", .value = FieldValue{ .string = "tunnel inventory not implemented yet" } },
    });
}

/// Write tunnel stats with actual values.
pub fn logTunnelStatsFull(writer: *BufferedWriter, total: u32, up: u32, down: u32) !void {
    try emit(.tunnel_stats, writer, &.{
        .{ .name = "tunnels_total", .value = FieldValue{ .integer = total } },
        .{ .name = "tunnels_up", .value = FieldValue{ .integer = up } },
        .{ .name = "tunnels_down", .value = FieldValue{ .integer = down } },
    });
}

// --- Tests ---

test "BufferedWriter writes bytes correctly" {
    var w = BufferedWriter.init();
    try w.writeAll("hello");
    try std.testing.expectEqual(@as(usize, 5), w.len);
    try std.testing.expectEqualSlices(u8, "hello", w.slice());
}

test "BufferedWriter writeByte" {
    var w = BufferedWriter.init();
    try w.writeByte('a');
    try w.writeByte('b');
    try std.testing.expectEqual(@as(usize, 2), w.len);
}

test "BufferedWriter reset" {
    var w = BufferedWriter.init();
    try w.writeAll("hello");
    w.reset();
    try std.testing.expectEqual(@as(usize, 0), w.len);
}

test "BufferedWriter overflow returns error" {
    var w = BufferedWriter.init();
    var i: usize = 0;
    while (i < BufferedWriter.BufSize) : (i += 1) {
        w.buf[i] = 'x';
        w.len += 1;
    }
    try std.testing.expectError(error.BufferOverflow, w.writeAll("y"));
}

test "escapeJsonString handles empty string" {
    var w = BufferedWriter.init();
    try escapeJsonString("", &w);
    try std.testing.expectEqualSlices(u8, "\"\"", w.slice());
}

test "escapeJsonString handles normal string" {
    var w = BufferedWriter.init();
    try escapeJsonString("hello", &w);
    try std.testing.expectEqualSlices(u8, "\"hello\"", w.slice());
}

test "writeFieldValue handles string" {
    var w = BufferedWriter.init();
    try writeFieldValue(&w, FieldValue{ .string = "test" });
    try std.testing.expectEqualSlices(u8, "\"test\"", w.slice());
}

test "writeFieldValue handles integer" {
    var w = BufferedWriter.init();
    try writeFieldValue(&w, FieldValue{ .integer = 42 });
    try std.testing.expectEqualSlices(u8, "42", w.slice());
}

test "writeFieldValue handles boolean true" {
    var w = BufferedWriter.init();
    try writeFieldValue(&w, FieldValue{ .boolean = true });
    try std.testing.expectEqualSlices(u8, "true", w.slice());
}

test "writeFieldValue handles boolean false" {
    var w = BufferedWriter.init();
    try writeFieldValue(&w, FieldValue{ .boolean = false });
    try std.testing.expectEqualSlices(u8, "false", w.slice());
}

test "writeFieldValue handles null" {
    var w = BufferedWriter.init();
    try writeFieldValue(&w, FieldValue{ .null = {} });
    try std.testing.expectEqualSlices(u8, "null", w.slice());
}

test "emit produces valid JSON record" {
    var w = BufferedWriter.init();
    try emit(.http_server_listening, &w, &.{
        .{ .name = "port", .value = FieldValue{ .integer = 8317 } },
        .{ .name = "addr", .value = FieldValue{ .string = "127.0.0.1" } },
    });

    const output = w.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"event\":\"http_server_listening\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"service\":\"tovarisch\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"level\":\"info\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"port\":8317"));
    try std.testing.expect(output[output.len - 1] == '\n');
}

test "emit handles empty fields" {
    var w = BufferedWriter.init();
    try emit(.http_accept_loop_started, &w, &.{});

    const output = w.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"event\":\"http_accept_loop_started\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"fields\":{}"));
}

test "emit http_accept_loop_error has level error" {
    var w = BufferedWriter.init();
    try emit(.http_accept_loop_error, &w, &.{
        .{ .name = "error", .value = FieldValue{ .string = "AcceptFailed" } },
    });

    const output = w.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"event\":\"http_accept_loop_error\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"level\":\"error\""));
}

test "logTunnelStats with placeholder" {
    var w = BufferedWriter.init();
    try logTunnelStats(&w);

    const output = w.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"event\":\"tunnel_stats\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"tunnels_total\":0"));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "tunnel inventory not implemented yet"));
}

test "logTunnelStatsFull with actual values" {
    var w = BufferedWriter.init();
    try logTunnelStatsFull(&w, 5, 3, 2);

    const output = w.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"tunnels_total\":5"));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"tunnels_up\":3"));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"tunnels_down\":2"));
}

test "Event tag names are snake_case" {
    try std.testing.expectEqualStrings("http_server_listening", @tagName(.http_server_listening));
    try std.testing.expectEqualStrings("http_accept_loop_started", @tagName(.http_accept_loop_started));
    try std.testing.expectEqualStrings("http_accept_loop_error", @tagName(.http_accept_loop_error));
    try std.testing.expectEqualStrings("tunnel_stats", @tagName(.tunnel_stats));
    try std.testing.expectEqualStrings("app_startup", @tagName(.app_startup));
    try std.testing.expectEqualStrings("app_shutdown", @tagName(.app_shutdown));
    try std.testing.expectEqualStrings("server_error", @tagName(.server_error));
    try std.testing.expectEqualStrings("uvb76_signal_ready", @tagName(.uvb76_signal_ready));
}

test "levelFor returns info for info events" {
    try std.testing.expectEqualStrings("info", levelFor(.http_server_listening));
    try std.testing.expectEqualStrings("info", levelFor(.http_accept_loop_started));
    try std.testing.expectEqualStrings("info", levelFor(.tunnel_stats));
    try std.testing.expectEqualStrings("info", levelFor(.app_startup));
    try std.testing.expectEqualStrings("info", levelFor(.app_shutdown));
    try std.testing.expectEqualStrings("info", levelFor(.uvb76_signal_ready));
}

test "levelFor returns error for error events" {
    try std.testing.expectEqualStrings("error", levelFor(.http_accept_loop_error));
    try std.testing.expectEqualStrings("error", levelFor(.server_error));
}

test "emit record ends with newline for NDJSON" {
    var w = BufferedWriter.init();
    try emit(.http_server_listening, &w, &.{});

    const output = w.slice();
    try std.testing.expect(output[output.len - 1] == '\n');
}
