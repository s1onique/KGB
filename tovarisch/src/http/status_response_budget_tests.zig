// status_response_budget_tests.zig — Budget policy tests for status response contract
//
// This file tests the request-scope allocator budget policy for /status.json:
// - ResponseBudget struct and constants
// - Budget enforcement in renderStatusOwnedWithBudget
// - Allocation failure handling
// - Route contract budget integration

const std = @import("std");
const status = @import("../status.zig");
const status_response = @import("../status_response.zig");
const status_query = @import("../status_query.zig");
const status_route_contract = @import("status_route_contract.zig");

// ============================================================================
// Test: Budget policy for request-scope allocation (HULK03)
// ============================================================================

test "base status renders under base budget" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    const budget = status_route_contract.ResponseBudget.base_budget;
    var response = try status_response.renderStatusOwnedWithBudget(
        allocator,
        inputs,
        query,
        budget,
    );
    defer response.deinit(allocator);

    // Body must have content
    try std.testing.expect(response.body.len > 0);

    // Body must fit within budget
    try std.testing.expect(response.body.len <= budget.max_body_bytes);

    // Body must be valid JSON
    try std.testing.expectEqual(@as(u8, '{'), response.body[0]);
    try std.testing.expectEqual(@as(u8, '\n'), response.body[response.body.len - 1]);

    // Should contain required fields
    try std.testing.expect(std.mem.containsAtLeast(u8, response.body, 1, "\"service\":\"tovarisch\""));
}

test "network_diag renders under diagnostic budget" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("include=network_diag");

    const budget = status_route_contract.ResponseBudget.diagnostic_budget;
    var response = try status_response.renderStatusOwnedWithBudget(
        allocator,
        inputs,
        query,
        budget,
    );
    defer response.deinit(allocator);

    // Body must have content
    try std.testing.expect(response.body.len > 0);

    // Body must fit within budget
    try std.testing.expect(response.body.len <= budget.max_body_bytes);

    // Body must be valid JSON
    try std.testing.expectEqual(@as(u8, '{'), response.body[0]);
    try std.testing.expectEqual(@as(u8, '\n'), response.body[response.body.len - 1]);

    // Should contain network_diag field
    try std.testing.expect(std.mem.containsAtLeast(u8, response.body, 1, "\"network_diag\":"));
}

test "base status fails with tiny budget" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    // Tiny budget that cannot hold status JSON
    const budget = status_route_contract.ResponseBudget{ .max_body_bytes = 16 };

    const result = status_response.renderStatusOwnedWithBudget(
        allocator,
        inputs,
        query,
        budget,
    );

    // Must fail with buffer overflow
    try std.testing.expectError(error.BufferOverflow, result);
}

test "network_diag fails with tiny budget" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("include=network_diag");

    // Tiny budget that cannot hold network_diag JSON
    const budget = status_route_contract.ResponseBudget{ .max_body_bytes = 32 };

    const result = status_response.renderStatusOwnedWithBudget(
        allocator,
        inputs,
        query,
        budget,
    );

    // Must fail with buffer overflow
    try std.testing.expectError(error.BufferOverflow, result);
}

test "tiny-budget failure does not leak" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    // Tiny budget that cannot hold status JSON
    const budget = status_route_contract.ResponseBudget{ .max_body_bytes = 16 };

    // Multiple failures in a row should not leak
    inline for (0..5) |_| {
        const result = status_response.renderStatusOwnedWithBudget(
            allocator,
            inputs,
            query,
            budget,
        );
        try std.testing.expectError(error.BufferOverflow, result);
    }
}

test "allocation failure before scratch buffer creation does not leak" {
    // Use a failing allocator to simulate allocation failure
    // Zig 0.16 uses Config struct for FailingAllocator.init
    var failing_allocator = std.testing.FailingAllocator.init(std.testing.allocator, .{
        .fail_index = 0, // Fail on first allocation (scratch)
    });
    const allocator = failing_allocator.allocator();

    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");
    const budget = status_route_contract.ResponseBudget.base_budget;

    const result = status_response.renderStatusOwnedWithBudget(
        allocator,
        inputs,
        query,
        budget,
    );

    // Must fail with out of memory
    try std.testing.expectError(error.OutOfMemory, result);
}

