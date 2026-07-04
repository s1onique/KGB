// status_route_contract_test.zig — Tests for route contract table
//
// This file contains comprehensive tests for the route contract module,
// including compile-time validation tests.
//
// Tests cover:
// 1. Route existence and properties
// 2. Route lookup by path
// 3. Method allowance checking
// 4. Query param/value validation
// 5. Compile-time validation for invalid tables (via helper functions)

const std = @import("std");
const status_route_contract = @import("status_route_contract.zig");

// ============================================================================
// Route existence tests
// ============================================================================

test "/status.json route exists in table" {
    const route = status_route_contract.lookupRoute(
        &status_route_contract.routes,
        "/status.json",
    );
    try std.testing.expect(route != null);
}

test "/status.json has status_json response kind" {
    const route = status_route_contract.lookupRoute(
        &status_route_contract.routes,
        "/status.json",
    ).?;
    try std.testing.expect(route.response_kind == .status_json);
}

// ============================================================================
// Method tests
// ============================================================================

test "/status.json supports GET" {
    const route = status_route_contract.lookupRoute(
        &status_route_contract.routes,
        "/status.json",
    ).?;
    try std.testing.expect(status_route_contract.isMethodAllowed(route, .get));
}

test "method lookup accepts GET for /status.json" {
    const result = status_route_contract.lookupRouteWithMethod(
        &status_route_contract.routes,
        "/status.json",
        .get,
    );
    try std.testing.expect(result != null);
    try std.testing.expect(result.?.method_allowed);
}

// ============================================================================
// Query contract tests
// ============================================================================

test "/status.json declares include=network_diag" {
    const route = status_route_contract.lookupRoute(
        &status_route_contract.routes,
        "/status.json",
    ).?;
    const param = status_route_contract.findQueryParam(route, "include");
    try std.testing.expect(param != null);
    try std.testing.expect(status_route_contract.isQueryValueAllowed(param.?, "network_diag"));
}

test "query contract lookup accepts include=network_diag" {
    const route = status_route_contract.lookupRoute(
        &status_route_contract.routes,
        "/status.json",
    ).?;
    const param = status_route_contract.findQueryParam(route, "include");
    try std.testing.expect(param != null);
    try std.testing.expect(status_route_contract.isQueryValueAllowed(param.?, "network_diag"));
}

test "query contract lookup rejects unknown include value" {
    const route = status_route_contract.lookupRoute(
        &status_route_contract.routes,
        "/status.json",
    ).?;
    const param = status_route_contract.findQueryParam(route, "include");
    try std.testing.expect(param != null);
    try std.testing.expect(!status_route_contract.isQueryValueAllowed(param.?, "unknown_value"));
}

test "query contract lookup rejects unknown param" {
    const route = status_route_contract.lookupRoute(
        &status_route_contract.routes,
        "/status.json",
    ).?;
    const param = status_route_contract.findQueryParam(route, "nonexistent");
    try std.testing.expect(param == null);
}

// ============================================================================
// Route lookup tests
// ============================================================================

test "route lookup finds /status.json" {
    const route = status_route_contract.lookupRoute(
        &status_route_contract.routes,
        "/status.json",
    );
    try std.testing.expect(route != null);
    try std.testing.expectEqualStrings("/status.json", route.?.path);
}

test "route lookup rejects unknown path" {
    const route = status_route_contract.lookupRoute(
        &status_route_contract.routes,
        "/unknown",
    );
    try std.testing.expect(route == null);
}

test "route lookup rejects /healthz" {
    const route = status_route_contract.lookupRoute(
        &status_route_contract.routes,
        "/healthz",
    );
    try std.testing.expect(route == null);
}

test "route lookup rejects /metrics.json" {
    const route = status_route_contract.lookupRoute(
        &status_route_contract.routes,
        "/metrics.json",
    );
    try std.testing.expect(route == null);
}

// ============================================================================
// Method rejection tests
// ============================================================================

test "route lookup with method rejects unknown path" {
    const result = status_route_contract.lookupRouteWithMethod(
        &status_route_contract.routes,
        "/unknown",
        .get,
    );
    try std.testing.expect(result == null);
}

// ============================================================================
// Compile-time validation tests (via helper functions)
// ============================================================================

test "validateRouteTable accepts valid table" {
    // Create a minimal valid table for testing validation
    const valid_table = [_]status_route_contract.RouteContract{
        .{
            .path = "/test",
            .methods = &.{.get},
            .query_params = &.{},
            .response_kind = .status_json,
        },
    };

    // This should not error
    try status_route_contract.validateRouteTable(&valid_table);
}

