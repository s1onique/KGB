const std = @import("std");
const routes = @import("routes.zig");

// --- Request parsing tests ---

test "parseRequestLine parses valid GET request" {
    const req = routes.parseRequestLine("GET /healthz HTTP/1.1");
    try std.testing.expect(req != null);
    try std.testing.expect(req.?.method == .get);
    try std.testing.expect(std.mem.eql(u8, req.?.path, "/healthz"));
    try std.testing.expect(std.mem.eql(u8, req.?.version, "HTTP/1.1"));
}

test "parseRequestLine parses status.json request" {
    const req = routes.parseRequestLine("GET /status.json HTTP/1.1");
    try std.testing.expect(req != null);
    try std.testing.expect(req.?.method == .get);
    try std.testing.expect(std.mem.eql(u8, req.?.path, "/status.json"));
}

test "parseRequestLine parses metrics.json request" {
    const req = routes.parseRequestLine("GET /metrics.json HTTP/1.1");
    try std.testing.expect(req != null);
    try std.testing.expect(req.?.method == .get);
    try std.testing.expect(std.mem.eql(u8, req.?.path, "/metrics.json"));
}

test "parseRequestLine parses /status alias request" {
    const req = routes.parseRequestLine("GET /status HTTP/1.1");
    try std.testing.expect(req != null);
    try std.testing.expect(req.?.method == .get);
    try std.testing.expect(std.mem.eql(u8, req.?.path, "/status"));
}

test "parseRequestLine parses /lab/probe request" {
    const req = routes.parseRequestLine("GET /lab/probe HTTP/1.1");
    try std.testing.expect(req != null);
    try std.testing.expect(req.?.method == .get);
    try std.testing.expect(std.mem.eql(u8, req.?.path, "/lab/probe"));
}

test "parseRequestLine parses /unknown path for 404" {
    const req = routes.parseRequestLine("GET /unknown HTTP/1.1");
    try std.testing.expect(req != null);
    try std.testing.expect(req.?.method == .get);
    try std.testing.expect(std.mem.eql(u8, req.?.path, "/unknown"));
}

test "parseRequestLine returns null for invalid line" {
    try std.testing.expect(routes.parseRequestLine("") == null);
    try std.testing.expect(routes.parseRequestLine("INVALID") == null);
    try std.testing.expect(routes.parseRequestLine("GET") == null);
    try std.testing.expect(routes.parseRequestLine("GET /") == null);
}

test "parseRequestLine handles unknown methods" {
    const req = routes.parseRequestLine("INVALIDMETHOD /test HTTP/1.1");
    try std.testing.expect(req != null);
    try std.testing.expect(req.?.method == .unknown);
}

test "parseRequestLine handles all HTTP methods" {
    try std.testing.expect(routes.parseRequestLine("GET /test HTTP/1.1") != null);
    try std.testing.expect(routes.parseRequestLine("POST /test HTTP/1.1") != null);
    try std.testing.expect(routes.parseRequestLine("PUT /test HTTP/1.1") != null);
    try std.testing.expect(routes.parseRequestLine("DELETE /test HTTP/1.1") != null);
    try std.testing.expect(routes.parseRequestLine("PATCH /test HTTP/1.1") != null);
    try std.testing.expect(routes.parseRequestLine("HEAD /test HTTP/1.1") != null);
    try std.testing.expect(routes.parseRequestLine("OPTIONS /test HTTP/1.1") != null);
}

test "parseMethod maps uppercase HTTP methods to enum" {
    try std.testing.expect(routes.parseMethod("GET") == .get);
    try std.testing.expect(routes.parseMethod("POST") == .post);
    try std.testing.expect(routes.parseMethod("PUT") == .put);
    try std.testing.expect(routes.parseMethod("DELETE") == .delete);
    try std.testing.expect(routes.parseMethod("PATCH") == .patch);
    try std.testing.expect(routes.parseMethod("HEAD") == .head);
    try std.testing.expect(routes.parseMethod("OPTIONS") == .options);
    try std.testing.expect(routes.parseMethod("INVALID") == .unknown);
}

test "parseRequestLine handles query strings" {
    // Path with query string
    const req = routes.parseRequestLine("GET /status?include=network_diag HTTP/1.1");
    try std.testing.expect(req != null);
    try std.testing.expect(std.mem.eql(u8, req.?.path, "/status"));
    try std.testing.expect(std.mem.eql(u8, req.?.query, "include=network_diag"));
}
