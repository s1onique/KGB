// status_ownership_tests.zig — Ownership and reentrancy tests for status rendering
//
// These tests verify that status rendering is fully reentrant and safe for
// future threaded HTTP serving. Key invariants tested:
//
// 1. Two independent render contexts do NOT alias mutable detail buffers
// 2. Repeated rendering does NOT mutate previously rendered detail slices
// 3. BFD and BGP dynamic detail rendering uses caller-owned scratch
// 4. No production status rendering path uses module-level mutable buffers
//
// The original bugs this tests against:
// - buildStatusCheck() used static module-level buffer (leaked between calls)
// - local_checks_buf was module-level mutable (not thread-safe)
// - page_allocator used in request path (memory leaks per request)

const std = @import("std");
const status = @import("status.zig");
const bfd_status = @import("bfd/status.zig");
const bgp_status = @import("bgp/status.zig");
const status_checks = @import("status_checks.zig");
const wg_boundary = @import("net/wg_status_boundary.zig");

// ============================================================================
// Test helpers
// ============================================================================

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
        // Use for loop instead of @memcpy to avoid aliasing panic in Zig 0.16
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
};

// ============================================================================
// BFD detail buffer ownership tests
// ============================================================================

test "buildStatusCheckInto uses caller-owned buffer for partial peers" {
    // Test that buildStatusCheckInto writes to caller's buffer, not a static
    const snapshot = bfd_status.StatusSnapshot{
        .peer_count = 3,
        .up_count = 2,
        .has_peers = true,
    };

    var caller_buf: [64]u8 = undefined;
    const check = bfd_status.buildStatusCheckInto(snapshot, &caller_buf);

    try std.testing.expect(check.status == .warn);
    try std.testing.expectEqualStrings("2/3 bfd sessions up", check.detail);
    // CRITICAL: Verify detail points to our buffer, not some static
    try std.testing.expect(@intFromPtr(check.detail.ptr) == @intFromPtr(&caller_buf[0]));
}

test "two BFD scratch buffers are independent (no aliasing)" {
    // Test that two separate scratch buffers don't alias
    // When all peers are up, the detail is a static string, not a buffer pointer.
    const snapshot1 = bfd_status.StatusSnapshot{
        .peer_count = 3,
        .up_count = 2,
        .has_peers = true,
    };
    const snapshot2 = bfd_status.StatusSnapshot{
        .peer_count = 5,
        .up_count = 5,
        .has_peers = true,
    };

    var buf1: [64]u8 = undefined;
    var buf2: [64]u8 = undefined;

    const check1 = bfd_status.buildStatusCheckInto(snapshot1, &buf1);
    const check2 = bfd_status.buildStatusCheckInto(snapshot2, &buf2);

    // Verify check1 points to our buffer (partial peers need dynamic formatting)
    try std.testing.expect(@intFromPtr(check1.detail.ptr) == @intFromPtr(&buf1[0]));

    // check2.detail is a static string when all peers are up (no dynamic formatting needed)
    // This is expected behavior - static strings are safe and don't require buffer storage

    // Verify content is correct for each snapshot
    try std.testing.expectEqualStrings("2/3 bfd sessions up", check1.detail);
    try std.testing.expectEqualStrings("bfd sessions up", check2.detail);
}

test "repeated BFD renders with partial peers don't corrupt previous output" {
    // Test that rendering the same snapshot multiple times is consistent
    const snapshot = bfd_status.StatusSnapshot{
        .peer_count = 4,
        .up_count = 2,
        .has_peers = true,
    };

    var buf1: [64]u8 = undefined;
    var buf2: [64]u8 = undefined;
    var buf3: [64]u8 = undefined;

    const check1 = bfd_status.buildStatusCheckInto(snapshot, &buf1);
    const check2 = bfd_status.buildStatusCheckInto(snapshot, &buf2);
    const check3 = bfd_status.buildStatusCheckInto(snapshot, &buf3);

    // All should produce the same output
    try std.testing.expectEqualStrings(check1.detail, check2.detail);
    try std.testing.expectEqualStrings(check2.detail, check3.detail);
    try std.testing.expectEqualStrings("2/4 bfd sessions up", check1.detail);

    // Verify each points to its own buffer
    try std.testing.expect(@intFromPtr(check1.detail.ptr) == @intFromPtr(&buf1[0]));
    try std.testing.expect(@intFromPtr(check2.detail.ptr) == @intFromPtr(&buf2[0]));
    try std.testing.expect(@intFromPtr(check3.detail.ptr) == @intFromPtr(&buf3[0]));
}

// ============================================================================
// BGP detail buffer ownership tests
// ============================================================================

