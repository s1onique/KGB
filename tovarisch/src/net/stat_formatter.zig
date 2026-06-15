// stat_formatter.zig — Human-readable compact interface stats formatter
//
// ACT: Add --statonly operator stats mode
//
// Formats interface rate deltas into compact human-readable lines for terminal
// operators. This is intentionally non-JSON for 03:00 readability.
//
// Output format for one interface:
//   eth0 rx=1.2MiB/s tx=420KiB/s rxp=1.8k/s txp=900/s err=0 drop=0
//
// Output format for multiple interfaces:
//   eth0 rx=1.2MiB/s tx=420KiB/s | wg0 rx=220KiB/s tx=180KiB/s
//
// Special cases:
//   - No interfaces: "net: no interfaces"
//   - Error/drop deltas: raw integers per interval

const std = @import("std");
const rates = @import("rates.zig");

// ============================================================================
// Formatters
// ============================================================================

/// Format bytes per second in human-readable form.
/// Uses binary units (KiB, MiB) per the rate calculation convention.
fn formatBytesPerSecond(bytes_per_sec: u64, buf: []u8) []const u8 {
    if (bytes_per_sec < 1024) {
        // Raw bytes: "123B/s"
        return std.fmt.bufPrint(buf, "{d}B/s", .{bytes_per_sec}) catch "0B/s";
    } else if (bytes_per_sec < 1024 * 1024) {
        // Kibibytes: "123KiB/s" or "12.3KiB/s"
        const kib = bytes_per_sec / 1024;
        const remainder = bytes_per_sec % 1024;
        if (remainder == 0) {
            return std.fmt.bufPrint(buf, "{d}KiB/s", .{kib}) catch "0KiB/s";
        } else {
            // Show one decimal place for fractional KiB
            const tenths = (remainder * 10) / 1024;
            return std.fmt.bufPrint(buf, "{d}.{d}KiB/s", .{ kib, tenths }) catch "0KiB/s";
        }
    } else {
        // Mebibytes: "123MiB/s" or "12.3MiB/s"
        const mib = bytes_per_sec / (1024 * 1024);
        const remainder = bytes_per_sec % (1024 * 1024);
        if (remainder == 0) {
            return std.fmt.bufPrint(buf, "{d}MiB/s", .{mib}) catch "0MiB/s";
        } else {
            // Show one decimal place for fractional MiB
            const tenths = (remainder * 10) / (1024 * 1024);
            return std.fmt.bufPrint(buf, "{d}.{d}MiB/s", .{ mib, tenths }) catch "0MiB/s";
        }
    }
}

/// Format packets per second in human-readable form.
/// Uses k/s notation for >= 1000 packets/sec.
fn formatPacketsPerSecond(packets_per_sec: u64, buf: []u8) []const u8 {
    if (packets_per_sec < 1000) {
        // Raw packets: "900/s"
        return std.fmt.bufPrint(buf, "{d}/s", .{packets_per_sec}) catch "0/s";
    } else {
        // Convert to k/s notation: "1.2k/s"
        const k_val = packets_per_sec / 1000;
        const remainder = packets_per_sec % 1000;
        if (remainder == 0) {
            return std.fmt.bufPrint(buf, "{d}k/s", .{k_val}) catch "0k/s";
        } else {
            // Show one decimal place
            const tenths = remainder / 100;
            return std.fmt.bufPrint(buf, "{d}.{d}k/s", .{ k_val, tenths }) catch "0k/s";
        }
    }
}

// ============================================================================
// Compact Line Formatter
// ============================================================================

/// Formatted interface stats for output.
pub const FormattedInterfaceLine = struct {
    name: []const u8,
    rx_str: []const u8,
    tx_str: []const u8,
    rxp_str: []const u8,
    txp_str: []const u8,
    errors_delta: u64,
    drops_delta: u64,
};

