// runtime/allocation_tracker_tracking_allocator.zig — producer-side
// `TrackingAllocator`. This sibling is INTENTIONALLY NOT re-exported from
// `allocation_tracker.zig`. External code obtains a classified allocator
// through the `trackingAllocator(state, …)` factory in
// `allocation_tracker_internal.zig`, which owns the internal storage and
// hands back a `std.mem.Allocator` interface.
//
// The struct itself is `pub const` only so the internal factory (in a
// sibling file) can construct and store it; the public surface never
// re-exports the name.

const std = @import("std");
const internal = @import("allocation_tracker_internal.zig");

const AllocationOwner = internal.AllocationOwner;
const AllocationLifetime = internal.AllocationLifetime;
const AllocationMetrics = internal.AllocationMetrics;

fn cellPtr(metrics: *AllocationMetrics, owner: AllocationOwner, lifetime: AllocationLifetime) *internal.OwnerMetrics {
    return &metrics.owners[@intFromEnum(owner)][@intFromEnum(lifetime)];
}

fn recomputeTotals(metrics: *AllocationMetrics) void {
    var sum: u64 = 0;
    for (0..internal.num_owners) |oi| {
        for (0..internal.num_lifetimes) |li| {
            sum = std.math.add(u64, sum, metrics.owners[oi][li].live_bytes) catch @panic("total live bytes overflow");
        }
    }
    metrics.total_live_bytes = sum;
    if (sum > metrics.total_peak_bytes) metrics.total_peak_bytes = sum;
}

// ---------------------------------------------------------------------------
// TrackingAllocator — private struct; only constructed by the internal
// factory and stored inside the opaque state. External callers receive a
// `std.mem.Allocator` and never see this struct type.
// ---------------------------------------------------------------------------

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
        const m = cellPtr(self.metrics, self.owner, self.lifetime);
        m.total_allocation_count = std.math.add(u64, m.total_allocation_count, 1) catch @panic("allocation count overflow");
        m.generation_allocation_count = std.math.add(u64, m.generation_allocation_count, 1) catch @panic("generation allocation count overflow");
        m.live_bytes = std.math.add(u64, m.live_bytes, byte_count) catch @panic("live bytes overflow");
        m.current_allocations = std.math.add(u32, m.current_allocations, 1) catch @panic("current allocations overflow");
        if (m.live_bytes > m.generation_peak_bytes) m.generation_peak_bytes = m.live_bytes;
        if (m.live_bytes > m.total_peak_bytes) m.total_peak_bytes = m.live_bytes;
        recomputeTotals(self.metrics);
    }

    fn recordResizeGrowth(self: *Self, delta: usize) void {
        const m = cellPtr(self.metrics, self.owner, self.lifetime);
        m.live_bytes = std.math.add(u64, m.live_bytes, delta) catch @panic("resize live bytes overflow");
        if (m.live_bytes > m.generation_peak_bytes) m.generation_peak_bytes = m.live_bytes;
        if (m.live_bytes > m.total_peak_bytes) m.total_peak_bytes = m.live_bytes;
        recomputeTotals(self.metrics);
    }

    fn recordResizeShrink(self: *Self, delta: usize) void {
        const m = cellPtr(self.metrics, self.owner, self.lifetime);
        if (m.live_bytes < delta) @panic("recordResizeShrink: live_bytes underflow");
        m.live_bytes -= delta;
        recomputeTotals(self.metrics);
    }

    fn recordFree(self: *Self, byte_count: usize) void {
        const m = cellPtr(self.metrics, self.owner, self.lifetime);
        if (m.current_allocations == 0) @panic("recordFree: current_allocations underflow");
        if (m.live_bytes < byte_count) @panic("recordFree: live_bytes underflow");
        m.total_free_count = std.math.add(u64, m.total_free_count, 1) catch @panic("total free count overflow");
        m.generation_free_count = std.math.add(u64, m.generation_free_count, 1) catch @panic("generation free count overflow");
        m.current_allocations -= 1;
        m.live_bytes -= byte_count;
        recomputeTotals(self.metrics);
    }
};
