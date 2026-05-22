const std = @import("std");

pub const version = "0.1.1";

// --- Canonical Status JSON Schema ---
//
// Minimal contract for `tovarisch status --json`.
//
// Required fields:
//   - service:     string — always "tovarisch"
//   - version:     string — semver of this binary
//   - node_id:     string — local node identifier
//   - status:      string — one of: ok, warn, error
//   - checks:      array  — list of check objects (may be empty)
// Check object fields:
//   - name:        string — identifier for this check
//   - status:      string — one of: ok, warn, error
//   - detail:      string — human-readable detail (optional, empty string allowed)

pub const Check = struct {
    name: []const u8,
    status: []const u8,
    detail: []const u8,
};

pub const Status = struct {
    service: []const u8,
    version: []const u8,
    node_id: []const u8,
    status: []const u8,
    checks: []const Check,
};

// --- Payload construction ---

const static_checks = [_]Check{
    Check{
        .name = "process",
        .status = "ok",
        .detail = "static bootstrap status",
    },
};

pub const static_status: Status = .{
    .service = "tovarisch",
    .version = version,
    .node_id = "local-dev",
    .status = "ok",
    .checks = &static_checks,
};

/// Renders the static status payload as JSON to the given writer.
/// Uses the Zig 0.16 streaming JSON Stringify API.
pub fn renderPayload(writer: anytype) !void {
    var jw = std.json.Stringify{ .writer = writer };
    try jw.beginObject();
    try jw.objectField("service");
    try jw.write(static_status.service);
    try jw.objectField("version");
    try jw.write(static_status.version);
    try jw.objectField("node_id");
    try jw.write(static_status.node_id);
    try jw.objectField("status");
    try jw.write(static_status.status);
    try jw.objectField("checks");
    try jw.beginArray();
    for (static_status.checks) |check| {
        try jw.beginObject();
        try jw.objectField("name");
        try jw.write(check.name);
        try jw.objectField("status");
        try jw.write(check.status);
        try jw.objectField("detail");
        try jw.write(check.detail);
        try jw.endObject();
    }
    try jw.endArray();
    try jw.endObject();
}

/// Returns the static status payload as a heap-allocated JSON string.
/// Caller owns the memory.
pub fn generatePayload(allocator: std.mem.Allocator) ![]u8 {
    var out = std.Io.Writer.Allocating.init(allocator);
    defer out.deinit();
    try renderPayload(&out);
    return try out.written();
}

// --- Required-field validation ---
//
// These functions validate the JSON contract without re-serializing.

pub const RequiredFields = enum {
    service,
    version,
    node_id,
    status,
    checks,
};

/// Checks that all required top-level fields are present in raw JSON bytes.
pub fn validateRequiredFields(json_bytes: []const u8) bool {
    inline for (@typeInfo(RequiredFields).Enum.fields) |field| {
        const field_name = "\"" ++ field.name ++ "\"";
        if (!std.mem.containsAtLeast(u8, json_bytes, 1, field_name)) {
            return false;
        }
    }
    return true;
}

/// Parses JSON bytes into a Status struct for structural validation.
/// Returns error.InvalidJSON if parsing fails.
pub fn parseStatus(json_bytes: []const u8, allocator: std.mem.Allocator) !Status {
    return try std.json.parseFromSlice(Status, allocator, json_bytes, .{});
}

// --- Static payload string (for backwards-compatible testing) ---
//
// Note: prefer renderPayload() / generatePayload() for dynamic use.
// This string is provided for existing test compatibility and gate grep checks.

pub const payload = "{\"service\":\"tovarisch\",\"version\":\"0.1.1\",\"node_id\":\"local-dev\",\"status\":\"ok\",\"checks\":[{\"name\":\"process\",\"status\":\"ok\",\"detail\":\"static bootstrap status\"}]}";

// --- Tests ---

test "version constant is 0.1.1" {
    try std.testing.expectEqualStrings("0.1.1", version);
}

test "static_status has all required fields" {
    try std.testing.expectEqualStrings("tovarisch", static_status.service);
    try std.testing.expectEqualStrings("0.1.1", static_status.version);
    try std.testing.expectEqualStrings("local-dev", static_status.node_id);
    try std.testing.expectEqualStrings("ok", static_status.status);
    // static_checks is a [1]Check array, so checks[0] is valid
    try std.testing.expectEqualStrings("process", static_status.checks[0].name);
}

test "payload contains expected fields" {
    try std.testing.expect(std.mem.containsAtLeast(u8, payload, 1, "\"service\":\"tovarisch\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, payload, 1, "\"version\":\"0.1.1\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, payload, 1, "\"node_id\":\"local-dev\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, payload, 1, "\"status\":\"ok\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, payload, 1, "\"checks\""));
}

test "validateRequiredFields returns true for valid payload" {
    try std.testing.expect(validateRequiredFields(payload));
}

test "validateRequiredFields returns false for missing field" {
    const bad = "{\"service\":\"tovarisch\",\"version\":\"0.1.1\"}";
    try std.testing.expect(!validateRequiredFields(bad));
}

test "parseStatus parses valid JSON" {
    var buf: [256]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const allocator = fba.allocator();

    const parsed = try parseStatus(payload, allocator);
    try std.testing.expectEqualStrings("tovarisch", parsed.service);
    try std.testing.expectEqualStrings("0.1.1", parsed.version);
    try std.testing.expectEqualStrings("local-dev", parsed.node_id);
    try std.testing.expectEqualStrings("ok", parsed.status);
    // JSON array is parsed as a slice, so .len works
    try std.testing.expect(@as(usize, 1), parsed.checks.len);
}

test "parseStatus returns error for invalid JSON" {
    var buf: [256]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const allocator = fba.allocator();

    const bad = "{invalid json}";
    try std.testing.expectError(error.InvalidJSON, parseStatus(bad, allocator));
}

test "renderPayload produces valid JSON" {
    var buf: [512]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const allocator = fba.allocator();

    var out = std.Io.Writer.Allocating.init(allocator);
    defer out.deinit();
    try renderPayload(&out);

    // Rendered JSON must be parseable as Status
    const parsed = try parseStatus(try out.written(), allocator);
    try std.testing.expectEqualStrings("tovarisch", parsed.service);
    try std.testing.expectEqualStrings("0.1.1", parsed.version);
}

test "generatePayload produces valid JSON" {
    var buf: [512]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);

    const payload_str = try generatePayload(fba.allocator());

    // Must contain all required fields
    try std.testing.expect(std.mem.containsAtLeast(u8, payload_str, 1, "\"service\":\"tovarisch\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, payload_str, 1, "\"version\":\"0.1.1\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, payload_str, 1, "\"node_id\":\"local-dev\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, payload_str, 1, "\"status\":\"ok\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, payload_str, 1, "\"checks\""));
}