/// Writer interface for compact stat lines.
pub const CompactLineWriter = struct {
    const Self = @This();

    buf: [256]u8 = undefined,
    len: usize = 0,

    pub fn init() Self {
        return .{ .buf = undefined, .len = 0 };
    }

    pub fn writeAll(self: *Self, bytes: []const u8) !void {
        if (self.len + bytes.len > self.buf.len) return error.BufferOverflow;
        // MemoryCopySafety: self.buf is a fixed [256]u8 buffer. bytes is a
        // caller-provided slice. They are distinct memory regions; no aliasing.
        @memcpy(self.buf[self.len..][0..bytes.len], bytes);
        self.len += bytes.len;
    }

    pub fn writeByte(self: *Self, byte: u8) !void {
        if (self.len >= self.buf.len) return error.BufferOverflow;
        self.buf[self.len] = byte;
        self.len += 1;
    }

    pub fn slice(self: *const Self) []const u8 {
        return self.buf[0..self.len];
    }
};

/// Format a single interface into a compact line.
/// Uses pre-allocated buffers for the formatted values.
pub fn formatInterfaceLine(
    name: []const u8,
    rate: ?rates.InterfaceRate,
    err_delta: u64,
    drop_delta: u64,
    writer: *CompactLineWriter,
) !void {
    try writer.writeAll(name);
    try writer.writeAll(" rx=");

    if (rate) |r| {
        var rx_buf: [32]u8 = undefined;
        try writer.writeAll(formatBytesPerSecond(r.rx_bytes_per_second, &rx_buf));
        try writer.writeAll(" tx=");

        var tx_buf: [32]u8 = undefined;
        try writer.writeAll(formatBytesPerSecond(r.tx_bytes_per_second, &tx_buf));
        try writer.writeAll(" rxp=");

        var rxp_buf: [32]u8 = undefined;
        try writer.writeAll(formatPacketsPerSecond(r.rx_packets_per_second, &rxp_buf));
        try writer.writeAll(" txp=");

        var txp_buf: [32]u8 = undefined;
        try writer.writeAll(formatPacketsPerSecond(r.tx_packets_per_second, &txp_buf));
        try writer.writeAll(" err=");
        try writer.writeAll(std.fmt.bufPrint(&tx_buf, "{d}", .{err_delta}) catch "0");
        try writer.writeAll(" drop=");
        try writer.writeAll(std.fmt.bufPrint(&tx_buf, "{d}", .{drop_delta}) catch "0");
    } else {
        // No rate available yet (first sample or counter reset)
        try writer.writeAll("0B/s tx=0B/s rxp=0/s txp=0/s");
        try writer.writeAll(" err=");
        var err_buf: [32]u8 = undefined;
        try writer.writeAll(std.fmt.bufPrint(&err_buf, "{d}", .{err_delta}) catch "0");
        try writer.writeAll(" drop=");
        var drop_buf: [32]u8 = undefined;
        try writer.writeAll(std.fmt.bufPrint(&drop_buf, "{d}", .{drop_delta}) catch "0");
    }
}

/// Format a single interface into a compact line string.
/// Returns an owned string that the caller must free.
/// This is a convenience wrapper for cases where you just need the string.
pub fn formatInterfaceLineAlloc(
    allocator: std.mem.Allocator,
    name: []const u8,
    rate: ?rates.InterfaceRate,
    err_delta: u64,
    drop_delta: u64,
) ![]u8 {
    var writer = CompactLineWriter.init();
    try formatInterfaceLine(name, rate, err_delta, drop_delta, &writer);
    return allocator.dupe(u8, writer.slice());
}

// ============================================================================
// Tests
// ============================================================================

test "formatBytesPerSecond under 1KB" {
    var buf: [32]u8 = undefined;
    const result = formatBytesPerSecond(123, &buf);
    try std.testing.expectEqualSlices(u8, "123B/s", result);
}

test "formatBytesPerSecond exact KiB" {
    var buf: [32]u8 = undefined;
    const result = formatBytesPerSecond(1024, &buf);
    try std.testing.expectEqualSlices(u8, "1KiB/s", result);
}

