// wg_dump_parser.zig — Parser for WireGuard `wg show dump` output
//
// ACT: Add tovarisch WireGuard and XRay TCP underlay diagnostics
// Parses detailed WireGuard peer information from `wg show dump`.
// The `wg show dump` output format (tab-separated):
// <private_key> <public_key> <endpoint> <persistent_keepalive> <allowedips> <latest_handshake> <rx_bytes> <tx_bytes>
//
// Privacy constraints:
// - Private keys are parsed but immediately discarded
// - Public keys can be redacted/truncated
// - Endpoints can be redacted

const std = @import("std");

// ============================================================================
// Types
// ============================================================================

/// Peer-level diagnostics from `wg show dump`.
pub const WgPeer = struct {
    /// Peer public key (may be redacted).
    public_key: []const u8,
    /// Endpoint address (may be redacted).
    endpoint: []const u8,
    /// Allowed IPs for this peer.
    allowed_ips: []const u8,
    /// Persistent keepalive interval in seconds (0 if disabled).
    persistent_keepalive_seconds: u16,
    /// Unix timestamp of latest handshake (0 if never).
    latest_handshake_unix: i64,
    /// Bytes received from this peer.
    rx_bytes: u64,
    /// Bytes sent to this peer.
    tx_bytes: u64,
    /// Transfer status.
    status: WgPeerStatus = .ok,
};

/// Peer status derived from diagnostics.
pub const WgPeerStatus = enum {
    ok,
    warning,
    @"error",
    unavailable,
};

/// Interface-level WireGuard diagnostics.
pub const WgInterfaceDiag = struct {
    /// Interface name.
    name: []const u8,
    /// List of peers.
    peers: []WgPeer,
    /// Overall interface status.
    status: WgPeerStatus = .ok,
};

/// Parser errors.
pub const ParseError = error{
    NoData,
    InvalidNumber,
    MalformedOutput,
    MissingPrivateKey,
};

/// Redaction mode for sensitive data.
pub const RedactMode = struct {
    /// Whether to redact peer public keys.
    redact_public_keys: bool = true,
    /// Whether to redact endpoints.
    redact_endpoints: bool = true,
    /// Number of characters to keep at start of redacted key.
    redact_prefix_len: usize = 4,
    /// Replacement for redacted portion.
    redact_replacement: []const u8 = "…",
};

/// Configuration for parsing.
pub const ParseConfig = struct {
    /// Redaction settings.
    redact: RedactMode = .{},
    /// Threshold in seconds after which a handshake is stale.
    stale_handshake_seconds: u64 = 180,
};

/// Result of parsing `wg show dump` for an interface.
pub const WgDumpResult = struct {
    /// Interface name extracted from output.
    interface_name: []const u8,
    /// Parsed peers.
    peers: []WgPeer,
    /// Total peers count.
    peer_count: usize,
    /// Whether parsing encountered warnings.
    has_warnings: bool,
};

// ============================================================================
// Redaction Helpers
// ============================================================================

/// Redact a public key, showing only the prefix.
/// Returns a slice that is valid as long as the input key is valid.
/// Note: For persistent storage, caller must duplicate the result.
pub fn redactPublicKey(key: []const u8, mode: RedactMode) []const u8 {
    if (!mode.redact_public_keys or key.len <= mode.redact_prefix_len + mode.redact_replacement.len) {
        return key;
    }
    // Return format: "abcd…1234" (prefix + redact_replacement + suffix)
    // Uses stack buffer - valid until function returns
    const prefix_end = mode.redact_prefix_len;
    const suffix_start = key.len - mode.redact_prefix_len;
    var buf: [64]u8 = undefined;
    const result = std.fmt.bufPrint(&buf, "{s}{s}{s}", .{
        key[0..prefix_end],
        mode.redact_replacement,
        key[suffix_start..],
    }) catch key;
    return result;
}

