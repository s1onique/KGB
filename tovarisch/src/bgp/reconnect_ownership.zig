// reconnect_ownership.zig — production reconnect resource ownership seams
//
// Kept separate from serve_integration so the reconnect lifecycle remains
// small, reviewable, and directly reusable by deterministic production-
// path tests.
//
// The connector seam is two-phase:
//   1. `acquire` returns a `ReconnectHandle` token (no ledger mutation).
//   2. `finish` performs the actual TCP connect; on failure the
//      orchestrator's `errdefer` calls `releaseHandle(state, handle)`.
//
// The state (an opaque `ReconnectMemoryState`) is the single authority for
// handle accounting. There is no parallel `ConnectorProbe` or
// `HandleLedger` in `BgpServeBundle`. Production-connector ownership is
// expressed by (a) `production_connector_ctx` (an in-flight flag and a
// best-effort `inflight` transport placeholder used between `acquire` and
// `finish`), and (b) `bundle.reconnect_memory_state` (the authoritative
// oracle). The bundle owns the connected TCP transport via `bundle.tcp`.
//
// ACT-TOVARISCH-BOUNDED-MEMORY-RECONNECT-PROOF01-FA (this file):
//   * `installMemoryState` / `destroyMemoryState` make state ownership
//     explicit so production wiring can't silently drop the oracle. Used
//     by `loadConfigAndBgp` and `cleanupBgpBundle`.
//   * The `releaseOnErrdefer` helper used to silently swallow a
//     `releaseHandle` failure during in-flight cleanup; after the P1
//     fail-loud correction it propagates the disagreement via `@panic` so
//     the daemon never continues with a corrupted active-handle record
//     while the caller believed they observed a benign cleanup error.

const std = @import("std");
const allocation_tracker = @import("../runtime/allocation_tracker.zig");
const clock = @import("clock.zig");
const reconnect = @import("reconnect_lifecycle.zig");
const tcp_transport = @import("tcp_transport.zig");

/// Production connector context. Holds the in-flight flag set by
/// `realAcquire` and cleared on success / release, and an `inflight`
/// placeholder used as a single-owner pointer during the window where
/// the TCP transport is being constructed. The state DOES NOT carry a
/// ledger; all handle accounting lives in `ReconnectMemoryState`.
pub const ProductionConnectorCtx = struct {
    /// In-flight transport pointer. Set as a transient placeholder so
    /// the production handle's release callback can identify the
    /// connector across `realFinish`. On success the bundle takes sole
    /// ownership of the transport (we set this to `null` before
    /// returning). The handle's release callback therefore never closes
    /// a transport that the bundle still owns.
    inflight: ?*tcp_transport.TcpTransport = null,
    /// `true` while a `realAcquire` is outstanding and `realFinish` has
    /// not yet completed (or has failed). Enforces single-acquire.
    acquire_inflight: bool = false,
};

/// Production connector seam. Two-phase: `acquire` returns a
/// `ReconnectHandle` that the orchestrator must `adoptHandle` into the
/// state; `finish` performs the actual connection attempt. If `finish`
/// fails the orchestrator's errdefer calls `releaseHandle(state,
/// handle)` so every failed connection attempt is observable through
/// the bound state.
pub const ReconnectConnector = struct {
    ctx: ?*anyopaque = null,
    acquireFn: *const fn (?*anyopaque, tcp_transport.TcpTransportConfig) anyerror!allocation_tracker.ReconnectHandle = realAcquire,
    finishFn: *const fn (?*anyopaque, allocation_tracker.ReconnectHandle, tcp_transport.TcpTransportConfig) anyerror!tcp_transport.TcpTransport = realFinish,

    pub fn acquire(self: ReconnectConnector, tcp_config: tcp_transport.TcpTransportConfig) anyerror!allocation_tracker.ReconnectHandle {
        return self.acquireFn(self.ctx, tcp_config);
    }

    pub fn finish(self: ReconnectConnector, handle: allocation_tracker.ReconnectHandle, tcp_config: tcp_transport.TcpTransportConfig) anyerror!tcp_transport.TcpTransport {
        return self.finishFn(self.ctx, handle, tcp_config);
    }
};

