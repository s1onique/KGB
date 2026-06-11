// bgp/status.zig — BGP status reporting for tovarisch
//
// Provides BGP status snapshot derivation for integration with the main status system.
// The BGP status module is stateless: it transforms an optional bundle pointer
// into a contract-valid check. Daemon owns the runtime, not the status module.
//
// KEY CONSTRAINT: This module does NOT perform network I/O.
// Status rendering is purely derived from the bundle's immutable config state.

const std = @import("std");
const serve_integration = @import("serve_integration.zig");

/// BGP check status enum (mirrors status.CheckStatus).
pub const CheckStatus = enum {
    ok,
    warn,
    @"error",
    unknown,
};

/// BGP status check result (mirrors status.Check).
pub const BgpCheck = struct {
    name: []const u8,
    status: CheckStatus,
    detail: []const u8,
};

/// BGP status state for status reporting.
/// This represents the effective runtime state derived at serve startup.
/// 
/// Ownership: Caller owns the bundle pointer for the .configured case.
/// The status module does NOT own the bundle.
pub const BgpStatusState = union(enum) {
    /// No config path was provided to serve command.
    no_config,
    /// Config file present but no [bgp] section.
    not_configured,
    /// [bgp] section present but enabled=false.
    disabled,
    /// BGP configured and valid, including zero or more prefixes.
    configured: Configured,
    /// BGP config build or validation failed.
    failed: Failure,
    /// BGP runtime failed post-startup.
    runtime_failed: Failure,

    pub const Configured = struct {
        /// Number of advertised prefixes (may be 0).
        advertised_prefix_count: usize,
        /// Current BGP FSM state (idle, connect, open_sent, open_confirm, established, failed, stopped).
        fsm_state: []const u8,
        /// Peer address as raw bytes (avoids dangling slice from stack buffer).
        peer_address: [4]u8,
        /// Peer's ASN.
        peer_as: u16,
        /// Our local ASN.
        local_as: u16,
        /// Last error message (null if no error).
        last_error: ?[]const u8,
        /// Messages sent counter.
        messages_sent: u64,
        /// Messages received counter.
        messages_received: u64,
        /// Keepalives sent counter.
        keepalives_sent: u64,
        /// Keepalives received counter.
        keepalives_received: u64,
    };

    pub const Failure = struct {
        /// Sanitized error message (no internal details exposed).
        message: []const u8,
    };
};

/// Maximum buffer size for BGP detail formatting.
/// Format: "BGP configured; 9999 advertised prefixes" = 42 chars max.
const BGP_DETAIL_BUF_SIZE: usize = 64;

/// Build a BGP check from BgpStatusState using caller-provided buffer.
/// This is the allocation-free variant - callers pass a buffer for dynamic details.
/// 
/// The returned BgpCheck.detail points to either:
/// - A static string for non-partial cases
/// - The caller's buffer for dynamic messages
pub fn buildBgpCheckInto(
    state: BgpStatusState,
    detail_buf: *[BGP_DETAIL_BUF_SIZE]u8,
) BgpCheck {
    switch (state) {
        .no_config => return .{
            .name = "bgp",
            .status = .warn,
            .detail = "BGP not configured",
        },
        .not_configured => return .{
            .name = "bgp",
            .status = .warn,
            .detail = "BGP not configured",
        },
        .disabled => return .{
            .name = "bgp",
            .status = .ok,
            .detail = "BGP disabled by config",
        },
        .configured => |cfg| {
            if (cfg.advertised_prefix_count == 0) {
                return .{
                    .name = "bgp",
                    .status = .warn,
                    .detail = "BGP configured with no advertised prefixes",
                };
            }
            // Format into caller's buffer - avoid inline conditional in format
            const prefix_label = if (cfg.advertised_prefix_count == 1) "prefix" else "prefixes";
            const detail = std.fmt.bufPrint(
                detail_buf,
                "BGP configured; {d} advertised {s}",
                .{ cfg.advertised_prefix_count, prefix_label },
            ) catch {
                // Buffer too small - use static fallback
                return .{
                    .name = "bgp",
                    .status = .ok,
                    .detail = "BGP configured",
                };
            };
            return .{
                .name = "bgp",
                .status = .ok,
                .detail = detail,
            };
        },
        .failed => |fail| return .{
            .name = "bgp",
            .status = .@"error",
            .detail = fail.message,
        },
        .runtime_failed => |fail| return .{
            .name = "bgp",
            .status = .@"error",
            .detail = fail.message,
        },
    }
}