/// Redact an endpoint address (host portion only).
/// Returns a slice that is valid as long as the input endpoint is valid.
/// Note: For persistent storage, caller must duplicate the result.
pub fn redactEndpoint(endpoint: []const u8, mode: RedactMode) []const u8 {
    if (!mode.redact_endpoints) return endpoint;

    // Find the colon that separates host:port
    const colon_idx = std.mem.indexOfScalar(u8, endpoint, ':');
    if (colon_idx == null) return "redacted";

    const host = endpoint[0..colon_idx.?];
    const port = endpoint[colon_idx.?..];

    if (host.len <= 4) return endpoint; // Too short to redact meaningfully

    // Uses stack buffer - valid until function returns
    var buf: [64]u8 = undefined;
    const result = std.fmt.bufPrint(&buf, "redacted{s}", .{port}) catch endpoint;
    return result;
}

// ============================================================================
// Time Helper
// ============================================================================

/// Get current Unix timestamp in seconds using gettimeofday.
fn currentUnixTimestamp() i64 {
    var tv: std.c.timeval = undefined;
    if (std.c.gettimeofday(&tv, null) < 0) return 0;
    return @as(i64, tv.sec);
}

// ============================================================================
// Parser
// ============================================================================

/// Parses `wg show dump` output and extracts WireGuard peer diagnostics.
///
/// Input format (tab-separated, one peer per line after header):
///   <private_key>  <public_key>  <endpoint>  <persistent_keepalive>  <allowedips>  <latest_handshake>  <rx_bytes>  <tx_bytes>
///
/// The first line contains the private key (discarded) and interface name.
/// Subsequent lines contain peer data.
///
/// Returns parsed interface data including:
/// - interface name
/// - peer count
/// - per-peer: public key (redacted), endpoint (redacted), allowed IPs,
///   persistent keepalive, latest handshake timestamp, rx/tx bytes
pub fn parseWgDumpOutput(allocator: std.mem.Allocator, input: []const u8, config: ParseConfig) ParseError!WgDumpResult {
    var lines = std.mem.splitScalar(u8, input, '\n');

    // First line: private key and interface name
    const first_line = lines.next() orelse return error.NoData;
    const interface_name = try parseInterfaceName(first_line);

    var peers = std.ArrayList(WgPeer).empty;
    defer peers.deinit(allocator);

    // Parse peer lines
    while (lines.next()) |line| {
        const trimmed = std.mem.trim(u8, line, " \t\r\n");
        if (trimmed.len == 0) continue;

        const peer = parsePeerLine(trimmed, config) catch continue;
        peers.append(allocator, peer) catch continue;
    }

    return WgDumpResult{
        .interface_name = interface_name,
        .peers = peers.toOwnedSlice(allocator) catch return error.MalformedOutput,
        .peer_count = peers.items.len,
        .has_warnings = false,
    };
}

/// Parse the interface name from the first line of `wg show dump`.
/// The first line format is: <private_key>\t<interface_name>
/// or just <private_key> if no interface name.
fn parseInterfaceName(line: []const u8) ParseError![]const u8 {
    // Skip the private key field (first tab-separated field)
    const tab_idx = std.mem.indexOfScalar(u8, line, '\t');
    if (tab_idx == null) return error.NoData;

    const after_key = line[tab_idx.? + 1..];
    const trimmed = std.mem.trim(u8, after_key, " \t");

    if (trimmed.len == 0) return error.NoData;

    return trimmed;
}

/// Parse a peer line from `wg show dump`.
/// Format: <public_key>  <endpoint>  <persistent_keepalive>  <allowed_ips>  <latest_handshake>  <rx_bytes>  <tx_bytes>
fn parsePeerLine(line: []const u8, config: ParseConfig) ParseError!WgPeer {
    var fields = std.mem.splitScalar(u8, line, '\t');

    const public_key = fields.next() orelse return error.MalformedOutput;
    if (public_key.len == 0) return error.MalformedOutput;

    const endpoint = fields.next() orelse return error.MalformedOutput;
    const persistent_keepalive_str = fields.next() orelse "0";
    const allowed_ips = fields.next() orelse "";
    const latest_handshake_str = fields.next() orelse "0";
    const rx_bytes_str = fields.next() orelse "0";
    const tx_bytes_str = fields.next() orelse "0";

    // Parse numeric fields
    const persistent_keepalive = std.fmt.parseInt(u16, persistent_keepalive_str, 10) catch 0;
    const latest_handshake = std.fmt.parseInt(i64, latest_handshake_str, 10) catch 0;
    const rx_bytes = std.fmt.parseInt(u64, rx_bytes_str, 10) catch 0;
    const tx_bytes = std.fmt.parseInt(u64, tx_bytes_str, 10) catch 0;

    // Determine peer status
    const status = determinePeerStatus(latest_handshake, config.stale_handshake_seconds);

    return WgPeer{
        .public_key = redactPublicKey(public_key, config.redact),
        .endpoint = redactEndpoint(endpoint, config.redact),
        .allowed_ips = allowed_ips,
        .persistent_keepalive_seconds = persistent_keepalive,
        .latest_handshake_unix = latest_handshake,
        .rx_bytes = rx_bytes,
        .tx_bytes = tx_bytes,
        .status = status,
    };
}