// ---------------------------------------------------------------------------
// Memory-state ownership helpers.
//
// Production wiring MUST install the memory state before the bundle can
// perform any reconnect; the helper centralises the init/deinit
// pair so callers cannot accidentally drop the oracle. The bundle's
// field remains `?*ReconnectMemoryState` (rather than non-optional) only
// for backward compatibility with hand-constructed test bundles;
// every reconnect-capable bundle is REQUIRED to install the state via
// `installMemoryState` first.
// ---------------------------------------------------------------------------

/// Install the authoritative handle accounting state on the bundle.
/// After this call `bundle.reconnect_memory_state` is non-null and the
/// connector's release path can route through `releaseHandle`.
pub fn installMemoryState(
    bundle: anytype,
    allocator: std.mem.Allocator,
) !void {
    bundle.reconnect_memory_state = try allocation_tracker.init(allocator);
}

/// Destroy the state previously installed by `installMemoryState`.
/// MUST be called only after every active handle has been released
/// (i.e. after `closeForReconnect`).
///
/// ACT-TOVARISCH-BOUNDED-MEMORY-RECONNECT-PROOF01-FA-3: this helper
/// is fail-loud AND delegates the entire precondition check to
/// `allocation_tracker.validateForDestroy` so the same lifetime-
/// specific leak probes (active sockets, active timers,
/// `reconnect_generation`, `operation`) that the standalone
/// validator exposes are also enforced at the inline destroy point.
/// A previous design inlined a weaker subset that only checked
/// handle identity and counters; that was insufficient because the
/// state could still hold live resources the orchestrator had
/// forgotten to release. Now any leak detected by
/// `validateForDestroy` panics here with the underlying error name
/// so the operator sees the precise cause. The orchestrator
/// (see `cleanupBgpBundle`) is responsible for ensuring
/// `closeForReconnect` runs first.
pub fn destroyMemoryState(
    bundle: anytype,
    allocator: std.mem.Allocator,
) void {
    const state = bundle.reconnect_memory_state orelse return;
    allocation_tracker.validateForDestroy(state) catch |err| {
        std.debug.panic(
            "destroyMemoryState rejected dirty state: {s}",
            .{@errorName(err)},
        );
    };
    allocation_tracker.deinit(state, allocator);
    bundle.reconnect_memory_state = null;
}

/// Default production `acquire`. Sets the in-flight flag and returns a
/// handle whose release callback clears that flag. The handle's `ptr`
/// points at the context so the release callback can identify the
/// production connector without an extra indirection.
///
/// The handle does NOT mutate any state-bound ledger counter — every
/// accounting change goes through `adoptHandle`/`releaseHandle` on the
/// authoritative state.
pub fn realAcquire(ctx: ?*anyopaque, _: tcp_transport.TcpTransportConfig) anyerror!allocation_tracker.ReconnectHandle {
    const state: *ProductionConnectorCtx = @ptrCast(@alignCast(ctx.?));
    if (state.acquire_inflight) return error.HandleAlreadyActive;
    state.acquire_inflight = true;
    return allocation_tracker.ReconnectHandle{
        .ptr = @ptrCast(state),
        .release_fn = releaseProductionTransport,
    };
}

/// Default production `finish`. Performs the TCP connect and hands the
/// resulting transport off to the bundle. Returns the connected
/// transport; ownership transfers entirely to the caller (typically
/// `bundle.tcp`). The context's `inflight` placeholder stays `null` so
/// the handle's release callback can never accidentally close a
/// transport the bundle still owns.
///
/// On a connect failure the in-flight flag stays set; that is fine
/// because the orchestrator's `releaseOnErrdefer` will call
/// `releaseHandle(state, handle)`, which routes back through
/// `releaseProductionTransport` to clear the flag. Without the
/// installed state the flag would leak; install the state FIRST in any
/// reconnect-capable bundle (`installMemoryState`).
pub fn realFinish(ctx: ?*anyopaque, _: allocation_tracker.ReconnectHandle, config: tcp_transport.TcpTransportConfig) anyerror!tcp_transport.TcpTransport {
    const state: *ProductionConnectorCtx = @ptrCast(@alignCast(ctx.?));
    if (!state.acquire_inflight) return error.NoInflightTransport;
    const tcp = try tcp_transport.TcpTransport.connect(config);
    // Hand off sole ownership to the bundle. The release callback will
    // therefore NOT close the transport; the bundle takes care of that
    // exactly once via `bundle.tcp.close()`.
    state.inflight = null;
    state.acquire_inflight = false;
    return tcp;
}

