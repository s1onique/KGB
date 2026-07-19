// runtime/allocation_tracker_internal.zig — implementation surface for the
// bounded-memory instrumentation. Concrete metrics structs, the opaque
// `ReconnectMemoryState`, and all mutation/commit helpers live here, hidden
// from importing files.
//
// The state is **heap-allocated** and `opaque {}`; external callers obtain
// `*ReconnectMemoryState` exclusively through `init(allocator)` and release
// it with `deinit(state, allocator)`. The state is the **single authority**
// for connector-handle accounting: `active_handle` (which now stores the
// full `{ptr, release_fn}` token), `handles_acquired`, and
// `handles_released` live inside the state. Connectors only produce
// `ReconnectHandle` tokens; the orchestrator owns the transition through
// `adoptHandle`/`releaseHandle`, and `finishReconnectBoundary` reads only
// the bound counters.
//
// The `trackingAllocator(state, …)` factory is the only way to obtain a
// classified allocator; commit a generation boundary only via
// `finishReconnectBoundary(state, baseline_sockets)`.

const std = @import("std");
const tracking_allocator_sibling = @import("allocation_tracker_tracking_allocator.zig");
const connector_probe_sibling = @import("allocation_tracker_connector_probe.zig");

const TrackingAllocator = tracking_allocator_sibling.TrackingAllocator;
const ReconnectHandle = connector_probe_sibling.ReconnectHandle;

// ---------------------------------------------------------------------------
// Classification enums.
// ---------------------------------------------------------------------------

pub const AllocationOwner = enum(u4) { process = 0, bgp_subsystem = 1, bgp_session = 2 };
pub const AllocationLifetime = enum(u4) { permanent = 0, reconnect_generation = 1, operation = 2 };

pub const num_owners = std.enums.values(AllocationOwner).len;
pub const num_lifetimes = std.enums.values(AllocationLifetime).len;

comptime {
    std.debug.assert(@intFromEnum(AllocationOwner.process) == 0);
    std.debug.assert(@intFromEnum(AllocationOwner.bgp_subsystem) == 1);
    std.debug.assert(@intFromEnum(AllocationOwner.bgp_session) == 2);
    std.debug.assert(@intFromEnum(AllocationLifetime.permanent) == 0);
    std.debug.assert(@intFromEnum(AllocationLifetime.reconnect_generation) == 1);
    std.debug.assert(@intFromEnum(AllocationLifetime.operation) == 2);
}

// ---------------------------------------------------------------------------
// Metrics types. Kept `pub` so the in-package `TrackingAllocator` and
// snapshot helpers can construct them; intentionally NOT re-exported from
// the public surface so importing files cannot name `*AllocationMetrics`.
// ---------------------------------------------------------------------------

pub const OwnerMetrics = struct {
    live_bytes: u64 = 0,
    current_allocations: u32 = 0,
    total_peak_bytes: u64 = 0,
    total_allocation_count: u64 = 0,
    total_free_count: u64 = 0,
    generation_peak_bytes: u64 = 0,
    generation_allocation_count: u64 = 0,
    generation_free_count: u64 = 0,
};

