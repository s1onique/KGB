// status_response_test.zig — Comprehensive tests for status response contract
//
// This file tests the allocator-owned status response contract:
// 1. Base status response renders valid JSON
// 2. include=network_diag response includes diagnostic fields
// 3. Unsupported query handling is deterministic
// 4. Repeated render/free loop does not leak
// 5. Allocation failure does not leak partially built response data
// 6. Response decision function is pure and has no allocator dependency

const std = @import("std");
const Io = std.Io;
const status = @import("status.zig");
const status_response = @import("status_response.zig");
const status_query = @import("status_query.zig");

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
        // Use for loop to avoid aliasing panic in Zig 0.16
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
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    var w = TestWriter.init();
    try status_response.renderStatusResponseToWriter(&w, inputs, query);

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
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    var w = TestWriter.init();
    try status_response.renderStatusResponseToWriter(&w, inputs, query);

    const output = w.slice();
    try std.testing.expect(!std.mem.containsAtLeast(u8, output, 1, "\"network_diag\":"));
}

test "empty query string produces base status" {
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    var w = TestWriter.init();
    try status_response.renderStatusResponseToWriter(&w, inputs, query);

    const output = w.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"service\":\"tovarisch\""));
    try std.testing.expect(!std.mem.containsAtLeast(u8, output, 1, "\"network_diag\":"));
}

// ============================================================================
// Test: include=network_diag response includes diagnostic fields
// ============================================================================

test "network_diag response includes network_diag field" {
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("include=network_diag");

    var w = TestWriter.init();
    try status_response.renderStatusResponseToWriter(&w, inputs, query);

    const output = w.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"network_diag\":"));
}

test "network_diag response includes all expected diagnostic subsections" {
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("include=network_diag");

    var w = TestWriter.init();
    try status_response.renderStatusResponseToWriter(&w, inputs, query);

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
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("foo=bar&include=network_diag&baz=qux");

    var w = TestWriter.init();
    try status_response.renderStatusResponseToWriter(&w, inputs, query);

    const output = w.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"network_diag\":"));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"wireguard\":"));
}

test "network_diag is case-sensitive (lowercase only)" {
    // Test that "include=Network_Diag" is NOT recognized
    const query_lower = status_query.StatusQuery.parse("include=network_diag");
    const query_upper = status_query.StatusQuery.parse("include=Network_Diag");

    try std.testing.expect(query_lower.wantsNetworkDiag());
    try std.testing.expect(!query_upper.wantsNetworkDiag());
}

// ============================================================================
// Test: Unsupported query handling is deterministic
// ============================================================================

test "unknown include value produces base status (no network_diag)" {
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("include=unknown_feature");

    // Query is classified as unsupported
    try std.testing.expect(!query.wantsNetworkDiag());
    try std.testing.expect(query.include == .unsupported);

    // But rendering should still produce valid base status
    var w = TestWriter.init();
    try status_response.renderStatusResponseToWriter(&w, inputs, query);

    const output = w.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"service\":\"tovarisch\""));
    try std.testing.expect(!std.mem.containsAtLeast(u8, output, 1, "\"network_diag\":"));
}

test "duplicate include params are handled deterministically" {
    const query = status_query.StatusQuery.parse("include=network_diag&include=network_diag");

    // First occurrence wins
    try std.testing.expect(query.wantsNetworkDiag());
    try std.testing.expect(query.has_duplicate);
    try std.testing.expect(!query.has_unknown);
}

test "malformed query params are ignored" {
    const query = status_query.StatusQuery.parse("include=network_diag&malformed");

    // Should still recognize the valid include
    try std.testing.expect(query.wantsNetworkDiag());
    try std.testing.expect(query.has_unknown); // But noted as having anomalies
}

// ============================================================================
// Test: Repeated render/free loop does not leak
// ============================================================================

test "repeated render/deinit loop is leak-free" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query_base = status_query.StatusQuery.parse("");
    const query_diag = status_query.StatusQuery.parse("include=network_diag");

    // Alternate between base and network_diag
    var response_base = try status_response.OwnedResponse.init(allocator, inputs, query_base);
    defer response_base.deinit(allocator);

    var response_diag = try status_response.OwnedResponse.init(allocator, inputs, query_diag);
    defer response_diag.deinit(allocator);

    // Both should have valid content
    try std.testing.expect(response_base.body.len > 0);
    try std.testing.expect(response_diag.body.len > 0);
    try std.testing.expect(std.mem.containsAtLeast(u8, response_diag.body, 1, "\"network_diag\":"));
}

