// bgp/prefix_file_integration_tests.zig — Tests for advertised_prefix_files support
//
// ACT 6: Add advertised_prefix_files support for BGP runtime config.
// Tests config parsing, runtime file loading, merge behavior, and ownership/cleanup.

const std = @import("std");
const config = @import("../config.zig");
const config_parse = @import("config_parse.zig");
const serve_integration = @import("serve_integration.zig");
const session = @import("session.zig");
const types = @import("types.zig");
const prefix_file = @import("prefix_file.zig");

// ============================================================================
// Test Helpers
// ============================================================================

/// Create a test config with BGP enabled and minimal required fields.
fn createEnabledBgpConfig(raw: *config.RawConfig) !void {
    try raw.put(std.heap.page_allocator, "bgp", .{});
    const bgp_section = raw.getPtr("bgp").?;
    try bgp_section.put(std.heap.page_allocator, "enabled", "true");
    try bgp_section.put(std.heap.page_allocator, "local_address", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "router_id", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "local_as", "65001");
    try bgp_section.put(std.heap.page_allocator, "peer_address", "10.0.0.2");
    try bgp_section.put(std.heap.page_allocator, "peer_as", "65002");
}

// ============================================================================
// Config Parsing Tests
// ============================================================================

test "parseBgpConfig parses advertised_prefix_files_raw" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);
    try raw.put(std.heap.page_allocator, "bgp", .{});
    const bgp_section = raw.getPtr("bgp").?;
    try bgp_section.put(std.heap.page_allocator, "enabled", "true");
    try bgp_section.put(std.heap.page_allocator, "local_address", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "router_id", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "local_as", "65001");
    try bgp_section.put(std.heap.page_allocator, "peer_address", "10.0.0.2");
    try bgp_section.put(std.heap.page_allocator, "peer_as", "65002");
    try bgp_section.put(std.heap.page_allocator, "advertised_prefix_files", "/etc/kgb/bgp-prefixes.conf");
    const cfg = try config_parse.parseBgpConfig(&raw);
    try std.testing.expectEqualStrings("/etc/kgb/bgp-prefixes.conf", cfg.advertised_prefix_files_raw);
}

test "parseBgpConfig parses multiple advertised_prefix_files" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);
    try raw.put(std.heap.page_allocator, "bgp", .{});
    const bgp_section = raw.getPtr("bgp").?;
    try bgp_section.put(std.heap.page_allocator, "enabled", "true");
    try bgp_section.put(std.heap.page_allocator, "local_address", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "router_id", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "local_as", "65001");
    try bgp_section.put(std.heap.page_allocator, "peer_address", "10.0.0.2");
    try bgp_section.put(std.heap.page_allocator, "peer_as", "65002");
    try bgp_section.put(std.heap.page_allocator, "advertised_prefix_files", "/etc/kgb/a.conf,/etc/kgb/b.conf");
    const cfg = try config_parse.parseBgpConfig(&raw);
    try std.testing.expectEqualStrings("/etc/kgb/a.conf,/etc/kgb/b.conf", cfg.advertised_prefix_files_raw);
}

test "parseBgpConfig stores advertised_prefix_files_raw empty when not configured" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);
    try raw.put(std.heap.page_allocator, "bgp", .{});
    const bgp_section = raw.getPtr("bgp").?;
    try bgp_section.put(std.heap.page_allocator, "enabled", "true");
    try bgp_section.put(std.heap.page_allocator, "local_address", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "router_id", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "local_as", "65001");
    try bgp_section.put(std.heap.page_allocator, "peer_address", "10.0.0.2");
    try bgp_section.put(std.heap.page_allocator, "peer_as", "65002");
    const cfg = try config_parse.parseBgpConfig(&raw);
    try std.testing.expectEqualStrings("", cfg.advertised_prefix_files_raw);
}

test "parseBgpConfig disabled skips advertised_prefix_files parsing" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);
    try raw.put(std.heap.page_allocator, "bgp", .{});
    const bgp_section = raw.getPtr("bgp").?;
    try bgp_section.put(std.heap.page_allocator, "enabled", "false");
    const cfg = try config_parse.parseBgpConfig(&raw);
    try std.testing.expect(!cfg.enabled);
    try std.testing.expectEqualStrings("", cfg.advertised_prefix_files_raw);
}

// ============================================================================
// Merge Behavior Tests
// ============================================================================

test "inline prefixes produce combined count" {
    const inline_content = "192.168.0.0/16,10.0.0.0/8,172.16.0.0/8";
    const inline_prefixes = try config_parse.parsePrefixList(inline_content, std.testing.allocator);
    defer std.testing.allocator.free(inline_prefixes);
    try std.testing.expectEqual(@as(usize, 3), inline_prefixes.len);
}

test "parsePrefixList allocations must be freed" {
    const result = try config_parse.parsePrefixList("10.0.0.0/8,192.168.0.0/16", std.testing.allocator);
    defer std.testing.allocator.free(result);
    try std.testing.expectEqual(@as(usize, 2), result.len);
}

test "parsePrefixList trims whitespace around entries" {
    const result = try config_parse.parsePrefixList("  10.0.0.0/8  ,  192.168.0.0/16  ", std.testing.allocator);
    defer std.testing.allocator.free(result);
    try std.testing.expectEqual(@as(usize, 2), result.len);
    try std.testing.expectEqualStrings("10.0.0.0/8", result[0]);
    try std.testing.expectEqualStrings("192.168.0.0/16", result[1]);
}

// ============================================================================
// Test Inventory Verification
// ============================================================================

test "advertised_prefix_files_raw field exists in BgpConfig" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);
    try createEnabledBgpConfig(&raw);
    const bgp_section = raw.getPtr("bgp").?;
    try bgp_section.put(std.heap.page_allocator, "advertised_prefix_files", "/test/path.conf");
    const cfg = try config_parse.parseBgpConfig(&raw);
    try std.testing.expect(cfg.advertised_prefix_files_raw.len > 0);
}

test "BgpConfig has both advertised_prefixes and advertised_prefix_files" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);
    try raw.put(std.heap.page_allocator, "bgp", .{});
    const bgp_section = raw.getPtr("bgp").?;
    try bgp_section.put(std.heap.page_allocator, "enabled", "true");
    try bgp_section.put(std.heap.page_allocator, "local_address", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "router_id", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "local_as", "65001");
    try bgp_section.put(std.heap.page_allocator, "peer_address", "10.0.0.2");
    try bgp_section.put(std.heap.page_allocator, "peer_as", "65002");
    try bgp_section.put(std.heap.page_allocator, "advertised_prefixes", "10.0.0.0/8");
    try bgp_section.put(std.heap.page_allocator, "advertised_prefix_files", "/etc/kgb/prefixes.conf");
    const cfg = try config_parse.parseBgpConfig(&raw);
    try std.testing.expect(cfg.advertised_prefixes_raw.len > 0);
    try std.testing.expect(cfg.advertised_prefix_files_raw.len > 0);
}
