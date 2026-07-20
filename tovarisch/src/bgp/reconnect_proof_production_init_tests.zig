// bgp/reconnect_proof_production_init_tests.zig — production-init lifecycle
// regression.
//
// ACT-TOVARISCH-BOUNDED-MEMORY-RECONNECT-PROOF01-FA-3 production-init
// regression. Goes through the canonical production constructor
// (`serve_integration.initBgpServeBundle`) and the public cleanup
// helper (`serve_integration.cleanupBgpBundle`) so the test catches
// ordering defects in the real production path.
//
// Required invariants:
//
//   After `initBgpServeBundle` returns successfully:
//   - `reconnect_memory_state` is non-null (production installed it)
//   - the production connector context is heap-stable (ctx wired)
//   - the bundle's session and TCP transport are initialised
//   - the live socket is recorded with the state (so the
//     eventual `recordSocketClose` does not panic on a 0-count)
//
//   After `cleanupBgpBundle` returns with an active adopted handle:
//   - `closeForReconnect` ran FIRST and released the handle
//     (so destroyMemoryState would not see an active handle and
//      would not panic on the unbalanced-acquire guard)
//   - `destroyMemoryState` ran LAST and did NOT panic
//   - the production release callback fired exactly once
//
// To prove the cleanup-ordering invariant end-to-end the test
// establishes a precondition (`active handle present`,
// `acquired = 1`, `released = 0`) BEFORE invoking
// `cleanupBgpBundle`. Without this precondition BOTH ordering
// variants (`closeForReconnect` FIRST, `destroyMemoryState` FIRST)
// pass silently because the empty oracle has nothing to release
// in either order.

const std = @import("std");
const allocation_tracker = @import("../runtime/allocation_tracker.zig");
const config = @import("../config.zig");
const reconnect_stress = @import("reconnect_stress_support.zig");
const serve_integration = @import("serve_integration.zig");
const tcp_transport = @import("tcp_transport.zig");
const types = @import("types.zig");

const VoidWriter = struct {
    pub fn writeAll(_: @This(), _: []const u8) error{}!void {}
    pub fn print(_: @This(), _: []const u8, _: anytype) error{}!void {}
};

const ReleaseTracker = struct {
    release_calls: u64 = 0,
};

fn releaseCounter(ptr: *anyopaque) void {
    const t: *ReleaseTracker = @ptrCast(@alignCast(ptr));
    t.release_calls += 1;
}

/// A stub `TcpTransport` value with no live socket. We use a stack-
/// allocated placeholder so the test does not depend on the
/// host's network or a listener. The placeholder has `socket_fd = -1`
/// and `closed = true`, so both `bundle.tcp.close()` and the helpers
/// `cleanupBgpBundle` invokes (`closeForReconnect`,
/// `cancelReconnectTimer`, etc.) become no-ops against the socket —
/// which is exactly the contract the production initialization helper
/// expects when the initial TCP connect succeeds into a then-closed
/// transport on the test side.
fn closedStubTransport() tcp_transport.TcpTransport {
    return .{
        .socket_fd = -1,
        .recv_buf = undefined,
        .recv_len = 0,
        .closed = true,
        .peer_address = .{ 0, 0, 0, 0 },
        .peer_port = 0,
    };
}

