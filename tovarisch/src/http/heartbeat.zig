/// Heartbeat thread for tovarisch.
/// 
/// Implements a daemon-lifetime thread that emits heartbeat logs every 30 seconds
/// independently of HTTP request traffic. Uses std.c.nanosleep for blocking
/// sleep (Zig 0.16 std.Thread does not expose a sleep function).

const std = @import("std");
const c = std.c;
const status = @import("../status.zig");

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

/// Heartbeat-specific fixed buffer writer.
/// Uses 4096 bytes to ensure heartbeat logs never overflow.
/// This is larger than the 1024-byte logging.BufferedWriter to handle
/// the heartbeat-specific record format safely.
///
/// IMPORTANT: Overflow behavior is NDJSON-safe. When the buffer fills,
/// the entire record is marked as dropped (not just the overflow chunk).
/// This ensures malformed/truncated JSON is never emitted - an empty
/// record is written instead. Heartbeat is decorative; clean NDJSON
/// hygiene is preferred over partial records.
const HeartbeatWriter = struct {
    const Self = @This();
    const BufSize = 4096;

    buf: [BufSize]u8 = undefined,
    len: usize = 0,
    /// Set to true when buffer overflows. Causes slice() to return
    /// empty string, ensuring no malformed JSON is emitted.
    dropped: bool = false,

    pub fn init() Self {
        return .{ .buf = undefined, .len = 0, .dropped = false };
    }

    pub fn writeAll(self: *Self, bytes: []const u8) void {
        if (self.dropped) return;
        if (self.len + bytes.len > Self.BufSize) {
            // Mark dropped and clear buffer to prevent malformed JSON emission.
            self.len = 0;
            self.dropped = true;
            return;
        }
        @memcpy(self.buf[self.len..][0..bytes.len], bytes);
        self.len += bytes.len;
    }

    pub fn slice(self: *const Self) []const u8 {
        if (self.dropped) return "";
        return self.buf[0..self.len];
    }
};

/// Write decimal integer to HeartbeatWriter.
fn writeDecimalToHeartbeat(writer: *HeartbeatWriter, value: u64) void {
    var buf: [32]u8 = undefined;
    const slice = std.fmt.bufPrint(&buf, "{d}", .{value}) catch return;
    writer.writeAll(slice);
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
///
/// IMPORTANT: This function is panic-proof by design:
/// - Uses HeartbeatWriter with 4096-byte buffer (larger than logging.BufferedWriter)
/// - Overflow marks record dropped (returns empty slice) - no malformed JSON
/// - Never propagates errors or panics
/// - Background heartbeat must never crash the leaf daemon
///
/// Note: ts field is omitted until real timestamp formatting is available.
/// journald already provides accurate receipt timestamps; a fake fixed
/// timestamp in the payload would be misleading to operators.
fn emitHeartbeatToFd(uptime_seconds: u64) void {
    // Get current status from the same derivation as /status
    const current_status = status.getStatus();

    // Format the heartbeat JSON into our own 4096-byte buffer
    // This is separate from logging.BufferedWriter to avoid any shared state
    var log_buf = HeartbeatWriter.init();

    // Write heartbeat record manually to avoid any potential panic paths
    // Note: ts field omitted - journald provides real receipt timestamp
    log_buf.writeAll("{\"level\":\"info\",\"event\":\"heartbeat\",\"service\":\"tovarisch\",");

    log_buf.writeAll("\"uptime_seconds\":");
    writeDecimalToHeartbeat(&log_buf, uptime_seconds);

    log_buf.writeAll(",\"status\":\"");
    log_buf.writeAll(@tagName(current_status.status));

    log_buf.writeAll("\",\"checks_count\":");
    writeDecimalToHeartbeat(&log_buf, current_status.checks.len);

    log_buf.writeAll(",\"tunnels_count\":0,\"rx_bytes\":0,\"tx_bytes\":0}\n");

    // Write to stdout fd. Ignore partial writes to not disrupt thread.
    const bytes = log_buf.slice();
    _ = c.write(1, bytes.ptr, bytes.len);
}

// --- Tests ---

test "HeartbeatWriter writes bytes correctly" {
    var w = HeartbeatWriter.init();
    w.writeAll("hello");
    try std.testing.expectEqual(@as(usize, 5), w.len);
    try std.testing.expectEqualSlices(u8, "hello", w.slice());
}

test "HeartbeatWriter writes without error return" {
    var w = HeartbeatWriter.init();
    w.writeAll("test");
    try std.testing.expectEqualSlices(u8, "test", w.slice());
}

test "HeartbeatWriter silently drops on overflow" {
    var w = HeartbeatWriter.init();
    // Fill the buffer
    const big_data = "x" ** HeartbeatWriter.BufSize;
    w.writeAll(big_data);
    try std.testing.expectEqual(HeartbeatWriter.BufSize, w.len);
    
    // Try to write more - should mark dropped and clear
    w.writeAll("overflow");
    try std.testing.expectEqual(@as(usize, 0), w.len);
    try std.testing.expect(w.dropped);
}

test "HeartbeatWriter slice returns empty when dropped" {
    var w = HeartbeatWriter.init();
    // Fill buffer then overflow
    const big_data = "x" ** HeartbeatWriter.BufSize;
    w.writeAll(big_data);
    w.writeAll("overflow");
    
    // Slice should return empty when dropped
    try std.testing.expectEqualSlices(u8, "", w.slice());
}

test "HeartbeatWriter slice returns written content when not dropped" {
    var w = HeartbeatWriter.init();
    w.writeAll("partial");
    const slice = w.slice();
    try std.testing.expectEqualSlices(u8, "partial", slice);
    try std.testing.expect(!w.dropped);
}

test "writeDecimalToHeartbeat writes u64 value" {
    var w = HeartbeatWriter.init();
    writeDecimalToHeartbeat(&w, 12345);
    try std.testing.expectEqualSlices(u8, "12345", w.slice());
}

test "HeartbeatWriter BufSize is 4096" {
    try std.testing.expectEqual(@as(usize, 4096), HeartbeatWriter.BufSize);
}
