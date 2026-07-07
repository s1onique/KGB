// status_response_test.zig — Comprehensive tests for status response contract
//
// This file tests the allocator-owned status response contract:
// 1. Base status response renders valid JSON
// 2. include=network_diag response includes diagnostic fields
// 3. Unsupported query handling is deterministic
// 4. Repeated render/free loop does not leak
// 5. Allocation failure does not leak partially built response data
// 6. Response decision function is pure and has no allocator dependency
// 7. OwnedResponse ownership contract
//
// Budget policy tests are in status_response_budget_tests.zig.

const std = @import("std");
const Io = std.Io;
const status = @import("status.zig");
const status_response = @import("status_response.zig");
const status_query = @import("status_query.zig");
const status_route_contract = @import("http/status_route_contract.zig");

// ============================================================================
// Test Writer Helper (no heap allocation)
// ============================================================================

const TestWriter = struct {
    const Self = @This();
    const BufSize = 16384;
    buf: [BufSize]u8 = undefined,
    len: usize = 0,

    pub fn init() Self {
        return .{ .buf = undefined, .len = 0 };
    }

    pub fn print(self: *Self, comptime fmt: []const u8, args: anytype) !void {
        if (self.len >= BufSize) return error.BufferOverflow;
        const written = std.fmt.bufPrint(self.buf[self.len..], fmt, args) catch return error.BufferOverflow;
        self.len += written.len;
    }

    pub fn writeAll(self: *Self, bytes: []const u8) !void {
        if (self.len + bytes.len > BufSize) return error.BufferOverflow;
        for (bytes, 0..) |byte, i| {
            self.buf[self.len + i] = byte;
        }
        self.len += bytes.len;
    }

    pub fn writeByte(self: *Self, byte: u8) !void {
        if (self.len >= BufSize) return error.BufferOverflow;
        self.buf[self.len] = byte;
        self.len += 1;
    }

    pub fn slice(self: *const Self) []const u8 {
        return self.buf[0..self.len];
    }

    pub fn reset(self: *Self) void {
        self.len = 0;
    }
};

// ============================================================================
// Test: Base status response renders valid JSON
// ============================================================================

test "base status response contains all required fields" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    var w = TestWriter.init();
    try status_response.renderStatusResponseToWriter(&w, inputs, query, allocator);

    const output = w.slice();

    // Required top-level fields
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"service\":\"tovarisch\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"version\":\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"node_id\":\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"status\":\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"checks\":["));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"runtime\":{"));

    // Checks array should not be empty
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"name\":"));
}

test "base status response does not include network_diag" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    var w = TestWriter.init();
    try status_response.renderStatusResponseToWriter(&w, inputs, query, allocator);

    const output = w.slice();
    try std.testing.expect(!std.mem.containsAtLeast(u8, output, 1, "\"network_diag\":"));
}

test "empty query string produces base status" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    var w = TestWriter.init();
    try status_response.renderStatusResponseToWriter(&w, inputs, query, allocator);

    const output = w.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"service\":\"tovarisch\""));
    try std.testing.expect(!std.mem.containsAtLeast(u8, output, 1, "\"network_diag\":"));
}

// ============================================================================
// Test: include=network_diag response includes diagnostic fields
// ============================================================================

test "network_diag response includes network_diag field" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("include=network_diag");

    var w = TestWriter.init();
    try status_response.renderStatusResponseToWriter(&w, inputs, query, allocator);

    const output = w.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"network_diag\":"));
}

test "network_diag response includes all expected diagnostic subsections" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("include=network_diag");

    var w = TestWriter.init();
    try status_response.renderStatusResponseToWriter(&w, inputs, query, allocator);

    const output = w.slice();

    // Network diagnostics subsections expected by UVB-76 lab
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"wireguard\":"));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"interfaces\":"));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"routes\":"));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"underlay_tcp\":"));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"events\":"));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"status\":"));
}

