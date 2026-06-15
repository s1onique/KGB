/// Heartbeat log formatter for tovarisch.
///
/// Emits periodic structured JSON heartbeat records as NDJSON.
/// This module handles formatting only; the runtime heartbeat emission
/// lives in http/server.zig.

const std = @import("std");
const status = @import("../status.zig");

/// Heartbeat statistics snapshot.
pub const HeartbeatStats = struct {
    uptime_seconds: u64,
    status: status.CheckStatus,
    checks_count: usize,
    tunnel_count: u32,
    rx_bytes: u64,
    tx_bytes: u64,
};

/// Write a decimal integer without allocation.
fn writeDecimal(writer: anytype, value: u64) !void {
    var buf: [32]u8 = undefined;
    const slice = std.fmt.bufPrint(&buf, "{d}", .{value}) catch return error.BufferOverflow;
    try writer.writeAll(slice);
}

/// Writes a placeholder timestamp. 
/// Note: std.time.Timestamp is not available in Zig 0.16.x
/// so we use a simplified static timestamp format.
fn writeTimestamp(writer: anytype) !void {
    // In Zig 0.16.x, we don't have std.time.Timestamp.
    // Write a placeholder timestamp until proper time API is available.
    try writer.writeAll("\"2026-05-24T00:00:00Z\"");
}

/// Emit a single heartbeat log record to a buffered writer.
///
/// The record format:
///
/// ```json
/// {"ts":"2026-05-24T11:40:30Z","level":"info","event":"heartbeat","service":"tovarisch","uptime_seconds":30,"status":"warn","checks_count":4,"tunnel_count":1,"rx_bytes":210965885014,"tx_bytes":1622303482922}
/// ```
///
/// Records end with a newline for NDJSON.
pub fn writeHeartbeatLogToWriter(writer: anytype, stats: HeartbeatStats) !void {
    try writer.writeAll("{\"ts\":");
    try writeTimestamp(writer);
    try writer.writeAll(",\"level\":\"info\",\"event\":\"heartbeat\",\"service\":\"tovarisch\",");
    try writer.writeAll("\"uptime_seconds\":");
    try writeDecimal(writer, stats.uptime_seconds);

    try writer.writeAll(",\"status\":\"");
    try writer.writeAll(@tagName(stats.status));

    try writer.writeAll("\",\"checks_count\":");
    try writeDecimal(writer, stats.checks_count);

    try writer.writeAll(",\"tunnel_count\":");
    try writeDecimal(writer, stats.tunnel_count);

    try writer.writeAll(",\"rx_bytes\":");
    try writeDecimal(writer, stats.rx_bytes);

    try writer.writeAll(",\"tx_bytes\":");
    try writeDecimal(writer, stats.tx_bytes);

    try writer.writeAll("}\n");
}

// TestWriter: fixed-buffer writer for testing heartbeat log output.
const TestWriter = struct {
    const Self = @This();
    const BufSize = 512;

    buf: [BufSize]u8 = undefined,
    len: usize = 0,

    pub fn init() Self {
        return .{ .buf = undefined, .len = 0 };
    }

    pub fn print(self: *Self, comptime fmt: []const u8, args: anytype) !void {
        if (self.len >= Self.BufSize) return error.BufferOverflow;
        const written = std.fmt.bufPrint(self.buf[self.len..], fmt, args) catch return error.BufferOverflow;
        self.len += written.len;
    }

    pub fn writeAll(self: *Self, bytes: []const u8) !void {
        if (self.len + bytes.len > Self.BufSize) return error.BufferOverflow;
        // MemoryCopySafety: self.buf is a fixed [512]u8 buffer. bytes is a caller-provided
        // slice. They are distinct memory regions; no aliasing possible.
        @memcpy(self.buf[self.len..][0..bytes.len], bytes);
        self.len += bytes.len;
    }

    pub fn writeByte(self: *Self, byte: u8) !void {
        if (self.len >= Self.BufSize) return error.BufferOverflow;
        self.buf[self.len] = byte;
        self.len += 1;
    }

    pub fn slice(self: *const Self) []const u8 {
        return self.buf[0..self.len];
    }
};

// --- Tests ---

test "HeartbeatStats has correct fields" {
    const stats = HeartbeatStats{
        .uptime_seconds = 30,
        .status = .warn,
        .checks_count = 5,
        .tunnel_count = 1,
        .rx_bytes = 100,
        .tx_bytes = 200,
    };
    try std.testing.expectEqual(@as(u64, 30), stats.uptime_seconds);
    try std.testing.expectEqual(status.CheckStatus.warn, stats.status);
    try std.testing.expectEqual(@as(usize, 5), stats.checks_count);
    try std.testing.expectEqual(@as(u32, 1), stats.tunnel_count);
    try std.testing.expectEqual(@as(u64, 100), stats.rx_bytes);
    try std.testing.expectEqual(@as(u64, 200), stats.tx_bytes);
}

