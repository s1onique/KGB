// runtime/allocation_tracker_snapshots.zig — JSON-safe projections and
// read-only accessors on top of `ReconnectMemoryState`.
//
// This sibling carries the snapshot / read-only accessor surface so the
// state file (`allocation_tracker_internal.zig`) can stay focused on
// authoritative accounting (init/deinit, handle bookkeeping, classified
// allocator factory, atomic boundary). All snapshots are constructed
// through `fromState` which takes the opaque state pointer and
// `@ptrCast`s to the concrete `StateImpl` only for the duration of the
// read.
//
// Sibling files in this directory carry the remaining instrumentation:
//   * `allocation_tracker.zig`                       — public re-exports.
//   * `allocation_tracker_internal.zig`              — opaque state +
//                                                    handle bookkeeping +
//                                                    `trackingAllocator`
//                                                    + `finishReconnectBoundary`.
//   * `allocation_tracker_connector_probe.zig`      — `ReconnectHandle`
//                                                    + release fn typedef.

const std = @import("std");
const connector_probe_sibling = @import("allocation_tracker_connector_probe.zig");
const internal = @import("allocation_tracker_internal.zig");

const AllocationLifetime = internal.AllocationLifetime;
const num_owners = internal.num_owners;
const ReconnectMemoryState = internal.ReconnectMemoryState;
const StateImpl = internal.StateImpl;
const ReconnectHandle = connector_probe_sibling.ReconnectHandle;

// ---------------------------------------------------------------------------
// JSON-safe memory projection.
// ---------------------------------------------------------------------------

pub const MemorySnapshot = struct {
    live_bytes: u64,
    peak_bytes: u64,
    reconnect_allocation_count: u64,
    reconnect_free_count: u64,
    reconnect_generation: u64,
    baseline_live_bytes: ?u64,
    reconnect_live_bytes: u64,
    baseline_delta_bytes: i128,

    pub fn fromState(s: *const ReconnectMemoryState) MemorySnapshot {
        const impl: *const StateImpl = @ptrCast(@alignCast(s));
        const metrics = &impl.allocations;
        var reconnect_live: u64 = 0;
        var reconnect_alloc: u64 = 0;
        var reconnect_free: u64 = 0;
        for (0..num_owners) |oi| {
            const m = &metrics.owners[oi][@intFromEnum(AllocationLifetime.reconnect_generation)];
            reconnect_live = std.math.add(u64, reconnect_live, m.live_bytes) catch @panic("reconnect live bytes overflow");
            reconnect_alloc = std.math.add(u64, reconnect_alloc, m.total_allocation_count) catch @panic("reconnect allocation count overflow");
            reconnect_free = std.math.add(u64, reconnect_free, m.total_free_count) catch @panic("reconnect free count overflow");
        }
        var delta: i128 = 0;
        if (metrics.baseline_live_bytes) |b| {
            delta = @as(i128, @intCast(metrics.total_live_bytes)) - @as(i128, @intCast(b));
        }
        return .{
            .live_bytes = metrics.total_live_bytes,
            .peak_bytes = metrics.total_peak_bytes,
            .reconnect_allocation_count = reconnect_alloc,
            .reconnect_free_count = reconnect_free,
            .reconnect_generation = impl.generation,
            .baseline_live_bytes = metrics.baseline_live_bytes,
            .reconnect_live_bytes = reconnect_live,
            .baseline_delta_bytes = delta,
        };
    }
};

// ---------------------------------------------------------------------------
// JSON-safe resource projection (sockets, timers).
// ---------------------------------------------------------------------------

pub const ResourceSnapshot = struct {
    active_sockets: u32,
    peak_sockets: u32,
    active_timers: u32,
    peak_timers: u32,
    error_history_count: u32,
    error_history_capacity: u32,
    retry_collection_count: u32,
    retry_collection_capacity: u32,
    reconnect_generation: u64,

    pub fn fromState(s: *const ReconnectMemoryState) ResourceSnapshot {
        const impl: *const StateImpl = @ptrCast(@alignCast(s));
        const c = &impl.resources;
        return .{
            .active_sockets = c.active_sockets,
            .peak_sockets = c.peak_sockets,
            .active_timers = c.active_timers,
            .peak_timers = c.peak_timers,
            .error_history_count = c.error_history_count,
            .error_history_capacity = c.error_history_capacity,
            .retry_collection_count = c.retry_collection_count,
            .retry_collection_capacity = c.retry_collection_capacity,
            .reconnect_generation = impl.generation,
        };
    }
};

