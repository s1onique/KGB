// wg_show_parser.zig — Parser for WireGuard `wg show` output
//
// ACT: Parser-only support for WireGuard diagnostics.
//
// Scope:
// - Parse `wg show` output to extract:
//   - interface name
//   - peer count
//   - latest handshake age in seconds (null if unavailable)
//   - aggregate rx_bytes
//   - aggregate tx_bytes
//
// Explicitly ignored (discarded):
// - public keys
// - private keys
// - preshared keys
// - endpoints
// - allowed IPs
//
// Non-goals:
// - No process spawning (future ACT)
// - No status contract changes (future ACT)

const std = @import("std");

// ============================================================================
// Data Structures
// ============================================================================

/// Parsed WireGuard interface diagnostics.
/// Only includes privacy-aligned, aggregate fields.
pub const WgInterface = struct {
    /// WireGuard interface name (e.g., "wg0").
    interface: []const u8,
    /// Number of configured peers.
    peer_count: u32,
    /// Seconds since most recent handshake across all peers.
    /// Null if no handshakes have occurred (e.g., never-handshaked peer).
    latest_handshake_age_sec: ?u64,
    /// Total bytes received via this interface.
    rx_bytes: u64,
    /// Total bytes transmitted via this interface.
    tx_bytes: u64,
};

/// Parser errors.
pub const ParseError = error{
    /// Input is empty or contains no interface section.
    NoInterface,
    /// Failed to parse a numeric field.
    InvalidNumber,
    /// Output is truncated or malformed.
    MalformedOutput,
};

// ============================================================================
// Parser
// ============================================================================

/// Parses `wg show` output and extracts WireGuard diagnostics.
///
/// Returns parsed interface data including:
/// - interface name
/// - peer count
/// - latest handshake age (null if unavailable)
/// - aggregate transfer bytes
///
/// Fields that are parsed but discarded:
/// - public/private keys
/// - endpoints
/// - allowed IPs
/// - preshared keys
///
/// Input format expected (real `wg show` human-readable output):
///
///   interface: wg0
///     public key: xxxxx
///     private key: (hidden)
///     listening port: 51820
///   peer: xxxxx
///     endpoint: 1.2.3.4:51820
///     allowed ips: 10.0.0.2/32
///     latest handshake: 123 seconds ago
///     transfer: 1000 bytes received, 2000 bytes sent
///
/// The parser walks line-by-line, extracting only the fields we care about.
/// Lines containing ignored fields are skipped.
pub fn parseWgShowOutput(input: []const u8) ParseError!WgInterface {
    var it = std.mem.splitScalar(u8, input, '\n');

    // First line must contain the interface section
    const first_line = it.next() orelse return error.NoInterface;

    // Parse interface name from "interface: wg0" format
    const iface = parseInterfaceHeader(first_line) orelse return error.NoInterface;

    // Initialize aggregate counters
    var peer_count: u32 = 0;
    var latest_handshake: ?u64 = null;
    var rx_bytes: u64 = 0;
    var tx_bytes: u64 = 0;

    // Process remaining lines
    while (it.next()) |line| {
        const trimmed = std.mem.trim(u8, line, " \t");

        // Count peer lines to determine peer count
        if (std.mem.startsWith(u8, trimmed, "peer:")) {
            peer_count += 1;
            continue;
        }

        // Check for handshake age
        if (parseHandshakeAge(trimmed)) |age| {
            // Keep the latest (most recent) handshake
            if (latest_handshake == null or age < latest_handshake.?) {
                latest_handshake = age;
            }
            continue;
        }

        // Check for transfer bytes
        if (parseTransferBytes(trimmed)) |transfer| {
            rx_bytes += transfer.rx;
            tx_bytes += transfer.tx;
            continue;
        }

        // All other lines (public key, endpoint, allowed ips, etc.) are ignored
    }

    return WgInterface{
        .interface = iface,
        .peer_count = peer_count,
        .latest_handshake_age_sec = latest_handshake,
        .rx_bytes = rx_bytes,
        .tx_bytes = tx_bytes,
    };
}

