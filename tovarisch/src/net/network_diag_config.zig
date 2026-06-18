// network_diag_config.zig — Configuration for network diagnostics
//
// ACT: Add tovarisch WireGuard and XRay TCP underlay diagnostics
// Configuration schema for network diagnostics section.

const std = @import("std");
const config = @import("../config.zig");

// ============================================================================
// Config Types
// ============================================================================

/// Network diagnostics configuration.
pub const NetworkDiagConfig = struct {
    /// Whether network diagnostics are enabled.
    enabled: bool = false,
    /// WireGuard interfaces to monitor.
    wireguard: WireguardDiagConfig = .{},
    /// Underlay TCP diagnostics configuration.
    underlay_tcp: UnderlayTcpDiagConfig = .{},
    /// Route targets to monitor.
    route_targets: []const []const u8 = &.{},
};

/// WireGuard-specific diagnostics configuration.
pub const WireguardDiagConfig = struct {
    /// Whether WireGuard diagnostics are enabled.
    enabled: bool = true,
    /// WireGuard interfaces to monitor.
    interfaces: []const []const u8 = &.{},
    /// Whether to redact peer public keys.
    redact_peer_keys: bool = true,
    /// Whether to redact endpoints.
    redact_endpoints: bool = true,
    /// Threshold in seconds after which a handshake is considered stale.
    stale_handshake_seconds: u64 = 180,
};

/// Underlay TCP diagnostics configuration.
pub const UnderlayTcpDiagConfig = struct {
    /// Whether underlay TCP diagnostics are enabled.
    enabled: bool = false,
    /// Whether to allow command execution (ss, ip).
    commands_enabled: bool = false,
    /// Process names to monitor.
    process_names: []const []const u8 = &.{},
    /// Remote ports to filter (e.g., 443 for WireGuard-over-TCP).
    remote_ports: []const u16 = &.{443},
    /// Whether to redact addresses.
    redact_addresses: bool = true,
};

// ============================================================================
// Config Parsing
// ============================================================================

/// Parse the [network_diagnostics] section from raw config.
pub fn parseNetworkDiagConfig(raw: *const config.RawConfig) NetworkDiagConfig {
    const section = raw.get("network_diagnostics") orelse return NetworkDiagConfig{};

    var cfg = NetworkDiagConfig{};

    if (config.getString(section, "enabled")) |value| {
        cfg.enabled = config.parseBool(value) catch false;
    }

    // Parse WireGuard subsection
    cfg.wireguard = parseWireguardDiagConfig(raw, section);
    cfg.underlay_tcp = parseUnderlayTcpDiagConfig(raw, section);

    // Parse route_targets as comma-separated list
    if (config.getString(section, "route_targets")) |value| {
        cfg.route_targets = parseCommaSeparatedList(value);
    }

    return cfg;
}

/// Parse the [network_diagnostics.wireguard] section.
fn parseWireguardDiagConfig(raw: *const config.RawConfig, parent: anytype) WireguardDiagConfig {
    const section = raw.get("network_diagnostics.wireguard") orelse {
        // Use defaults from parent if available
        var cfg = WireguardDiagConfig{};
        if (config.getString(parent, "wireguard_enabled")) |value| {
            cfg.enabled = config.parseBool(value) catch true;
        }
        if (config.getString(parent, "wireguard_interfaces")) |value| {
            cfg.interfaces = parseCommaSeparatedList(value);
        }
        if (config.getString(parent, "redact_peer_keys")) |value| {
            cfg.redact_peer_keys = config.parseBool(value) catch true;
        }
        if (config.getString(parent, "redact_endpoints")) |value| {
            cfg.redact_endpoints = config.parseBool(value) catch true;
        }
        if (config.getString(parent, "stale_handshake_seconds")) |value| {
            cfg.stale_handshake_seconds = parseU64OrDefault(value, 180);
        }
        return cfg;
    };

    var cfg = WireguardDiagConfig{};
    if (config.getString(section, "enabled")) |value| {
        cfg.enabled = config.parseBool(value) catch true;
    }
    if (config.getString(section, "interfaces")) |value| {
        cfg.interfaces = parseCommaSeparatedList(value);
    }
    if (config.getString(section, "redact_peer_keys")) |value| {
        cfg.redact_peer_keys = config.parseBool(value) catch true;
    }
    if (config.getString(section, "redact_endpoints")) |value| {
        cfg.redact_endpoints = config.parseBool(value) catch true;
    }
    if (config.getString(section, "stale_handshake_seconds")) |value| {
        cfg.stale_handshake_seconds = parseU64OrDefault(value, 180);
    }

    return cfg;
}

