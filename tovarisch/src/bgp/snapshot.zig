// snapshot.zig — BGP snapshot budget types and state enums for tovarisch
//
// ACT-TOVARISCH-ZIG-HULK14: Harden BGP/BFD allocation and state boundaries.
//
// This module provides bounded, typed contracts for BGP runtime snapshots:
// - Budget limits prevent unbounded memory growth
// - Closed state enums ensure exhaustive handling
// - Structured outcomes classify collection results

const std = @import("std");

/// BGP snapshot budget limits.
///
/// These budgets constrain the size of BGP diagnostic data to prevent
/// unbounded memory growth. All collection logic must respect these limits.
pub const BgpSnapshotBudget = struct {
    /// Maximum number of BGP peers to include in snapshot.
    /// Typical deployments have 1-3 peers; limit prevents runaway enumeration.
    max_peers: usize = 8,

    /// Maximum number of routes/prefixes to include in snapshot.
    /// Even large BGP routers rarely have more than a few thousand paths.
    max_routes: usize = 16384,

    /// Maximum AS_PATH length (number of ASNs in path).
    /// RFC 4271 limits to 255, but we use a smaller budget for sanity.
    max_as_path_len: usize = 64,

    /// Maximum bytes for any single string field (peer name, error, etc.).
    max_string_bytes: usize = 256,

    /// Maximum total bytes for the entire BGP snapshot.
    /// Prevents single snapshots from consuming excessive memory.
    max_total_bytes: usize = 65536,
};

/// BGP peer state enumeration.
///
/// This is a CLOSED enum - all valid states are listed. Unknown states
/// from external sources (daemons, wire) are mapped to `.unknown`.
pub const BgpPeerState = enum {
    /// Idle: Initial state, no connection.
    idle,
    /// Connect: TCP connection in progress.
    connect,
    /// Active: Listening for incoming connection.
    active,
    /// OpenSent: OPEN sent, waiting for peer's OPEN.
    open_sent,
    /// OpenConfirm: OPEN received, waiting for KEEPALIVE.
    open_confirm,
    /// Established: Connection active, can send UPDATE.
    established,
    /// Unknown state from external source (daemon, wire).
    /// This ensures exhaustive enum coverage for untrusted input.
    unknown,
};

/// Map a string FSM state to BgpPeerState.
///
/// Handles common FSM state strings from BGP implementations
/// (BIRD, Quagga, FRR, GoBGP, etc.). Unknown strings map to `.unknown`.
pub fn parseBgpPeerState(str: []const u8) BgpPeerState {
    if (std.mem.eql(u8, str, "Idle") or std.mem.eql(u8, str, "idle") or std.mem.eql(u8, str, "IDLE")) {
        return .idle;
    }
    if (std.mem.eql(u8, str, "Connect") or std.mem.eql(u8, str, "connect") or std.mem.eql(u8, str, "CONNECT")) {
        return .connect;
    }
    if (std.mem.eql(u8, str, "Active") or std.mem.eql(u8, str, "active") or std.mem.eql(u8, str, "ACTIVE")) {
        return .active;
    }
    if (std.mem.eql(u8, str, "OpenSent") or std.mem.eql(u8, str, "open_sent") or std.mem.eql(u8, str, "OpenSent") or std.mem.eql(u8, str, "OPENSENT")) {
        return .open_sent;
    }
    if (std.mem.eql(u8, str, "OpenConfirm") or std.mem.eql(u8, str, "open_confirm") or std.mem.eql(u8, str, "OpenConfirm") or std.mem.eql(u8, str, "OPENCONFIRM")) {
        return .open_confirm;
    }
    if (std.mem.eql(u8, str, "Established") or std.mem.eql(u8, str, "established") or std.mem.eql(u8, str, "ESTABLISHED")) {
        return .established;
    }
    return .unknown;
}

/// BGP snapshot collection result.
///
/// Structured outcomes prevent panics on malformed input and ensure
/// all collection paths are handled explicitly.
pub const RoutingSnapshotResult = union(enum) {
    /// Data is available and fresh.
    available,
    /// Collection failed - daemon unavailable or command failed.
    unavailable,
    /// Data is stale (collected but exceeded age threshold).
    stale,
    /// Data was truncated to fit budget limits.
    truncated,
    /// Input was malformed (unparseable daemon output).
    malformed,
    /// Collection not supported on this platform/configuration.
    unsupported,
};

/// Bounded BGP peer snapshot.
///
/// This struct constrains all string fields to explicit byte limits
/// and uses fixed-size arrays where possible.
pub const BgpPeerSnapshot = struct {
    /// Peer address bytes (IPv4) or truncated string.
    /// Bounded to budget.max_string_bytes.
    peer_address: []const u8,
    /// Peer's AS number (supports 4-byte ASNs per RFC 6793).
    peer_as: u32,
    /// Our local AS number (supports 4-byte ASNs per RFC 6793).
    local_as: u32,
    /// Current FSM state.
    state: BgpPeerState,
    /// Established duration in seconds (0 if not established).
    uptime_seconds: u64,
    /// Last error message (null if no error).
    last_error: ?[]const u8,
    /// Messages received count.
    messages_received: u64,
    /// Messages sent count.
    messages_sent: u64,
    /// Prefixes received count.
    prefixes_received: u64,
    /// Prefixes sent count.
    prefixes_sent: u64,
};

