// bgp/serve_integration.zig — daemon's BGP config wrapper, status
// accessors, and lifecycle re-exports. The canonical constructor
// lives in `serve_bundle_constructor.zig` (FA-3 final split). This
// module creates the TCP transport and BGP session at load time;
// reconnect/backoff lifecycle is handled by the runtime thread.
//
// KEY CONSTRAINT: When BGP is disabled, ZERO sockets are created.
//
// Thread Safety Model:
// - cleanup_requested: atomic u8 with release/acquire ordering;
//   runtime thread is joined on cleanup before bundle destruction.
// - Bundle state (state, backoff_ms, last_error): written by runtime
//   thread only. Status reads are best-effort snapshots; no mutex
//   (deferred to a future ACT per tiny-leafs doctrine).
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
const clock = @import("clock.zig");
const reconnect = @import("reconnect_lifecycle.zig");
const reconnect_ownership = @import("reconnect_ownership.zig");
const allocation_tracker = @import("../runtime/allocation_tracker.zig");
const prefix_file_loader = @import("prefix_file_loader.zig");
const prefix_file = @import("prefix_file.zig");
const session_config_builder = @import("session_config_builder.zig");
const update_diagnostics = @import("update_diagnostics.zig");
const export_reload_apply = @import("export_reload_apply.zig");
const prefix_watch = @import("prefix_watch.zig");
const serve_export_integration = @import("serve_export_integration.zig");
const serve_bundle_constructor = @import("serve_bundle_constructor.zig");

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

pub const ReconnectConnector = reconnect_ownership.ReconnectConnector;
pub const ReconnectFaultPlan = reconnect_ownership.ReconnectFaultPlan;
pub const ProductionConnectorCtx = reconnect_ownership.ProductionConnectorCtx;
pub const installMemoryState = reconnect_ownership.installMemoryState;
pub const destroyMemoryState = reconnect_ownership.destroyMemoryState;

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
    reconnect_timer_active: bool = false,
    reconnect_clock: clock.Clock = clock.RealClock,
    socket_owned: bool = false,
    allocator: std.mem.Allocator = std.heap.page_allocator,
    /// Production connector context. Holds the in-flight TCP transport
    /// between `acquire` and `finish`. Lives on the bundle so the
    /// connector's `ctx: ?*anyopaque` points at heap-stable memory.
    production_connector_ctx: reconnect_ownership.ProductionConnectorCtx = .{},
    /// Production connector seam. Initialised to the real default
    /// (`realAcquire` / `realFinish`) so an ordinary production bundle
    /// never panics on its first reconnect attempt.
    reconnect_connector: reconnect_ownership.ReconnectConnector = reconnect_ownership.ReconnectConnector{
        .ctx = null,
        .acquireFn = reconnect_ownership.realAcquire,
        .finishFn = reconnect_ownership.realFinish,
    },
    /// Authoritative handle-accounting state.
    reconnect_memory_state: ?*allocation_tracker.ReconnectMemoryState = null,
    /// The connector handle most recently adopted into
    /// `reconnect_memory_state`. Held until `closeForReconnect` calls
    /// `allocation_tracker.releaseHandle`. Null between generations.
    active_connector_handle: ?allocation_tracker.ReconnectHandle = null,
    reconnect_faults: ?*reconnect_ownership.ReconnectFaultPlan = null,
    // Reconnect statistics for status reporting and diagnostics
    reconnect_count: u64 = 0, // Total reconnect attempts since startup
    last_reconnect_time: clock.MonoTime = 0, // Monotonic ms of last reconnect attempt
    last_socket_error: ?[]const u8 = null, // Last TCP socket error message
    // Thread safety: Atomic u8 for cross-thread signaling.
    // cleanupBgpBundle stores 1, isCleanupRequested loads with acquire ordering.
    // This ensures the runtime thread sees the flag before bundle destruction.
    cleanup_requested: u8 = 0,

    // Runtime thread handle (null if not started or already joined)
    runtime_thread: ?std.Thread = null,

    // Export state for prefix reload + delta application
    export_state: export_reload_apply.ExportState = .{},

    // Prefix file watcher and debouncer for watched reload
    // null when no prefix files are configured
    watcher: ?prefix_watch.Watcher = null,
    debouncer: prefix_watch.Debouncer = .{},
};

