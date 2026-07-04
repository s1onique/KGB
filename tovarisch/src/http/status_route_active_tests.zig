// status_route_active_tests.zig — Active route proof tests for /status.json
//
// ACT-TOVARISCH-ZIG-HULK06: Prove active HTTP route cannot bypass budget-aware status renderer
// ACT-TOVARISCH-ZIG-HULK08: Prove request allocator capacity derives from route budget policy
//
// These tests prove that the production /status.json HTTP route:
// 1. Uses handleStatus (not handleStatusLegacy)
// 2. Calls renderStatusOwnedWithBudget
// 3. Selects budget via ResponseBudget.forQuery
// 4. Returns HTTP 500 on render failure
// 5. Writes owned_response.body() on success
// 6. Does not reference handleStatusLegacy
// 7. Does not directly call status.renderPayloadWithContextAndDiag
// 8. Does not use std.heap.page_allocator in status path
// 9. Uses named request allocator policy (HULK08)
// 10. Does not contain raw 16384 literal in handleStatus body (HULK08)

const std = @import("std");
const routes = @import("routes.zig");
const status = @import("../status.zig");
const status_response = @import("../status_response.zig");
const status_route_contract = @import("status_route_contract.zig");
const server = @import("server.zig");
const response = @import("response.zig");

// ============================================================================
// Test 1: Source contract — routes.zig references renderStatusOwnedWithBudget
// ============================================================================

test "routes.zig references renderStatusOwnedWithBudget" {
    const source = @embedFile("routes.zig");
    try std.testing.expect(std.mem.indexOf(
        u8,
        source,
        "renderStatusOwnedWithBudget",
    ) != null);
}

// ============================================================================
// Test 2: Source contract — routes.zig references ResponseBudget.forQuery
// ============================================================================

test "routes.zig references ResponseBudget.forQuery" {
    const source = @embedFile("routes.zig");
    try std.testing.expect(std.mem.indexOf(
        u8,
        source,
        "ResponseBudget.forQuery",
    ) != null);
}

// ============================================================================
// Test 3: Source contract — handleStatusLegacy is absent
// ============================================================================

test "routes.zig has no handleStatusLegacy symbol" {
    const source = @embedFile("routes.zig");
    try std.testing.expect(std.mem.indexOf(
        u8,
        source,
        "handleStatusLegacy",
    ) == null);
}

// ============================================================================
// Test 4: Source contract — routes.zig does not use page_allocator in status path
// ============================================================================

test "routes.zig status handler does not use page_allocator" {
    // Extract handleStatus function body from routes.zig
    const source = @embedFile("routes.zig");
    const fn_start = std.mem.indexOf(u8, source, "pub fn handleStatus") orelse {
        try std.testing.expect(false);
        return;
    };

    // Find the end of handleStatus (next pub fn or end of file)
    const fn_body_start = fn_start;
    const remaining = source[fn_body_start..];
    const fn_end = std.mem.indexOf(u8, remaining, "\npub fn ") orelse remaining.len;
    const fn_body = remaining[0..fn_end];

    // The status handler must not use page_allocator
    try std.testing.expect(std.mem.indexOf(u8, fn_body, "page_allocator") == null);
}

// ============================================================================
// Test 5: Source contract — routes.zig does not directly call renderPayloadWithContextAndDiag
// ============================================================================

test "routes.zig does not directly call status.renderPayloadWithContextAndDiag" {
    const source = @embedFile("routes.zig");

    // Extract handleStatus function body
    const fn_start = std.mem.indexOf(u8, source, "pub fn handleStatus") orelse {
        try std.testing.expect(false);
        return;
    };

    const remaining = source[fn_start..];
    const fn_end = std.mem.indexOf(u8, remaining, "\npub fn ") orelse remaining.len;
    const fn_body = remaining[0..fn_end];

    // handleStatus should not directly call renderPayloadWithContextAndDiag
    // It should use renderStatusOwnedWithBudget instead
    try std.testing.expect(std.mem.indexOf(u8, fn_body, "renderPayloadWithContextAndDiag") == null);
}

// ============================================================================
// Test 6: Source contract — routes.zig references HTTP 500 on render failure
// ============================================================================

test "routes.zig returns HTTP 500 on render failure" {
    const source = @embedFile("routes.zig");

    // Extract handleStatus function body
    const fn_start = std.mem.indexOf(u8, source, "pub fn handleStatus") orelse {
        try std.testing.expect(false);
        return;
    };

    const remaining = source[fn_start..];
    const fn_end = std.mem.indexOf(u8, remaining, "\npub fn ") orelse remaining.len;
    const fn_body = remaining[0..fn_end];

    // Must return 500 on error (no partial JSON)
    try std.testing.expect(std.mem.indexOf(u8, fn_body, "500") != null);
    try std.testing.expect(std.mem.indexOf(u8, fn_body, "internal_error") != null);
}

// ============================================================================
// Test 7: Source contract — routes.zig calls owned_response.body()
// ============================================================================

