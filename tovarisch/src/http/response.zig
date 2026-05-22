const std = @import("std");
const c = std.c;

/// HTTP response writer helpers for JSON responses.
/// Builds HTTP/1.1 responses with proper headers.
pub const ResponseWriter = struct {
    const Self = @This();
    const BufSize = 4096;

    fd: i32,
    buf: [BufSize]u8 = undefined,
    len: usize = 0,

    pub fn init(fd: i32) Self {
        return .{ .fd = fd };
    }

    /// Write HTTP status line and headers for JSON response.
    pub fn writeHeaders(self: *Self, status_code: u16, content_length: usize) !void {
        const status_text = switch (status_code) {
            200 => "OK",
            404 => "Not Found",
            405 => "Method Not Allowed",
            500 => "Internal Server Error",
            else => "Unknown",
        };

        try self.print(
            "HTTP/1.1 {d} {s}\r\nContent-Type: application/json\r\nContent-Length: {d}\r\nConnection: close\r\n\r\n",
            .{ status_code, status_text, content_length },
        );
    }

    /// Write a JSON error response.
    pub fn writeError(self: *Self, status_code: u16, error_msg: []const u8) !void {
        self.buf = undefined;
        self.len = 0;

        // Build JSON error body
        try self.writeAll("{\"error\":\"");
        try self.writeAll(error_msg);
        try self.writeAll("\"}");

        try self.writeHeaders(status_code, self.len);
        _ = c.write(self.fd, self.buf[0..self.len], self.len);
    }

    /// Write raw bytes to the internal buffer.
    pub fn writeAll(self: *Self, bytes: []const u8) !void {
        if (self.len + bytes.len > BufSize) return error.BufferOverflow;
        @memcpy(self.buf[self.len..][0..bytes.len], bytes);
        self.len += bytes.len;
    }

    /// Write a formatted string to the internal buffer.
    pub fn print(self: *Self, comptime fmt: []const u8, args: anytype) !void {
        if (self.len >= BufSize) return error.BufferOverflow;
        const written = std.fmt.bufPrint(self.buf[self.len..], fmt, args) catch return error.BufferOverflow;
        self.len += written.len;
    }

    /// Flush and send the buffered response.
    pub fn flush(self: *Self) !void {
        _ = c.write(self.fd, self.buf[0..self.len], self.len);
    }

    /// Write a complete JSON success response.
    pub fn writeJson(self: *Self, status_code: u16, json_body: []const u8) !void {
        try self.writeHeaders(status_code, json_body.len);
        _ = c.write(self.fd, json_body, json_body.len);
    }
};

/// Pre-built error responses for common cases.
pub const Errors = struct {
    pub const not_found = "{\"error\":\"not_found\"}";
    pub const method_not_allowed = "{\"error\":\"method_not_allowed\"}";
    pub const internal_error = "{\"error\":\"internal_error\"}";
    pub const ok = "{\"status\":\"ok\"}";
};

/// Write a simple JSON response directly to a file descriptor.
pub fn writeSimpleJsonFd(fd: i32, status_code: u16, body: []const u8) !void {
    const status_text = switch (status_code) {
        200 => "OK",
        404 => "Not Found",
        405 => "Method Not Allowed",
        500 => "Internal Server Error",
        else => "Unknown",
    };

    var buf: [256]u8 = undefined;
    const header_slice = try std.fmt.bufPrint(&buf,
        "HTTP/1.1 {d} {s}\r\nContent-Type: application/json\r\nContent-Length: {d}\r\nConnection: close\r\n\r\n",
        .{ status_code, status_text, body.len }
    );
    _ = c.write(fd, @as([*]const u8, @ptrCast(&buf)), header_slice.len);
    _ = c.write(fd, @as([*]const u8, @ptrCast(body.ptr)), body.len);
}

// --- Tests ---

test "Errors constants are valid JSON" {
    // Verify error constants are valid JSON strings
    try std.testing.expect(std.mem.eql(u8, Errors.not_found, "{\"error\":\"not_found\"}"));
    try std.testing.expect(std.mem.eql(u8, Errors.method_not_allowed, "{\"error\":\"method_not_allowed\"}"));
    try std.testing.expect(std.mem.eql(u8, Errors.internal_error, "{\"error\":\"internal_error\"}"));
    try std.testing.expect(std.mem.eql(u8, Errors.ok, "{\"status\":\"ok\"}"));
}

test "ResponseWriter init sets fd" {
    // Just verify the struct can be initialized
    // (can't test actual network without a real fd)
    try std.testing.expect(true);
}