pub const AllocationMetrics = struct {
    owners: [num_owners][num_lifetimes]OwnerMetrics = [_][num_lifetimes]OwnerMetrics{
        [_]OwnerMetrics{.{}} ** num_lifetimes,
        [_]OwnerMetrics{.{}} ** num_lifetimes,
        [_]OwnerMetrics{.{}} ** num_lifetimes,
    },
    baseline_live_bytes: ?u64 = null,
    total_live_bytes: u64 = 0,
    total_peak_bytes: u64 = 0,
    pub const warmup_generation_count: u64 = 100;

    fn getMut(self: *AllocationMetrics, owner: AllocationOwner, lifetime: AllocationLifetime) *OwnerMetrics {
        return &self.owners[@intFromEnum(owner)][@intFromEnum(lifetime)];
    }

    fn validateForGeneration(self: *const AllocationMetrics) LeakCheckError!void {
        inline for (std.enums.values(AllocationLifetime)) |lt| {
            if (lt == .permanent) continue;
            for (0..num_owners) |oi| {
                const m = self.owners[oi][@intFromEnum(lt)];
                if (m.live_bytes != 0) return LeakCheckError.LiveBytesRemaining;
                if (m.current_allocations != 0) return LeakCheckError.UnfreedAllocations;
            }
        }
    }

    fn validateBaseline(self: *const AllocationMetrics) LeakCheckError!void {
        if (self.baseline_live_bytes) |baseline| {
            if (self.total_live_bytes != baseline) return LeakCheckError.BaselineDrift;
        }
    }

    fn commitGeneration(self: *AllocationMetrics, next_generation: u64) void {
        if (self.baseline_live_bytes == null and next_generation == AllocationMetrics.warmup_generation_count) {
            self.baseline_live_bytes = self.total_live_bytes;
        }
        inline for (std.enums.values(AllocationLifetime)) |lt| {
            if (lt == .permanent) continue;
            for (0..num_owners) |oi| {
                self.owners[oi][@intFromEnum(lt)].generation_peak_bytes = 0;
                self.owners[oi][@intFromEnum(lt)].generation_allocation_count = 0;
                self.owners[oi][@intFromEnum(lt)].generation_free_count = 0;
            }
        }
    }
};

pub const LeakCheckError = error{ LiveBytesRemaining, UnfreedAllocations, BaselineDrift };

// ---------------------------------------------------------------------------
// Bounded resource counters. Public so the targeted tests can drive them.
// All increments use checked arithmetic; overflows are reported, not
// silently wrapped.
// ---------------------------------------------------------------------------

pub const BoundedResourceCounters = struct {
    active_sockets: u32 = 0,
    peak_sockets: u32 = 0,
    active_timers: u32 = 0,
    peak_timers: u32 = 0,
    error_history_count: u32 = 0,
    retry_collection_count: u32 = 0,
    error_history_capacity: u32 = 16,
    retry_collection_capacity: u32 = 64,

    pub fn recordSocketOpen(self: *BoundedResourceCounters) void {
        self.active_sockets = std.math.add(u32, self.active_sockets, 1) catch @panic("socket count overflow");
        if (self.active_sockets > self.peak_sockets) self.peak_sockets = self.active_sockets;
    }
    pub fn recordSocketClose(self: *BoundedResourceCounters) void {
        if (self.active_sockets == 0) @panic("recordSocketClose: no active sockets to close");
        self.active_sockets -= 1;
    }
    pub fn recordTimerStart(self: *BoundedResourceCounters) void {
        self.active_timers = std.math.add(u32, self.active_timers, 1) catch @panic("timer count overflow");
        if (self.active_timers > self.peak_timers) self.peak_timers = self.active_timers;
    }
    pub fn recordTimerStop(self: *BoundedResourceCounters) void {
        if (self.active_timers == 0) @panic("recordTimerStop: no active timers to stop");
        self.active_timers -= 1;
    }
    pub fn tryReserveErrorHistorySlot(self: *BoundedResourceCounters) bool {
        if (self.error_history_count < self.error_history_capacity) {
            self.error_history_count = std.math.add(u32, self.error_history_count, 1) catch @panic("error_history overflow");
            return true;
        }
        return false;
    }
    pub fn releaseErrorHistorySlot(self: *BoundedResourceCounters) void {
        if (self.error_history_count == 0) @panic("releaseErrorHistorySlot: no reserved slot");
        self.error_history_count -= 1;
    }
    pub fn tryReserveRetryCollectionSlot(self: *BoundedResourceCounters) bool {
        if (self.retry_collection_count < self.retry_collection_capacity) {
            self.retry_collection_count = std.math.add(u32, self.retry_collection_count, 1) catch @panic("retry_collection overflow");
            return true;
        }
        return false;
    }
    pub fn releaseRetryCollectionSlot(self: *BoundedResourceCounters) void {
        if (self.retry_collection_count == 0) @panic("releaseRetryCollectionSlot: no reserved slot");
        self.retry_collection_count -= 1;
    }
    pub fn validateGenerationComplete(self: *const BoundedResourceCounters, baseline_sockets: u32) ResourceError!void {
        if (self.active_sockets != baseline_sockets) return ResourceError.SocketLeak;
        if (self.active_timers != 0) return ResourceError.TimerLeak;
    }
    pub const ResourceError = error{ SocketLeak, TimerLeak };
};

