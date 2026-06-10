// bfd/config_parse.zig — BFD configuration parsing
//
// Parses the [bfd] section from tovarisch.conf INI files.

const std = @import("std");
const config = @import("../config.zig");

/// BfdConfig represents the [bfd] section parsed from tovarisch.conf.
pub const BfdConfig = struct {
    /// Whether BFD is enabled.
    enabled: bool = false,
    /// Local BFD address.
    local_addr: []const u8 = "",
    /// Peer BFD address.
    peer_addr: []const u8 = "",
    /// Transmit interval in milliseconds.
    interval_ms: u32 = 800,
    /// Detection multiplier.
    multiplier: u8 = 3,
};

/// Parse the [bfd] section from raw config into BfdConfig.
/// If [bfd] section is missing, returns BfdConfig with defaults (disabled).
/// If enabled=true, validates required fields.
pub fn parseBfdConfig(raw: *const config.RawConfig) config.ConfigError!BfdConfig {
    const bfd_section = raw.get("bfd") orelse {
        // No [bfd] section - return disabled defaults
        return BfdConfig{};
    };

    var cfg = BfdConfig{};

    if (config.getString(bfd_section, "enabled")) |value| {
        cfg.enabled = try config.parseBool(value);
    }

    // If disabled, return defaults (no validation needed)
    if (!cfg.enabled) {
        return cfg;
    }

    // Parse local_addr (required when enabled)
    if (config.getString(bfd_section, "local_addr")) |value| {
        try config.requireNonEmpty(value);
        cfg.local_addr = value;
    } else {
        return config.ConfigError.MissingKey;
    }

    // Parse peer_addr (required when enabled)
    if (config.getString(bfd_section, "peer_addr")) |value| {
        try config.requireNonEmpty(value);
        cfg.peer_addr = value;
    } else {
        return config.ConfigError.MissingKey;
    }

    // Parse optional interval_ms
    if (config.getString(bfd_section, "interval_ms")) |value| {
        const trimmed = std.mem.trim(u8, value, " \t\r\n");
        cfg.interval_ms = std.fmt.parseInt(u32, trimmed, 10) catch return config.ConfigError.InvalidValue;
    }

    // Parse optional multiplier
    if (config.getString(bfd_section, "multiplier")) |value| {
        const trimmed = std.mem.trim(u8, value, " \t\r\n");
        cfg.multiplier = std.fmt.parseInt(u8, trimmed, 10) catch return config.ConfigError.InvalidValue;
    }

    return cfg;
}

// --- Tests ---

test "parseBfdConfig returns disabled by default" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);

    const cfg = try parseBfdConfig(&raw);
    try std.testing.expect(!cfg.enabled);
}

test "parseBfdConfig parses enabled config" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);

    try raw.put(std.heap.page_allocator, "bfd", .{});
    const bfd_section = raw.getPtr("bfd").?;
    try bfd_section.put(std.heap.page_allocator, "enabled", "true");
    try bfd_section.put(std.heap.page_allocator, "local_addr", "10.149.149.1");
    try bfd_section.put(std.heap.page_allocator, "peer_addr", "10.149.149.10");
    try bfd_section.put(std.heap.page_allocator, "interval_ms", "800");
    try bfd_section.put(std.heap.page_allocator, "multiplier", "3");

    const cfg = try parseBfdConfig(&raw);
    try std.testing.expect(cfg.enabled);
    try std.testing.expectEqualStrings("10.149.149.1", cfg.local_addr);
    try std.testing.expectEqualStrings("10.149.149.10", cfg.peer_addr);
    try std.testing.expectEqual(@as(u32, 800), cfg.interval_ms);
    try std.testing.expectEqual(@as(u8, 3), cfg.multiplier);
}

test "parseBfdConfig requires local_addr when enabled" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);

    try raw.put(std.heap.page_allocator, "bfd", .{});
    const bfd_section = raw.getPtr("bfd").?;
    try bfd_section.put(std.heap.page_allocator, "enabled", "true");
    try bfd_section.put(std.heap.page_allocator, "peer_addr", "10.149.149.10");

    try std.testing.expectError(config.ConfigError.MissingKey, parseBfdConfig(&raw));
}

test "parseBfdConfig requires peer_addr when enabled" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);

    try raw.put(std.heap.page_allocator, "bfd", .{});
    const bfd_section = raw.getPtr("bfd").?;
    try bfd_section.put(std.heap.page_allocator, "enabled", "true");
    try bfd_section.put(std.heap.page_allocator, "local_addr", "10.149.149.1");

    try std.testing.expectError(config.ConfigError.MissingKey, parseBfdConfig(&raw));
}

test "parseBfdConfig accepts disabled with missing addresses" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);

    try raw.put(std.heap.page_allocator, "bfd", .{});
    const bfd_section = raw.getPtr("bfd").?;
    try bfd_section.put(std.heap.page_allocator, "enabled", "false");

    const cfg = try parseBfdConfig(&raw);
    try std.testing.expect(!cfg.enabled);
}
