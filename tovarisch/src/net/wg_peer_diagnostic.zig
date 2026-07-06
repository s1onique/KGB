// wg_peer_diagnostic.zig — WireGuard peer diagnostic structures
//
// ACT-HULK29R-ZIG016-WG-PEERS-TIMEOUT-DIAGNOSTIC-SEAM
// Structured diagnostic information for WireGuard peer check failures.

const std = @import("std");

// ============================================================================
// WireGuard Peer Diagnostic
// ============================================================================

/// Structured diagnostic information for WireGuard peer check failures.
/// Used to provide actionable detail without leaking borrowed memory.
///
/// Memory ownership rules:
/// - All fields are value types or borrowed slices that must NOT outlive
///   OwnedWgCommandResult.deinit()
/// - stderr_excerpt is null unless copied/escaped before deinit
pub const WireGuardPeerDiagnostic = struct {
    /// Backend used: "cli" or "netlink".
    backend: []const u8,
    /// Selected interface name (e.g., "wg-kgb0").
    selected_interface: []const u8,
    /// Command that was attempted (e.g., "wg show wg-kgb0 dump").
    command: []const u8,
    /// Timeout in seconds if applicable.
    timeout_secs: ?u64,
    /// Exit code from the command.
    exit_code: ?u8,
    /// Machine-readable error kind for programmatic inspection.
    error_kind: []const u8,
    /// Stderr length in bytes (safe value - no slice borrow).
    stderr_len: usize,
    /// Stdout length in bytes (safe value - no slice borrow).
    stdout_len: usize,
};

/// Maximum length for formatted diagnostic detail strings.
pub const DIAGNOSTIC_DETAIL_BUF_SIZE: usize = 256;

/// Formats a detailed diagnostic string for status check detail field.
/// Uses value fields only (no borrowed slices from command results).
///
/// Returns a static or formatted string safe for status JSON.
/// Format: "wg <error_kind>: interface=<iface> backend=<backend> [timeout_secs=<n>] [exit=<n>]"
///
/// Uses branch-specific std.fmt.bufPrint calls for bounds-safe formatting.
/// Falls back to truncatedDiagnostic if buffer capacity is exhausted.
pub fn formatPeerDiagnosticDetail(diag: WireGuardPeerDiagnostic, buf: *[DIAGNOSTIC_DETAIL_BUF_SIZE]u8) []const u8 {
    if (diag.timeout_secs) |timeout| {
        if (diag.exit_code) |code| {
            return std.fmt.bufPrint(
                buf,
                "wg {s}: interface={s} backend={s} timeout_secs={d} exit={d}",
                .{ diag.error_kind, diag.selected_interface, diag.backend, timeout, code },
            ) catch truncatedDiagnostic(buf);
        }

        return std.fmt.bufPrint(
            buf,
            "wg {s}: interface={s} backend={s} timeout_secs={d}",
            .{ diag.error_kind, diag.selected_interface, diag.backend, timeout },
        ) catch truncatedDiagnostic(buf);
    }

    if (diag.exit_code) |code| {
        return std.fmt.bufPrint(
            buf,
            "wg {s}: interface={s} backend={s} exit={d}",
            .{ diag.error_kind, diag.selected_interface, diag.backend, code },
        ) catch truncatedDiagnostic(buf);
    }

    return std.fmt.bufPrint(
        buf,
        "wg {s}: interface={s} backend={s}",
        .{ diag.error_kind, diag.selected_interface, diag.backend },
    ) catch truncatedDiagnostic(buf);
}

/// Truncated fallback when buffer capacity is exhausted.
/// MemoryCopySafety: buf and fallback are independent buffers; fallback is a string literal.
fn truncatedDiagnostic(buf: *[DIAGNOSTIC_DETAIL_BUF_SIZE]u8) []const u8 {
    const fallback = "wg diagnostic_truncated";
    const n = @min(fallback.len, buf.len);
    @memcpy(buf[0..n], fallback[0..n]);
    return buf[0..n];
}

// ============================================================================
// Tests
// ============================================================================

const testing = std.testing;

test "formatPeerDiagnosticDetail: no optional fields" {
    const diag = WireGuardPeerDiagnostic{
        .backend = "cli",
        .selected_interface = "wg-kgb0",
        .command = "wg show wg-kgb0 dump",
        .timeout_secs = null,
        .exit_code = null,
        .error_kind = "backend_missing",
        .stderr_len = 0,
        .stdout_len = 0,
    };
    var buf: [DIAGNOSTIC_DETAIL_BUF_SIZE]u8 = undefined;
    const result = formatPeerDiagnosticDetail(diag, &buf);
    try testing.expectEqualStrings("wg backend_missing: interface=wg-kgb0 backend=cli", result);
}

test "formatPeerDiagnosticDetail: timeout only" {
    const diag = WireGuardPeerDiagnostic{
        .backend = "cli",
        .selected_interface = "wg-kgb0",
        .command = "wg show wg-kgb0 dump",
        .timeout_secs = 5,
        .exit_code = null,
        .error_kind = "timeout",
        .stderr_len = 0,
        .stdout_len = 0,
    };
    var buf: [DIAGNOSTIC_DETAIL_BUF_SIZE]u8 = undefined;
    const result = formatPeerDiagnosticDetail(diag, &buf);
    try testing.expectEqualStrings("wg timeout: interface=wg-kgb0 backend=cli timeout_secs=5", result);
}

test "formatPeerDiagnosticDetail: exit only" {
    const diag = WireGuardPeerDiagnostic{
        .backend = "cli",
        .selected_interface = "wg-kgb0",
        .command = "wg show wg-kgb0 dump",
        .timeout_secs = null,
        .exit_code = 1,
        .error_kind = "interface_missing",
        .stderr_len = 0,
        .stdout_len = 0,
    };
    var buf: [DIAGNOSTIC_DETAIL_BUF_SIZE]u8 = undefined;
    const result = formatPeerDiagnosticDetail(diag, &buf);
    try testing.expectEqualStrings("wg interface_missing: interface=wg-kgb0 backend=cli exit=1", result);
}

test "formatPeerDiagnosticDetail: timeout and exit" {
    const diag = WireGuardPeerDiagnostic{
        .backend = "cli",
        .selected_interface = "wg-kgb0",
        .command = "wg show wg-kgb0 dump",
        .timeout_secs = 5,
        .exit_code = 255,
        .error_kind = "timeout",
        .stderr_len = 0,
        .stdout_len = 0,
    };
    var buf: [DIAGNOSTIC_DETAIL_BUF_SIZE]u8 = undefined;
    const result = formatPeerDiagnosticDetail(diag, &buf);
    try testing.expectEqualStrings("wg timeout: interface=wg-kgb0 backend=cli timeout_secs=5 exit=255", result);
}
