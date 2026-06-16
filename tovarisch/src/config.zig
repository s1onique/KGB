// config.zig — INI-style configuration parser for tovarisch
//
// Parses tovarisch.conf INI files with sections like:
//   [server]
//   listen = "127.0.0.1:8317"
//
//   [wg]
//   enabled = true
//   interface = wg-kgb0

const std = @import("std");
const bfd_config = @import("bfd/config_parse.zig");

/// Re-export BFD config types for backwards compatibility.
pub const BfdConfig = bfd_config.BfdConfig;
pub const parseBfdConfig = bfd_config.parseBfdConfig;

/// Configuration parse errors
pub const ConfigError = error{
    /// Section does not exist in config
    SectionNotFound,
    /// Required key is missing
    MissingKey,
    /// Value failed to parse (e.g., invalid boolean, port out of range)
    InvalidValue,
    /// Empty string when non-empty required
    EmptyValue,
    /// CIDR notation is invalid
    InvalidCidr,
    /// Port number out of valid range (1..65535)
    InvalidPort,
    /// WireGuard key is invalid (not 44 base64 characters)
    InvalidKey,
};

/// Raw section store for INI parsing.
/// Sections are stored as map of key=value strings.
pub const RawConfig = std.StringArrayHashMapUnmanaged(std.StringArrayHashMapUnmanaged([]const u8));

/// WgConfig represents the [wg] section parsed from tovarisch.conf.
pub const WgConfig = struct {
    /// Whether WireGuard config generation is enabled.
    enabled: bool = false,
    /// Local WireGuard interface name (e.g., "wg-kgb0").
    interface: []const u8 = "wg-kgb0",
    /// Address in CIDR notation (e.g., "10.77.0.1/24").
    address: []const u8 = "10.77.0.1/24",
    /// UDP listen port (1..65535).
    listen_port: u16 = 51820,
    /// Directory where generated WireGuard config files are written.
    output_dir: []const u8 = "/var/lib/kgb/wireguard",
    /// Path to the server private key file.
    private_key_file: []const u8 = "",
    /// Path to the server public key file (for client config generation).
    public_key_file: []const u8 = "",
    /// Allowed IPs for clients in generated client configs.
    client_allowed_ips: []const u8 = "10.149.149.0/24",
};

/// VpnMasqueradeConfig represents the [vpn_masquerade] section parsed from tovarisch.conf.
pub const VpnMasqueradeConfig = struct {
    /// Whether VPN masquerading is enabled.
    enabled: bool = false,
    /// VPN source CIDR for MASQUERADE rule (e.g., "10.0.0.0/8").
    vpn_cidr: []const u8 = "",
    /// Public egress interface for MASQUERADE rule (e.g., "eth0").
    public_interface: []const u8 = "",
};

/// Parse a boolean value from a string.
/// Accepts: "true", "false", "1", "0" (case-insensitive).
pub fn parseBool(value: []const u8) ConfigError!bool {
    const trimmed = std.mem.trim(u8, value, " \t\r\n");
    if (std.ascii.eqlIgnoreCase(trimmed, "true") or std.mem.eql(u8, trimmed, "1")) {
        return true;
    }
    if (std.ascii.eqlIgnoreCase(trimmed, "false") or std.mem.eql(u8, trimmed, "0")) {
        return false;
    }
    return ConfigError.InvalidValue;
}

/// Parse a port number from a string. Port must be in range 1..65535.
pub fn parsePort(value: []const u8) ConfigError!u16 {
    const trimmed = std.mem.trim(u8, value, " \t\r\n");
    const port = std.fmt.parseInt(u16, trimmed, 10) catch {
        return ConfigError.InvalidPort;
    };
    if (port < 1 or port > 65535) {
        return ConfigError.InvalidPort;
    }
    return port;
}

/// Parse CIDR notation. Returns address and prefix length.
pub fn parseCidr(value: []const u8) ConfigError!struct { address: []const u8, prefix: u8 } {
    const trimmed = std.mem.trim(u8, value, " \t\r\n");
    const slash_idx = std.mem.indexOfScalar(u8, trimmed, '/') orelse {
        return ConfigError.InvalidCidr;
    };
    const address_part = trimmed[0..slash_idx];
    const prefix_part = trimmed[slash_idx + 1 ..];

    if (address_part.len == 0) return ConfigError.InvalidCidr;

    var octets: [4]u8 = undefined;
    var octet_count: usize = 0;
    var start: usize = 0;

    for (address_part, 0..) |c, i| {
        if (c == '.') {
            if (i == start) return ConfigError.InvalidCidr;
            const octet_str = address_part[start..i];
            const octet = std.fmt.parseInt(u8, octet_str, 10) catch return ConfigError.InvalidCidr;
            if (octet_count >= 4) return ConfigError.InvalidCidr;
            octets[octet_count] = octet;
            octet_count += 1;
            start = i + 1;
        } else if (c < '0' or c > '9') {
            return ConfigError.InvalidCidr;
        }
    }

    if (start >= address_part.len) return ConfigError.InvalidCidr;
    if (address_part.len > 0 and address_part[address_part.len - 1] == '.') return ConfigError.InvalidCidr;
    const last_octet = std.fmt.parseInt(u8, address_part[start..], 10) catch return ConfigError.InvalidCidr;
    if (octet_count >= 4) return ConfigError.InvalidCidr;
    octets[octet_count] = last_octet;
    octet_count += 1;

    if (octet_count != 4) return ConfigError.InvalidCidr;

    const prefix = std.fmt.parseInt(u8, prefix_part, 10) catch return ConfigError.InvalidCidr;
    if (prefix > 32) return ConfigError.InvalidCidr;

    return .{ .address = address_part, .prefix = prefix };
}

