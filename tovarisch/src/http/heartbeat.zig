/// Heartbeat thread for tovarisch.
/// 
/// Implements a daemon-lifetime thread that emits heartbeat logs every 30 seconds
/// independently of HTTP request traffic. Uses std.c.nanosleep for blocking
/// sleep (Zig 0.16 std.Thread does not expose a sleep function).
///
/// Design note: The heartbeat thread owns its state locally. No mutex, no shared
/// context, no @constCast. For a decorative heartbeat, this is the correct
/// engineering choice - the thread runs until process exit with no coupling
/// to the main thread's lifecycle.
///
/// Tunnel metrics: Heartbeat collects interface stats directly using the same
/// interface enumeration as /metrics.json, then aggregates tunnel counters.
/// This ensures heartbeat logs are consistent with metrics output.

const std = @import("std");
const c = std.c;
const heartbeat_emit = @import("heartbeat_emit.zig");
const idle_telemetry = @import("../runtime/idle_telemetry.zig");

/// Heartbeat thread configuration.
pub const HEARTBEAT_INTERVAL_SECS: u64 = 30;

/// Re-export heartbeat_emit types for backwards compatibility.
pub const HeartbeatWriter = heartbeat_emit.HeartbeatWriter;
pub const TunnelSummary = heartbeat_emit.TunnelSummary;
pub const TunnelSummaryWithStats = heartbeat_emit.TunnelSummaryWithStats;
pub const collectTunnelSummary = heartbeat_emit.collectTunnelSummary;
pub const collectTunnelSummaryWithStats = heartbeat_emit.collectTunnelSummaryWithStats;
pub const freeTunnelSummarySnapshots = heartbeat_emit.freeTunnelSummarySnapshots;

/// Heartbeat thread entry point.
///
/// Loops every HEARTBEAT_INTERVAL_SECS and emits heartbeat logs.
/// Uses std.c.nanosleep for cross-platform blocking sleep (Zig 0.16 doesn't have std.Thread.sleep).
///
/// This function owns all its state locally:
/// - uptime_seconds is a local u64 counter
/// - No mutex needed (thread doesn't share mutable state)
/// - No @constCast needed (no const context from spawn)
///
/// Design rationale: For a decorative heartbeat with daemon-lifetime, shared
/// mutable state adds complexity without benefit. The thread runs until process
/// exit; local state is sufficient.
pub fn heartbeatThread() void {
    heartbeatThreadWithEvents(null);
}

/// Heartbeat thread entry point with native event emission support.
///
/// When lab_emitter is provided and enabled, emits native events around
/// the heartbeat tick for idle staircase memory lab attribution.
pub fn heartbeatThreadWithEvents(lab_emitter: ?*anyopaque) void {
    var uptime_seconds: u64 = 0;

    while (true) {
        // Sleep for HEARTBEAT_INTERVAL_SECS using libc nanosleep.
        // Zig 0.16 std.Thread does not expose a sleep function.
        // std.c.nanosleep is the portable blocking sleep API.
        // Note: Zig 0.16 c.timespec uses .sec/.nsec, not .tv_sec/.tv_nsec
        var ts: c.timespec = .{
            .sec = @intCast(HEARTBEAT_INTERVAL_SECS),
            .nsec = 0,
        };
        _ = c.nanosleep(&ts, null);

        // Increment uptime (local state, no mutex needed)
        uptime_seconds += HEARTBEAT_INTERVAL_SECS;

        // Increment heartbeat tick counter for memory attribution
        // This is called every HEARTBEAT_INTERVAL_SECS (30s)
        idle_telemetry.incrementHeartbeatTicks();

        // Convert uptime to milliseconds for native event emission
        const elapsed_millis = @as(u32, @intCast(uptime_seconds * 1000));

        // Emit heartbeat tick start event if lab emitter is available
        if (lab_emitter) |emitter| {
            const LabEventEmitter = @import("../runtime/lab_events.zig").LabEventEmitter;
            const emitter_ptr: *LabEventEmitter = @ptrCast(@alignCast(emitter));
            if (emitter_ptr.shouldEmit()) {
                emitter_ptr.emitHeartbeatStart(elapsed_millis);
            }
        }

        // Emit heartbeat log to stdout fd (fd=1).
        const emit_result = heartbeat_emit.emitHeartbeatToFdResult(uptime_seconds);

        // Emit heartbeat tick end or failed event if lab emitter is available
        if (lab_emitter) |emitter| {
            const LabEventEmitter = @import("../runtime/lab_events.zig").LabEventEmitter;
            const emitter_ptr: *LabEventEmitter = @ptrCast(@alignCast(emitter));
            if (emitter_ptr.shouldEmit()) {
                if (emit_result) {
                    emitter_ptr.emitHeartbeatEnd(elapsed_millis);
                } else {
                    emitter_ptr.emitHeartbeatFailed(elapsed_millis, "emit_failed");
                }
            }
        }
    }
}
