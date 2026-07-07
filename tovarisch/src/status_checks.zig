// status_checks.zig — Status check implementations
//
// Contains check functions for:
// - getWgPeersCheck() - WireGuard peer diagnostics with diagnostic detail
// - getWgPeersCheckFromParsed() - test helper
// - getWgPeersCheckFromError() - test helper
//
// Production WireGuard status is now wired through the wg_status_boundary
// typed boundary (Phase 1 complete). The old wg_show_collector is retained
// for legacy test coverage only; production path uses the typed boundary.
//
// WireGuard interface identity is explicit via wg_status_boundary_cli.DEFAULT_WG_INTERFACE.
// No hard-coded "wg0" remains in production path.
//
// ACT-HULK29R-ZIG016-WG-PEERS-DIAGNOSTIC-INTEGRATION:
// The wg_peers check now uses diagnostic-aware status collection to provide
// structured detail such as "wg timeout: interface=wg-kgb0 backend=cli timeout_secs=5".

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
///
/// Diagnostic detail: On error, the check detail includes structured context
/// such as interface=wg-kgb0 backend=cli timeout_secs=5 exit=1.
pub fn getWgPeersCheck(allocator: std.mem.Allocator) status.Check {
    // Use diagnostic-aware status collection for structured error detail
    const attempt = wg_boundary_cli.cliWireguardStatusDiagnosticAttemptWithRunner(
        allocator,
        null, // use real wg path lookup
        wg_boundary_cli.WgCommandRunner{
            .runFn = struct {
                fn f(
                    alloc: std.mem.Allocator,
                    _: ?*anyopaque,
                    wg_path: [*:0]const u8,
                    interface_name: []const u8,
                    timeout_secs: u64,
                ) anyerror!wg_boundary_cli.OwnedWgCommandResult {
                    return wg_boundary_cli.runWgShowDump(alloc, wg_path, interface_name, timeout_secs);
                }
            }.f,
        },
    );

    switch (attempt) {
        .ok => |ok_result| {
            // Success: convert WireGuardStatus to Check via boundary helper
            const boundary_check = wg_boundary.toCheck(ok_result.status, null);
            return status.Check{
                .name = boundary_check.name,
                .status = mapBoundaryStatus(boundary_check.status),
                .detail = boundary_check.detail,
            };
        },
        .err => |bad| {
            // Error path: format diagnostic detail into the check
            // MemoryOwnership: detail string must be allocated via the passed allocator
            // so it outlives this function and remains valid during JSON serialization.
            // For CLI status rendering, this allocation is bounded and lives until
            // process exit after JSON serialization. Callers that provide a freeing
            // allocator may release it after rendering.
            var detail_buf: [wg_boundary.DIAGNOSTIC_DETAIL_BUF_SIZE]u8 = undefined;
            const detail_formatted = wg_boundary.formatPeerDiagnosticDetail(bad.diagnostic, &detail_buf);

            // Allocate owned copy so the slice is valid until after JSON serialization
            const detail_owned = allocator.dupe(u8, detail_formatted) catch {
                // Fallback to static string on allocation failure
                const detail = "wg diagnostic error";
                return status.Check{
                    .name = "wg_peers",
                    .status = .warn,
                    .detail = detail,
                };
            };

            // Create boundary check with the allocated diagnostic detail
            const boundary_check = wg_boundary.toCheck(
                wg_boundary.WireGuardStatus.noInterface(),
                detail_owned,
            );
            return status.Check{
                .name = boundary_check.name,
                .status = mapBoundaryStatus(boundary_check.status),
                .detail = boundary_check.detail,
            };
        },
    }
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
