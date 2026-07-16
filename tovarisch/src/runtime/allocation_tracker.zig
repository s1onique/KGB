// runtime/allocation_tracker.zig — Bounded-memory instrumentation for BGP reconnect cycles
//
// ACT-TOVARISCH-BOUNDED-MEMORY-INSTRUMENTATION01
//
// Design invariants:
// - Exhaustive enums with compile-time contiguity assertions
// - Single generation authority: ReconnectMemoryState.generation
// - Failure-atomic generation transitions: validate everything, then mutate atomically
// - All counter arithmetic uses checked std.math.add
// - Snapshot uses signed delta for drift telemetry
// - Operations fail loud on under/overflow

const std = @import("std");
const builtin = @import("builtin");

// ============================================================================
// Two-Dimensional Classification (exhaustive, contiguous)
// ============================================================================

pub const AllocationOwner = enum(u4) {
    process = 0,
    bgp_subsystem = 1,
    bgp_session = 2,
};

pub const AllocationLifetime = enum(u4) {
    /// Process-lifetime (never freed except at shutdown)
    permanent = 0,
    /// One BGP reconnect generation (created/destroyed per generation)
    reconnect_generation = 1,
    /// One runOnce / status render (destroyed before returning)
    operation = 2,
};

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

// ============================================================================
// Per-Owner Tracking
// ============================================================================

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

    /// Total live bytes captured after the configured warm-up generations.
    baseline_live_bytes: ?u64 = null,

    total_live_bytes: u64 = 0,
    total_peak_bytes: u64 = 0,

    pub const warmup_generation_count: u64 = 100;

    fn getMut(self: *AllocationMetrics, owner: AllocationOwner, lifetime: AllocationLifetime) *OwnerMetrics {
        return &self.owners[@intFromEnum(owner)][@intFromEnum(lifetime)];
    }

    fn get(self: *const AllocationMetrics, owner: AllocationOwner, lifetime: AllocationLifetime) *const OwnerMetrics {
        return &self.owners[@intFromEnum(owner)][@intFromEnum(lifetime)];
    }

    /// Validate: no per-generation cells have live bytes or current allocations.
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

    /// Validate baseline drift if baseline was captured.
    fn validateBaseline(self: *const AllocationMetrics) LeakCheckError!void {
        if (self.baseline_live_bytes) |baseline| {
            if (self.total_live_bytes != baseline) return LeakCheckError.BaselineDrift;
        }
    }

    /// COMMIT phase: must only be called after a successful validate.
    /// Captures baseline exactly at warmup, resets per-generation counters,
    fn commitGeneration(self: *AllocationMetrics, next_generation: u64) void {
        // Capture baseline at warmup
        if (self.baseline_live_bytes == null and next_generation == warmup_generation_count) {
            self.baseline_live_bytes = self.total_live_bytes;
        }

        // Reset per-generation counters (private to this function)
        inline for (std.enums.values(AllocationLifetime)) |lt| {
            if (lt == .permanent) continue;
            for (0..num_owners) |oi| {
                self.owners[oi][@intFromEnum(lt)].generation_peak_bytes = 0;
                self.owners[oi][@intFromEnum(lt)].generation_allocation_count = 0;
                self.owners[oi][@intFromEnum(lt)].generation_free_count = 0;
            }
        }

    }

    fn updateTotals(self: *AllocationMetrics) void {
        var sum: u64 = 0;
        for (0..num_owners) |oi| {
            for (0..num_lifetimes) |li| {
                sum = std.math.add(u64, sum, self.owners[oi][li].live_bytes) catch @panic("total live bytes overflow");
            }
        }
        self.total_live_bytes = sum;
        if (sum > self.total_peak_bytes) self.total_peak_bytes = sum;
    }
};

pub const LeakCheckError = error{
    LiveBytesRemaining,
    UnfreedAllocations,
    BaselineDrift,
};

// ============================================================================
// TrackingAllocator: real std.mem.Allocator wrapper
// ============================================================================

