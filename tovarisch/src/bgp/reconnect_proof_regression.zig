// bgp/reconnect_proof_regression.zig — production connector regression.
//
// Verifies the production wiring under the new single-authority state:
//   * failed real attempt → the state's `adoptHandle` / `releaseHandle`
//     pair balance, `finishReconnectBoundary` succeeds, generation
//     advances.
//   * successful real attempt → the bundle becomes the SOLE owner of
//     the connected socket. `bundle.tcp.close()` runs exactly once;
//     no retained transport copy in `ProductionConnectorCtx`.
//   * skipped `releaseHandle` → the state's active handle stays
//     outstanding, the boundary refuses the commit, generation is
//     unchanged.
//
// A short-loopback thread accepts the connection during the success
// test. The regression is environment-independent: only loopback.

const std = @import("std");
const allocation_tracker = @import("../runtime/allocation_tracker.zig");
const clock = @import("clock.zig");
const reconnect_stress = @import("reconnect_stress_support.zig");
const serve_integration = @import("serve_integration.zig");
const session = @import("session.zig");
const tcp_transport = @import("tcp_transport.zig");
const types = @import("types.zig");

/// Bind a TCP listener on 127.0.0.1 via libc, capture the assigned
/// port, then close the listener. The returned port is (almost
/// certainly) free on loopback, so a follow-up connect attempt gets
/// ECONNREFUSED.
fn grabUnusedLoopbackPort() !u16 {
    const AF_INET: c_int = 2;
    const SOCK_STREAM: c_int = 1;
    const sock = std.c.socket(AF_INET, SOCK_STREAM, 0);
    if (sock < 0) return error.SocketFailed;
    defer _ = std.c.close(sock);
    var addr = std.mem.zeroes(std.c.sockaddr.in);
    addr.family = std.c.AF.INET;
    addr.port = std.mem.nativeToBig(u16, 0);
    addr.addr = std.mem.nativeToBig(u32, 0x7F000001); // 127.0.0.1
    if (std.c.bind(sock, @ptrCast(&addr), @sizeOf(std.c.sockaddr.in)) != 0) return error.BindFailed;
    if (std.c.listen(sock, 8) != 0) return error.ListenFailed;
    var local: std.c.sockaddr.in = undefined;
    var len: std.c.socklen_t = @sizeOf(std.c.sockaddr.in);
    if (std.c.getsockname(sock, @ptrCast(&local), &len) != 0) return error.GetsocknameFailed;
    return std.mem.bigToNative(u16, local.port);
}

/// Set up the bundle so that `runReconnectAttempt` will trigger the
/// production `realAcquire` / `realFinish` pair against `port`.
fn makeBundle(state: *allocation_tracker.ReconnectMemoryState, subsystem_alloc: std.mem.Allocator, port: u16, prefixes: []types.Ipv4Prefix) !*serve_integration.BgpServeBundle {
    const bundle = try subsystem_alloc.create(serve_integration.BgpServeBundle);
    errdefer subsystem_alloc.destroy(bundle);

    const cfg = reconnect_stress.makeTestSessionConfig();
    var bundle_cfg = cfg;
    bundle_cfg.peer_port = port;

    bundle.* = .{
        .raw = undefined,
        .bgp_config = undefined,
        .session_config = bundle_cfg,
        .state = .configured,
        .prefixes = prefixes,
        .tcp = .{
            .socket_fd = -1,
            .recv_buf = undefined,
            .recv_len = 0,
            .closed = true,
            .peer_address = .{ 127, 0, 0, 1 },
            .peer_port = port,
        },
        .trans = undefined,
        .sess = undefined,
        .allocator = subsystem_alloc,
        .reconnect_memory_state = state,
        .reconnect_connector = .{
            // Real default acquires via the production connector context.
            .ctx = @ptrCast(&bundle.production_connector_ctx),
        },
        .reconnect_clock = clock.MockClock.interface(),
    };
    bundle.trans = bundle.tcp.toTransport();
    bundle.sess = try session.initWithClock(bundle_cfg, &bundle.trans, clock.MockClock.interface());
    return bundle;
}