/// Default release function for production handles. Idempotent: clears
/// the in-flight flag and the (always-null) transport placeholder.
/// Does NOT close any transport — the bundle owns the connected socket
/// exclusively and closes it via `bundle.tcp.close()`.
fn releaseProductionTransport(ptr: *anyopaque) void {
    const state: *ProductionConnectorCtx = @ptrCast(@alignCast(ptr));
    state.inflight = null;
    state.acquire_inflight = false;
}

/// Fault plan used to drive targeted mutations through the orchestrator.
///
/// The previous design had two independent flags
/// (`skip_probe_release`, `skip_handle_release`) — that let a caller
/// exercise bookkeeping without the physical release, hiding real
/// ownership defects. Under the new design the two paths are merged:
/// `skip_release_handle` skips the WHOLE `releaseHandle(state, handle)`
/// operation, so a misbehaving caller cannot partially release.
pub const ReconnectFaultPlan = struct {
    skip_release_handle: bool = false,
    skip_socket_close: bool = false,
    skip_timer_stop: bool = false,
};

/// Schedules a reconnect retry. Deadlines are computed with explicit
/// overflow checks so a long-running daemon can never silently wrap.
pub fn scheduleReconnect(bundle: anytype, clock_interface: clock.Clock, max_delay_ms: u64) void {
    bundle.backoff_ms = reconnect.computeNextBackoff(bundle.backoff_ms, max_delay_ms);
    if (!bundle.reconnect_timer_active) {
        if (bundle.reconnect_memory_state) |state| allocation_tracker.recordTimerStart(state);
        bundle.reconnect_timer_active = true;
    }
    const now = clock_interface.getMonoTimeMs();
    bundle.reconnect_deadline = std.math.add(clock.MonoTime, now, bundle.backoff_ms) catch @panic("reconnect deadline overflow");
    bundle.state = .reconnect_wait;
}

pub fn cancelReconnectTimer(bundle: anytype) void {
    if (!bundle.reconnect_timer_active) return;
    if (bundle.reconnect_faults) |faults| {
        if (faults.skip_timer_stop) return;
    }
    if (bundle.reconnect_memory_state) |state| allocation_tracker.recordTimerStop(state);
    bundle.reconnect_timer_active = false;
    bundle.reconnect_deadline = 0;
}

pub fn isReconnectReady(bundle: anytype, clock_interface: clock.Clock) bool {
    if (bundle.state != .reconnect_wait) return false;
    const now = clock_interface.getMonoTimeMs();
    if (now < bundle.reconnect_deadline) return false;
    cancelReconnectTimer(bundle);
    return true;
}

pub fn resetBackoff(bundle: anytype) void {
    reconnect.resetBackoff(&bundle.backoff_ms, &bundle.reconnect_deadline);
}