pub const TrackingAllocator = struct {
    backing: std.mem.Allocator,
    metrics: *AllocationMetrics,
    owner: AllocationOwner,
    lifetime: AllocationLifetime,

    const Self = @This();

    pub fn allocator(self: *Self) std.mem.Allocator {
        return .{
            .ptr = self,
            .vtable = &.{
                .alloc = allocFn,
                .resize = resizeFn,
                .remap = remapFn,
                .free = freeFn,
            },
        };
    }

    fn allocFn(ctx: *anyopaque, len: usize, alignment: std.mem.Alignment, ra: usize) ?[*]u8 {
        const self: *Self = @ptrCast(@alignCast(ctx));
        const result = self.backing.rawAlloc(len, alignment, ra);
        if (result != null) {
            self.recordAlloc(len);
        }
        return result;
    }

    fn resizeFn(ctx: *anyopaque, buf: []u8, alignment: std.mem.Alignment, new_len: usize, ra: usize) bool {
        const self: *Self = @ptrCast(@alignCast(ctx));
        const old_len = buf.len;
        const success = self.backing.rawResize(buf, alignment, new_len, ra);
        if (success) {
            if (new_len > old_len) {
                self.recordResizeGrowth(new_len - old_len);
            } else {
                self.recordResizeShrink(old_len - new_len);
            }
        }
        return success;
    }

    fn remapFn(ctx: *anyopaque, buf: []u8, alignment: std.mem.Alignment, new_len: usize, ra: usize) ?[*]u8 {
        const self: *Self = @ptrCast(@alignCast(ctx));
        const old_len = buf.len;
        const new_ptr = self.backing.rawRemap(buf, alignment, new_len, ra);
        if (new_ptr != null) {
            if (new_len > old_len) {
                self.recordResizeGrowth(new_len - old_len);
            } else {
                self.recordResizeShrink(old_len - new_len);
            }
        }
        return new_ptr;
    }

    fn freeFn(ctx: *anyopaque, buf: []u8, alignment: std.mem.Alignment, ra: usize) void {
        const self: *Self = @ptrCast(@alignCast(ctx));
        self.recordFree(buf.len);
        self.backing.rawFree(buf, alignment, ra);
    }

    fn recordAlloc(self: *Self, byte_count: usize) void {
        const m = self.metrics.getMut(self.owner, self.lifetime);
        m.total_allocation_count = std.math.add(u64, m.total_allocation_count, 1) catch @panic("allocation count overflow");
        m.generation_allocation_count = std.math.add(u64, m.generation_allocation_count, 1) catch @panic("generation allocation count overflow");
        m.live_bytes = std.math.add(u64, m.live_bytes, byte_count) catch @panic("live bytes overflow");
        m.current_allocations = std.math.add(u32, m.current_allocations, 1) catch @panic("current allocations overflow");
        if (m.live_bytes > m.generation_peak_bytes) m.generation_peak_bytes = m.live_bytes;
        if (m.live_bytes > m.total_peak_bytes) m.total_peak_bytes = m.live_bytes;
        self.metrics.updateTotals();
    }

    fn recordResizeGrowth(self: *Self, delta: usize) void {
        const m = self.metrics.getMut(self.owner, self.lifetime);
        m.live_bytes = std.math.add(u64, m.live_bytes, delta) catch @panic("resize live bytes overflow");
        if (m.live_bytes > m.generation_peak_bytes) m.generation_peak_bytes = m.live_bytes;
        if (m.live_bytes > m.total_peak_bytes) m.total_peak_bytes = m.live_bytes;
        self.metrics.updateTotals();
    }

    fn recordResizeShrink(self: *Self, delta: usize) void {
        const m = self.metrics.getMut(self.owner, self.lifetime);
        if (m.live_bytes < delta) @panic("recordResizeShrink: live_bytes underflow");
        m.live_bytes -= delta;
        self.metrics.updateTotals();
    }

    fn recordFree(self: *Self, byte_count: usize) void {
        const m = self.metrics.getMut(self.owner, self.lifetime);
        if (m.current_allocations == 0) @panic("recordFree: current_allocations underflow");
        if (m.live_bytes < byte_count) @panic("recordFree: live_bytes underflow");
        m.total_free_count = std.math.add(u64, m.total_free_count, 1) catch @panic("total free count overflow");
        m.generation_free_count = std.math.add(u64, m.generation_free_count, 1) catch @panic("generation free count overflow");
        m.current_allocations -= 1;
        m.live_bytes -= byte_count;
        self.metrics.updateTotals();
    }
};

// ============================================================================
// BoundedResourceCounters
// ============================================================================

/// Tracks resources that MUST be returned to baseline after each reconnect generation.
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
            self.error_history_count += 1;
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
            self.retry_collection_count += 1;
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


    pub const ResourceError = error{
        SocketLeak,
        TimerLeak,
    };
};

// ============================================================================
// ReconnectMemoryState: unified coordinator (SINGLE authority for generation)
// ============================================================================

