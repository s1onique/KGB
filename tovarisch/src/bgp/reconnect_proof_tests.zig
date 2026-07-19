// bgp/reconnect_proof_tests.zig — bounded-memory reconnect proof + state-
// ownership contract tests. The harness, connector types, and shared
// expectations live in `reconnect_proof_harness.zig` so this file stays
// under the LLM-friendliness hard limit.
//
// The 10,000-generation proof asserts absolute counts so deleting every
// acquire/release pair cannot return the proof to a vacuous `0 == 0`.

const std = @import("std");
const allocation_tracker = @import("../runtime/allocation_tracker.zig");
const clock = @import("clock.zig");
const harness = @import("reconnect_proof_harness.zig");
const serve_integration = @import("serve_integration.zig");

const ProductionReconnectHarness = harness.ProductionReconnectHarness;
const expectSessionBaseline = harness.expectSessionBaseline;
const expectBoundary = harness.expectBoundary;
const STRESS_TEST_GENERATIONS = harness.STRESS_TEST_GENERATIONS;
const WARMUP_GENERATIONS = harness.WARMUP_GENERATIONS;
const reconnect_boundary_socket_baseline = harness.reconnect_boundary_socket_baseline;
const RECONNECT_PEAK_SOCKET_BOUND = harness.RECONNECT_PEAK_SOCKET_BOUND;

test "production reconnect path is memory-neutral across 10,000 failed generations" {
    clock.MockClock.reset();
    var harness_state: ProductionReconnectHarness = .{};
    try harness_state.init(std.testing.allocator);
    try harness_state.setup();
    defer harness_state.deinit(std.testing.allocator);

    var generation: u64 = 0;
    while (generation < STRESS_TEST_GENERATIONS) : (generation += 1) {
        try harness_state.runOneFailedReconnectGeneration();
        try allocation_tracker.finishReconnectBoundary(harness_state.state, reconnect_boundary_socket_baseline);
        try std.testing.expectEqual(generation + 1, allocation_tracker.generation(harness_state.state));

        const bundle = harness_state.bundle.?;
        try expectBoundary(harness_state.state, bundle);

        try std.testing.expectEqual(allocation_tracker.generation(harness_state.state), generation + 1);
        try std.testing.expectEqual(allocation_tracker.generation(harness_state.state), bundle.reconnect_count);
    }

    const memory = allocation_tracker.memorySnapshot(harness_state.state);
    const resources = allocation_tracker.resourceSnapshot(harness_state.state);
    const handles = allocation_tracker.handleSnapshot(harness_state.state);

    try std.testing.expectEqual(STRESS_TEST_GENERATIONS, allocation_tracker.generation(harness_state.state));
    // Absolute count assertions — without these, deleting every
    // acquire/release pair could return the proof to a vacuous `0 == 0`.
    try std.testing.expectEqual(STRESS_TEST_GENERATIONS, harness_state.connector.connect_attempts);
    try std.testing.expectEqual(STRESS_TEST_GENERATIONS, harness_state.connector.physical_resource_acquired);
    try std.testing.expectEqual(STRESS_TEST_GENERATIONS, harness_state.connector.physical_resource_released);
    try std.testing.expectEqual(@as(u32, 0), harness_state.connector.physical_resource_live);
    try std.testing.expectEqual(STRESS_TEST_GENERATIONS, handles.handles_acquired);
    try std.testing.expectEqual(STRESS_TEST_GENERATIONS, handles.handles_released);
    try std.testing.expectEqual(STRESS_TEST_GENERATIONS, handles.release_calls);
    try std.testing.expectEqual(@as(i128, 0), handles.delta);
    try std.testing.expect(handles.active_handle == null);
    try std.testing.expectEqual(RECONNECT_PEAK_SOCKET_BOUND, resources.peak_sockets);

    std.debug.print(
        "RECONNECT_PROOF completed_generations={d} warmup_generations={d} baseline_live_bytes={d} final_live_bytes={d} baseline_delta_bytes={d} peak_total_live_bytes={d} reconnect_live_bytes_at_end={d} peak_active_sockets={d} active_sockets_at_end={d} peak_active_timers={d} active_timers_at_end={d} total_reconnect_attempts={d} handle_probe_delta={d}\n",
        .{
            allocation_tracker.generation(harness_state.state),
            WARMUP_GENERATIONS,
            allocation_tracker.baselineLiveBytes(harness_state.state).?,
            memory.live_bytes,
            memory.baseline_delta_bytes,
            allocation_tracker.totalPeakBytes(harness_state.state),
            memory.reconnect_live_bytes,
            resources.peak_sockets,
            resources.active_sockets,
            resources.peak_timers,
            resources.active_timers,
            harness_state.connector.connect_attempts,
            handles.delta,
        },
    );
}

