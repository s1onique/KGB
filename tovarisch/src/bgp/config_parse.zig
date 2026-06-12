// bgp/config_parse.zig — BGP configuration parsing
//
// ACT 4: Wire BGP session into tovarisch serve runtime.
// Parses the [bgp] section from tovarisch.conf INI files.
// BGP is disabled by default - no sockets are created unless explicitly enabled.
//
// References: RFC 4271 (BGP-4)

const std = @import("std");
const config = @import("../config.zig");
const types = @import("types.zig");
const validation = @import("validation.zig");

/// BgpConfig represents the [bgp] section parsed from tovarisch.conf.
/// This struct holds raw string values only - no allocations.
/// Runtime-owned derived values (like parsed prefixes) are built in serve_integration.zig.
pub const BgpConfig = struct {
    /// Whether the [bgp] section was present in the config file.
    present: bool = false,
    /// Whether BGP is enabled.
    enabled: bool = false,
    /// Our local IPv4 address as string.
    local_address: []const u8 = "",
    /// Our router ID as string (IPv4 dotted notation).
    router_id: []const u8 = "",
    /// Our local ASN (1..65535).
    local_as: u16 = 0,
    /// Peer's IPv4 address as string.
    peer_address: []const u8 = "",
    /// Peer's BGP port (default 179).
    peer_port: u16 = 179,
    /// Peer's ASN (1..65535).
    peer_as: u16 = 0,
    /// Hold time in seconds (0 or >= 3).
    hold_time_seconds: u16 = 180,
    /// Keepalive interval in seconds (< hold_time when hold_time != 0).
    keepalive_seconds: u16 = 60,
    /// TCP connection timeout in milliseconds (not yet enforced).
    connect_timeout_ms: u32 = 1000,
    /// Raw advertised_prefixes string (comma-separated CIDR list).
    /// Parsed into owned Ipv4Prefix slice in serve_integration.zig.
    advertised_prefixes_raw: []const u8 = "",
    /// Raw advertised_prefix_files string (comma-separated file paths).
    /// Parsed into owned Ipv4Prefix slice in serve_integration.zig.
    advertised_prefix_files_raw: []const u8 = "",
    /// If true, AS_PATH is empty (same-AS/iBGP style).
    same_as: bool = false,
};

/// Parse a plain IPv4 address string to [4]u8.
/// Unlike Ipv4Prefix.parse(), this does NOT accept CIDR notation.
/// Returns error on invalid format, IPv6, or CIDR suffix.
pub fn parseIpv4Address(addr_str: []const u8) config.ConfigError![4]u8 {
    var addr: [4]u8 = .{ 0, 0, 0, 0 };
    var octet_idx: usize = 0;
    var pos: usize = 0;
    var octet_val: u32 = 0;
    var saw_digit_in_octet: bool = false;

    while (pos <= addr_str.len) : (pos += 1) {
        const c = if (pos < addr_str.len) addr_str[pos] else '.';
        if (c == '.') {
            // Reject empty octet (leading dot, consecutive dots, or trailing dot)
            if (!saw_digit_in_octet) return config.ConfigError.InvalidCidr;
            if (octet_idx >= 4) return config.ConfigError.InvalidCidr;
            if (octet_val > 255) return config.ConfigError.InvalidCidr;
            addr[octet_idx] = @as(u8, @intCast(octet_val));
            octet_idx += 1;
            octet_val = 0;
            saw_digit_in_octet = false;
        } else if (c >= '0' and c <= '9') {
            octet_val = octet_val * 10 + (c - '0');
            if (octet_val > 255) return config.ConfigError.InvalidCidr;
            saw_digit_in_octet = true;
        } else if (c == ':') {
            // IPv6 indicator
            return config.ConfigError.InvalidCidr;
        } else {
            return config.ConfigError.InvalidCidr;
        }
    }
    // Reject if we didn't get exactly 4 octets
    if (octet_idx != 4) return config.ConfigError.InvalidCidr;

    return addr;
}

/// Parse a comma-separated list of CIDR prefixes.
/// Returns allocated slice of CIDR strings.
/// Empty string is allowed - enables zero-prefix BGP smoke test mode.
pub fn parsePrefixList(prefix_list_str: []const u8, allocator: std.mem.Allocator) config.ConfigError![]const []const u8 {
    // Empty string is valid - returns empty slice for zero-prefix BGP smoke test
    if (prefix_list_str.len == 0) {
        return &[_][]const u8{};
    }

    var result = std.ArrayList([]const u8).empty;
    errdefer result.deinit(allocator);

    var start: usize = 0;
    for (prefix_list_str, 0..) |c, i| {
        if (c == ',') {
            const prefix = std.mem.trim(u8, prefix_list_str[start..i], " \t");
            if (prefix.len > 0) {
                result.append(allocator, prefix) catch return config.ConfigError.InvalidValue;
            }
            start = i + 1;
        }
    }
    // Last entry
    const last = std.mem.trim(u8, prefix_list_str[start..], " \t");
    if (last.len > 0) {
        result.append(allocator, last) catch return config.ConfigError.InvalidValue;
    }

    // Empty result is valid - caller handles zero prefixes case
    return result.toOwnedSlice(allocator) catch return config.ConfigError.InvalidValue;
}