test "validateRouteTable rejects empty path" {
    const invalid_table = [_]status_route_contract.RouteContract{
        .{
            .path = "",
            .methods = &.{.get},
            .query_params = &.{},
            .response_kind = .status_json,
        },
    };

    const result = status_route_contract.validateRouteTable(&invalid_table);
    try std.testing.expectError(status_route_contract.ValidateRouteError.EmptyPath, result);
}

test "validateRouteTable rejects duplicate paths" {
    const invalid_table = [_]status_route_contract.RouteContract{
        .{
            .path = "/test",
            .methods = &.{.get},
            .query_params = &.{},
            .response_kind = .status_json,
        },
        .{
            .path = "/test",
            .methods = &.{.get},
            .query_params = &.{},
            .response_kind = .status_json,
        },
    };

    const result = status_route_contract.validateRouteTable(&invalid_table);
    try std.testing.expectError(status_route_contract.ValidateRouteError.DuplicatePath, result);
}

test "validateRouteTable rejects empty methods" {
    const invalid_table = [_]status_route_contract.RouteContract{
        .{
            .path = "/test",
            .methods = &.{},
            .query_params = &.{},
            .response_kind = .status_json,
        },
    };

    const result = status_route_contract.validateRouteTable(&invalid_table);
    try std.testing.expectError(status_route_contract.ValidateRouteError.NoMethods, result);
}

test "validateRouteTable rejects empty query param name" {
    const invalid_table = [_]status_route_contract.RouteContract{
        .{
            .path = "/test",
            .methods = &.{.get},
            .query_params = &.{
                .{
                    .name = "",
                    .values = &.{.{.name = "value"}},
                },
            },
            .response_kind = .status_json,
        },
    };

    const result = status_route_contract.validateRouteTable(&invalid_table);
    try std.testing.expectError(status_route_contract.ValidateRouteError.EmptyQueryParam, result);
}

test "validateRouteTable rejects duplicate query params" {
    const invalid_table = [_]status_route_contract.RouteContract{
        .{
            .path = "/test",
            .methods = &.{.get},
            .query_params = &.{
                .{
                    .name = "param",
                    .values = &.{},
                },
                .{
                    .name = "param",
                    .values = &.{},
                },
            },
            .response_kind = .status_json,
        },
    };

    const result = status_route_contract.validateRouteTable(&invalid_table);
    try std.testing.expectError(status_route_contract.ValidateRouteError.DuplicateQueryParam, result);
}

test "validateRouteTable rejects empty query value" {
    const invalid_table = [_]status_route_contract.RouteContract{
        .{
            .path = "/test",
            .methods = &.{.get},
            .query_params = &.{
                .{
                    .name = "param",
                    .values = &.{.{.name = ""}},
                },
            },
            .response_kind = .status_json,
        },
    };

    const result = status_route_contract.validateRouteTable(&invalid_table);
    try std.testing.expectError(status_route_contract.ValidateRouteError.EmptyQueryValue, result);
}

test "validateRouteTable rejects duplicate query values" {
    const invalid_table = [_]status_route_contract.RouteContract{
        .{
            .path = "/test",
            .methods = &.{.get},
            .query_params = &.{
                .{
                    .name = "param",
                    .values = &.{
                        .{ .name = "value" },
                        .{ .name = "value" },
                    },
                },
            },
            .response_kind = .status_json,
        },
    };

    const result = status_route_contract.validateRouteTable(&invalid_table);
    try std.testing.expectError(status_route_contract.ValidateRouteError.DuplicateQueryValue, result);
}

// ============================================================================
// Singleton accessor tests
// ============================================================================

test "status_json_route singleton points to /status.json" {
    try std.testing.expectEqualStrings(
        "/status.json",
        status_route_contract.status_json_route.path,
    );
}

test "status_json_route has GET method" {
    try std.testing.expect(
        status_route_contract.isMethodAllowed(
            status_route_contract.status_json_route,
            .get,
        ),
    );
}

// ============================================================================
// Response kind tests
// ============================================================================

test "/status.json route has status_json response kind" {
    const route = status_route_contract.lookupRoute(
        &status_route_contract.routes,
        "/status.json",
    ).?;
    try std.testing.expect(route.response_kind == .status_json);
}