/// Validate a non-empty string value.
pub fn requireNonEmpty(value: []const u8) ConfigError!void {
    const trimmed = std.mem.trim(u8, value, " \t\r\n");
    if (trimmed.len == 0) return ConfigError.EmptyValue;
}

/// Get a string value from a section, trimming whitespace.
pub fn getString(section: anytype, key: []const u8) ?[]const u8 {
    if (section.get(key)) |value| {
        return std.mem.trim(u8, value, " \t\r\n");
    }
    return null;
}

/// Parse the [wg] section from raw config into WgConfig.
pub fn parseWgConfig(raw: *const RawConfig) ConfigError!WgConfig {
    const wg_section = raw.get("wg") orelse return WgConfig{};

    var cfg = WgConfig{};
    if (getString(wg_section, "enabled")) |value| {
        cfg.enabled = try parseBool(value);
    }
    if (!cfg.enabled) return cfg;

    if (getString(wg_section, "interface")) |value| {
        try requireNonEmpty(value);
        cfg.interface = value;
    } else return ConfigError.MissingKey;

    if (getString(wg_section, "address")) |value| {
        try requireNonEmpty(value);
        const cidr = try parseCidr(value);
        cfg.address = value;
        _ = cidr;
    } else return ConfigError.MissingKey;

    if (getString(wg_section, "listen_port")) |value| {
        cfg.listen_port = try parsePort(value);
    } else return ConfigError.MissingKey;

    if (getString(wg_section, "output_dir")) |value| {
        try requireNonEmpty(value);
        cfg.output_dir = value;
    } else return ConfigError.MissingKey;

    if (getString(wg_section, "private_key_file")) |value| {
        try requireNonEmpty(value);
        cfg.private_key_file = value;
    } else return ConfigError.MissingKey;

    if (getString(wg_section, "public_key_file")) |value| {
        try requireNonEmpty(value);
        cfg.public_key_file = value;
    }

    if (getString(wg_section, "client_allowed_ips")) |value| {
        try requireNonEmpty(value);
        cfg.client_allowed_ips = value;
    }

    return cfg;
}

/// Parse the [vpn_masquerade] section from raw config into VpnMasqueradeConfig.
pub fn parseVpnMasqueradeConfig(raw: *const RawConfig) ConfigError!VpnMasqueradeConfig {
    const section = raw.get("vpn_masquerade") orelse return VpnMasqueradeConfig{};

    var cfg = VpnMasqueradeConfig{};
    if (getString(section, "enabled")) |value| {
        cfg.enabled = try parseBool(value);
    }
    if (!cfg.enabled) return cfg;

    // When enabled, require both vpn_cidr and public_interface
    if (getString(section, "vpn_cidr")) |value| {
        try requireNonEmpty(value);
        // Validate CIDR format
        _ = try parseCidr(value);
        cfg.vpn_cidr = value;
    } else return ConfigError.MissingKey;

    if (getString(section, "public_interface")) |value| {
        try requireNonEmpty(value);
        // Validate interface name conservatively
        if (!isValidInterfaceName(value)) {
            return ConfigError.InvalidValue;
        }
        cfg.public_interface = value;
    } else return ConfigError.MissingKey;

    return cfg;
}

/// Validates a network interface name conservatively.
/// Local conservative validator kept here to avoid config depending on net/iptables.
fn isValidInterfaceName(name: []const u8) bool {
    if (name.len == 0 or name.len > 15) return false;

    for (name) |c| {
        // Allow only conservative interface-name characters: [A-Za-z0-9_.-]
        if (c >= 'A' and c <= 'Z') continue;
        if (c >= 'a' and c <= 'z') continue;
        if (c >= '0' and c <= '9') continue;
        if (c == '_' or c == '.' or c == '-') continue;
        return false;
    }

    return true;
}

// --- Tests ---

test "parseBool accepts true variants" {
    try std.testing.expect(try parseBool("true"));
    try std.testing.expect(try parseBool("TRUE"));
    try std.testing.expect(try parseBool("True"));
    try std.testing.expect(try parseBool("1"));
}

test "parseBool accepts false variants" {
    try std.testing.expect(!try parseBool("false"));
    try std.testing.expect(!try parseBool("FALSE"));
    try std.testing.expect(!try parseBool("False"));
    try std.testing.expect(!try parseBool("0"));
}

