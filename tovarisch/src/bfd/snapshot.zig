// snapshot.zig — BFD snapshot budget types and state enums for tovarisch
//
// ACT-TOVARISCH-ZIG-HULK14: Harden BGP/BFD allocation and state boundaries.
//
// This module provides bounded, typed contracts for BFD runtime snapshots:
// - Budget limits prevent unbounded memory growth
// - Closed state enums ensure exhaustive handling
// - Structured outcomes classify collection results

const std = @import("std");

/// BFD snapshot budget limits.
///
/// These budgets constrain the size of BFD diagnostic data to prevent
/// unbounded memory growth. All collection logic must respect these limits.
pub const BfdSnapshotBudget = struct {
    /// Maximum number of BFD sessions to include in snapshot.
    /// Typical deployments have 1-4 sessions; limit prevents runaway enumeration.
    max_sessions: usize = 16,

    /// Maximum bytes for any single string field (diagnostic, error, etc.).
    max_diag_string_bytes: usize = 128,

    /// Maximum total bytes for the entire BFD snapshot.
    /// Prevents single snapshots from consuming excessive memory.
    max_total_bytes: usize = 16384,
};

/// BFD session state enumeration.
///
/// This is a CLOSED enum - all valid states are listed. Unknown states
/// from external sources (daemons, wire) are mapped to `.unknown`.
pub const BfdState = enum {
    /// AdminDown: Session administratively disabled.
    admin_down,
    /// Down: Session is down.
    down,
    /// Init: Session initialization in progress.
    init,
    /// Up: Session is up and active.
    up,
    /// Unknown state from external source (daemon, wire).
    /// This ensures exhaustive enum coverage for untrusted input.
    unknown,
};

/// Map a u2 wire state to BfdState.
///
/// BFD wire format uses 2-bit state values (RFC 5880 Section 6.8.1).
/// Values outside 0-3 are mapped to `.unknown`.
pub fn parseBfdStateWire(state_val: u2) BfdState {
    return switch (state_val) {
        0 => .admin_down,
        1 => .down,
        2 => .init,
        3 => .up,
    };
}

/// Map a string state to BfdState.
///
/// Handles common state strings from BFD implementations
/// (BIRD, Quagga, FRR, etc.). Unknown strings map to `.unknown`.
pub fn parseBfdStateString(str: []const u8) BfdState {
    if (std.mem.eql(u8, str, "AdminDown") or std.mem.eql(u8, str, "admin_down") or std.mem.eql(u8, str, "AdminDown") or std.mem.eql(u8, str, "ADMINDOWN")) {
        return .admin_down;
    }
    if (std.mem.eql(u8, str, "Down") or std.mem.eql(u8, str, "down") or std.mem.eql(u8, str, "DOWN")) {
        return .down;
    }
    if (std.mem.eql(u8, str, "Init") or std.mem.eql(u8, str, "init") or std.mem.eql(u8, str, "INIT")) {
        return .init;
    }
    if (std.mem.eql(u8, str, "Up") or std.mem.eql(u8, str, "up") or std.mem.eql(u8, str, "UP")) {
        return .up;
    }
    return .unknown;
}

/// BFD collection result.
///
/// Structured outcomes prevent panics on malformed input and ensure
/// all collection paths are handled explicitly.
pub const BfdCollectionResult = union(enum) {
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
    /// BFD not configured on this node.
    not_configured,
};

/// Bounded BFD session snapshot.
///
/// This struct constrains all fields to explicit limits
/// and uses fixed-size arrays where possible.
pub const BfdSessionSnapshot = struct {
    /// Peer address string (bounded to budget.max_diag_string_bytes).
    peer_address: []const u8,
    /// Local discriminator.
    local_discr: u32,
    /// Remote discriminator.
    remote_discr: u32,
    /// Current session state.
    state: BfdState,
    /// Desired transmit interval in milliseconds.
    tx_interval_ms: u32,
    /// Detection multiplier.
    multiplier: u8,
    /// Detection timeout in milliseconds.
    detection_timeout_ms: u32,
    /// Packets sent count.
    packets_sent: u64,
    /// Packets received count.
    packets_received: u64,
    /// Detection timeouts count.
    detection_timeouts: u64,
};