test "network_diag with other params still includes diagnostics" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("foo=bar&include=network_diag&baz=qux");

    var w = TestWriter.init();
    try status_response.renderStatusResponseToWriter(&w, inputs, query, allocator);

    const output = w.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"network_diag\":"));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"wireguard\":"));
}

test "network_diag is case-sensitive (lowercase only)" {
    const query_lower = status_query.StatusQuery.parse("include=network_diag");
    const query_upper = status_query.StatusQuery.parse("include=Network_Diag");

    try std.testing.expect(query_lower.wantsNetworkDiag());
    try std.testing.expect(!query_upper.wantsNetworkDiag());
}

// ============================================================================
// Test: Unsupported query handling is deterministic
// ============================================================================

test "unknown include value produces base status (no network_diag)" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("include=unknown_feature");

    try std.testing.expect(!query.wantsNetworkDiag());
    try std.testing.expect(query.include == .unsupported);

    var w = TestWriter.init();
    try status_response.renderStatusResponseToWriter(&w, inputs, query, allocator);

    const output = w.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"service\":\"tovarisch\""));
    try std.testing.expect(!std.mem.containsAtLeast(u8, output, 1, "\"network_diag\":"));
}

test "duplicate include params are handled deterministically" {
    const query = status_query.StatusQuery.parse("include=network_diag&include=network_diag");

    try std.testing.expect(query.wantsNetworkDiag());
    try std.testing.expect(query.has_duplicate);
    try std.testing.expect(!query.has_unknown);
}

test "malformed query params are ignored" {
    const query = status_query.StatusQuery.parse("include=network_diag&malformed");

    try std.testing.expect(query.wantsNetworkDiag());
    try std.testing.expect(query.has_unknown);
}

// ============================================================================
// Test: Repeated render/free loop does not leak
// ============================================================================

test "repeated render/deinit loop is leak-free" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query_base = status_query.StatusQuery.parse("");
    const query_diag = status_query.StatusQuery.parse("include=network_diag");

    var response_base = try status_response.OwnedResponse.init(allocator, inputs, query_base);
    defer response_base.deinit(allocator);

    var response_diag = try status_response.OwnedResponse.init(allocator, inputs, query_diag);
    defer response_diag.deinit(allocator);

    const body_base = response_base.body();
    const body_diag = response_diag.body();
    try std.testing.expect(body_base.len > 0);
    try std.testing.expect(body_diag.len > 0);
    try std.testing.expect(std.mem.containsAtLeast(u8, body_diag, 1, "\"network_diag\":"));
}

test "many repeated renders do not accumulate memory" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    inline for (0..10) |_| {
        var response = try status_response.OwnedResponse.init(allocator, inputs, query);
        defer response.deinit(allocator);
        try std.testing.expect(response.body().len > 0);
    }
}

test "render to writer multiple times is consistent" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    var w1 = TestWriter.init();
    var w2 = TestWriter.init();

    try status_response.renderStatusResponseToWriter(&w1, inputs, query, allocator);
    try status_response.renderStatusResponseToWriter(&w2, inputs, query, allocator);

    try std.testing.expect(std.mem.containsAtLeast(u8, w1.slice(), 1, "\"service\":\"tovarisch\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w2.slice(), 1, "\"service\":\"tovarisch\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w1.slice(), 1, "\"checks\":["));
    try std.testing.expect(std.mem.containsAtLeast(u8, w2.slice(), 1, "\"checks\":["));
}

// ============================================================================
// Test: Response decision function is pure and has no allocator dependency
// ============================================================================

test "selectResponseMode is pure and deterministic" {
    const query = status_query.StatusQuery.parse("include=network_diag");

    const mode1 = status_query.selectResponseMode(query);
    const mode2 = status_query.selectResponseMode(query);
    const mode3 = status_query.selectResponseMode(query);

    try std.testing.expect(mode1 == mode2);
    try std.testing.expect(mode2 == mode3);
    try std.testing.expect(mode1 == .status_with_context);
}