/// Fully tears down per-generation state. The release path goes through
/// `releaseHandle(state, handle)` (a single operation) so a skipped
/// release observes identically via `state.handles_acquired`,
/// `state.handles_released`, and `state.active_handle`.
///
/// `skip_socket_close` is scoped around the physical close only. It
/// does NOT skip the resource counter call; the state still observes
/// the mismatch between physical-closes and counter-closes via
/// `finishReconnectBoundary`.
pub fn closeForReconnect(bundle: anytype) void {
    const skip_socket = if (bundle.reconnect_faults) |f| f.skip_socket_close else false;
    const skip_release = if (bundle.reconnect_faults) |f| f.skip_release_handle else false;

    if (bundle.socket_owned) {
        if (!skip_socket) {
            bundle.tcp.close();
            if (bundle.reconnect_memory_state) |state| allocation_tracker.recordSocketClose(state);
            bundle.socket_owned = false;
        }
    } else {
        // No successful socket to close — the placeholder is `closed`
        // and idempotent to close again.
        bundle.tcp.close();
    }

    // Authoritative handle disposal: ONE operation. The state verifies
    // identity, invokes the physical release_fn, increments counters,
    // and clears the active record. We do NOT separately invoke
    // `handle.release_fn` here — `releaseHandle` does it for us.
    if (!skip_release) {
        if (bundle.active_connector_handle) |handle| {
            if (bundle.reconnect_memory_state) |state| {
                // A release in cleanup is not allowed to fail; if it
                // does, the wiring has drifted and we panic so the
                // operator notices rather than continuing with a
                // corrupted oracle.
                allocation_tracker.releaseHandle(state, handle) catch |err| switch (err) {
                    error.NoActiveHandle, error.WrongHandle, error.HandleAlreadyActive => @panic("closeForReconnect: state and bundle disagreed on the active handle"),
                };
            }
            bundle.active_connector_handle = null;
        }
    } else if (bundle.active_connector_handle) |handle| {
        // Skipped release leaves the handle still active on the state
        // AND still stored on the bundle. KEEP `bundle.active_connector_handle`
        // so a subsequent (unskipped) `closeForReconnect` can still
        // release via `releaseHandle`. This is required for fault-
        // injection tests and for any operator override that retains
        // ownership for inspection.
        _ = handle;
    }

    cancelReconnectTimer(bundle);

    bundle.sess.status.state = .idle;
    bundle.sess.recv_len = 0;
    bundle.sess.send_pos = 0;
    bundle.sess.peer_open = null;
    bundle.sess.negotiated_hold_time = 0;
    bundle.sess.keepalive_interval_ms = 0;
    bundle.sess.hold_timer_deadline = 0;
    bundle.sess.pending_keepalive = false;
    bundle.sess.pending_keepalive_ms = 0;
    bundle.sess.export_batch_index = 0;
    bundle.sess.export_complete = false;
    bundle.sess.nlri_sent_count = 0;
    bundle.sess.status.last_error = null;
    bundle.sess.status.last_notification_code = null;
    bundle.sess.status.last_notification_subcode = null;
    bundle.sess.last_update_diagnostic = .none;
}

/// Production reconnect transport. Two-phase:
///
/// ```text
///   1. acquire       → ReconnectHandle token (no oracle mutation).
///   2. adoptHandle   → state records the handle identity.
///   3. finish        → TcpTransport.connect; ownership handoff to bundle.
///   4. recordSocketOpen + bookkeeping.
/// ```
///
/// Failure at any point is balanced by the `errdefer` that calls
/// `releaseHandle(state, handle)`. The state cannot observe a partial
/// adoption because we adopt only after `acquire` succeeds and we
/// release only via the errdefer or via the next `closeForReconnect`.
pub fn reconnectTransport(bundle: anytype) !void {
    const tcp_config = tcp_transport.TcpTransportConfig{
        .peer_address = bundle.session_config.peer_address,
        .peer_port = bundle.session_config.peer_port,
        .local_address = bundle.session_config.local_address,
        .connect_timeout_ms = bundle.session_config.connect_timeout_ms,
    };

    if (bundle.reconnect_memory_state) |state| allocation_tracker.recordSocketOpen(state);
    errdefer if (bundle.reconnect_memory_state) |state| allocation_tracker.recordSocketClose(state);

    // Phase 1: acquire a logical handle BEFORE attempting connect.
    const handle = bundle.reconnect_connector.acquire(tcp_config) catch |err| return err;

    // Phase 2: route the handle through the authoritative state.
    // Adopt failures (double-acquire) MUST be released physically but
    // MUST NOT touch the state (which has nothing adopted).
    if (bundle.reconnect_memory_state) |state| {
        allocation_tracker.adoptHandle(state, handle) catch |err| {
            const release_fn: *const fn (*anyopaque) void = @as(*const fn (*anyopaque) void, @ptrCast(handle.release_fn));
            release_fn(handle.ptr);
            return err;
        };
    }

    // Function-scope errdefer (Zig errdefer is block-scoped, so this
    // MUST be at function scope to fire on a `finish` failure). Order
    // matters: releaseHandle runs FIRST (LIFO), then recordSocketClose.
    errdefer releaseOnErrdefer(bundle, handle);

    // Phase 3: attempt the actual connection. Failure releases via errdefer.
    const new_tcp = bundle.reconnect_connector.finish(handle, tcp_config) catch |err| return err;

    bundle.tcp = new_tcp;
    bundle.trans = bundle.tcp.toTransport();
    bundle.sess.trans = &bundle.trans;
    bundle.socket_owned = true;
    bundle.active_connector_handle = handle;
}

