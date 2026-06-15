// good-memory-copy-pattern.zig — GOOD sentinel fixture for memory copy safety gate
//
// This file intentionally contains @memcpy usage with safe MemoryCopySafety annotations.
// The memory copy safety gate should PASS on this file.
//
// This is a TEST FIXTURE demonstrating correct usage patterns.
//
// SAFE PATTERNS IN THIS FILE:
// 1. @memcpy with MemoryCopySafety annotation (independent buffers)
// 2. The annotations correctly explain WHY overlap is impossible

const std = @import("std");

// GOOD: Independent fixed-size buffers
pub fn copyPathToBuffer(buf: *[256]u8, path: []const u8) void {
    if (path.len >= buf.len) return;
    // MemoryCopySafety: dst is a [256]u8 buffer. src is caller-provided.
    // Distinct allocations; no aliasing.
    @memcpy(buf[0..path.len], path);
    buf[path.len] = 0;
}

// GOOD: Copy from temp array to items array
pub fn copyPollEvents(self: *struct {
    poll_events: [64]std.BufMap.Entry,
    poll_count: usize,
    events: std.BufMap,
}) void {
    // MemoryCopySafety: poll_events is a fixed temp array. events.items is dynamic.
    // Different allocations; no aliasing.
    @memcpy(self.poll_events[0..self.poll_count], self.events.items[0..self.poll_count]);
    self.events.clearRetainingCapacity();
}

// GOOD: Struct field copies
pub fn copyStructFields(dst: *struct { x: u32, y: u32 }, src: *const struct { x: u32, y: u32 }) void {
    // MemoryCopySafety: dst and src are independent struct stack variables.
    // Different stack slots; no aliasing.
    @memcpy(std.mem.asBytes(dst), std.mem.asBytes(src));
}

// GOOD: Protocol copy
pub fn copyPacketToSession(
    session: *struct { last_exported_prefixes: [1024]u8 },
    packet: []const u8,
    offset: usize,
    len: usize,
) void {
    // MemoryCopySafety: session owns [1024]u8 array. packet is a recv slice.
    // Distinct memory regions; no aliasing.
    @memcpy(session.last_exported_prefixes[0..len], packet[offset..offset + len]);
}

// GOOD: Formatter buffer copy
pub fn formatAndCopy(
    buf: *[512]u8,
    pos: *usize,
    bytes: []const u8,
    max_len: usize,
) !void {
    const copy_len = @min(bytes.len, max_len);
    if (pos.* + copy_len > buf.len) return error.BufferOverflow;
    // MemoryCopySafety: buf is [512]u8 caller-owned. bytes is from different source.
    // Distinct allocations; no aliasing.
    @memcpy(buf[pos.*..][0..copy_len], bytes[0..copy_len]);
    pos.* += copy_len;
}
