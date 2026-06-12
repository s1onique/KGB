// bgp/serve_integration.zig — BGP runtime integration for serve command
//
// Loads config from file and creates BGP runtime for the daemon.
// Keeps config memory alive for daemon lifetime to avoid dangling slices.
//
// KEY CONSTRAINT: When BGP is disabled, ZERO sockets are created.
//
// This module creates the TCP transport and BGP session at load time,
// enabling the session state machine to run during serve.
// Reconnect/backoff lifecycle is handled by the runtime thread.
//
// Thread Safety Model:
// - cleanup_requested: atomic u8 with release/acquire ordering
// - runtime_thread: joined on cleanup (not detached)
// - Bundle state (state, backoff_ms, last_error): written by runtime thread only
// - Status reads: best-effort snapshot during HTTP requests
//
// NOTE: /status reads bundle state without mutex protection. This is acceptable
// because: (1) runtime thread is joined on cleanup before bundle destruction,
// (2) status reads during normal operation may see stale but not corrupt data,
// (3) heavyweight mutex is deferred to future ACT per tiny-leafs doctrine.
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
const passive_listener = @import("passive_listener.zig");
const passive_listener_integration = @import("passive_listener_integration.zig");
const logging = @import("../logging.zig");
const clock = @import("clock.zig");
const reconnect = @import("reconnect_lifecycle.zig");
const prefix_file_loader = @import("prefix_file_loader.zig");
const prefix_file = @import("prefix_file.zig");

// Runtime state for BGP session including reconnect lifecycle.
pub const BgpRuntimeState = enum {
    not_configured,
    disabled,
    configured,
    reconnect_wait,
    failed,
};

// Load result union: no_config, not_configured, disabled, configured, failed.
pub const BgpLoadResult = union(enum) {
    no_config,
    not_configured,
    disabled,
    configured: *BgpServeBundle,
    failed: LoadFailure,
};

pub const LoadFailure = struct {
    message: []const u8,
};

// Serve bundle: owns config memory, BGP runtime, passive listener.
pub const BgpServeBundle = struct {
    const Self = @This();

    raw: config.RawConfig,
    bgp_config: config_parse.BgpConfig,
    session_config: session.SessionConfig,
    state: BgpRuntimeState = .not_configured,
    last_error: ?[]const u8 = null,
    last_error_buf: [64]u8 = undefined,
    prefixes: []types.Ipv4Prefix = &.{},
    tcp: tcp_transport.TcpTransport,
    trans: transport.Transport,
    sess: session.Session,

    // Passive listener for accepting incoming BGP connections.
    // null when no passive listener is configured.
    passive_listener: ?passive_listener.PassiveListener = null,

    // Reconnect state
    backoff_ms: u64 = 0,
    reconnect_deadline: clock.MonoTime = 0,
    // Thread safety: Atomic u8 for cross-thread signaling.
    // cleanupBgpBundle stores 1, isCleanupRequested loads with acquire ordering.
    // This ensures the runtime thread sees the flag before bundle destruction.
    cleanup_requested: u8 = 0,

    // Runtime thread handle (null if not started or already joined)
    runtime_thread: ?std.Thread = null,
};

// ============================================================================
// Config Loading
// ============================================================================

