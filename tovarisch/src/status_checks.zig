// status_checks.zig — Status check implementations
//
// Contains check functions for:
// - getWgPeersCheck() - WireGuard peer diagnostics
// - getWgPeersCheckFromParsed() - test helper
// - getWgPeersCheckFromError() - test helper

const std = @import("std");
const wg_show_collector = @import("net/wg_show_collector.zig");
const status = @import("status.zig");

// ============================================================================
// WireGuard Peer Diagnostics Check
// ============================================================================

/// Collects WireGuard diagnostics and returns the appropriate status check.
///
/// Status semantics:
/// - `ok`: `wg show` succeeds and at least one peer is detected.
/// - `warn`: `wg` unavailable, command fails, malformed output, no peers, or no
///   handshake yet.
/// - `warn`: output truncated.
/// - No hard error for unavailable WireGuard tooling.
///
/// Note: On success, we use a static detail string rather than returning
/// `diag.diagnostics.interface` because `defer diag.deinit()` would free the
/// backing buffer before the return. Static detail is safe for v0.
pub fn getWgPeersCheck(allocator: std.mem.Allocator) status.Check {
    const result = wg_show_collector.collectWgDiagnosticsOwned(allocator);

    // Handle all error paths as warn (no hard errors for unavailable tooling)
    var diag = result catch |err| {
        const detail: []const u8 = switch (err) {
            error.CommandNotFound => "wg command not available",
            error.CommandFailed => "wg command failed",
            error.PipeFailed => "wg pipe creation failed",
            error.ForkFailed => "wg fork failed",
            error.ExecFailed => "wg exec failed",
            error.OutputTruncated => "wg output truncated",
            error.MalformedOutput => "wg output malformed",
            error.OutOfMemory => "wg check out of memory",
        };
        return status.Check{
            .name = "wg_peers",
            .status = .warn,
            .detail = detail,
        };
    };
    defer diag.deinit(allocator);

    // Check for at least one peer
    if (diag.diagnostics.peer_count == 0) {
        return status.Check{
            .name = "wg_peers",
            .status = .warn,
            .detail = "no peers detected",
        };
    }

    // Check for handshake presence (warn if never-handshaked)
    if (diag.diagnostics.latest_handshake_age_sec == null) {
        return status.Check{
            .name = "wg_peers",
            .status = .warn,
            .detail = "no handshake yet",
        };
    }

    // Success: at least one peer with a handshake
    // Use static detail to avoid dangling pointer from freed stdout_buf
    return status.Check{
        .name = "wg_peers",
        .status = .ok,
        .detail = "wireguard peers healthy",
    };
}

/// Test helper: creates a wg_peers check from pre-parsed WireGuard data.
/// This bypasses the collector to allow deterministic unit testing.
pub fn getWgPeersCheckFromParsed(comptime peer_count: u32, comptime has_handshake: bool) status.Check {
    if (peer_count == 0) {
        return status.Check{
            .name = "wg_peers",
            .status = .warn,
            .detail = "no peers detected",
        };
    }
    if (!has_handshake) {
        return status.Check{
            .name = "wg_peers",
            .status = .warn,
            .detail = "no handshake yet",
        };
    }
    return status.Check{
        .name = "wg_peers",
        .status = .ok,
        .detail = "wg0",
    };
}

/// Test helper: creates a wg_peers check from a collector error.
pub fn getWgPeersCheckFromError(err: wg_show_collector.CollectError) status.Check {
    const detail = switch (err) {
        error.CommandNotFound => "wg command not available",
        error.CommandFailed => "wg command failed",
        error.PipeFailed => "wg pipe creation failed",
        error.ForkFailed => "wg fork failed",
        error.ExecFailed => "wg exec failed",
        error.OutputTruncated => "wg output truncated",
        error.MalformedOutput => "wg output malformed",
        error.OutOfMemory => "wg check out of memory",
    };
    return status.Check{
        .name = "wg_peers",
        .status = .warn,
        .detail = detail,
    };
}

// ============================================================================
// Tests for WireGuard Peer Diagnostics
// ============================================================================

test "getWgPeersCheckFromParsed returns ok for peer with handshake" {
    const check = getWgPeersCheckFromParsed(1, true);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.ok, check.status);
    try std.testing.expectEqualStrings("wg0", check.detail);
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

test "getWgPeersCheckFromError returns warn for command not found" {
    const check = getWgPeersCheckFromError(error.CommandNotFound);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("wg command not available", check.detail);
}

test "getWgPeersCheckFromError returns warn for command failed" {
    const check = getWgPeersCheckFromError(error.CommandFailed);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("wg command failed", check.detail);
}

test "getWgPeersCheckFromError returns warn for malformed output" {
    const check = getWgPeersCheckFromError(error.MalformedOutput);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("wg output malformed", check.detail);
}

test "getWgPeersCheckFromError returns warn for output truncated" {
    const check = getWgPeersCheckFromError(error.OutputTruncated);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("wg output truncated", check.detail);
}

test "getWgPeersCheckFromError returns warn for pipe failed" {
    const check = getWgPeersCheckFromError(error.PipeFailed);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("wg pipe creation failed", check.detail);
}

test "getWgPeersCheckFromError returns warn for fork failed" {
    const check = getWgPeersCheckFromError(error.ForkFailed);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("wg fork failed", check.detail);
}

test "getWgPeersCheckFromError returns warn for out of memory" {
    const check = getWgPeersCheckFromError(error.OutOfMemory);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("wg check out of memory", check.detail);
}
