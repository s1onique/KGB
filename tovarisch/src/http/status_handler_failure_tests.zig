// status_handler_failure_tests.zig — Forced render failure tests for /status.json
//
// ACT-TOVARISCH-ZIG-HULK11R: Prove that handleStatus() writes HTTP 500 on render failure.
//
// Tests use routes.handleStatusWithPolicy() directly to exercise THE SAME catch block
// that production handleStatus() uses. This is not a copy of the catch path - it IS
// the production catch path.
//
// Tests cover:
// 1. Forced base render failure writes HTTP 500
// 2. Forced diagnostic render failure writes HTTP 500
// 3. Forced failure body contains internal error JSON
// 4. Forced failure output does not contain "service":"tovarisch"
// 5. Forced failure output does not contain "network_diag"
// 6. Forced failure Content-Length matches error body length
// 7. Forced failure response is independent from a later success response

const std = @import("std");
const zig_c = std.c;
const routes = @import("routes.zig");
const server = @import("server.zig");
const response = @import("response.zig");
const status_route_contract = @import("status_route_contract.zig");

// ============================================================================
// Forced Failure Tests
// Force handleStatus() into its render-failure catch path via tiny budget.
// Tests call routes.handleStatusWithPolicy() directly - the SAME catch block
// that production handleStatus() uses.
// ============================================================================

/// Tiny budget that's guaranteed to cause BufferOverflow in renderStatusOwnedWithBudget().
/// The base status JSON is ~300-400 bytes, so 16 bytes is far too small.
const tiny_budget = status_route_contract.ResponseBudget{ .max_body_bytes = 16 };

/// Allocator buffer size for forced failure tests.
/// Must be large enough for the allocator to initialize but small enough that
/// renderStatusOwnedWithBudget() will fail with the tiny budget.
const test_allocator_bytes = 64;

// ============================================================================
// Test 1: Forced base render failure writes HTTP 500
// ============================================================================

test "handleStatus forced base render failure writes HTTP 500" {
    const allocator = std.testing.allocator;

    var pipe_fds: [2]i32 = undefined;
    if (zig_c.pipe(&pipe_fds) != 0) return error.PipeFailed;
    defer {
        _ = zig_c.close(pipe_fds[0]);
        _ = zig_c.close(pipe_fds[1]);
    }

    var serve_ctx = server.ServeContext.init(allocator);
    defer serve_ctx.deinit();

    // Call the SHARED helper with tiny budget - exercises THE SAME catch block as production
    try routes.handleStatusWithPolicy(
        pipe_fds[1],
        &serve_ctx,
        false,
        test_allocator_bytes,
        tiny_budget,
    );
    _ = zig_c.close(pipe_fds[1]);

    var buf: [4096]u8 = undefined;
    const bytes_read = zig_c.read(pipe_fds[0], &buf, buf.len);
    try std.testing.expect(bytes_read > 0);

    const output = buf[0..@as(usize, @intCast(bytes_read))];

    // Response MUST contain HTTP 500
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "HTTP/1.1 500"));
}

// ============================================================================
// Test 2: Forced diagnostic render failure writes HTTP 500
// ============================================================================

test "handleStatus forced diagnostic render failure writes HTTP 500" {
    const allocator = std.testing.allocator;

    var pipe_fds: [2]i32 = undefined;
    if (zig_c.pipe(&pipe_fds) != 0) return error.PipeFailed;
    defer {
        _ = zig_c.close(pipe_fds[0]);
        _ = zig_c.close(pipe_fds[1]);
    }

    var serve_ctx = server.ServeContext.init(allocator);
    defer serve_ctx.deinit();

    // Call the SHARED helper with tiny budget - exercises THE SAME catch block as production
    try routes.handleStatusWithPolicy(
        pipe_fds[1],
        &serve_ctx,
        true,
        test_allocator_bytes,
        tiny_budget,
    );
    _ = zig_c.close(pipe_fds[1]);

    var buf: [4096]u8 = undefined;
    const bytes_read = zig_c.read(pipe_fds[0], &buf, buf.len);
    try std.testing.expect(bytes_read > 0);

    const output = buf[0..@as(usize, @intCast(bytes_read))];

    // Response MUST contain HTTP 500
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "HTTP/1.1 500"));
}

// ============================================================================
// Test 3: Forced failure body contains internal error JSON
// ============================================================================

test "handleStatus forced failure body contains internal error JSON" {
    const allocator = std.testing.allocator;

    var pipe_fds: [2]i32 = undefined;
    if (zig_c.pipe(&pipe_fds) != 0) return error.PipeFailed;
    defer {
        _ = zig_c.close(pipe_fds[0]);
        _ = zig_c.close(pipe_fds[1]);
    }

    var serve_ctx = server.ServeContext.init(allocator);
    defer serve_ctx.deinit();

    try routes.handleStatusWithPolicy(
        pipe_fds[1],
        &serve_ctx,
        false,
        test_allocator_bytes,
        tiny_budget,
    );
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

    // Body MUST contain the internal_error JSON from response.Errors.internal_error
    try std.testing.expect(std.mem.containsAtLeast(u8, body, 1, "internal_error"));
}

// ============================================================================
// Test 4: Forced failure output does not contain "service":"tovarisch"
// ============================================================================

