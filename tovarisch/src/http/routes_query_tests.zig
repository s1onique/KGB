// routes_query_tests.zig — Tests for HTTP route query parameter handling
//
// Extracted from routes.zig to satisfy LLM-friendly line limits.
// Tests cover:
// - Request target parsing with query strings
// - Query parameter extraction
// - include=network_diag detection

const std = @import("std");
const routes = @import("routes.zig");

// --- Query parameter parsing tests ---

test "parseRequestLine extracts query from request target" {
    const req = routes.parseRequestLine("GET /status.json?include=network_diag HTTP/1.1");
    try std.testing.expect(req != null);
    try std.testing.expect(std.mem.eql(u8, req.?.path, "/status.json"));
    try std.testing.expect(std.mem.eql(u8, req.?.query, "include=network_diag"));
}

test "parseRequestLine extracts query with multiple params" {
    const req = routes.parseRequestLine("GET /status.json?foo=bar&include=network_diag HTTP/1.1");
    try std.testing.expect(req != null);
    try std.testing.expect(std.mem.eql(u8, req.?.path, "/status.json"));
    try std.testing.expect(std.mem.eql(u8, req.?.query, "foo=bar&include=network_diag"));
}

test "parseRequestLine handles query with trailing param" {
    const req = routes.parseRequestLine("GET /status.json?include=network_diag&debug=true HTTP/1.1");
    try std.testing.expect(req != null);
    try std.testing.expect(std.mem.eql(u8, req.?.path, "/status.json"));
    try std.testing.expect(std.mem.eql(u8, req.?.query, "include=network_diag&debug=true"));
}

test "parseRequestLine handles path without query" {
    const req = routes.parseRequestLine("GET /status.json HTTP/1.1");
    try std.testing.expect(req != null);
    try std.testing.expect(std.mem.eql(u8, req.?.path, "/status.json"));
    try std.testing.expect(std.mem.eql(u8, req.?.query, ""));
}

test "parseRequestLine handles /status path with query" {
    const req = routes.parseRequestLine("GET /status?include=network_diag HTTP/1.1");
    try std.testing.expect(req != null);
    try std.testing.expect(std.mem.eql(u8, req.?.path, "/status"));
    try std.testing.expect(std.mem.eql(u8, req.?.query, "include=network_diag"));
}

test "parseRequestLine handles healthz path without query" {
    const req = routes.parseRequestLine("GET /healthz HTTP/1.1");
    try std.testing.expect(req != null);
    try std.testing.expect(std.mem.eql(u8, req.?.path, "/healthz"));
    try std.testing.expect(std.mem.eql(u8, req.?.query, ""));
}

test "parseRequestLine handles metrics.json path with query" {
    const req = routes.parseRequestLine("GET /metrics.json?debug=1 HTTP/1.1");
    try std.testing.expect(req != null);
    try std.testing.expect(std.mem.eql(u8, req.?.path, "/metrics.json"));
    try std.testing.expect(std.mem.eql(u8, req.?.query, "debug=1"));
}

test "parseRequestLine handles /status.json/extra as different path" {
    // Verify that paths with extra segments don't accidentally match
    const req = routes.parseRequestLine("GET /status.json/extra?include=network_diag HTTP/1.1");
    try std.testing.expect(req != null);
    try std.testing.expect(std.mem.eql(u8, req.?.path, "/status.json/extra"));
    try std.testing.expect(std.mem.eql(u8, req.?.query, "include=network_diag"));
}

test "parseRequestLine handles /api/v1/status as different path" {
    const req = routes.parseRequestLine("GET /api/v1/status?include=network_diag HTTP/1.1");
    try std.testing.expect(req != null);
    try std.testing.expect(std.mem.eql(u8, req.?.path, "/api/v1/status"));
    try std.testing.expect(std.mem.eql(u8, req.?.query, "include=network_diag"));
}
