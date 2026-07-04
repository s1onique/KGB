// status_route_contract.zig — Comptime route/query contract for /status.json
//
// This module defines a compile-time visible route contract table for the
// HTTP status boundary. Route contracts are declared once and validated
// at compile time.
//
// Design goals:
// - Route/query contracts are comptime-visible and table-shaped
// - Compile-time validation catches duplicate paths, methods, and query params
// - Path/method matching uses the route table
// - Query parsing still delegates to status_query.StatusQuery.parse
// - Response rendering still delegates to status_response.OwnedResponse

const std = @import("std");

/// Supported HTTP methods.
pub const Method = enum(u8) {
    get = 0,
};

/// Recognized query values.
pub const QueryValue = struct {
    name: []const u8,
};

/// Query parameter definition.
pub const QueryParam = struct {
    name: []const u8,
    values: []const QueryValue,
};

/// Response kind for a route.
pub const ResponseKind = enum(u8) {
    /// Status JSON response (base or with network_diag).
    status_json,
};

/// Route contract definition.
pub const RouteContract = struct {
    /// The route path (e.g., "/status.json").
    path: []const u8,

    /// Allowed HTTP methods for this route.
    methods: []const Method,

    /// Allowed query parameters and their values.
    query_params: []const QueryParam,

    /// The response kind for this route.
    response_kind: ResponseKind,
};

/// Compile-time validation error types.
pub const ValidateRouteError = error{
    /// Route path is empty.
    EmptyPath,
    /// Route path appears more than once in the table.
    DuplicatePath,
    /// Route has no methods defined.
    NoMethods,
    /// Method name is empty.
    EmptyMethod,
    /// Query param name is empty.
    EmptyQueryParam,
    /// Query param appears more than once in the route.
    DuplicateQueryParam,
    /// Query value name is empty.
    EmptyQueryValue,
    /// Query value appears more than once in the query param.
    DuplicateQueryValue,
    /// Route does not exist.
    RouteNotFound,
    /// Method not supported for route.
    MethodNotSupported,
    /// Query value not recognized for route.
    QueryValueNotRecognized,
};

/// Validate that a route table has no duplicate paths.
fn validateNoDuplicatePaths(table: []const RouteContract) ValidateRouteError!void {
    for (table, 0..) |route, i| {
        if (route.path.len == 0) {
            return error.EmptyPath;
        }
        for (table[0..i]) |prev| {
            if (std.mem.eql(u8, prev.path, route.path)) {
                return error.DuplicatePath;
            }
        }
    }
}

/// Validate that all routes have valid method lists.
fn validateMethods(table: []const RouteContract) ValidateRouteError!void {
    for (table) |route| {
        if (route.methods.len == 0) {
            return error.NoMethods;
        }
    }
}

/// Validate that a query param list has no duplicates.
fn validateQueryParams(route: *const RouteContract) ValidateRouteError!void {
    for (route.query_params, 0..) |param, i| {
        if (param.name.len == 0) {
            return error.EmptyQueryParam;
        }
        for (route.query_params[0..i]) |prev| {
            if (std.mem.eql(u8, prev.name, param.name)) {
                return error.DuplicateQueryParam;
            }
        }
        // Validate query values
        for (param.values, 0..) |value, j| {
            if (value.name.len == 0) {
                return error.EmptyQueryValue;
            }
            for (param.values[0..j]) |prev| {
                if (std.mem.eql(u8, prev.name, value.name)) {
                    return error.DuplicateQueryValue;
                }
            }
        }
    }
}

/// Validate an entire route table at compile time.
/// This function is called during comptime to ensure the route table
/// has no structural issues.
pub fn validateRouteTable(table: []const RouteContract) ValidateRouteError!void {
    try validateNoDuplicatePaths(table);
    try validateMethods(table);
    for (table) |*route| {
        try validateQueryParams(route);
    }
}

/// Lookup a route by path.
/// Returns the route contract if found, null otherwise.
pub fn lookupRoute(table: []const RouteContract, path: []const u8) ?*const RouteContract {
    for (table) |*route| {
        if (std.mem.eql(u8, route.path, path)) {
            return route;
        }
    }
    return null;
}

/// Check if a method is allowed for a route.
/// Returns true if the method is supported, false otherwise.
pub fn isMethodAllowed(route: *const RouteContract, method: Method) bool {
    for (route.methods) |allowed| {
        if (allowed == method) {
            return true;
        }
    }
    return false;
}

/// Check if a query value is allowed for a query param.
/// Returns true if the value is recognized, false otherwise.
pub fn isQueryValueAllowed(param: *const QueryParam, value_name: []const u8) bool {
    for (param.values) |value| {
        if (std.mem.eql(u8, value.name, value_name)) {
            return true;
        }
    }
    return false;
}

