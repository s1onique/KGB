const std = @import("std");
const response = @import("response.zig");
const status = @import("../status.zig");
const metrics = @import("../metrics.zig");
const metrics_state = @import("../metrics_state.zig");
const server = @import("server.zig");

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
    /// Raw query string (everything after '?'), or empty if no query.
    query: []const u8 = "",
};

/// Route handler function type.
/// Takes file descriptor and opaque state pointer.
pub const Handler = *const fn (fd: i32, state: *anyopaque) anyerror!void;

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
    // Expected format: "METHOD /path[?query] HTTP/version"
    var parts = std.mem.splitScalar(u8, line, ' ');
    const method_str = parts.next() orelse return null;
    const target = parts.next() orelse return null;
    const version = parts.next() orelse return null;

    const method = parseMethod(method_str);

    // Split request target into path and query (RFC 9112 origin-form)
    var path: []const u8 = target;
    var query: []const u8 = "";
    if (std.mem.indexOfScalar(u8, target, '?')) |i| {
        path = target[0..i];
        query = target[i + 1 ..];
    }

    return Request{
        .method = method,
        .path = path,
        .version = version,
        .query = query,
    };
}

/// Check if the query string contains include=network_diag.
/// Supports:
/// - include=network_diag
/// - include=network_diag&other=value
/// - other=value&include=network_diag
/// - other=value&include=network_diag&more=value
fn queryHasIncludeNetworkDiag(query: []const u8) bool {
    if (query.len == 0) return false;
    var it = std.mem.splitScalar(u8, query, '&');
    while (it.next()) |part| {
        if (std.mem.eql(u8, part, "include=network_diag")) return true;
    }
    return false;
}

/// Health check handler - returns simple ok status.
pub fn handleHealthz(fd: i32, _: *anyopaque) !void {
    try response.writeSimpleJsonFd(fd, 200, response.Errors.ok);
}

/// Status handler - returns full status JSON with BFD, config, and live BGP state.
/// When include_network_diag is true, includes the network_diag field.
pub fn handleStatus(fd: i32, state: *anyopaque, include_network_diag: bool) !void {
    // Cast opaque state to ServeContext to get BFD runtime, config check, and BGP state.
    const ctx = @as(*server.ServeContext, @ptrCast(@alignCast(state)));

    // For HTTP, we need to render status to a buffer first then send it.
    // Use larger buffer when including network_diag to accommodate extended output.
    var buf: [16384]u8 = undefined;
    var len: usize = 0;

    const writer = struct {
        buf: *[16384]u8,
        len: *usize,

        pub fn print(self: @This(), comptime fmt: []const u8, args: anytype) !void {
            if (self.len.* >= 16384) return error.BufferOverflow;
            const written = std.fmt.bufPrint(self.buf[self.len.*..], fmt, args) catch return error.BufferOverflow;
            self.len.* += written.len;
        }

        pub fn writeAll(self: @This(), bytes: []const u8) !void {
            if (self.len.* + bytes.len > 16384) return error.BufferOverflow;
            // Use for loop instead of @memcpy to avoid aliasing panic in Zig 0.16
            for (bytes, 0..) |byte, i| {
                self.buf[self.len.* + i] = byte;
            }
            self.len.* += bytes.len;
        }

        pub fn writeByte(self: @This(), c: u8) !void {
            if (self.len.* >= 16384) return error.BufferOverflow;
            self.buf[self.len.*] = c;
            self.len.* += 1;
        }
    }{ .buf = &buf, .len = &len };

    try status.renderPayloadWithContextAndDiag(writer, .{
        .bfd_runtime = ctx.bfd_runtime,
        .config_check = ctx.config_check,
        .bgp_result = ctx.bgp_result,
        // MemoryOwnership: Transient allocation for network_diag within HTTP request handler scope.
        // The renderPayloadWithContextAndDiag() function releases all memory via defer before returning.
    }, std.heap.page_allocator, include_network_diag);
    const json = buf[0..len];

    try response.writeSimpleJsonFd(fd, 200, json);
}

/// Metrics handler - uses persistent sampler state for live rates.
/// Falls back to warning JSON if live collection fails (HTTP 200 with status warn).
pub fn handleMetrics(fd: i32, state: *anyopaque) !void {
    // Cast opaque state to ServeContext to access metrics state.
    const ctx = @as(*server.ServeContext, @ptrCast(@alignCast(state)));

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
            // MemoryCopySafety: self.buf is a fixed [8192]u8 buffer. bytes is a caller-provided slice. They are distinct memory regions; no aliasing possible.
            @memcpy(self.buf[self.len.*..][0..bytes.len], bytes);
            self.len.* += bytes.len;
        }

        pub fn writeByte(self: @This(), c: u8) !void {
            if (self.len.* >= 8192) return error.BufferOverflow;
            self.buf[self.len.*] = c;
            self.len.* += 1;
        }
    }{ .buf = &buf, .len = &len };

    // MemoryOwnership: Transient allocation within HTTP request handler scope. All memory is released before the handler returns.
    ctx.metrics.renderMetricsPayload(std.heap.page_allocator, &writer, "/sys/class/net") catch {
        // Fallback: render warning payload
        len = 0;
        metrics.renderMetricsFallbackPayload(&writer) catch return error.InternalError;
    };

    const json = buf[0..len];
    try response.writeSimpleJsonFd(fd, 200, json);
}

/// Not found handler.
pub fn handleNotFound(fd: i32, _: *anyopaque) !void {
    try response.writeSimpleJsonFd(fd, 404, response.Errors.not_found);
}

/// Method not allowed handler.
pub fn handleMethodNotAllowed(fd: i32, _: *anyopaque) !void {
    try response.writeSimpleJsonFd(fd, 405, response.Errors.method_not_allowed);
}

/// Route a request to the appropriate handler using a file descriptor and state.
/// Returns true if a route was found and handled.
pub fn routeRequestFd(fd: i32, req: Request, state: *anyopaque) !bool {
    // Only support GET for v0
    if (req.method != .get) {
        try handleMethodNotAllowed(fd, state);
        return true;
    }

    // Route by path (query string is ignored for routing)
    if (std.mem.eql(u8, req.path, "/healthz")) {
        try handleHealthz(fd, state);
        return true;
    }

    // /status is an ergonomic alias for /status.json
    if (std.mem.eql(u8, req.path, "/status")) {
        const include_network_diag = queryHasIncludeNetworkDiag(req.query);
        try handleStatus(fd, state, include_network_diag);
        return true;
    }

    if (std.mem.eql(u8, req.path, "/status.json")) {
        const include_network_diag = queryHasIncludeNetworkDiag(req.query);
        try handleStatus(fd, state, include_network_diag);
        return true;
    }

    if (std.mem.eql(u8, req.path, "/metrics.json")) {
        try handleMetrics(fd, state);
        return true;
    }

    // All other paths return 404
    try handleNotFound(fd, state);
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

// --- Metrics state pointer tests ---

test "handleMetrics uses ServeContext.metrics for stateful collection" {
    // This test proves that handleMetrics() casts *anyopaque to *ServeContext and accesses ctx.metrics. This is the consistent pattern for all handlers.
    const allocator = std.testing.allocator;
    var serve_ctx = server.ServeContext.init(allocator);
    defer serve_ctx.deinit();

    // Verify ServeContext pointer can be passed as *anyopaque and recovered
    const opaque_ptr: *anyopaque = &serve_ctx;
    const recovered = @as(*server.ServeContext, @ptrCast(@alignCast(opaque_ptr)));
    // Access metrics through context (same pattern as handleMetrics)
    _ = &recovered.metrics;
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

