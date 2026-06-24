// config_parse_helpers.zig — Shared configuration parsing helpers
//
// Extracted from config.zig to avoid import cycles when config_lab.zig
// needs to share parsing utilities.

const std = @import("std");

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

/// Validates a network interface name conservatively.
/// Local conservative validator kept here to avoid config depending on net/iptables.
pub fn isValidInterfaceName(name: []const u8) bool {
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
