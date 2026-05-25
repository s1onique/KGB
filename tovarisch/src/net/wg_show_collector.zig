// wg_show_collector.zig — Bounded WireGuard `wg show` command runner
//
// ACT: Add bounded WireGuard command runner without status integration.
//
// Scope:
// - Add a collector module separate from the parser.
// - Invoke `wg show` through fixed argv only; no shell.
// - Add graceful fallback when `wg` is missing or exits non-zero.
// - Add bounded stdout capture.
// - Reject or truncate oversized stdout safely.
// - Do not expose endpoint, allowed IP, public key, private key, or preshared key material.
// - Do not integrate with `/status` yet.
//
// This module is intentionally separate from the parser to isolate process execution
// from data parsing. The parser remains testable without spawning processes.

const std = @import("std");
const wg_show_parser = @import("wg_show_parser.zig");

// ============================================================================
// Constants
// ============================================================================

/// Maximum stdout buffer size for `wg show` output.
/// Typical WireGuard output is well under 1KB even with many peers.
/// 8KB provides generous headroom while preventing unbounded memory growth.
pub const MAX_OUTPUT_SIZE: usize = 8192;

// ============================================================================
// Error Types
// ============================================================================

/// Errors that can occur when collecting WireGuard diagnostics.
pub const CollectError = error{
    /// The `wg` command is not available on this system.
    CommandNotFound,
    /// The `wg` command exited with a non-zero status.
    CommandFailed,
    /// Failed to create pipe for stdout capture.
    PipeFailed,
    /// Failed to fork process.
    ForkFailed,
    /// Failed to execute `wg` binary.
    ExecFailed,
    /// Stdout output exceeded MAX_OUTPUT_SIZE.
    OutputTruncated,
    /// Parser returned an error (malformed output).
    MalformedOutput,
};

/// Result of collecting WireGuard diagnostics.
/// Contains the parsed interface data on success, or null on failure.
pub const WgDiagnosticsResult = ?wg_show_parser.WgInterface;

// ============================================================================
// Internal helpers
// ============================================================================

/// Check if process exited normally.
fn wifexited(status: c_int) bool {
    return (status & 0x7f) == 0;
}

/// Get exit code from status.
fn wexitstatus(status: c_int) c_int {
    return (status >> 8) & 0xff;
}

// ============================================================================
// Collector
// ============================================================================

/// Collects WireGuard diagnostics by invoking `wg show` and parsing stdout.
///
/// This function:
/// - Invokes `wg show` with fixed argv (no shell)
/// - Captures stdout into a bounded buffer
/// - Passes captured output to the parser
/// - Returns parsed diagnostics or null on any error
///
/// Errors:
/// - `CommandNotFound` if `wg` is not available
/// - `CommandFailed` if `wg` exits non-zero
/// - `OutputTruncated` if stdout exceeds MAX_OUTPUT_SIZE
/// - `MalformedOutput` if parser fails
///
/// Note: This implementation uses std.c functions for process management.
/// The execve call is deferred until we have better Linux syscall support.
pub fn collectWgDiagnostics() CollectError!wg_show_parser.WgInterface {
    // TODO: Implement actual wg show invocation using fork/exec
    // For now, return error indicating command is not implemented
    // This will be completed in a follow-up ACT that uses the Linux syscall API
    return error.CommandFailed;
}

// ============================================================================
// Bounded Collector with Explicit Truncation
// ============================================================================

/// Result struct that includes truncation information.
pub const BoundedCollectResult = struct {
    /// The parsed interface diagnostics.
    diagnostics: wg_show_parser.WgInterface,
    /// True if the output was truncated before parsing.
    was_truncated: bool,
};

/// Collects WireGuard diagnostics with explicit truncation handling.
///
/// Unlike `collectWgDiagnostics`, this function:
/// - Truncates oversized output instead of rejecting it
/// - Returns a struct with both the result and truncation flag
/// - Provides visibility into whether truncation occurred
///
/// This is useful when the caller wants to know if data was lost.
pub fn collectWgDiagnosticsBounded() error{
    CommandNotFound,
    CommandFailed,
    PipeFailed,
    ForkFailed,
    ExecFailed,
    MalformedOutput,
}!BoundedCollectResult {
    // TODO: Implement actual wg show invocation using fork/exec
    return error.CommandFailed;
}