test "buildBgpCheckInto uses caller-owned buffer for configured case" {
    const state = bgp_status.BgpStatusState{
        .configured = .{
            .configured_prefix_count = 3,
            .updates_sent = 1,
            .nlri_sent_count = 3,
            .fsm_state = .established,
            .peer_address = .{ 10, 0, 0, 2 },
            .peer_as = 65002,
            .local_as = 65001,
            .last_error = null,
            .messages_sent = 5,
            .messages_received = 4,
            .keepalives_sent = 2,
            .keepalives_received = 2,
            .passive_listener_state = .disabled,
            .passive_listener_error = null,
        },
    };

    var caller_buf: [64]u8 = undefined;
    const check = bgp_status.buildBgpCheckInto(state, &caller_buf);

    try std.testing.expect(check.status == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, check.detail, 1, "3 configured"));
    // CRITICAL: Verify detail points to our buffer
    try std.testing.expect(@intFromPtr(check.detail.ptr) == @intFromPtr(&caller_buf[0]));
}

test "two BGP scratch buffers are independent (no aliasing)" {
    const state1 = bgp_status.BgpStatusState{
        .configured = .{
            .configured_prefix_count = 10,
            .updates_sent = 0,
            .nlri_sent_count = 0,
            .fsm_state = .open_sent,
            .peer_address = .{ 10, 0, 0, 2 },
            .peer_as = 65002,
            .local_as = 65001,
            .last_error = null,
            .messages_sent = 1,
            .messages_received = 0,
            .keepalives_sent = 0,
            .keepalives_received = 0,
            .passive_listener_state = .disabled,
            .passive_listener_error = null,
        },
    };
    const state2 = bgp_status.BgpStatusState{
        .configured = .{
            .configured_prefix_count = 1,
            .updates_sent = 0,
            .nlri_sent_count = 0,
            .fsm_state = .established,
            .peer_address = .{ 10, 0, 0, 2 },
            .peer_as = 65002,
            .local_as = 65001,
            .last_error = null,
            .messages_sent = 3,
            .messages_received = 2,
            .keepalives_sent = 1,
            .keepalives_received = 1,
            .passive_listener_state = .disabled,
            .passive_listener_error = null,
        },
    };

    var buf1: [64]u8 = undefined;
    var buf2: [64]u8 = undefined;

    const check1 = bgp_status.buildBgpCheckInto(state1, &buf1);
    const check2 = bgp_status.buildBgpCheckInto(state2, &buf2);

    // Verify they point to different buffers
    try std.testing.expect(@intFromPtr(check1.detail.ptr) != @intFromPtr(check2.detail.ptr));

    // Verify content is correct for each state
    try std.testing.expect(std.mem.containsAtLeast(u8, check1.detail, 1, "10 configured"));
    try std.testing.expect(std.mem.containsAtLeast(u8, check2.detail, 1, "1 configured prefix"));
}

// ============================================================================
// StatusScratch ownership tests
// ============================================================================

test "StatusScratch contains dedicated BFD and BGP detail buffers" {
    var scratch = status.StatusScratch{ .allocator = std.heap.page_allocator };
    
    // Verify the struct has the expected fields
    const snapshot = bfd_status.StatusSnapshot{
        .peer_count = 2,
        .up_count = 1,
        .has_peers = true,
    };
    
    // Build BFD check using scratch's bfd_detail buffer
    const bfd_check = bfd_status.buildStatusCheckInto(snapshot, &scratch.bfd_detail);
    try std.testing.expectEqualStrings("1/2 bfd sessions up", bfd_check.detail);
    try std.testing.expect(@intFromPtr(bfd_check.detail.ptr) == @intFromPtr(&scratch.bfd_detail[0]));
}

test "getBfdCheck uses caller-provided buffer" {
    var scratch = status.StatusScratch{ .allocator = std.heap.page_allocator };
    
    const check = status.getBfdCheck(null, &scratch.bfd_detail);
    
    try std.testing.expectEqualStrings("bfd", check.name);
    try std.testing.expect(check.status == .warn);
    try std.testing.expectEqualStrings("bfd not configured", check.detail);
}

test "getBgpCheck uses caller-provided buffer" {
    var scratch = status.StatusScratch{ .allocator = std.heap.page_allocator };
    
    const check = status.getBgpCheck(.no_config, &scratch.bgp_detail);
    
    try std.testing.expectEqualStrings("bgp", check.name);
    try std.testing.expect(check.status == .warn);
    try std.testing.expectEqualStrings("BGP not configured", check.detail);
}

// ============================================================================
// Full render path reentrancy tests
// ============================================================================

