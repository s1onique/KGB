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
pub fn formatPeerDiagnosticDetail(diag: WireGuardPeerDiagnostic, buf: *[DIAGNOSTIC_DETAIL_BUF_SIZE]u8) []const u8 {
    const prefix = "wg ";
    const interface_tag = "interface=";
    const backend_tag = "backend=";
    const timeout_tag = "timeout_secs=";
    const exit_tag = "exit=";

    var pos: usize = 0;

    // "wg <error_kind>: "
    // MemoryCopySafety: buf is fixed-size output buffer, prefix is a string literal (const).
    @memcpy(buf[pos..][0..prefix.len], prefix);
    pos += prefix.len;
    // MemoryCopySafety: buf is fixed-size output buffer, diag.error_kind is an input slice.
    @memcpy(buf[pos..][0..diag.error_kind.len], diag.error_kind);
    pos += diag.error_kind.len;
    buf[pos] = ':';
    pos += 1;
    buf[pos] = ' ';
    pos += 1;

    // "interface=<iface> "
    // MemoryCopySafety: buf is fixed-size output buffer, interface_tag is a string literal.
    @memcpy(buf[pos..][0..interface_tag.len], interface_tag);
    pos += interface_tag.len;
    // MemoryCopySafety: buf is fixed-size output buffer, diag.selected_interface is an input slice.
    @memcpy(buf[pos..][0..diag.selected_interface.len], diag.selected_interface);
    pos += diag.selected_interface.len;
    buf[pos] = ' ';
    pos += 1;

    // "backend=<backend> "
    // MemoryCopySafety: buf is fixed-size output buffer, backend_tag is a string literal.
    @memcpy(buf[pos..][0..backend_tag.len], backend_tag);
    pos += backend_tag.len;
    // MemoryCopySafety: buf is fixed-size output buffer, diag.backend is an input slice.
    @memcpy(buf[pos..][0..diag.backend.len], diag.backend);
    pos += diag.backend.len;
    buf[pos] = ' ';
    pos += 1;

    // "[timeout_secs=<n>] "
    if (diag.timeout_secs) |timeout| {
        // MemoryCopySafety: buf is fixed-size output buffer, timeout_tag is a string literal.
        @memcpy(buf[pos..][0..timeout_tag.len], timeout_tag);
        pos += timeout_tag.len;
        const timeout_str = std.fmt.bufPrint(buf[pos..], "{d}", .{timeout}) catch unreachable;
        pos += timeout_str.len;
        buf[pos] = ' ';
        pos += 1;
    }

    // "[exit=<n>]"
    if (diag.exit_code) |code| {
        // MemoryCopySafety: buf is fixed-size output buffer, exit_tag is a string literal.
        @memcpy(buf[pos..][0..exit_tag.len], exit_tag);
        pos += exit_tag.len;
        const exit_str = std.fmt.bufPrint(buf[pos..], "{d}", .{code}) catch unreachable;
        pos += exit_str.len;
    }

    return buf[0..pos];
}