// ---------------------------------------------------------------------------
// StateImpl — concrete state. Lives only inside heap-allocated
// `ReconnectMemoryState`; never visible to application code.
//
// The state is the **single authority** for connector-handle accounting.
// `active_handle` (the full `{ptr, release_fn}` token), `handles_acquired`,
// `handles_released`, and `release_calls` are read via `handleSnapshot`.
// No external struct (no `ConnectorProbe`, no `HandleLedger`, no
// `HandleOracle`) can drive these counters: the only valid write paths
// are `adoptHandle` and `releaseHandle` on the opaque state itself.
//
// `releaseHandle` MUST verify BOTH fields of the stored token
// (`active_handle.ptr` and `active_handle.release_fn`) before invoking
// the physical callback. A forged handle that shares the original
// pointer but supplies a different `release_fn` returns
// `error.WrongHandle` without invoking either callback, so a mis-wired
// caller cannot accidentally close through a different physical cleanup
// path.
//
// Classified allocators are still bound to one backing allocator per cell;
// a different backing allocator for an already-bound cell returns
// `BackingAllocatorMismatch`.
// ---------------------------------------------------------------------------

pub const StateImpl = struct {
    generation: u64 = 0,
    allocations: AllocationMetrics = .{},
    resources: BoundedResourceCounters = .{},
    /// Authoritative handle accounting. Stores the FULL
    /// `{ptr, release_fn}` token so `releaseHandle` can verify identity
    /// against BOTH fields. External code reads via `handleSnapshot`
    /// (which exposes only the pointer for backward compatibility) and
    /// writes via `adoptHandle` / `releaseHandle` only.
    active_handle: ?ReconnectHandle = null,
    handles_acquired: u64 = 0,
    handles_released: u64 = 0,
    /// Number of times `release_fn` was actually invoked by
    /// `releaseHandle`. Lets tests assert that the physical callback ran
    /// even when the state rejected the release on identity mismatch.
    release_calls: u64 = 0,
    /// Per-class tracking allocators, lazy-initialised through `trackingAllocator`.
    /// Stored here so the metrics pointer can never escape the opaque boundary.
    trackers: [num_owners][num_lifetimes]?TrackingAllocator = [_][num_lifetimes]?TrackingAllocator{
        [_]?TrackingAllocator{null} ** num_lifetimes,
        [_]?TrackingAllocator{null} ** num_lifetimes,
        [_]?TrackingAllocator{null} ** num_lifetimes,
    },

    fn validateHandleAccounting(self: *const StateImpl) error{HandleLeak}!void {
        if (self.active_handle != null) return error.HandleLeak;
        if (self.handles_acquired != self.handles_released) return error.HandleLeak;
    }

    fn commitGeneration(self: *StateImpl, baseline_sockets: u32) ReconnectGenerationError!void {
        const next_generation = std.math.add(u64, self.generation, 1) catch @panic("generation count overflow");
        try self.allocations.validateForGeneration();
        try self.allocations.validateBaseline();
        try self.resources.validateGenerationComplete(baseline_sockets);
        self.allocations.commitGeneration(next_generation);
        self.generation = next_generation;
    }

    fn getOrCreateTracker(
        self: *StateImpl,
        owner: AllocationOwner,
        lifetime: AllocationLifetime,
        backing: std.mem.Allocator,
    ) BackingAllocatorError!*TrackingAllocator {
        const slot = &self.trackers[@intFromEnum(owner)][@intFromEnum(lifetime)];
        if (slot.*) |*existing| {
            if (!std.meta.eql(existing.backing, backing)) return BackingAllocatorError.BackingAllocatorMismatch;
            return existing;
        }
        slot.* = TrackingAllocator{
            .backing = backing,
            .metrics = &self.allocations,
            .owner = owner,
            .lifetime = lifetime,
        };
        return &(slot.*).?;
    }
};