test "two independent StatusScratch contexts don't alias" {
    var scratch1 = status.StatusScratch{ .allocator = std.heap.page_allocator };
    var scratch2 = status.StatusScratch{ .allocator = std.heap.page_allocator };

    // Build checks with different BGP states
    const checks1 = status.getLocalChecksWithBgp(
        null,
        status.getDefaultConfigCheck(),
        .no_config,
        &scratch1,
    );
    const checks2 = status.getLocalChecksWithBgp(
        null,
        status.getDefaultConfigCheck(),
        .{
            .configured = .{
                .configured_prefix_count = 5,
                .updates_sent = 0,
                .nlri_sent_count = 0,
                .fsm_state = .established,
                .peer_address = .{ 10, 0, 0, 2 },
                .peer_as = 65002,
                .local_as = 65001,
                .last_error = null,
                .messages_sent = 10,
                .messages_received = 8,
                .keepalives_sent = 5,
                .keepalives_received = 5,
                .passive_listener_state = .disabled,
                .passive_listener_error = null,
            },
        },
        &scratch2,
    );

    // Find BGP check in each
    var bgp_check1: ?status.Check = null;
    var bgp_check2: ?status.Check = null;
    for (checks1) |c| {
        if (std.mem.eql(u8, c.name, "bgp")) bgp_check1 = c;
    }
    for (checks2) |c| {
        if (std.mem.eql(u8, c.name, "bgp")) bgp_check2 = c;
    }

    try std.testing.expect(bgp_check1 != null);
    try std.testing.expect(bgp_check2 != null);

    // Verify BGP detail points to different buffers
    try std.testing.expect(@intFromPtr(bgp_check1.?.detail.ptr) != @intFromPtr(bgp_check2.?.detail.ptr));

    // Verify content is correct
    try std.testing.expectEqualStrings("BGP not configured", bgp_check1.?.detail);
    try std.testing.expect(std.mem.containsAtLeast(u8, bgp_check2.?.detail, 1, "5 configured"));
}

test "repeated rendering with separate scratch is consistent" {
    var w1 = TestWriter.init();
    var w2 = TestWriter.init();
    var w3 = TestWriter.init();

    // Render three times with separate scratch each time
    try status.renderPayload(&w1);
    try status.renderPayload(&w2);
    try status.renderPayload(&w3);

    // All renders should produce valid JSON with same structure
    try std.testing.expect(std.mem.containsAtLeast(u8, w1.slice(), 1, "\"service\":\"tovarisch\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w2.slice(), 1, "\"service\":\"tovarisch\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w3.slice(), 1, "\"service\":\"tovarisch\""));

    // All should have same check names
    try std.testing.expect(std.mem.containsAtLeast(u8, w1.slice(), 1, "\"name\":\"bfd\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w2.slice(), 1, "\"name\":\"bfd\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w3.slice(), 1, "\"name\":\"bfd\""));
}

// ============================================================================
// No module-level mutable state in status rendering
// ============================================================================

test "getLocalChecks returns slice to caller-owned scratch.checks" {
    var scratch = status.StatusScratch{ .allocator = std.heap.page_allocator };
    const checks = status.getLocalChecksWithBgp(
        null,
        status.getDefaultConfigCheck(),
        .no_config,
        &scratch,
    );

    // The checks slice should point into scratch.checks
    try std.testing.expect(@intFromPtr(checks.ptr) == @intFromPtr(&scratch.checks[0]));
    try std.testing.expectEqual(@as(usize, 9), checks.len);
}

test "buildStatusWithInputs uses caller-provided scratch" {
    var scratch = status.StatusScratch{ .allocator = std.heap.page_allocator };
    const s = status.buildStatusWithInputs(.{
        .bfd_runtime = null,
        .config_check = .no_config,
        .bgp_result = .{ .no_config = {} },
    }, &scratch);

    // Status.checks should point to scratch
    try std.testing.expect(@intFromPtr(s.checks.ptr) == @intFromPtr(&scratch.checks[0]));
    try std.testing.expectEqual(@as(usize, 9), s.checks.len);
}

// ============================================================================
// Static string safety tests (verify static checks don't use mutable state)
// ============================================================================

test "static checks have detail pointing to static strings" {
    // Process check should have immutable static detail
    var scratch = status.StatusScratch{ .allocator = std.heap.page_allocator };
    const checks = status.getLocalChecksWithBgp(
        null,
        status.getDefaultConfigCheck(),
        .no_config,
        &scratch,
    );
    
    var process_check: ?status.Check = null;
    for (checks) |c| {
        if (std.mem.eql(u8, c.name, "process")) process_check = c;
    }
    
    try std.testing.expect(process_check != null);
    // Static detail "running" is a string literal, immutable
    try std.testing.expectEqualStrings("running", process_check.?.detail);
}