test "production connector: failed real attempt is observed by the state oracle" {
    clock.MockClock.reset();
    const port = try grabUnusedLoopbackPort();

    const state = try allocation_tracker.init(std.testing.allocator);
    defer allocation_tracker.deinit(state, std.testing.allocator);
    const subsystem_alloc = try allocation_tracker.trackingAllocator(state, std.testing.allocator, .bgp_subsystem, .permanent);
    var prefixes = [_]types.Ipv4Prefix{types.Ipv4Prefix.init("10.0.0.0/8")};

    const bundle = try makeBundle(state, subsystem_alloc, port, prefixes[0..]);
    defer subsystem_alloc.destroy(bundle);

    const test_clock = clock.MockClock.interface();
    serve_integration.scheduleReconnect(bundle, test_clock, serve_integration.DEFAULT_RECONNECT_MAX_MS);
    clock.MockClock.setTime(bundle.reconnect_deadline);
    if (!serve_integration.isReconnectReady(bundle, test_clock)) return error.ReconnectNotReady;

    // The connect attempt to a closed loopback port returns
    // ECONNREFUSED; the runtime surfaces that as ConnectionFailed.
    serve_integration.runReconnectAttempt(bundle, test_clock) catch {};

    // The state MUST have observed the failed attempt: adopt count ==
    // release count, active handle clear, release callback invoked
    // exactly once.
    const handles = allocation_tracker.handleSnapshot(state);
    try std.testing.expectEqual(@as(u64, 1), handles.handles_acquired);
    try std.testing.expectEqual(@as(u64, 1), handles.handles_released);
    try std.testing.expectEqual(@as(u64, 1), handles.release_calls);
    try std.testing.expect(handles.active_handle == null);
    try std.testing.expectEqual(@as(i128, 0), handles.delta);

    // ProductionConnectorCtx has no retained transport copy and no
    // pending in-flight flag.
    try std.testing.expectEqual(@as(?*tcp_transport.TcpTransport, null), bundle.production_connector_ctx.inflight);
    try std.testing.expect(!bundle.production_connector_ctx.acquire_inflight);

    // The boundary accepts the balanced handle accounting and the
    // generation advances.
    try allocation_tracker.finishReconnectBoundary(state, 0);
    try std.testing.expectEqual(@as(u64, 1), allocation_tracker.generation(state));
}

// ACT-TOVARISCH-BOUNDED-MEMORY-RECONNECT-PROOF01-FA production-init
// regression: prove that TWO sequential failed real reconnects both
// reach `realFinish`. Without `installMemoryState` wired into
// `loadConfigAndBgp` (i.e. the bundle's `reconnect_memory_state` is
// null on the first failed attempt), the production connector's
// `releaseOnErrdefer` cannot route through `releaseHandle`. The
// `ProductionConnectorCtx.acquire_inflight` flag stays true, and the
// SECOND `realAcquire` refuses at the gate with
// `error.HandleAlreadyActive` — production stops attempting
// connections entirely after its first failed reconnect.
test "production connector: two consecutive real failed reconnects both reach realFinish" {
    clock.MockClock.reset();
    const port = try grabUnusedLoopbackPort();

    const state = try allocation_tracker.init(std.testing.allocator);
    defer allocation_tracker.deinit(state, std.testing.allocator);
    const subsystem_alloc = try allocation_tracker.trackingAllocator(state, std.testing.allocator, .bgp_subsystem, .permanent);
    var prefixes = [_]types.Ipv4Prefix{types.Ipv4Prefix.init("10.0.0.0/8")};

    const bundle = try makeBundle(state, subsystem_alloc, port, prefixes[0..]);
    defer subsystem_alloc.destroy(bundle);

    // FIRST failed attempt.
    {
        const test_clock = clock.MockClock.interface();
        serve_integration.scheduleReconnect(bundle, test_clock, serve_integration.DEFAULT_RECONNECT_MAX_MS);
        clock.MockClock.setTime(bundle.reconnect_deadline);
        if (!serve_integration.isReconnectReady(bundle, test_clock)) return error.ReconnectNotReady;
        serve_integration.runReconnectAttempt(bundle, test_clock) catch {};

        const handles = allocation_tracker.handleSnapshot(state);
        try std.testing.expectEqual(@as(u64, 1), handles.handles_acquired);
        try std.testing.expectEqual(@as(u64, 1), handles.handles_released);
        try std.testing.expectEqual(@as(u64, 1), handles.release_calls);
        try std.testing.expect(!bundle.production_connector_ctx.acquire_inflight);
        try allocation_tracker.finishReconnectBoundary(state, 0);
        try std.testing.expectEqual(@as(u64, 1), allocation_tracker.generation(state));
    }

    // SECOND failed attempt. Without `installMemoryState` wired into
    // production, this would refuse at `realAcquire` with
    // `HandleAlreadyActive`. With the install,
    // `releaseOnErrdefer` cleared the flag and the second attempt
    // reaches `realFinish` cleanly. Counters balance to 2=2 across
    // both attempts.
    {
        const test_clock = clock.MockClock.interface();
        serve_integration.scheduleReconnect(bundle, test_clock, serve_integration.DEFAULT_RECONNECT_MAX_MS);
        clock.MockClock.setTime(bundle.reconnect_deadline);
        if (!serve_integration.isReconnectReady(bundle, test_clock)) return error.ReconnectNotReady;
        serve_integration.runReconnectAttempt(bundle, test_clock) catch {};

        const handles = allocation_tracker.handleSnapshot(state);
        try std.testing.expectEqual(@as(u64, 2), handles.handles_acquired);
        try std.testing.expectEqual(@as(u64, 2), handles.handles_released);
        try std.testing.expectEqual(@as(u64, 2), handles.release_calls);
        try std.testing.expect(handles.active_handle == null);
        try std.testing.expectEqual(@as(i128, 0), handles.delta);
        try std.testing.expect(!bundle.production_connector_ctx.acquire_inflight);
        try allocation_tracker.finishReconnectBoundary(state, 0);
        try std.testing.expectEqual(@as(u64, 2), allocation_tracker.generation(state));
    }
}

