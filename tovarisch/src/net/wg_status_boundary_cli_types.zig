// wg_status_boundary_cli_types.zig — CLI backend types for WireGuard status boundary
//
// Part of wg_status_boundary_cli.zig (split to satisfy LLM-friendliness limits).
// Contains types, constants, and interface configuration.
//
// ACT-HULK29R-ZIG016-WG-STATUS-CLASSIFICATION-FIX

const std = @import("std");
const wg = @import("wg_status_boundary.zig");
const config_parse_helpers = @import("../config_parse_helpers.zig");

// ============================================================================
// Diagnostic Attempt Union
// ============================================================================

/// Union that carries both status and diagnostic on all paths.
/// This allows callers to get structured diagnostic context even on error paths,
/// unlike an error union which would lose diagnostic context on `return error.timeout`.
///
/// Usage:
///   const attempt = cliWireguardStatusDiagnosticAttemptWithRunner(...);
///   switch (attempt) {
///       .ok => |ok| use ok.status and ok.diagnostic,
///       .err => |bad| use bad.err and bad.diagnostic,
///   }
pub const WireGuardStatusDiagnosticAttempt = union(enum) {
    ok: struct {
        status: wg.WireGuardStatus,
        diagnostic: wg.WireGuardPeerDiagnostic,
    },
    err: struct {
        err: wg.StatusError,
        diagnostic: wg.WireGuardPeerDiagnostic,
    },
};

// ============================================================================
// Interface Name Configuration
// ============================================================================

/// Default WireGuard interface name when not explicitly configured.
/// Documented single source of truth for this default value.
pub const DEFAULT_WG_INTERFACE: [:0]const u8 = "wg-kgb0";

/// Validates an interface name for safety before passing to wg command.
pub fn isValidInterfaceName(name: []const u8) bool {
    return config_parse_helpers.isValidInterfaceName(name);
}

// ============================================================================
// CLI Backend Constants
// ============================================================================

/// Default timeout for wg show command (5 seconds).
pub const DEFAULT_TIMEOUT_SECS: u64 = 5;

/// Maximum stdout buffer size (8KB) - sufficient for dump output.
pub const MAX_STDOUT_SIZE: usize = 8192;

/// Maximum stderr buffer size (1KB).
pub const MAX_STDERR_SIZE: usize = 1024;

/// Allowed paths to the WireGuard `wg` command.
pub const WG_PATHS = [_][*:0]const u8{
    "/usr/bin/wg",
    "/usr/sbin/wg",
    "/sbin/wg",
};

// ============================================================================
// WireGuard Command Kinds
// ============================================================================

/// WireGuard command kinds for diagnostic reporting.
pub const WgCommandKind = enum {
    /// wg show <interface> dump
    show_dump,
    /// wg show interfaces
    show_interfaces,
    /// ip -d link show <interface>
    ip_link_show,
};

/// Returns a stable command label without the interface name.
pub fn wgCommandLabel(kind: WgCommandKind) []const u8 {
    return switch (kind) {
        .show_dump => "wg show <interface> dump",
        .show_interfaces => "wg show interfaces",
        .ip_link_show => "ip -d link show <interface>",
    };
}