// Config Loading
//
// ACT-TOVARISCH-BOUNDED-MEMORY-RECONNECT-PROOF01-FA-3 production wiring:
// this wrapper now DELEGATES to the canonical `initBgpServeBundle`
// helper after parsing the file-system inputs. The previous design had
// two parallel constructors (the wrapper + the helper) that could
// drift apart; the production daemon and the lifecycle regression
// tests now exercise the same code path.
//
// Ownership rules (FA-3 re-verdict P0):
//   * `raw`, `tcp`, and `owned_prefixes` are TRANSFERRED to
//     `initBgpServeBundle`. After this function hands them off, the
//     wrapper MUST NOT touch any of them — the constructor is the
//     sole owner and releases them on every failure branch.
//   * The `.failed` branch is a thin wrapper: we do not close
//     `tcp`, free `owned_prefixes`, or `raw.deinit(...)` here.
//     Doing so would double-free / double-close the kernel fd.
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

    const session_config = session_config_builder.buildSessionConfig(bgp_cfg, prefixes.items) catch |e| {
        stderr.print("error: failed to build BGP session config: {s}\n", .{@errorName(e)}) catch {};
        prefixes.deinit(allocator);
        raw.deinit(std.heap.page_allocator);
        return .{ .failed = .{ .message = "failed to build BGP session config" } };
    };

    session.validateConfig(session_config) catch |e| {
        stderr.print("error: invalid BGP session config: {s}\n", .{@errorName(e)}) catch {};
        prefixes.deinit(allocator);
        raw.deinit(std.heap.page_allocator);
        return .{ .failed = .{ .message = "invalid BGP session config" } };
    };

    const tcp_config = tcp_transport.TcpTransportConfig{
        .peer_address = session_config.peer_address,
        .peer_port = bgp_cfg.peer_port,
        .local_address = session_config.local_address,
        .connect_timeout_ms = bgp_cfg.connect_timeout_ms,
    };
    var tcp = tcp_transport.TcpTransport.connect(tcp_config) catch |e| {
        stderr.print("error: failed to connect to BGP peer: {s}\n", .{@errorName(e)}) catch {};
        prefixes.deinit(allocator);
        raw.deinit(std.heap.page_allocator);
        return .{ .failed = .{ .message = "BGP connect failed" } };
    };

    // FA-3 production-load ownership transfer: `prefixes.items` is a
    // VIEW into the ArrayList's internal storage; passing it directly
    // would dangle as soon as the constructor releases the list. The
    // ownership-transfer operation `toOwnedSlice(allocator)` returns
    // a slice the caller can free independently of the (now-empty)
    // ArrayList. The constructor takes ownership of the returned
    // slice via its `prefixes: []types.Ipv4Prefix` parameter.
    //
    // `toOwnedSlice` may fail (out of memory); the wrapper still owns
    // `tcp` and `raw` at this point so we close / deinit them
    // ourselves before returning.
    const owned_prefixes = prefixes.toOwnedSlice(allocator) catch {
        stderr.writeAll("error: out of memory transferring prefixes\n") catch {};
        tcp.close();
        raw.deinit(std.heap.page_allocator);
        return .{ .failed = .{ .message = "out of memory transferring prefixes" } };
    };

    // After this point: `prefixes` is empty; `owned_prefixes`, `tcp`,
    // and `raw` are transferred to the constructor and we MUST NOT
    // touch them. The constructor owns every failure-path release.
    return initBgpServeBundle(
        raw,
        bgp_cfg,
        session_config,
        &tcp,
        owned_prefixes,
        stderr,
        allocator,
    );
}

// Cleanup
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

    // Clean up prefix file watcher (closes inotify fd on Linux)
    serve_export_integration.destroyPrefixWatcher(bundle);

    // Clean up export state (frees daemon-owned prefix memory)
    bundle.export_state.deinit();

    // Tear down per-generation resources FIRST while the tracker
    // state still exists so it can record the final release,
    // socket close, and timer cancel.
    closeForReconnect(bundle);

    // Destroy the oracle LAST, after every tracked handle, socket,
    // timer, and classified allocation has been released. The
    // fail-loud helper panics if the state is still dirty at this
    // point, surfacing ownership drift as a crash instead of a
    // silent release through a corrupted oracle.
    destroyMemoryState(bundle, allocator);

    // Now safe to free the input prefixes and raw config; the bundle
    // struct itself is released last.
    allocator.free(bundle.prefixes);
    bundle.raw.deinit(std.heap.page_allocator);
    std.heap.page_allocator.destroy(bundle);
}