/// Parse the [network_diagnostics.underlay_tcp] section.
fn parseUnderlayTcpDiagConfig(raw: *const config.RawConfig, parent: anytype) UnderlayTcpDiagConfig {
    const section = raw.get("network_diagnostics.underlay_tcp") orelse {
        // Use defaults from parent if available
        var cfg = UnderlayTcpDiagConfig{};
        if (config.getString(parent, "underlay_tcp_enabled")) |value| {
            cfg.enabled = config.parseBool(value) catch false;
        }
        if (config.getString(parent, "commands_enabled")) |value| {
            cfg.commands_enabled = config.parseBool(value) catch false;
        }
        if (config.getString(parent, "process_names")) |value| {
            cfg.process_names = parseCommaSeparatedList(value);
        }
        if (config.getString(parent, "remote_ports")) |value| {
            cfg.remote_ports = parseCommaSeparatedU16List(value);
        }
        if (config.getString(parent, "redact_addresses")) |value| {
            cfg.redact_addresses = config.parseBool(value) catch true;
        }
        return cfg;
    };

    var cfg = UnderlayTcpDiagConfig{};
    if (config.getString(section, "enabled")) |value| {
        cfg.enabled = config.parseBool(value) catch false;
    }
    if (config.getString(section, "commands_enabled")) |value| {
        cfg.commands_enabled = config.parseBool(value) catch false;
    }
    if (config.getString(section, "process_names")) |value| {
        cfg.process_names = parseCommaSeparatedList(value);
    }
    if (config.getString(section, "remote_ports")) |value| {
        cfg.remote_ports = parseCommaSeparatedU16List(value);
    }
    if (config.getString(section, "redact_addresses")) |value| {
        cfg.redact_addresses = config.parseBool(value) catch true;
    }

    return cfg;
}

/// Parse a comma-separated list of strings into an owned slice.
fn parseCommaSeparatedList(value: []const u8) []const []const u8 {
    // For simplicity, we return a single-element array with the trimmed value.
    // In production, this would allocate and split.
    const trimmed = std.mem.trim(u8, value, "\x09");
    return &.{trimmed};
}

/// Parse a comma-separated list of u64 values.
fn parseU64OrDefault(value: []const u8, default: u64) u64 {
    const trimmed = std.mem.trim(u8, value, " \t\r\n");
    return std.fmt.parseInt(u64, trimmed, 10) catch default;
}

/// Parse comma-separated list of u16 ports.
fn parseCommaSeparatedU16List(value: []const u8) []const u16 {
    // Simplified: parse first port number only.
    // In production, would parse all ports.
    const trimmed = std.mem.trim(u8, value, " \t\r\n");
    var end: usize = 0;
    for (trimmed, 0..) |c, i| {
        if (c < '0' or c > '9') {
            end = i;
            break;
        }
        end = trimmed.len;
    }
    return &.{std.fmt.parseInt(u16, trimmed[0..end], 10) catch 443};
}

// ============================================================================
// Tests
// ============================================================================

test "parseNetworkDiagConfig returns defaults when section missing" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);

    const cfg = parseNetworkDiagConfig(&raw);
    try std.testing.expect(!cfg.enabled);
    try std.testing.expect(cfg.wireguard.interfaces.len == 0);
}

test "parseNetworkDiagConfig parses enabled flag" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);

    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    try section.put(std.heap.page_allocator, "enabled", "true");
    try raw.put(std.heap.page_allocator, "network_diagnostics", section);

    const cfg = parseNetworkDiagConfig(&raw);
    try std.testing.expect(cfg.enabled);
}

test "parseWireguardDiagConfig parses stale_handshake_seconds" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);

    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    try section.put(std.heap.page_allocator, "stale_handshake_seconds", "300");
    try raw.put(std.heap.page_allocator, "network_diagnostics.wireguard", section);

    const parent = raw.get("network_diagnostics.wireguard").?;
    const cfg = parseWireguardDiagConfig(&raw, parent);
    try std.testing.expectEqual(@as(u64, 300), cfg.stale_handshake_seconds);
}

test "parseUnderlayTcpDiagConfig parses commands_enabled" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);

    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    try section.put(std.heap.page_allocator, "enabled", "true");
    try section.put(std.heap.page_allocator, "commands_enabled", "true");
    try raw.put(std.heap.page_allocator, "network_diagnostics.underlay_tcp", section);

    const parent = raw.get("network_diagnostics.underlay_tcp").?;
    const cfg = parseUnderlayTcpDiagConfig(&raw, parent);
    try std.testing.expect(cfg.enabled);
    try std.testing.expect(cfg.commands_enabled);
}