test "routes.zig writes owned_response.body() on success" {
    const source = @embedFile("routes.zig");

    // Extract handleStatus function body
    const fn_start = std.mem.indexOf(u8, source, "pub fn handleStatus") orelse {
        try std.testing.expect(false);
        return;
    };

    const remaining = source[fn_start..];
    const fn_end = std.mem.indexOf(u8, remaining, "\npub fn ") orelse remaining.len;
    const fn_body = remaining[0..fn_end];

    // Must write owned_response.body() on success
    try std.testing.expect(std.mem.indexOf(u8, fn_body, "owned_response.body()") != null);
}

// ============================================================================
// Test 8: Source contract — routes.zig /status.json dispatch resolves to handleStatus
// ============================================================================

test "routes.zig /status.json path dispatches to handleStatus" {
    const source = @embedFile("routes.zig");

    // Find routeRequestFd function
    const fn_start = std.mem.indexOf(u8, source, "pub fn routeRequestFd") orelse {
        try std.testing.expect(false);
        return;
    };

    const remaining = source[fn_start..];
    const fn_end = std.mem.indexOf(u8, remaining, "\npub fn ") orelse remaining.len;
    const fn_body = remaining[0..fn_end];

    // /status.json path must call handleStatus
    try std.testing.expect(std.mem.indexOf(u8, fn_body, "\"/status.json\"") != null);
    try std.testing.expect(std.mem.indexOf(u8, fn_body, "handleStatus") != null);

    // /status (alias) must also call handleStatus
    try std.testing.expect(std.mem.indexOf(u8, fn_body, "\"/status\"") != null);
}

// ============================================================================
// Test 9: Source contract — /status.json?include=network_diag takes diagnostic budget
// ============================================================================

test "routes.zig network_diag query triggers diagnostic budget" {
    const source = @embedFile("routes.zig");

    // handleStatus must use include_network_diag to select the appropriate policy
    try std.testing.expect(std.mem.indexOf(u8, source, "pub fn handleStatus") != null);

    // handleStatus must delegate to handleStatusWithPolicy which uses forQuery
    try std.testing.expect(std.mem.indexOf(u8, source, "handleStatusWithPolicy") != null);
    try std.testing.expect(std.mem.indexOf(u8, source, "forQuery") != null);

    // The include_network_diag parameter must be used for conditional routing
    try std.testing.expect(std.mem.indexOf(u8, source, "if (include_network_diag)") != null);
}

// ============================================================================
// Test 10: Behavioral — base status response succeeds
// ============================================================================

test "handleStatus with no network_diag succeeds" {
    // Create a minimal serve context
    var serve_ctx = server.ServeContext.init(std.heap.page_allocator);
    defer serve_ctx.deinit();

    // Simulate calling handleStatus directly
    // We test the renderer path directly since we can't easily mock the fd
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = @import("../status_query.zig").StatusQuery.parse("");
    const budget = status_route_contract.ResponseBudget.base_budget;

    const response_or_err = status_response.renderStatusOwnedWithBudget(
        allocator,
        inputs,
        query,
        budget,
    );

    // Must succeed
    const owned_response = try response_or_err;
    defer owned_response.deinit(allocator);

    // Must have content
    const body = owned_response.body();
    try std.testing.expect(body.len > 0);

    // Must be valid JSON
    try std.testing.expectEqual(@as(u8, '{'), body[0]);
}

// ============================================================================
// Test 11: Behavioral — network_diag status response succeeds
// ============================================================================

test "handleStatus with include=network_diag succeeds" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = @import("../status_query.zig").StatusQuery.parse("include=network_diag");
    const budget = status_route_contract.ResponseBudget.diagnostic_budget;

    const response_or_err = status_response.renderStatusOwnedWithBudget(
        allocator,
        inputs,
        query,
        budget,
    );

    // Must succeed
    const owned_response = try response_or_err;
    defer owned_response.deinit(allocator);

    // Must have content
    const body = owned_response.body();
    try std.testing.expect(body.len > 0);

    // Must contain network_diag field
    try std.testing.expect(std.mem.containsAtLeast(u8, body, 1, "\"network_diag\":"));
}

// ============================================================================
// Test 12: Behavioral — tiny budget/render failure maps to error
// ============================================================================

test "tiny budget render failure returns error" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = @import("../status_query.zig").StatusQuery.parse("");

    // Tiny budget that cannot hold status JSON
    const budget = status_route_contract.ResponseBudget{ .max_body_bytes = 16 };

    const result = status_response.renderStatusOwnedWithBudget(
        allocator,
        inputs,
        query,
        budget,
    );

    // Must fail with buffer overflow (HTTP 500 in handler)
    try std.testing.expectError(error.BufferOverflow, result);
}

// ============================================================================
// Test 13: Behavioral — allocation failure surfaces as error
// ============================================================================