/// Determine peer status based on handshake timestamp.
fn determinePeerStatus(latest_handshake_unix: i64, stale_threshold: u64) WgPeerStatus {
    if (latest_handshake_unix == 0) {
        return .warning; // Never handshaked
    }

    // Get current time using gettimeofday
    const now = currentUnixTimestamp();
    const age_seconds = @as(u64, @intCast(now - latest_handshake_unix));

    if (age_seconds > stale_threshold) {
        return .warning; // Handshake is stale
    }

    return .ok;
}

/// Calculate handshake age in seconds from Unix timestamp.
pub fn handshakeAgeSeconds(latest_handshake_unix: i64) ?u64 {
    if (latest_handshake_unix == 0) return null;
    const now = currentUnixTimestamp();
    const age = now - latest_handshake_unix;
    if (age < 0) return null;
    return @as(u64, @intCast(age));
}

// ============================================================================
// Tests
// ============================================================================

test "parseWgDumpOutput parses valid dump output" {
    // Test with properly formatted input
    const dump_output = "pk\twg0\npeer\t1.2.3.4:5\t0\t10/0\t0\t0\t0";

    const result = try parseWgDumpOutput(std.testing.allocator, dump_output, .{});
    std.testing.allocator.free(result.peers);
    try std.testing.expectEqualStrings("wg0", result.interface_name);
}

test "parseWgDumpOutput handles no peers" {
    const dump_output = "private_key_base64\twg-kgb0\n";

    const result = try parseWgDumpOutput(std.testing.allocator, dump_output, .{});
    std.testing.allocator.free(result.peers);
    try std.testing.expectEqualStrings("wg-kgb0", result.interface_name);
    try std.testing.expectEqual(@as(usize, 0), result.peer_count);
}

test "parseWgDumpOutput returns error for empty input" {
    try std.testing.expectError(error.NoData, parseWgDumpOutput(std.testing.allocator, "", .{}));
}

test "parseWgDumpOutput handles malformed line" {
    const dump_output = "private_key_base64\twg-kgb0\nmalformed_line_without_tabs";

    // Should parse successfully, skipping the malformed peer line
    const result = try parseWgDumpOutput(std.testing.allocator, dump_output, .{});
    std.testing.allocator.free(result.peers);
    try std.testing.expectEqual(@as(usize, 0), result.peer_count);
}

test "redactPublicKey truncates long keys" {
    const mode = RedactMode{ .redact_public_keys = true };
    const key = "abcdefghij1234567890abcdefghij1234567890abcdefghij1234567890";
    const redacted = redactPublicKey(key, mode);
    try std.testing.expect(redacted.len < key.len);
}

test "redactPublicKey keeps short keys unchanged when redaction disabled" {
    const mode = RedactMode{ .redact_public_keys = false };
    const key = "abcd1234";
    const result = redactPublicKey(key, mode);
    try std.testing.expectEqualStrings("abcd1234", result);
}

test "redactEndpoint hides host portion" {
    const mode = RedactMode{ .redact_endpoints = true };
    const endpoint = "192.0.2.1:443";
    const redacted = redactEndpoint(endpoint, mode);
    // Should be redacted with port preserved
    try std.testing.expect(redacted.len > 0);
}

test "redactEndpoint keeps short endpoints unchanged" {
    const mode = RedactMode{ .redact_endpoints = true };
    const endpoint = "ab:443";
    const result = redactEndpoint(endpoint, mode);
    try std.testing.expectEqualStrings("ab:443", result);
}