test "state oracle rejects a skipped releaseHandle (leaked active handle)" {
    // Skipping the entire releaseHandle leaves the state's active handle
    // outstanding. The boundary must reject the commit.
    clock.MockClock.reset();
    var harness_state: ProductionReconnectHarness = .{};
    try harness_state.init(std.testing.allocator);
    try harness_state.setup();
    defer harness_state.deinit(std.testing.allocator);
    try harness_state.runOneSuccessfulReconnect();
    harness_state.faults = .{ .skip_release_handle = true };
    const bundle = harness_state.bundle.?;
    serve_integration.closeForReconnect(bundle);
    const handles = allocation_tracker.handleSnapshot(harness_state.state);
    try std.testing.expectEqual(@as(u64, 1), handles.handles_acquired);
    try std.testing.expectEqual(@as(u64, 0), handles.handles_released);
    try std.testing.expect(handles.active_handle != null);
    try std.testing.expectEqual(@as(i128, 1), handles.delta);
    try std.testing.expectError(
        error.HandleLeak,
        allocation_tracker.finishReconnectBoundary(harness_state.state, reconnect_boundary_socket_baseline),
    );
    try std.testing.expectEqual(@as(u64, 0), allocation_tracker.generation(harness_state.state));
    // Cleanup: clear faults; bundle.active_connector_handle is
    // preserved across the skipped release, so a normal closeForReconnect
    // can now release through releaseHandle.
    harness_state.faults = .{};
    serve_integration.closeForReconnect(bundle);
    const final = allocation_tracker.handleSnapshot(harness_state.state);
    try std.testing.expectEqual(@as(i128, 0), final.delta);
    try std.testing.expect(final.active_handle == null);
}



test "mutation B: production oracle detects an omitted socket close" {
    clock.MockClock.reset();
    var harness_state: ProductionReconnectHarness = .{};
    try harness_state.init(std.testing.allocator);
    try harness_state.setup();
    defer harness_state.deinit(std.testing.allocator);
    try harness_state.runOneSuccessfulReconnect();
    harness_state.faults = .{ .skip_socket_close = true };
    try harness_state.runOneFailedReconnectGeneration();
    try std.testing.expectError(
        error.SocketLeak,
        allocation_tracker.finishReconnectBoundary(harness_state.state, reconnect_boundary_socket_baseline),
    );
    try std.testing.expectEqual(@as(u64, 0), allocation_tracker.generation(harness_state.state));
}

test "mutation C: production oracle detects an omitted timer cancellation" {
    clock.MockClock.reset();
    var harness_state: ProductionReconnectHarness = .{};
    try harness_state.init(std.testing.allocator);
    try harness_state.setup();
    defer harness_state.deinit(std.testing.allocator);
    harness_state.faults = .{ .skip_timer_stop = true };
    try harness_state.runOneFailedReconnectGeneration();
    try std.testing.expectError(
        error.TimerLeak,
        allocation_tracker.finishReconnectBoundary(harness_state.state, reconnect_boundary_socket_baseline),
    );
    try std.testing.expectEqual(@as(u64, 0), allocation_tracker.generation(harness_state.state));
}

test "mutation D: oracle detects permanent growth after warm-up" {
    clock.MockClock.reset();
    var harness_state: ProductionReconnectHarness = .{};
    try harness_state.init(std.testing.allocator);
    try harness_state.setup();
    defer harness_state.deinit(std.testing.allocator);

    var generation: u64 = 0;
    while (generation < WARMUP_GENERATIONS) : (generation += 1) {
        try harness_state.runOneFailedReconnectGeneration();
        try allocation_tracker.finishReconnectBoundary(harness_state.state, reconnect_boundary_socket_baseline);
    }

    const permanent_allocator = try allocation_tracker.trackingAllocator(
        harness_state.state,
        std.testing.allocator,
        .bgp_session,
        .permanent,
    );
    const leaked = try permanent_allocator.alloc(u8, 1);
    try std.testing.expectError(
        error.BaselineDrift,
        allocation_tracker.finishReconnectBoundary(harness_state.state, reconnect_boundary_socket_baseline),
    );
    try std.testing.expectEqual(WARMUP_GENERATIONS, allocation_tracker.generation(harness_state.state));
    permanent_allocator.free(leaked);
}

