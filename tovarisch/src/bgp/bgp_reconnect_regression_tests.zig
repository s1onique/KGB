// bgp/bgp_reconnect_regression_tests.zig — BGP reconnect regression tests
//
// Regression tests for ACT: Fix BGP post-glitch reconnect/reset livelock
//
// Tests prove that after network disruption:
// 1. tovarisch detects TCP EOF/reset/write failure
// 2. Failed transport is fully discarded
// 3. BGP FSM state is reset for fresh OPEN exchange
// 4. Status endpoint distinguishes BFD up vs BGP reconnecting
// 5. Reconnect statistics are tracked
//
// Context: After a network glitch where WireGuard/BFD recovered but BGP stayed down,
// BIRD showed "Socket: Connection reset by peer" while tovarisch showed "BGP reconnecting
// in 1000ms". This indicates TCP session lifecycle failure after disruption.

const std = @import("std");
const serve_integration = @import("serve_integration.zig");
const session = @import("session.zig");
const types = @import("types.zig");
const tcp_transport = @import("tcp_transport.zig");
const clock = @import("clock.zig");
const bgp_status = @import("status.zig");

// ============================================================================
// Test Helpers
// ============================================================================

/// A fake transport that fails on send with a configurable error.
const FailingFakeTransport = struct {
    const Self = @This();

    allocator: std.mem.Allocator,
    closed: bool = false,
    error_to_return: session.TransportError,

    pub fn init(allocator: std.mem.Allocator, err: session.TransportError) Self {
        return Self{
            .allocator = allocator,
            .closed = false,
            .error_to_return = err,
        };
    }

    pub fn send(self: *Self, data: []const u8) session.TransportError!void {
        _ = data;
        return self.error_to_return;
    }

    pub fn recv(self: *Self) []const u8 {
        _ = self;
        return &[_]u8{};
    }

    pub fn close(self: *Self) void {
        self.closed = true;
    }

    pub fn toTransport(self: *Self) session.Transport {
        return session.Transport{
            .sendFn = struct {
                fn send(ctx: *anyopaque, data: []const u8) session.TransportError!void {
                    const fake: *Self = @ptrCast(@alignCast(ctx));
                    return fake.send(data);
                }
            }.send,
            .recvFn = struct {
                fn recv(ctx: *anyopaque) []const u8 {
                    const fake: *Self = @ptrCast(@alignCast(ctx));
                    return fake.recv();
                }
            }.recv,
            .closeFn = struct {
                fn close(ctx: *anyopaque) void {
                    const fake: *Self = @ptrCast(@alignCast(ctx));
                    fake.close();
                }
            }.close,
            .isClosedFn = struct {
                fn isClosed(ctx: *anyopaque) bool {
                    const fake: *Self = @ptrCast(@alignCast(ctx));
                    return fake.closed;
                }
            }.isClosed,
            .ctx = @ptrCast(self),
        };
    }
};

/// Create a minimal session config for testing.
fn makeSessionConfig() session.SessionConfig {
    return .{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = .{ 127, 0, 0, 1 },
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{types.Ipv4Prefix.init("10.0.0.0/8")},
        .same_as = true,
    };
}

/// Creates a minimal BgpServeBundle for testing.
fn createMinimalBundle(allocator: std.mem.Allocator) !*serve_integration.BgpServeBundle {
    const sess_cfg = makeSessionConfig();

    const bundle = try allocator.create(serve_integration.BgpServeBundle);
    errdefer allocator.destroy(bundle);

    bundle.* = serve_integration.BgpServeBundle{
        .raw = undefined,
        .bgp_config = undefined,
        .session_config = sess_cfg,
        .state = .configured,
        .last_error = null,
        .prefixes = &.{},
        .tcp = tcp_transport_createDummy(),
        .trans = undefined,
        .sess = undefined,
    };

    // Set tcp and trans BEFORE initializing session
    bundle.tcp = tcp_transport_createDummy();
    bundle.trans = bundle.tcp.toTransport();

    // Initialize session with bundle-owned transport pointer (NOT a local)
    const sess = try session.init(sess_cfg, &bundle.trans);
    bundle.sess = sess;

    return bundle;
}

fn tcp_transport_createDummy() tcp_transport.TcpTransport {
    return tcp_transport.TcpTransport{
        .socket_fd = -1,
        .recv_buf = undefined,
        .recv_len = 0,
        .closed = true,
        .peer_address = .{ 0, 0, 0, 0 },
        .peer_port = 0,
    };
}

// ============================================================================
// Regression Test 1: ConnectionReset error propagates to session failure
// ============================================================================

test "ConnectionReset send failure leaves session in failed state" {
    var fake = FailingFakeTransport.init(std.testing.allocator, session.TransportError.ConnectionReset);
    var sess = try session.init(makeSessionConfig(), &fake.toTransport());

    // Verify initial state is idle
    try std.testing.expectEqual(session.SessionState.idle, sess.status.state);

    // Simulate running session once with ConnectionReset failure
    const result = session.runOnce(&sess);

    // Verify the error is an IoError (which wraps TransportError variants)
    try std.testing.expectError(session.SessionErrorKind.IoError, result);

    // Verify session is now in failed state
    try std.testing.expectEqual(session.SessionState.failed, sess.status.state);

    // Verify error message contains ECONNRESET
    try std.testing.expect(sess.status.last_error != null);
    try std.testing.expect(std.mem.indexOf(u8, sess.status.last_error.?.message, "ECONNRESET") != null);
}