/// Format IPv4 address as string into buffer.
fn formatPeerAddress(addr: [4]u8, buf: *[16]u8) []const u8 {
    const result = std.fmt.bufPrint(buf, "{}.{}.{}.{}", .{
        addr[0],
        addr[1],
        addr[2],
        addr[3],
    }) catch unreachable;
    return result;
}

/// Derive BgpStatusState from an optional BgpServeBundle pointer.
/// Returns .no_config when bundle is null (no config path was provided).
/// Returns .not_configured when bundle state is .not_configured.
/// Returns .disabled when bundle state is .disabled.
/// Returns .configured when bundle state is .configured.
/// Returns .failed when bundle state is .failed.
/// Returns .failed when bundle has last_error set.
/// 
/// When .configured, populates runtime FSM state and message counters.
pub fn deriveStatusStateFromBundle(bundle: ?*serve_integration.BgpServeBundle) BgpStatusState {
    if (bundle == null) {
        return .no_config;
    }

    const b = bundle.?;
    switch (b.state) {
        .not_configured => return .not_configured,
        .disabled => return .disabled,
        .configured => {
            // Get session status for runtime data
            const sess_status = serve_integration.getSessionStatus(b);

            // If session has failed, report runtime failure with concrete error.
            // This handles the case where bundle.state is still .configured but
            // session.runOnce() failed with a concrete send error.
            if (sess_status.state == .failed) {
                const err_msg = if (sess_status.last_error) |e| e.message else "session failed";
                return .{ .runtime_failed = .{ .message = err_msg } };
            }

            const fsm_state = @tagName(sess_status.state);
            return .{
                .configured = .{
                    .advertised_prefix_count = b.prefixes.len,
                    .fsm_state = fsm_state,
                    .peer_address = sess_status.peer_address,
                    .peer_as = sess_status.peer_as,
                    .local_as = sess_status.local_as,
                    .last_error = if (sess_status.last_error) |e| e.message else null,
                    .messages_sent = sess_status.messages_sent,
                    .messages_received = sess_status.messages_received,
                    .keepalives_sent = sess_status.keepalives_sent,
                    .keepalives_received = sess_status.keepalives_received,
                },
            };
        },
        .failed => {
            const err_msg = if (b.last_error) |e| e else "configuration failed";
            return .{ .failed = .{ .message = err_msg } };
        },
    }
}

// --- Tests ---

test "buildBgpCheckInto returns warn for no_config" {
    var detail_buf: [BGP_DETAIL_BUF_SIZE]u8 = undefined;
    const check = buildBgpCheckInto(.no_config, &detail_buf);
    try std.testing.expectEqualStrings("bgp", check.name);
    try std.testing.expect(check.status == .warn);
    try std.testing.expectEqualStrings("BGP not configured", check.detail);
}

test "buildBgpCheckInto returns warn for not_configured" {
    var detail_buf: [BGP_DETAIL_BUF_SIZE]u8 = undefined;
    const check = buildBgpCheckInto(.not_configured, &detail_buf);
    try std.testing.expectEqualStrings("bgp", check.name);
    try std.testing.expect(check.status == .warn);
    try std.testing.expectEqualStrings("BGP not configured", check.detail);
}

test "buildBgpCheckInto returns ok for disabled" {
    var detail_buf: [BGP_DETAIL_BUF_SIZE]u8 = undefined;
    const check = buildBgpCheckInto(.disabled, &detail_buf);
    try std.testing.expectEqualStrings("bgp", check.name);
    try std.testing.expect(check.status == .ok);
    try std.testing.expectEqualStrings("BGP disabled by config", check.detail);
}