/// Parse the [bgp] section from raw config into BgpConfig.
/// If [bgp] section is missing, returns BgpConfig with defaults (disabled, present=false).
/// If enabled=true, validates required fields.
pub fn parseBgpConfig(raw: *const config.RawConfig) config.ConfigError!BgpConfig {
    const bgp_section = raw.get("bgp") orelse {
        // No [bgp] section - return disabled defaults
        return BgpConfig{ .present = false };
    };

    var cfg = BgpConfig{ .present = true };

    if (config.getString(bgp_section, "enabled")) |value| {
        cfg.enabled = try config.parseBool(value);
    }

    // If disabled, return defaults (no validation needed)
    if (!cfg.enabled) {
        return cfg;
    }

    // Parse local_address (required when enabled)
    if (config.getString(bgp_section, "local_address")) |value| {
        try config.requireNonEmpty(value);
        cfg.local_address = value;
    } else {
        return config.ConfigError.MissingKey;
    }

    // Parse router_id (required when enabled)
    if (config.getString(bgp_section, "router_id")) |value| {
        try config.requireNonEmpty(value);
        cfg.router_id = value;
    } else {
        return config.ConfigError.MissingKey;
    }

    // Parse local_as (required when enabled)
    if (config.getString(bgp_section, "local_as")) |value| {
        const trimmed = std.mem.trim(u8, value, " \t\r\n");
        cfg.local_as = std.fmt.parseInt(u16, trimmed, 10) catch return config.ConfigError.InvalidValue;
    } else {
        return config.ConfigError.MissingKey;
    }

    // Parse peer_address (required when enabled)
    if (config.getString(bgp_section, "peer_address")) |value| {
        try config.requireNonEmpty(value);
        cfg.peer_address = value;
    } else {
        return config.ConfigError.MissingKey;
    }

    // Parse peer_port (optional, default 179)
    if (config.getString(bgp_section, "peer_port")) |value| {
        cfg.peer_port = try config.parsePort(value);
    }

    // Parse peer_as (required when enabled)
    if (config.getString(bgp_section, "peer_as")) |value| {
        const trimmed = std.mem.trim(u8, value, " \t\r\n");
        cfg.peer_as = std.fmt.parseInt(u16, trimmed, 10) catch return config.ConfigError.InvalidValue;
    } else {
        return config.ConfigError.MissingKey;
    }

    // Parse hold_time_seconds (optional)
    if (config.getString(bgp_section, "hold_time_seconds")) |value| {
        const trimmed = std.mem.trim(u8, value, " \t\r\n");
        cfg.hold_time_seconds = std.fmt.parseInt(u16, trimmed, 10) catch return config.ConfigError.InvalidValue;
    }

    // Parse keepalive_seconds (optional)
    if (config.getString(bgp_section, "keepalive_seconds")) |value| {
        const trimmed = std.mem.trim(u8, value, " \t\r\n");
        cfg.keepalive_seconds = std.fmt.parseInt(u16, trimmed, 10) catch return config.ConfigError.InvalidValue;
    }

    // Parse connect_timeout_ms (optional)
    if (config.getString(bgp_section, "connect_timeout_ms")) |value| {
        const trimmed = std.mem.trim(u8, value, " \t\r\n");
        cfg.connect_timeout_ms = std.fmt.parseInt(u32, trimmed, 10) catch return config.ConfigError.InvalidValue;
    }

    // Parse advertised_prefixes (comma-separated CIDR list)
    // Store raw string - parsing into owned Ipv4Prefix slice happens in serve_integration.zig.
    // Empty string is allowed for zero-prefix BGP smoke test mode.
    if (config.getString(bgp_section, "advertised_prefixes")) |value| {
        cfg.advertised_prefixes_raw = value;
    }

    // Parse advertised_prefix_files (comma-separated file paths)
    // Store raw string - parsing into owned Ipv4Prefix slice happens in serve_integration.zig.
    // Empty string is allowed (treated as no prefix files).
    if (config.getString(bgp_section, "advertised_prefix_files")) |value| {
        cfg.advertised_prefix_files_raw = value;
    }

    // Parse same_as (optional, default false)
    if (config.getString(bgp_section, "same_as")) |value| {
        cfg.same_as = try config.parseBool(value);
    }

    return cfg;
}

// --- Tests ---

const VoidWriter = struct {
    const Self = @This();
    pub fn writeAll(_: Self, _: []const u8) error{}!void {}
    pub fn write(_: Self, _: []const u8) error{}!void {}
    pub fn print(_: Self, _: []const u8, _: anytype) error{}!void {}
};

test "parseBgpConfig returns disabled by default" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);

    const cfg = try parseBgpConfig(&raw);
    try std.testing.expect(!cfg.enabled);
}