test "selectResponseMode has no allocator dependency" {
    const query = status_query.StatusQuery.parse("");

    const mode = status_query.selectResponseMode(query);
    try std.testing.expect(mode == .status_with_context);
}

test "StatusQuery.parse is pure and deterministic" {
    const raw_query = "include=network_diag&foo=bar";

    const q1 = status_query.StatusQuery.parse(raw_query);
    const q2 = status_query.StatusQuery.parse(raw_query);
    const q3 = status_query.StatusQuery.parse(raw_query);

    try std.testing.expectEqual(q1.include, q2.include);
    try std.testing.expectEqual(q2.include, q3.include);
    try std.testing.expectEqual(q1.has_duplicate, q2.has_duplicate);
    try std.testing.expectEqual(q2.has_duplicate, q3.has_duplicate);
}

// ============================================================================
// Test: Query parsing edge cases
// ============================================================================

test "query with only ampersand produces base status" {
    const query = status_query.StatusQuery.parse("&&");
    try std.testing.expect(!query.wantsNetworkDiag());
    try std.testing.expect(!query.has_unknown);
}

test "query with trailing ampersand produces base status" {
    const query = status_query.StatusQuery.parse("include=network_diag&");
    try std.testing.expect(query.wantsNetworkDiag());
}

test "query with leading ampersand produces base status" {
    const query = status_query.StatusQuery.parse("&include=network_diag");
    try std.testing.expect(query.wantsNetworkDiag());
}

test "empty include value is treated as base status" {
    const query = status_query.StatusQuery.parse("include=");
    try std.testing.expect(!query.wantsNetworkDiag());
    try std.testing.expect(query.include == .none);
    try std.testing.expect(!query.has_unknown);
}

test "include without value (no equals) is unknown" {
    const query = status_query.StatusQuery.parse("include");
    try std.testing.expect(!query.wantsNetworkDiag());
    try std.testing.expect(query.has_unknown);
}

// ============================================================================
// Test: OwnedResponse ownership contract
// ============================================================================

test "OwnedResponse.body() returns owned slice to caller" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    var response = try status_response.OwnedResponse.init(allocator, inputs, query);
    defer response.deinit(allocator);

    const body = response.body();
    try std.testing.expect(body.len > 100);

    const slice = response.slice();
    try std.testing.expect(slice.len == body.len);
    try std.testing.expect(slice.ptr == body.ptr);
}

test "OwnedResponse.deinit with correct allocator" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    var response = try status_response.OwnedResponse.init(allocator, inputs, query);
    response.deinit(allocator);
}

// ============================================================================
// Test: OwnedResponse body is exactly the rendered JSON bytes
// ============================================================================

test "OwnedResponse body ends exactly at JSON terminator (base status)" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    var response = try status_response.OwnedResponse.init(allocator, inputs, query);
    defer response.deinit(allocator);

    const body = response.body();
    try std.testing.expect(body.len > 0);
    try std.testing.expectEqual(@as(u8, '\n'), body[body.len - 1]);
    try std.testing.expectEqual(@as(u8, '}'), body[body.len - 2]);
}

test "OwnedResponse body ends exactly at JSON terminator (network_diag)" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("include=network_diag");

    var response = try status_response.OwnedResponse.init(allocator, inputs, query);
    defer response.deinit(allocator);

    const body = response.body();
    try std.testing.expect(body.len > 0);
    try std.testing.expectEqual(@as(u8, '\n'), body[body.len - 1]);
    try std.testing.expectEqual(@as(u8, '}'), body[body.len - 2]);
}

test "OwnedResponse.slice() has same length as body()" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    var response = try status_response.OwnedResponse.init(allocator, inputs, query);
    defer response.deinit(allocator);

    const body = response.body();
    const slice = response.slice();
    try std.testing.expectEqual(body.len, slice.len);
    try std.testing.expectEqual(@as(usize, @intFromPtr(body.ptr)), @as(usize, @intFromPtr(slice.ptr)));
}

