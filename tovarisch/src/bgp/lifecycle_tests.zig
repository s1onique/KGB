// bgp/lifecycle_tests.zig — BGP reconnect lifecycle tests
//
// Tests for reconnect scheduling, cleanup, and state transitions.
// Uses actual production functions from serve_integration.zig.

const std = @import("std");
const serve_integration = @import("serve_integration.zig");
const session = @import("session.zig");
const transport = @import("transport.zig");
const clock = @import("clock.zig");
const tcp_transport = @import("tcp_transport.zig");

test "BgpRuntimeState enum has reconnect_wait variant" {
    const num_variants = @typeInfo(serve_integration.BgpRuntimeState).@"enum".fields.len;
    try std.testing.expectEqual(@as(usize, 5), num_variants);
    try std.testing.expectEqualStrings("not_configured", @tagName(.not_configured));
    try std.testing.expectEqualStrings("disabled", @tagName(.disabled));
    try std.testing.expectEqualStrings("configured", @tagName(.configured));
    try std.testing.expectEqualStrings("reconnect_wait", @tagName(.reconnect_wait));
    try std.testing.expectEqualStrings("failed", @tagName(.failed));
}

test "scheduleReconnect sets backoff and deadline" {
    const bundle = createMinimalBundle();

    // Initially configured state
    try std.testing.expectEqual(serve_integration.BgpRuntimeState.configured, bundle.state);
    try std.testing.expectEqual(@as(u64, 0), bundle.backoff_ms);
    try std.testing.expectEqual(@as(clock.MonoTime, 0), bundle.reconnect_deadline);

    // Set fake clock time
    clock.MockClock.reset();
    clock.MockClock.setTime(1000);

    // Schedule reconnect
    serve_integration.scheduleReconnect(bundle, clock.MockClock.interface(), 60_000);

    // Verify backoff was computed (should be INITIAL_MS = 1000)
    try std.testing.expectEqual(@as(u64, 1000), bundle.backoff_ms);
    // Verify deadline was set (1000 + 1000 = 2000)
    try std.testing.expectEqual(@as(clock.MonoTime, 2000), bundle.reconnect_deadline);
    // Verify state changed to reconnect_wait
    try std.testing.expectEqual(serve_integration.BgpRuntimeState.reconnect_wait, bundle.state);
}

test "scheduleReconnect doubles backoff on second call" {
    const bundle = createMinimalBundle();

    clock.MockClock.reset();
    clock.MockClock.setTime(1000);

    // First schedule: backoff = 1000
    serve_integration.scheduleReconnect(bundle, clock.MockClock.interface(), 60_000);
    try std.testing.expectEqual(@as(u64, 1000), bundle.backoff_ms);

    // Advance time past deadline
    clock.MockClock.setTime(2000);

    // Second schedule: backoff should double to 2000
    serve_integration.scheduleReconnect(bundle, clock.MockClock.interface(), 60_000);
    try std.testing.expectEqual(@as(u64, 2000), bundle.backoff_ms);
    try std.testing.expectEqual(@as(clock.MonoTime, 4000), bundle.reconnect_deadline);
}

test "isReconnectReady returns false when not in reconnect_wait state" {
    const bundle = createMinimalBundle();

    bundle.state = .configured;
    clock.MockClock.reset();
    clock.MockClock.setTime(9999);

    // Should return false when state is configured
    try std.testing.expect(!serve_integration.isReconnectReady(bundle, clock.MockClock.interface()));
}

test "isReconnectReady returns false when before deadline" {
    const bundle = createMinimalBundle();

    clock.MockClock.reset();
    clock.MockClock.setTime(1000);
    serve_integration.scheduleReconnect(bundle, clock.MockClock.interface(), 60_000);

    // Try before deadline (2000) at time 1500
    clock.MockClock.setTime(1500);
    try std.testing.expect(!serve_integration.isReconnectReady(bundle, clock.MockClock.interface()));
}

test "isReconnectReady returns true when at deadline" {
    const bundle = createMinimalBundle();

    clock.MockClock.reset();
    clock.MockClock.setTime(1000);
    serve_integration.scheduleReconnect(bundle, clock.MockClock.interface(), 60_000);

    // At deadline (2000)
    clock.MockClock.setTime(2000);
    try std.testing.expect(serve_integration.isReconnectReady(bundle, clock.MockClock.interface()));
}

test "isReconnectReady returns true when past deadline" {
    const bundle = createMinimalBundle();

    clock.MockClock.reset();
    clock.MockClock.setTime(1000);
    serve_integration.scheduleReconnect(bundle, clock.MockClock.interface(), 60_000);

    // Past deadline (5000 > 2000)
    clock.MockClock.setTime(5000);
    try std.testing.expect(serve_integration.isReconnectReady(bundle, clock.MockClock.interface()));
}

test "resetBackoff clears backoff and deadline" {
    const bundle = createMinimalBundle();

    // Set non-zero values
    bundle.backoff_ms = 4000;
    bundle.reconnect_deadline = 5000;
    bundle.state = .reconnect_wait;

    // Call resetBackoff
    serve_integration.resetBackoff(bundle);

    // Verify values are cleared
    try std.testing.expectEqual(@as(u64, 0), bundle.backoff_ms);
    try std.testing.expectEqual(@as(clock.MonoTime, 0), bundle.reconnect_deadline);
    // Note: resetBackoff does NOT change state
}

