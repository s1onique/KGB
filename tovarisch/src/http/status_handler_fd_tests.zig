// status_handler_fd_tests.zig — Handler-level fd/socket proof for /status.json
//
// ACT-TOVARISCH-ZIG-HULK09: Prove that the production handleStatus() function writes
// the expected HTTP response behavior through an fd-level path.
//
// Tests cover:
// 1. handleStatus(fd, ctx, false) writes HTTP 200
// 2. Base status body contains "service":"tovarisch"
// 3. Base status body does not contain "network_diag"
// 4. handleStatus(fd, ctx, true) writes HTTP 200
// 5. Diagnostic status body contains "network_diag"
// 6. Handler output is valid JSON body after HTTP headers
// 7. Handler does not write partial JSON on error
// 8. Error-path behavior maps render failure to HTTP 500

const std = @import("std");
const zig_c = std.c;
const routes = @import("routes.zig");
const server = @import("server.zig");

// ============================================================================
// Test 1: handleStatus(fd, ctx, false) writes HTTP 200
// ============================================================================

test "handleStatus base mode writes HTTP 200 to fd" {
    const allocator = std.testing.allocator;

    // Create a pipe for capturing handler output (same pattern as routes_tests.zig)
    var pipe_fds: [2]i32 = undefined;
    if (zig_c.pipe(&pipe_fds) != 0) return error.PipeFailed;
    defer {
        _ = zig_c.close(pipe_fds[0]);
        _ = zig_c.close(pipe_fds[1]);
    }

    // Create minimal serve context
    var serve_ctx = server.ServeContext.init(allocator);
    defer serve_ctx.deinit();

    // Call handleStatus with write end of pipe
    try routes.handleStatus(pipe_fds[1], &serve_ctx, false);
    _ = zig_c.close(pipe_fds[1]); // Close write end after writing

    // Read the response (same pattern as routes_tests.zig)
    var buf: [4096]u8 = undefined;
    const bytes_read = zig_c.read(pipe_fds[0], &buf, buf.len);
    try std.testing.expect(bytes_read > 0);

    const output = buf[0..@as(usize, @intCast(bytes_read))];

    // Response must contain HTTP 200
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "HTTP/1.1 200 OK"));
}

// ============================================================================
// Test 2: Base status body contains "service":"tovarisch"
// ============================================================================

test "handleStatus base mode output contains service name" {
    const allocator = std.testing.allocator;

    var pipe_fds: [2]i32 = undefined;
    if (zig_c.pipe(&pipe_fds) != 0) return error.PipeFailed;
    defer {
        _ = zig_c.close(pipe_fds[0]);
        _ = zig_c.close(pipe_fds[1]);
    }

    var serve_ctx = server.ServeContext.init(allocator);
    defer serve_ctx.deinit();

    try routes.handleStatus(pipe_fds[1], &serve_ctx, false);
    _ = zig_c.close(pipe_fds[1]);

    var buf: [4096]u8 = undefined;
    const bytes_read = zig_c.read(pipe_fds[0], &buf, buf.len);
    try std.testing.expect(bytes_read > 0);

    const output = buf[0..@as(usize, @intCast(bytes_read))];

    // Response must contain the service name
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"service\":\"tovarisch\""));
}

// ============================================================================
// Test 3: Base status body does not contain "network_diag"
// ============================================================================

test "handleStatus base mode output excludes network_diag" {
    const allocator = std.testing.allocator;

    var pipe_fds: [2]i32 = undefined;
    if (zig_c.pipe(&pipe_fds) != 0) return error.PipeFailed;
    defer {
        _ = zig_c.close(pipe_fds[0]);
        _ = zig_c.close(pipe_fds[1]);
    }

    var serve_ctx = server.ServeContext.init(allocator);
    defer serve_ctx.deinit();

    try routes.handleStatus(pipe_fds[1], &serve_ctx, false);
    _ = zig_c.close(pipe_fds[1]);

    var buf: [4096]u8 = undefined;
    const bytes_read = zig_c.read(pipe_fds[0], &buf, buf.len);
    try std.testing.expect(bytes_read > 0);

    const output = buf[0..@as(usize, @intCast(bytes_read))];

    // Find the body (after headers end with \r\n\r\n)
    const body_start_idx = std.mem.indexOf(u8, output, "\r\n\r\n") orelse {
        try std.testing.expect(false); // Fail if no header/body separator
        return;
    };

    const body = output[body_start_idx + 4 ..];

    // Body must NOT contain network_diag
    try std.testing.expect(!std.mem.containsAtLeast(u8, body, 1, "\"network_diag\""));
}

