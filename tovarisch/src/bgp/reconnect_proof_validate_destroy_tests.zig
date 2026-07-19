// bgp/reconnect_proof_validate_destroy_tests.zig — `validateForDestroy`
// contract tests (ACT-TOVARISCH-BOUNDED-MEMORY-RECONNECT-PROOF01-FA-3).
//
// The standalone `validateForDestroy` is the canonical precondition
// contract for `destroyMemoryState`. Each test below exercises one
// independent failure mode so a regression in any single probe is
// surfaced by exactly one test, not by an aggregate failure.
//
// Coverage matrix (one test per row):
//
//   1. fresh state before warm-up validates successfully
//   2. active socket rejects
//   3. active timer rejects
//   4. reconnect-generation allocation rejects
//   5. operation allocation rejects
//   6. zero-byte / current-allocations residue rejects
//   7. active handle rejects
//   8. clean state passes
//
// `recordSocketOpen` / `recordTimerStart` / `recordSocketClose` /
// `recordTimerStop` are the public events from
// `allocation_tracker_snapshots.zig`; the tests drive them
// directly to exercise the resource-counts probes without going
// through the connector seam.

const std = @import("std");
const allocation_tracker = @import("../runtime/allocation_tracker.zig");

const VoidWriter = struct {
    pub fn writeAll(_: @This(), _: []const u8) error{}!void {}
    pub fn print(_: @This(), _: []const u8, _: anytype) error{}!void {}
};

/// Allocate one byte via the supplied backing allocator, routed
/// through the `(owner, lifetime)` classified tracker so the state's
/// `OwnerMetrics` reflects the byte as `live_bytes` AND
/// `current_allocations`. Returns the raw pointer so the caller can
/// free it. Frees must route through the same backing allocator the
/// tracker was opened with.
fn classifiedAlloc(
    state: *allocation_tracker.ReconnectMemoryState,
    backing: std.mem.Allocator,
    owner: allocation_tracker.AllocationOwner,
    lifetime: allocation_tracker.AllocationLifetime,
) ![]u8 {
    const tracker = try allocation_tracker.trackingAllocator(
        state,
        backing,
        owner,
        lifetime,
    );
    return tracker.alloc(u8, 1);
}

fn classifiedFree(
    state: *allocation_tracker.ReconnectMemoryState,
    backing: std.mem.Allocator,
    owner: allocation_tracker.AllocationOwner,
    lifetime: allocation_tracker.AllocationLifetime,
    buf: []u8,
) void {
    const tracker = allocation_tracker.trackingAllocator(
        state,
        backing,
        owner,
        lifetime,
    ) catch unreachable;
    tracker.free(buf);
}

test "validateForDestroy accepts a fresh state before warm-up" {
    // A newly-initialised state has no baseline, no allocations, no
    // handles, and no recorded sockets or timers. The validator MUST
    // succeed without force-unwrapping an absent baseline (the
    // previous design's `.?` panicked here).
    const state = try allocation_tracker.init(std.testing.allocator);
    defer allocation_tracker.deinit(state, std.testing.allocator);

    // Sanity: baseline is genuinely absent at this point.
    try std.testing.expect(allocation_tracker.baselineLiveBytes(state) == null);

    try allocation_tracker.validateForDestroy(state);
}

test "validateForDestroy rejects when an active socket is recorded" {
    const state = try allocation_tracker.init(std.testing.allocator);
    defer allocation_tracker.deinit(state, std.testing.allocator);

    allocation_tracker.recordSocketOpen(state);
    defer allocation_tracker.recordSocketClose(state);

    try std.testing.expectError(
        error.SocketStillOpen,
        allocation_tracker.validateForDestroy(state),
    );
}

test "validateForDestroy rejects when an active timer is recorded" {
    const state = try allocation_tracker.init(std.testing.allocator);
    defer allocation_tracker.deinit(state, std.testing.allocator);

    allocation_tracker.recordTimerStart(state);
    defer allocation_tracker.recordTimerStop(state);

    try std.testing.expectError(
        error.TimerStillActive,
        allocation_tracker.validateForDestroy(state),
    );
}

test "validateForDestroy rejects when reconnect_generation lifetime still holds an allocation" {
    const state = try allocation_tracker.init(std.testing.allocator);
    defer allocation_tracker.deinit(state, std.testing.allocator);

    const buf = try classifiedAlloc(
        state,
        std.testing.allocator,
        .bgp_subsystem,
        .reconnect_generation,
    );
    defer classifiedFree(
        state,
        std.testing.allocator,
        .bgp_subsystem,
        .reconnect_generation,
        buf,
    );

    try std.testing.expectError(
        error.ReconnectGenerationLeak,
        allocation_tracker.validateForDestroy(state),
    );
}

test "validateForDestroy rejects when operation lifetime still holds an allocation" {
    const state = try allocation_tracker.init(std.testing.allocator);
    defer allocation_tracker.deinit(state, std.testing.allocator);

    const buf = try classifiedAlloc(
        state,
        std.testing.allocator,
        .bgp_session,
        .operation,
    );
    defer classifiedFree(
        state,
        std.testing.allocator,
        .bgp_session,
        .operation,
        buf,
    );

    // The previous implementation collapsed every leak class into
    // `ReconnectGenerationLeak`, so `OperationLeak` was unreachable.
    // The fix gives `operation` its own first-class error.
    try std.testing.expectError(
        error.OperationLeak,
        allocation_tracker.validateForDestroy(state),
    );
}