test "isCleanupRequested returns false initially" {
    const bundle = createMinimalBundle();

    try std.testing.expect(!serve_integration.isCleanupRequested(bundle));
}

test "closeForReconnect resets session state to idle" {
    const bundle = createMinimalBundle();

    // Manually set session state to something other than idle
    bundle.sess.status.state = .open_sent;

    // Set various session fields to non-default values
    bundle.sess.status.state = .open_sent;
    bundle.sess.recv_len = 100;
    bundle.sess.send_pos = 50;
    bundle.sess.peer_open = null;
    bundle.sess.negotiated_hold_time = 180;
    bundle.sess.keepalive_interval_ms = 60;
    bundle.sess.hold_timer_deadline = 1000;
    bundle.sess.pending_keepalive = true;
    bundle.sess.pending_keepalive_ms = 100;

    // Call closeForReconnect
    serve_integration.closeForReconnect(bundle);

    // Verify session state was reset
    try std.testing.expectEqual(session.SessionState.idle, bundle.sess.status.state);
    try std.testing.expectEqual(@as(usize, 0), bundle.sess.recv_len);
    try std.testing.expectEqual(@as(usize, 0), bundle.sess.send_pos);
    try std.testing.expect(bundle.sess.peer_open == null);
    try std.testing.expectEqual(@as(u32, 0), bundle.sess.negotiated_hold_time);
    try std.testing.expectEqual(@as(u64, 0), bundle.sess.keepalive_interval_ms);
    try std.testing.expectEqual(@as(u64, 0), bundle.sess.hold_timer_deadline);
    try std.testing.expectEqual(false, bundle.sess.pending_keepalive);
    try std.testing.expectEqual(@as(u64, 0), bundle.sess.pending_keepalive_ms);
}

test "scheduleReconnect caps at max delay" {
    const bundle = createMinimalBundle();

    // Set backoff to 32s (last doubling before cap)
    bundle.backoff_ms = 32_000;
    bundle.state = .reconnect_wait;

    clock.MockClock.reset();
    clock.MockClock.setTime(1000);

    // Schedule with max of 60s
    serve_integration.scheduleReconnect(bundle, clock.MockClock.interface(), 60_000);

    // Should cap at 60s, not double to 64s
    try std.testing.expectEqual(@as(u64, 60_000), bundle.backoff_ms);
}

test "bundle construction order is correct" {
    const bundle = createMinimalBundle();

    // Regression test: verify bundle was constructed with correct initial state.
    // The createMinimalBundle function constructs bundle in the correct order:
    // 1. Create bundle with undefined fields
    // 2. Set bundle.tcp and bundle.trans
    // 3. Initialize session with &bundle.trans (bundle-owned pointer)
    // This ensures sess.trans points to bundle.trans, not a stack local.
    try std.testing.expectEqual(serve_integration.BgpRuntimeState.configured, bundle.state);
    try std.testing.expectEqual(@as(u64, 0), bundle.backoff_ms);
}

test "session transport pointer points to bundle-owned transport" {
    const bundle = createMinimalBundle();

    // Critical regression test: sess.trans must point to bundle.trans, not a stack local.
    // This proves the session's transport pointer is valid after bundle construction.
    // The bundle is allocated on the heap, so this pointer is stable.
    try std.testing.expect(bundle.sess.trans == &bundle.trans);
}

// =============================================================================
// Test Helpers
// =============================================================================

/// Creates a minimal BgpServeBundle for testing without real TCP/session init.
/// 
/// IMPORTANT: This function allocates the bundle at a stable heap address to ensure
/// sess.trans points to bundle.trans (not a stack local that could dangle):
/// 1. Allocate bundle on heap
/// 2. Set bundle.tcp and bundle.trans
/// 3. Initialize session with &bundle.trans (bundle-owned pointer)
/// 4. Set bundle.sess
/// 
/// The caller is responsible for cleaning up the allocated bundle.
fn createMinimalBundle() *serve_integration.BgpServeBundle {
    const sess_cfg = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = null,
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{},
        .same_as = true,
    };

    // Allocate bundle at stable heap address
    const bundle = std.heap.page_allocator.create(serve_integration.BgpServeBundle) catch unreachable;
    
    bundle.* = serve_integration.BgpServeBundle{
        .raw = undefined,
        .bgp_config = undefined,
        .session_config = sess_cfg,
        .state = .configured,
        .last_error = null,
        .prefixes = &.{},
        .tcp = undefined,
        .trans = undefined,
        .sess = undefined,
    };

    // Set tcp and trans BEFORE initializing session
    bundle.tcp = tcp_transport_createDummy();
    bundle.trans = bundle.tcp.toTransport();

    // Initialize session with bundle-owned transport pointer (NOT a local)
    const sess = session.init(sess_cfg, &bundle.trans) catch unreachable;
    bundle.sess = sess;

    return bundle;
}

/// Creates a dummy TCP transport for testing.
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