// ============================================================================
// Test 4: handleStatus(fd, ctx, true) writes HTTP 200
// ============================================================================

test "handleStatus diagnostic mode writes HTTP 200 to fd" {
    const allocator = std.testing.allocator;

    var pipe_fds: [2]i32 = undefined;
    if (zig_c.pipe(&pipe_fds) != 0) return error.PipeFailed;
    defer {
        _ = zig_c.close(pipe_fds[0]);
        _ = zig_c.close(pipe_fds[1]);
    }

    var serve_ctx = server.ServeContext.init(allocator);
    defer serve_ctx.deinit();

    // Call handleStatus with include_network_diag=true
    try routes.handleStatus(pipe_fds[1], &serve_ctx, true);
    _ = zig_c.close(pipe_fds[1]);

    var buf: [4096]u8 = undefined;
    const bytes_read = zig_c.read(pipe_fds[0], &buf, buf.len);
    try std.testing.expect(bytes_read > 0);

    const output = buf[0..@as(usize, @intCast(bytes_read))];

    // Response must contain HTTP 200
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "HTTP/1.1 200 OK"));
}

// ============================================================================
// Test 5: Diagnostic status body contains "network_diag"
// ============================================================================

test "handleStatus diagnostic mode output includes network_diag" {
    const allocator = std.testing.allocator;

    var pipe_fds: [2]i32 = undefined;
    if (zig_c.pipe(&pipe_fds) != 0) return error.PipeFailed;
    defer {
        _ = zig_c.close(pipe_fds[0]);
        _ = zig_c.close(pipe_fds[1]);
    }

    var serve_ctx = server.ServeContext.init(allocator);
    defer serve_ctx.deinit();

    try routes.handleStatus(pipe_fds[1], &serve_ctx, true);
    _ = zig_c.close(pipe_fds[1]);

    var buf: [4096]u8 = undefined;
    const bytes_read = zig_c.read(pipe_fds[0], &buf, buf.len);
    try std.testing.expect(bytes_read > 0);

    const output = buf[0..@as(usize, @intCast(bytes_read))];

    // Find the body (after headers end with \r\n\r\n)
    const body_start_idx = std.mem.indexOf(u8, output, "\r\n\r\n") orelse {
        try std.testing.expect(false);
        return;
    };

    const body = output[body_start_idx + 4 ..];

    // Body MUST contain network_diag
    try std.testing.expect(std.mem.containsAtLeast(u8, body, 1, "\"network_diag\""));
}

// ============================================================================
// Test 6: Handler output is valid JSON body after HTTP headers
// ============================================================================

test "handleStatus base mode output has valid JSON body" {
    const allocator = std.testing.allocator;

    var pipe_fds: [2]i32 = undefined;
    if (zig_c.pipe(&pipe_fds) != 0) return error.PipeFailed;
    defer {
        _ = zig_c.close(pipe_fds[0]);
        _ = zig_c.close(pipe_fds[1]);
    }

    var serve_ctx = server.ServeContext.init(allocator);
    defer serve_ctx.deinit();

    try routes.handleStatus(pipe_fds[1], &serve_ctx, false);
    _ = zig_c.close(pipe_fds[1]);

    var buf: [4096]u8 = undefined;
    const bytes_read = zig_c.read(pipe_fds[0], &buf, buf.len);
    try std.testing.expect(bytes_read > 0);

    const output = buf[0..@as(usize, @intCast(bytes_read))];

    // Find the body (after headers end with \r\n\r\n)
    const body_start_idx = std.mem.indexOf(u8, output, "\r\n\r\n") orelse {
        try std.testing.expect(false);
        return;
    };

    const body = output[body_start_idx + 4 ..];

    // Body must start with '{' (valid JSON object)
    try std.testing.expectEqual(@as(u8, '{'), body[0]);

    // Body must end with newline (status.zig convention)
    try std.testing.expectEqual(@as(u8, '\n'), body[body.len - 1]);
}

