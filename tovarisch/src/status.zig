// status.zig — Status payload rendering for tovarisch
//
// Renders the v0 status JSON payload for `tovarisch status --json`.
// Uses injectable checks for testability:
// - getLocalChecks() returns all local health checks
// - getStatus() builds the full status payload
//
// Check ordering (stable for operator readability):
//   1. process  - daemon is running
//   2. binary   - binary name is correct
//   3. config   - configuration state
//   4. state_dir - state directory exists
//   5. http     - HTTP service route available
//   6. tunnel   - tunnel interface presence

const std = @import("std");
const telemetry = @import("runtime/telemetry.zig");
const tunnel_check = @import("tunnel_check.zig");

pub const version = "0.1.1";

/// Default state directory path relative to working directory.
pub const DEFAULT_STATE_DIR = ".tovarisch/state";

// --- Canonical Status JSON Schema ---
//
// Minimal contract for `tovarisch status --json`.

pub const CheckStatus = enum {
    ok,
    warn,
    @"error",
    unknown,
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

/// Performs a filesystem check for the given path.
/// This is the core filesystem observation logic.
/// Does not create, delete, or mutate filesystem state.
///
/// Uses opendir() to differentiate:
/// - opendir succeeds → path is a directory → ok
/// - opendir fails with ENOENT/ENOTDIR → path does not exist → warn
/// - opendir fails with other error → inaccessible → unknown
pub fn getStateDirCheckForPath(path: []const u8) Check {
    var path_buf: [4096]u8 = undefined;
    const c_path_result = toCString(path, &path_buf);
    if (c_path_result) |c_path| {
        // Try to open as directory
        const dir = std.c.opendir(c_path);
        if (dir) |d| {
            // Path exists and is a directory
            _ = std.c.closedir(d);
            return Check{
                .name = "state_dir",
                .status = .ok,
                .detail = "state directory ready",
            };
        }

        // opendir failed - check the errno to determine the failure type
        const errno = std.c._errno().*;
        const e_noent = @intFromEnum(std.c.E.NOENT);
        const e_notdir = @intFromEnum(std.c.E.NOTDIR);
        if (errno == e_noent or errno == e_notdir) {
            // Path does not exist (ENOENT) or exists but is not a directory (ENOTDIR)
            return Check{
                .name = "state_dir",
                .status = .warn,
                .detail = "state directory not found",
            };
        }

        // Unexpected error (permission denied, I/O error, etc.)
        return Check{
            .name = "state_dir",
            .status = .unknown,
            .detail = "state directory inaccessible",
        };
    } else {
        // Path too long
        return Check{
            .name = "state_dir",
            .status = .unknown,
            .detail = "state directory inaccessible",
        };
    }
}

/// Default state directory check using DEFAULT_STATE_DIR.
pub fn getStateDirCheck() Check {
    return getStateDirCheckForPath(DEFAULT_STATE_DIR);
}

/// Converts a Zig slice to a null-terminated C string.
pub fn toCString(path: []const u8, buf: *[4096]u8) ?[*:0]const u8 {
    if (path.len >= buf.len) return null;
    @memcpy(buf[0..path.len], path);
    buf[path.len] = 0;
    return @as([*:0]const u8, @ptrCast(buf));
}

/// Static buffer for local checks. Ensures stable memory addresses.
var local_checks_buf: [6]Check = undefined;

pub fn getLocalChecks() []const Check {
    local_checks_buf[0] = process_check;
    local_checks_buf[1] = binary_check;
    local_checks_buf[2] = config_check;
    local_checks_buf[3] = getStateDirCheck();
    local_checks_buf[4] = http_check;
    local_checks_buf[5] = tunnel_check.getTunnelCheckDefault();
    return &local_checks_buf;
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
fn renderStatus(writer: anytype, s: Status) !void {
    try writer.writeAll("{\"service\":\"");
    try writer.writeAll(s.service);
    try writer.writeAll("\",\"version\":\"");
    try writer.writeAll(s.version);
    try writer.writeAll("\",\"node_id\":\"");
    try writer.writeAll(s.node_id);
    try writer.writeAll("\",\"status\":\"");
    try writer.writeAll(@tagName(s.status));
    try writer.writeAll("\",\"checks\":[");

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

test "getLocalChecks returns six checks" {
    const checks = getLocalChecks();
    try std.testing.expectEqual(@as(usize, 6), checks.len);
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
    try std.testing.expect(s.status == .ok or s.status == .warn or s.status == .@"error");
    try std.testing.expectEqual(@as(usize, 6), s.checks.len);
}

test "getStateDirCheck returns correct name" {
    const check = getStateDirCheck();
    try std.testing.expectEqualStrings("state_dir", check.name);
}

test "status JSON contains all required top-level fields" {
    const s = getStatus();
    try std.testing.expectEqualStrings("tovarisch", s.service);
    try std.testing.expectEqualStrings("0.1.1", s.version);
    try std.testing.expectEqualStrings("local-dev", s.node_id);
    try std.testing.expect(s.status == .ok or s.status == .warn or s.status == .@"error");
    try std.testing.expect(s.checks.len > 0);
}

test "status JSON contains all six checks including tunnel" {
    const checks = getLocalChecks();

    var has_process = false;
    var has_binary = false;
    var has_config = false;
    var has_state_dir = false;
    var has_http = false;
    var has_tunnel = false;

    for (checks) |check| {
        if (std.mem.eql(u8, check.name, "process")) has_process = true;
        if (std.mem.eql(u8, check.name, "binary")) has_binary = true;
        if (std.mem.eql(u8, check.name, "config")) has_config = true;
        if (std.mem.eql(u8, check.name, "state_dir")) has_state_dir = true;
        if (std.mem.eql(u8, check.name, "http")) has_http = true;
        if (std.mem.eql(u8, check.name, "tunnel")) has_tunnel = true;
    }

    try std.testing.expect(has_process);
    try std.testing.expect(has_binary);
    try std.testing.expect(has_config);
    try std.testing.expect(has_state_dir);
    try std.testing.expect(has_http);
    try std.testing.expect(has_tunnel);
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

test "renderPayload output contains checks array" {
    var w = TestWriter.init();
    try renderPayload(&w);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"checks\":["));
}

test "renderPayload output contains all six check names" {
    var w = TestWriter.init();
    try renderPayload(&w);

    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"process\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"binary\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"config\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"state_dir\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"http\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"tunnel\""));
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
    try std.testing.expect(s.runtime.rss_kib == null or s.runtime.rss_kib.? >= 0);
}
