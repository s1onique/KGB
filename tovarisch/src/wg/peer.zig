// wg/peer.zig — WireGuard peer configuration types
//
// This module defines the WgPeer struct and peer-related parsing
// and validation functions.

const std = @import("std");
const config = @import("../config.zig");

/// Configuration parse errors for peer config
pub const PeerConfigError = error{
    /// Required key is missing
    MissingKey,
    /// Value failed to parse (e.g., invalid boolean, CIDR, port out of range)
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

/// WgPeer represents a single [wg.peer.<name>] section.
pub const WgPeer = struct {
    /// Peer name (e.g., "phone", "laptop").
    name: []const u8,
    /// Whether this peer is enabled.
    enabled: bool = false,
    /// Peer interface address in CIDR notation (e.g., "10.149.149.10/32").
    address: []const u8 = "",
    /// Path to the peer's private key file.
    private_key_file: []const u8 = "",
    /// Path to the peer's public key file.
    public_key_file: []const u8 = "",
    /// Allowed IPs for this peer (comma-separated CIDR list).
    allowed_ips: []const u8 = "",
    /// Optional endpoint for the peer.
    endpoint: ?[]const u8 = null,
    /// Optional persistent keepalive interval (0..65535 seconds).
    persistent_keepalive: ?u16 = null,
    /// Path to write the generated client config file.
    client_output_file: []const u8 = "",
};

/// Validate a WireGuard key (44 base64 characters).
/// Accepts keys where the final character may be '=' (padded output from wg pubkey).
/// Total length must be exactly 44 characters.
pub fn validateKey(key: []const u8) PeerConfigError!void {
    const trimmed = std.mem.trim(u8, key, " \t\r\n");

    // WireGuard keys are exactly 44 characters total (including optional final '=')
    if (trimmed.len != 44) {
        return PeerConfigError.InvalidKey;
    }

    for (trimmed, 0..) |c, i| {
        const valid_base64 =
            (c >= 'A' and c <= 'Z') or
            (c >= 'a' and c <= 'z') or
            (c >= '0' and c <= '9') or
            c == '+' or
            c == '/';

        if (valid_base64) continue;

        // Allow '=' only as the final character
        if (c == '=' and i == trimmed.len - 1) continue;

        return PeerConfigError.InvalidKey;
    }
}

/// Parse persistent_keepalive value (optional, 0..65535).
pub fn parseKeepalive(value: []const u8) PeerConfigError!u16 {
    const trimmed = std.mem.trim(u8, value, " \t\r\n");
    const keepalive = std.fmt.parseInt(u16, trimmed, 10) catch {
        return PeerConfigError.InvalidPort;
    };
    // 0 is valid (disabled), 1..65535 is valid keepalive
    if (keepalive > 65535) {
        return PeerConfigError.InvalidPort;
    }
    return keepalive;
}

/// Parse peer section from raw config into WgPeer.
/// Validates required fields if enabled.
pub fn parsePeerConfig(
    name: []const u8,
    section: *const std.StringArrayHashMapUnmanaged([]const u8),
) PeerConfigError!WgPeer {
    var peer = WgPeer{ .name = name };

    if (config.getString(section, "enabled")) |value| {
        peer.enabled = config.parseBool(value) catch {
            return PeerConfigError.InvalidValue;
        };
    }

    // If disabled, return defaults (no validation needed)
    if (!peer.enabled) {
        return peer;
    }

    // Parse address (CIDR)
    if (config.getString(section, "address")) |value| {
        config.requireNonEmpty(value) catch return PeerConfigError.EmptyValue;
        _ = config.parseCidr(value) catch return PeerConfigError.InvalidCidr;
        peer.address = value;
    } else {
        return PeerConfigError.MissingKey;
    }

    // Parse private_key_file
    if (config.getString(section, "private_key_file")) |value| {
        config.requireNonEmpty(value) catch return PeerConfigError.EmptyValue;
        peer.private_key_file = value;
    } else {
        return PeerConfigError.MissingKey;
    }

    // Parse public_key_file
    if (config.getString(section, "public_key_file")) |value| {
        config.requireNonEmpty(value) catch return PeerConfigError.EmptyValue;
        peer.public_key_file = value;
    } else {
        return PeerConfigError.MissingKey;
    }

    // Parse allowed_ips
    if (config.getString(section, "allowed_ips")) |value| {
        config.requireNonEmpty(value) catch return PeerConfigError.EmptyValue;
        peer.allowed_ips = value;
    } else {
        return PeerConfigError.MissingKey;
    }

    // Parse optional endpoint
    if (config.getString(section, "endpoint")) |value| {
        config.requireNonEmpty(value) catch return PeerConfigError.EmptyValue;
        peer.endpoint = value;
    }

    // Parse optional persistent_keepalive
    if (config.getString(section, "persistent_keepalive")) |value| {
        peer.persistent_keepalive = try parseKeepalive(value);
    }

    // Parse client_output_file
    if (config.getString(section, "client_output_file")) |value| {
        config.requireNonEmpty(value) catch return PeerConfigError.EmptyValue;
        peer.client_output_file = value;
    } else {
        return PeerConfigError.MissingKey;
    }

    return peer;
}

/// Read and validate a WireGuard public key from file.
pub fn readPublicKey(key_path: []const u8, allocator: std.mem.Allocator) config.ConfigError![]const u8 {
    var path_buf: [4096]u8 = undefined;
    if (key_path.len >= path_buf.len) return config.ConfigError.InvalidValue;
    // MemoryCopySafety: path_buf is a fixed [4096]u8 buffer, key_path is a caller-provided slice. These buffers are independent - no aliasing.
    @memcpy(path_buf[0..key_path.len], key_path);
    path_buf[key_path.len] = 0;
    const c_path: [*:0]const u8 = @ptrCast(&path_buf);

    const fd = std.c.open(c_path, @bitCast(@as(u32, 0)));
    if (fd < 0) {
        return config.ConfigError.InvalidValue;
    }
    defer _ = std.c.close(fd);

    // Read the key (WireGuard public keys are 44 base64 chars + newline).
    var key_buf: [64]u8 = undefined;
    const bytes_read = std.c.read(fd, &key_buf, key_buf.len);
    if (bytes_read < 0) {
        return config.ConfigError.InvalidValue;
    }

    // Trim whitespace
    const key = std.mem.trim(u8, key_buf[0..@as(usize, @intCast(bytes_read))], " \t\r\n");

    // Validate key length and format
    try validateKey(key);

    // Duplicate the key for the caller
    const owned_key = allocator.dupe(u8, key) catch return config.ConfigError.EmptyValue;
    return owned_key;
}

// --- Tests ---

// 44-character base64 key constant for tests
const valid_key_44 = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA";

test "validateKey accepts valid key" {
    try validateKey(valid_key_44);
}

test "validateKey rejects wrong length" {
    try std.testing.expectError(PeerConfigError.InvalidKey, validateKey("short"));
    try std.testing.expectError(PeerConfigError.InvalidKey, validateKey("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")); // 43 chars
    try std.testing.expectError(PeerConfigError.InvalidKey, validateKey("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")); // 45 chars
}

test "validateKey rejects invalid characters" {
    // Key with invalid character (!)
    const invalid_key = valid_key_44 ++ "!";
    try std.testing.expectError(PeerConfigError.InvalidKey, validateKey(invalid_key));
}

test "validateKey accepts padded key" {
    // 44-character key: 43 base64 chars + final '=' (normal wg pubkey output)
    try validateKey("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=");
}

test "validateKey accepts unpadded key" {
    // 44 base64 chars without padding
    try validateKey(valid_key_44);
}

test "validateKey rejects key with invalid padding" {
    // Key with invalid character after base64 portion
    const invalid_padding = valid_key_44 ++ "!";
    try std.testing.expectError(PeerConfigError.InvalidKey, validateKey(invalid_padding));
}

test "parseKeepalive accepts valid values" {
    try std.testing.expectEqual(@as(u16, 0), try parseKeepalive("0"));
    try std.testing.expectEqual(@as(u16, 25), try parseKeepalive("25"));
    try std.testing.expectEqual(@as(u16, 65535), try parseKeepalive("65535"));
}

test "parseKeepalive rejects invalid values" {
    try std.testing.expectError(PeerConfigError.InvalidPort, parseKeepalive("65536"));
    try std.testing.expectError(PeerConfigError.InvalidPort, parseKeepalive("abc"));
}

test "parsePeerConfig returns disabled defaults for disabled peer" {
    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    defer section.deinit(std.heap.page_allocator);

    const peer = try parsePeerConfig("phone", &section);
    try std.testing.expect(!peer.enabled);
}

test "parsePeerConfig returns enabled peer with all fields" {
    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    defer section.deinit(std.heap.page_allocator);

    try section.put(std.heap.page_allocator, "enabled", "true");
    try section.put(std.heap.page_allocator, "address", "10.149.149.10/32");
    try section.put(std.heap.page_allocator, "private_key_file", "/etc/kgb/wireguard/peers/phone.key");
    try section.put(std.heap.page_allocator, "public_key_file", "/etc/kgb/wireguard/peers/phone.pub");
    try section.put(std.heap.page_allocator, "allowed_ips", "10.149.149.10/32");
    try section.put(std.heap.page_allocator, "endpoint", "127.0.0.1:51821");
    try section.put(std.heap.page_allocator, "persistent_keepalive", "25");
    try section.put(std.heap.page_allocator, "client_output_file", "/var/lib/kgb/wireguard/clients/phone.conf");

    const peer = try parsePeerConfig("phone", &section);
    try std.testing.expect(peer.enabled);
    try std.testing.expectEqualStrings("phone", peer.name);
    try std.testing.expectEqualStrings("10.149.149.10/32", peer.address);
    try std.testing.expectEqualStrings("/etc/kgb/wireguard/peers/phone.key", peer.private_key_file);
    try std.testing.expectEqualStrings("/etc/kgb/wireguard/peers/phone.pub", peer.public_key_file);
    try std.testing.expectEqualStrings("10.149.149.10/32", peer.allowed_ips);
    try std.testing.expect(peer.endpoint != null);
    try std.testing.expectEqualStrings("127.0.0.1:51821", peer.endpoint.?);
    try std.testing.expect(peer.persistent_keepalive != null);
    try std.testing.expectEqual(@as(u16, 25), peer.persistent_keepalive.?);
    try std.testing.expectEqualStrings("/var/lib/kgb/wireguard/clients/phone.conf", peer.client_output_file);
}

test "parsePeerConfig requires address" {
    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    defer section.deinit(std.heap.page_allocator);

    try section.put(std.heap.page_allocator, "enabled", "true");
    try section.put(std.heap.page_allocator, "private_key_file", "/path/to/key");
    try section.put(std.heap.page_allocator, "public_key_file", "/path/to/pub");
    try section.put(std.heap.page_allocator, "allowed_ips", "10.0.0.1/32");
    try section.put(std.heap.page_allocator, "client_output_file", "/path/to/output");

    const result = parsePeerConfig("phone", &section);
    try std.testing.expectError(PeerConfigError.MissingKey, result);
}