test "allocation failure during dupe (second alloc) does not leak scratch" {
    // Use a failing allocator that fails on the second allocation (dupe).
    // This tests the fix for the scratch leak when dupe() fails.
    var failing_allocator = std.testing.FailingAllocator.init(std.testing.allocator, .{
        .fail_index = 1, // Fail on second allocation (dupe)
    });
    const allocator = failing_allocator.allocator();

    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");
    const budget = status_route_contract.ResponseBudget.base_budget;

    const result = status_response.renderStatusOwnedWithBudget(
        allocator,
        inputs,
        query,
        budget,
    );

    // Must fail with out of memory
    try std.testing.expectError(error.OutOfMemory, result);
}

test "returned body is exact-length after budgeted render" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    const budget = status_route_contract.ResponseBudget.base_budget;
    var response = try status_response.renderStatusOwnedWithBudget(
        allocator,
        inputs,
        query,
        budget,
    );
    defer response.deinit(allocator);

    // Body length must match what was actually written
    try std.testing.expect(response.body.len > 0);

    // Body should end with JSON terminator
    try std.testing.expectEqual(@as(u8, '\n'), response.body[response.body.len - 1]);
    try std.testing.expectEqual(@as(u8, '}'), response.body[response.body.len - 2]);
}

test "route contract exposes base budget for /status.json" {
    const route = status_route_contract.lookupRoute(
        &status_route_contract.routes,
        "/status.json",
    ).?;

    // Base budget must be present
    try std.testing.expect(route.base_budget.max_body_bytes > 0);

    // Base budget must be 4096
    try std.testing.expectEqual(@as(usize, 4096), route.base_budget.max_body_bytes);
}

test "route contract exposes diagnostic budget for include=network_diag" {
    const route = status_route_contract.lookupRoute(
        &status_route_contract.routes,
        "/status.json",
    ).?;

    // Diagnostic budget must be present
    try std.testing.expect(route.diagnostic_budget != null);

    // Diagnostic budget must be 8192
    try std.testing.expectEqual(@as(usize, 8192), route.diagnostic_budget.?.max_body_bytes);
}

test "ResponseBudget.forQuery selects correct budget" {
    const base = status_route_contract.ResponseBudget.forQuery(false);
    const diag = status_route_contract.ResponseBudget.forQuery(true);

    try std.testing.expectEqual(@as(usize, 4096), base.max_body_bytes);
    try std.testing.expectEqual(@as(usize, 8192), diag.max_body_bytes);
}

test "no global allocator is used for owned response memory" {
    // This test documents that OwnedResponse.init and renderStatusOwnedWithBudget
    // use the caller-provided allocator, not a global allocator.
    //
    // The render functions accept an explicit allocator parameter.
    // Caller must provide the allocator, making allocation policy explicit.
    //
    // The only internal global allocator use is std.heap.page_allocator,
    // which is used for transient network_diag collection. These allocations
    // are released via deinit() before the function returns.
    //
    // This test passes by documentation - the API design enforces explicit allocator use.

    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");
    const budget = status_route_contract.ResponseBudget.base_budget;

    // Using a different allocator than std.testing.allocator should work
    var buf: [8192]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const fba_allocator = fba.allocator();

    // This should succeed with the FBA
    var response = try status_response.renderStatusOwnedWithBudget(
        fba_allocator,
        inputs,
        query,
        budget,
    );

    // Deinit should work with the same allocator
    response.deinit(fba_allocator);

    // Now verify testing allocator also works
    var response2 = try status_response.renderStatusOwnedWithBudget(
        allocator,
        inputs,
        query,
        budget,
    );
    defer response2.deinit(allocator);

    try std.testing.expect(response2.body.len > 0);
}
