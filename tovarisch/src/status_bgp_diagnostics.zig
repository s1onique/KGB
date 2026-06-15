// status_bgp_diagnostics.zig — BGP diagnostics for HTTP status endpoint
//
// Extracts machine-readable BGP runtime diagnostics from BgpStatusState.
// Enables lab verification of reconnect_count via HTTP /status.json.
//
// The preferred JSON shape for lab parsing is:
//
// {
//   "checks": [...],
//   "bgp": {
//     "state": "established",
//     "reconnect_count": 1,
//     "last_socket_error": null
//   }
// }

const std = @import("std");
const bgp_status = @import("bgp/status.zig");
const bgp_serve = @import("cli/bgp_serve.zig");

/// BGP runtime diagnostics for machine-readable access.
/// This enables lab verification of reconnect_count via HTTP /status.json.
pub const BgpDiagnostics = struct {
    /// FSM state string (idle, connect, open_sent, established, etc.)
    /// null when BGP is not configured.
    state: ?[]const u8,
    /// Total reconnect attempts since startup.
    /// Enables lab verification: reconnect_count increased from baseline.
    reconnect_count: u64,
    /// Last TCP socket error message.
    /// null when no socket error has occurred.
    last_socket_error: ?[]const u8,
};

/// Derive BGP diagnostics from BgpStatusState.
/// This extracts the machine-readable fields needed for lab verification.
pub fn deriveBgpDiagnostics(state: bgp_status.BgpStatusState) BgpDiagnostics {
    switch (state) {
        .no_config, .not_configured, .disabled => {
            return .{
                .state = null,
                .reconnect_count = 0,
                .last_socket_error = null,
            };
        },
        .failed, .runtime_failed => {
            return .{
                .state = null,
                .reconnect_count = 0,
                .last_socket_error = null,
            };
        },
        .reconnect_wait => |rw| {
            return .{
                .state = "reconnect_wait",
                .reconnect_count = rw.reconnect_count,
                .last_socket_error = rw.last_socket_error,
            };
        },
        .configured => |cfg| {
            return .{
                .state = cfg.fsm_state,
                // Use persisted reconnect_count from bundle - this survives recovery.
                .reconnect_count = cfg.reconnect_count,
                .last_socket_error = cfg.last_socket_error,
            };
        },
    }
}

/// Derive BGP diagnostics for machine-readable access.
/// Returns null when BGP is not configured, enabling lab verification.
pub fn deriveBgpFromResult(result: bgp_serve.BgpLoadResult) ?BgpDiagnostics {
    const state = bgp_status.statusStateFromLoadResult(result);
    // Only include bgp diagnostics when BGP is configured (has runtime state).
    // .no_config, .not_configured, .disabled, .failed, .runtime_failed return null.
    switch (state) {
        .configured, .reconnect_wait => {},
        else => return null,
    }
    return deriveBgpDiagnostics(state);
}
