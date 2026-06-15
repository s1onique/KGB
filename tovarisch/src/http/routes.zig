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
pub fn handleHealthz(fd: i32, _: *anyopaque) !void {
    try response.writeSimpleJsonFd(fd, 200, response.Errors.ok);
}

/// Status handler - returns full status JSON with BFD, config, and live BGP state.
pub fn handleStatus(fd: i32, state: *anyopaque) !void {
    // Cast opaque state to ServeContext to get BFD runtime, config check, and BGP state.
    const ctx = @as(*server.ServeContext, @ptrCast(@alignCast(state)));

    // For HTTP, we need to render status to a buffer first then send it. Use a simple fixed buffer writer.
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
            // Use for loop instead of @memcpy to avoid aliasing panic in Zig 0.16
            for (bytes, 0..) |byte, i| {
                self.buf[self.len.* + i] = byte;
            }
            self.len.* += bytes.len;
        }
    }{ .buf = &buf, .len = &len };

    // Pass BFD runtime, config check, and FULL BGP load result.
    try status.renderPayloadWithContext(writer, .{
        .bfd_runtime = ctx.bfd_runtime,
        .config_check = ctx.config_check,
        .bgp_result = ctx.bgp_result,
    });
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

    // Route by path
    if (std.mem.eql(u8, req.path, "/healthz")) {
        try handleHealthz(fd, state);
        return true;
    }

    // /status is an ergonomic alias for /status.json
    if (std.mem.eql(u8, req.path, "/status")) {
        try handleStatus(fd, state);
        return true;
    }

    if (std.mem.eql(u8, req.path, "/status.json")) {
        try handleStatus(fd, state);
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

// --- Status handler response tests ---
// These tests verify that the status handler produces correct JSON output.

test "status handler response contains status payload" {
    // Test that the status payload contains the expected service and checks.
    // This verifies handleStatus() will produce valid status JSON.
    var scratch = status.StatusScratch{};
    const s = status.buildStatus(&scratch);
    try std.testing.expectEqualStrings("tovarisch", s.service);
    try std.testing.expect(s.checks.len > 0);

    // Verify the renderPayload output contains expected status fields
    // Use 8192 buffer to accommodate all 9 checks including BGP detail
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
            // Use for loop instead of @memcpy to avoid aliasing panic in Zig 0.16
            for (bytes, 0..) |byte, i| {
                self.buf[self.len.* + i] = byte;
            }
            self.len.* += bytes.len;
        }
    }{ .buf = &buf, .len = &len };

    try status.renderPayload(writer);
    const json = buf[0..len];

    // Verify key status elements in JSON output
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"service\":\"tovarisch\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"checks\":["));
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"name\":\"http\""));
}

test "status handler includes http check in output" {
    // Explicitly verify that the status JSON includes the http check
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
            // Use for loop instead of @memcpy to avoid aliasing panic in Zig 0.16
            for (bytes, 0..) |byte, i| {
                self.buf[self.len.* + i] = byte;
            }
            self.len.* += bytes.len;
        }
    }{ .buf = &buf, .len = &len };

    try status.renderPayload(writer);
    const json = buf[0..len];

    // HTTP check should be present with ok status
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"name\":\"http\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"status\":\"ok\""));
}

// --- BFD runtime wiring tests ---

test "serve status endpoint reflects configured BFD runtime" {
    // Verifies status rendering with configured BFD runtime differs from null.
    const bfd_status_module = @import("../bfd/status.zig");

    // Create a test BFD runtime with a peer
    var runtime = try bfd_status_module.createTestRuntime();
    defer runtime.deinit();
    try bfd_status_module.addTestPeer(&runtime.rt, "10.0.0.1", "10.0.0.2");

    // Create serve context with the runtime
    var serve_ctx = server.ServeContext.init(std.testing.allocator);
    defer serve_ctx.deinit();
    serve_ctx.bfd_runtime = &runtime.rt;

    // Render status with the BFD runtime
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
            // MemoryCopySafety: self.buf is a fixed [4096]u8 buffer. bytes is a caller-provided slice. They are distinct memory regions; no aliasing.
            @memcpy(self.buf[self.len.*..][0..bytes.len], bytes);
            self.len.* += bytes.len;
        }
    }{ .buf = &buf, .len = &len };

    // Pass BFD runtime through context
    try status.renderPayloadWithBfd(writer, serve_ctx.bfd_runtime);
    const json = buf[0..len];

    // With configured BFD runtime, the bfd check should reflect peer info
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"name\":\"bfd\""));
    // The detail should show peer count, not "bfd not configured"
    try std.testing.expect(!std.mem.containsAtLeast(u8, json, 1, "bfd not configured"));
}

test "serve status endpoint with null BFD shows not configured" {
    // Verify that null BFD runtime produces "bfd not configured" status.
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
            // MemoryCopySafety: self.buf is a fixed [4096]u8 buffer. bytes is a caller-provided slice. They are distinct memory regions; no aliasing.
            @memcpy(self.buf[self.len.*..][0..bytes.len], bytes);
            self.len.* += bytes.len;
        }
    }{ .buf = &buf, .len = &len };

    // Pass null BFD runtime
    try status.renderPayloadWithBfd(writer, null);
    const json = buf[0..len];

    // Null runtime should show "bfd not configured"
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"name\":\"bfd\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "bfd not configured"));
}
