const std = @import("std");
const zig_c = std.c;
const response = @import("response.zig");
const status = @import("../status.zig");
const status_response = @import("../status_response.zig");
const metrics = @import("../metrics.zig");
const metrics_state = @import("../metrics_state.zig");
const server = @import("server.zig");
const status_route_contract = @import("status_route_contract.zig");

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

/// Check if the given path matches a route contract.
/// This is a narrow seam that consults the route contract table for path validation.
///
/// Returns true if the path matches a known route contract.
pub fn isStatusJsonRouteContract(path: []const u8) bool {
    return status_route_contract.lookupRoute(
        &status_route_contract.routes,
        path,
    ) != null;
}

/// Health check handler - returns simple ok status.
pub fn handleHealthz(fd: i32, _: *anyopaque) !void {
    try response.writeSimpleJsonFd(fd, 200, response.Errors.ok);
}

/// Status handler - returns full status JSON with BFD, config, and live BGP state.
/// When include_network_diag is true, includes the network_diag field.
///
/// This handler uses the route contract's response budget and request allocator
/// policy to bound memory usage. The request allocator capacity derives from
/// the response budget plus a named overhead for transient allocations.
/// On budget overflow or allocation failure, returns an internal error response.
pub fn handleStatus(fd: i32, state: *anyopaque, include_network_diag: bool) !void {
    // Cast opaque state to ServeContext to get BFD runtime, config check, and BGP state.
    const ctx = @as(*server.ServeContext, @ptrCast(@alignCast(state)));

    // Build status inputs (shared between branches)
    const inputs = status.RuntimeStatusInputs{
        .bfd_runtime = ctx.bfd_runtime,
        .config_check = ctx.config_check,
        .bgp_result = ctx.bgp_result,
        .network_diag_config = ctx.network_diag_config,
        .lab_config = ctx.lab_config,
        .lab_event_emitter = ctx.lab_event_emitter,
    };

    // Branch on diagnostic mode to select appropriate budget and allocator capacity.
    // This is necessary because Zig requires comptime-known array sizes.
    // The allocator capacity derives from the route contract's request allocator policy.
    // Use ResponseBudget.forQuery() to select the budget helper.
    if (include_network_diag) {
        // Diagnostic mode: use the route contract's budget helper
        const budget = status_route_contract.ResponseBudget.forQuery(true);
        var fixed_buf: [status_route_contract.requestAllocatorBytesForQuery(true)]u8 = undefined;
        var fba = std.heap.FixedBufferAllocator.init(&fixed_buf);
        const fba_allocator = fba.allocator();

        const query = @import("../status_query.zig").StatusQuery.parse("include=network_diag");
        const response_or_err = status_response.renderStatusOwnedWithBudget(
            fba_allocator,
            inputs,
            query,
            budget,
        );

        const owned_response = response_or_err catch {
            try response.writeSimpleJsonFd(fd, 500, response.Errors.internal_error);
            return;
        };
        defer owned_response.deinit(fba_allocator);
        try response.writeSimpleJsonFd(fd, 200, owned_response.body());
    } else {
        // Base mode: use the route contract's budget helper
        const budget = status_route_contract.ResponseBudget.forQuery(false);
        var fixed_buf: [status_route_contract.requestAllocatorBytesForQuery(false)]u8 = undefined;
        var fba = std.heap.FixedBufferAllocator.init(&fixed_buf);
        const fba_allocator = fba.allocator();

        const query = @import("../status_query.zig").StatusQuery.parse("");
        const response_or_err = status_response.renderStatusOwnedWithBudget(
            fba_allocator,
            inputs,
            query,
            budget,
        );

        const owned_response = response_or_err catch {
            try response.writeSimpleJsonFd(fd, 500, response.Errors.internal_error);
            return;
        };
        defer owned_response.deinit(fba_allocator);
        try response.writeSimpleJsonFd(fd, 200, owned_response.body());
    }
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

/// Lab probe handler - lab-only probe endpoint for KGB netns testing.
///
/// Behavior:
/// - When lab_mode is false: returns 404 (not a production control surface)
/// - When lab_mode is true and failure file does not exist: returns 200 (healthy)
/// - When lab_mode is true and failure file exists: returns 503 (failing)
///
/// The failure file path is configured via lab_probe_failure_file in tovarisch.conf.
pub fn handleLabProbe(fd: i32, state: *anyopaque) !void {
    const ctx = @as(*server.ServeContext, @ptrCast(@alignCast(state)));

    // If lab mode is not enabled, return 404 (not a production control surface)
    if (!ctx.lab_config.lab_mode) {
        try response.writeSimpleJsonFd(fd, 404, response.Errors.not_found);
        return;
    }

    // Check if the failure file exists
    const failure_file_path = ctx.lab_config.lab_probe_failure_file;
    if (failure_file_path.len == 0) {
        // No failure file configured, treat as healthy
        try response.writeSimpleJsonFd(fd, 200, response.Errors.ok);
        return;
    }

    // Use C-style access() to check file existence.
    // We need a null-terminated string for the C call.
    // MemoryOwnership: Transient allocation within HTTP request handler scope.
    // Memory is released via defer before handler returns.
    const null_terminated_path = try std.fmt.allocPrint(std.heap.page_allocator, "{s}\x00", .{failure_file_path});
    defer std.heap.page_allocator.free(null_terminated_path);
    if (zig_c.access(@ptrCast(null_terminated_path.ptr), 0) == 0) {
        // File exists - return 503 Service Unavailable
        try response.writeSimpleJsonFd(fd, 503, response.Errors.lab_probe_failing);
    } else {
        // File does not exist - return 200 OK
        try response.writeSimpleJsonFd(fd, 200, response.Errors.ok);
    }
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

    // /lab/probe - lab-only probe endpoint for KGB netns testing.
    // Returns 200 when healthy, 503 when failing (controlled by file existence).
    // Returns 404 when lab_mode is false (not a production control surface).
    if (std.mem.eql(u8, req.path, "/lab/probe")) {
        try handleLabProbe(fd, state);
        return true;
    }

    // All other paths return 404
    try handleNotFound(fd, state);
    return true;
}


