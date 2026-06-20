// status_network_diag_ownership_tests.zig — Memory ownership tests for network diagnostics
const std = @import("std");
const testing = std.testing;
const NetworkDiag = @import("status_network_diag_types.zig").NetworkDiag;
const RouteOutput = @import("status_network_diag_types.zig").RouteOutput;

test "NetworkDiag.deinit handles empty slices" {
    const allocator = testing.allocator;
    var diag = NetworkDiag{
        .started_at = try allocator.dupe(u8, "12345"),
        .status = .ok,
        .wireguard = null,
        .interfaces = &.{},
        .routes = &.{},
        .underlay_tcp = &.{},
        .events = &.{},
    };
    diag.deinit(allocator);
}

test "RouteOutput with null gateway" {
    const allocator = testing.allocator;
    const route = RouteOutput{
        .target = try allocator.dupe(u8, "10.0.0.0/8"),
        .interface = try allocator.dupe(u8, "lo"),
        .source = try allocator.dupe(u8, ""),
        .gateway = null,
        .status = try allocator.dupe(u8, "up"),
    };

    defer {
        allocator.free(route.target);
        allocator.free(route.interface);
        allocator.free(route.source);
        allocator.free(route.status);
    }

    try testing.expect(route.gateway == null);
}
