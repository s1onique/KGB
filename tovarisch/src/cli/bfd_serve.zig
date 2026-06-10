// cli/bfd_serve.zig — BFD runtime configuration for serve command
//
// Loads config from file and creates BFD runtime for the daemon.
// Keeps config memory alive for daemon lifetime to avoid dangling slices.
// When BFD is enabled, also binds UDP 4784 and starts receive loop.

const std = @import("std");
const wg_args = @import("wg_args.zig");
const config = @import("../config.zig");
const bfd_config = @import("../bfd/config.zig");
const bfd_status = @import("../bfd/status.zig");
const bfd_transport = @import("../bfd/transport.zig");
const bfd_clock = @import("../bfd/clock.zig");
const bfd_receive = @import("../bfd/receive.zig");

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

/// Bundle that owns config memory, BFD runtime, and receive thread.
/// Thread is joinable to prevent use-after-free during cleanup.
/// Socket ownership: loop state owns the socket.
pub const BfdServeBundle = struct {
    const Self = @This();

    /// Owned config memory - must outlive runtime.
    raw: config.RawConfig,
    /// The BFD runtime - uses addresses from raw.
    runtime: bfd_status.BfdRuntime,
    /// Stop signal for receive loop thread.
    stop_signal: bfd_receive.StopSignal = .{},
    /// Peer address for discriminator learning.
    peer_addr: []const u8,
    /// Local address for discriminator learning.
    local_addr: []const u8,
    /// Receive loop state (heap-allocated, owned by this bundle).
    loop_state: ?*bfd_receive.BfdReceiveLoopState = null,
    /// Thread handle (null if not started).
    thread: ?std.Thread = null,
    /// Flag indicating BFD was configured and socket was bound.
    bfd_active: bool = false,
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

    // Bind UDP socket for BFD receive
    var socket = bfd_receive.BfdReceiveSocket.bind(bfd_receive.MULTIHOP_PORT) catch |err| {
        stderr.print("error: failed to bind UDP {d} for BFD: {s}\n", .{
            bfd_receive.MULTIHOP_PORT, @errorName(err),
        }) catch {};
        raw.deinit(std.heap.page_allocator);
        std.heap.page_allocator.destroy(bundle);
        return .failed;
    };

    bundle.peer_addr = bfd_cfg.peer_addr;
    bundle.local_addr = bfd_cfg.local_addr;

    // Transfer raw config ownership to bundle
    bundle.raw = raw;
    bundle.bfd_active = true;

    // Allocate loop state on heap
    var loop_state = std.heap.page_allocator.create(bfd_receive.BfdReceiveLoopState) catch {
        stderr.writeAll("error: out of memory for BFD receive loop\n") catch {};
        socket.close();
        raw.deinit(std.heap.page_allocator);
        std.heap.page_allocator.destroy(bundle);
        return .failed;
    };
    bundle.loop_state = loop_state;

    // Initialize loop state (socket ownership transferred)
    loop_state.* = bfd_receive.BfdReceiveLoopState{
        .runtime = &bundle.runtime,
        .socket = socket,
        .stop = &bundle.stop_signal,
        .local_addr = bundle.local_addr,
        .needs_cleanup = true,
    };

    // Spawn thread (NOT detached - we need to join it on cleanup)
    const thread = std.Thread.spawn(.{}, bfd_receive.bfdReceiveLoop, .{loop_state}) catch |err| {
        stderr.print("error: failed to start BFD receive thread: {s}\n", .{@errorName(err)}) catch {};
        // socket is owned by loop_state, close it here on failure
        loop_state.socket.close();
        std.heap.page_allocator.destroy(loop_state);
        raw.deinit(std.heap.page_allocator);
        std.heap.page_allocator.destroy(bundle);
        return .failed;
    };
    bundle.thread = thread;

    return .{ .configured = bundle };
}

/// Clean up a BFD bundle when shutting down.
/// Thread-safe cleanup order:
/// 1. Signal stop flag
/// 2. Close socket to wake recvfrom
/// 3. Join thread
/// 4. Destroy loop state
/// 5. Deinit raw config
/// 6. Destroy bundle
pub fn cleanupBfdBundle(bundle: *BfdServeBundle) void {
    // 1. Signal receive loop to stop
    bundle.stop_signal.store();

    // 2. If BFD was active, close socket and join thread
    if (bundle.bfd_active) {
        // Close socket to wake recvfrom (non-blocking will return)
        if (bundle.loop_state) |state| {
            state.socket.close();
        }

        // 3. Join thread (blocks until loop exits)
        if (bundle.thread) |thread| {
            thread.join();
        }

        // 4. Destroy loop state
        if (bundle.loop_state) |state| {
            std.heap.page_allocator.destroy(state);
            bundle.loop_state = null;
        }
    }

    // 5. Deinit raw config
    bundle.raw.deinit(std.heap.page_allocator);

    // 6. Destroy bundle
    std.heap.page_allocator.destroy(bundle);
}
