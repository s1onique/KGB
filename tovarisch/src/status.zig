const std = @import("std");
const telemetry = @import("runtime/telemetry.zig");

pub const version = "0.1.1";

// --- Canonical Status JSON Schema ---
//
// Minimal contract for `tovarisch status --json`.

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
    runtime: telemetry.RuntimeTelemetry,
};

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

const http_check = Check{
    .name = "http",
    .status = .ok,
    .detail = "http service route available",
};

pub fn getStateDirCheck() Check {
    return Check{
        .name = "state_dir",
        .status = .warn,
        .detail = "state directory not found",
    };
}

const state_check = getStateDirCheck();
const all_checks = [_]Check{ process_check, binary_check, config_check, state_check, http_check };

pub fn getLocalChecks() []const Check {
    return &all_checks;
}

pub fn getStatus() Status {
    const checks = getLocalChecks();
    return Status{
        .service = "tovarisch",
        .version = version,
        .node_id = "local-dev",
        .status = deriveStatus(checks),
        .checks = checks,
        .runtime = telemetry.getRuntimeTelemetry(),
    };
}

/// Renders the current status payload to the given writer.
pub fn renderPayload(writer: anytype) !void {
    try renderStatus(writer, getStatus());
}

/// Renders the given status as JSON to the writer.
/// NOTE: This manual JSON construction does not escape special characters.
/// All current values are static strings, so this is safe for now.
fn renderStatus(writer: anytype, s: Status) !void {
    // Build header using writeAll for safety
    try writer.writeAll("{\"service\":\"");
    try writer.writeAll(s.service);
    try writer.writeAll("\",\"version\":\"");
    try writer.writeAll(s.version);
    try writer.writeAll("\",\"node_id\":\"");
    try writer.writeAll(s.node_id);
    try writer.writeAll("\",\"status\":\"");
    try writer.writeAll(@tagName(s.status));
    try writer.writeAll("\",\"checks\":[");

    // Render each check
    for (s.checks, 0..) |check, i| {
        if (i > 0) try writer.writeAll(",");
        try writer.writeAll("{\"name\":\"");
        try writer.writeAll(check.name);
        try writer.writeAll("\",\"status\":\"");
        try writer.writeAll(@tagName(check.status));
        try writer.writeAll("\",\"detail\":\"");
        try writer.writeAll(check.detail);
        try writer.writeAll("\"}");
    }

    // Render runtime block
    try writer.writeAll("],\"runtime\":{\"pid\":");
    try writer.print("{d}", .{s.runtime.pid});
    if (s.runtime.rss_kib) |rss| {
        try writer.writeAll(",\"rss_kib\":");
        try writer.print("{d}", .{rss});
    } else {
        try writer.writeAll(",\"rss_kib\":null");
    }
    try writer.writeAll("}}\n");
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

test "getLocalChecks returns five checks" {
    const checks = getLocalChecks();
    try std.testing.expectEqual(@as(usize, 5), checks.len);
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
    try std.testing.expectEqual(CheckStatus.warn, s.status);
    try std.testing.expectEqual(@as(usize, 5), s.checks.len);
}

test "getStateDirCheck returns correct name" {
    const check = getStateDirCheck();
    try std.testing.expectEqualStrings("state_dir", check.name);
}

test "getStateDirCheck status is warn" {
    const check = getStateDirCheck();
    try std.testing.expectEqual(CheckStatus.warn, check.status);
}

// --- JSON Contract Tests ---
// These tests verify the status JSON contract structure.
// Actual JSON parseability is verified by verify_status_json.sh in the gate.

test "status JSON contains all required top-level fields" {
    // Verify that getStatus() returns correct field values
    // which are rendered as JSON by renderPayload()
    const s = getStatus();
    try std.testing.expectEqualStrings("tovarisch", s.service);
    try std.testing.expectEqualStrings("0.1.1", s.version);
    try std.testing.expectEqualStrings("local-dev", s.node_id);
    // status is "warn" because config and state_dir are warn
    try std.testing.expectEqual(CheckStatus.warn, s.status);
    try std.testing.expect(s.checks.len > 0);
}

test "status JSON contains all five checks" {
    // Verify all check names that should appear in JSON output
    const checks = getLocalChecks();

    var has_process = false;
    var has_binary = false;
    var has_config = false;
    var has_state_dir = false;
    var has_http = false;

    for (checks) |check| {
        if (std.mem.eql(u8, check.name, "process")) has_process = true;
        if (std.mem.eql(u8, check.name, "binary")) has_binary = true;
        if (std.mem.eql(u8, check.name, "config")) has_config = true;
        if (std.mem.eql(u8, check.name, "state_dir")) has_state_dir = true;
        if (std.mem.eql(u8, check.name, "http")) has_http = true;
    }

    try std.testing.expect(has_process);
    try std.testing.expect(has_binary);
    try std.testing.expect(has_config);
    try std.testing.expect(has_state_dir);
    try std.testing.expect(has_http);
}

test "state_dir check has correct detail" {
    const check = getStateDirCheck();
    try std.testing.expectEqualStrings("state directory not found", check.detail);
}

test "config check has warn status" {
    const checks = getLocalChecks();
    for (checks) |check| {
        if (std.mem.eql(u8, check.name, "config")) {
            try std.testing.expectEqual(CheckStatus.warn, check.status);
        }
    }
}

// TestWriter: fixed-buffer writer for testing renderPayload output.
const TestWriter = struct {
    const Self = @This();
    const BufSize = 4096;

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
        @memcpy(self.buf[self.len..][0..bytes.len], bytes);
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
};

// --- Rendered output tests ---
// These tests verify that renderPayload() produces correct JSON output.

test "renderPayload output contains service:tovarisch" {
    var w = TestWriter.init();
    try renderPayload(&w);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"service\":\"tovarisch\""));
}

test "renderPayload output contains version:0.1.1" {
    var w = TestWriter.init();
    try renderPayload(&w);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"version\":\"0.1.1\""));
}

test "renderPayload output contains node_id:local-dev" {
    var w = TestWriter.init();
    try renderPayload(&w);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"node_id\":\"local-dev\""));
}

test "renderPayload output contains status:warn" {
    var w = TestWriter.init();
    try renderPayload(&w);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"status\":\"warn\""));
}

test "renderPayload output contains checks array" {
    var w = TestWriter.init();
    try renderPayload(&w);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"checks\":["));
}

test "renderPayload output contains all five check names" {
    var w = TestWriter.init();
    try renderPayload(&w);

    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"process\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"binary\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"config\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"state_dir\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"http\""));
}

test "renderPayload output contains runtime block" {
    var w = TestWriter.init();
    try renderPayload(&w);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"runtime\":{"));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"pid\":"));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"rss_kib\":"));
}

test "getStatus includes runtime telemetry" {
    const s = getStatus();
    try std.testing.expect(s.runtime.pid > 0);
    // rss_kib can be null on non-Linux platforms or valid on Linux
    try std.testing.expect(s.runtime.rss_kib == null or s.runtime.rss_kib.? >= 0);
}