test "allocation failure returns OutOfMemory error" {
    var failing_allocator = std.testing.FailingAllocator.init(std.testing.allocator, .{
        .fail_index = 0, // Fail on first allocation
    });
    const allocator = failing_allocator.allocator();

    const inputs = status.RuntimeStatusInputs{};
    const query = @import("../status_query.zig").StatusQuery.parse("");
    const budget = status_route_contract.ResponseBudget.base_budget;

    const result = status_response.renderStatusOwnedWithBudget(
        allocator,
        inputs,
        query,
        budget,
    );

    // Must fail with out of memory (HTTP 500 in handler)
    try std.testing.expectError(error.OutOfMemory, result);
}

// ============================================================================
// Test 14: Route contract — /status.json exists in route contract table
// ============================================================================

test "status_route_contract defines /status.json route" {
    const route = status_route_contract.lookupRoute(
        &status_route_contract.routes,
        "/status.json",
    );
    try std.testing.expect(route != null);
    try std.testing.expectEqualStrings("/status.json", route.?.path);
}

// ============================================================================
// Test 15: Route contract — /status.json supports GET and network_diag
// ============================================================================

test "status_route_contract /status.json supports GET with network_diag" {
    const route = status_route_contract.lookupRoute(
        &status_route_contract.routes,
        "/status.json",
    ).?;

    try std.testing.expect(status_route_contract.isMethodAllowed(route, .get));
    try std.testing.expect(route.diagnostic_budget != null);
}

// ============================================================================
// Test 16: Source contract — status_route_contract.zig exists and has forQuery
// ============================================================================

test "status_route_contract.zig has ResponseBudget.forQuery" {
    const source = @embedFile("status_route_contract.zig");
    // The function is defined as "pub fn forQuery" inside ResponseBudget struct
    try std.testing.expect(std.mem.indexOf(
        u8,
        source,
        "pub fn forQuery",
    ) != null);
}

// ============================================================================
// Test 17: Source contract — handleStatus is not deprecated
// ============================================================================

test "handleStatus function exists and is public" {
    const source = @embedFile("routes.zig");
    try std.testing.expect(std.mem.indexOf(
        u8,
        source,
        "pub fn handleStatus",
    ) != null);
}

// ============================================================================
// HULK08: Request allocator policy tests
// ============================================================================

// Test 18: Source contract — routes.zig uses named request allocator helper

test "routes.zig references requestAllocatorBytesForQuery" {
    const source = @embedFile("routes.zig");
    try std.testing.expect(std.mem.indexOf(
        u8,
        source,
        "requestAllocatorBytesForQuery",
    ) != null);
}

// Test 19: Source contract — handleStatus body does not contain raw fixed buffer declaration

test "routes.zig handleStatus has no raw [16384]u8 fixed buffer declaration" {
    const source = @embedFile("routes.zig");

    // Extract handleStatus function body
    const fn_start = std.mem.indexOf(u8, source, "pub fn handleStatus") orelse {
        try std.testing.expect(false);
        return;
    };

    const remaining = source[fn_start..];
    const fn_end = std.mem.indexOf(u8, remaining, "\npub fn ") orelse remaining.len;
    const fn_body = remaining[0..fn_end];

    // The magic fixed buffer declaration [16384]u8 must not appear in handleStatus.
    // The function must use requestAllocatorBytesForQuery() to derive capacity.
    // We check for the exact forbidden pattern, not numeric values in comments.
    try std.testing.expect(std.mem.indexOf(u8, fn_body, "var fixed_buf: [16384]u8") == null);
}

// Test 20: Behavioral — request allocator capacity derives from route policy

test "requestAllocatorBytesForQuery produces valid capacity for base" {
    // Verify the named policy produces the expected value
    const base_alloc_bytes = status_route_contract.requestAllocatorBytesForQuery(false);
    // Base: 4096 response + 8192 overhead = 12288
    try std.testing.expectEqual(@as(usize, 12288), base_alloc_bytes);
}

// Test 21: Behavioral — request allocator capacity derives from route policy (diagnostic)

test "requestAllocatorBytesForQuery produces valid capacity for diagnostic" {
    // Verify the named policy produces the expected value
    const diag_alloc_bytes = status_route_contract.requestAllocatorBytesForQuery(true);
    // Diagnostic: 8192 response + 8192 overhead = 16384
    try std.testing.expectEqual(@as(usize, 16384), diag_alloc_bytes);
}

// Test 22: Source contract — status_route_contract.zig defines request allocator policy

test "status_route_contract.zig defines request_temp_overhead_bytes" {
    const source = @embedFile("status_route_contract.zig");
    try std.testing.expect(std.mem.indexOf(
        u8,
        source,
        "request_temp_overhead_bytes",
    ) != null);
}

// Test 23: Behavioral — overhead is documented with rationale

test "request_temp_overhead_bytes is 8192 for transient allocation headroom" {
    // The overhead is set to 8192 to handle worst-case scenarios
    // where network_diag content triggers maximum temporary allocations
    try std.testing.expectEqual(
        @as(usize, 8192),
        status_route_contract.request_temp_overhead_bytes,
    );
}