/// Parses the interface header line to extract the interface name.
/// Handles: "interface: wg0" format from real `wg show` output.
pub fn parseInterfaceHeader(line: []const u8) ?[]const u8 {
    // Handle "interface: <name>" format
    const prefix = "interface:";
    if (std.mem.startsWith(u8, line, prefix)) {
        const after_prefix = line[prefix.len..];
        return std.mem.trim(u8, after_prefix, " \t");
    }

    // Fallback: find colon or space to terminate interface name
    var end: usize = 0;
    for (line, 0..) |c, i| {
        if (c == ':' or c == ' ') {
            end = i;
            break;
        }
    }

    if (end == 0) return null;
    return std.mem.trim(u8, line[0..end], " \t");
}

/// Parses handshake age from a line like "latest handshake: 123 seconds ago"
/// Returns the age in seconds, or null if the line doesn't contain handshake info.
pub fn parseHandshakeAge(line: []const u8) ?u64 {
    // Look for "latest handshake:" prefix
    const prefix = "latest handshake:";
    if (!std.mem.startsWith(u8, line, prefix)) return null;

    const after_prefix = line[prefix.len..];
    const trimmed = std.mem.trim(u8, after_prefix, " \t");

    // Parse the number before "seconds"
    var num_str: []const u8 = undefined;
    for (trimmed, 0..) |c, i| {
        if (c < '0' or c > '9') {
            num_str = trimmed[0..i];
            break;
        }
        if (i == trimmed.len - 1) {
            num_str = trimmed;
            break;
        }
    }

    if (num_str.len == 0) return null;

    // Parse u64
    var result: u64 = 0;
    for (num_str) |c| {
        if (c < '0' or c > '9') return null;
        result = result * 10 + @as(u64, c - '0');
    }

    return result;
}

/// Parses transfer bytes from a line like "transfer: 1000 bytes received, 2000 bytes sent"
/// Returns rx and tx byte counts.
pub fn parseTransferBytes(line: []const u8) ?struct { rx: u64, tx: u64 } {
    // Look for "transfer:" prefix
    const prefix = "transfer:";
    if (!std.mem.startsWith(u8, line, prefix)) return null;

    const after_prefix = line[prefix.len..];
    const trimmed = std.mem.trim(u8, after_prefix, " \t");

    // Parse "N bytes received" and "M bytes sent"
    var rx_bytes: ?u64 = null;
    var tx_bytes: ?u64 = null;

    // Split by comma for the two parts
    var parts = std.mem.splitSequence(u8, trimmed, ",");
    while (parts.next()) |part| {
        const part_trimmed = std.mem.trim(u8, part, " \t");

        // Check if this part contains "bytes received"
        if (std.mem.containsAtLeast(u8, part_trimmed, 1, "bytes received")) {
            rx_bytes = parseBytesValue(part_trimmed);
        }
        // Check if this part contains "bytes sent"
        if (std.mem.containsAtLeast(u8, part_trimmed, 1, "bytes sent")) {
            tx_bytes = parseBytesValue(part_trimmed);
        }
    }

    if (rx_bytes == null and tx_bytes == null) return null;

    return .{
        .rx = rx_bytes orelse 0,
        .tx = tx_bytes orelse 0,
    };
}

/// Parses the byte count value from a string like "1000 bytes received"
pub fn parseBytesValue(line: []const u8) u64 {
    // Find the first digit
    var num_start: ?usize = null;
    for (line, 0..) |c, i| {
        if (c >= '0' and c <= '9') {
            num_start = i;
            break;
        }
    }

    if (num_start == null) return 0;

    // Parse the number
    var result: u64 = 0;
    var has_digit = false;
    for (line[num_start.?..]) |c| {
        if (c >= '0' and c <= '9') {
            result = result * 10 + @as(u64, c - '0');
            has_digit = true;
        } else if (has_digit) {
            // Stop at first non-digit after reading at least one digit
            break;
        }
    }

    return result;
}