// ---------------------------------------------------------------------------
// Read-only handle projection.
//
// `active_handle` exposes the FULL `{ptr, release_fn}` token so tests
// can verify BOTH halves of the identity check performed by
// `releaseHandle` (the production oracle rejects a forged handle that
// shares the original pointer but supplies a different `release_fn`).
// Writes go through `adoptHandle` / `releaseHandle` only.
// ---------------------------------------------------------------------------

pub const HandleSnapshot = struct {
    active_handle: ?ReconnectHandle,
    handles_acquired: u64,
    handles_released: u64,
    release_calls: u64,
    delta: i128,

    pub fn fromState(s: *const ReconnectMemoryState) HandleSnapshot {
        const impl: *const StateImpl = @ptrCast(@alignCast(s));
        return .{
            .active_handle = impl.active_handle,
            .handles_acquired = impl.handles_acquired,
            .handles_released = impl.handles_released,
            .release_calls = impl.release_calls,
            .delta = @as(i128, @intCast(impl.handles_acquired)) -
                @as(i128, @intCast(impl.handles_released)),
        };
    }
};

// ---------------------------------------------------------------------------
// Read-only accessors over the opaque state.
// ---------------------------------------------------------------------------

pub fn handleSnapshot(self: *const ReconnectMemoryState) HandleSnapshot {
    return HandleSnapshot.fromState(self);
}

pub fn memorySnapshot(self: *const ReconnectMemoryState) MemorySnapshot {
    return MemorySnapshot.fromState(self);
}

pub fn resourceSnapshot(self: *const ReconnectMemoryState) ResourceSnapshot {
    return ResourceSnapshot.fromState(self);
}

pub fn generation(self: *const ReconnectMemoryState) u64 {
    return (@as(*const StateImpl, @ptrCast(@alignCast(self)))).generation;
}

pub fn liveBytesForLifetime(self: *const ReconnectMemoryState, lifetime: AllocationLifetime) u64 {
    const impl: *const StateImpl = @ptrCast(@alignCast(self));
    var total: u64 = 0;
    for (0..num_owners) |oi| {
        total = std.math.add(u64, total, impl.allocations.owners[oi][@intFromEnum(lifetime)].live_bytes) catch @panic("live bytes overflow");
    }
    return total;
}

pub fn currentAllocationsForLifetime(self: *const ReconnectMemoryState, lifetime: AllocationLifetime) u64 {
    const impl: *const StateImpl = @ptrCast(@alignCast(self));
    var total: u64 = 0;
    for (0..num_owners) |oi| {
        total = std.math.add(u64, total, impl.allocations.owners[oi][@intFromEnum(lifetime)].current_allocations) catch @panic("allocations overflow");
    }
    return total;
}

pub fn baselineLiveBytes(self: *const ReconnectMemoryState) ?u64 {
    return (@as(*const StateImpl, @ptrCast(@alignCast(self)))).allocations.baseline_live_bytes;
}

pub fn totalPeakBytes(self: *const ReconnectMemoryState) u64 {
    return (@as(*const StateImpl, @ptrCast(@alignCast(self)))).allocations.total_peak_bytes;
}

// ---------------------------------------------------------------------------
// Resource event recorders. Forwarders to the bounded counters table.
// ---------------------------------------------------------------------------

pub fn recordSocketOpen(self: *ReconnectMemoryState) void {
    (@as(*StateImpl, @ptrCast(@alignCast(self)))).resources.recordSocketOpen();
}

pub fn recordSocketClose(self: *ReconnectMemoryState) void {
    (@as(*StateImpl, @ptrCast(@alignCast(self)))).resources.recordSocketClose();
}

pub fn recordTimerStart(self: *ReconnectMemoryState) void {
    (@as(*StateImpl, @ptrCast(@alignCast(self)))).resources.recordTimerStart();
}

pub fn recordTimerStop(self: *ReconnectMemoryState) void {
    (@as(*StateImpl, @ptrCast(@alignCast(self)))).resources.recordTimerStop();
}
