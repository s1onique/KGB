// good-memory-ownership-pattern.zig — GOOD sentinel fixture for memory ownership gate
//
// This file intentionally contains page_allocator usage WITH safe MemoryOwnership
// annotation. The memory ownership gate should PASS on this file.
//
// This is a TEST FIXTURE demonstrating correct usage patterns.
//
// SAFE PATTERN: All page_allocator usages are annotated with MemoryOwnership:
// that explains WHY the pattern is safe (not just "it's bounded").
//
// Examples of safe annotations:
// - "transient: freed after use within same function"
// - "caller owns the buffer for the scope of this operation"
// - "startup-only: one-time allocation at daemon init, never per-request"

const std = @import("std");

// GOOD: page_allocator for truly one-time startup allocation
// This is intentional for startup-only initialization.
pub fn initWithStartupAllocation() !void {
    // MemoryOwnership: Startup-only one-time allocation.
    // This runs once at daemon startup, not per-request. The allocation
    // persists for the entire daemon lifetime but is bounded (single allocation).
    const allocator = std.heap.page_allocator;
    var buf: [256]u8 = undefined;
    _ = try std.fmt.bufPrint(&buf, "status: {s}", .{"ok"});
}

// GOOD: ArenaAllocator with explicit scope and deinit
// ArenaAllocator is used intentionally for bounded collection scope.
pub fn renderStatusWithArena() !void {
    // MemoryOwnership: All allocations freed when arena.deinit() is called.
    // This is NOT unbounded growth because arena is deinitialized after use.
    var arena = std.heap.ArenaAllocator.init(std.heap.page_allocator);
    defer arena.deinit();
    const result = try std.fmt.allocPrint(arena.allocator(), "status: {s}", .{"ok"});
    defer arena.allocator().free(result);
    var buf: [256]u8 = undefined;
    _ = try std.fmt.bufPrint(&buf, "{s}", .{result});
}

// GOOD: toOwnedSlice with explicit free
// Ownership is properly transferred and freed.
pub fn renderStatusWithOwnedSlice() !void {
    // MemoryOwnership: toOwnedSlice result is immediately freed.
    // Transient allocation within same function scope.
    var list = std.ArrayList(u8).init(std.heap.page_allocator);
    defer list.deinit();
    try list.appendSlice("status");
    const owned = try list.toOwnedSlice();
    defer std.heap.page_allocator.free(owned);
    var buf: [256]u8 = undefined;
    _ = try std.fmt.bufPrint(&buf, "{s}", .{owned});
}
