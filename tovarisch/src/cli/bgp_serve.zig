// cli/bgp_serve.zig — BGP runtime configuration for serve command
//
// ACT 4: Wire BGP session into tovarisch serve runtime.
// Loads config from file and creates BGP runtime for the daemon.
// Keeps config memory alive for daemon lifetime to avoid dangling slices.
//
// KEY CONSTRAINT: When BGP is disabled, ZERO sockets are created.
// This module must NOT call TcpTransport.connect() when BGP is disabled.
//
// ACT runtime: BGP FSM runs in a detached thread (bgp/runtime.zig).
//
// References: RFC 4271 (BGP-4)

const std = @import("std");
const bgp_serve_integration = @import("../bgp/serve_integration.zig");
const bgp_runtime = @import("../bgp/runtime.zig");

/// Result of loading BGP configuration.
pub const BgpLoadResult = bgp_serve_integration.BgpLoadResult;

/// Bundle that owns config memory and BGP runtime state.
pub const BgpServeBundle = bgp_serve_integration.BgpServeBundle;

/// Runtime state for BGP session.
pub const BgpRuntimeState = bgp_serve_integration.BgpRuntimeState;

/// Load config file and validate BGP configuration.
/// Returns BgpLoadResult to distinguish between "no config", "disabled", and errors.
/// Caller owns the returned pointer for the configured case.
///
/// CRITICAL: When this returns .disabled or .no_config, NO sockets are created.
pub fn loadConfigAndBgp(
    config_path: ?[]const u8,
    stderr: anytype,
) BgpLoadResult {
    return bgp_serve_integration.loadConfigAndBgp(config_path, stderr, std.heap.page_allocator);
}

/// Clean up a BGP bundle when shutting down.
pub fn cleanupBgpBundle(bundle: *BgpServeBundle) void {
    bgp_serve_integration.cleanupBgpBundle(bundle, std.heap.page_allocator);
}

/// Get current BGP runtime state.
pub fn getBgpState(bundle: *const BgpServeBundle) BgpRuntimeState {
    return bgp_serve_integration.getBgpState(bundle);
}

/// Get last error message if any.
pub fn getBgpLastError(bundle: *const BgpServeBundle) ?[]const u8 {
    return bgp_serve_integration.getBgpLastError(bundle);
}

/// Start the BGP runtime thread for a configured bundle.
/// Returns true if thread was spawned successfully.
/// Thread failures are non-fatal.
pub fn startBgpRuntimeThread(bundle: *BgpServeBundle, stderr: anytype) bool {
    return bgp_runtime.startBgpRuntimeThread(bundle, stderr);
}