pub fn loadConfigAndBgp(
    config_path: ?[]const u8,
    stderr: anytype,
    allocator: std.mem.Allocator,
) BgpLoadResult {
    if (config_path == null) return .no_config;

    const path = config_path.?;
    var raw = wg_args.readConfig(path, std.heap.page_allocator) catch |e| {
        stderr.print("error: failed to read config file '{s}': {s}\n", .{ path, @errorName(e) }) catch {};
        return .{ .failed = .{ .message = "failed to read config" } };
    };

    const bgp_cfg = config_parse.parseBgpConfig(&raw) catch |e| {
        stderr.print("error: failed to parse BGP config: {s}\n", .{@errorName(e)}) catch {};
        raw.deinit(std.heap.page_allocator);
        return .{ .failed = .{ .message = "failed to parse BGP config" } };
    };

    if (!bgp_cfg.present) {
        raw.deinit(std.heap.page_allocator);
        return .no_config;
    }

    // [bgp] present but not enabled → .disabled (operator explicitly opted out).
    // .not_configured is used when [bgp] section is absent entirely.
    if (!bgp_cfg.enabled) {
        raw.deinit(std.heap.page_allocator);
        return .disabled;
    }

    const local_addr = config_parse.parseIpv4Address(bgp_cfg.local_address) catch |e| {
        stderr.print("error: invalid local_address '{s}': {s}\n", .{ bgp_cfg.local_address, @errorName(e) }) catch {};
        raw.deinit(std.heap.page_allocator);
        return .{ .failed = .{ .message = "invalid local_address" } };
    };

    const router_addr = config_parse.parseIpv4Address(bgp_cfg.router_id) catch |e| {
        stderr.print("error: invalid router_id '{s}': {s}\n", .{ bgp_cfg.router_id, @errorName(e) }) catch {};
        raw.deinit(std.heap.page_allocator);
        return .{ .failed = .{ .message = "invalid router_id" } };
    };

    const peer_addr = config_parse.parseIpv4Address(bgp_cfg.peer_address) catch |e| {
        stderr.print("error: invalid peer_address '{s}': {s}\n", .{ bgp_cfg.peer_address, @errorName(e) }) catch {};
        raw.deinit(std.heap.page_allocator);
        return .{ .failed = .{ .message = "invalid peer_address" } };
    };

    var prefixes = std.ArrayList(types.Ipv4Prefix).empty;
    errdefer prefixes.deinit(allocator);

    // Parse inline prefixes first
    const prefix_strings = config_parse.parsePrefixList(bgp_cfg.advertised_prefixes_raw, allocator) catch |e| {
        stderr.print("error: failed to parse advertised_prefixes: {s}\n", .{@errorName(e)}) catch {};
        raw.deinit(std.heap.page_allocator);
        return .{ .failed = .{ .message = "failed to parse advertised_prefixes" } };
    };
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

    // Load and append prefixes from prefix files (if any)
    if (bgp_cfg.advertised_prefix_files_raw.len > 0) {
        // Load file-by-file to provide path-specific diagnostics
        const file_paths = config_parse.parsePrefixList(bgp_cfg.advertised_prefix_files_raw, allocator) catch |e| {
            stderr.print("error: failed to parse advertised_prefix_files: {s}\n", .{@errorName(e)}) catch {};
            prefixes.deinit(allocator);
            raw.deinit(std.heap.page_allocator);
            return .{ .failed = .{ .message = "failed to parse advertised_prefix_files" } };
        };
        defer allocator.free(file_paths);

        for (file_paths) |file_path| {
            const file_content = prefix_file_loader.loadPrefixFile(file_path, allocator) catch |e| {
                stderr.print("error: failed to read prefix file '{s}': {s}\n", .{ file_path, @errorName(e) }) catch {};
                prefixes.deinit(allocator);
                raw.deinit(std.heap.page_allocator);
                return .{ .failed = .{ .message = "failed to read prefix file" } };
            };
            defer allocator.free(file_content);

            const parse_result = prefix_file.parse(file_content, allocator) catch |e| {
                stderr.print("error: failed to parse prefix file '{s}': {s}\n", .{ file_path, @errorName(e) }) catch {};
                prefixes.deinit(allocator);
                raw.deinit(std.heap.page_allocator);
                return .{ .failed = .{ .message = "failed to parse prefix file" } };
            };
            defer allocator.free(parse_result.prefixes);

            for (parse_result.prefixes) |prefix| {
                prefixes.append(allocator, prefix) catch {
                    stderr.writeAll("error: out of memory merging prefix files\n") catch {};
                    prefixes.deinit(allocator);
                    raw.deinit(std.heap.page_allocator);
                    return .{ .failed = .{ .message = "out of memory merging prefix files" } };
                };
            }
        }
    }


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

    session.validateConfig(session_config) catch |e| {
        stderr.print("error: invalid BGP session config: {s}\n", .{@errorName(e)}) catch {};
        prefixes.deinit(allocator);
        raw.deinit(std.heap.page_allocator);
        return .{ .failed = .{ .message = "invalid BGP session config" } };
    };

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

    const bundle = std.heap.page_allocator.create(BgpServeBundle) catch {
        stderr.writeAll("error: out of memory creating BGP bundle\n") catch {};
        tcp.close();
        prefixes.deinit(allocator);
        raw.deinit(std.heap.page_allocator);
        return .{ .failed = .{ .message = "out of memory creating BGP bundle" } };
    };

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
    bundle.trans = bundle.tcp.toTransport();

    const sess = session.init(session_config, &bundle.trans) catch |e| {
        stderr.print("error: failed to create BGP session: {s}\n", .{@errorName(e)}) catch {};
        bundle.tcp.close();
        prefixes.deinit(allocator);
        raw.deinit(std.heap.page_allocator);
        std.heap.page_allocator.destroy(bundle);
        return .{ .failed = .{ .message = "failed to create BGP session" } };
    };
    bundle.sess = sess;
    bundle.state = .configured;

    // Create passive listener for inbound BGP connections when local_address is configured.
    // The passive listener binds to the configured local_address on port 179 and accepts
    // incoming BGP peer connections alongside the active outbound session.
    passive_listener_integration.createPassiveListener(bundle, stderr);

    return .{ .configured = bundle };
}

// ============================================================================
// Cleanup
// ============================================================================

pub fn cleanupBgpBundle(bundle: *BgpServeBundle, allocator: std.mem.Allocator) void {
    // Signal stop via atomic store (main thread -> runtime thread)
    @atomicStore(u8, &bundle.cleanup_requested, 1, .release);

    // Join runtime thread if present (ensures thread has exited before we destroy bundle)
    if (bundle.runtime_thread) |thread| {
        thread.join();
        bundle.runtime_thread = null;
    }

    // Clean up passive listener if it was created.
    // This stops the listener thread, closes the listen socket, and clears the stored listener.
    passive_listener_integration.closePassiveListener(bundle);

    // Now safe to clean up resources
    bundle.tcp.close();
    allocator.free(bundle.prefixes);
    bundle.raw.deinit(std.heap.page_allocator);
    std.heap.page_allocator.destroy(bundle);
}

