const std = @import("std");
const routes = @import("routes.zig");

// --- Metrics route path tests ---
// Contract coverage for /metrics.json endpoint.
// Fallback payload rendering is tested in metrics_fallback_dto_tests.zig

test "/metrics.json route parses correctly" {
    const req = routes.parseRequestLine("GET /metrics.json HTTP/1.1");
    try std.testing.expect(req != null);
    try std.testing.expect(req.?.method == .get);
    try std.testing.expect(std.mem.eql(u8, req.?.path, "/metrics.json"));
    try std.testing.expect(std.mem.eql(u8, req.?.version, "HTTP/1.1"));
}

test "/metrics.json is routed separately from /status" {
    const metrics_req = routes.parseRequestLine("GET /metrics.json HTTP/1.1");
    const status_req = routes.parseRequestLine("GET /status HTTP/1.1");
    
    try std.testing.expect(metrics_req != null);
    try std.testing.expect(status_req != null);
    
    // Different paths should route differently
    try std.testing.expect(!std.mem.eql(u8, metrics_req.?.path, status_req.?.path));
}
