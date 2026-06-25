// status_checks.zig — Status check implementations
//
// Contains check functions for:
// - getWgPeersCheck() - WireGuard peer diagnostics
// - getWgPeersCheckFromParsed() - test helper
// - getWgPeersCheckFromError() - test helper
//
// Production WireGuard status is now wired through the wg_status_boundary
// typed boundary (Phase 1 complete). The old wg_show_collector is retained
// for legacy test coverage only; production path uses the typed boundary.
//
// WireGuard interface identity is explicit via wg_status_boundary_cli.DEFAULT_WG_INTERFACE.
// No hard-coded "wg0" remains in production path.

const std = @import("std");
const wg_boundary = @import("net/wg_status_boundary.zig");
const wg_boundary_cli = @import("net/wg_status_boundary_cli.zig");
const status = @import("status.zig");

// ============================================================================
// WireGuard Peer Diagnostics Check
// ============================================================================

/// Collects WireGuard diagnostics and returns the appropriate status check.
///
/// Production path: Uses wg_status_boundary CLI backend (Phase 1 complete).
/// The boundary provides typed WireGuardStatus with structured error handling.
///
/// Status semantics:
/// - `ok`: WireGuard interface exists with at least one peer and a handshake.
/// - `warn`: `wg` unavailable, permission denied, malformed output, no peers,
///   or no handshake yet.
/// - All errors map to warn (no hard errors for unavailable tooling).
pub fn getWgPeersCheck(allocator: std.mem.Allocator) status.Check {
    // Phase 1: Use the typed WireGuard status boundary (CLI backend)
    var cli_backend = wg_boundary_cli.CliBackend.init();
    const backend = cli_backend.asBackend();

    // Collect status through the typed boundary
    const wg_result = backend.wireguardStatus(allocator) catch |err| {
        // Handle all error paths as warn (no hard errors for unavailable tooling)
        const detail = wg_boundary.statusErrorDetail(err);
        const boundary_check = wg_boundary.toCheck(wg_boundary.WireGuardStatus.noInterface(), detail);
        return status.Check{
            .name = boundary_check.name,
            .status = mapBoundaryStatus(boundary_check.status),
            .detail = boundary_check.detail,
        };
    };

    // Success: convert WireGuardStatus to Check via boundary helper
    const boundary_check = wg_boundary.toCheck(wg_result.status, null);
    return status.Check{
        .name = boundary_check.name,
        .status = mapBoundaryStatus(boundary_check.status),
        .detail = boundary_check.detail,
    };
}

/// Test helper: creates a wg_peers check from pre-parsed WireGuard data.
/// This bypasses the collector to allow deterministic unit testing.
pub fn getWgPeersCheckFromParsed(comptime peer_count: u32, comptime has_handshake: bool) status.Check {
    // Build WireGuardStatus from parameters
    // Note: latest_handshake_epoch_sec is a Unix timestamp; we use a fake epoch for testing
    const wg_status = wg_boundary.WireGuardStatus{
        .interface = "wg0",
        .peer_count = peer_count,
        .latest_handshake_epoch_sec = if (has_handshake) @as(u64, 1700000000) else null,
        .rx_bytes = 0,
        .tx_bytes = 0,
        .listen_port = null,
        .public_key_redacted = "",
    };

    // Use boundary helper to convert to Check, then map to status.Check
    const boundary_check = wg_boundary.toCheck(wg_status, null);
    return status.Check{
        .name = boundary_check.name,
        .status = mapBoundaryStatus(boundary_check.status),
        .detail = boundary_check.detail,
    };
}

/// Maps boundary CheckStatus to status.CheckStatus.
fn mapBoundaryStatus(boundary_status: wg_boundary.status.CheckStatus) status.CheckStatus {
    return switch (boundary_status) {
        .ok => .ok,
        .warn => .warn,
        .@"error" => .@"error",
        .unknown => .unknown,
    };
}

/// Test helper: creates a wg_peers check from a boundary StatusError.
pub fn getWgPeersCheckFromError(err: wg_boundary.StatusError) status.Check {
    const detail = wg_boundary.statusErrorDetail(err);
    const boundary_check = wg_boundary.toCheck(wg_boundary.WireGuardStatus.noInterface(), detail);
    return status.Check{
        .name = boundary_check.name,
        .status = mapBoundaryStatus(boundary_check.status),
        .detail = boundary_check.detail,
    };
}

// ============================================================================
// Tests for WireGuard Peer Diagnostics
// ============================================================================

test "getWgPeersCheckFromParsed returns ok for peer with handshake" {
    const check = getWgPeersCheckFromParsed(1, true);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.ok, check.status);
    try std.testing.expectEqualStrings("wireguard peers healthy", check.detail);
}

test "getWgPeersCheckFromParsed returns warn for no peers" {
    const check = getWgPeersCheckFromParsed(0, true);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("no peers detected", check.detail);
}

test "getWgPeersCheckFromParsed returns warn for no handshake" {
    const check = getWgPeersCheckFromParsed(1, false);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("no handshake yet", check.detail);
}

test "getWgPeersCheckFromError returns warn for backend_missing" {
    const check = getWgPeersCheckFromError(error.backend_missing);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("wg command not available", check.detail);
}

test "getWgPeersCheckFromError returns warn for permission_denied" {
    const check = getWgPeersCheckFromError(error.permission_denied);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("wg permission denied", check.detail);
}

test "getWgPeersCheckFromError returns warn for malformed_output" {
    const check = getWgPeersCheckFromError(error.malformed_output);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("wg output malformed", check.detail);
}

test "getWgPeersCheckFromError returns warn for timeout" {
    const check = getWgPeersCheckFromError(error.timeout);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("wg command timeout", check.detail);
}

test "getWgPeersCheckFromError returns warn for interface_missing" {
    const check = getWgPeersCheckFromError(error.interface_missing);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("wg interface not found", check.detail);
}

test "getWgPeersCheckFromError returns warn for out_of_memory" {
    const check = getWgPeersCheckFromError(error.out_of_memory);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("wg check out of memory", check.detail);
}