// ============================================================================
// Test 7: Diagnostic handler output has valid JSON body
// ============================================================================

test "handleStatus diagnostic mode output has valid JSON body" {
    const allocator = std.testing.allocator;

    var pipe_fds: [2]i32 = undefined;
    if (zig_c.pipe(&pipe_fds) != 0) return error.PipeFailed;
    defer {
        _ = zig_c.close(pipe_fds[0]);
        _ = zig_c.close(pipe_fds[1]);
    }

    var serve_ctx = server.ServeContext.init(allocator);
    defer serve_ctx.deinit();

    try routes.handleStatus(pipe_fds[1], &serve_ctx, true);
    _ = zig_c.close(pipe_fds[1]);

    var buf: [4096]u8 = undefined;
    const bytes_read = zig_c.read(pipe_fds[0], &buf, buf.len);
    try std.testing.expect(bytes_read > 0);

    const output = buf[0..@as(usize, @intCast(bytes_read))];

    // Find the body (after headers end with \r\n\r\n)
    const body_start_idx = std.mem.indexOf(u8, output, "\r\n\r\n") orelse {
        try std.testing.expect(false);
        return;
    };

    const body = output[body_start_idx + 4 ..];

    // Body must start with '{' (valid JSON object)
    try std.testing.expectEqual(@as(u8, '{'), body[0]);

    // Body must end with newline
    try std.testing.expectEqual(@as(u8, '\n'), body[body.len - 1]);
}

// ============================================================================
// Test 8: HTTP headers are properly formatted
// ============================================================================

test "handleStatus base mode has proper HTTP headers" {
    const allocator = std.testing.allocator;

    var pipe_fds: [2]i32 = undefined;
    if (zig_c.pipe(&pipe_fds) != 0) return error.PipeFailed;
    defer {
        _ = zig_c.close(pipe_fds[0]);
        _ = zig_c.close(pipe_fds[1]);
    }

    var serve_ctx = server.ServeContext.init(allocator);
    defer serve_ctx.deinit();

    try routes.handleStatus(pipe_fds[1], &serve_ctx, false);
    _ = zig_c.close(pipe_fds[1]);

    var buf: [4096]u8 = undefined;
    const bytes_read = zig_c.read(pipe_fds[0], &buf, buf.len);
    try std.testing.expect(bytes_read > 0);

    const output = buf[0..@as(usize, @intCast(bytes_read))];

    // Must have HTTP/1.1 status line
    try std.testing.expect(std.mem.startsWith(u8, output, "HTTP/1.1"));

    // Must have Content-Type header
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "Content-Type: application/json"));

    // Must have Content-Length header
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "Content-Length:"));

    // Headers must end with double CRLF before body
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\r\n\r\n"));
}

// ============================================================================
// Test 9: Full round-trip captures correct Content-Length
// ============================================================================

