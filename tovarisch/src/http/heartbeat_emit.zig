// heartbeat_emit.zig — Heartbeat log emission helpers
//
// Extracted from heartbeat.zig to keep file sizes under LLM-friendly limits.
// Contains the HeartbeatWriter, decimal formatting, and emit functions.

const std = @import("std");
const c = std.c;
const status = @import("../status.zig");
const linux_interface_stats = @import("../net/linux_interface_stats.zig");
const interface_filter = @import("../net/interface_filter.zig");

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
pub const HeartbeatWriter = struct {
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
        // MemoryCopySafety: self.buf is a fixed [4096]u8 buffer. bytes is a caller-provided
        // slice. They are distinct memory regions; no aliasing possible.
        @memcpy(self.buf[self.len..][0..bytes.len], bytes);
        self.len += bytes.len;
    }

    pub fn slice(self: *const Self) []const u8 {
        if (self.dropped) return "";
        return self.buf[0..self.len];
    }
};

/// Write decimal integer to HeartbeatWriter.
pub fn writeDecimalToHeartbeat(writer: *HeartbeatWriter, value: u64) void {
    var buf: [32]u8 = undefined;
    const slice = std.fmt.bufPrint(&buf, "{d}", .{value}) catch return;
    writer.writeAll(slice);
}

/// Tunnel summary derived from live interface stats.
///
/// Mirrors the aggregation logic used by /metrics.json for consistency:
/// - Counts interfaces where is_tunnel = true
/// - Aggregates rx_bytes / tx_bytes from tunnel interfaces
pub const TunnelSummary = struct {
    count: u32,
    rx_bytes: u64,
    tx_bytes: u64,
};

/// Collects tunnel interface stats by enumerating all interfaces and filtering
/// for tunnel type using the same interface_filter.isTunnelInterface() logic
/// that /metrics.json uses.
///
/// This function mirrors the tunnel observation path used by metrics rendering,
/// ensuring heartbeat logs are consistent with /metrics.json output.
///
/// **MEMORY OWNERSHIP**: The caller MUST call freeTunnelSummarySnapshots() with
/// the same allocator after using the returned TunnelSummaryWithStats, before
/// the next call to collectTunnelSummaryWithStats(). This is required because
/// the stats snapshot must be freed to prevent memory growth on each heartbeat.
///
/// Returns tunnel summary with aggregated counters plus owned stats snapshots.
pub const TunnelSummaryWithStats = struct {
    summary: TunnelSummary,
    stats: []linux_interface_stats.InterfaceStatsSnapshot,
};

/// Frees tunnel summary stats returned by collectTunnelSummaryWithStats().
/// Must be called by the caller after processing the summary.
pub fn freeTunnelSummarySnapshots(allocator: std.mem.Allocator, result: TunnelSummaryWithStats) void {
    linux_interface_stats.freeInterfaceStatsSnapshots(allocator, result.stats);
}

/// Collects tunnel interface stats with owned snapshots for deterministic memory management.
///
/// This is the preferred API for repeated heartbeat calls. The caller must:
/// 1. Call this function to get the summary and owned snapshots
/// 2. Process the summary
/// 3. Call freeTunnelSummarySnapshots() to release memory
///
/// Failure to call freeTunnelSummarySnapshots() will cause memory growth
/// on each heartbeat cycle (approximately 1-2KB per cycle per interface).
pub fn collectTunnelSummaryWithStats(allocator: std.mem.Allocator, sysfs_root: []const u8) TunnelSummaryWithStats {
    const stats = linux_interface_stats.collectInterfaceStats(allocator, sysfs_root, .sysfs_net) catch {
        // On collection failure, return zero summary (metrics will show warning)
        return .{ .summary = .{ .count = 0, .rx_bytes = 0, .tx_bytes = 0 }, .stats = &.{} };
    };

    var summary = TunnelSummary{ .count = 0, .rx_bytes = 0, .tx_bytes = 0 };
    for (stats) |snap| {
        if (interface_filter.isTunnelInterface(snap.name)) {
            summary.count += 1;
            summary.rx_bytes += snap.stats.rx_bytes;
            summary.tx_bytes += snap.stats.tx_bytes;
        }
    }
    return .{ .summary = summary, .stats = stats };
}

/// Legacy tunnel summary collector for single-shot use cases.
///
/// This function properly frees the stats snapshots before returning,
/// so it's safe for repeated calls without leaking memory.
/// Exists for API compatibility; prefer collectTunnelSummaryWithStats()
/// when you need access to the raw stats snapshots.
///
/// Returns tunnel summary with aggregated counters.
pub fn collectTunnelSummary(allocator: std.mem.Allocator, sysfs_root: []const u8) TunnelSummary {
    const result = collectTunnelSummaryWithStats(allocator, sysfs_root);
    const summary = result.summary;
    // Free the snapshots immediately since this is single-shot use
    linux_interface_stats.freeInterfaceStatsSnapshots(allocator, result.stats);
    return summary;
}

/// Emit heartbeat log and return success/failure.
/// Used by heartbeatThreadWithEvents to report emit status to lab events.
pub fn emitHeartbeatToFdResult(uptime_seconds: u64) bool {
    // Get current status from the same derivation as /status
    var scratch = status.StatusScratch{ .allocator = std.heap.page_allocator };
    const current_status = status.buildStatus(&scratch);

    // Collect tunnel summary using the same interface enumeration as /metrics.json
    // This ensures heartbeat logs are consistent with metrics output.
    // MemoryOwnership: page_allocator used with collectTunnelSummaryWithStats.
    // The freeTunnelSummarySnapshots() deferred call releases memory immediately
    // after use, before the next heartbeat cycle. Memory is bounded/fixed per cycle.
    const tunnel_result = collectTunnelSummaryWithStats(std.heap.page_allocator, "/sys/class/net");
    defer freeTunnelSummarySnapshots(std.heap.page_allocator, tunnel_result);
    const tunnel_summary = tunnel_result.summary;

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
    // Use singular "tunnel_count" to match /metrics.json contract
    log_buf.writeAll(",\"tunnel_count\":");
    writeDecimalToHeartbeat(&log_buf, tunnel_summary.count);
    log_buf.writeAll(",\"rx_bytes\":");
    writeDecimalToHeartbeat(&log_buf, tunnel_summary.rx_bytes);
    log_buf.writeAll(",\"tx_bytes\":");
    writeDecimalToHeartbeat(&log_buf, tunnel_summary.tx_bytes);
    log_buf.writeAll("}\n");

    // Write to stdout fd. Return true if non-empty bytes written, false otherwise.
    const bytes = log_buf.slice();
    if (bytes.len > 0) {
        _ = c.write(1, bytes.ptr, bytes.len);
        return true;
    }
    return false;
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
    try std.testing.expectEqual(@as(usize, 4), w.len);
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