test "parseBool rejects invalid values" {
    try std.testing.expectError(ConfigError.InvalidValue, parseBool("yes"));
    try std.testing.expectError(ConfigError.InvalidValue, parseBool("no"));
    try std.testing.expectError(ConfigError.InvalidValue, parseBool(""));
    try std.testing.expectError(ConfigError.InvalidValue, parseBool("2"));
}

test "parsePort accepts valid ports" {
    try std.testing.expectEqual(@as(u16, 1), try parsePort("1"));
    try std.testing.expectEqual(@as(u16, 51820), try parsePort("51820"));
    try std.testing.expectEqual(@as(u16, 65535), try parsePort("65535"));
}

test "parsePort rejects invalid ports" {
    try std.testing.expectError(ConfigError.InvalidPort, parsePort("0"));
    try std.testing.expectError(ConfigError.InvalidPort, parsePort("65536"));
    try std.testing.expectError(ConfigError.InvalidPort, parsePort("abc"));
    try std.testing.expectError(ConfigError.InvalidPort, parsePort("-1"));
}

test "parseCidr accepts valid CIDR" {
    const result = try parseCidr("10.77.0.1/24");
    try std.testing.expectEqualStrings("10.77.0.1", result.address);
    try std.testing.expectEqual(@as(u8, 24), result.prefix);
}

test "parseCidr accepts /0 prefix" {
    const result = try parseCidr("0.0.0.0/0");
    try std.testing.expectEqualStrings("0.0.0.0", result.address);
    try std.testing.expectEqual(@as(u8, 0), result.prefix);
}

test "parseCidr rejects invalid CIDR" {
    try std.testing.expectError(ConfigError.InvalidCidr, parseCidr("10.77.0.1"));
    try std.testing.expectError(ConfigError.InvalidCidr, parseCidr("10.77.0.1/33"));
    try std.testing.expectError(ConfigError.InvalidCidr, parseCidr("10.77.0.1/ab"));
    try std.testing.expectError(ConfigError.InvalidCidr, parseCidr("/24"));
}

test "parseCidr rejects invalid IPv4 addresses" {
    try std.testing.expectError(ConfigError.InvalidCidr, parseCidr("999.1.1.1/24"));
    try std.testing.expectError(ConfigError.InvalidCidr, parseCidr("10.77.0/24"));
    try std.testing.expectError(ConfigError.InvalidCidr, parseCidr(".10.77.0.1/24"));
}

test "requireNonEmpty accepts non-empty" {
    try requireNonEmpty("value");
    try requireNonEmpty("  trimmed  ");
}

test "requireNonEmpty rejects empty" {
    try std.testing.expectError(ConfigError.EmptyValue, requireNonEmpty(""));
    try std.testing.expectError(ConfigError.EmptyValue, requireNonEmpty("   "));
}

test "getString returns trimmed value" {
    var map = std.StringArrayHashMapUnmanaged([]const u8){};
    defer map.deinit(std.heap.page_allocator);
    try map.put(std.heap.page_allocator, "key", "  value  ");
    try std.testing.expectEqualStrings("value", getString(&map, "key").?);
}

test "getString returns null for missing key" {
    var map = std.StringArrayHashMapUnmanaged([]const u8){};
    defer map.deinit(std.heap.page_allocator);
    try std.testing.expect(getString(&map, "missing") == null);
}

// ============================================================================
// VPN Masquerade Config Tests
// ============================================================================

test "parseVpnMasqueradeConfig returns disabled default for missing section" {
    var raw = std.StringArrayHashMapUnmanaged(std.StringArrayHashMapUnmanaged([]const u8)){};
    defer raw.deinit(std.heap.page_allocator);
    const cfg = try parseVpnMasqueradeConfig(&raw);
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
    const cfg = try parseVpnMasqueradeConfig(&raw);
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
    const cfg = try parseVpnMasqueradeConfig(&raw);
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
    try std.testing.expectError(ConfigError.MissingKey, parseVpnMasqueradeConfig(&raw));
}

test "parseVpnMasqueradeConfig fails when enabled but missing public_interface" {
    var raw = std.StringArrayHashMapUnmanaged(std.StringArrayHashMapUnmanaged([]const u8)){};
    defer raw.deinit(std.heap.page_allocator);
    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    defer section.deinit(std.heap.page_allocator);
    try section.put(std.heap.page_allocator, "enabled", "true");
    try section.put(std.heap.page_allocator, "vpn_cidr", "10.0.0.0/8");
    try raw.put(std.heap.page_allocator, "vpn_masquerade", section);
    try std.testing.expectError(ConfigError.MissingKey, parseVpnMasqueradeConfig(&raw));
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
    try std.testing.expectError(ConfigError.InvalidCidr, parseVpnMasqueradeConfig(&raw));
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
    try std.testing.expectError(ConfigError.EmptyValue, parseVpnMasqueradeConfig(&raw));
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
    try std.testing.expectError(ConfigError.EmptyValue, parseVpnMasqueradeConfig(&raw));
}