test "formatBytesPerSecond fractional KiB" {
    var buf: [32]u8 = undefined;
    const result = formatBytesPerSecond(1536, &buf);
    try std.testing.expectEqualSlices(u8, "1.5KiB/s", result);
}

test "formatBytesPerSecond exact MiB" {
    var buf: [32]u8 = undefined;
    const result = formatBytesPerSecond(1048576, &buf);
    try std.testing.expectEqualSlices(u8, "1MiB/s", result);
}

test "formatBytesPerSecond fractional MiB" {
    var buf: [32]u8 = undefined;
    const result = formatBytesPerSecond(1572864, &buf);
    try std.testing.expectEqualSlices(u8, "1.5MiB/s", result);
}

test "formatBytesPerSecond large values" {
    var buf: [32]u8 = undefined;
    const result = formatBytesPerSecond(12 * 1024 * 1024, &buf);
    try std.testing.expectEqualSlices(u8, "12MiB/s", result);
}

test "formatPacketsPerSecond under 1000" {
    var buf: [32]u8 = undefined;
    const result = formatPacketsPerSecond(900, &buf);
    try std.testing.expectEqualSlices(u8, "900/s", result);
}

test "formatPacketsPerSecond exactly 1k" {
    var buf: [32]u8 = undefined;
    const result = formatPacketsPerSecond(1000, &buf);
    try std.testing.expectEqualSlices(u8, "1k/s", result);
}

test "formatPacketsPerSecond fractional k" {
    var buf: [32]u8 = undefined;
    const result = formatPacketsPerSecond(1200, &buf);
    try std.testing.expectEqualSlices(u8, "1.2k/s", result);
}

test "formatPacketsPerSecond larger values" {
    var buf: [32]u8 = undefined;
    const result = formatPacketsPerSecond(8200, &buf);
    try std.testing.expectEqualSlices(u8, "8.2k/s", result);
}

test "CompactLineWriter writes interface name" {
    var writer = CompactLineWriter.init();
    try writer.writeAll("eth0");
    try std.testing.expectEqualSlices(u8, "eth0", writer.slice());
}

test "CompactLineWriter writes rx value" {
    var writer = CompactLineWriter.init();
    try writer.writeAll(" rx=");
    try std.testing.expectEqualSlices(u8, " rx=", writer.slice());
}

test "formatInterfaceLine with rate" {
    var writer = CompactLineWriter.init();
    const rate = rates.InterfaceRate{
        .window_seconds = 30,
        .rx_bytes_delta = 30000,
        .tx_bytes_delta = 12000,
        .rx_packets_delta = 30,
        .tx_packets_delta = 20,
        .rx_bytes_per_second = 1000,
        .tx_bytes_per_second = 400,
        .rx_packets_per_second = 1,
        .tx_packets_per_second = 0,
    };

    try formatInterfaceLine("eth0", rate, 0, 0, &writer);

    const output = writer.slice();
    // Should contain eth0, rx value, tx value, error/drop
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "eth0"));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "rx="));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "tx="));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "err=0"));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "drop=0"));
}

test "formatInterfaceLine without rate (null)" {
    var writer = CompactLineWriter.init();
    try formatInterfaceLine("wg0", null, 5, 3, &writer);

    const output = writer.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "wg0"));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "err=5"));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "drop=3"));
}

test "formatInterfaceLine zero deltas render cleanly" {
    var writer = CompactLineWriter.init();
    const rate = rates.InterfaceRate{
        .window_seconds = 30,
        .rx_bytes_delta = 0,
        .tx_bytes_delta = 0,
        .rx_packets_delta = 0,
        .tx_packets_delta = 0,
        .rx_bytes_per_second = 0,
        .tx_bytes_per_second = 0,
        .rx_packets_per_second = 0,
        .tx_packets_per_second = 0,
    };

    try formatInterfaceLine("lo", rate, 0, 0, &writer);

    const output = writer.slice();
    // Zero values should render as "0B/s", "0/s", etc.
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "rx=0B/s"));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "tx=0B/s"));
}
