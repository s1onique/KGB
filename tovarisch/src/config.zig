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
const Io = std.Io;

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
    /// Allowed IPs for clients in generated client configs (e.g., "10.149.149.0/24").
    client_allowed_ips: []const u8 = "10.149.149.0/24",
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

/// Parse a port number from a string.
/// Port must be in range 1..65535.
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

/// Parse CIDR notation from a string.
/// Returns the address portion (without prefix) and prefix length.
/// Format: "a.b.c.d/prefix" where prefix is 0..32.
pub fn parseCidr(value: []const u8) ConfigError!struct { address: []const u8, prefix: u8 } {
    const trimmed = std.mem.trim(u8, value, " \t\r\n");
    const slash_idx = std.mem.indexOfScalar(u8, trimmed, '/') orelse {
        return ConfigError.InvalidCidr;
    };
    const address_part = trimmed[0..slash_idx];
    const prefix_part = trimmed[slash_idx + 1 ..];

    // Validate address is not empty
    if (address_part.len == 0) {
        return ConfigError.InvalidCidr;
    }

    // Validate IPv4 octets (a.b.c.d format)
    var octets: [4]u8 = undefined;
    var octet_count: usize = 0;
    var start: usize = 0;

    for (address_part, 0..) |c, i| {
        if (c == '.') {
            if (i == start) return ConfigError.InvalidCidr; // leading dot
            const octet_str = address_part[start..i];
            const octet = std.fmt.parseInt(u8, octet_str, 10) catch return ConfigError.InvalidCidr;
            if (octet_count >= 4) return ConfigError.InvalidCidr;
            octets[octet_count] = octet;
            octet_count += 1;
            start = i + 1;
        } else if (c < '0' or c > '9') {
            return ConfigError.InvalidCidr; // non-digit character
        }
    }

    // Parse last octet
    if (start >= address_part.len) return ConfigError.InvalidCidr;
    if (address_part.len > 0 and address_part[address_part.len - 1] == '.') return ConfigError.InvalidCidr;
    const last_octet = std.fmt.parseInt(u8, address_part[start..], 10) catch return ConfigError.InvalidCidr;
    if (octet_count >= 4) return ConfigError.InvalidCidr;
    octets[octet_count] = last_octet;
    octet_count += 1;

    // Must have exactly 4 octets
    if (octet_count != 4) {
        return ConfigError.InvalidCidr;
    }

    // Validate prefix is a number 0..32
    const prefix = std.fmt.parseInt(u8, prefix_part, 10) catch {
        return ConfigError.InvalidCidr;
    };
    if (prefix > 32) {
        return ConfigError.InvalidCidr;
    }

    return .{
        .address = address_part,
        .prefix = prefix,
    };
}

/// Validate a non-empty string value.
pub fn requireNonEmpty(value: []const u8) ConfigError!void {
    const trimmed = std.mem.trim(u8, value, " \t\r\n");
    if (trimmed.len == 0) {
        return ConfigError.EmptyValue;
    }
}

/// Get a string value from a section, trimming whitespace.
pub fn getString(section: anytype, key: []const u8) ?[]const u8 {
    if (section.get(key)) |value| {
        return std.mem.trim(u8, value, " \t\r\n");
    }
    return null;
}

/// Parse the [wg] section from raw config into WgConfig.
/// If [wg] section is missing, returns WgConfig with defaults (disabled).
/// If enabled=true, validates required fields.
pub fn parseWgConfig(raw: *const RawConfig) ConfigError!WgConfig {
    const wg_section = raw.get("wg") orelse {
        // No [wg] section - return disabled defaults
        return WgConfig{};
    };

    var cfg = WgConfig{};

    if (getString(wg_section, "enabled")) |value| {
        cfg.enabled = try parseBool(value);
    }

    // If disabled, return defaults (no validation needed)
    if (!cfg.enabled) {
        return cfg;
    }

    // Parse interface
    if (getString(wg_section, "interface")) |value| {
        try requireNonEmpty(value);
        cfg.interface = value;
    } else {
        return ConfigError.MissingKey;
    }

    // Parse address (CIDR)
    if (getString(wg_section, "address")) |value| {
        try requireNonEmpty(value);
        const cidr = try parseCidr(value);
        // Reconstruct the full address with CIDR prefix
        cfg.address = value;
        _ = cidr; // Just validate it exists
    } else {
        return ConfigError.MissingKey;
    }

    // Parse listen_port
    if (getString(wg_section, "listen_port")) |value| {
        cfg.listen_port = try parsePort(value);
    } else {
        return ConfigError.MissingKey;
    }

    // Parse output_dir
    if (getString(wg_section, "output_dir")) |value| {
        try requireNonEmpty(value);
        cfg.output_dir = value;
    } else {
        return ConfigError.MissingKey;
    }

    // Parse private_key_file
    if (getString(wg_section, "private_key_file")) |value| {
        try requireNonEmpty(value);
        cfg.private_key_file = value;
    } else {
        return ConfigError.MissingKey;
    }

    // Parse optional public_key_file (needed for client config generation)
    if (getString(wg_section, "public_key_file")) |value| {
        try requireNonEmpty(value);
        cfg.public_key_file = value;
    }

    // Parse optional client_allowed_ips (defaults to 10.149.149.0/24)
    if (getString(wg_section, "client_allowed_ips")) |value| {
        try requireNonEmpty(value);
        cfg.client_allowed_ips = value;
    }

    return cfg;
}

// --- Tests ---

const VoidWriter = struct {
    const Self = @This();
    pub fn writeAll(_: Self, _: []const u8) error{}!void {}
    pub fn write(_: Self, _: []const u8) error{}!void {}
    pub fn print(_: Self, _: []const u8, _: anytype) error{}!void {}
    pub fn writeByte(_: Self, _: u8) error{}!void {}
    pub fn flush(_: Self) error{}!void {}
};

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

    const result2 = try parseCidr("192.168.1.1/32");
    try std.testing.expectEqualStrings("192.168.1.1", result2.address);
    try std.testing.expectEqual(@as(u8, 32), result2.prefix);
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
    // Non-numeric octets
    try std.testing.expectError(ConfigError.InvalidCidr, parseCidr("999.1.1.1/24"));
    try std.testing.expectError(ConfigError.InvalidCidr, parseCidr("abc.def.ghi.jkl/24"));
    // Wrong number of octets
    try std.testing.expectError(ConfigError.InvalidCidr, parseCidr("10.77.0/24"));
    try std.testing.expectError(ConfigError.InvalidCidr, parseCidr("10.77.0.1.2/24"));
    // Trailing/leading dots
    try std.testing.expectError(ConfigError.InvalidCidr, parseCidr(".10.77.0.1/24"));
    try std.testing.expectError(ConfigError.InvalidCidr, parseCidr("10.77.0.1./24"));
}

test "requireNonEmpty accepts non-empty" {
    try requireNonEmpty("value");
    try requireNonEmpty("  trimmed  ");
}

test "requireNonEmpty rejects empty" {
    try std.testing.expectError(ConfigError.EmptyValue, requireNonEmpty(""));
    try std.testing.expectError(ConfigError.EmptyValue, requireNonEmpty("   "));
    try std.testing.expectError(ConfigError.EmptyValue, requireNonEmpty("\t"));
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
