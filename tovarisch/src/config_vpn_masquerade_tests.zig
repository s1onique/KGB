// config_vpn_masquerade_tests.zig — Tests for [vpn_masquerade] section config parsing
//
// These tests verify that parseVpnMasqueradeConfig correctly handles the
// [vpn_masquerade] configuration option.

const std = @import("std");
const config = @import("config.zig");

test "parseVpnMasqueradeConfig returns disabled default for missing section" {
    var raw = std.StringArrayHashMapUnmanaged(std.StringArrayHashMapUnmanaged([]const u8)){};
    defer raw.deinit(std.heap.page_allocator);
    const cfg = try config.parseVpnMasqueradeConfig(&raw);
    try std.testing.expect(!cfg.enabled);
    try std.testing.expectEqualStrings("", cfg.vpn_cidr);
    try std.testing.expectEqualStrings("", cfg.public_interface);
}

test "parseVpnMasqueradeConfig returns disabled for explicit false" {
    var raw = std.StringArrayHashMapUnmanaged(std.StringArrayHashMapUnmanaged([]const u8)){};
    defer raw.deinit(std.heap.page_allocator);
    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    defer section.deinit(std.heap.page_allocator);
    try section.put(std.heap.page_allocator, "enabled", "false");
    try raw.put(std.heap.page_allocator, "vpn_masquerade", section);
    const cfg = try config.parseVpnMasqueradeConfig(&raw);
    try std.testing.expect(!cfg.enabled);
}

test "parseVpnMasqueradeConfig accepts enabled with valid CIDR and interface" {
    var raw = std.StringArrayHashMapUnmanaged(std.StringArrayHashMapUnmanaged([]const u8)){};
    defer raw.deinit(std.heap.page_allocator);
    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    defer section.deinit(std.heap.page_allocator);
    try section.put(std.heap.page_allocator, "enabled", "true");
    try section.put(std.heap.page_allocator, "vpn_cidr", "10.0.0.0/8");
    try section.put(std.heap.page_allocator, "public_interface", "eth0");
    try raw.put(std.heap.page_allocator, "vpn_masquerade", section);
    const cfg = try config.parseVpnMasqueradeConfig(&raw);
    try std.testing.expect(cfg.enabled);
    try std.testing.expectEqualStrings("10.0.0.0/8", cfg.vpn_cidr);
    try std.testing.expectEqualStrings("eth0", cfg.public_interface);
}

test "parseVpnMasqueradeConfig fails when enabled but missing vpn_cidr" {
    var raw = std.StringArrayHashMapUnmanaged(std.StringArrayHashMapUnmanaged([]const u8)){};
    defer raw.deinit(std.heap.page_allocator);
    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    defer section.deinit(std.heap.page_allocator);
    try section.put(std.heap.page_allocator, "enabled", "true");
    try section.put(std.heap.page_allocator, "public_interface", "eth0");
    try raw.put(std.heap.page_allocator, "vpn_masquerade", section);
    try std.testing.expectError(config.ConfigError.MissingKey, config.parseVpnMasqueradeConfig(&raw));
}

test "parseVpnMasqueradeConfig fails when enabled but missing public_interface" {
    var raw = std.StringArrayHashMapUnmanaged(std.StringArrayHashMapUnmanaged([]const u8)){};
    defer raw.deinit(std.heap.page_allocator);
    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    defer section.deinit(std.heap.page_allocator);
    try section.put(std.heap.page_allocator, "enabled", "true");
    try section.put(std.heap.page_allocator, "vpn_cidr", "10.0.0.0/8");
    try raw.put(std.heap.page_allocator, "vpn_masquerade", section);
    try std.testing.expectError(config.ConfigError.MissingKey, config.parseVpnMasqueradeConfig(&raw));
}

test "parseVpnMasqueradeConfig fails on malformed CIDR" {
    var raw = std.StringArrayHashMapUnmanaged(std.StringArrayHashMapUnmanaged([]const u8)){};
    defer raw.deinit(std.heap.page_allocator);
    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    defer section.deinit(std.heap.page_allocator);
    try section.put(std.heap.page_allocator, "enabled", "true");
    try section.put(std.heap.page_allocator, "vpn_cidr", "invalid-cidr");
    try section.put(std.heap.page_allocator, "public_interface", "eth0");
    try raw.put(std.heap.page_allocator, "vpn_masquerade", section);
    try std.testing.expectError(config.ConfigError.InvalidCidr, config.parseVpnMasqueradeConfig(&raw));
}

test "parseVpnMasqueradeConfig fails on empty vpn_cidr" {
    var raw = std.StringArrayHashMapUnmanaged(std.StringArrayHashMapUnmanaged([]const u8)){};
    defer raw.deinit(std.heap.page_allocator);
    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    defer section.deinit(std.heap.page_allocator);
    try section.put(std.heap.page_allocator, "enabled", "true");
    try section.put(std.heap.page_allocator, "vpn_cidr", "");
    try section.put(std.heap.page_allocator, "public_interface", "eth0");
    try raw.put(std.heap.page_allocator, "vpn_masquerade", section);
    try std.testing.expectError(config.ConfigError.EmptyValue, config.parseVpnMasqueradeConfig(&raw));
}

test "parseVpnMasqueradeConfig fails on empty public_interface" {
    var raw = std.StringArrayHashMapUnmanaged(std.StringArrayHashMapUnmanaged([]const u8)){};
    defer raw.deinit(std.heap.page_allocator);
    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    defer section.deinit(std.heap.page_allocator);
    try section.put(std.heap.page_allocator, "enabled", "true");
    try section.put(std.heap.page_allocator, "vpn_cidr", "10.0.0.0/8");
    try section.put(std.heap.page_allocator, "public_interface", "");
    try raw.put(std.heap.page_allocator, "vpn_masquerade", section);
    try std.testing.expectError(config.ConfigError.EmptyValue, config.parseVpnMasqueradeConfig(&raw));
}
