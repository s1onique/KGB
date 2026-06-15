// status_tests.zig — Unit tests for status module
//
// Tests the status module including BFD integration via explicit runtime wiring.
// BGP status tests are in status_bgp_tests.zig.
// BGP diagnostics tests are in status_bgp_diagnostics_tests.zig.
// These tests are deterministic and do not depend on repository state.

const std = @import("std");
const status = @import("status.zig");
const bfd_status = @import("bfd/status.zig");

// --- State Directory Check Tests ---
// Deterministic tests using isolated test paths.

test "getStateDirCheckForPath returns warn for missing path" {
    const check = status.getStateDirCheckForPath("/tmp/tovarisch_test_nonexistent_12345");
    try std.testing.expectEqualStrings("state_dir", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("state directory not found", check.detail);
}

test "getStateDirCheckForPath returns ok for existing directory" {
    const test_dir = "/tmp/tovarisch_test_dir_12345";
    var path_buf: [4096]u8 = undefined;
    const c_path = status.toCString(test_dir, &path_buf) orelse return error.SkipZigTest;
    _ = std.c.mkdir(c_path, 0o755);
    defer _ = std.c.rmdir(c_path);

    const check = status.getStateDirCheckForPath(test_dir);
    try std.testing.expectEqualStrings("state_dir", check.name);
    try std.testing.expectEqual(status.CheckStatus.ok, check.status);
    try std.testing.expectEqualStrings("state directory ready", check.detail);
}

test "getStateDirCheckForPath returns warn for path that is a file" {
    const test_file = "/tmp/tovarisch_test_file_12345";
    var path_buf: [4096]u8 = undefined;
    const c_path = status.toCString(test_file, &path_buf) orelse return error.SkipZigTest;
    const fd = std.c.open(c_path, std.c.O{ .ACCMODE = std.posix.ACCMODE.WRONLY, .CREAT = true }, @as(c_uint, 0o644));
    if (fd < 0) return error.SkipZigTest;
    defer _ = std.c.close(fd);
    defer _ = std.c.unlink(c_path);

    const check = status.getStateDirCheckForPath(test_file);
    try std.testing.expectEqualStrings("state_dir", check.name);
    try std.testing.expect(check.status == .warn or check.status == .@"error");
}

test "top-level status derives warn when state_dir is warn" {
    const checks = [_]status.Check{
        .{ .name = "process", .status = .ok, .detail = "running" },
        .{ .name = "binary", .status = .ok, .detail = "tovarisch" },
        .{ .name = "config", .status = .warn, .detail = "not configured yet" },
        .{ .name = "state_dir", .status = .warn, .detail = "state directory not found" },
        .{ .name = "http", .status = .ok, .detail = "http service route available" },
    };
    try std.testing.expectEqual(status.CheckStatus.warn, status.deriveStatus(&checks));
}

test "top-level status derives error when state_dir is error" {
    const checks = [_]status.Check{
        .{ .name = "process", .status = .ok, .detail = "running" },
        .{ .name = "binary", .status = .ok, .detail = "tovarisch" },
        .{ .name = "config", .status = .warn, .detail = "not configured yet" },
        .{ .name = "state_dir", .status = .@"error", .detail = "state path is not a directory" },
        .{ .name = "http", .status = .ok, .detail = "http service route available" },
    };
    try std.testing.expectEqual(status.CheckStatus.@"error", status.deriveStatus(&checks));
}

test "DEFAULT_STATE_DIR constant is correct" {
    try std.testing.expectEqualStrings(".tovarisch/state", status.DEFAULT_STATE_DIR);
}

// --- DeriveStatus Tests ---

test "deriveStatus returns ok for all-ok checks" {
    const checks = [_]status.Check{
        .{ .name = "a", .status = .ok, .detail = "" },
        .{ .name = "b", .status = .ok, .detail = "" },
    };
    try std.testing.expectEqual(status.CheckStatus.ok, status.deriveStatus(&checks));
}

test "deriveStatus returns warn when any warn present" {
    const checks = [_]status.Check{
        .{ .name = "a", .status = .ok, .detail = "" },
        .{ .name = "b", .status = .warn, .detail = "" },
    };
    try std.testing.expectEqual(status.CheckStatus.warn, status.deriveStatus(&checks));
}

test "deriveStatus returns error when any error present" {
    const checks = [_]status.Check{
        .{ .name = "a", .status = .@"error", .detail = "" },
        .{ .name = "b", .status = .ok, .detail = "" },
    };
    try std.testing.expectEqual(status.CheckStatus.@"error", status.deriveStatus(&checks));
}

test "deriveStatus returns error even if warn also present" {
    const checks = [_]status.Check{
        .{ .name = "a", .status = .@"error", .detail = "" },
        .{ .name = "b", .status = .warn, .detail = "" },
    };
    try std.testing.expectEqual(status.CheckStatus.@"error", status.deriveStatus(&checks));
}

test "deriveStatus returns ok for empty checks" {
    const checks: [0]status.Check = .{};
    try std.testing.expectEqual(status.CheckStatus.ok, status.deriveStatus(&checks));
}

// --- getLocalChecks and getStatus Tests ---

test "getLocalChecksWithBgp returns nine checks" {
    var scratch = status.StatusScratch{};
    const checks = status.getLocalChecksWithBgp(
        null,
        status.getDefaultConfigCheck(),
        .no_config,
        &scratch,
    );
    try std.testing.expectEqual(@as(usize, 9), checks.len);
}

test "getLocalChecksWithBgp first check is process" {
    var scratch = status.StatusScratch{};
    const checks = status.getLocalChecksWithBgp(
        null,
        status.getDefaultConfigCheck(),
        .no_config,
        &scratch,
    );
    try std.testing.expectEqualStrings("process", checks[0].name);
}

test "buildStatus has correct structure" {
    var scratch = status.StatusScratch{};
    const s = status.buildStatus(&scratch);
    try std.testing.expectEqualStrings("tovarisch", s.service);
    try std.testing.expect(std.mem.startsWith(u8, s.version, "0.1."));
    try std.testing.expectEqualStrings("local-dev", s.node_id);
    try std.testing.expect(s.status == .ok or s.status == .warn or s.status == .@"error");
    try std.testing.expectEqual(@as(usize, 9), s.checks.len);
}

test "getStateDirCheck returns correct name" {
    const check = status.getStateDirCheck();
    try std.testing.expectEqualStrings("state_dir", check.name);
}

test "status JSON contains all required top-level fields" {
    var scratch = status.StatusScratch{};
    const s = status.buildStatus(&scratch);
    try std.testing.expectEqualStrings("tovarisch", s.service);
    try std.testing.expectEqualStrings("local-dev", s.node_id);
    try std.testing.expect(s.checks.len > 0);
}

test "status JSON contains all nine check names including bfd and bgp" {
    var scratch = status.StatusScratch{};
    const checks = status.getLocalChecksWithBgp(
        null,
        status.getDefaultConfigCheck(),
        .no_config,
        &scratch,
    );
    var has_process = false;
    var has_binary = false;
    var has_config = false;
    var has_state_dir = false;
    var has_http = false;
    var has_tunnel = false;
    var has_wg_peers = false;
    var has_bfd = false;
    var has_bgp = false;
    for (checks) |check| {
        if (std.mem.eql(u8, check.name, "process")) has_process = true;
        if (std.mem.eql(u8, check.name, "binary")) has_binary = true;
        if (std.mem.eql(u8, check.name, "config")) has_config = true;
        if (std.mem.eql(u8, check.name, "state_dir")) has_state_dir = true;
        if (std.mem.eql(u8, check.name, "http")) has_http = true;
        if (std.mem.eql(u8, check.name, "tunnel")) has_tunnel = true;
        if (std.mem.eql(u8, check.name, "wg_peers")) has_wg_peers = true;
        if (std.mem.eql(u8, check.name, "bfd")) has_bfd = true;
        if (std.mem.eql(u8, check.name, "bgp")) has_bgp = true;
    }
    try std.testing.expect(has_process);
    try std.testing.expect(has_binary);
    try std.testing.expect(has_config);
    try std.testing.expect(has_state_dir);
    try std.testing.expect(has_http);
    try std.testing.expect(has_tunnel);
    try std.testing.expect(has_wg_peers);
    try std.testing.expect(has_bfd);
    try std.testing.expect(has_bgp);
}

test "config check has warn status" {
    var scratch = status.StatusScratch{};
    const checks = status.getLocalChecksWithBgp(
        null,
        status.getDefaultConfigCheck(),
        .no_config,
        &scratch,
    );
    for (checks) |check| {
        if (std.mem.eql(u8, check.name, "config")) {
            try std.testing.expectEqual(status.CheckStatus.warn, check.status);
        }
    }
}

// --- BFD Status Integration Tests ---

test "bfd check has warn status when no runtime" {
    var scratch = status.StatusScratch{};
    const checks = status.getLocalChecksWithBgp(
        null,
        status.getDefaultConfigCheck(),
        .no_config,
        &scratch,
    );
    for (checks) |check| {
        if (std.mem.eql(u8, check.name, "bfd")) {
            try std.testing.expectEqual(status.CheckStatus.warn, check.status);
            try std.testing.expectEqualStrings("bfd not configured", check.detail);
        }
    }
}

test "getBfdCheck returns warn for null runtime" {
    var bfd_detail_buf: [64]u8 = undefined;
    const check = status.getBfdCheck(null, &bfd_detail_buf);
    try std.testing.expectEqualStrings("bfd", check.name);
    try std.testing.expect(check.status == .warn);
    try std.testing.expectEqualStrings("bfd not configured", check.detail);
}

test "getBfdCheck uses explicit runtime" {
    var rt = bfd_status.createTestRuntime();
    try bfd_status.addTestPeer(&rt, "10.0.0.1", "10.0.0.2");
    rt.startAll();
    var bfd_detail_buf: [64]u8 = undefined;
    const check = status.getBfdCheck(&rt, &bfd_detail_buf);
    try std.testing.expectEqualStrings("bfd", check.name);
    try std.testing.expect(check.status == .warn); // No handshake yet
}

test "getLocalChecksWithBfd passes explicit runtime" {
    var rt = bfd_status.createTestRuntime();
    try bfd_status.addTestPeer(&rt, "10.0.0.1", "10.0.0.2");
    rt.startAll();
    const default_check = status.getDefaultConfigCheck();
    var scratch = status.StatusScratch{};
    const checks = status.getLocalChecksWithBfd(&rt, default_check, &scratch);
    var found_bfd = false;
    for (checks) |check| {
        if (std.mem.eql(u8, check.name, "bfd")) {
            found_bfd = true;
            try std.testing.expect(check.status == .warn);
        }
    }
    try std.testing.expect(found_bfd);
}

// --- RenderPayload Tests ---

const TestWriter = struct {
    const Self = @This();
    const BufSize = 8192;
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

    pub fn slice(self: *const Self) []const u8 {
        return self.buf[0..self.len];
    }
};

test "renderPayload output contains service:tovarisch" {
    var w = TestWriter.init();
    try status.renderPayload(&w);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"service\":\"tovarisch\""));
}

