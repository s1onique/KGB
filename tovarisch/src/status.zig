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

pub const CheckStatus = enum {
    ok,
    warn,
    @"error",
};

pub const Check = struct {
    name: []const u8,
    status: CheckStatus,
    detail: []const u8,
};

pub const Status = struct {
    service: []const u8,
    version: []const u8,
    node_id: []const u8,
    status: CheckStatus,
    checks: []const Check,
};

// --- Status Derivation ---
//
// Top-level status is derived from child checks:
//   - any error => error
//   - else any warn => warn
//   - else ok

/// Derives the top-level status from an array of checks.
pub fn deriveStatus(checks: []const Check) CheckStatus {
    for (checks) |check| {
        if (check.status == .@"error") return .@"error";
    }
    for (checks) |check| {
        if (check.status == .warn) return .warn;
    }
    return .ok;
}

// --- Local Health Checks ---
//
// Static checks - no dynamic allocation needed for basic checks

const process_check = Check{
    .name = "process",
    .status = .ok,
    .detail = "running",
};

const binary_check = Check{
    .name = "binary",
    .status = .ok,
    .detail = "tovarisch",
};

const config_check = Check{
    .name = "config",
    .status = .warn,
    .detail = "not configured yet",
};

/// Returns the static array of local health checks.
pub fn getLocalChecks() []const Check {
    return &[_]Check{ process_check, binary_check, config_check };
}

// --- Built Status ---
//
// Pre-computed status using static checks.

const local_status: Status = .{
    .service = "tovarisch",
    .version = version,
    .node_id = "local-dev",
    .status = deriveStatus(getLocalChecks()),
    .checks = getLocalChecks(),
};

/// Returns the current status (static, using local checks).
pub fn getStatus() Status {
    return local_status;
}

// --- Payload construction ---

/// Renders the given status as JSON to the writer.
pub fn renderStatus(writer: anytype, s: Status) !void {
    var jw = std.json.Stringify{ .writer = writer };
    try jw.beginObject();
    try jw.objectField("service");
    try jw.write(s.service);
    try jw.objectField("version");
    try jw.write(s.version);
    try jw.objectField("node_id");
    try jw.write(s.node_id);
    try jw.objectField("status");
    try jw.write(@tagName(s.status));
    try jw.objectField("checks");
    try jw.beginArray();
    for (s.checks) |check| {
        try jw.beginObject();
        try jw.objectField("name");
        try jw.write(check.name);
        try jw.objectField("status");
        try jw.write(@tagName(check.status));
        try jw.objectField("detail");
        try jw.write(check.detail);
        try jw.endObject();
    }
    try jw.endArray();
    try jw.endObject();
}

/// Renders the current status payload to the given writer.
pub fn renderPayload(writer: anytype) !void {
    try renderStatus(writer, local_status);
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

// --- Tests ---

test "version constant is 0.1.1" {
    try std.testing.expectEqualStrings("0.1.1", version);
}

test "deriveStatus returns ok for all-ok checks" {
    const checks = [_]Check{
        .{ .name = "a", .status = .ok, .detail = "" },
        .{ .name = "b", .status = .ok, .detail = "" },
    };
    try std.testing.expectEqual(CheckStatus.ok, deriveStatus(&checks));
}

test "deriveStatus returns warn when any warn present" {
    const checks = [_]Check{
        .{ .name = "a", .status = .ok, .detail = "" },
        .{ .name = "b", .status = .warn, .detail = "" },
    };
    try std.testing.expectEqual(CheckStatus.warn, deriveStatus(&checks));
}

test "deriveStatus returns error when any error present" {
    const checks = [_]Check{
        .{ .name = "a", .status = .@"error", .detail = "" },
        .{ .name = "b", .status = .ok, .detail = "" },
    };
    try std.testing.expectEqual(CheckStatus.@"error", deriveStatus(&checks));
}

test "deriveStatus returns error even if warn also present" {
    const checks = [_]Check{
        .{ .name = "a", .status = .@"error", .detail = "" },
        .{ .name = "b", .status = .warn, .detail = "" },
    };
    try std.testing.expectEqual(CheckStatus.@"error", deriveStatus(&checks));
}

test "deriveStatus returns ok for empty checks" {
    const checks: [0]Check = .{};
    try std.testing.expectEqual(CheckStatus.ok, deriveStatus(&checks));
}

test "getLocalChecks returns three checks" {
    const checks = getLocalChecks();
    try std.testing.expect(@as(usize, 3), checks.len);
}

test "getLocalChecks first check is process" {
    const checks = getLocalChecks();
    try std.testing.expectEqualStrings("process", checks[0].name);
}

test "local_status has correct structure" {
    try std.testing.expectEqualStrings("tovarisch", local_status.service);
    try std.testing.expectEqualStrings("0.1.1", local_status.version);
    try std.testing.expectEqualStrings("local-dev", local_status.node_id);
    // config is warn, so top-level should be warn
    try std.testing.expectEqual(CheckStatus.warn, local_status.status);
    try std.testing.expect(@as(usize, 3), local_status.checks.len);
}

test "getStatus returns local_status" {
    const s = getStatus();
    try std.testing.expectEqualStrings("tovarisch", s.service);
}

test "parseStatus parses valid JSON" {
    var buf: [1024]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const allocator = fba.allocator();

    const json = "{\"service\":\"tovarisch\",\"version\":\"0.1.1\",\"node_id\":\"local-dev\",\"status\":\"warn\",\"checks\":[{\"name\":\"test\",\"status\":\"ok\",\"detail\":\"hi\"}]}";
    const parsed = try parseStatus(json, allocator);
    try std.testing.expectEqualStrings("tovarisch", parsed.service);
    try std.testing.expectEqualStrings("0.1.1", parsed.version);
    try std.testing.expectEqualStrings("local-dev", parsed.node_id);
    try std.testing.expectEqual(CheckStatus.warn, parsed.status);
    try std.testing.expect(@as(usize, 1), parsed.checks.len);
}

test "parseStatus returns error for invalid JSON" {
    var buf: [256]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const allocator = fba.allocator();

    const bad = "{invalid json}";
    try std.testing.expectError(error.InvalidJSON, parseStatus(bad, allocator));
}

test "renderStatus produces valid JSON" {
    var buf: [2048]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const allocator = fba.allocator();

    var out = std.Io.Writer.Allocating.init(allocator);
    defer out.deinit();
    try renderStatus(&out, local_status);

    const json_str = try out.written();
    // Validate it parses back
    const parsed = try parseStatus(json_str, allocator);
    try std.testing.expectEqualStrings("tovarisch", parsed.service);
}

test "renderPayload produces valid JSON with all checks" {
    var buf: [2048]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const allocator = fba.allocator();

    var out = std.Io.Writer.Allocating.init(allocator);
    defer out.deinit();
    try renderPayload(&out);

    const json_str = try out.written();

    // Must contain all required fields
    try std.testing.expect(std.mem.containsAtLeast(u8, json_str, 1, "\"service\":\"tovarisch\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, json_str, 1, "\"version\":\"0.1.1\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, json_str, 1, "\"node_id\":\"local-dev\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, json_str, 1, "\"checks\""));
    // Must contain all three checks
    try std.testing.expect(std.mem.containsAtLeast(u8, json_str, 1, "\"name\":\"process\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, json_str, 1, "\"name\":\"binary\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, json_str, 1, "\"name\":\"config\""));
}