/// Coordinated reconnect-memory state. Single authority for generation number.
/// Generation transitions are failure-atomic:
///   1. Compute next generation
///   2. Validate EVERYTHING (allocations + resources)
///   3. If validation passes, commit atomically
///   4. If validation fails, NO state changes
pub const ReconnectMemoryState = struct {
    generation: u64 = 0,
    allocations: AllocationMetrics = .{},
    resources: BoundedResourceCounters = .{},

    pub fn finishGeneration(self: *ReconnectMemoryState, baseline_sockets: u32) ReconnectGenerationError!void {
        // Step 1: Compute next generation (this can panic on overflow, which is fine)
        const next_generation = std.math.add(u64, self.generation, 1) catch @panic("generation count overflow");

        // Step 2: Validate EVERYTHING before mutating anything
        try self.allocations.validateForGeneration();
        try self.allocations.validateBaseline();
        try self.resources.validateGenerationComplete(baseline_sockets);

        // Step 3: Commit atomically (only if all validations passed)
        self.allocations.commitGeneration(next_generation);
        self.generation = next_generation;
    }
};

pub const ReconnectGenerationError = error{
    LiveBytesRemaining,
    UnfreedAllocations,
    BaselineDrift,
    SocketLeak,
    TimerLeak,
};

// ============================================================================
// Snapshot for /status
// ============================================================================

/// JSON-safe snapshot of memory metrics for status output.
/// Uses SIGNED delta for drift so telemetry matches the underlying contract.
pub const MemorySnapshot = struct {
    live_bytes: u64,
    peak_bytes: u64,
    reconnect_allocation_count: u64,
    reconnect_free_count: u64,
    reconnect_generation: u64,
    baseline_live_bytes: ?u64,
    reconnect_live_bytes: u64,
    /// Signed delta from baseline: positive = growth, negative = reduction.
    /// 0 means exact match.
    baseline_delta_bytes: i128,

    pub fn fromState(state: *const ReconnectMemoryState) MemorySnapshot {
        const metrics = &state.allocations;

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
            .reconnect_generation = state.generation,
            .baseline_live_bytes = metrics.baseline_live_bytes,
            .reconnect_live_bytes = reconnect_live,
            .baseline_delta_bytes = delta,
        };
    }
};

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

    pub fn fromState(state: *const ReconnectMemoryState) ResourceSnapshot {
        const c = &state.resources;
        return .{
            .active_sockets = c.active_sockets,
            .peak_sockets = c.peak_sockets,
            .active_timers = c.active_timers,
            .peak_timers = c.peak_timers,
            .error_history_count = c.error_history_count,
            .error_history_capacity = c.error_history_capacity,
            .retry_collection_count = c.retry_collection_count,
            .retry_collection_capacity = c.retry_collection_capacity,
            .reconnect_generation = state.generation,
        };
    }
};

// ============================================================================
// Tests
// ============================================================================

test "TrackingAllocator: alloc/free records correctly" {
    var metrics = AllocationMetrics{};
    var ta = TrackingAllocator{
        .backing = std.testing.allocator,
        .metrics = &metrics,
        .owner = .bgp_session,
        .lifetime = .operation,
    };
    const a = ta.allocator();

    const buf1 = try a.alloc(u8, 100);
    defer a.free(buf1);
    const m = metrics.get(.bgp_session, .operation);
    try std.testing.expectEqual(@as(u64, 100), m.live_bytes);
    try std.testing.expectEqual(@as(u32, 1), m.current_allocations);
    try std.testing.expectEqual(@as(u64, 1), m.total_allocation_count);
}

test "TrackingAllocator: resize updates slice and preserves allocation count" {
    var metrics = AllocationMetrics{};
    var ta = TrackingAllocator{
        .backing = std.testing.allocator,
        .metrics = &metrics,
        .owner = .bgp_session,
        .lifetime = .operation,
    };
    const a = ta.allocator();

    var buffer = try a.alloc(u8, 100);
    defer a.free(buffer);

    const m = metrics.get(.bgp_session, .operation);
    const initial_alloc = m.total_allocation_count;

    if (a.resize(buffer, 200)) {
        buffer.len = 200;
        try std.testing.expectEqual(initial_alloc, m.total_allocation_count);
        try std.testing.expectEqual(@as(u64, 200), m.live_bytes);
    }

    if (a.resize(buffer, 50)) {
        buffer.len = 50;
        try std.testing.expectEqual(initial_alloc, m.total_allocation_count);
        try std.testing.expectEqual(@as(u64, 50), m.live_bytes);
    }
}

test "AllocationMetrics: baseline captured exactly at warmup_generation_count" {
    var state = ReconnectMemoryState{};
    var i: u64 = 0;
    while (i < 99) : (i += 1) {
        try state.finishGeneration(0);
    }
    try std.testing.expectEqual(@as(u64, 99), state.generation);
    try std.testing.expect(state.allocations.baseline_live_bytes == null);

    try state.finishGeneration(0);
    try std.testing.expectEqual(@as(u64, 100), state.generation);
    try std.testing.expect(state.allocations.baseline_live_bytes != null);
}

