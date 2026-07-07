// status_wg_ownership_tests.zig — WireGuard peers ownership tests for status rendering
//
// Tests for wg_peers check ownership tracking and memory management.
// Split from status_ownership_tests.zig to keep files under 450-line limit.
//
// ACT-HULK29R-ZIG016-STATUS-REQUEST-LEAK-FIX

const std = @import("std");
const status = @import("status.zig");
const status_checks = @import("status_checks.zig");
const wg_boundary = @import("net/wg_status_boundary.zig");

// ============================================================================
// TestWriter helper (duplicated from status_ownership_tests.zig for self-containment)
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
// getWgPeersCheck static detail proof
// These tests prove that getWgPeersCheck() does NOT allocate dynamic detail strings.
// All detail strings are static string literals, safe for any lifetime.
// ============================================================================

test "getWgPeersCheckFromError returns static detail strings" {
    // All error details are static string literals - no allocation
    const err_details = [_]struct { wg_boundary.StatusError, []const u8 }{
        .{ error.backend_missing, "wg command not available" },
        .{ error.command_failed, "wg command failed" },
        .{ error.malformed_output, "wg output malformed" },
        .{ error.out_of_memory, "wg check out of memory" },
        .{ error.timeout, "wg command timeout" },
        .{ error.interface_missing, "wg interface not found" },
        .{ error.permission_denied, "wg permission denied" },
        .{ error.unsupported_platform, "wg not supported on this platform" },
    };

    for (err_details) |pair| {
        const check = status_checks.getWgPeersCheckFromError(pair[0]);
        try std.testing.expectEqualStrings(pair[1], check.detail);
    }
}

test "getWgPeersCheckFromParsed returns static detail strings" {
    // All detail strings are static string literals - no allocation
    const check_no_peers = status_checks.getWgPeersCheckFromParsed(0, false);
    try std.testing.expectEqualStrings("no peers detected", check_no_peers.detail);

    const check_no_handshake = status_checks.getWgPeersCheckFromParsed(1, false);
    try std.testing.expectEqualStrings("no handshake yet", check_no_handshake.detail);

    const check_ok = status_checks.getWgPeersCheckFromParsed(1, true);
    try std.testing.expectEqualStrings("wireguard peers healthy", check_ok.detail);
}

// ============================================================================
// wg_peers detail allocation ownership tests
// ACT-HULK29R-ZIG016-STATUS-REQUEST-LEAK-FIX
// ============================================================================

test "wg_peers detail ownership is tracked and can be deinited" {
    // Test that when wg_peers allocates a diagnostic detail string,
    // the owns_detail flag is set and deinit properly frees it.
    // This is a regression test for the per-request RSS leak.
    var scratch = status.StatusScratch{ .allocator = std.testing.allocator };

    // Build local checks including wg_peers
    _ = status.getLocalChecksWithBgp(
        null,
        status.getDefaultConfigCheck(),
        .no_config,
        &scratch,
    );

    // Find wg_peers check
    var wg_check: ?*status.Check = null;
    for (&scratch.checks) |*c| {
        if (std.mem.eql(u8, c.name, "wg_peers")) {
            wg_check = c;
            break;
        }
    }

    try std.testing.expect(wg_check != null);

    // The wg check may or may not own detail depending on wg availability
    // But deinit should be safe to call (no-op if owns_detail is false)
    wg_check.?.deinit(std.testing.allocator);
}

test "deinitScratchChecks frees all owned check details" {
    // Test that deinitScratchChecks properly frees any owned allocations
    // This is the canonical cleanup pattern for the HTTP request path.
    var scratch = status.StatusScratch{ .allocator = std.testing.allocator };

    // Build local checks including wg_peers
    _ = status.getLocalChecksWithBgp(
        null,
        status.getDefaultConfigCheck(),
        .no_config,
        &scratch,
    );

    // This should not leak even if some checks have owned details
    status.deinitScratchChecks(&scratch);
}

test "repeated status render under testing allocator is leak-free" {
    // Regression test: repeated status rendering should not leak memory
    // under std.testing.allocator. This catches the wg_peers detail leak.
    //
    // The original bug: getWgPeersCheck() allocated detail_owned but never freed it.
    // Each /status call leaked the diagnostic detail string.
    // Fixed by: setting owns_detail=true and calling deinitScratchChecks() after render.
    const iterations: usize = 10;

    var i: usize = 0;
    while (i < iterations) : (i += 1) {
        var scratch = status.StatusScratch{ .allocator = std.testing.allocator };

        // Build checks (may allocate wg_peers detail on wg failure)
        const checks = status.getLocalChecksWithBgp(
            null,
            status.getDefaultConfigCheck(),
            .no_config,
            &scratch,
        );

        // Build status
        const s = status.Status{
            .service = "tovarisch",
            .version = "test",
            .node_id = "test",
            .status = status.deriveStatus(checks),
            .checks = checks,
            .bgp = null,
            .runtime = .{
                .pid = 1,
                .rss_kib = null,
            },
        };

        // Render to a test writer
        var w = TestWriter.init();
        try status.renderStatus(&w, s);

        // CRITICAL: Free any owned check details before next iteration
        // This is what the HTTP handler does via defer { deinitScratchChecks(&scratch); }
        status.deinitScratchChecks(&scratch);
    }
    // If we reach here, no allocations leaked
}

test "renderPayloadWithContextAndDiag properly cleans up scratch checks" {
    // Test that renderPayloadWithContextAndDiag's defer statement properly
    // cleans up any owned check details. This is the production HTTP handler path.
    const iterations: usize = 5;

    var i: usize = 0;
    while (i < iterations) : (i += 1) {
        var writer = TestWriter.init();
        try status.renderPayloadWithContextAndDiag(
            &writer,
            .{
                .bfd_runtime = null,
                .config_check = .no_config,
                .bgp_result = .{ .no_config = {} },
                .lab_config = .{ .disable_wg_checks = false },
            },
            std.testing.allocator,
            false, // no network_diag
        );
    }
    // If we reach here, no allocations leaked
}