test "buildBgpCheckInto returns warn for configured with zero prefixes" {
    var detail_buf: [BGP_DETAIL_BUF_SIZE]u8 = undefined;
    const check = buildBgpCheckInto(.{
        .configured = .{
            .advertised_prefix_count = 0,
            .fsm_state = "idle",
            .peer_address = .{ 10, 0, 0, 2 },
            .peer_as = 65002,
            .local_as = 65001,
            .last_error = null,
            .messages_sent = 0,
            .messages_received = 0,
            .keepalives_sent = 0,
            .keepalives_received = 0,
        },
    }, &detail_buf);
    try std.testing.expectEqualStrings("bgp", check.name);
    try std.testing.expect(check.status == .warn);
    try std.testing.expectEqualStrings("BGP configured with no advertised prefixes", check.detail);
}

test "buildBgpCheckInto returns ok for configured with one prefix" {
    var detail_buf: [BGP_DETAIL_BUF_SIZE]u8 = undefined;
    const check = buildBgpCheckInto(.{
        .configured = .{
            .advertised_prefix_count = 1,
            .fsm_state = "established",
            .peer_address = .{ 10, 0, 0, 2 },
            .peer_as = 65002,
            .local_as = 65001,
            .last_error = null,
            .messages_sent = 3,
            .messages_received = 2,
            .keepalives_sent = 1,
            .keepalives_received = 1,
        },
    }, &detail_buf);
    try std.testing.expectEqualStrings("bgp", check.name);
    try std.testing.expect(check.status == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, check.detail, 1, "1 advertised prefix"));
}

test "buildBgpCheckInto returns ok for configured with multiple prefixes" {
    var detail_buf: [BGP_DETAIL_BUF_SIZE]u8 = undefined;
    const check = buildBgpCheckInto(.{
        .configured = .{
            .advertised_prefix_count = 5,
            .fsm_state = "open_sent",
            .peer_address = .{ 10, 0, 0, 2 },
            .peer_as = 65002,
            .local_as = 65001,
            .last_error = null,
            .messages_sent = 1,
            .messages_received = 0,
            .keepalives_sent = 0,
            .keepalives_received = 0,
        },
    }, &detail_buf);
    try std.testing.expectEqualStrings("bgp", check.name);
    try std.testing.expect(check.status == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, check.detail, 1, "5 advertised prefixes"));
}

test "buildBgpCheckInto returns error for failed" {
    var detail_buf: [BGP_DETAIL_BUF_SIZE]u8 = undefined;
    const check = buildBgpCheckInto(.{ .failed = .{ .message = "invalid AS number" } }, &detail_buf);
    try std.testing.expectEqualStrings("bgp", check.name);
    try std.testing.expect(check.status == .@"error");
    try std.testing.expectEqualStrings("invalid AS number", check.detail);
}

test "buildBgpCheckInto returns error for runtime_failed" {
    var detail_buf: [BGP_DETAIL_BUF_SIZE]u8 = undefined;
    const check = buildBgpCheckInto(.{ .runtime_failed = .{ .message = "session lost" } }, &detail_buf);
    try std.testing.expectEqualStrings("bgp", check.name);
    try std.testing.expect(check.status == .@"error");
    try std.testing.expectEqualStrings("session lost", check.detail);
}

test "buildBgpCheckInto uses caller's buffer" {
    var detail_buf: [BGP_DETAIL_BUF_SIZE]u8 = undefined;
    const check = buildBgpCheckInto(.{
        .configured = .{
            .advertised_prefix_count = 3,
            .fsm_state = "established",
            .peer_address = .{ 10, 0, 0, 2 },
            .peer_as = 65002,
            .local_as = 65001,
            .last_error = null,
            .messages_sent = 5,
            .messages_received = 4,
            .keepalives_sent = 2,
            .keepalives_received = 2,
        },
    }, &detail_buf);
    
    try std.testing.expect(check.status == .ok);
    // Verify the detail points to our buffer (not heap-allocated)
    try std.testing.expect(@intFromPtr(check.detail.ptr) == @intFromPtr(&detail_buf[0]));
}

test "deriveStatusStateFromBundle returns no_config for null" {
    const state = deriveStatusStateFromBundle(null);
    try std.testing.expect(state == .no_config);
}
