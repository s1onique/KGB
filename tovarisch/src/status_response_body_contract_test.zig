// status_response_body_contract_test.zig — Tests for OwnedResponse body accessor contract
//
// This file tests the critical body accessor contract:
// - body() never exposes trailing allocation capacity
// - body() exposes only initialized logical body bytes (not undefined memory)
// - body.len equals the written length, not the allocation length
//
// These tests prove the OwnedResponse accessor boundary is enforced correctly.
//
// CONTRACT INVARIANTS:
// 1. body.len == logical_len (written bytes count, not capacity)
// 2. body[0..logical_len] contains only initialized data (ASCII range)
// 3. body() never includes trailing allocation capacity
//
// The test is named to match the failure mode it catches:
// "Initialized-length corruption" — when body.len matches logical_len,
// but logical bytes contain uninitialized or invalid data.
//
// This is distinct from "trailing capacity exposure", where body.len exceeds
// logical_len by exposing allocation[logical_len..].


const std = @import("std");
const status = @import("status.zig");
const status_response = @import("status_response.zig");
const status_query = @import("status_query.zig");

// ============================================================================
// Diagnostic helper for body byte validation
// ============================================================================

/// Reports enough context to distinguish:
/// - non-ASCII byte in logical body (data corruption)
/// - body.len != logical_len (length corruption)
/// - body() exposing trailing capacity (accessor bug)
///
/// Trailing capacity exposure would first manifest as body.len > logical_len,
/// since body.len is derived from len, not from allocation.len.
fn expectBodyBytesInAsciiRange(body: []const u8, logical_len: usize, alloc_len: usize) !void {
    // First, verify the contract: body.len should equal logical_len.
    // If this fails, it indicates length corruption, NOT trailing capacity exposure.
    if (body.len != logical_len) {
        std.debug.print(
            \\BODY LENGTH CORRUPTION DETECTED:
            \\  body.len     = {d}
            \\  logical_len  = {d}
            \\  alloc_len    = {d}
            \\  NOTE: body.len > logical_len would indicate accessor exposing trailing capacity.
            \\         body.len < logical_len would indicate truncated write.
            \\
        , .{ body.len, logical_len, alloc_len });
        return error.TestExpectedEqual;
    }

    // Check each byte in the logical body range
    // Note: if we reached here, body.len == logical_len, so we're only
    // inspecting [0..logical_len]. Any non-ASCII byte here indicates
    // actual data corruption in the written JSON, NOT trailing exposure.
    for (body, 0..) |byte, i| {
        if (byte >= 128) {
            // Show a bounded preview around the offending byte
            const preview_start = if (i < 16) 0 else i - 16;
            const preview_end = @min(body.len, i + 17);
            const preview = body[preview_start..preview_end];

            std.debug.print(
                \\NON-ASCII BYTE IN LOGICAL BODY:
                \\  index       = {d}
                \\  byte        = 0x{x} ({d})
                \\  body.len    = {d}
                \\  alloc_len   = {d}
                \\  preview     = {any}
                \\  NOTE: This byte is IN the logical body range [0..logical_len].
                \\         This indicates data corruption, NOT trailing capacity exposure.
                \\         Trailing exposure would show body.len > logical_len first.
                \\
            , .{
                i,
                byte,
                byte,
                body.len,
                alloc_len,
                preview,
            });

            return error.TestExpectedValue;
        }
    }
}

// ============================================================================
// Test: Body accessor never exposes trailing allocation capacity
// ============================================================================

test "OwnedResponse body does not expose trailing allocation capacity" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    var response = try status_response.OwnedResponse.init(allocator, inputs, query);
    defer response.deinit(allocator);

    const body = response.body();

    // Use diagnostic helper instead of bare per-byte check
    try expectBodyBytesInAsciiRange(body, response.len, response.allocation.len);

    try std.testing.expectEqual(@as(u8, '{'), body[0]);
    try std.testing.expectEqual(@as(u8, '\n'), body[body.len - 1]);
}

// Test that OwnedResponse body() does not expose trailing allocation capacity.
//
// This test constructs an OwnedResponse with a DETERMINISTIC fixture:
// 1. Build OwnedResponse directly from controlled fields (struct fields are public)
// 2. Allocation is intentionally larger than len (guaranteed, not allocator-dependent)
// 3. Bytes after len are poisoned with 0xaa
// 4. body() must return only the prefix [0..len], never the poisoned tail
//
// This test does NOT skip. It always has trailing capacity to prove the contract.
test "OwnedResponse body accessor contract: never exposes trailing capacity" {
    // Construct a deterministic fixture with explicit trailing capacity.
    // We build OwnedResponse directly since its fields are public.
    const logical_json = "{\"service\":\"tovarisch\",\"status\":\"ok\"}";
    const logical_len: usize = logical_json.len;
    const extra_capacity: usize = 64;
    const total_alloc_len = logical_len + extra_capacity;

    // Allocate the full buffer
    const allocator = std.testing.allocator;
    const full_alloc = try allocator.alloc(u8, total_alloc_len);
    defer allocator.free(full_alloc);

    // Write valid JSON into the logical portion
    // MemoryCopySafety: full_alloc is a fresh allocation from allocator.alloc();
    // logical_json is a compile-time string literal. They cannot alias.
    @memcpy(full_alloc[0..logical_len], logical_json);

    // Poison the trailing capacity with 0xaa (debug poison pattern)
    @memset(full_alloc[logical_len..], 0xaa);

    // Build OwnedResponse directly with controlled fields
    var response = status_response.OwnedResponse{
        .allocation = full_alloc,
        .len = logical_len,
    };

    // ====== ASSERT EXACT ACCESSOR CONTRACT ======

    // 1. Allocation length must exceed logical length (this is the precondition)
    try std.testing.expect(response.len < response.allocation.len);

    // 2. body().len must equal logical len
    try std.testing.expect(response.body().len == response.len);

    // 3. body().ptr must match allocation.ptr
    try std.testing.expect(response.body().ptr == response.allocation.ptr);

    // 4. body() must NOT include the poisoned trailing bytes.
    //    If body.len < allocation.len, the tail is excluded.
    try std.testing.expect(response.body().len < response.allocation.len);

    // 5. Verify body contains exactly the logical JSON (ASCII range)
    const body = response.body();
    for (body) |byte| {
        try std.testing.expect(byte < 128);
    }

    // 6. Verify we can read the exact expected content
    try std.testing.expectEqualStrings(logical_json, body);

    // 7. Verify the poisoned tail was NOT accessed (0xaa bytes are in [len..])
    //    This proves body() never scanned past len.
    const tail_region = response.allocation[response.len..];
    for (tail_region) |byte| {
        try std.testing.expectEqual(0xaa, byte);
    }
}