test "parseBgpConfig parses enabled config with required fields" {
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

    const cfg = try parseBgpConfig(&raw);
    try std.testing.expect(cfg.enabled);
    try std.testing.expectEqualStrings("10.0.0.1", cfg.local_address);
    try std.testing.expectEqualStrings("10.0.0.1", cfg.router_id);
    try std.testing.expectEqual(@as(u16, 65001), cfg.local_as);
    try std.testing.expectEqualStrings("10.0.0.2", cfg.peer_address);
    try std.testing.expectEqual(@as(u16, 65002), cfg.peer_as);
    try std.testing.expectEqual(@as(u16, 179), cfg.peer_port); // default
    try std.testing.expect(cfg.advertised_prefixes_raw.len > 0);
}

test "parseBgpConfig requires local_address when enabled" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);

    try raw.put(std.heap.page_allocator, "bgp", .{});
    const bgp_section = raw.getPtr("bgp").?;
    try bgp_section.put(std.heap.page_allocator, "enabled", "true");
    try bgp_section.put(std.heap.page_allocator, "router_id", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "local_as", "65001");
    try bgp_section.put(std.heap.page_allocator, "peer_address", "10.0.0.2");
    try bgp_section.put(std.heap.page_allocator, "peer_as", "65002");

    try std.testing.expectError(config.ConfigError.MissingKey, parseBgpConfig(&raw));
}

test "parseBgpConfig requires peer_address when enabled" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);

    try raw.put(std.heap.page_allocator, "bgp", .{});
    const bgp_section = raw.getPtr("bgp").?;
    try bgp_section.put(std.heap.page_allocator, "enabled", "true");
    try bgp_section.put(std.heap.page_allocator, "local_address", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "router_id", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "local_as", "65001");
    try bgp_section.put(std.heap.page_allocator, "peer_as", "65002");

    try std.testing.expectError(config.ConfigError.MissingKey, parseBgpConfig(&raw));
}

test "parseBgpConfig accepts disabled with missing fields" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);

    try raw.put(std.heap.page_allocator, "bgp", .{});
    const bgp_section = raw.getPtr("bgp").?;
    try bgp_section.put(std.heap.page_allocator, "enabled", "false");

    const cfg = try parseBgpConfig(&raw);
    try std.testing.expect(!cfg.enabled);
}

test "parseBgpConfig parses multiple advertised_prefixes" {
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
    try bgp_section.put(std.heap.page_allocator, "advertised_prefixes", "10.0.0.0/8,192.168.0.0/16");

    const cfg = try parseBgpConfig(&raw);
    try std.testing.expectEqualStrings("10.0.0.0/8,192.168.0.0/16", cfg.advertised_prefixes_raw);
}

test "parseIpv4Address accepts valid IPv4" {
    const addr = try parseIpv4Address("10.0.0.1");
    try std.testing.expectEqual(@as(u8, 10), addr[0]);
    try std.testing.expectEqual(@as(u8, 0), addr[1]);
    try std.testing.expectEqual(@as(u8, 0), addr[2]);
    try std.testing.expectEqual(@as(u8, 1), addr[3]);
}

test "parseIpv4Address accepts 127.0.0.1" {
    const addr = try parseIpv4Address("127.0.0.1");
    try std.testing.expectEqual(@as(u8, 127), addr[0]);
    try std.testing.expectEqual(@as(u8, 0), addr[1]);
    try std.testing.expectEqual(@as(u8, 0), addr[2]);
    try std.testing.expectEqual(@as(u8, 1), addr[3]);
}

test "parseIpv4Address rejects CIDR suffix" {
    try std.testing.expectError(config.ConfigError.InvalidCidr, parseIpv4Address("10.0.0.1/32"));
}

test "parseIpv4Address rejects IPv6" {
    try std.testing.expectError(config.ConfigError.InvalidCidr, parseIpv4Address("::1"));
    try std.testing.expectError(config.ConfigError.InvalidCidr, parseIpv4Address("2001:db8::1"));
}

test "parseIpv4Address rejects out of range octets" {
    try std.testing.expectError(config.ConfigError.InvalidCidr, parseIpv4Address("10.0.0.256"));
    try std.testing.expectError(config.ConfigError.InvalidCidr, parseIpv4Address("256.0.0.1"));
}

test "parsePrefixList parses single prefix" {
    const result = try parsePrefixList("10.0.0.0/8", std.heap.page_allocator);
    defer std.heap.page_allocator.free(result);
    try std.testing.expectEqual(@as(usize, 1), result.len);
    try std.testing.expectEqualStrings("10.0.0.0/8", result[0]);
}

test "parsePrefixList parses multiple prefixes" {
    const result = try parsePrefixList("10.0.0.0/8, 192.168.0.0/16", std.heap.page_allocator);
    defer std.heap.page_allocator.free(result);
    try std.testing.expectEqual(@as(usize, 2), result.len);
}

test "parsePrefixList accepts empty for zero-prefix BGP smoke test" {
    const result = try parsePrefixList("", std.heap.page_allocator);
    defer std.heap.page_allocator.free(result);
    try std.testing.expectEqual(@as(usize, 0), result.len);
}

test "parsePrefixList accepts whitespace-only for zero-prefix BGP smoke test" {
    const result = try parsePrefixList("   ", std.heap.page_allocator);
    defer std.heap.page_allocator.free(result);
    try std.testing.expectEqual(@as(usize, 0), result.len);
}
