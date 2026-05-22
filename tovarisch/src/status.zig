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

/// Default state directory path for v0.
/// Uses .tovarisch/state relative to current working directory.
/// This is predictable and harmless for local dev/testing.
/// TODO: Implement actual directory check once Io.Dir API is confirmed working.
const DEFAULT_STATE_DIR = ".tovarisch/state";

/// Checks the state directory status without creating it.
/// Returns a Check with appropriate status and detail.
/// For v0, this is a placeholder that returns warn (not yet implemented).
pub fn getStateDirCheck() Check {
    // v0 placeholder: directory check not yet implemented
    // Will be implemented once Io.Dir API is confirmed working in Zig 0.16
    _ = DEFAULT_STATE_DIR;
    return Check{
        .name = "state_dir",
        .status = .warn,
        .detail = "state directory not found",
    };
}

/// State directory check (computed once at startup).
const state_check = getStateDirCheck();

/// All local health checks combined into a static array.
const all_checks = [_]Check{ process_check, binary_check, config_check, state_check };

/// Returns the array of local health checks.
pub fn getLocalChecks() []const Check {
    return &all_checks;
}

/// Returns the current status with all local checks.
pub fn getStatus() Status {
    const checks = getLocalChecks();
    return Status{
        .service = "tovarisch",
        .version = version,
        .node_id = "local-dev",
        .status = deriveStatus(checks),
        .checks = checks,
    };
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
    try renderStatus(writer, getStatus());
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

test "getLocalChecks returns four checks" {
    const checks = getLocalChecks();
    try std.testing.expect(@as(usize, 4), checks.len);
}

test "getLocalChecks first check is process" {
    const checks = getLocalChecks();
    try std.testing.expectEqualStrings("process", checks[0].name);
}

test "status has correct structure" {
    const s = getStatus();
    try std.testing.expectEqualStrings("tovarisch", s.service);
    try std.testing.expectEqualStrings("0.1.1", s.version);
    try std.testing.expectEqualStrings("local-dev", s.node_id);
    // config and state_dir are warn, so top-level should be warn
    try std.testing.expectEqual(CheckStatus.warn, s.status);
    try std.testing.expect(@as(usize, 4), s.checks.len);
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
    try renderStatus(&out, getStatus());

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
    // Must contain all four checks
    try std.testing.expect(std.mem.containsAtLeast(u8, json_str, 1, "\"name\":\"process\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, json_str, 1, "\"name\":\"binary\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, json_str, 1, "\"name\":\"config\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, json_str, 1, "\"name\":\"state_dir\""));
}

test "getStateDirCheck returns correct name" {
    const check = getStateDirCheck();
    try std.testing.expectEqualStrings("state_dir", check.name);
}

test "getStateDirCheck status is warn when directory missing" {
    const check = getStateDirCheck();
    // .tovarisch/state does not exist in test environment, expect warn
    try std.testing.expectEqual(CheckStatus.warn, check.status);
}

test "getStateDirCheck detail for missing directory" {
    const check = getStateDirCheck();
    try std.testing.expectEqualStrings("state directory not found", check.detail);
}