// Status Accessors
pub fn getBgpState(bundle: *const BgpServeBundle) BgpRuntimeState {
    return bundle.state;
}

pub fn getBgpLastError(bundle: *const BgpServeBundle) ?[]const u8 {
    return bundle.last_error;
}

pub fn getSessionStatus(bundle: *BgpServeBundle) session.SessionStatus {
    return session.getStatus(&bundle.sess);
}

// Session Execution
pub fn runSessionOnce(bundle: *BgpServeBundle) session.RunResult {
    const result = session.runOnce(&bundle.sess) catch |e| {
        bundle.last_error = if (bundle.sess.status.last_error) |session_err|
            copyErrorToBundle(bundle, session_err.message)
        else
            copyErrorToBundle(bundle, @errorName(e));
        bundle.state = .failed;
        return .failed;
    };

    // Log UPDATE using session.last_update_info captured before flush
    update_diagnostics.logUpdateFromSession(&bundle.sess);

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
    // MemoryCopySafety: message is a parameter slice, bundle.last_error_buf is a fixed [64]u8 field.
    // These buffers are independent - message content is copied into the bundle's own storage.
    @memcpy(bundle.last_error_buf[0..message.len], message);
    return bundle.last_error_buf[0..message.len];
}

// Reconnect/Backoff Lifecycle
pub const computeNextBackoff = reconnect.computeNextBackoff;

pub const scheduleReconnect = reconnect_ownership.scheduleReconnect;

pub const isReconnectReady = reconnect_ownership.isReconnectReady;

pub const resetBackoff = reconnect_ownership.resetBackoff;

pub const closeForReconnect = reconnect_ownership.closeForReconnect;

pub const reconnectTransport = reconnect_ownership.reconnectTransport;

pub const doReconnect = reconnect_ownership.doReconnect;

pub fn isCleanupRequested(bundle: *BgpServeBundle) bool {
    return @atomicLoad(u8, &bundle.cleanup_requested, .acquire) != 0;
}

// Constants Export
pub const DEFAULT_RECONNECT_INITIAL_MS = reconnect.DEFAULT_RECONNECT_INITIAL_MS;
pub const DEFAULT_RECONNECT_MAX_MS = reconnect.DEFAULT_RECONNECT_MAX_MS;
pub const DEFAULT_RECONNECT_MULTIPLIER = reconnect.DEFAULT_RECONNECT_MULTIPLIER;

/// Drive one reconnect attempt via the orchestrator. Added so the
/// `runtime.zig` worker thread can call the canonical constructor
/// boundary without duplicating the reconnect logic.
pub fn doReconnectWithClock(bundle: *BgpServeBundle, clock_interface: clock.Clock) !void {
    return reconnect_ownership.doReconnectWithClock(bundle, clock_interface);
}

/// Drive one reconnect attempt, used by the runtime worker thread.
/// Thin alias for `doReconnectWithClock` so the runtime can pin the
/// clock at the call site (matching `reconnect_ownership`).
pub fn runReconnectAttempt(bundle: *BgpServeBundle, clock_interface: clock.Clock) !void {
    return reconnect_ownership.runReconnectAttempt(bundle, clock_interface);
}

/// `initBgpServeBundle` and `releaseBundleOnFailure` moved to
/// `serve_bundle_constructor.zig` (FA-3 final file split). The
/// canonical constructor now lives in its own small module; this
/// file keeps the daemon's configuration wrapper, status
/// accessors, and lifecycle re-exports.
///
/// `initBgpServeBundle` is re-exported below so existing call
/// sites (`serve_integration.initBgpServeBundle`) continue to
/// resolve after the split.
pub const initBgpServeBundle = serve_bundle_constructor.initBgpServeBundle;

/// Cancel a pending reconnect timer. Routes to the canonical
/// lifecycle helper.
pub fn cancelReconnectTimer(bundle: *BgpServeBundle) void {
    reconnect_ownership.cancelReconnectTimer(bundle);
}
