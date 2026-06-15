// bad-memory-copy-pattern.zig — BAD sentinel fixture for memory copy safety gate
//
// This file intentionally contains forbidden @memcpy patterns. The memory copy
// safety gate should FAIL on this file if scanned.
//
// This is a TEST FIXTURE - DO NOT USE IN PRODUCTION.
//
// FORBIDDEN PATTERNS IN THIS FILE:
// 1. Same-buffer @memcpy (buf[0..n], buf[x..x+n]) — always forbidden
// 2. Raw @memcpy without MemoryCopySafety annotation

const std = @import("std");

// BAD: Same-buffer copy — always forbidden regardless of annotation
// This pattern causes Zig 0.16 panic: "@memcpy arguments alias"
pub fn sameBufferCopy(dst: []u8, src_offset: usize, len: usize) void {
    // This is the exact pattern that caused the original crash.
    // recv_buf[0..n] and recv_buf[x..x+n] share the same backing array.
    @memcpy(dst[0..len], dst[src_offset..src_offset + len]);
}

// BAD: Same-buffer with different slice syntax
// sess.recv_buf[dest..] and sess.recv_buf[src..] are the same buffer
pub fn sessionBufferShift(sess: *struct { recv_buf: [4096]u8 }, dest: usize, src: usize, len: usize) void {
    // MemoryCopySafety: — this annotation is WRONG, same-buffer is always forbidden
    @memcpy(sess.recv_buf[dest..][0..len], sess.recv_buf[src..][0..len]);
}

// BAD: Raw @memcpy without annotation
// dst is fixed-size [256]u8, src is caller-provided []const u8.
// Without annotation, this is flagged.
pub fn rawMemcpyWithoutAnnotation(dst: *[256]u8, src: []const u8) void {
    const copy_len = @min(src.len, 256);
    @memcpy(dst[0..copy_len], src[0..copy_len]);
}

// BAD: Another same-buffer pattern with nested field access
pub fn nestedBufferCopy(
    outer: *struct { inner: struct { buf: [1024]u8 } },
    offset: usize,
    count: usize,
) void {
    // MemoryCopySafety: — STILL FORBIDDEN. Same buffer, different slices.
    @memcpy(outer.inner.buf[0..count], outer.inner.buf[offset..offset + count]);
}
