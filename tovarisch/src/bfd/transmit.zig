// transmit.zig — BFD transmit scheduler for multihop sessions
//
// Implements a daemon-owned periodic transmit loop that calls runtime.tick()
// to send BFD control packets at negotiated intervals. This enables the daemon
// to both respond to peer packets AND proactively send periodic BFD control
// packets per RFC 5880.
//
// The transmit loop runs in a separate thread, periodically calling tick()
// on the BFD runtime. The tick() method handles:
// - Detection timeout processing
// - Transmit scheduling (isTransmitDue)
// - Building and sending BFD control packets
//
// Linux-only (tovarisch targets Linux).

const std = @import("std");
const c = std.c;
const runtime = @import("runtime.zig");
const receive = @import("receive.zig");

// Re-export StopSignal from receive.zig for compatibility
// Both loops share the same stop signal type
pub const StopSignal = receive.StopSignal;

/// Default tick interval in milliseconds.
/// This is the polling interval for checking if any session needs to transmit.
/// Actual transmit intervals are negotiated per-session.
pub const DEFAULT_TICK_INTERVAL_MS: u32 = 100;

/// Transmit loop state passed to the transmit thread.
pub const BfdTransmitLoopState = struct {
    const Self = @This();

    /// Pointer to the BFD runtime to tick.
    runtime: *runtime.BfdRuntime,
    /// Stop signal (set to true to stop the loop).
    /// Uses the same StopSignal type as receive.zig for shared state.
    stop: *receive.StopSignal,
    /// Tick interval in milliseconds.
    tick_interval_ms: u32 = DEFAULT_TICK_INTERVAL_MS,
    /// Flag indicating if state needs cleanup.
    needs_cleanup: bool = true,
};

/// Build a timespec for nanosleep from milliseconds.
/// Handles cross-platform differences between Linux (.tv_sec/.tv_nsec)
/// and macOS (.sec/.nsec).
fn makeTimespec(ms: u32) c.timespec {
    const sec = @as(c_long, @intCast(ms / 1000));
    const nsec = @as(c_long, @intCast((ms % 1000) * 1_000_000));
    
    // Detect field names: Zig 0.16 stable uses .sec/.nsec on macOS
    if (@hasDecl(c.timespec, "tv_sec")) {
        var ts: c.timespec = undefined;
        ts.tv_sec = sec;
        ts.tv_nsec = nsec;
        return ts;
    } else {
        var ts: c.timespec = undefined;
        ts.sec = sec;
        ts.nsec = nsec;
        return ts;
    }
}

/// BFD transmit loop function.
/// Runs in a separate thread, periodically calls runtime.tick() to send
/// BFD control packets at negotiated intervals.
///
/// This enables the daemon to:
/// - Respond to peer's BFD session establishment
/// - Proactively send periodic BFD control packets
/// - Process detection timeouts
pub fn bfdTransmitLoop(state: *BfdTransmitLoopState) void {
    // Diagnostic: BFD tick loop started
    std.debug.print("[BFD] bfd_tick_started interval_ms={d}\n", .{state.tick_interval_ms});

    // Verify stop_signal is false at startup
    if (state.stop.load()) {
        std.debug.print("[BFD] ERROR: transmit loop started with stop_signal already set\n", .{});
        return;
    }

    // Build sleep timespec for nanosleep
    const sleep_ts = makeTimespec(state.tick_interval_ms);

    while (!state.stop.load()) {
        // Call runtime.tick() to process detection timeouts and send due packets
        state.runtime.tick() catch {
            // On error, yield and retry
            std.debug.print("[BFD] bfd_control_packet_send_failed reason=tick_error\n", .{});
            std.Thread.yield() catch {};
            _ = c.nanosleep(&sleep_ts, null);
            continue;
        };

        // Sleep until next tick
        _ = c.nanosleep(&sleep_ts, null);
    }

    std.debug.print("[BFD] bfd_tick_stopped\n", .{});
}

// ============================================================================
// Tests
// ============================================================================

test "BfdTransmitLoopState default values" {
    const state = BfdTransmitLoopState{
        .runtime = undefined,
        .stop = undefined,
    };
    try std.testing.expectEqual(@as(u32, DEFAULT_TICK_INTERVAL_MS), state.tick_interval_ms);
    try std.testing.expect(state.needs_cleanup);
}

test "StopSignal store and load" {
    var signal = StopSignal{};
    try std.testing.expect(!signal.load());
    signal.store();
    try std.testing.expect(signal.load());
}