pub const BackingAllocatorError = error{ BackingAllocatorMismatch };

/// Heap-allocated opaque state. Application code reaches the state through
/// `init(allocator)` / `deinit(state, allocator)`; the layout is hidden by
/// `opaque {}`.
pub const ReconnectMemoryState = opaque {};

/// Allocate a fresh state. The state owns its own handle oracle
/// (full-token `active_handle`, `handles_acquired`, `handles_released`)
/// for its entire lifetime — no external oracle argument is accepted,
/// so a differently-wired probe or ledger cannot shadow the state.
pub fn init(allocator: std.mem.Allocator) !*ReconnectMemoryState {
    const impl = try allocator.create(StateImpl);
    impl.* = StateImpl{};
    return @ptrCast(impl);
}

/// Release a state previously returned by `init`. Does not deallocate the
/// classified trackers (each `TrackingAllocator` defers to its backing
/// allocator and does not own heap memory of its own).
pub fn deinit(state: *ReconnectMemoryState, allocator: std.mem.Allocator) void {
    const impl: *StateImpl = @ptrCast(@alignCast(state));
    allocator.destroy(impl);
}

// `DestroyError`, `hasLiveLifetime`, and `validateForDestroy` moved
// to `allocation_tracker_destroy.zig` (FA-3 final file split; the
// destroy-time contract now lives in its own small module). The
// public surface (`allocation_tracker.zig`) re-exports the moved
// symbols so importing code is unaffected.

pub const ReconnectGenerationError = error{
    LiveBytesRemaining,
    UnfreedAllocations,
    BaselineDrift,
    SocketLeak,
    TimerLeak,
};

/// Atomic boundary coordinator error. `HandleLeak` covers BOTH an
/// outstanding active handle AND a misbalanced acquire/release total. The
/// rest mirror `ReconnectGenerationError`.
pub const BoundaryError = error{
    HandleLeak,
    LiveBytesRemaining,
    UnfreedAllocations,
    BaselineDrift,
    SocketLeak,
    TimerLeak,
};

/// Errors surfaced by `adoptHandle` and `releaseHandle`. Connectors MUST
/// let these propagate; callers MUST NOT swallow them silently.
pub const HandleError = error{
    /// `adoptHandle` was called while another handle is still active.
    HandleAlreadyActive,
    /// `releaseHandle` was called with no active handle.
    NoActiveHandle,
    /// `releaseHandle` was called with a handle whose identity does not
    /// match the state's recorded active handle. Identity includes BOTH
    /// `ptr` and `release_fn`; a forged handle sharing the original
    /// pointer but supplying a different callback is rejected here too.
    WrongHandle,
};

// JSON-safe projections (`MemorySnapshot`, `ResourceSnapshot`,
// `HandleSnapshot`) and the read-only accessor surface live in the
// sibling file `allocation_tracker_snapshots.zig` so this file can
// stay focused on authoritative accounting.

// ---------------------------------------------------------------------------
// Factory: obtain a classified allocator through the opaque state.
// Returns `BackingAllocatorMismatch` if the cell was already bound to a
// different backing allocator.
// ---------------------------------------------------------------------------

/// Returns a `std.mem.Allocator` interface for allocations classified as
/// `(owner, lifetime)`. The backing tracker is stored inside the opaque
/// state; external code never obtains `*AllocationMetrics`.
///
/// Each `(owner, lifetime)` cell is bound to exactly one backing allocator
/// on first use. A second call with a different `backing` returns
/// `error.BackingAllocatorMismatch` so a moving/mis-wired caller cannot
/// silently observe a different backing than expected.
pub fn trackingAllocator(
    self: *ReconnectMemoryState,
    backing: std.mem.Allocator,
    owner: AllocationOwner,
    lifetime: AllocationLifetime,
) BackingAllocatorError!std.mem.Allocator {
    const impl: *StateImpl = @ptrCast(@alignCast(self));
    const tracker = try impl.getOrCreateTracker(owner, lifetime, backing);
    return tracker.allocator();
}