test "production init via initBgpServeBundle installs and cleanupBgpBundle destroys state LAST (with active handle)" {
    // 1. Build pre-parsed inputs that `initBgpServeBundle` accepts.
    //    Use a closed stub transport so the test does not require
    //    a live peer address.
    var stub_tcp = closedStubTransport();

    var raw = config.RawConfig{};
    defer raw.deinit(std.testing.allocator);

    // Note: ownership of the prefixes slice transfers to the bundle;
    // `cleanupBgpBundle` frees it via the allocator argument, so this
    // test must NOT also free it in a defer.
    const prefixes = try std.testing.allocator.alloc(types.Ipv4Prefix, 1);
    prefixes[0] = types.Ipv4Prefix.init("10.0.0.0/8");
    const cfg = reconnect_stress.makeTestSessionConfig();

    const stderr = VoidWriter{};
    const load = serve_integration.initBgpServeBundle(
        raw,
        .{ .present = true, .enabled = true },
        cfg,
        &stub_tcp,
        prefixes[0..],
        stderr,
        std.testing.allocator,
    );
    const bundle = switch (load) {
        .configured => |b| b,
        else => |t| {
            std.debug.print("initBgpServeBundle failed: {s}\n", .{@tagName(t)});
            return error.LoadFailed;
        },
    };

    // 2. Post-construction invariants: the canonical constructor
    //    installed the state and wired everything through, including
    //    the initial `recordSocketOpen` (the FA-3 socket accounting
    //    fix). The state snapshot MUST show `active_sockets = 1`
    //    (the live transport) and zero handles — the bundle has not
    //    yet done a reconnect.
    try std.testing.expect(bundle.reconnect_memory_state != null);
    const state = bundle.reconnect_memory_state.?;
    const initial = allocation_tracker.handleSnapshot(state);
    try std.testing.expectEqual(@as(u64, 0), initial.handles_acquired);
    try std.testing.expectEqual(@as(u64, 0), initial.handles_released);
    try std.testing.expect(initial.active_handle == null);
    // The initial live socket is paired via `recordSocketOpen` in
    // `initBgpServeBundle`. We observe it through the dedicated
    // ResourceSnapshot rather than the HandleSnapshot.
    const initial_resources = allocation_tracker.resourceSnapshot(state);
    try std.testing.expectEqual(@as(u64, 1), initial_resources.active_sockets);
    try std.testing.expectEqual(@as(u64, 0), initial_resources.active_timers);

    // 3. Establish the precondition that the cleanup ordering must
    //    satisfy. We adopt a real handle into the state AND wire a
    //    release callback that increments an external counter. The
    //    counter is what makes the order swap observable from
    //    outside the test runner: it would fire zero times if
    //    `closeForReconnect` never ran.
    var release_tracker: ReleaseTracker = .{};
    const test_handle = allocation_tracker.ReconnectHandle{
        .ptr = @ptrCast(&release_tracker),
        .release_fn = releaseCounter,
    };
    // Adopt into the state (so the state's `active_handle` and
    // counters reflect the precondition) AND set
    // `bundle.active_connector_handle` (so the production
    // `closeForReconnect` path routes through `releaseHandle`).
    // The two fields are normally set together by
    // `reconnectTransport`; the test sets them explicitly to
    // exercise the release path through the bundle field.
    try allocation_tracker.adoptHandle(state, test_handle);
    bundle.active_connector_handle = test_handle;
    const adopted = allocation_tracker.handleSnapshot(state);
    try std.testing.expectEqual(@as(u64, 1), adopted.handles_acquired);
    try std.testing.expectEqual(@as(u64, 0), adopted.handles_released);
    try std.testing.expect(adopted.active_handle != null);

    // 4. Run the production cleanup path. With the precondition in
    //    place:
    //      * `closeForReconnect` runs FIRST and routes the adopted
    //        handle through `releaseHandle`, which calls our
    //        `releaseCounter` and decrements `active_handle` to
    //        null. `recordSocketClose` then balances `active_sockets`
    //        from 1 to 0.
    //      * `destroyMemoryState` runs LAST and would PANIC if called
    //        BEFORE `closeForReconnect` (it checks `active_handle ==
    //        null` and `acquired == released`).
    serve_integration.cleanupBgpBundle(bundle, std.testing.allocator);

    // 5. The release callback fired exactly once. If `closeForReconnect`
    //    had been skipped (e.g. because the cleanup ordering was
    //    wrong), the counter would still be 0 here.
    try std.testing.expectEqual(@as(u64, 1), release_tracker.release_calls);
}
