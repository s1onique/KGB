// bgp/serve_integration.zig — BGP runtime integration for serve command
//
// ACT 5: Wire BGP session into tovarisch serve runtime.
// Loads config from file and creates BGP runtime for the daemon.
// Keeps config memory alive for daemon lifetime to avoid dangling slices.
//
// KEY CONSTRAINT: When BGP is disabled, ZERO sockets are created.
// This module must NOT call TcpTransport.connect() when BGP is disabled.
//
// This module creates the TCP transport and BGP session at load time,
// enabling the session state machine to run during serve.
//
// References: RFC 4271 (BGP-4)

const std = @import("std");
const wg_args = @import("../cli/wg_args.zig");
const config = @import("../config.zig");
const config_parse = @import("config_parse.zig");
const session = @import("session.zig");
const types = @import("types.zig");
const tcp_transport = @import("tcp_transport.zig");
const transport = @import("transport.zig");

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
    /// Includes error message for status reporting.
    failed: LoadFailure,
};

/// Error details for failed BGP load.
pub const LoadFailure = struct {
    /// Sanitized error message for status reporting.
    message: []const u8,
};

/// Bundle that owns config memory and BGP runtime state.
/// Includes TCP transport and session for the full BGP state machine.
pub const BgpServeBundle = struct {
    const Self = @This();

    /// Owned config memory - must outlive runtime.
    raw: config.RawConfig,
    /// The parsed BGP config.
    bgp_config: config_parse.BgpConfig,
    /// BGP session config (built at load time).
    session_config: session.SessionConfig,
    /// Current runtime state.
    state: BgpRuntimeState = .not_configured,
    /// Last error message (null if no error).
    last_error: ?[]const u8 = null,
    /// Advertised prefixes (parsed from config).
    prefixes: []types.Ipv4Prefix = &.{},
    /// TCP transport for the BGP session.
    tcp: tcp_transport.TcpTransport,
    /// Transport wrapper (owned by bundle, lives as long as sess needs it).
    trans: transport.Transport,
    /// BGP session state machine.
    sess: session.Session,
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
        return .{ .failed = .{ .message = "failed to read config" } };
    };

    // Parse BGP config (includes advertised_prefixes parsing now)
    const bgp_cfg = config_parse.parseBgpConfig(&raw) catch |e| {
        stderr.print("error: failed to parse BGP config: {s}\n", .{@errorName(e)}) catch {};
        raw.deinit(std.heap.page_allocator);
        return .{ .failed = .{ .message = "failed to parse BGP config" } };
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
        return .{ .failed = .{ .message = "invalid local_address" } };
    };

    // Parse router_id using plain IPv4 parser
    const router_addr = config_parse.parseIpv4Address(bgp_cfg.router_id) catch |e| {
        stderr.print("error: invalid router_id '{s}': {s}\n", .{ bgp_cfg.router_id, @errorName(e) }) catch {};
        raw.deinit(std.heap.page_allocator);
        return .{ .failed = .{ .message = "invalid router_id" } };
    };

    // Parse peer address using plain IPv4 parser
    const peer_addr = config_parse.parseIpv4Address(bgp_cfg.peer_address) catch |e| {
        stderr.print("error: invalid peer_address '{s}': {s}\n", .{ bgp_cfg.peer_address, @errorName(e) }) catch {};
        raw.deinit(std.heap.page_allocator);
        return .{ .failed = .{ .message = "invalid peer_address" } };
    };

    // Parse advertised prefixes from raw config string (comma-separated CIDR list)
    // This is the runtime-owned allocation - freed by defer below
    var prefixes = std.ArrayList(types.Ipv4Prefix).empty;
    errdefer prefixes.deinit(allocator);

    // Parse the comma-separated prefix list
    const prefix_strings = config_parse.parsePrefixList(bgp_cfg.advertised_prefixes_raw, allocator) catch |e| {
        stderr.print("error: failed to parse advertised_prefixes: {s}\n", .{@errorName(e)}) catch {};
        raw.deinit(std.heap.page_allocator);
        return .{ .failed = .{ .message = "failed to parse advertised_prefixes" } };
    };
    // Free prefix_strings on any exit from this block
    defer allocator.free(prefix_strings);

    for (prefix_strings) |cidr| {
        const prefix = types.Ipv4Prefix.parse(cidr) catch |e| {
            stderr.print("error: invalid advertised_prefix '{s}': {s}\n", .{ cidr, @errorName(e) }) catch {};
            prefixes.deinit(allocator);
            raw.deinit(std.heap.page_allocator);
            return .{ .failed = .{ .message = "invalid advertised_prefix" } };
        };
        prefixes.append(allocator, prefix) catch {
            stderr.writeAll("error: out of memory parsing prefixes\n") catch {};
            prefixes.deinit(allocator);
            raw.deinit(std.heap.page_allocator);
            return .{ .failed = .{ .message = "out of memory parsing prefixes" } };
        };
    }

    // Zero prefixes is valid - allows OPEN/KEEPALIVE-only smoke test without route advertisement

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
        return .{ .failed = .{ .message = "invalid BGP session config" } };
    };

    // Create TCP transport with bounded connect timeout
    const tcp_config = tcp_transport.TcpTransportConfig{
        .peer_address = peer_addr,
        .peer_port = bgp_cfg.peer_port,
        .local_address = local_addr,
        .connect_timeout_ms = bgp_cfg.connect_timeout_ms,
    };
    var tcp = tcp_transport.TcpTransport.connect(tcp_config) catch |e| {
        stderr.print("error: failed to connect to BGP peer: {s}\n", .{@errorName(e)}) catch {};
        prefixes.deinit(allocator);
        raw.deinit(std.heap.page_allocator);
        return .{ .failed = .{ .message = "BGP connect failed" } };
    };

    // Allocate bundle first so we can store trans in it
    const bundle = std.heap.page_allocator.create(BgpServeBundle) catch {
        stderr.writeAll("error: out of memory creating BGP bundle\n") catch {};
        tcp.close();
        prefixes.deinit(allocator);
        raw.deinit(std.heap.page_allocator);
        return .{ .failed = .{ .message = "out of memory creating BGP bundle" } };
    };

    // Initialize transport wrapper in bundle (owned by bundle, lives as long as sess)
    bundle.* = BgpServeBundle{
        .raw = raw,
        .bgp_config = bgp_cfg,
        .session_config = session_config,
        .state = .not_configured,
        .last_error = null,
        .prefixes = prefixes.items,
        .tcp = undefined,
        .trans = undefined,
        .sess = undefined,
    };
    bundle.tcp = tcp;
    bundle.trans = tcp.toTransport();

    // Create BGP session with the bundle-owned transport
    const sess = session.init(session_config, &bundle.trans) catch |e| {
        stderr.print("error: failed to create BGP session: {s}\n", .{@errorName(e)}) catch {};
        tcp.close();
        prefixes.deinit(allocator);
        raw.deinit(std.heap.page_allocator);
        std.heap.page_allocator.destroy(bundle);
        return .{ .failed = .{ .message = "failed to create BGP session" } };
    };
    bundle.sess = sess;
    bundle.state = .configured;

    return .{ .configured = bundle };
}

/// Clean up a BGP bundle when shutting down.
pub fn cleanupBgpBundle(bundle: *BgpServeBundle, allocator: std.mem.Allocator) void {
    // Close TCP transport
    bundle.tcp.close();

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

/// Run one iteration of the BGP session state machine.
/// Returns RunResult indicating session state after the iteration.
pub fn runSessionOnce(bundle: *BgpServeBundle) session.RunResult {
    const result = session.runOnce(&bundle.sess) catch |e| {
        bundle.last_error = @errorName(e);
        bundle.state = .failed;
        return .failed;
    };

    // Sync runtime state with session state
    switch (result) {
        .established => bundle.state = .configured,
        .failed => {
            bundle.state = .failed;
            if (bundle.sess.status.last_error) |err| {
                bundle.last_error = err.message;
            }
        },
        .stopped => bundle.state = .configured,
        .ok => {},
    }

    return result;
}

/// Get the BGP session status for /status JSON output.
pub fn getSessionStatus(bundle: *BgpServeBundle) session.SessionStatus {
    return session.getStatus(&bundle.sess);
}