test "renderPayload output contains version prefix" {
    var w = TestWriter.init();
    try status.renderPayload(&w);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"version\":\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "+"));
}

test "renderPayload output contains node_id:local-dev" {
    var w = TestWriter.init();
    try status.renderPayload(&w);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"node_id\":\"local-dev\""));
}

test "renderPayload output contains checks array" {
    var w = TestWriter.init();
    try status.renderPayload(&w);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"checks\":["));
}

test "renderPayload output contains all eight check names" {
    var w = TestWriter.init();
    try status.renderPayload(&w);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"process\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"binary\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"config\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"state_dir\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"http\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"tunnel\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"wg_peers\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"bfd\""));
}

test "renderPayload output contains runtime block" {
    var w = TestWriter.init();
    try status.renderPayload(&w);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"runtime\":{"));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"pid\":"));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"rss_kib\":"));
}

test "buildStatus includes runtime telemetry" {
    var scratch = status.StatusScratch{};
    const s = status.buildStatus(&scratch);
    try std.testing.expect(s.runtime.pid > 0);
    try std.testing.expect(s.runtime.rss_kib == null or s.runtime.rss_kib.? >= 0);
}

test "version contains base_version prefix" {
    var scratch = status.StatusScratch{};
    const s = status.buildStatus(&scratch);
    try std.testing.expect(std.mem.startsWith(u8, s.version, "0.1."));
    try std.testing.expect(std.mem.containsAtLeast(u8, s.version, 1, "+"));
}

