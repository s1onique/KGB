// bad-memory-ownership-pattern.zig — BAD sentinel fixture for memory ownership gate
//
// This file intentionally contains a page_allocator usage WITHOUT MemoryOwnership
// annotation. The memory ownership gate should FAIL on this file if scanned.
//
// This is a TEST FIXTURE - DO NOT USE IN PRODUCTION.
//
// RISKY PATTERN: This file contains std.heap.page_allocator without annotation.
// Each /status request will leak ~4 KiB of page-backed memory.

const std = @import("std");

// BAD: page_allocator without MemoryOwnership annotation
// This will cause RSS leak on each request.
pub fn renderStatusWithoutOwnership() !void {
    const allocator = std.heap.page_allocator;
    var buf: [256]u8 = undefined;
    // Memory is leaked here - no deallocation
    const formatted = try std.fmt.allocPrint(allocator, "status: {s}", .{"ok"});
    _ = formatted;
    _ = std.io.getStdOut().writer().print("{s}", .{formatted});
}

// BAD: ArenaAllocator without MemoryOwnership annotation
// ArenaAllocator has unbounded growth potential.
pub fn renderStatusWithArena() !void {
    var arena = std.heap.ArenaAllocator.init(std.heap.page_allocator);
    defer arena.deinit();
    // Memory is leaked if render fails before deinit
    const result = try std.fmt.allocPrint(arena.allocator(), "status: {s}", .{"ok"});
    _ = result;
}

// BAD: toOwnedSlice without MemoryOwnership annotation
pub fn renderStatusWithOwnedSlice() !void {
    var list = std.ArrayList(u8).init(std.heap.page_allocator);
    defer list.deinit();
    try list.appendSlice("status");
    // toOwnedSlice transfers ownership - must be freed
    const owned = try list.toOwnedSlice();
    _ = owned; // Memory leak - owned was never freed
}

// BAD: .dupe( without MemoryOwnership annotation
// .dupe() allocates a heap copy that must be freed.
// This function is independent of page_allocator/allocPrint patterns,
// so it specifically validates that the gate catches .dupe(.
pub fn renderStatusWithDupe(peer_names: []const []const u8) ![]const u8 {
    // .dupe() makes a heap copy - must be freed by caller
    var result = std.ArrayList(u8).init(std.testing.allocator);
    for (peer_names) |name| {
        const copy = name.dupe(std.testing.allocator);
        try result.appendSlice(copy);
        // Memory leak: copy is never freed
    }
    return result.toOwnedSlice();
}