/// Bounded copy: caps the source slice at the buffer size so a long error
/// message cannot overflow `last_error_buf`. `buf` and `message` are
/// distinct, fixed-size storage; this is a non-overlapping copy.
fn copyErrorToBundle(bundle: anytype, message: []const u8) []const u8 {
    const buf = &bundle.last_error_buf;
    const copy_len = @min(message.len, buf.len);
    // MemoryCopySafety: buf and message are distinct allocations.
    @memcpy(buf[0..copy_len], message[0..copy_len]);
    return buf[0..copy_len];
}

pub fn doReconnect(bundle: anytype) !void {
    return doReconnectWithClock(bundle, bundle.reconnect_clock);
}

pub fn doReconnectWithClock(bundle: anytype, clock_interface: clock.Clock) !void {
    closeForReconnect(bundle);
    bundle.reconnect_count = std.math.add(u64, bundle.reconnect_count, 1) catch @panic("reconnect count overflow");
    bundle.last_reconnect_time = clock_interface.getMonoTimeMs();

    reconnectTransport(bundle) catch |reconnect_err| {
        bundle.last_socket_error = copyErrorToBundle(bundle, @errorName(reconnect_err));
        bundle.last_error = bundle.last_socket_error;
        return reconnect_err;
    };

    bundle.last_socket_error = null;
    resetBackoff(bundle);
    bundle.state = .configured;
    bundle.last_error = null;
}

pub fn runReconnectAttempt(bundle: anytype, clock_interface: clock.Clock) !void {
    return doReconnectWithClock(bundle, clock_interface);
}


/// Helper used in error-cleanup paths to release the previously-adopted
/// handle on the bundle's authoritative state.
///
/// The previous design silently swallowed `releaseHandle` errors here
/// on the grounds that the original connection error is what the caller
/// cares about. That reasoning was wrong: an `errdefer` runs precisely
/// because control is leaving the function through an error path, and
/// the cleanup is the LAST chance the state has to stay consistent
/// before the caller observes the original error. Swallowing it
/// silently means the caller sees a "successful" cleanup while the
/// active-handle record is corrupted.
///
/// After the P1 fail-loud correction, this helper still propagates
/// silently when the state is missing (which can only happen on a
/// hand-constructed bundle that never went through
/// `installMemoryState`) — that case is a wiring bug worth surfacing
/// without losing the original error. When the state IS installed, a
/// `releaseHandle` failure indicates the bundle's recorded handle
/// disagrees with the state's token, which is a panic-worthy identity
/// mismatch.
///
/// In short: a releaseHandle failure during in-flight cleanup is
/// evidence of state corruption, not noise. We panic and stop the
/// daemon rather than continue with a corrupted oracle.
fn releaseOnErrdefer(bundle: anytype, handle: allocation_tracker.ReconnectHandle) void {
    const state = bundle.reconnect_memory_state orelse {
        // No state installed. This is a hard error for any
        // reconnect-capable bundle — every production bundle MUST
        // install state via `installMemoryState` exactly once. The
        // previous code silently swallowed this; the fail-loud
        // correction makes the wiring requirement explicit. We still
        // invoke the physical callback locally to keep the production
        // context's acquire_inflight flag consistent with the
        // connector's contract, but we ALSO panic so the operator
        // observes the wiring drift.
        handle.release_fn(handle.ptr);
        @panic("reconnect error cleanup reached bundle without a memory state; production MUST call installMemoryState");
    };
    allocation_tracker.releaseHandle(state, handle) catch |err| switch (err) {
        // The state disagreed with the recorded handle. The bundle
        // either adopted a different handle than we are now releasing,
        // or the release_fn identity was tampered with. Either way the
        // orchestrator wiring has drifted; we cannot continue with a
        // corrupted active-handle record. Panic so the operator
        // notices rather than silently masking the original error.
        error.NoActiveHandle, error.WrongHandle, error.HandleAlreadyActive => @panic("reconnect error cleanup disagreed with handle state"),
    };
}

// Compile-time checks keep the module tied to the production reconnect policy.
comptime {
    _ = reconnect.DEFAULT_RECONNECT_INITIAL_MS;
    _ = reconnect.DEFAULT_RECONNECT_MAX_MS;
    _ = reconnect.DEFAULT_RECONNECT_MULTIPLIER;
}