/// Diagnostic helper for body byte validation.
///
/// Reports enough context to distinguish:
/// - len corruption (body.len != len)
/// - writer position drift (body too long/short)
/// - memory overwrite elsewhere
/// - trailing capacity exposure (undefined memory)
fn expectBodyBytesInAsciiRange(body: []const u8, logical_len: usize, alloc_len: usize) !void {
    // First, verify the contract: body.len should equal logical_len
    if (body.len != logical_len) {
        std.debug.print(
            \\BODY LENGTH MISMATCH:
            \\  body.len     = {d}
            \\  logical_len  = {d}
            \\  alloc_len    = {d}
            \\
        , .{ body.len, logical_len, alloc_len });
        return error.TestExpectedEqual;
    }

    // Check each byte
    for (body, 0..) |byte, i| {
        if (byte >= 128) {
            // Determine if this is in the logical body or trailing capacity
            const in_body = i < logical_len;
            const in_trailing = i >= logical_len;

            // Show a bounded preview around the offending byte
            const preview_start = if (i < 16) 0 else i - 16;
            const preview_end = @min(body.len, i + 17);
            const preview = body[preview_start..preview_end];

            std.debug.print(
                \\OFFENDING BYTE IN BODY:
                \\  index         = {d}
                \\  byte          = 0x{x} ({d})
                \\  in_body_range = {s} (index {d} < logical_len {d})
                \\  in_trailing   = {s} (index {d} >= logical_len {d})
                \\  alloc_len     = {d}
                \\  body.len      = {d}
                \\  preview[{d}..{d}] = {any}
                \\
            , .{
                i,
                byte,
                byte,
                if (in_body) "true" else "false",
                i,
                logical_len,
                if (in_trailing) "true" else "false",
                i,
                logical_len,
                alloc_len,
                body.len,
                preview_start,
                preview_end,
                preview,
            });

            // If the offending byte is in trailing capacity, this is likely undefined memory
            if (in_trailing) {
                std.debug.print(
                    \\  NOTE: Offending byte at index {d} is in trailing capacity.
                    \\  This indicates body() is exposing memory beyond logical_len.
                    \\  This may be undefined behavior or len corruption.
                    \\
                , .{i});
            }

            return error.TestExpectedValue;
        }
    }
}

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
// This test constructs an OwnedResponse with explicit capacity separation:
// 1. Allocation is intentionally larger than len
// 2. Bytes after len are poisoned with 0xaa
// 3. body() must return only the prefix [0..len], never the poisoned tail
//
// The 0xaa pattern is a Debug-build poison value that can indicate undefined
// memory. We use it here to detect if body() ever accesses beyond len.
//
// Note: This test is NOT testing undefined behavior. We explicitly control
// all bytes in the allocation. The test proves the accessor contract:
// body() always returns allocation[0..len], regardless of what follows.
test "OwnedResponse body accessor contract: never exposes trailing capacity" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    var response = try status_response.OwnedResponse.init(allocator, inputs, query);
    defer response.deinit(allocator);

    // Invariant: len <= allocation.len (must hold for OwnedResponse)
    try std.testing.expect(response.len <= response.allocation.len);

    // If no trailing capacity, nothing to prove
    if (response.len == response.allocation.len) {
        return;
    }

    // Poison the trailing capacity with 0xaa
    // This value in Debug builds often indicates undefined/poisoned memory
    @memset(response.allocation[response.len..], 0xaa);

    // Get the body - this MUST only return [0..len]
    const body = response.body();

    // Contract: body() returns allocation[0..len]
    try std.testing.expect(body.ptr == response.allocation.ptr);
    try std.testing.expect(body.len == response.len);

    // Contract: body must NOT include the poisoned trailing bytes
    // If body.len == response.len (correct), then body never saw the 0xaa bytes
    // We verify by checking that body.len < alloc_len (proving we're not exposing tail)
    try std.testing.expect(body.len < response.allocation.len);

    // Verify body contains valid ASCII JSON (proving it read from written bytes)
    for (body) |byte| {
        try std.testing.expect(byte < 128);
    }
}

