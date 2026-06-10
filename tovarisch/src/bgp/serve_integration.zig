// bgp/serve_integration.zig — BGP runtime integration for serve command
//
// ACT 4: Wire BGP session into tovarisch serve runtime.
// Loads config from file and creates BGP runtime for the daemon.
// Keeps config memory alive for daemon lifetime to avoid dangling slices.
//
// KEY CONSTRAINT: When BGP is disabled, ZERO sockets are created.
// This module must NOT call TcpTransport.connect() when BGP is disabled.
//
// ACT 4 Scope: This module only builds and validates BGP runtime config.
// Real connection startup is deferred to a later ACT after bounded connect exists.
// This keeps serve startup safe - no blocking connect calls.
//
// References: RFC 4271 (BGP-4)

const std = @import("std");
const wg_args = @import("../cli/wg_args.zig");
const config = @import("../config.zig");
const config_parse = @import("config_parse.zig");
const session = @import("session.zig");
const types = @import("types.zig");

/// Runtime state for BGP session
pub const BgpRuntimeState = enum {
    /// BGP not configured (no [bgp] section in config)
    not_configured,
    /// BGP configured but disabled
    disabled,
    /// BGP config built and validated, ready for connection
    configured,
    /// BGP config build or validation failed
    failed,
};

/// Result of loading BGP configuration.
pub const BgpLoadResult = union(enum) {
    /// No config path provided - BGP not requested.
    no_config,
    /// Config exists but BGP is disabled.
    disabled,
    /// BGP runtime configured - pointer owned by caller.
    /// Note: Session is ready but not yet connected (connect deferred).
    configured: *BgpServeBundle,
    /// Config loading or BGP initialization failed.
    failed,
};

/// Bundle that owns config memory and BGP runtime state.
/// This ACT builds and validates config but does NOT call TcpTransport.connect().
/// Connection is deferred to a future ACT after bounded nonblocking connect exists.
pub const BgpServeBundle = struct {
    const Self = @This();

    /// Owned config memory - must outlive runtime.
    raw: config.RawConfig,
    /// The parsed BGP config.
    bgp_config: config_parse.BgpConfig,
    /// BGP session config (built but session not created yet).
    session_config: session.SessionConfig,
    /// Current runtime state.
    state: BgpRuntimeState = .not_configured,
    /// Last error message (null if no error).
    last_error: ?[]const u8 = null,
    /// Advertised prefixes (parsed from config).
    prefixes: []types.Ipv4Prefix = &.{},
};