/// Find a query param by name in a route.
/// Returns the query param if found, null otherwise.
pub fn findQueryParam(route: *const RouteContract, name: []const u8) ?*const QueryParam {
    for (route.query_params) |*param| {
        if (std.mem.eql(u8, param.name, name)) {
            return param;
        }
    }
    return null;
}

/// Route lookup result.
pub const RouteLookupResult = struct {
    route: *const RouteContract,
    method_allowed: bool,
};

/// Perform route lookup by path and method.
/// Returns the lookup result if route found, null otherwise.
pub fn lookupRouteWithMethod(
    table: []const RouteContract,
    path: []const u8,
    method: Method,
) ?RouteLookupResult {
    if (lookupRoute(table, path)) |route| {
        return RouteLookupResult{
            .route = route,
            .method_allowed = isMethodAllowed(route, method),
        };
    }
    return null;
}

/// The route contract table for /status.json.
///
/// This table is validated at compile time to ensure:
/// - No duplicate paths
/// - Every route has at least one method
/// - Query param names are non-empty
/// - No duplicate query param names within a route
/// - Query values are non-empty
/// - No duplicate query values within a query param
pub const routes = blk: {
    const route_table = [_]RouteContract{
        .{
            .path = "/status.json",
            .methods = &.{.get},
            .query_params = &.{
                .{
                    .name = "include",
                    .values = &.{.{.name = "network_diag"}},
                },
            },
            .response_kind = .status_json,
        },
    };

    // Validate at comptime - this will fail compilation if the table is invalid
    validateRouteTable(&route_table) catch |err| {
        // Force a compile error with the actual error
        if (err == error.DuplicatePath) {
            @compileError("route table error: duplicate path detected");
        } else if (err == error.EmptyPath) {
            @compileError("route table error: empty path detected");
        } else if (err == error.NoMethods) {
            @compileError("route table error: route has no methods");
        } else if (err == error.EmptyMethod) {
            @compileError("route table error: empty method detected");
        } else if (err == error.EmptyQueryParam) {
            @compileError("route table error: empty query param detected");
        } else if (err == error.DuplicateQueryParam) {
            @compileError("route table error: duplicate query param detected");
        } else if (err == error.EmptyQueryValue) {
            @compileError("route table error: empty query value detected");
        } else if (err == error.DuplicateQueryValue) {
            @compileError("route table error: duplicate query value detected");
        }
    };

    break :blk route_table;
};

/// Singleton access to the route table.
pub const status_json_route = &routes[0];

// ============================================================================
// Tests
// ============================================================================

test "route table has exactly one entry" {
    try std.testing.expect(routes.len == 1);
}

test "route table contains /status.json" {
    try std.testing.expectEqualStrings("/status.json", routes[0].path);
}

test "/status.json route supports GET method" {
    try std.testing.expect(isMethodAllowed(&routes[0], .get));
}

test "/status.json route includes network_diag query value" {
    const route = lookupRoute(&routes, "/status.json");
    try std.testing.expect(route != null);

    const param = findQueryParam(route.?, "include");
    try std.testing.expect(param != null);
    try std.testing.expect(isQueryValueAllowed(param.?, "network_diag"));
}

test "lookupRoute finds /status.json" {
    const route = lookupRoute(&routes, "/status.json");
    try std.testing.expect(route != null);
}

test "lookupRoute returns null for unknown path" {
    const route = lookupRoute(&routes, "/unknown");
    try std.testing.expect(route == null);
}

test "lookupRouteWithMethod accepts GET for /status.json" {
    const result = lookupRouteWithMethod(&routes, "/status.json", .get);
    try std.testing.expect(result != null);
    try std.testing.expect(result.?.method_allowed);
}

test "lookupRouteWithMethod rejects unknown path" {
    const result = lookupRouteWithMethod(&routes, "/unknown", .get);
    try std.testing.expect(result == null);
}

test "findQueryParam returns null for unknown param" {
    const route = lookupRoute(&routes, "/status.json").?;
    const param = findQueryParam(route, "unknown");
    try std.testing.expect(param == null);
}

test "isQueryValueAllowed returns false for unknown value" {
    const route = lookupRoute(&routes, "/status.json").?;
    const param = findQueryParam(route, "include").?;
    try std.testing.expect(!isQueryValueAllowed(param, "unknown_value"));
}

test "isMethodAllowed returns true for allowed method" {
    try std.testing.expect(isMethodAllowed(&routes[0], .get));
}