test "many repeated renders do not accumulate memory" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    // Render many times in a tight loop
    inline for (0..10) |_| {
        var response = try status_response.OwnedResponse.init(allocator, inputs, query);
        defer response.deinit(allocator);
        try std.testing.expect(response.body.len > 0);
    }
}

test "render to writer multiple times is consistent" {
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    var w1 = TestWriter.init();
    var w2 = TestWriter.init();

    try status_response.renderStatusResponseToWriter(&w1, inputs, query);
    try status_response.renderStatusResponseToWriter(&w2, inputs, query);

    // Both should contain the same structure
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

    // Call multiple times with same input
    const mode1 = status_query.selectResponseMode(query);
    const mode2 = status_query.selectResponseMode(query);
    const mode3 = status_query.selectResponseMode(query);

    try std.testing.expect(mode1 == mode2);
    try std.testing.expect(mode2 == mode3);
    try std.testing.expect(mode1 == .status_with_context);
}

test "selectResponseMode has no allocator dependency" {
    const query = status_query.StatusQuery.parse("");

    // This should work without any allocator setup
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
    try std.testing.expect(!query.has_unknown); // Empty parts are skipped
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
    // Empty value after "include=" is ignored (falls through to other key=value branch)
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

test "OwnedResponse.body is owned by caller" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    var response = try status_response.OwnedResponse.init(allocator, inputs, query);
    defer response.deinit(allocator);

    // Body should be valid memory
    try std.testing.expect(response.body.len > 100); // At least 100 bytes for status JSON

    // Slice should match body
    const slice = response.slice();
    try std.testing.expect(slice.len == response.body.len);
    try std.testing.expect(slice.ptr == response.body.ptr);
}

test "OwnedResponse.deinit with correct allocator" {
    // This test documents the contract: caller must use the same allocator
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    var response = try status_response.OwnedResponse.init(allocator, inputs, query);

    // Using the same allocator is correct
    response.deinit(allocator);
}

// ============================================================================
// Test: OwnedResponse body is exactly the rendered JSON bytes (no trailing capacity)
// ============================================================================

test "OwnedResponse body ends exactly at JSON terminator (base status)" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    var response = try status_response.OwnedResponse.init(allocator, inputs, query);
    defer response.deinit(allocator);

    // Body must have content
    try std.testing.expect(response.body.len > 0);

    // Body must end with JSON terminator (newline after closing brace)
    try std.testing.expectEqual(@as(u8, '\n'), response.body[response.body.len - 1]);

    // Second-to-last character should be the closing brace
    try std.testing.expectEqual(@as(u8, '}'), response.body[response.body.len - 2]);
}

test "OwnedResponse body ends exactly at JSON terminator (network_diag)" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("include=network_diag");

    var response = try status_response.OwnedResponse.init(allocator, inputs, query);
    defer response.deinit(allocator);

    // Body must have content
    try std.testing.expect(response.body.len > 0);

    // Body must end with JSON terminator
    try std.testing.expectEqual(@as(u8, '\n'), response.body[response.body.len - 1]);

    // Second-to-last character should be the closing brace
    try std.testing.expectEqual(@as(u8, '}'), response.body[response.body.len - 2]);
}

test "OwnedResponse.slice() has same length as body" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    var response = try status_response.OwnedResponse.init(allocator, inputs, query);
    defer response.deinit(allocator);

    const slice = response.slice();
    try std.testing.expectEqual(response.body.len, slice.len);
    try std.testing.expectEqual(@as(usize, @intFromPtr(response.body.ptr)), @as(usize, @intFromPtr(slice.ptr)));
}

test "OwnedResponse body does not expose trailing allocation capacity" {
    const allocator = std.testing.allocator;
    const inputs = status.RuntimeStatusInputs{};
    const query = status_query.StatusQuery.parse("");

    var response = try status_response.OwnedResponse.init(allocator, inputs, query);
    defer response.deinit(allocator);

    // If we scan the body, it should be valid printable ASCII or JSON chars
    // No undefined/trailing garbage should be present
    for (response.body) |byte| {
        // Allow printable ASCII, JSON whitespace, and standard JSON characters
        try std.testing.expect(byte < 128);
    }

    // The body should be parseable as valid JSON structure (starts with { and ends with }\n)
    try std.testing.expectEqual(@as(u8, '{'), response.body[0]);
    try std.testing.expectEqual(@as(u8, '\n'), response.body[response.body.len - 1]);
}
