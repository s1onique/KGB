// runtime/allocation_tracker_destroy.zig — non-committing shutdown
// validator for the bounded-memory instrumentation.
//
// Carries `DestroyError`, the lifetime-specific leak probe
// `hasLiveLifetime`, and the standalone `validateForDestroy` so the
// destroy-time contract (the precondition `destroyMemoryState`
// enforces inline via `@panic`) lives in its own small module. The
// split keeps `allocation_tracker_internal.zig` focused on the
// state-mutation surface (init/deinit, adoptHandle/releaseHandle,
// `trackingAllocator`, `finishReconnectBoundary`).
//
// `validateForDestroy` is intentionally NOT a committing
// validator: it returns `DestroyError` if the state still holds
// ANY tracked resource the orchestrator was supposed to have
// released. This is the canonical contract that
// `destroyMemoryState` enforces inline (via `@panic`); the
// standalone form here lets callers assert the precondition
// without actually deallocating the state, and exposes every
// failure as a structured error union instead of an immediate
// process abort.

const std = @import("std");
const internal = @import("allocation_tracker_internal.zig");

const AllocationLifetime = internal.AllocationLifetime;
const num_owners = internal.num_owners;
const AllocationMetrics = internal.AllocationMetrics;
const StateImpl = internal.StateImpl;
const ReconnectMemoryState = internal.ReconnectMemoryState;

/// Errors surfaced by `validateForDestroy`. The state is at rest
/// when none of these fire; any one of them means the orchestrator
/// still has tracked resources to release before the oracle can be
/// safely freed.
///
/// `PermanentLeak` covers a class of allocations that the
/// generation-boundary validator (`validateForGeneration`) and the
/// `finishReconnectBoundary` coordinator deliberately EXCLUDE: by
/// contract a `.permanent` classified allocator is allowed to
/// outlive any single reconnect-generation boundary. That is
/// exactly why the destroy validator MUST surface it as a
/// first-class error: if a `.permanent` allocator still holds live
/// bytes when the orchestrator wants to deinit the state, the
/// allocation points inside the very memory the oracle is about
/// to free. Without this check a classified allocator could be
/// referencing storage inside a destroyed `ReconnectMemoryState`,
/// and any subsequent access would be a use-after-free.
pub const DestroyError = error{
    /// `active_handle` was non-null at the boundary check.
    HandleStillAdopted,
    /// `handles_acquired` does not equal `handles_released`.
    HandleCountImbalance,
    /// `active_sockets` is non-zero.
    SocketStillOpen,
    /// `active_timers` is non-zero.
    TimerStillActive,
    /// A `reconnect_generation` lifetime still holds bytes.
    ReconnectGenerationLeak,
    /// An `operation` lifetime still holds bytes.
    OperationLeak,
    /// A `permanent` lifetime still holds bytes. Permanent
    /// allocations are allowed to survive reconnect-generation
    /// boundaries, but they MUST be released before the state is
    /// destroyed (the allocation points inside the very memory
    /// being freed).
    PermanentLeak,
};

/// Lifetime-specific leak probe: returns true iff ANY owner cell in
/// the given lifetime has either non-zero `live_bytes` OR non-zero
/// `current_allocations`. The `validateForDestroy` contract uses
/// BOTH signals because `live_bytes == 0` with `current_allocations
/// > 0` (or vice versa) represents a corrupted-but-not-yet-released
/// allocation: the bookkeeping can drift independently of the byte
/// total. A previous design collapsed these into a single
/// `total_live_bytes != baseline_live_bytes.?` comparison, which:
///   * force-unwrapped a missing baseline (panicked for fresh
///     states),
///   * made `OperationLeak` unreachable (it was declared but never
///     returned),
///   * conflated reconnect-generation drift, operation drift, and
///     permanent drift into one error.
pub fn hasLiveLifetime(
    metrics: *const AllocationMetrics,
    lifetime: AllocationLifetime,
) bool {
    for (0..num_owners) |oi| {
        const m = &metrics.owners[oi][@intFromEnum(lifetime)];
        if (m.live_bytes != 0 or m.current_allocations != 0) return true;
    }
    return false;
}

/// ACT-TOVARISCH-BOUNDED-MEMORY-RECONNECT-PROOF01-FA-3: non-committing
/// shutdown validator. Returns `DestroyError` if the state still
/// holds ANY tracked resource that the orchestrator was supposed
/// to have released. This is the canonical contract that
/// `destroyMemoryState` enforces inline (via `@panic`); the
/// standalone form here lets callers assert the precondition
/// without actually deallocating the state, and exposes every
/// failure as a structured error union instead of an immediate
/// process abort.
///
/// The check covers:
///   * the active handle identity (must be `null`)
///   * the acquire / release balance (must be equal)
///   * the live socket counter (must be `0`)
///   * the live timer counter (must be `0`)
///   * the `reconnect_generation` lifetime (live_bytes AND
///     current_allocations must both be zero across all owners)
///   * the `operation` lifetime (same)
///   * the `permanent` lifetime (same) — permanent allocations
///     are allowed to survive generation boundaries but they
///     MUST be released before the state itself is destroyed,
///     because the allocation points inside the very memory the
///     destroy will free.
///
/// `baseline_live_bytes` is NOT consulted directly: the warm-up
/// baseline is used by `finishReconnectBoundary` (which DOES
/// know when warm-up completes), but the destroy validator must
/// work against a fresh state where no baseline has been recorded
/// yet.
pub fn validateForDestroy(state: *const ReconnectMemoryState) DestroyError!void {
    const impl: *const StateImpl = @ptrCast(@alignCast(state));
    if (impl.active_handle != null) return error.HandleStillAdopted;
    if (impl.handles_acquired != impl.handles_released) return error.HandleCountImbalance;
    if (impl.resources.active_sockets != 0) return error.SocketStillOpen;
    if (impl.resources.active_timers != 0) return error.TimerStillActive;
    if (hasLiveLifetime(&impl.allocations, .reconnect_generation)) {
        return error.ReconnectGenerationLeak;
    }
    if (hasLiveLifetime(&impl.allocations, .operation)) {
        return error.OperationLeak;
    }
    if (hasLiveLifetime(&impl.allocations, .permanent)) {
        return error.PermanentLeak;
    }
}
