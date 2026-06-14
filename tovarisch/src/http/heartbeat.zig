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
const status = @import("../status.zig");
const linux_interface_stats = @import("../net/linux_interface_stats.zig");
const interface_filter = @import("../net/interface_filter.zig");

/// Heartbeat thread configuration.
pub const HEARTBEAT_INTERVAL_SECS: u64 = 30;

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
/// Returns tunnel summary with aggregated counters.
pub fn collectTunnelSummary(allocator: std.mem.Allocator, sysfs_root: []const u8) TunnelSummary {
    const stats = linux_interface_stats.collectInterfaceStats(allocator, sysfs_root) catch {
        // On collection failure, return zero summary (metrics will show warning)
        return .{ .count = 0, .rx_bytes = 0, .tx_bytes = 0 };
    };
    defer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, stats);

    var summary = TunnelSummary{ .count = 0, .rx_bytes = 0, .tx_bytes = 0 };
    for (stats) |snap| {
        if (interface_filter.isTunnelInterface(snap.name)) {
            summary.count += 1;
            summary.rx_bytes += snap.stats.rx_bytes;
            summary.tx_bytes += snap.stats.tx_bytes;
        }
    }
    return summary;
}

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

        // Emit heartbeat log to stdout fd (fd=1).
        emitHeartbeatToFd(uptime_seconds);
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
    var scratch = status.StatusScratch{};
    const current_status = status.buildStatus(&scratch);

    // Collect tunnel summary using the same interface enumeration as /metrics.json
    // This ensures heartbeat logs are consistent with metrics output.
    //
    // MemoryOwnership: Transient allocation within heartbeat emit cycle.
    // Memory is released after JSON formatting completes.
    const tunnel_summary = collectTunnelSummary(std.heap.page_allocator, "/sys/class/net");

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

// Re-export linux_stats helpers for test fixture creation
const linux_stats = @import("../net/linux_stats.zig");
const makeDir = linux_stats.makeDir;
const deleteTree = linux_stats.deleteTree;
const writeFile = linux_stats.writeFile;

/// Helper: Create a fixture interface with statistics (same as linux_interface_stats_tests.zig)
fn createIfaceWithStats(base: []const u8, iface: []const u8, rx_bytes: u64, tx_bytes: u64, rx_packets: u64, tx_packets: u64) !void {
    var path_buf1: [4096]u8 = undefined;
    var path_buf2: [4096]u8 = undefined;
    var num_buf: [64]u8 = undefined;

    const iface_path = try std.fmt.bufPrint(&path_buf1, "{s}/{s}", .{ base, iface });
    try makeDir(iface_path);

    const stats_path = try std.fmt.bufPrint(&path_buf2, "{s}/statistics", .{iface_path});
    try makeDir(stats_path);

    var rx_bytes_path_buf: [4096]u8 = undefined;
    var tx_bytes_path_buf: [4096]u8 = undefined;
    var rx_packets_path_buf: [4096]u8 = undefined;
    var tx_packets_path_buf: [4096]u8 = undefined;

    const rx_bytes_path = try std.fmt.bufPrint(&rx_bytes_path_buf, "{s}/rx_bytes", .{stats_path});
    const tx_bytes_path = try std.fmt.bufPrint(&tx_bytes_path_buf, "{s}/tx_bytes", .{stats_path});
    const rx_packets_path = try std.fmt.bufPrint(&rx_packets_path_buf, "{s}/rx_packets", .{stats_path});
    const tx_packets_path = try std.fmt.bufPrint(&tx_packets_path_buf, "{s}/tx_packets", .{stats_path});

    const rx_bytes_str = try std.fmt.bufPrint(&num_buf, "{d}\n", .{rx_bytes});
    try writeFile(rx_bytes_path, rx_bytes_str);

    const tx_bytes_str = try std.fmt.bufPrint(&num_buf, "{d}\n", .{tx_bytes});
    try writeFile(tx_bytes_path, tx_bytes_str);

    const rx_packets_str = try std.fmt.bufPrint(&num_buf, "{d}\n", .{rx_packets});
    try writeFile(rx_packets_path, rx_packets_str);

    const tx_packets_str = try std.fmt.bufPrint(&num_buf, "{d}\n", .{tx_packets});
    try writeFile(tx_packets_path, tx_packets_str);
}

// --- collectTunnelSummary regression tests ---

test "collectTunnelSummary: fake sysfs with wg0 returns count=1 and non-zero rx/tx" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_heartbeat_tunnel_wg0";

    try makeDir(base);
    defer deleteTree(base) catch {};

    // Create wg0 with non-zero counters (mimics WireGuard with traffic)
    try createIfaceWithStats(base, "wg0", 210965885014, 1622303482922, 1234567, 987654);

    const summary = collectTunnelSummary(allocator, base);

    // wg0 is a tunnel interface, should be counted
    try std.testing.expectEqual(@as(u32, 1), summary.count);
    try std.testing.expectEqual(@as(u64, 210965885014), summary.rx_bytes);
    try std.testing.expectEqual(@as(u64, 1622303482922), summary.tx_bytes);
}

test "collectTunnelSummary: fake sysfs with non-tunnel only returns count=0 and zeros" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_heartbeat_tunnel_eth_only";

    try makeDir(base);
    defer deleteTree(base) catch {};

    // Create podman1 and eth0 (non-tunnel interfaces)
    try createIfaceWithStats(base, "podman1", 1000, 2000, 10, 20);
    try createIfaceWithStats(base, "eth0", 500, 600, 5, 6);

    const summary = collectTunnelSummary(allocator, base);

    // No tunnel interfaces, should return zeros
    try std.testing.expectEqual(@as(u32, 0), summary.count);
    try std.testing.expectEqual(@as(u64, 0), summary.rx_bytes);
    try std.testing.expectEqual(@as(u64, 0), summary.tx_bytes);
}

test "collectTunnelSummary: fake sysfs with mixed tunnel and non-tunnel" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_heartbeat_tunnel_mixed";

    try makeDir(base);
    defer deleteTree(base) catch {};

    // Create eth0 (non-tunnel), wg0 (tunnel), wg1 (tunnel)
    try createIfaceWithStats(base, "eth0", 100, 200, 10, 20);
    try createIfaceWithStats(base, "wg0", 300, 400, 30, 40);
    try createIfaceWithStats(base, "wg1", 500, 600, 50, 60);

    const summary = collectTunnelSummary(allocator, base);

    // Only wg0 and wg1 should be counted
    try std.testing.expectEqual(@as(u32, 2), summary.count);
    try std.testing.expectEqual(@as(u64, 300 + 500), summary.rx_bytes);
    try std.testing.expectEqual(@as(u64, 400 + 600), summary.tx_bytes);
}

test "collectTunnelSummary: returns zero summary on missing root" {
    const allocator = std.testing.allocator;
    const base = "/tmp/kgb_heartbeat_tunnel_nonexistent_1234567890";

    deleteTree(base) catch {};

    const summary = collectTunnelSummary(allocator, base);

    // Should return zeros on error
    try std.testing.expectEqual(@as(u32, 0), summary.count);
    try std.testing.expectEqual(@as(u64, 0), summary.rx_bytes);
    try std.testing.expectEqual(@as(u64, 0), summary.tx_bytes);
}
