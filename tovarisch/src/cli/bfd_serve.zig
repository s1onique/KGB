// cli/bfd_serve.zig — BFD runtime configuration for serve command
//
// Loads config from file and creates BFD runtime for the daemon.
// Keeps config memory alive for daemon lifetime to avoid dangling slices.

const std = @import("std");
const wg_args = @import("wg_args.zig");
const config = @import("../config.zig");
const bfd_config = @import("../bfd/config.zig");
const bfd_status = @import("../bfd/status.zig");
const bfd_transport = @import("../bfd/transport.zig");
const bfd_clock = @import("../bfd/clock.zig");

/// Result of loading BFD configuration.
pub const BfdLoadResult = union(enum) {
    /// No config path provided - BFD not requested.
    no_config,
    /// Config exists but BFD is disabled.
    disabled,
    /// BFD runtime successfully created - pointer owned by caller.
    configured: *BfdServeBundle,
    /// Config loading or BFD initialization failed.
    failed,
};

/// Bundle that owns both config memory and BFD runtime.
/// This ensures the address strings stay valid for daemon lifetime.
pub const BfdServeBundle = struct {
    /// Owned config memory - must outlive runtime.
    raw: config.RawConfig,
    /// The BFD runtime - uses addresses from raw.
    runtime: bfd_status.BfdRuntime,
};

/// Load config file and optionally create BFD runtime.
/// Returns BfdLoadResult to distinguish between "no config", "disabled", and errors.
/// Caller owns the returned pointer for the configured case.
pub fn loadConfigAndBfd(
    config_path: ?[]const u8,
    stderr: anytype,
) BfdLoadResult {
    if (config_path == null) {
        return .no_config;
    }

    const path = config_path.?;

    // Read config file
    var raw = wg_args.readConfig(path, std.heap.page_allocator) catch |e| {
        stderr.print("error: failed to read config file '{s}': {s}\n", .{ path, @errorName(e) }) catch {};
        return .failed;
    };

    // Parse BFD config
    const bfd_cfg = config.parseBfdConfig(&raw) catch |e| {
        stderr.print("error: failed to parse BFD config: {s}\n", .{@errorName(e)}) catch {};
        raw.deinit(std.heap.page_allocator);
        return .failed;
    };

    // If BFD is not enabled, clean up and return disabled
    if (!bfd_cfg.enabled) {
        raw.deinit(std.heap.page_allocator);
        return .disabled;
    }

    // Allocate bundle first
    var bundle = std.heap.page_allocator.create(BfdServeBundle) catch {
        stderr.writeAll("error: out of memory creating BFD bundle\n") catch {};
        raw.deinit(std.heap.page_allocator);
        return .failed;
    };

    // Initialize with real UDP transport and real system clock
    bundle.runtime = bfd_status.BfdRuntime.init(
        bfd_transport.RealTransport.interface(),
        bfd_clock.RealClock,
    );

    // Create BFD session config using slices from parsed config
    // NOTE: These slices point into 'raw' which is owned by the bundle.
    const session_cfg = bfd_config.BfdConfig{
        .mode = .multihop,
        .local_addr = bfd_cfg.local_addr,
        .peer_addr = bfd_cfg.peer_addr,
        .interval_ms = bfd_cfg.interval_ms,
        .multiplier = bfd_cfg.multiplier,
        .role = .initiator,
    };

    // Add peer to runtime
    bundle.runtime.addPeer(session_cfg) catch |e| {
        stderr.print("error: failed to add BFD peer: {s}\n", .{@errorName(e)}) catch {};
        raw.deinit(std.heap.page_allocator);
        std.heap.page_allocator.destroy(bundle);
        return .failed;
    };

    // Start all sessions
    bundle.runtime.startAll();

    // Transfer raw config ownership to bundle
    bundle.raw = raw;

    return .{ .configured = bundle };
}

/// Clean up a BFD bundle when shutting down.
pub fn cleanupBfdBundle(bundle: *BfdServeBundle) void {
    bundle.raw.deinit(std.heap.page_allocator);
    std.heap.page_allocator.destroy(bundle);
}