/// Load config file and validate BGP configuration.
/// Returns BgpLoadResult to distinguish between "no config", "disabled", and errors.
/// Caller owns the returned pointer for the configured case.
///
/// CRITICAL: When this returns .disabled or .no_config, NO sockets are created.
/// ACT 4: This function does NOT call TcpTransport.connect().
/// Connection is deferred to a future ACT after bounded nonblocking connect exists.
pub fn loadConfigAndBgp(
    config_path: ?[]const u8,
    stderr: anytype,
    allocator: std.mem.Allocator,
) BgpLoadResult {
    if (config_path == null) {
        return .no_config;
    }

    const path = config_path.?;

    // Read config file
    var raw = wg_args.readConfig(path, std.heap.page_allocator) catch |e| {
        stderr.print("error: failed to read config file '{s}': {s}\n", .{ path, @errorName(e) }) catch {};
        return .failed;
    };

    // Parse BGP config (includes advertised_prefixes parsing now)
    const bgp_cfg = config_parse.parseBgpConfig(&raw) catch |e| {
        stderr.print("error: failed to parse BGP config: {s}\n", .{@errorName(e)}) catch {};
        raw.deinit(std.heap.page_allocator);
        return .failed;
    };

    // If [bgp] section is not present, return no_config
    // This is distinguishable from present-but-disabled
    if (!bgp_cfg.present) {
        raw.deinit(std.heap.page_allocator);
        return .no_config;
    }

    // If BGP is not enabled, clean up and return disabled
    // CRITICAL: Zero sockets are created here
    if (!bgp_cfg.enabled) {
        raw.deinit(std.heap.page_allocator);
        return .disabled;
    }

    // Now BGP is enabled - validate and build config.
    // We do NOT call TcpTransport.connect() in this ACT.
    // Connection is deferred to a future ACT.

    // Parse local address using plain IPv4 parser (not CIDR)
    const local_addr = config_parse.parseIpv4Address(bgp_cfg.local_address) catch |e| {
        stderr.print("error: invalid local_address '{s}': {s}\n", .{ bgp_cfg.local_address, @errorName(e) }) catch {};
        raw.deinit(std.heap.page_allocator);
        return .failed;
    };

    // Parse router_id using plain IPv4 parser
    const router_addr = config_parse.parseIpv4Address(bgp_cfg.router_id) catch |e| {
        stderr.print("error: invalid router_id '{s}': {s}\n", .{ bgp_cfg.router_id, @errorName(e) }) catch {};
        raw.deinit(std.heap.page_allocator);
        return .failed;
    };

    // Parse peer address using plain IPv4 parser
    const peer_addr = config_parse.parseIpv4Address(bgp_cfg.peer_address) catch |e| {
        stderr.print("error: invalid peer_address '{s}': {s}\n", .{ bgp_cfg.peer_address, @errorName(e) }) catch {};
        raw.deinit(std.heap.page_allocator);
        return .failed;
    };

    // Parse advertised prefixes from raw config string (comma-separated CIDR list)
    // This is the runtime-owned allocation - freed by defer below
    var prefixes = std.ArrayList(types.Ipv4Prefix).empty;
    errdefer prefixes.deinit(allocator);

    // Parse the comma-separated prefix list
    const prefix_strings = config_parse.parsePrefixList(bgp_cfg.advertised_prefixes_raw, allocator) catch |e| {
        stderr.print("error: failed to parse advertised_prefixes: {s}\n", .{@errorName(e)}) catch {};
        raw.deinit(std.heap.page_allocator);
        return .failed;
    };
    // Free prefix_strings on any exit from this block
    defer allocator.free(prefix_strings);

    for (prefix_strings) |cidr| {
        const prefix = types.Ipv4Prefix.parse(cidr) catch |e| {
            stderr.print("error: invalid advertised_prefix '{s}': {s}\n", .{ cidr, @errorName(e) }) catch {};
            prefixes.deinit(allocator);
            raw.deinit(std.heap.page_allocator);
            return .failed;
        };
        prefixes.append(allocator, prefix) catch {
            stderr.writeAll("error: out of memory parsing prefixes\n") catch {};
            prefixes.deinit(allocator);
            raw.deinit(std.heap.page_allocator);
            return .failed;
        };
    }

    // Require at least one prefix when enabled
    if (prefixes.items.len == 0) {
        stderr.writeAll("error: no advertised_prefixes configured\n") catch {};
        prefixes.deinit(allocator);
        raw.deinit(std.heap.page_allocator);
        return .failed;
    }

    // Build SessionConfig for BGP session
    const session_config = session.SessionConfig{
        .peer_address = peer_addr,
        .peer_port = bgp_cfg.peer_port,
        .local_address = local_addr,
        .local_as = bgp_cfg.local_as,
        .peer_as = bgp_cfg.peer_as,
        .router_id = router_addr,
        .hold_time_seconds = bgp_cfg.hold_time_seconds,
        .keepalive_seconds = bgp_cfg.keepalive_seconds,
        .connect_timeout_ms = bgp_cfg.connect_timeout_ms,
        .prefixes = prefixes.items,
        .same_as = bgp_cfg.same_as,
    };

    // Validate session config (ASN ranges, hold time, etc.)
    session.validateConfig(session_config) catch |e| {
        stderr.print("error: invalid BGP session config: {s}\n", .{@errorName(e)}) catch {};
        prefixes.deinit(allocator);
        raw.deinit(std.heap.page_allocator);
        return .failed;
    };

    // Allocate bundle
    const bundle = std.heap.page_allocator.create(BgpServeBundle) catch {
        stderr.writeAll("error: out of memory creating BGP bundle\n") catch {};
        prefixes.deinit(allocator);
        raw.deinit(std.heap.page_allocator);
        return .failed;
    };

    // Initialize bundle - state is "configured" not "running"
    // because we haven't connected yet (deferred to future ACT)
    bundle.* = BgpServeBundle{
        .raw = raw,
        .bgp_config = bgp_cfg,
        .session_config = session_config,
        .state = .configured,
        .last_error = null,
        .prefixes = prefixes.items,
    };

    return .{ .configured = bundle };
}

/// Clean up a BGP bundle when shutting down.
pub fn cleanupBgpBundle(bundle: *BgpServeBundle, allocator: std.mem.Allocator) void {
    // Free prefixes
    allocator.free(bundle.prefixes);

    // Deinit raw config
    bundle.raw.deinit(std.heap.page_allocator);

    // Destroy bundle
    std.heap.page_allocator.destroy(bundle);
}

/// Get current BGP runtime state.
pub fn getBgpState(bundle: *const BgpServeBundle) BgpRuntimeState {
    return bundle.state;
}

/// Get last error message if any.
pub fn getBgpLastError(bundle: *const BgpServeBundle) ?[]const u8 {
    return bundle.last_error;
}
