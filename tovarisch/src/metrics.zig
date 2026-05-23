// metrics.zig — Metrics payload rendering for /metrics.json
//
// ACT 5h: Wire private interface stats into /metrics.json.
//
// This module renders the v0 metrics JSON payload for the /metrics.json endpoint.
// It provides both a pure renderer (for testing with known snapshots) and a
// live renderer (for production use with real sysfs + rtnetlink collection).
//
// v0 JSON shape:
//
//   {
//     "service": "tovarisch",
//     "version": "0.1.1",
//     "metrics_version": "0.1",
//     "private_interfaces": [
//       {
//         "name": "eth0",
//         "rx_bytes": 123,
//         "tx_bytes": 456,
//         "rx_packets": 7,
//         "tx_packets": 8
//       }
//     ],
//     "notes": [
//       "interface counters are cumulative, not rates",
//       "IPv4 private interfaces only; IPv6 is deferred"
//     ]
//   }
//
// Fallback shape (on live collection failure):
//
//   {
//     "service": "tovarisch",
//     "version": "0.1.1",
//     "metrics_version": "0.1",
//     "status": "warn",
//     "private_interfaces": [],
//     "error": "metrics_unavailable",
//     "detail": "private interface stats unavailable",
//     "notes": [
//       "interface counters are cumulative, not rates",
//       "IPv4 private interfaces only; IPv6 is deferred"
//     ]
//   }
//
// Non-goals:
// - No per-second rate calculation
// - No tunnel detection
// - No IPv6 support (deferred)
// - No Prometheus format

const std = @import("std");
const private_interface_stats = @import("net/private_interface_stats.zig");
const linux_interface_stats = @import("net/linux_interface_stats.zig");

// Re-export types for convenience
pub const InterfaceStatsSnapshot = linux_interface_stats.InterfaceStatsSnapshot;
pub const CollectError = private_interface_stats.CollectError;

// Service version constant (matches status.zig)
const service_version = "0.1.1";
const metrics_version = "0.1";

// ============================================================================
// JSON String Escaping
// ============================================================================

/// Escapes special JSON characters in a string and writes to the writer.
/// Handles: " -> \", \ -> \\, \n -> \\n, \r -> \\r, \t -> \\t
/// Control characters (0x00-0x1F) are passed through directly.
/// Linux interface names are constrained and should not contain control chars.
fn writeJsonString(writer: anytype, s: []const u8) !void {
    for (s) |c| {
        switch (c) {
            '"' => try writer.writeAll("\\\""),
            '\\' => try writer.writeAll("\\\\"),
            '\n' => try writer.writeAll("\\n"),
            '\r' => try writer.writeAll("\\r"),
            '\t' => try writer.writeAll("\\t"),
            else => try writer.writeByte(c),
        }
    }
}

// ============================================================================
// Pure Renderer: renderMetricsPayloadFromSnapshots
// ============================================================================

/// Renders the metrics payload JSON from already-collected interface stats snapshots.
/// This is a pure function suitable for testing with fixture data.
///
/// The caller owns the snapshots and must free them via
/// `linux_interface_stats.freeInterfaceStatsSnapshots()` after rendering.
pub fn renderMetricsPayloadFromSnapshots(
    writer: anytype,
    snapshots: []const InterfaceStatsSnapshot,
) !void {
    // Service and version header
    try writer.writeAll("{\"service\":\"tovarisch\",\"version\":\"");
    try writer.print("{s}\",\"metrics_version\":\"", .{service_version});
    try writer.print("{s}\",\"private_interfaces\":[", .{metrics_version});

    // Render each interface
    for (snapshots, 0..) |snap, i| {
        if (i > 0) try writer.writeAll(",");
        try writer.writeAll("{\"name\":\"");
        try writeJsonString(writer, snap.name);
        try writer.print(
            "\",\"rx_bytes\":{d},\"tx_bytes\":{d},\"rx_packets\":{d},\"tx_packets\":{d}",
            .{
                snap.stats.rx_bytes,
                snap.stats.tx_bytes,
                snap.stats.rx_packets,
                snap.stats.tx_packets,
            },
        );
        try writer.writeAll("}");
    }

    // Notes footer
    try writer.writeAll("],\"notes\":[\"interface counters are cumulative, not rates\",\"IPv4 private interfaces only; IPv6 is deferred\"]}");
}

// ============================================================================
// Fallback Renderer: renderMetricsFallbackPayload
// ============================================================================

/// Renders the fallback warning payload when live metrics collection fails.
/// Returns HTTP 200 with a valid JSON payload indicating the warning state.
pub fn renderMetricsFallbackPayload(writer: anytype) !void {
    try writer.writeAll(
        "{\"service\":\"tovarisch\",\"version\":\"0.1.1\",\"metrics_version\":\"0.1\",\"status\":\"warn\",\"private_interfaces\":[],\"error\":\"metrics_unavailable\",\"detail\":\"private interface stats unavailable\",\"notes\":[\"interface counters are cumulative, not rates\",\"IPv4 private interfaces only; IPv6 is deferred\"]}",
    );
}

