const std = @import("std");
const response = @import("response.zig");
const status = @import("../status.zig");

/// HTTP method types.
pub const Method = enum {
    get,
    post,
    put,
    delete,
    patch,
    head,
    options,
    unknown,
};

/// Parsed HTTP request.
pub const Request = struct {
    method: Method,
    path: []const u8,
    version: []const u8,
};

/// Route handler function type.
pub const Handler = *const fn (fd: i32) anyerror!void;

/// Route definition.
pub const Route = struct {
    path: []const u8,
    method: Method,
    handler: Handler,
};

/// Parse HTTP method string to Method enum.
/// Handles uppercase HTTP methods (GET, POST, etc.) and maps to lowercase enum tags.
fn parseMethod(method_str: []const u8) Method {
    if (std.mem.eql(u8, method_str, "GET")) return .get;
    if (std.mem.eql(u8, method_str, "POST")) return .post;
    if (std.mem.eql(u8, method_str, "PUT")) return .put;
    if (std.mem.eql(u8, method_str, "DELETE")) return .delete;
    if (std.mem.eql(u8, method_str, "PATCH")) return .patch;
    if (std.mem.eql(u8, method_str, "HEAD")) return .head;
    if (std.mem.eql(u8, method_str, "OPTIONS")) return .options;
    return .unknown;
}

/// Parse an HTTP request line.
/// Returns the parsed request or null if invalid.
pub fn parseRequestLine(line: []const u8) ?Request {
    // Expected format: "METHOD /path HTTP/version"
    var parts = std.mem.splitScalar(u8, line, ' ');
    const method_str = parts.next() orelse return null;
    const path = parts.next() orelse return null;
    const version = parts.next() orelse return null;

    const method = parseMethod(method_str);

    return Request{
        .method = method,
        .path = path,
        .version = version,
    };
}

/// Health check handler - returns simple ok status.
pub fn handleHealthz(fd: i32) !void {
    try response.writeSimpleJsonFd(fd, 200, response.Errors.ok);
}

/// Status handler - returns full status JSON.
pub fn handleStatus(fd: i32) !void {
    // For HTTP, we need to render status to a buffer first
    // then send it. Use a simple fixed buffer writer.
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

    try status.renderPayload(writer);
    const json = buf[0..len];

    try response.writeSimpleJsonFd(fd, 200, json);
}

/// Metrics handler placeholder - returns empty metrics for now.
pub fn handleMetrics(fd: i32) !void {
    // TODO: implement metrics collection
    // For now, return a minimal metrics response
    const metrics = "{\"service\":\"tovarisch\",\"version\":\"0.1.1\",\"node_id\":\"local-dev\",\"captured_at\":\"2026-05-22T21:00:00+00:00\",\"interfaces\":[],\"tunnels\":[]}";
    try response.writeSimpleJsonFd(fd, 200, metrics);
}

/// Not found handler.
pub fn handleNotFound(fd: i32) !void {
    try response.writeSimpleJsonFd(fd, 404, response.Errors.not_found);
}

/// Method not allowed handler.
pub fn handleMethodNotAllowed(fd: i32) !void {
    try response.writeSimpleJsonFd(fd, 405, response.Errors.method_not_allowed);
}

/// Route a request to the appropriate handler using a file descriptor.
/// Returns true if a route was found and handled.
pub fn routeRequestFd(fd: i32, req: Request) !bool {
    // Only support GET for v0
    if (req.method != .get) {
        try handleMethodNotAllowed(fd);
        return true;
    }

    // Route by path
    if (std.mem.eql(u8, req.path, "/healthz")) {
        try handleHealthz(fd);
        return true;
    }

    // /status is an ergonomic alias for /status.json
    if (std.mem.eql(u8, req.path, "/status")) {
        try handleStatus(fd);
        return true;
    }

    if (std.mem.eql(u8, req.path, "/status.json")) {
        try handleStatus(fd);
        return true;
    }

    if (std.mem.eql(u8, req.path, "/metrics.json")) {
        try handleMetrics(fd);
        return true;
    }

    // All other paths return 404
    try handleNotFound(fd);
    return true;
}

// --- Tests ---

test "parseRequestLine parses valid GET request" {
    const req = parseRequestLine("GET /healthz HTTP/1.1");
    try std.testing.expect(req != null);
    try std.testing.expect(req.?.method == .get);
    try std.testing.expect(std.mem.eql(u8, req.?.path, "/healthz"));
    try std.testing.expect(std.mem.eql(u8, req.?.version, "HTTP/1.1"));
}

test "parseRequestLine parses status.json request" {
    const req = parseRequestLine("GET /status.json HTTP/1.1");
    try std.testing.expect(req != null);
    try std.testing.expect(req.?.method == .get);
    try std.testing.expect(std.mem.eql(u8, req.?.path, "/status.json"));
}

test "parseRequestLine parses metrics.json request" {
    const req = parseRequestLine("GET /metrics.json HTTP/1.1");
    try std.testing.expect(req != null);
    try std.testing.expect(req.?.method == .get);
    try std.testing.expect(std.mem.eql(u8, req.?.path, "/metrics.json"));
}

test "parseRequestLine returns null for invalid line" {
    try std.testing.expect(parseRequestLine("") == null);
    try std.testing.expect(parseRequestLine("INVALID") == null);
    try std.testing.expect(parseRequestLine("GET") == null);
    try std.testing.expect(parseRequestLine("GET /") == null);
}

test "parseRequestLine handles unknown methods" {
    // "INVALIDMETHOD" is not a known HTTP method, so it should be .unknown
    const req = parseRequestLine("INVALIDMETHOD /test HTTP/1.1");
    try std.testing.expect(req != null);
    try std.testing.expect(req.?.method == .unknown);
}

test "parseRequestLine handles all HTTP methods" {
    try std.testing.expect(parseRequestLine("GET /test HTTP/1.1") != null);
    try std.testing.expect(parseRequestLine("POST /test HTTP/1.1") != null);
    try std.testing.expect(parseRequestLine("PUT /test HTTP/1.1") != null);
    try std.testing.expect(parseRequestLine("DELETE /test HTTP/1.1") != null);
    try std.testing.expect(parseRequestLine("PATCH /test HTTP/1.1") != null);
    try std.testing.expect(parseRequestLine("HEAD /test HTTP/1.1") != null);
    try std.testing.expect(parseRequestLine("OPTIONS /test HTTP/1.1") != null);
}

test "parseMethod maps uppercase HTTP methods to enum" {
    try std.testing.expect(parseMethod("GET") == .get);
    try std.testing.expect(parseMethod("POST") == .post);
    try std.testing.expect(parseMethod("PUT") == .put);
    try std.testing.expect(parseMethod("DELETE") == .delete);
    try std.testing.expect(parseMethod("PATCH") == .patch);
    try std.testing.expect(parseMethod("HEAD") == .head);
    try std.testing.expect(parseMethod("OPTIONS") == .options);
    try std.testing.expect(parseMethod("INVALID") == .unknown);
}

// --- Route path tests ---

test "parseRequestLine parses /status alias request" {
    const req = parseRequestLine("GET /status HTTP/1.1");
    try std.testing.expect(req != null);
    try std.testing.expect(req.?.method == .get);
    try std.testing.expect(std.mem.eql(u8, req.?.path, "/status"));
}

test "parseRequestLine parses /unknown path for 404" {
    const req = parseRequestLine("GET /unknown HTTP/1.1");
    try std.testing.expect(req != null);
    try std.testing.expect(req.?.method == .get);
    try std.testing.expect(std.mem.eql(u8, req.?.path, "/unknown"));
}