test "state oracle: adoptHandle rejects double-acquire" {
    const state = try allocation_tracker.init(std.testing.allocator);
    defer allocation_tracker.deinit(state, std.testing.allocator);

    var dummy_a: u32 = 0;
    var dummy_b: u32 = 0;
    const cb = struct {
        fn noop(_: *anyopaque) void {}
    }.noop;
    const h1 = allocation_tracker.ReconnectHandle{ .ptr = @ptrCast(&dummy_a), .release_fn = cb };
    try allocation_tracker.adoptHandle(state, h1);
    const h2 = allocation_tracker.ReconnectHandle{ .ptr = @ptrCast(&dummy_b), .release_fn = cb };
    try std.testing.expectError(error.HandleAlreadyActive, allocation_tracker.adoptHandle(state, h2));
    // Clean up.
    try allocation_tracker.releaseHandle(state, h1);
}

test "state oracle: releaseHandle fires the physical release_fn exactly once" {
    const state = try allocation_tracker.init(std.testing.allocator);
    defer allocation_tracker.deinit(state, std.testing.allocator);

    const Counter = struct {
        value: u64 = 0,
    };
    var counter: Counter = .{};
    const release_callback: allocation_tracker.reconnect_handle_release_fn = struct {
        fn cb(ptr: *anyopaque) void {
            const c: *Counter = @ptrCast(@alignCast(ptr));
            c.value += 1;
        }
    }.cb;
    const handle = allocation_tracker.ReconnectHandle{
        .ptr = @ptrCast(&counter),
        .release_fn = release_callback,
    };

    try allocation_tracker.adoptHandle(state, handle);
    try allocation_tracker.releaseHandle(state, handle);

    try std.testing.expectEqual(@as(u64, 1), counter.value);
    const snap = allocation_tracker.handleSnapshot(state);
    try std.testing.expectEqual(@as(u64, 1), snap.handles_acquired);
    try std.testing.expectEqual(@as(u64, 1), snap.handles_released);
    try std.testing.expectEqual(@as(u64, 1), snap.release_calls);
    try std.testing.expect(snap.active_handle == null);
    try std.testing.expectEqual(@as(i128, 0), snap.delta);
}

test "state oracle: releaseHandle rejects a wrong handle without invoking release_fn" {
    const state = try allocation_tracker.init(std.testing.allocator);
    defer allocation_tracker.deinit(state, std.testing.allocator);

    const Counter = struct {
        value: u64 = 0,
    };
    var counter_a: Counter = .{};
    const release_callback_a: allocation_tracker.reconnect_handle_release_fn = struct {
        fn cb(ptr: *anyopaque) void {
            const c: *Counter = @ptrCast(@alignCast(ptr));
            c.value += 1;
        }
    }.cb;

    const h_correct = allocation_tracker.ReconnectHandle{
        .ptr = @ptrCast(&counter_a),
        .release_fn = release_callback_a,
    };
    var wrong: u32 = 0;
    const h_wrong = allocation_tracker.ReconnectHandle{
        .ptr = @ptrCast(&wrong),
        .release_fn = release_callback_a,
    };

    try allocation_tracker.adoptHandle(state, h_correct);
    try std.testing.expectError(error.WrongHandle, allocation_tracker.releaseHandle(state, h_wrong));
    try std.testing.expectEqual(@as(u64, 0), counter_a.value);
    // Still adoptable: clean up with the matching handle.
    try allocation_tracker.releaseHandle(state, h_correct);
    try std.testing.expectEqual(@as(u64, 1), counter_a.value);
}

