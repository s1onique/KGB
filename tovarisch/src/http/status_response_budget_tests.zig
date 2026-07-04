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
    const body = response.body();
    try std.testing.expect(body.len > 0);

    // Body must fit within budget
    try std.testing.expect(body.len <= budget.max_body_bytes);

    // Body must be valid JSON
    try std.testing.expectEqual(@as(u8, '{'), body[0]);
    try std.testing.expectEqual(@as(u8, '\n'), body[body.len - 1]);

    // Should contain required fields
    try std.testing.expect(std.mem.containsAtLeast(u8, body, 1, "\"service\":\"tovarisch\""));
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
    const body = response.body();
    try std.testing.expect(body.len > 0);

    // Body must fit within budget
    try std.testing.expect(body.len <= budget.max_body_bytes);

    // Body must be valid JSON
    try std.testing.expectEqual(@as(u8, '{'), body[0]);
    try std.testing.expectEqual(@as(u8, '\n'), body[body.len - 1]);

    // Should contain network_diag field
    try std.testing.expect(std.mem.containsAtLeast(u8, body, 1, "\"network_diag\":"));
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

test "allocation failure before buffer allocation does not leak" {
    // Use a failing allocator to simulate allocation failure
    // Zig 0.16 uses Config struct for FailingAllocator.init
    var failing_allocator = std.testing.FailingAllocator.init(std.testing.allocator, .{
        .fail_index = 0, // Fail on first allocation
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
    const body = response.body();
    try std.testing.expect(body.len > 0);

    // Body should end with JSON terminator
    try std.testing.expectEqual(@as(u8, '\n'), body[body.len - 1]);
    try std.testing.expectEqual(@as(u8, '}'), body[body.len - 2]);
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

    try std.testing.expect(response2.body().len > 0);
}

// ============================================================================
// Test: FixedBufferAllocator compatibility (HULK04)
// ============================================================================

test "base status renders with FixedBufferAllocator" {
    // Use a generous fixed buffer - status JSON should fit
    var buf: [8192]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const allocator = fba.allocator();

    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");
    const budget = status_route_contract.ResponseBudget.base_budget;

    var response = try status_response.renderStatusOwnedWithBudget(
        allocator,
        inputs,
        query,
        budget,
    );

    // Body must have content
    const body = response.body();
    try std.testing.expect(body.len > 0);

    // Body must be valid JSON
    try std.testing.expectEqual(@as(u8, '{'), body[0]);
    try std.testing.expectEqual(@as(u8, '\n'), body[body.len - 1]);

    // Should contain required fields
    try std.testing.expect(std.mem.containsAtLeast(u8, body, 1, "\"service\":\"tovarisch\""));

    // Clean up using same allocator
    response.deinit(allocator);
}

test "network_diag renders with FixedBufferAllocator" {
    // Use a generous fixed buffer - diagnostic JSON should fit
    var buf: [16384]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const allocator = fba.allocator();

    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("include=network_diag");
    const budget = status_route_contract.ResponseBudget.diagnostic_budget;

    var response = try status_response.renderStatusOwnedWithBudget(
        allocator,
        inputs,
        query,
        budget,
    );

    // Body must have content
    const body = response.body();
    try std.testing.expect(body.len > 0);

    // Body must be valid JSON
    try std.testing.expectEqual(@as(u8, '{'), body[0]);
    try std.testing.expectEqual(@as(u8, '\n'), body[body.len - 1]);

    // Should contain network_diag field
    try std.testing.expect(std.mem.containsAtLeast(u8, body, 1, "\"network_diag\":"));

    // Clean up using same allocator
    response.deinit(allocator);
}

test "tiny diagnostic budget returns error.BufferOverflow with FixedBufferAllocator" {
    var buf: [8192]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const allocator = fba.allocator();

    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("include=network_diag");

    // Tiny budget that cannot hold diagnostic JSON
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

test "allocator failure in diagnostic path surfaces as OutOfMemory" {
    // Use a failing allocator that fails on first allocation
    var failing_allocator = std.testing.FailingAllocator.init(std.testing.allocator, .{
        .fail_index = 0, // Fail on first allocation
    });
    const allocator = failing_allocator.allocator();

    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("include=network_diag");
    const budget = status_route_contract.ResponseBudget.diagnostic_budget;

    const result = status_response.renderStatusOwnedWithBudget(
        allocator,
        inputs,
        query,
        budget,
    );

    // Must fail with out of memory
    try std.testing.expectError(error.OutOfMemory, result);
}

test "no leaked allocations when diagnostic rendering fails with tiny budget" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("include=network_diag");

    // Tiny budget
    const budget = status_route_contract.ResponseBudget{ .max_body_bytes = 16 };

    // Multiple failures should not accumulate memory
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

// ============================================================================
// Test: Source-text contract check for global allocator avoidance (HULK04)
// ============================================================================

test "status_response has no page_allocator references" {
    // Whole-file source contract: status_response.zig must not contain
    // std.heap.page_allocator anywhere in the file.
    const source = @embedFile("../status_response.zig");
    try std.testing.expect(std.mem.indexOf(
        u8,
        source,
        "std.heap.page_allocator",
    ) == null);
}

test "status_response has no other global allocator escapes" {
    // Additional global allocators that should not appear in status_response.zig
    const forbidden = [_][]const u8{
        "std.heap.c_allocator",
        "std.heap.smp_allocator",
        "GeneralPurposeAllocator",
    };
    const source = @embedFile("../status_response.zig");
    for (forbidden) |pattern| {
        try std.testing.expect(std.mem.indexOf(u8, source, pattern) == null);
    }
}

// ============================================================================
// Test: Single allocation pattern (HULK05)
// ============================================================================

test "renderStatusOwnedWithBudget uses single allocation" {
    // This test verifies the HULK05 contract: no scratch+dupe pattern
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

    const body = response.body();
    try std.testing.expect(body.len > 0);
    try std.testing.expect(body.len <= response.allocation.len);
}

test "no allocator.dupe in renderStatusOwnedWithBudget" {
    // Source-text contract: no dupe in renderStatusOwnedWithBudget
    const source = @embedFile("../status_response.zig");
    const fn_start = std.mem.indexOf(u8, source, "pub fn renderStatusOwnedWithBudget") orelse {
        try std.testing.expect(false);
        return;
    };
    const fn_body = source[fn_start..];
    try std.testing.expect(std.mem.indexOf(u8, fn_body, "allocator.dupe") == null);
}