test "production connector: skipped releaseHandle keeps the handle outstanding and rejects the boundary" {
    clock.MockClock.reset();
    const port = try grabUnusedLoopbackPort();

    const state = try allocation_tracker.init(std.testing.allocator);
    defer allocation_tracker.deinit(state, std.testing.allocator);
    const subsystem_alloc = try allocation_tracker.trackingAllocator(state, std.testing.allocator, .bgp_subsystem, .permanent);
    var prefixes = [_]types.Ipv4Prefix{types.Ipv4Prefix.init("10.0.0.0/8")};

    const bundle = try makeBundle(state, subsystem_alloc, port, prefixes[0..]);
    defer subsystem_alloc.destroy(bundle);

    // Drive a successful real attach so a handle is currently adopted,
    // then SKIP the release on cleanup.
    const listener = std.c.socket(2, 1, 0);
    if (listener < 0) return error.SocketFailed;
    defer _ = std.c.close(listener);
    var addr = std.mem.zeroes(std.c.sockaddr.in);
    addr.family = std.c.AF.INET;
    addr.port = std.mem.nativeToBig(u16, port);
    addr.addr = std.mem.nativeToBig(u32, 0x7F000001);
    if (std.c.bind(listener, @ptrCast(&addr), @sizeOf(std.c.sockaddr.in)) != 0) return error.BindFailed;
    if (std.c.listen(listener, 8) != 0) return error.ListenFailed;

    const accept_ctx = struct {
        fn run(listener_fd: std.c.fd_t) void {
            var client_addr: std.c.sockaddr = undefined;
            var client_len: std.c.socklen_t = @sizeOf(std.c.sockaddr);
            const c = std.c.accept(listener_fd, &client_addr, &client_len);
            if (c >= 0) _ = std.c.close(c);
        }
    }.run;

    const thread = try std.Thread.spawn(.{}, accept_ctx, .{listener});
    defer thread.join();

    // Pre-condition: the state starts empty.
    const before = allocation_tracker.handleSnapshot(state);
    try std.testing.expectEqual(@as(u64, 0), before.handles_acquired);
    try std.testing.expect(before.active_handle == null);

    // First successful reconnect attempt — adopts a handle and
    // actually connects to the listener.
    const test_clock = clock.MockClock.interface();
    serve_integration.scheduleReconnect(bundle, test_clock, serve_integration.DEFAULT_RECONNECT_MAX_MS);
    clock.MockClock.setTime(bundle.reconnect_deadline);
    if (!serve_integration.isReconnectReady(bundle, test_clock)) return error.ReconnectNotReady;
    try serve_integration.runReconnectAttempt(bundle, test_clock);

    const after_adopt = allocation_tracker.handleSnapshot(state);
    try std.testing.expectEqual(@as(u64, 1), after_adopt.handles_acquired);
    try std.testing.expectEqual(@as(u64, 0), after_adopt.handles_released);
    try std.testing.expect(after_adopt.active_handle != null);
    try std.testing.expectEqual(@as(i128, 1), after_adopt.delta);

    // Now SKIP releaseHandle on cleanup. The state should observe an
    // outstanding handle, and the boundary must reject the commit.
    bundle.reconnect_faults = @constCast(&@as(serve_integration.ReconnectFaultPlan, .{ .skip_release_handle = true }));
    serve_integration.closeForReconnect(bundle);
    bundle.reconnect_faults = null;

    const after_skip = allocation_tracker.handleSnapshot(state);
    try std.testing.expectEqual(@as(u64, 1), after_skip.handles_acquired);
    try std.testing.expectEqual(@as(u64, 0), after_skip.handles_released);
    try std.testing.expect(after_skip.active_handle != null);
    try std.testing.expectEqual(@as(i128, 1), after_skip.delta);

    try std.testing.expectError(
        error.HandleLeak,
        allocation_tracker.finishReconnectBoundary(state, 0),
    );
    try std.testing.expectEqual(@as(u64, 0), allocation_tracker.generation(state));

    // Cleanup: bundle.active_connector_handle is preserved across the
    // skipped release, so a normal closeForReconnect can release
    // through releaseHandle.
    bundle.reconnect_faults = null;
    serve_integration.closeForReconnect(bundle);
    const final = allocation_tracker.handleSnapshot(state);
    try std.testing.expectEqual(@as(i128, 0), final.delta);
    try std.testing.expect(final.active_handle == null);
}
