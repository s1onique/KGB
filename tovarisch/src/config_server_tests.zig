// config_server_tests.zig — Tests for [server] section config parsing
//
// These tests verify that parseServerConfig correctly handles the [server].listen
// configuration option.

const std = @import("std");
const config = @import("config.zig");

test "parseServerConfig returns null listen for missing section" {
    var raw = std.StringArrayHashMapUnmanaged(std.StringArrayHashMapUnmanaged([]const u8)){};
    defer raw.deinit(std.heap.page_allocator);
    const cfg = config.parseServerConfig(&raw);
    try std.testing.expect(cfg.listen == null);
}

test "parseServerConfig returns null listen for empty section" {
    var raw = std.StringArrayHashMapUnmanaged(std.StringArrayHashMapUnmanaged([]const u8)){};
    defer raw.deinit(std.heap.page_allocator);
    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    defer section.deinit(std.heap.page_allocator);
    try raw.put(std.heap.page_allocator, "server", section);
    const cfg = config.parseServerConfig(&raw);
    try std.testing.expect(cfg.listen == null);
}

test "parseServerConfig parses listen address" {
    var raw = std.StringArrayHashMapUnmanaged(std.StringArrayHashMapUnmanaged([]const u8)){};
    defer raw.deinit(std.heap.page_allocator);
    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    defer section.deinit(std.heap.page_allocator);
    try section.put(std.heap.page_allocator, "listen", "10.149.149.1:8317");
    try raw.put(std.heap.page_allocator, "server", section);
    const cfg = config.parseServerConfig(&raw);
    try std.testing.expect(cfg.listen != null);
    try std.testing.expectEqualStrings("10.149.149.1:8317", cfg.listen.?);
}

test "parseServerConfig parses loopback listen address" {
    var raw = std.StringArrayHashMapUnmanaged(std.StringArrayHashMapUnmanaged([]const u8)){};
    defer raw.deinit(std.heap.page_allocator);
    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    defer section.deinit(std.heap.page_allocator);
    try section.put(std.heap.page_allocator, "listen", "127.0.0.1:8317");
    try raw.put(std.heap.page_allocator, "server", section);
    const cfg = config.parseServerConfig(&raw);
    try std.testing.expect(cfg.listen != null);
    try std.testing.expectEqualStrings("127.0.0.1:8317", cfg.listen.?);
}

test "parseServerConfig parses custom port" {
    var raw = std.StringArrayHashMapUnmanaged(std.StringArrayHashMapUnmanaged([]const u8)){};
    defer raw.deinit(std.heap.page_allocator);
    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    defer section.deinit(std.heap.page_allocator);
    try section.put(std.heap.page_allocator, "listen", "10.0.0.1:9999");
    try raw.put(std.heap.page_allocator, "server", section);
    const cfg = config.parseServerConfig(&raw);
    try std.testing.expect(cfg.listen != null);
    try std.testing.expectEqualStrings("10.0.0.1:9999", cfg.listen.?);
}

test "parseServerConfig parses 0.0.0.0 wildcard bind address" {
    var raw = std.StringArrayHashMapUnmanaged(std.StringArrayHashMapUnmanaged([]const u8)){};
    defer raw.deinit(std.heap.page_allocator);
    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    defer section.deinit(std.heap.page_allocator);
    try section.put(std.heap.page_allocator, "listen", "0.0.0.0:8317");
    try raw.put(std.heap.page_allocator, "server", section);
    const cfg = config.parseServerConfig(&raw);
    try std.testing.expect(cfg.listen != null);
    try std.testing.expectEqualStrings("0.0.0.0:8317", cfg.listen.?);
}
