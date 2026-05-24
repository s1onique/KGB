/// Heartbeat thread for tovarisch.
/// 
/// Implements a daemon-lifetime thread that emits heartbeat logs every 30 seconds
/// independently of HTTP request traffic. Uses std.c.nanosleep for blocking
/// sleep (Zig 0.16 std.Thread does not expose a sleep function).

const std = @import("std");
const c = std.c;
const heartbeat_log = @import("../runtime/heartbeat_log.zig");
const status = @import("../status.zig");
const logging = @import("../logging.zig");

/// Heartbeat thread configuration.
pub const HEARTBEAT_INTERVAL_SECS: u64 = 30;

/// Heartbeat thread context shared between main thread and heartbeat thread.
/// Uses std.c.pthread_mutex for cross-platform thread synchronization.
pub const HeartbeatContext = struct {
    const Self = @This();

    /// Mutex for protecting shared state (pthread_mutex_t).
    /// Uses PTHREAD_MUTEX_INITIALIZER for static initialization.
    /// This is the portable POSIX way - no runtime init needed.
    mutex: c.pthread_mutex_t,
    /// Current uptime in seconds (updated by heartbeat thread).
    uptime_seconds: u64,
    /// Flag to signal thread shutdown.
    done: bool,
};

/// Initialize heartbeat context for the daemon-lifetime heartbeat thread.
/// Uses PTHREAD_MUTEX_INITIALIZER for static, compile-time initialization.
/// No runtime initialization required - the mutex is valid after struct creation.
pub fn initHeartbeatContext() HeartbeatContext {
    return HeartbeatContext{
        // PTHREAD_MUTEX_INITIALIZER is a compile-time constant with default attributes.
        // This is the portable POSIX approach - no pthread_mutex_init() runtime call needed.
        .mutex = c.PTHREAD_MUTEX_INITIALIZER,
        .uptime_seconds = 0,
        .done = false,
    };
}

/// Heartbeat thread entry point.
/// Loops every HEARTBEAT_INTERVAL_SECS and emits heartbeat logs.
/// Uses std.c.nanosleep for cross-platform blocking sleep (Zig 0.16 doesn't have std.Thread.sleep).
/// Note: parameter is *const because Zig thread spawn passes const pointer.
pub fn heartbeatThread(ctx: *const HeartbeatContext) void {
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

        // @constCast needed: Zig passes *const, but pthread_mutex_lock needs *T
        // and we need to modify uptime_seconds
        var mutable_ctx = @constCast(ctx);

        _ = c.pthread_mutex_lock(&mutable_ctx.mutex);

        if (mutable_ctx.done) {
            _ = c.pthread_mutex_unlock(&mutable_ctx.mutex);
            return;
        }

        mutable_ctx.uptime_seconds += HEARTBEAT_INTERVAL_SECS;
        const uptime = mutable_ctx.uptime_seconds;

        _ = c.pthread_mutex_unlock(&mutable_ctx.mutex);

        // Emit heartbeat log to stdout fd (fd=1).
        emitHeartbeatToFd(uptime);
    }
}

/// Emit heartbeat log to a raw file descriptor.
/// Uses c.write for cross-platform compatibility.
fn emitHeartbeatToFd(uptime_seconds: u64) void {
    // Get current status from the same derivation as /status
    const current_status = status.getStatus();

    // Format the heartbeat JSON into a buffer
    var log_buf = logging.BufferedWriter.init();
    const stats = heartbeat_log.HeartbeatStats{
        .uptime_seconds = uptime_seconds,
        .status = current_status.status,
        .checks_count = current_status.checks.len,
        .tunnels_count = 0, // Placeholder until tunnel subsystem exists
        .rx_bytes = 0, // Placeholder until tunnel subsystem exists
        .tx_bytes = 0, // Placeholder until tunnel subsystem exists
    };

    heartbeat_log.writeHeartbeatLogToWriter(&log_buf, stats) catch return;

    // Write to stdout fd. Ignore partial writes to not disrupt thread.
    const bytes = log_buf.slice();
    _ = c.write(1, bytes.ptr, bytes.len);
}