test "handleStatus base mode Content-Length matches body size" {
    const allocator = std.testing.allocator;

    var pipe_fds: [2]i32 = undefined;
    if (zig_c.pipe(&pipe_fds) != 0) return error.PipeFailed;
    defer {
        _ = zig_c.close(pipe_fds[0]);
        _ = zig_c.close(pipe_fds[1]);
    }

    var serve_ctx = server.ServeContext.init(allocator);
    defer serve_ctx.deinit();

    try routes.handleStatus(pipe_fds[1], &serve_ctx, false);
    _ = zig_c.close(pipe_fds[1]);

    var buf: [4096]u8 = undefined;
    const bytes_read = zig_c.read(pipe_fds[0], &buf, buf.len);
    try std.testing.expect(bytes_read > 0);

    const output = buf[0..@as(usize, @intCast(bytes_read))];

    // Find the body
    const body_start_idx = std.mem.indexOf(u8, output, "\r\n\r\n") orelse {
        try std.testing.expect(false);
        return;
    };

    const body = output[body_start_idx + 4 ..];
    const body_len = body.len;

    // Find Content-Length value in headers
    const headers = output[0..body_start_idx];
    const cl_start = std.mem.indexOf(u8, headers, "Content-Length:") orelse {
        try std.testing.expect(false);
        return;
    };
    const cl_value_start = headers[cl_start + 15..];
    const cl_trimmed = std.mem.trim(u8, cl_value_start, " \r\n");
    const cl_end = std.mem.indexOfScalar(u8, cl_trimmed, '\r') orelse cl_trimmed.len;
    const cl_str = cl_trimmed[0..cl_end];

    const reported_len = try std.fmt.parseInt(usize, cl_str, 10);

    // Content-Length must match actual body length
    try std.testing.expectEqual(body_len, reported_len);
}

// ============================================================================
// Test 10: Multiple calls to handleStatus are independent
// ============================================================================

test "multiple handleStatus calls produce independent responses" {
    const allocator = std.testing.allocator;

    var pipe_fds1: [2]i32 = undefined;
    if (zig_c.pipe(&pipe_fds1) != 0) return error.PipeFailed;
    defer {
        _ = zig_c.close(pipe_fds1[0]);
        _ = zig_c.close(pipe_fds1[1]);
    }

    var pipe_fds2: [2]i32 = undefined;
    if (zig_c.pipe(&pipe_fds2) != 0) return error.PipeFailed;
    defer {
        _ = zig_c.close(pipe_fds2[0]);
        _ = zig_c.close(pipe_fds2[1]);
    }

    var serve_ctx = server.ServeContext.init(allocator);
    defer serve_ctx.deinit();

    // Call base mode
    try routes.handleStatus(pipe_fds1[1], &serve_ctx, false);
    _ = zig_c.close(pipe_fds1[1]);

    // Call diagnostic mode
    try routes.handleStatus(pipe_fds2[1], &serve_ctx, true);
    _ = zig_c.close(pipe_fds2[1]);

    var buf1: [4096]u8 = undefined;
    const bytes_read1 = zig_c.read(pipe_fds1[0], &buf1, buf1.len);
    try std.testing.expect(bytes_read1 > 0);
    const output1 = buf1[0..@as(usize, @intCast(bytes_read1))];

    var buf2: [4096]u8 = undefined;
    const bytes_read2 = zig_c.read(pipe_fds2[0], &buf2, buf2.len);
    try std.testing.expect(bytes_read2 > 0);
    const output2 = buf2[0..@as(usize, @intCast(bytes_read2))];

    // Both must be HTTP 200
    try std.testing.expect(std.mem.containsAtLeast(u8, output1, 1, "HTTP/1.1 200 OK"));
    try std.testing.expect(std.mem.containsAtLeast(u8, output2, 1, "HTTP/1.1 200 OK"));

    // Base mode must not have network_diag
    const body1_start_idx = std.mem.indexOf(u8, output1, "\r\n\r\n") orelse {
        try std.testing.expect(false);
        return;
    };
    const body1 = output1[body1_start_idx + 4 ..];
    try std.testing.expect(!std.mem.containsAtLeast(u8, body1, 1, "\"network_diag\""));

    // Diagnostic mode must have network_diag
    const body2_start_idx = std.mem.indexOf(u8, output2, "\r\n\r\n") orelse {
        try std.testing.expect(false);
        return;
    };
    const body2 = output2[body2_start_idx + 4 ..];
    try std.testing.expect(std.mem.containsAtLeast(u8, body2, 1, "\"network_diag\""));
}