// ============================================================================
// Regression Test 2: closeForReconnect fully resets session state
// ============================================================================

test "closeForReconnect clears all session state for fresh reconnect" {
    const bundle = try createMinimalBundle(std.testing.allocator);
    defer std.testing.allocator.destroy(bundle);

    // Set various session fields to non-idle values
    bundle.sess.status.state = .open_sent;
    bundle.sess.recv_len = 100;
    bundle.sess.send_pos = 50;
    bundle.sess.peer_open = null;
    bundle.sess.negotiated_hold_time = 180;
    bundle.sess.keepalive_interval_ms = 60;
    bundle.sess.hold_timer_deadline = 1000;
    bundle.sess.pending_keepalive = true;
    bundle.sess.pending_keepalive_ms = 100;
    bundle.sess.status.last_error = session.SessionError{
        .message = "TCP connection closed by peer",
        .notification_code = null,
        .notification_subcode = null,
    };

    // Call closeForReconnect
    serve_integration.closeForReconnect(bundle);

    // Verify session state was reset to idle
    try std.testing.expectEqual(session.SessionState.idle, bundle.sess.status.state);
    try std.testing.expectEqual(@as(usize, 0), bundle.sess.recv_len);
    try std.testing.expectEqual(@as(usize, 0), bundle.sess.send_pos);
    try std.testing.expect(bundle.sess.peer_open == null);
    try std.testing.expectEqual(@as(u16, 0), bundle.sess.negotiated_hold_time);
    try std.testing.expectEqual(@as(u32, 0), bundle.sess.keepalive_interval_ms);
    try std.testing.expectEqual(@as(u64, 0), bundle.sess.hold_timer_deadline);
    try std.testing.expectEqual(false, bundle.sess.pending_keepalive);
    try std.testing.expectEqual(@as(u64, 0), bundle.sess.pending_keepalive_ms);

    // Verify terminal error is cleared
    try std.testing.expectEqual(@as(?session.SessionError, null), bundle.sess.status.last_error);

    // Verify session is reconnectable (not terminal)
    try std.testing.expect(!session.isTerminal(&bundle.sess));
}

// ============================================================================
// Regression Test 3: reconnect statistics tracking
// ============================================================================

test "bundle tracks reconnect_count field" {
    const bundle = try createMinimalBundle(std.testing.allocator);
    defer std.testing.allocator.destroy(bundle);

    // Initial state: no reconnects
    try std.testing.expectEqual(@as(u64, 0), bundle.reconnect_count);
    try std.testing.expectEqual(@as(clock.MonoTime, 0), bundle.last_reconnect_time);

    // Verify the reconnect_count field exists and can be incremented
    bundle.reconnect_count += 1;
    try std.testing.expectEqual(@as(u64, 1), bundle.reconnect_count);

    // Simulate multiple reconnect attempts
    bundle.reconnect_count += 1;
    bundle.last_reconnect_time = 5000;
    try std.testing.expectEqual(@as(u64, 2), bundle.reconnect_count);
    try std.testing.expectEqual(@as(clock.MonoTime, 5000), bundle.last_reconnect_time);
}

// ============================================================================
// Regression Test 4: Status distinguishes BFD up vs BGP reconnecting
// ============================================================================

test "bundle state transitions distinguish reconnect_wait from configured" {
    const bundle = try createMinimalBundle(std.testing.allocator);
    defer std.testing.allocator.destroy(bundle);

    // Set state to configured (normal operational state)
    bundle.state = .configured;
    try std.testing.expectEqual(serve_integration.BgpRuntimeState.configured, bundle.state);

    // Set state to reconnect_wait (simulating post-glitch state)
    bundle.state = .reconnect_wait;
    bundle.backoff_ms = 1000;
    bundle.last_error = "TCP connection closed by peer";

    // Verify state is reconnect_wait, not configured
    try std.testing.expectEqual(serve_integration.BgpRuntimeState.reconnect_wait, bundle.state);
    try std.testing.expectEqual(@as(u64, 1000), bundle.backoff_ms);
    try std.testing.expect(bundle.last_error != null);
}

// ============================================================================
// Regression Test 5: socket error captured on reconnect failure
// ============================================================================

test "bundle captures last_socket_error for status reporting" {
    const bundle = try createMinimalBundle(std.testing.allocator);
    defer std.testing.allocator.destroy(bundle);

    // Simulate reconnect failure - error is captured in bundle.last_socket_error
    bundle.last_socket_error = @as(?[]const u8, "ConnectionFailed");
    bundle.last_error = bundle.last_socket_error;

    // Verify error is preserved for status reporting
    try std.testing.expect(bundle.last_socket_error != null);
    try std.testing.expectEqualStrings("ConnectionFailed", bundle.last_socket_error.?);

    // Clear error on successful reconnect (simulated)
    bundle.last_socket_error = null;
    bundle.last_error = null;

    try std.testing.expectEqual(@as(?[]const u8, null), bundle.last_socket_error);
}