/// Bounded BGP snapshot for status reporting.
///
/// All fields are bounded by BgpSnapshotBudget. String fields
/// are either static or caller-owned with explicit limits.
pub const BgpSnapshot = struct {
    /// Collection outcome classification.
    result: RoutingSnapshotResult,
    /// Peer snapshots (bounded by budget.max_peers).
    peers: []const BgpPeerSnapshot,
    /// Total peer count (may exceed len(peers) if truncated).
    total_peer_count: usize,
    /// Total route count across all peers.
    total_route_count: usize,
    /// Collection timestamp (monotonic ms).
    collected_at_ms: u64,
    /// Age of data in seconds (0 if just collected).
    age_seconds: u64,
    /// Whether data was truncated due to budget limits.
    was_truncated: bool,
};

/// Truncate a string to fit within byte budget.
///
/// Returns a slice of the input that fits within max_len bytes.
/// If input is already within budget, returns the full input.
pub fn truncateString(input: []const u8, max_len: usize) []const u8 {
    if (input.len <= max_len) return input;
    return input[0..max_len];
}

/// Check if a budget allows more items.
///
/// Returns true if current_count < budget_limit.
pub fn hasCapacity(current_count: usize, budget_limit: usize) bool {
    return current_count < budget_limit;
}

/// Calculate remaining capacity within a byte budget.
///
/// Returns the number of additional bytes that can fit.
pub fn remainingBytes(used: usize, budget: usize) usize {
    if (used >= budget) return 0;
    return budget - used;
}

// ============================================================================
// Tests
// ============================================================================

test "BgpPeerState enum has all expected variants" {
    try std.testing.expectEqual(@as(usize, 7), @typeInfo(BgpPeerState).@"enum".fields.len);
}

test "parseBgpPeerState handles idle variants" {
    try std.testing.expect(.idle == parseBgpPeerState("idle"));
    try std.testing.expect(.idle == parseBgpPeerState("Idle"));
    try std.testing.expect(.idle == parseBgpPeerState("IDLE"));
}

test "parseBgpPeerState handles connect variants" {
    try std.testing.expect(.connect == parseBgpPeerState("connect"));
    try std.testing.expect(.connect == parseBgpPeerState("Connect"));
}

test "parseBgpPeerState handles active variants" {
    try std.testing.expect(.active == parseBgpPeerState("active"));
    try std.testing.expect(.active == parseBgpPeerState("Active"));
}

test "parseBgpPeerState handles open_sent variants" {
    try std.testing.expect(.open_sent == parseBgpPeerState("open_sent"));
    try std.testing.expect(.open_sent == parseBgpPeerState("OpenSent"));
}

test "parseBgpPeerState handles open_confirm variants" {
    try std.testing.expect(.open_confirm == parseBgpPeerState("open_confirm"));
    try std.testing.expect(.open_confirm == parseBgpPeerState("OpenConfirm"));
}

test "parseBgpPeerState handles established variants" {
    try std.testing.expect(.established == parseBgpPeerState("established"));
    try std.testing.expect(.established == parseBgpPeerState("Established"));
}

test "parseBgpPeerState maps unknown strings to .unknown" {
    try std.testing.expect(.unknown == parseBgpPeerState("invalid"));
    try std.testing.expect(.unknown == parseBgpPeerState(""));
    try std.testing.expect(.unknown == parseBgpPeerState("UNKNOWN_STATE"));
    try std.testing.expect(.unknown == parseBgpPeerState("random"));
}

test "RoutingSnapshotResult union has all expected variants" {
    try std.testing.expectEqual(@as(usize, 6), @typeInfo(RoutingSnapshotResult).@"union".fields.len);
}

test "truncateString returns full input when within budget" {
    const input = "short";
    const result = truncateString(input, 10);
    try std.testing.expectEqualSlices(u8, "short", result);
}

test "truncateString truncates when exceeding budget" {
    const input = "this is a long string";
    const result = truncateString(input, 10);
    try std.testing.expectEqual(@as(usize, 10), result.len);
    try std.testing.expectEqualSlices(u8, "this is a ", result);
}

test "truncateString handles empty input" {
    const input: []const u8 = "";
    const result = truncateString(input, 10);
    try std.testing.expectEqualSlices(u8, "", result);
}

test "truncateString handles exact budget match" {
    const input = "exactly10!";
    const result = truncateString(input, 10);
    try std.testing.expectEqualSlices(u8, "exactly10!", result);
}

test "hasCapacity returns true when under limit" {
    try std.testing.expect(hasCapacity(0, 8));
    try std.testing.expect(hasCapacity(5, 8));
    try std.testing.expect(hasCapacity(7, 8));
}

test "hasCapacity returns false when at or over limit" {
    try std.testing.expect(!hasCapacity(8, 8));
    try std.testing.expect(!hasCapacity(9, 8));
    try std.testing.expect(!hasCapacity(100, 8));
}

test "remainingBytes calculates correctly" {
    try std.testing.expectEqual(@as(usize, 10), remainingBytes(0, 10));
    try std.testing.expectEqual(@as(usize, 5), remainingBytes(5, 10));
    try std.testing.expectEqual(@as(usize, 0), remainingBytes(10, 10));
    try std.testing.expectEqual(@as(usize, 0), remainingBytes(15, 10));
}

test "BgpSnapshotBudget default values are sane" {
    const budget = BgpSnapshotBudget{};
    try std.testing.expect(budget.max_peers >= 1);
    try std.testing.expect(budget.max_string_bytes >= 64);
    try std.testing.expect(budget.max_total_bytes >= 1024);
}
