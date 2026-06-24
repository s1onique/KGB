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
const config_parse_helpers = @import("config_parse_helpers.zig");
const config_lab = @import("config_lab.zig");

/// Re-export BFD config types for backwards compatibility.
pub const BfdConfig = bfd_config.BfdConfig;
pub const parseBfdConfig = bfd_config.parseBfdConfig;

/// Re-export config_parse_helpers types for backwards compatibility.
pub const ConfigError = config_parse_helpers.ConfigError;
pub const parseBool = config_parse_helpers.parseBool;
pub const parsePort = config_parse_helpers.parsePort;
pub const parseCidr = config_parse_helpers.parseCidr;
pub const requireNonEmpty = config_parse_helpers.requireNonEmpty;
pub const getString = config_parse_helpers.getString;
pub const isValidInterfaceName = config_parse_helpers.isValidInterfaceName;

/// Re-export LabConfig from config_lab for backwards compatibility.
pub const LabConfig = config_lab.LabConfig;

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

/// ServerConfig represents the [server] section parsed from tovarisch.conf.
/// This controls the HTTP status server bind address.
pub const ServerConfig = struct {
    /// Listen address and port (e.g., "127.0.0.1:8317" or "10.149.149.1:8317").
    /// When null, the HTTP server uses its default (127.0.0.1:8317).
    listen: ?[]const u8 = null,
};

/// Parse the [server] section from raw config into ServerConfig.
pub fn parseServerConfig(raw: *const RawConfig) ServerConfig {
    const section = raw.get("server") orelse return ServerConfig{};

    var cfg = ServerConfig{};
    if (getString(section, "listen")) |value| {
        cfg.listen = value;
    }

    return cfg;
}

/// VpnMasqueradeConfig represents the [vpn_masquerade] section parsed from tovarisch.conf.
pub const VpnMasqueradeConfig = struct {
    /// Whether VPN masquerading is enabled.
    enabled: bool = false,
    /// VPN source CIDR for MASQUERADE rule (e.g., "10.0.0.0/8").
    vpn_cidr: []const u8 = "",
    /// Public egress interface for MASQUERADE rule (e.g., "eth0").
    public_interface: []const u8 = "",
};

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

/// Parse the [lab] section from raw config into LabConfig.
/// Delegates to config_lab.parseLabConfigSection for the actual parsing.
pub fn parseLabConfig(raw: *const RawConfig) ConfigError!LabConfig {
    const section = raw.get("lab") orelse return LabConfig{};
    return config_lab.parseLabConfigSection(section);
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