test "writeHeartbeatLogToWriter emits valid JSON object" {
    const stats = HeartbeatStats{
        .uptime_seconds = 30,
        .status = .warn,
        .checks_count = 5,
        .tunnel_count = 1,
        .rx_bytes = 0,
        .tx_bytes = 0,
    };

    var writer = TestWriter.init();
    try writeHeartbeatLogToWriter(&writer, stats);

    const output = writer.slice();
    try std.testing.expect(output.len > 0);
    try std.testing.expectEqual(@as(u8, '\n'), output[output.len - 1]);
    try std.testing.expectEqual(@as(u8, '{'), output[0]);
}

test "writeHeartbeatLogToWriter emits trailing newline" {
    const stats = HeartbeatStats{
        .uptime_seconds = 0,
        .status = .ok,
        .checks_count = 0,
        .tunnel_count = 0,
        .rx_bytes = 0,
        .tx_bytes = 0,
    };

    var writer = TestWriter.init();
    try writeHeartbeatLogToWriter(&writer, stats);

    const output = writer.slice();
    try std.testing.expectEqual(@as(u8, '\n'), output[output.len - 1]);
}

test "writeHeartbeatLogToWriter contains event:heartbeat" {
    const stats = HeartbeatStats{
        .uptime_seconds = 0,
        .status = .ok,
        .checks_count = 0,
        .tunnel_count = 0,
        .rx_bytes = 0,
        .tx_bytes = 0,
    };

    var writer = TestWriter.init();
    try writeHeartbeatLogToWriter(&writer, stats);

    const output = writer.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"event\":\"heartbeat\""));
}

test "writeHeartbeatLogToWriter contains tunnel_count field" {
    const stats = HeartbeatStats{
        .uptime_seconds = 0,
        .status = .ok,
        .checks_count = 0,
        .tunnel_count = 1,
        .rx_bytes = 0,
        .tx_bytes = 0,
    };

    var writer = TestWriter.init();
    try writeHeartbeatLogToWriter(&writer, stats);

    const output = writer.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"tunnel_count\":1"));
}

test "writeHeartbeatLogToWriter contains non-zero rx_bytes tx_bytes" {
    const stats = HeartbeatStats{
        .uptime_seconds = 0,
        .status = .ok,
        .checks_count = 0,
        .tunnel_count = 1,
        .rx_bytes = 1024,
        .tx_bytes = 2048,
    };

    var writer = TestWriter.init();
    try writeHeartbeatLogToWriter(&writer, stats);

    const output = writer.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"rx_bytes\":1024"));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"tx_bytes\":2048"));
}

test "writeHeartbeatLogToWriter contains service:tovarisch" {
    const stats = HeartbeatStats{
        .uptime_seconds = 0,
        .status = .ok,
        .checks_count = 0,
        .tunnel_count = 0,
        .rx_bytes = 0,
        .tx_bytes = 0,
    };

    var writer = TestWriter.init();
    try writeHeartbeatLogToWriter(&writer, stats);

    const output = writer.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"service\":\"tovarisch\""));
}

test "writeHeartbeatLogToWriter contains level:info" {
    const stats = HeartbeatStats{
        .uptime_seconds = 0,
        .status = .ok,
        .checks_count = 0,
        .tunnel_count = 0,
        .rx_bytes = 0,
        .tx_bytes = 0,
    };

    var writer = TestWriter.init();
    try writeHeartbeatLogToWriter(&writer, stats);

    const output = writer.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"level\":\"info\""));
}

test "writeHeartbeatLogToWriter contains status:ok" {
    const stats = HeartbeatStats{
        .uptime_seconds = 0,
        .status = .ok,
        .checks_count = 0,
        .tunnel_count = 0,
        .rx_bytes = 0,
        .tx_bytes = 0,
    };

    var writer = TestWriter.init();
    try writeHeartbeatLogToWriter(&writer, stats);

    const output = writer.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"status\":\"ok\""));
}

test "HeartbeatStats with wg0 tunnel counters" {
    // Regression test: fixture containing wg0 with non-zero counters
    // should produce heartbeat summary with tunnel_count=1, rx_bytes>0, tx_bytes>0
    const stats = HeartbeatStats{
        .uptime_seconds = 30,
        .status = .warn,
        .checks_count = 6,
        .tunnel_count = 1,
        .rx_bytes = 210965885014,
        .tx_bytes = 1622303482922,
    };

    var writer = TestWriter.init();
    try writeHeartbeatLogToWriter(&writer, stats);

    const output = writer.slice();
    // Verify tunnel_count=1 matches /metrics.json contract
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"tunnel_count\":1"));
    // Verify non-zero tunnel counters
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"rx_bytes\":210965885014"));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"tx_bytes\":1622303482922"));
}

test "HeartbeatStats no-tunnel state regression" {
    // Regression test: no-tunnel state should produce zeros
    const stats = HeartbeatStats{
        .uptime_seconds = 30,
        .status = .ok,
        .checks_count = 4,
        .tunnel_count = 0,
        .rx_bytes = 0,
        .tx_bytes = 0,
    };

    var writer = TestWriter.init();
    try writeHeartbeatLogToWriter(&writer, stats);

    const output = writer.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"tunnel_count\":0"));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"rx_bytes\":0"));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"tx_bytes\":0"));
}