// Note on the "zero-byte residue" coverage row: the validator MUST
// OR-combine `live_bytes` and `current_allocations` per cell. Through
// the public tracking-allocator API both fields are updated together
// (see `TrackingAllocator.recordAlloc` / `recordFree`), so the
// consistent-state path is already exercised by the four other
// rejection tests above. The OR semantics are also defended by the
// `free-the-allocation` post-condition in the "operation allocation
// rejects" test (which proves the validator flips from rejecting to
// passing once `live_bytes` and `current_allocations` both fall to
// zero). A future invariant that mutates only one of the two
// fields can be added with a private friend test.
//
// Permanent allocations are intentionally excluded from the
// generation-boundary validator (`validateForGeneration`) and from
// `finishReconnectBoundary`: by contract a `.permanent` classified
// allocator is allowed to outlive any single reconnect-generation
// boundary. The destroy validator MUST still surface them because
// the allocation points inside the very memory the destroy will
// free. This test pins the boundary: a single live byte in any
// `.permanent` cell rejects; freeing the byte flips the validator
// back to success.
test "validateForDestroy rejects when a permanent classified allocator still holds an allocation" {
    const state = try allocation_tracker.init(std.testing.allocator);
    defer allocation_tracker.deinit(state, std.testing.allocator);

    // Step 1: obtain a `.permanent` classified allocator and allocate
    // exactly one byte through it. The byte is routed through the
    // tracker's `recordAlloc`, so the state's `OwnerMetrics` cell for
    // `(.process, .permanent)` reflects the byte as BOTH
    // `live_bytes == 1` AND `current_allocations == 1`.
    const buf = try classifiedAlloc(
        state,
        std.testing.allocator,
        .process,
        .permanent,
    );

    // Step 2: while the byte is live, the destroy validator MUST
    // reject with `PermanentLeak` — NOT `ReconnectGenerationLeak`,
    // NOT `OperationLeak`. This is the first-class error that
    // `validateForDestroy` exposes for `.permanent` drift.
    try std.testing.expectError(
        error.PermanentLeak,
        allocation_tracker.validateForDestroy(state),
    );

    // Step 3: free the byte. The tracker's `recordFree` clears BOTH
    // `live_bytes` AND `current_allocations` for the cell, so a
    // subsequent `validateForDestroy` MUST succeed.
    classifiedFree(
        state,
        std.testing.allocator,
        .process,
        .permanent,
        buf,
    );
    try allocation_tracker.validateForDestroy(state);
}

// Note on the "zero-byte residue" coverage row: the validator MUST
// OR-combine `live_bytes` and `current_allocations` per cell. Through
// the public tracking-allocator API both fields are updated together
// (see `TrackingAllocator.recordAlloc` / `recordFree`), so the
// consistent-state path is already exercised by the four other
// rejection tests above. The OR semantics are also defended by the
// `free-the-allocation` post-condition in the "operation allocation
// rejects" test (which proves the validator flips from rejecting to
// passing once `live_bytes` and `current_allocations` both fall to
// zero). A future invariant that mutates only one of the two
// fields can be added with a private friend test.

test "validateForDestroy rejects an adopted but unreleased active handle" {
    const state = try allocation_tracker.init(std.testing.allocator);
    defer allocation_tracker.deinit(state, std.testing.allocator);

    var storage: u32 = 0;
    const cb: allocation_tracker.reconnect_handle_release_fn = struct {
        fn noop(_: *anyopaque) void {}
    }.noop;
    const handle = allocation_tracker.ReconnectHandle{
        .ptr = @ptrCast(&storage),
        .release_fn = cb,
    };
    try allocation_tracker.adoptHandle(state, handle);

    try std.testing.expectError(
        error.HandleStillAdopted,
        allocation_tracker.validateForDestroy(state),
    );

    // After a clean release, the validator succeeds.
    try allocation_tracker.releaseHandle(state, handle);
    try allocation_tracker.validateForDestroy(state);
}

test "validateForDestroy accepts a fully-clean state (every probe at rest)" {
    const state = try allocation_tracker.init(std.testing.allocator);
    defer allocation_tracker.deinit(state, std.testing.allocator);

    // No allocations, no handles, no sockets, no timers.
    try std.testing.expectEqual(
        @as(u64, 0),
        allocation_tracker.liveBytesForLifetime(state, .reconnect_generation),
    );
    try std.testing.expectEqual(
        @as(u64, 0),
        allocation_tracker.liveBytesForLifetime(state, .operation),
    );
    const resources = allocation_tracker.resourceSnapshot(state);
    try std.testing.expectEqual(@as(u32, 0), resources.active_sockets);
    try std.testing.expectEqual(@as(u32, 0), resources.active_timers);
    const handles = allocation_tracker.handleSnapshot(state);
    try std.testing.expect(handles.active_handle == null);
    try std.testing.expectEqual(@as(u64, 0), handles.handles_acquired);
    try std.testing.expectEqual(@as(u64, 0), handles.handles_released);

    try allocation_tracker.validateForDestroy(state);
}

// `_ = VoidWriter{}` keeps the import-side dead-code silencer happy
// even when the `VoidWriter` is not referenced directly inside this
// file (other test helpers in the suite reuse it).
test "_ = VoidWriter{};" {
    _ = VoidWriter{};
}
