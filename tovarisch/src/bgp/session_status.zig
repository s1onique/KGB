// session_status.zig — BGP session state and status types
//
// ACT 2: Session state machine types for tovarisch BGP support.
// References: RFC 4271 Section 8 (BGP State Machine)

const std = @import("std");
const types = @import("types.zig");

/// BGP session state machine states per RFC 4271 Section 8.2.
pub const SessionState = enum {
    /// Idle: Initial state, no connection.
    idle,
    /// Connect: TCP connection in progress.
    connect,
    /// OpenSent: OPEN sent, waiting for peer's OPEN.
    open_sent,
    /// OpenConfirm: OPEN received, waiting for KEEPALIVE.
    open_confirm,
    /// Established: Connection active, can send UPDATE.
    established,
    /// Failed: Connection failed, should reconnect with backoff (next ACT).
    failed,
    /// Stopped: Session stopped cleanly.
    stopped,
};

/// BGP session error details
pub const SessionError = struct {
    /// Error message
    message: []const u8,
    /// Error code (for NOTIFICATION messages)
    notification_code: ?u8,
    /// Error subcode (for NOTIFICATION messages)
    notification_subcode: ?u8,
};

/// BGP session status (in-memory, not exposed via HTTP/status JSON yet).
/// This struct tracks the current state and statistics for a BGP session.
pub const SessionStatus = struct {
    /// Current session state
    state: SessionState,
    /// Peer IPv4 address (4 bytes)
    peer_address: [4]u8,
    /// Peer's ASN
    peer_as: u16,
    /// Our local ASN
    local_as: u16,
    /// Our router ID
    router_id: [4]u8,
    /// Number of prefixes currently advertised
    advertised_prefix_count: usize,
    /// Total BGP messages sent
    messages_sent: u64,
    /// Total BGP messages received
    messages_received: u64,
    /// Number of UPDATE messages sent
    updates_sent: u64,
    /// Number of KEEPALIVE messages sent
    keepalives_sent: u64,
    /// Number of KEEPALIVE messages received
    keepalives_received: u64,
    /// Last error encountered (null if no error)
    last_error: ?SessionError,
    /// Last NOTIFICATION error code received (null if none)
    last_notification_code: ?u8,
    /// Last NOTIFICATION error subcode received (null if none)
    last_notification_subcode: ?u8,
};

/// Create an initial SessionStatus in the idle state.
pub fn initStatus(peer_addr: [4]u8, local_as: u16, peer_as: u16, router_id: [4]u8, advertised_prefix_count: usize) SessionStatus {
    return SessionStatus{
        .state = .idle,
        .peer_address = peer_addr,
        .peer_as = peer_as,
        .local_as = local_as,
        .router_id = router_id,
        .advertised_prefix_count = advertised_prefix_count,
        .messages_sent = 0,
        .messages_received = 0,
        .updates_sent = 0,
        .keepalives_sent = 0,
        .keepalives_received = 0,
        .last_error = null,
        .last_notification_code = null,
        .last_notification_subcode = null,
    };
}

// === Tests ===

test "SessionState enum has expected variants" {
    try std.testing.expectEqual(@as(usize, 7), @typeInfo(SessionState).@"enum".fields.len);
    try std.testing.expectEqualStrings("idle", @tagName(.idle));
    try std.testing.expectEqualStrings("connect", @tagName(.connect));
    try std.testing.expectEqualStrings("open_sent", @tagName(.open_sent));
    try std.testing.expectEqualStrings("open_confirm", @tagName(.open_confirm));
    try std.testing.expectEqualStrings("established", @tagName(.established));
    try std.testing.expectEqualStrings("failed", @tagName(.failed));
    try std.testing.expectEqualStrings("stopped", @tagName(.stopped));
}

test "initStatus creates idle session" {
    const status = initStatus(.{ 10, 0, 0, 2 }, 65001, 65002, .{ 10, 0, 0, 1 }, 3);
    try std.testing.expectEqual(SessionState.idle, status.state);
    try std.testing.expectEqualSlices(u8, &.{ 10, 0, 0, 2 }, &status.peer_address);
    try std.testing.expectEqual(@as(u16, 65001), status.local_as);
    try std.testing.expectEqual(@as(u16, 65002), status.peer_as);
    try std.testing.expectEqual(@as(u64, 0), status.messages_sent);
    try std.testing.expectEqual(@as(u64, 0), status.messages_received);
    try std.testing.expectEqual(@as(u64, 0), status.updates_sent);
    try std.testing.expectEqual(@as(u64, 0), status.keepalives_sent);
    try std.testing.expectEqual(@as(u64, 0), status.keepalives_received);
    try std.testing.expectEqual(@as(?SessionError, null), status.last_error);
}

test "SessionStatus has expected fields" {
    const status = initStatus(.{ 127, 0, 0, 1 }, 100, 200, .{ 127, 0, 0, 1 }, 0);
    // Verify all fields are present and initialized correctly
    try std.testing.expect(status.advertised_prefix_count == 0);
    try std.testing.expect(status.last_notification_code == null);
    try std.testing.expect(status.last_notification_subcode == null);
}