// ACT-TOVARISCH-BOUNDED-MEMORY-RECONNECT-PROOF01-FA: forged-handle
// regression. The state's `releaseHandle` MUST verify BOTH `ptr` and
// `release_fn` before invoking either callback. A forged release that
// shares the legitimate `ptr` but supplies a different `release_fn`
// returns `error.WrongHandle`, invokes NEITHER physical callback,
// leaves the original handle active, and does NOT advance
// `handles_released`.
//
// Design note: the two callbacks cast the same `ptr` to the same
// concrete type (`*u32`) so alignment cannot unfairly reject one
// but not the other. The "forgery" is therefore in the callback
// function pointer, not in any layout property the alignment check
// can see.
test "state oracle: releaseHandle rejects a forged release_fn on the same pointer" {
    const state = try allocation_tracker.init(std.testing.allocator);
    defer allocation_tracker.deinit(state, std.testing.allocator);

    // Tag storage: both callbacks write distinct sentinel values to
    // the shared `u32` storage so the test can assert which path ran.
    var storage: u32 = 0;
    const LEGIT_TAG: u32 = 0x4C45_4749; // 'LEGI'
    const FORGED_TAG: u32 = 0xDEAD_BEEF;

    const legitimate_cb: allocation_tracker.reconnect_handle_release_fn = struct {
        fn cb(ptr: *anyopaque) void {
            const cell: *u32 = @ptrCast(@alignCast(ptr));
            cell.* = 0x4C45_4749; // LEGIT_TAG
        }
    }.cb;
    const forged_cb: allocation_tracker.reconnect_handle_release_fn = struct {
        fn cb(ptr: *anyopaque) void {
            const cell: *u32 = @ptrCast(@alignCast(ptr));
            cell.* = 0xDEAD_BEEF; // FORGED_TAG
        }
    }.cb;

    // Legitimate handle: same ptr, legitimate callback.
    const legit_handle = allocation_tracker.ReconnectHandle{
        .ptr = @ptrCast(&storage),
        .release_fn = legitimate_cb,
    };
    // Forged handle: SHARED ptr, DIFFERENT release_fn. The previous
    // (pointer-only) oracle accepted this. The current oracle MUST
    // reject it as `error.WrongHandle` without invoking either
    // callback so neither tag ends up in `storage`.
    const forged_handle = allocation_tracker.ReconnectHandle{
        .ptr = @ptrCast(&storage),
        .release_fn = forged_cb,
    };

    try allocation_tracker.adoptHandle(state, legit_handle);

    // The forged release must be rejected outright, with no callback
    // invoked (i.e. `storage` stays at its initial 0).
    try std.testing.expectError(
        error.WrongHandle,
        allocation_tracker.releaseHandle(state, forged_handle),
    );
    try std.testing.expectEqual(@as(u32, 0), storage);

    // The state's accounting is untouched by the rejected forged
    // release.
    const after_forge = allocation_tracker.handleSnapshot(state);
    try std.testing.expect(after_forge.active_handle != null);
    try std.testing.expectEqual(@as(u64, 1), after_forge.handles_acquired);
    try std.testing.expectEqual(@as(u64, 0), after_forge.handles_released);
    try std.testing.expectEqual(@as(u64, 0), after_forge.release_calls);

    // The forged handle shares the recorded `ptr` but a different
    // `release_fn`; assert the surfaced token still matches the
    // LEGITIMATE handle (i.e. the state did not silently adopt the
    // forged token).
    const recorded_ptr: *anyopaque = after_forge.active_handle.?.ptr;
    try std.testing.expectEqual(
        @as(*const u32, @ptrCast(&storage)),
        @as(*const u32, @alignCast(@ptrCast(recorded_ptr))),
    );
    try std.testing.expectEqual(legitimate_cb, after_forge.active_handle.?.release_fn);

    // Now release with the legitimate handle. The forged callback
    // MUST NOT have run, and the legitimate callback writes LEGIT_TAG.
    try allocation_tracker.releaseHandle(state, legit_handle);
    try std.testing.expectEqual(LEGIT_TAG, storage);

    const final = allocation_tracker.handleSnapshot(state);
    try std.testing.expect(final.active_handle == null);
    try std.testing.expectEqual(@as(u64, 1), final.handles_acquired);
    try std.testing.expectEqual(@as(u64, 1), final.handles_released);
    try std.testing.expectEqual(@as(u64, 1), final.release_calls);

    // Sanity: the FORGED_TAG must never appear in `storage` even after
    // a successful (legitimate) release path.
    try std.testing.expect(FORGED_TAG != storage);
}

test "state oracle: finishReconnectBoundary rejects when an active handle is outstanding" {
    const state = try allocation_tracker.init(std.testing.allocator);
    defer allocation_tracker.deinit(state, std.testing.allocator);

    var dummy: u32 = 0;
    const cb = struct {
        fn noop(_: *anyopaque) void {}
    }.noop;
    const handle = allocation_tracker.ReconnectHandle{ .ptr = @ptrCast(&dummy), .release_fn = cb };
    try allocation_tracker.adoptHandle(state, handle);
    try std.testing.expectError(
        error.HandleLeak,
        allocation_tracker.finishReconnectBoundary(state, 0),
    );
    try allocation_tracker.releaseHandle(state, handle);
    try allocation_tracker.finishReconnectBoundary(state, 0);
    try std.testing.expectEqual(@as(u64, 1), allocation_tracker.generation(state));
}
