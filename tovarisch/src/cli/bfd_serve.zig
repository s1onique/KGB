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
const bfd_transmit = @import("../bfd/transmit.zig");

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

/// Bundle that owns config memory, BFD runtime, and both BFD threads.
/// Threads are joinable to prevent use-after-free during cleanup.
/// Socket ownership: receive loop state owns the socket.
pub const BfdServeBundle = struct {
    const Self = @This();

    /// Owned config memory - must outlive runtime.
    raw: config.RawConfig,
    /// The BFD runtime - uses addresses from raw.
    runtime: bfd_status.BfdRuntime,
    /// Stop signal shared between receive and transmit loops.
    stop_signal: bfd_receive.StopSignal = .{},
    /// Peer address for discriminator learning.
    peer_addr: []const u8,
    /// Local address for discriminator learning.
    local_addr: []const u8,
    /// Receive loop state (heap-allocated, owned by this bundle).
    loop_state: ?*bfd_receive.BfdReceiveLoopState = null,
    /// Transmit loop state (heap-allocated, owned by this bundle).
    transmit_loop_state: ?*bfd_transmit.BfdTransmitLoopState = null,
    /// Receive thread handle (null if not started).
    receive_thread: ?std.Thread = null,
    /// Transmit thread handle (null if not started).
    transmit_thread: ?std.Thread = null,
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
    var runtime = bfd_status.BfdRuntime.init(
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
    runtime.addPeer(session_cfg) catch |e| {
        stderr.print("error: failed to add BFD peer: {s}\n", .{@errorName(e)}) catch {};
        raw.deinit(std.heap.page_allocator);
        std.heap.page_allocator.destroy(bundle);
        return .failed;
    };

    // Start all sessions
    runtime.startAll();

    // Bind UDP socket for BFD receive
    var socket = bfd_receive.BfdReceiveSocket.bind(bfd_receive.MULTIHOP_PORT) catch |err| {
        stderr.print("error: failed to bind UDP {d} for BFD: {s}\n", .{
            bfd_receive.MULTIHOP_PORT, @errorName(err),
        }) catch {};
        raw.deinit(std.heap.page_allocator);
        std.heap.page_allocator.destroy(bundle);
        return .failed;
    };

    // Transfer raw config ownership to bundle
    // Use full struct literal to ensure all fields are deterministically initialized.
    // This fixes the bug where raw heap allocation left stop_signal.flag undefined,
    // causing the receive loop to exit immediately and close the socket.
    bundle.* = BfdServeBundle{
        .raw = raw,
        .runtime = runtime,
        .stop_signal = .{},
        .peer_addr = bfd_cfg.peer_addr,
        .local_addr = bfd_cfg.local_addr,
        .loop_state = null,
        .transmit_loop_state = null,
        .receive_thread = null,
        .transmit_thread = null,
        .bfd_active = true,
    };

    // Allocate receive loop state on heap
    var loop_state = std.heap.page_allocator.create(bfd_receive.BfdReceiveLoopState) catch {
        stderr.writeAll("error: out of memory for BFD receive loop\n") catch {};
        socket.close();
        raw.deinit(std.heap.page_allocator);
        std.heap.page_allocator.destroy(bundle);
        return .failed;
    };
    bundle.loop_state = loop_state;

    // Initialize receive loop state (socket ownership transferred)
    loop_state.* = bfd_receive.BfdReceiveLoopState{
        .runtime = &bundle.runtime,
        .socket = socket,
        .stop = &bundle.stop_signal,
        .local_addr = bundle.local_addr,
        .needs_cleanup = true,
    };

    // Spawn receive thread (NOT detached - we need to join it on cleanup)
    const receive_thread = std.Thread.spawn(.{}, bfd_receive.bfdReceiveLoop, .{loop_state}) catch |err| {
        stderr.print("error: failed to start BFD receive thread: {s}\n", .{@errorName(err)}) catch {};
        // socket is owned by loop_state, close it here on failure
        loop_state.socket.close();
        std.heap.page_allocator.destroy(loop_state);
        raw.deinit(std.heap.page_allocator);
        std.heap.page_allocator.destroy(bundle);
        return .failed;
    };
    bundle.receive_thread = receive_thread;

    // Allocate transmit loop state on heap
    const transmit_loop_state = std.heap.page_allocator.create(bfd_transmit.BfdTransmitLoopState) catch |err| {
        stderr.print("error: out of memory for BFD transmit loop: {s}\n", .{@errorName(err)}) catch {};
        // Clean up receive thread before destroying bundle - it holds pointers into bundle.runtime
        cleanupStartedReceiveThread(bundle, loop_state);
        raw.deinit(std.heap.page_allocator);
        std.heap.page_allocator.destroy(bundle);
        return .failed;
    };
    bundle.transmit_loop_state = transmit_loop_state;

    // Initialize transmit loop state
    transmit_loop_state.* = bfd_transmit.BfdTransmitLoopState{
        .runtime = &bundle.runtime,
        .stop = &bundle.stop_signal,
        .tick_interval_ms = bfd_transmit.DEFAULT_TICK_INTERVAL_MS,
        .needs_cleanup = true,
    };

    // Spawn transmit thread (NOT detached - we need to join it on cleanup)
    const transmit_thread = std.Thread.spawn(.{}, bfd_transmit.bfdTransmitLoop, .{transmit_loop_state}) catch |err| {
        stderr.print("error: failed to start BFD transmit thread: {s}\n", .{@errorName(err)}) catch {};
        // Clean up receive thread before destroying bundle - it holds pointers into bundle.runtime
        cleanupStartedReceiveThread(bundle, loop_state);
        std.heap.page_allocator.destroy(transmit_loop_state);
        raw.deinit(std.heap.page_allocator);
        std.heap.page_allocator.destroy(bundle);
        return .failed;
    };
    bundle.transmit_thread = transmit_thread;

    return .{ .configured = bundle };
}

/// Clean up a receive thread that has been started but needs to be aborted.
/// This is called from error paths after receive_thread has been assigned
/// but before the bundle is fully initialized.
///
/// SAFETY: This function must be called while bundle and loop_state are still valid.
/// The receive thread holds pointers into bundle.runtime and bundle.stop_signal,
/// so we must stop it before destroying those structures.
fn cleanupStartedReceiveThread(bundle: *BfdServeBundle, loop_state: *bfd_receive.BfdReceiveLoopState) void {
    // Signal receive loop to stop
    bundle.stop_signal.store();
    // Close socket to wake recvfrom (non-blocking will return)
    loop_state.socket.close();
    // Join thread (blocks until loop exits)
    bundle.receive_thread.?.join();
    // Destroy receive loop state
    std.heap.page_allocator.destroy(loop_state);
    bundle.loop_state = null;
}

/// Clean up a BFD bundle when shutting down.
/// Thread-safe cleanup order:
/// 1. Signal stop flag (both loops check this)
/// 2. Close socket to wake recvfrom
/// 3. Join receive thread
/// 4. Join transmit thread
/// 5. Destroy receive loop state
/// 6. Destroy transmit loop state
/// 7. Deinit raw config
/// 8. Destroy bundle
pub fn cleanupBfdBundle(bundle: *BfdServeBundle) void {
    // 1. Signal both loops to stop
    bundle.stop_signal.store();

    // 2. If BFD was active, close socket and join threads
    if (bundle.bfd_active) {
        // Close socket to wake recvfrom (non-blocking will return)
        if (bundle.loop_state) |state| {
            state.socket.close();
        }

        // 3. Join receive thread (blocks until loop exits)
        if (bundle.receive_thread) |thread| {
            thread.join();
        }

        // 4. Join transmit thread (blocks until loop exits)
        if (bundle.transmit_thread) |thread| {
            thread.join();
        }

        // 5. Destroy receive loop state
        if (bundle.loop_state) |state| {
            std.heap.page_allocator.destroy(state);
            bundle.loop_state = null;
        }

        // 6. Destroy transmit loop state
        if (bundle.transmit_loop_state) |state| {
            std.heap.page_allocator.destroy(state);
            bundle.transmit_loop_state = null;
        }
    }

    // 7. Deinit raw config
    bundle.raw.deinit(std.heap.page_allocator);

    // 8. Destroy bundle
    std.heap.page_allocator.destroy(bundle);
}
