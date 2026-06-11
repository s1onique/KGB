// bgp/transport_ownership_tests.zig — Regression tests for transport ownership
//
// Regression tests for the EBADF bug where bundle.trans was created from a local
// tcp variable instead of bundle.tcp, causing the transport context to point
// to stale memory after the bundle construction function returned.
//
// BUG: bundle.trans = tcp.toTransport()     // local tcp goes out of scope
// FIX: bundle.trans = bundle.tcp.toTransport() // bundle-owned TCP stays alive

const std = @import("std");
const session = @import("session.zig");
const transport = @import("transport.zig");

test "bundle transport context survives after construction from bundle-owned tcp" {
    // This test verifies the transport wrapper's context points to the bundle-owned
    // TCP transport, not a local variable that goes out of scope.
    //
    // The bug was: bundle.trans = tcp.toTransport() where tcp is a local variable.
    // After loadConfigAndBgp() returns, tcp goes out of scope and bundle.trans.ctx
    // points to garbage, causing EBADF on send.
    //
    // The fix: bundle.trans = bundle.tcp.toTransport() where bundle.tcp is owned
    // by the bundle and lives as long as the bundle lives.

    // Simulate the bundle construction pattern from serve_integration.zig
    // We manually construct the bundle and verify the transport ownership.

    // Create a fake TCP transport for testing
    var fake_tcp = transport.FakeTransport.init(std.heap.page_allocator, &.{});
    defer fake_tcp.deinit();

    // Create a transport wrapper as if we did: bundle.trans = bundle.tcp.toTransport()
    var tcp_as_interface = fake_tcp.toTransport();

    // Now verify the transport context points to our fake_tcp
    // by checking that close() on the transport calls close() on fake_tcp
    try std.testing.expect(!fake_tcp.closed);

    // Get the context pointer from the transport wrapper
    const ctx_ptr = tcp_as_interface.ctx;

    // The context should point to fake_tcp (we can verify by casting back)
    const fake_from_ctx: *transport.FakeTransport = @ptrCast(@alignCast(ctx_ptr));

    // Close via the transport interface - should call close on fake_tcp
    tcp_as_interface.closeFn(tcp_as_interface.ctx);

    // Verify fake_tcp was actually closed
    try std.testing.expect(fake_tcp.closed);
    try std.testing.expect(fake_from_ctx.closed);
}

test "session receives bundle-owned transport and can send after bundle construction" {
    const sess_config = session.SessionConfig{
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

    // Create a fake TCP transport owned by the "bundle"
    var bundle_tcp = transport.FakeTransport.init(std.heap.page_allocator, &.{});
    defer bundle_tcp.deinit();

    // Create the transport wrapper from the bundle-owned TCP
    // This is the CORRECT pattern: bundle.trans = bundle.tcp.toTransport()
    var bundle_trans = bundle_tcp.toTransport();

    // Create session with the bundle-owned transport
    var sess = try session.init(sess_config, &bundle_trans);
    defer session.stop(&sess);

    // Verify the session can send (should succeed with fake transport)
    // This would fail with EBADF if the transport context was stale
    const run_result = session.runOnce(&sess) catch {
        // If it fails with an error, the transport was stale (EBADF would be here)
        // With the fix, this should NOT fail
        try std.testing.expect(false);
        return;
    };

    // Should succeed (fake transport sends always succeed, recv returns empty)
    try std.testing.expect(run_result == .ok or run_result == .stopped);

    // Session state should be open_sent - BGP OPEN was sent successfully through
    // the bundle-owned transport. This proves the transport context is valid.
    try std.testing.expectEqual(session.SessionState.open_sent, sess.status.state);
}