test "handleStatus forced failure output does not contain service name" {
    const allocator = std.testing.allocator;

    var pipe_fds: [2]i32 = undefined;
    if (zig_c.pipe(&pipe_fds) != 0) return error.PipeFailed;
    defer {
        _ = zig_c.close(pipe_fds[0]);
        _ = zig_c.close(pipe_fds[1]);
    }

    var serve_ctx = server.ServeContext.init(allocator);
    defer serve_ctx.deinit();

    try routes.handleStatusWithPolicy(
        pipe_fds[1],
        &serve_ctx,
        false,
        test_allocator_bytes,
        tiny_budget,
    );
    _ = zig_c.close(pipe_fds[1]);

    var buf: [4096]u8 = undefined;
    const bytes_read = zig_c.read(pipe_fds[0], &buf, buf.len);
    try std.testing.expect(bytes_read > 0);

    const output = buf[0..@as(usize, @intCast(bytes_read))];

    // Response MUST NOT contain success status JSON fields
    try std.testing.expect(!std.mem.containsAtLeast(u8, output, 1, "\"service\":\"tovarisch\""));
    try std.testing.expect(!std.mem.containsAtLeast(u8, output, 1, "\"status\":\"ok\""));
}

// ============================================================================
// Test 5: Forced failure output does not contain "network_diag"
// ============================================================================

test "handleStatus forced failure output does not contain network_diag" {
    const allocator = std.testing.allocator;

    var pipe_fds: [2]i32 = undefined;
    if (zig_c.pipe(&pipe_fds) != 0) return error.PipeFailed;
    defer {
        _ = zig_c.close(pipe_fds[0]);
        _ = zig_c.close(pipe_fds[1]);
    }

    var serve_ctx = server.ServeContext.init(allocator);
    defer serve_ctx.deinit();

    // Diagnostic mode - even with network_diag requested, failure should not write it
    try routes.handleStatusWithPolicy(
        pipe_fds[1],
        &serve_ctx,
        true,
        test_allocator_bytes,
        tiny_budget,
    );
    _ = zig_c.close(pipe_fds[1]);

    var buf: [4096]u8 = undefined;
    const bytes_read = zig_c.read(pipe_fds[0], &buf, buf.len);
    try std.testing.expect(bytes_read > 0);

    const output = buf[0..@as(usize, @intCast(bytes_read))];

    // Response MUST NOT contain network_diag from partial success JSON
    try std.testing.expect(!std.mem.containsAtLeast(u8, output, 1, "\"network_diag\""));
}

// ============================================================================
// Test 6: Forced failure Content-Length matches error body length
// ============================================================================

test "handleStatus forced failure Content-Length matches error body" {
    const allocator = std.testing.allocator;

    var pipe_fds: [2]i32 = undefined;
    if (zig_c.pipe(&pipe_fds) != 0) return error.PipeFailed;
    defer {
        _ = zig_c.close(pipe_fds[0]);
        _ = zig_c.close(pipe_fds[1]);
    }

    var serve_ctx = server.ServeContext.init(allocator);
    defer serve_ctx.deinit();

    try routes.handleStatusWithPolicy(
        pipe_fds[1],
        &serve_ctx,
        false,
        test_allocator_bytes,
        tiny_budget,
    );
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

    // Content-Length MUST match actual body length (proves no partial JSON was written)
    try std.testing.expectEqual(body_len, reported_len);

    // Also verify the body is exactly the error JSON length
    try std.testing.expectEqual(response.Errors.internal_error.len, reported_len);
}

// ============================================================================
// Test 7: Forced failure response is independent from a later success response
// ============================================================================

test "forced failure response is independent from later success response" {
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

    // First call: force failure via tiny budget using the SHARED helper
    try routes.handleStatusWithPolicy(
        pipe_fds1[1],
        &serve_ctx,
        false,
        test_allocator_bytes,
        tiny_budget,
    );
    _ = zig_c.close(pipe_fds1[1]);

    // Second call: use production handleStatus (should succeed)
    try routes.handleStatus(pipe_fds2[1], &serve_ctx, false);
    _ = zig_c.close(pipe_fds2[1]);

    var buf1: [4096]u8 = undefined;
    const bytes_read1 = zig_c.read(pipe_fds1[0], &buf1, buf1.len);
    try std.testing.expect(bytes_read1 > 0);
    const output1 = buf1[0..@as(usize, @intCast(bytes_read1))];

    var buf2: [4096]u8 = undefined;
    const bytes_read2 = zig_c.read(pipe_fds2[0], &buf2, buf2.len);
    try std.testing.expect(bytes_read2 > 0);
    const output2 = buf2[0..@as(usize, @intCast(bytes_read2))];

    // First response MUST be HTTP 500
    try std.testing.expect(std.mem.containsAtLeast(u8, output1, 1, "HTTP/1.1 500"));

    // Second response MUST be HTTP 200 (success)
    try std.testing.expect(std.mem.containsAtLeast(u8, output2, 1, "HTTP/1.1 200 OK"));

    // First response MUST NOT contain service name (proves it was error, not partial success)
    try std.testing.expect(!std.mem.containsAtLeast(u8, output1, 1, "\"service\":\"tovarisch\""));

    // Second response MUST contain service name (proves it was success)
    try std.testing.expect(std.mem.containsAtLeast(u8, output2, 1, "\"service\":\"tovarisch\""));

    // Responses are independent - first is error, second is success
    try std.testing.expect(!std.mem.eql(u8, output1, output2));
}