/// Bounded BFD snapshot for status reporting.
///
/// All fields are bounded by BfdSnapshotBudget. String fields
/// are either static or caller-owned with explicit limits.
pub const BfdSnapshot = struct {
    /// Collection outcome classification.
    result: BfdCollectionResult,
    /// Session snapshots (bounded by budget.max_sessions).
    sessions: []const BfdSessionSnapshot,
    /// Total session count (may exceed len(sessions) if truncated).
    total_session_count: usize,
    /// Sessions in Up state.
    up_count: usize,
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

/// Safe BFD timer calculation.
///
/// Computes detection timeout: (required_min_rx_interval_us * multiplier) / 1000.
/// Uses checked arithmetic to prevent overflow. Returns 0 on overflow.
pub fn safeDetectionTimeout(required_min_rx_us: u32, multiplier: u8) u32 {
    // Multiply first, checking for overflow
    const mult_u64 = @as(u64, required_min_rx_us) * @as(u64, multiplier);
    // Check if multiplication would overflow u32 (max u32 = 4,294,967,295)
    if (mult_u64 > 4_294_967_295) {
        return 0; // Overflow protection
    }
    // Convert to ms (divide by 1000)
    return @as(u32, @intCast(mult_u64)) / 1000;
}

/// BFD detail formatting budget.
///
/// Format: "9999/9999 bfd sessions up" = 24 chars max.
/// Additional safety margin for edge cases.
pub const BFD_DETAIL_BUF_SIZE: usize = 64;

// ============================================================================
// Tests
// ============================================================================

test "BfdState enum has all expected variants" {
    try std.testing.expectEqual(@as(usize, 5), @typeInfo(BfdState).@"enum".fields.len);
}

test "parseBfdStateWire handles all valid values" {
    try std.testing.expect(.admin_down == parseBfdStateWire(0));
    try std.testing.expect(.down == parseBfdStateWire(1));
    try std.testing.expect(.init == parseBfdStateWire(2));
    try std.testing.expect(.up == parseBfdStateWire(3));
}

test "parseBfdStateWire handles unknown values" {
    // u2 can only be 0-3, so all possible values are covered above
    // This test documents the explicit coverage
    inline for (.{ 0, 1, 2, 3 }) |v| {
        const s = parseBfdStateWire(@as(u2, @truncate(v)));
        try std.testing.expect(s != .unknown);
    }
}

test "parseBfdStateString handles all valid states" {
    try std.testing.expect(.admin_down == parseBfdStateString("admin_down"));
    try std.testing.expect(.admin_down == parseBfdStateString("AdminDown"));
    try std.testing.expect(.down == parseBfdStateString("down"));
    try std.testing.expect(.down == parseBfdStateString("Down"));
    try std.testing.expect(.init == parseBfdStateString("init"));
    try std.testing.expect(.init == parseBfdStateString("Init"));
    try std.testing.expect(.up == parseBfdStateString("up"));
    try std.testing.expect(.up == parseBfdStateString("Up"));
}

test "parseBfdStateString maps unknown strings to .unknown" {
    try std.testing.expect(.unknown == parseBfdStateString("invalid"));
    try std.testing.expect(.unknown == parseBfdStateString(""));
    try std.testing.expect(.unknown == parseBfdStateString("UNKNOWN"));
    try std.testing.expect(.unknown == parseBfdStateString("foobar"));
}

test "BfdCollectionResult union has all expected variants" {
    try std.testing.expectEqual(@as(usize, 6), @typeInfo(BfdCollectionResult).@"union".fields.len);
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
    try std.testing.expect(hasCapacity(0, 16));
    try std.testing.expect(hasCapacity(8, 16));
    try std.testing.expect(hasCapacity(15, 16));
}

test "hasCapacity returns false when at or over limit" {
    try std.testing.expect(!hasCapacity(16, 16));
    try std.testing.expect(!hasCapacity(17, 16));
    try std.testing.expect(!hasCapacity(100, 16));
}

test "safeDetectionTimeout handles normal values" {
    // 800ms * 3 = 2400ms
    try std.testing.expectEqual(@as(u32, 2400), safeDetectionTimeout(800_000, 3));
    // 1000ms * 3 = 3000ms
    try std.testing.expectEqual(@as(u32, 3000), safeDetectionTimeout(1_000_000, 3));
    // 100ms * 5 = 500ms
    try std.testing.expectEqual(@as(u32, 500), safeDetectionTimeout(100_000, 5));
}

test "safeDetectionTimeout handles zero multiplier" {
    // 0 multiplier should give 0 timeout
    try std.testing.expectEqual(@as(u32, 0), safeDetectionTimeout(800_000, 0));
}

test "safeDetectionTimeout handles zero interval" {
    // 0 interval should give 0 timeout
    try std.testing.expectEqual(@as(u32, 0), safeDetectionTimeout(0, 3));
}

test "safeDetectionTimeout prevents overflow with max u32" {
    // Max u32 (4294967295) * max multiplier (255) would overflow
    // Should return 0 instead of crashing
    const result = safeDetectionTimeout(4_294_967_295, 255);
    try std.testing.expectEqual(@as(u32, 0), result);
}

test "safeDetectionTimeout handles large but non-overflowing values" {
    // Just under overflow threshold: (max_u32 / 2) * 2
    const half_max: u32 = 2_147_483_647;
    const result = safeDetectionTimeout(half_max, 2);
    // Should not overflow and produce a valid result
    try std.testing.expect(result > 0);
}

test "BfdSnapshotBudget default values are sane" {
    const budget = BfdSnapshotBudget{};
    try std.testing.expect(budget.max_sessions >= 1);
    try std.testing.expect(budget.max_diag_string_bytes >= 64);
    try std.testing.expect(budget.max_total_bytes >= 1024);
}

test "BFD_DETAIL_BUF_SIZE is adequate for formatted output" {
    // "9999/9999 bfd sessions up" = 24 chars
    try std.testing.expect(BFD_DETAIL_BUF_SIZE >= 24);
    // With extra margin
    try std.testing.expect(BFD_DETAIL_BUF_SIZE >= 64);
}