// ---------------------------------------------------------------------------
// Authoritative handle lifecycle. Both functions write directly to the
// state's counters — there is no parallel probe or ledger struct that
// callers can substitute. Connectors only see `ReconnectHandle`; they
// MUST route every acquire through `adoptHandle` and every release
// through `releaseHandle` so `finishReconnectBoundary` reads a single,
// consistent counter pair.
//
// `releaseHandle` enforces a strict identity check on BOTH `ptr` and
// `release_fn`. A forged handle with the correct pointer but a wrong
// release_fn returns `error.WrongHandle` and never invokes either
// callback, so a mis-wired caller cannot accidentally close through a
// different physical cleanup path.
// ---------------------------------------------------------------------------

/// Adopt a freshly-acquired connector handle into the state. The
/// orchestrator MUST call this immediately after a successful
/// `connector.acquire` and BEFORE attempting the connect, so a failed
/// `connector.finish` can be balanced by `releaseHandle`.
///
/// Returns `error.HandleAlreadyActive` if a previous handle has not yet
/// been released through `releaseHandle`. The single-active-handle
/// invariant protects against accidental double-acquire from a mis-wired
/// orchestrator.
pub fn adoptHandle(
    self: *ReconnectMemoryState,
    handle: ReconnectHandle,
) HandleError!void {
    const impl: *StateImpl = @ptrCast(@alignCast(self));
    if (impl.active_handle != null) return error.HandleAlreadyActive;
    impl.handles_acquired = std.math.add(u64, impl.handles_acquired, 1)
        catch @panic("handles_acquired overflow");
    // Store the FULL token so `releaseHandle` can verify both `ptr`
    // and `release_fn`. A forged release with the same `ptr` but a
    // different `release_fn` is rejected as `WrongHandle`.
    impl.active_handle = handle;
}

/// Dispose of a previously-adopted handle. The state first verifies the
/// supplied `handle` matches the active token on BOTH `ptr` and
/// `release_fn`, THEN invokes `handle.release_fn(handle.ptr)`, and
/// ONLY THEN increments the release counter and clears the active
/// record.
///
/// A wrong handle returns `error.WrongHandle` without invoking the
/// physical callback (so a mis-wired caller cannot double-close through
/// a stale handle OR force a different cleanup path). A release with
/// no active handle returns `error.NoActiveHandle`.
///
/// Returns the same `HandleError` it would have surfaced; the orchestrator
/// is expected to propagate the error rather than swallow it.
pub fn releaseHandle(
    self: *ReconnectMemoryState,
    handle: ReconnectHandle,
) HandleError!void {
    const impl: *StateImpl = @ptrCast(@alignCast(self));
    const active = impl.active_handle orelse return error.NoActiveHandle;
    // Identity check covers BOTH `ptr` AND `release_fn`. A forged handle
    // that shares the original pointer but supplies a different
    // `release_fn` is rejected here without invoking either callback,
    // so the recorded release count cannot advance for a mismatched
    // physical cleanup path.
    if (active.ptr != handle.ptr or active.release_fn != handle.release_fn) {
        return error.WrongHandle;
    }
    // Physical release runs FIRST. If it crashes the caller has already
    // observed the wiring bug, but the state's accounting has not been
    // corrupted by a partial update.
    handle.release_fn(handle.ptr);
    impl.release_calls = std.math.add(u64, impl.release_calls, 1)
        catch @panic("release_calls overflow");
    impl.handles_released = std.math.add(u64, impl.handles_released, 1)
        catch @panic("handles_released overflow");
    impl.active_handle = null;
}

// ---------------------------------------------------------------------------
// Atomic boundary coordinator. Reads the state's own counters; never
// accepts an external probe or ledger argument. A commit is rejected
// whenever the active handle is still outstanding OR the acquire/release
// totals disagree.
// ---------------------------------------------------------------------------

pub fn finishReconnectBoundary(
    self: *ReconnectMemoryState,
    baseline_sockets: u32,
) BoundaryError!void {
    const impl: *StateImpl = @ptrCast(@alignCast(self));
    try impl.validateHandleAccounting();
    try impl.commitGeneration(baseline_sockets);
}