// ============================================================================
// Live Renderer: renderLiveMetricsPayload
// ============================================================================

/// Collects live private interface stats and renders them.
/// Uses collectPrivateInterfaceStats() for sysfs + rtnetlink collection.
/// Frees collected snapshots after rendering.
pub fn renderLiveMetricsPayload(
    allocator: std.mem.Allocator,
    writer: anytype,
) !void {
    const sysfs_root = "/sys/class/net";
    const snapshots = try private_interface_stats.collectPrivateInterfaceStats(allocator, sysfs_root);
    defer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, snapshots);

    try renderMetricsPayloadFromSnapshots(writer, snapshots);
}

// ============================================================================
// Tests
// ============================================================================

test "writeJsonString handles normal string" {
    var buf: [256]u8 = undefined;
    var len: usize = 0;

    const writer = struct {
        buf: *[256]u8,
        len: *usize,

        pub fn writeAll(self: @This(), bytes: []const u8) !void {
            @memcpy(self.buf[self.len.*..][0..bytes.len], bytes);
            self.len.* += bytes.len;
        }

        pub fn writeByte(self: @This(), c: u8) !void {
            if (self.len.* >= 256) return error.BufferOverflow;
            self.buf[self.len.*] = c;
            self.len.* += 1;
        }

        pub fn print(self: @This(), comptime fmt: []const u8, args: anytype) !void {
            const written = std.fmt.bufPrint(self.buf[self.len.*..], fmt, args) catch return error.BufferOverflow;
            @memcpy(self.buf[self.len.*..][0..written.len], written);
            self.len.* += written.len;
        }
    }{ .buf = &buf, .len = &len };

    try writeJsonString(&writer, "eth0");
    try std.testing.expectEqualSlices(u8, "eth0", buf[0..len]);
}

test "writeJsonString escapes double quote" {
    var buf: [256]u8 = undefined;
    var len: usize = 0;

    const writer = struct {
        buf: *[256]u8,
        len: *usize,

        pub fn writeAll(self: @This(), bytes: []const u8) !void {
            @memcpy(self.buf[self.len.*..][0..bytes.len], bytes);
            self.len.* += bytes.len;
        }

        pub fn writeByte(self: @This(), c: u8) !void {
            if (self.len.* >= 256) return error.BufferOverflow;
            self.buf[self.len.*] = c;
            self.len.* += 1;
        }

        pub fn print(self: @This(), comptime fmt: []const u8, args: anytype) !void {
            const written = std.fmt.bufPrint(self.buf[self.len.*..], fmt, args) catch return error.BufferOverflow;
            @memcpy(self.buf[self.len.*..][0..written.len], written);
            self.len.* += written.len;
        }
    }{ .buf = &buf, .len = &len };

    try writeJsonString(&writer, "eth\"0");
    try std.testing.expectEqualSlices(u8, "eth\\\"0", buf[0..len]);
}

test "writeJsonString escapes backslash" {
    var buf: [256]u8 = undefined;
    var len: usize = 0;

    const writer = struct {
        buf: *[256]u8,
        len: *usize,

        pub fn writeAll(self: @This(), bytes: []const u8) !void {
            @memcpy(self.buf[self.len.*..][0..bytes.len], bytes);
            self.len.* += bytes.len;
        }

        pub fn writeByte(self: @This(), c: u8) !void {
            if (self.len.* >= 256) return error.BufferOverflow;
            self.buf[self.len.*] = c;
            self.len.* += 1;
        }

        pub fn print(self: @This(), comptime fmt: []const u8, args: anytype) !void {
            const written = std.fmt.bufPrint(self.buf[self.len.*..], fmt, args) catch return error.BufferOverflow;
            @memcpy(self.buf[self.len.*..][0..written.len], written);
            self.len.* += written.len;
        }
    }{ .buf = &buf, .len = &len };

    try writeJsonString(&writer, "eth\\0");
    try std.testing.expectEqualSlices(u8, "eth\\\\0", buf[0..len]);
}

test "writeJsonString escapes newline" {
    var buf: [256]u8 = undefined;
    var len: usize = 0;

    const writer = struct {
        buf: *[256]u8,
        len: *usize,

        pub fn writeAll(self: @This(), bytes: []const u8) !void {
            @memcpy(self.buf[self.len.*..][0..bytes.len], bytes);
            self.len.* += bytes.len;
        }

        pub fn writeByte(self: @This(), c: u8) !void {
            if (self.len.* >= 256) return error.BufferOverflow;
            self.buf[self.len.*] = c;
            self.len.* += 1;
        }

        pub fn print(self: @This(), comptime fmt: []const u8, args: anytype) !void {
            const written = std.fmt.bufPrint(self.buf[self.len.*..], fmt, args) catch return error.BufferOverflow;
            @memcpy(self.buf[self.len.*..][0..written.len], written);
            self.len.* += written.len;
        }
    }{ .buf = &buf, .len = &len };

    try writeJsonString(&writer, "eth\n0");
    try std.testing.expectEqualSlices(u8, "eth\\n0", buf[0..len]);
}
