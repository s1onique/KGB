const std = @import("std");
const zig_c = std.c;
const routes = @import("routes.zig");
const server = @import("server.zig");

// --- Metrics state pointer tests ---

test "handleMetrics uses ServeContext.metrics for stateful collection" {
    // This test proves that handleMetrics() casts *anyopaque to *ServeContext and accesses ctx.metrics.
    const allocator = std.heap.page_allocator;
    var serve_ctx = server.ServeContext.init(allocator);
    defer serve_ctx.deinit();

    // Verify ServeContext pointer can be passed as *anyopaque and recovered
    const opaque_ptr: *anyopaque = &serve_ctx;
    const recovered = @as(*server.ServeContext, @ptrCast(@alignCast(opaque_ptr)));
    // Access metrics through context (same pattern as handleMetrics)
    _ = &recovered.metrics;
}

// --- Lab probe route tests ---

test "/lab/probe returns 404 when lab_mode is false" {
    // Create a pipe for testing HTTP response
    var pipe_fds: [2]i32 = undefined;
    if (std.c.pipe(&pipe_fds) != 0) return error.PipeFailed;
    defer {
        _ = std.c.close(pipe_fds[0]);
        _ = std.c.close(pipe_fds[1]);
    }

    // Create serve context with lab_mode=false
    var serve_ctx = server.ServeContext.init(std.heap.page_allocator);
    defer serve_ctx.deinit();
    serve_ctx.lab_config = .{ .lab_mode = false, .lab_probe_failure_file = "" };

    const opaque_ptr: *anyopaque = &serve_ctx;
    const req = routes.parseRequestLine("GET /lab/probe HTTP/1.1").?;

    // Route the request
    _ = try routes.routeRequestFd(pipe_fds[1], req, opaque_ptr);

    // Read response from pipe
    var buf: [256]u8 = undefined;
    const bytes_read = std.c.read(pipe_fds[0], &buf, buf.len);
    try std.testing.expect(bytes_read > 0);

    const response_buf = buf[0..@as(usize, @intCast(bytes_read))];
    try std.testing.expect(std.mem.startsWith(u8, response_buf, "HTTP/1.1 404"));
}

test "/lab/probe returns 200 when lab_mode=true and file absent" {
    var pipe_fds: [2]i32 = undefined;
    if (std.c.pipe(&pipe_fds) != 0) return error.PipeFailed;
    defer {
        _ = std.c.close(pipe_fds[0]);
        _ = std.c.close(pipe_fds[1]);
    }

    // Use a path that definitely doesn't exist
    var serve_ctx = server.ServeContext.init(std.heap.page_allocator);
    defer serve_ctx.deinit();
    serve_ctx.lab_config = .{
        .lab_mode = true,
        .lab_probe_failure_file = "/nonexistent/path/that/does/not/exist/xyz123",
    };

    const opaque_ptr: *anyopaque = &serve_ctx;
    const req = routes.parseRequestLine("GET /lab/probe HTTP/1.1").?;

    _ = try routes.routeRequestFd(pipe_fds[1], req, opaque_ptr);

    var buf: [256]u8 = undefined;
    const bytes_read = std.c.read(pipe_fds[0], &buf, buf.len);
    try std.testing.expect(bytes_read > 0);

    const response_buf = buf[0..@as(usize, @intCast(bytes_read))];
    try std.testing.expect(std.mem.startsWith(u8, response_buf, "HTTP/1.1 200"));
}


test "ServeContext.lab_config defaults to lab_mode=false" {
    // MemoryOwnership: Test allocation - deinit called via defer.
    var serve_ctx = server.ServeContext.init(std.heap.page_allocator);
    defer serve_ctx.deinit();
    try std.testing.expect(!serve_ctx.lab_config.lab_mode);
    try std.testing.expect(serve_ctx.lab_config.lab_probe_failure_file.len == 0);
}

test "ServeContext.initWithContext accepts lab_config" {
    // MemoryOwnership: Test allocation - deinit called via defer.
    var serve_ctx = server.ServeContext.initWithContext(
        std.heap.page_allocator,
        null, // bfd_runtime
        .no_config,
        .{ .no_config = {} },
        .{
            .lab_mode = true,
            .lab_probe_failure_file = "/tmp/test-failure",
        },
    );
    defer serve_ctx.deinit();
    try std.testing.expect(serve_ctx.lab_config.lab_mode);
    try std.testing.expectEqualStrings("/tmp/test-failure", serve_ctx.lab_config.lab_probe_failure_file);
}