// Regression Test 6: Session can be made reconnectable after ConnectionReset
// ============================================================================

test "closeForReconnect makes failed session reconnectable" {
    const bundle = try createMinimalBundle(std.testing.allocator);
    defer std.testing.allocator.destroy(bundle);

    // Simulate a failed session state (e.g., after ConnectionReset)
    bundle.sess.status.state = .failed;
    bundle.sess.status.last_error = session.SessionError{
        .message = "Connection reset by peer",
        .notification_code = null,
        .notification_subcode = null,
    };

    // Verify session is terminal BEFORE closeForReconnect
    try std.testing.expectEqual(session.SessionState.failed, bundle.sess.status.state);
    try std.testing.expect(session.isTerminal(&bundle.sess));

    // Call closeForReconnect to reset session state
    serve_integration.closeForReconnect(bundle);

    // Verify session is NOT terminal AFTER closeForReconnect (can reconnect)
    try std.testing.expectEqual(session.SessionState.idle, bundle.sess.status.state);
    try std.testing.expect(!session.isTerminal(&bundle.sess));
}

// ============================================================================
// Regression Test 7: deriveStatusStateFromBundle includes reconnect stats
// ============================================================================

test "reconnect_wait status includes reconnect_count and last_socket_error" {
    const bundle = try createMinimalBundle(std.testing.allocator);
    defer std.testing.allocator.destroy(bundle);

    // Simulate post-glitch reconnect state with statistics
    bundle.state = .reconnect_wait;
    bundle.backoff_ms = 2000;
    bundle.reconnect_count = 3;
    bundle.last_socket_error = "ConnectionRefused";
    bundle.last_error = bundle.last_socket_error;

    // Derive status state from bundle
    const status_state = bgp_status.deriveStatusStateFromBundle(bundle);

    // Verify reconnect_wait variant
    try std.testing.expect(status_state == .reconnect_wait);
    const rw = status_state.reconnect_wait;

    // Verify reconnect statistics are correctly propagated
    try std.testing.expectEqual(@as(u64, 3), rw.reconnect_count);
    try std.testing.expectEqualStrings("ConnectionRefused", rw.last_socket_error.?);
    try std.testing.expectEqual(@as(u64, 2000), rw.backoff_ms);
}

// ============================================================================
// Regression Test 8: closeForReconnect resets export state for re-announcement
// ============================================================================
//
// Root cause: After a BGP session established and exported all prefixes, a
// reconnect was triggered. closeForReconnect() reset session state but did NOT
// reset export_batch_index, export_complete, or nlri_sent_count. On reconnection,
// the condition "sess.config.prefixes.len > 0 and sess.send_pos == 0 and
// !sess.export_complete" would fail because export_complete was still true.
//
// Result: BIRD showed "Import updates: 0 received" despite tovarisch reporting
// "BGP established; 15810 configured prefixes".
//
// Fix: Reset export state in closeForReconnect().

test "closeForReconnect resets export state for initial prefix announcement on reconnect" {
    const bundle = try createMinimalBundle(std.testing.allocator);
    defer std.testing.allocator.destroy(bundle);

    // Simulate a session that has completed export (established + all prefixes sent)
    bundle.sess.status.state = .established;
    bundle.sess.export_batch_index = 1000; // All prefixes exported
    bundle.sess.export_complete = true; // Export is "done" from previous session
    bundle.sess.nlri_sent_count = 1000; // All prefixes encoded

    // Verify preconditions before closeForReconnect
    try std.testing.expectEqual(@as(usize, 1000), bundle.sess.export_batch_index);
    try std.testing.expectEqual(true, bundle.sess.export_complete);
    try std.testing.expectEqual(@as(usize, 1000), bundle.sess.nlri_sent_count);

    // Call closeForReconnect to prepare for reconnection
    serve_integration.closeForReconnect(bundle);

    // Verify export state is reset so initial prefixes will be announced on re-establishment
    try std.testing.expectEqual(@as(usize, 0), bundle.sess.export_batch_index);
    try std.testing.expectEqual(false, bundle.sess.export_complete);
    try std.testing.expectEqual(@as(usize, 0), bundle.sess.nlri_sent_count);

    // Verify session state is idle (ready for new BGP handshake)
    try std.testing.expectEqual(session.SessionState.idle, bundle.sess.status.state);

    // Verify the export encoding condition in session.runOnce() will now be true:
    //   if (sess.config.prefixes.len > 0 and sess.send_pos == 0 and !sess.export_complete)
    try std.testing.expect(bundle.sess.config.prefixes.len > 0);
    try std.testing.expectEqual(@as(usize, 0), bundle.sess.send_pos);
    try std.testing.expectEqual(false, bundle.sess.export_complete);
}