// ============================================================================
// Status Accessors
// ============================================================================

pub fn getBgpState(bundle: *const BgpServeBundle) BgpRuntimeState {
    return bundle.state;
}

pub fn getBgpLastError(bundle: *const BgpServeBundle) ?[]const u8 {
    return bundle.last_error;
}

pub fn getSessionStatus(bundle: *BgpServeBundle) session.SessionStatus {
    return session.getStatus(&bundle.sess);
}

// ============================================================================
// Session Execution
// ============================================================================

pub fn runSessionOnce(bundle: *BgpServeBundle) session.RunResult {
    const result = session.runOnce(&bundle.sess) catch |e| {
        bundle.last_error = if (bundle.sess.status.last_error) |session_err|
            copyErrorToBundle(bundle, session_err.message)
        else
            copyErrorToBundle(bundle, @errorName(e));
        bundle.state = .failed;
        return .failed;
    };

    switch (result) {
        .established => bundle.state = .configured,
        .failed => {
            bundle.state = .failed;
            if (bundle.sess.status.last_error) |err| {
                bundle.last_error = copyErrorToBundle(bundle, err.message);
            }
        },
        .stopped => bundle.state = .configured,
        .ok => {},
    }

    return result;
}

fn copyErrorToBundle(bundle: *BgpServeBundle, message: []const u8) []const u8 {
    @memcpy(bundle.last_error_buf[0..message.len], message);
    return bundle.last_error_buf[0..message.len];
}

// ============================================================================
// Reconnect/Backoff Lifecycle
// ============================================================================

pub fn computeNextBackoff(current_ms: u64, max_delay_ms: u64) u64 {
    return reconnect.computeNextBackoff(current_ms, max_delay_ms);
}

pub fn scheduleReconnect(
    bundle: *BgpServeBundle,
    clock_interface: clock.Clock,
    max_delay_ms: u64,
) void {
    // Compute next backoff delay
    bundle.backoff_ms = reconnect.computeNextBackoff(bundle.backoff_ms, max_delay_ms);

    // Set deadline
    const now = clock_interface.getMonoTimeMs();
    bundle.reconnect_deadline = now + bundle.backoff_ms;
    bundle.state = .reconnect_wait;
}

pub fn isReconnectReady(bundle: *BgpServeBundle, clock_interface: clock.Clock) bool {
    if (bundle.state != .reconnect_wait) {
        return false;
    }
    const now = clock_interface.getMonoTimeMs();
    return now >= bundle.reconnect_deadline;
}

pub fn resetBackoff(bundle: *BgpServeBundle) void {
    reconnect.resetBackoff(&bundle.backoff_ms, &bundle.reconnect_deadline);
}

pub fn closeForReconnect(bundle: *BgpServeBundle) void {
    bundle.tcp.close();
    bundle.sess.status.state = .idle;
    bundle.sess.recv_len = 0;
    bundle.sess.send_pos = 0;
    bundle.sess.peer_open = null;
    bundle.sess.negotiated_hold_time = 0;
    bundle.sess.keepalive_interval_ms = 0;
    bundle.sess.hold_timer_deadline = 0;
    bundle.sess.pending_keepalive = false;
    bundle.sess.pending_keepalive_ms = 0;
    // Clear terminal error so session appears reconnectable
    bundle.sess.status.last_error = null;
    bundle.sess.status.last_notification_code = null;
    bundle.sess.status.last_notification_subcode = null;
}

pub fn reconnectTransport(bundle: *BgpServeBundle) !void {
    const tcp_config = tcp_transport.TcpTransportConfig{
        .peer_address = bundle.session_config.peer_address,
        .peer_port = bundle.session_config.peer_port,
        .local_address = bundle.session_config.local_address,
        .connect_timeout_ms = bundle.session_config.connect_timeout_ms,
    };

    bundle.tcp = try tcp_transport.TcpTransport.connect(tcp_config);
    bundle.trans = bundle.tcp.toTransport();
    bundle.sess.trans = &bundle.trans;
}

pub fn doReconnect(bundle: *BgpServeBundle) !void {
    closeForReconnect(bundle);
    try reconnectTransport(bundle);
    resetBackoff(bundle);
    bundle.state = .configured;
    bundle.last_error = null;
}

pub fn isCleanupRequested(bundle: *BgpServeBundle) bool {
    return @atomicLoad(u8, &bundle.cleanup_requested, .acquire) != 0;
}

// ============================================================================
// Constants Export
// ============================================================================

pub const DEFAULT_RECONNECT_INITIAL_MS = reconnect.DEFAULT_RECONNECT_INITIAL_MS;
pub const DEFAULT_RECONNECT_MAX_MS = reconnect.DEFAULT_RECONNECT_MAX_MS;
pub const DEFAULT_RECONNECT_MULTIPLIER = reconnect.DEFAULT_RECONNECT_MULTIPLIER;