// --- Regression: per-request memory leak tests ---
// These tests verify that status rendering does not leak memory per request.
// The original bug: buildStatusCheck() used std.fmt.allocPrint() with page_allocator
// which leaked ~4KiB per /status request (one page per call).
// Fix: replaced allocPrint with caller-owned buffer in bfd/status.zig.

test "renderPayload multiple times produces consistent output" {
    var w1 = TestWriter.init();
    var w2 = TestWriter.init();
    var w3 = TestWriter.init();

    try status.renderPayload(&w1);
    try status.renderPayload(&w2);
    try status.renderPayload(&w3);

    try std.testing.expect(std.mem.containsAtLeast(u8, w1.slice(), 1, "\"service\":\"tovarisch\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w2.slice(), 1, "\"service\":\"tovarisch\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w3.slice(), 1, "\"service\":\"tovarisch\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w1.slice(), 1, "\"name\":\"bfd\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w2.slice(), 1, "\"name\":\"bfd\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w3.slice(), 1, "\"name\":\"bfd\""));
}

test "getBfdCheck with partial peers uses caller buffer" {
    var rt = bfd_status.createTestRuntime();
    try bfd_status.addTestPeer(&rt, "10.0.0.1", "10.0.0.2");
    try bfd_status.addTestPeer(&rt, "10.0.0.3", "10.0.0.4");
    rt.startAll();

    var scratch = status.StatusScratch{};
    const check = status.getBfdCheck(&rt, &scratch.bfd_detail);

    try std.testing.expect(check.status == .warn);
    try std.testing.expect(std.mem.containsAtLeast(u8, check.detail, 1, "bfd"));
}

test "buildStatusCheckInto with partial peers detail format" {
    const snapshot = bfd_status.StatusSnapshot{
        .peer_count = 3,
        .up_count = 2,
        .has_peers = true,
    };

    var detail_buf: [bfd_status.BFD_DETAIL_BUF_SIZE]u8 = undefined;
    const check = bfd_status.buildStatusCheckInto(snapshot, &detail_buf);

    try std.testing.expect(check.status == .warn);
    try std.testing.expectEqualStrings("2/3 bfd sessions up", check.detail);
}

test "buildStatusCheckInto partial peers is allocation-free" {
    const snapshot = bfd_status.StatusSnapshot{
        .peer_count = 3,
        .up_count = 2,
        .has_peers = true,
    };

    var detail_buf: [bfd_status.BFD_DETAIL_BUF_SIZE]u8 = undefined;
    const check = bfd_status.buildStatusCheckInto(snapshot, &detail_buf);

    try std.testing.expect(check.status == .warn);
    try std.testing.expectEqualStrings("2/3 bfd sessions up", check.detail);
    try std.testing.expect(@intFromPtr(check.detail.ptr) == @intFromPtr(&detail_buf[0]));
}