test "AllocationMetrics: BaselineDrift detected on permanent-classification growth" {
    var state = ReconnectMemoryState{};
    var i: u64 = 0;
    while (i < 100) : (i += 1) {
        try state.finishGeneration(0);
    }

    state.allocations.owners[@intFromEnum(AllocationOwner.bgp_session)][@intFromEnum(AllocationLifetime.permanent)].live_bytes = 4096;
    state.allocations.total_live_bytes = 4096;

    const result = state.finishGeneration(0);
    try std.testing.expectError(ReconnectGenerationError.BaselineDrift, result);

    // FAILURE-ATOMIC: state.generation MUST NOT have advanced.
    try std.testing.expectEqual(@as(u64, 100), state.generation);
}

test "AllocationMetrics: finishGeneration rejects unfreed allocation (failure-atomic)" {
    var state = ReconnectMemoryState{};
    state.allocations.owners[@intFromEnum(AllocationOwner.bgp_session)][@intFromEnum(AllocationLifetime.reconnect_generation)].current_allocations = 1;
    const result = state.finishGeneration(0);
    try std.testing.expectError(ReconnectGenerationError.UnfreedAllocations, result);
    try std.testing.expectEqual(@as(u64, 0), state.generation);
}

test "BoundedResourceCounters: socket open/close" {
    var c = BoundedResourceCounters{};
    c.recordSocketOpen();
    try std.testing.expectEqual(@as(u32, 1), c.active_sockets);
    c.recordSocketClose();
    try std.testing.expectEqual(@as(u32, 0), c.active_sockets);
}

test "BoundedResourceCounters: timer open/close" {
    var c = BoundedResourceCounters{};
    c.recordTimerStart();
    try std.testing.expectEqual(@as(u32, 1), c.active_timers);
    c.recordTimerStop();
    try std.testing.expectEqual(@as(u32, 0), c.active_timers);
}

test "BoundedResourceCounters: tryReserve + release slot pair" {
    var c = BoundedResourceCounters{};
    c.error_history_capacity = 2;
    try std.testing.expect(c.tryReserveErrorHistorySlot());
    try std.testing.expect(c.tryReserveErrorHistorySlot());
    try std.testing.expect(!c.tryReserveErrorHistorySlot());
    c.releaseErrorHistorySlot();
    try std.testing.expect(c.tryReserveErrorHistorySlot());
}

test "ReconnectMemoryState: coordinated finishGeneration (generation is single authority)" {
    var state = ReconnectMemoryState{};
    try state.finishGeneration(0);
    try std.testing.expectEqual(@as(u64, 1), state.generation);
}

test "ReconnectMemoryState: failure-atomic on resource leak" {
    var state = ReconnectMemoryState{};
    try state.finishGeneration(0); // generation = 1
    // Introduce socket that won't be closed:
    state.resources.active_sockets = 2;

    const result = state.finishGeneration(0);
    try std.testing.expectError(ReconnectGenerationError.SocketLeak, result);
    // Generation MUST NOT have advanced.
    try std.testing.expectEqual(@as(u64, 1), state.generation);
}

test "MemorySnapshot: fromState exposes signed baseline delta" {
    var state = ReconnectMemoryState{};
    state.allocations.total_live_bytes = 4096;
    state.allocations.total_peak_bytes = 8192;
    state.generation = 50;
    state.allocations.baseline_live_bytes = 4096;
    const snap = MemorySnapshot.fromState(&state);
    try std.testing.expectEqual(@as(u64, 4096), snap.live_bytes);
    try std.testing.expectEqual(@as(u64, 8192), snap.peak_bytes);
    try std.testing.expectEqual(@as(u64, 50), snap.reconnect_generation);
    try std.testing.expectEqual(@as(i128, 0), snap.baseline_delta_bytes);
}

test "MemorySnapshot: signed delta tracks growth" {
    var state = ReconnectMemoryState{};
    state.allocations.baseline_live_bytes = 1024;
    state.allocations.total_live_bytes = 4096;
    const snap = MemorySnapshot.fromState(&state);
    try std.testing.expectEqual(@as(i128, 3072), snap.baseline_delta_bytes);
}

test "ResourceSnapshot: fromState uses coordinator generation" {
    var state = ReconnectMemoryState{};
    state.generation = 42;
    const snap = ResourceSnapshot.fromState(&state);
    try std.testing.expectEqual(@as(u64, 42), snap.reconnect_generation);
}
