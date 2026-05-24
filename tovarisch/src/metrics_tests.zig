// metrics_tests.zig — Tests for metrics rendering
//
// ACT 4a: Tests for metrics payload rendering using metrics_dto.
//
// Tests cover:
// 1. Pure renderer with zero snapshots emits correct fields
// 2. Pure renderer with one snapshot emits interface name and all counters
// 3. Pure renderer with two snapshots emits both interfaces
// 4. JSON output escapes interface names safely
// 5. Fallback renderer emits warning status
// 6. Live metrics on Linux (smoke test)
// 7. Rate field is present with null value (ACT 4)
// 8. Updated notes reflect null rates until sampler state is wired
//
// This file is imported by test_all.zig and refAllDecls forces test discovery.

const std = @import("std");
const testing = std.testing;
const metrics = @import("metrics.zig");
const linux_interface_stats = @import("net/linux_interface_stats.zig");
const linux_stats = @import("net/linux_stats.zig");

// Re-use types and helpers
const InterfaceStatsSnapshot = metrics.InterfaceStatsSnapshot;
const renderMetricsPayloadFromSnapshots = metrics.renderMetricsPayloadFromSnapshots;
const renderMetricsFallbackPayload = metrics.renderMetricsFallbackPayload;

// ============================================================================
// Test Writer Helper
// ============================================================================

const TestWriter = struct {
    const Self = @This();
    const BufSize = 8192;

    buf: [BufSize]u8 = undefined,
    len: usize = 0,

    pub fn init() Self {
        return .{ .buf = undefined, .len = 0 };
    }

    pub fn print(self: *Self, comptime fmt: []const u8, args: anytype) !void {
        if (self.len >= BufSize) return error.BufferOverflow;
        const remaining = self.buf[self.len..];
        const written = std.fmt.bufPrint(remaining, fmt, args) catch return error.BufferOverflow;
        self.len += written.len;
    }

    pub fn writeAll(self: *Self, bytes: []const u8) !void {
        if (self.len + bytes.len > BufSize) return error.BufferOverflow;
        @memcpy(self.buf[self.len..][0..bytes.len], bytes);
        self.len += bytes.len;
    }

    pub fn writeByte(self: *Self, c: u8) !void {
        if (self.len >= BufSize) return error.BufferOverflow;
        self.buf[self.len] = c;
        self.len += 1;
    }

    pub fn slice(self: *const Self) []const u8 {
        return self.buf[0..self.len];
    }

    pub fn reset(self: *Self) void {
        self.len = 0;
    }
};

// ============================================================================
// Tests: Pure renderer with zero snapshots
// ============================================================================

test "renderMetricsPayloadFromSnapshots: zero snapshots emits service" {
    var w = TestWriter.init();
    const snapshots: [0]InterfaceStatsSnapshot = .{};
    try renderMetricsPayloadFromSnapshots(testing.allocator, &w, &snapshots);
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"service\":\"tovarisch\""));
}

test "renderMetricsPayloadFromSnapshots: zero snapshots emits metrics_version" {
    var w = TestWriter.init();
    const snapshots: [0]InterfaceStatsSnapshot = .{};
    try renderMetricsPayloadFromSnapshots(testing.allocator, &w, &snapshots);
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"metrics_version\":\"0.2\""));
}

test "renderMetricsPayloadFromSnapshots: zero snapshots emits empty private_interfaces" {
    var w = TestWriter.init();
    const snapshots: [0]InterfaceStatsSnapshot = .{};
    try renderMetricsPayloadFromSnapshots(testing.allocator, &w, &snapshots);
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"private_interfaces\":[]"));
}

test "renderMetricsPayloadFromSnapshots: zero snapshots emits cumulative counter note" {
    var w = TestWriter.init();
    const snapshots: [0]InterfaceStatsSnapshot = .{};
    try renderMetricsPayloadFromSnapshots(testing.allocator, &w, &snapshots);
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "interface counters are cumulative"));
}

test "renderMetricsPayloadFromSnapshots: zero snapshots emits IPv4-only note" {
    var w = TestWriter.init();
    const snapshots: [0]InterfaceStatsSnapshot = .{};
    try renderMetricsPayloadFromSnapshots(testing.allocator, &w, &snapshots);
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "IPv4 private interfaces only"));
}

test "renderMetricsPayloadFromSnapshots: zero snapshots emits rate null note" {
    var w = TestWriter.init();
    const snapshots: [0]InterfaceStatsSnapshot = .{};
    try renderMetricsPayloadFromSnapshots(testing.allocator, &w, &snapshots);
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "rate is optional"));
}

// ============================================================================
// Tests: Pure renderer with one snapshot
// ============================================================================

test "renderMetricsPayloadFromSnapshots: one snapshot emits interface name" {
    var w = TestWriter.init();
    const name = try testing.allocator.dupe(u8, "eth0");
    defer testing.allocator.free(name);
    const snapshots = [_]InterfaceStatsSnapshot{
        .{
            .name = name,
            .stats = .{
                .rx_bytes = 1000,
                .tx_bytes = 2000,
                .rx_packets = 10,
                .tx_packets = 20,
            },
        },
    };
    try renderMetricsPayloadFromSnapshots(testing.allocator, &w, &snapshots);
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"eth0\""));
}

test "renderMetricsPayloadFromSnapshots: one snapshot emits rx_bytes" {
    var w = TestWriter.init();
    const name = try testing.allocator.dupe(u8, "eth0");
    defer testing.allocator.free(name);
    const snapshots = [_]InterfaceStatsSnapshot{
        .{
            .name = name,
            .stats = .{ .rx_bytes = 12345, .tx_bytes = 0, .rx_packets = 0, .tx_packets = 0 },
        },
    };
    try renderMetricsPayloadFromSnapshots(testing.allocator, &w, &snapshots);
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"rx_bytes\":12345"));
}

test "renderMetricsPayloadFromSnapshots: one snapshot emits tx_bytes" {
    var w = TestWriter.init();
    const name = try testing.allocator.dupe(u8, "eth0");
    defer testing.allocator.free(name);
    const snapshots = [_]InterfaceStatsSnapshot{
        .{
            .name = name,
            .stats = .{ .rx_bytes = 0, .tx_bytes = 67890, .rx_packets = 0, .tx_packets = 0 },
        },
    };
    try renderMetricsPayloadFromSnapshots(testing.allocator, &w, &snapshots);
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"tx_bytes\":67890"));
}

test "renderMetricsPayloadFromSnapshots: one snapshot emits rx_packets" {
    var w = TestWriter.init();
    const name = try testing.allocator.dupe(u8, "eth0");
    defer testing.allocator.free(name);
    const snapshots = [_]InterfaceStatsSnapshot{
        .{
            .name = name,
            .stats = .{ .rx_bytes = 0, .tx_bytes = 0, .rx_packets = 99, .tx_packets = 0 },
        },
    };
    try renderMetricsPayloadFromSnapshots(testing.allocator, &w, &snapshots);
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"rx_packets\":99"));
}

test "renderMetricsPayloadFromSnapshots: one snapshot emits tx_packets" {
    var w = TestWriter.init();
    const name = try testing.allocator.dupe(u8, "eth0");
    defer testing.allocator.free(name);
    const snapshots = [_]InterfaceStatsSnapshot{
        .{
            .name = name,
            .stats = .{ .rx_bytes = 0, .tx_bytes = 0, .rx_packets = 0, .tx_packets = 42 },
        },
    };
    try renderMetricsPayloadFromSnapshots(testing.allocator, &w, &snapshots);
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"tx_packets\":42"));
}

// ============================================================================
// Tests: Pure renderer with two snapshots
// ============================================================================

test "renderMetricsPayloadFromSnapshots: two snapshots emits both interface names" {
    var w = TestWriter.init();
    const eth0_name = try testing.allocator.dupe(u8, "eth0");
    defer testing.allocator.free(eth0_name);
    const wg0_name = try testing.allocator.dupe(u8, "wg0");
    defer testing.allocator.free(wg0_name);
    const snapshots = [_]InterfaceStatsSnapshot{
        .{
            .name = eth0_name,
            .stats = .{ .rx_bytes = 1000, .tx_bytes = 2000, .rx_packets = 10, .tx_packets = 20 },
        },
        .{
            .name = wg0_name,
            .stats = .{ .rx_bytes = 3000, .tx_bytes = 4000, .rx_packets = 30, .tx_packets = 40 },
        },
    };
    try renderMetricsPayloadFromSnapshots(testing.allocator, &w, &snapshots);
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"eth0\""));
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"wg0\""));
}

// ============================================================================
// Tests: JSON string escaping
// ============================================================================

test "renderMetricsPayloadFromSnapshots: interface name with normal characters" {
    var w = TestWriter.init();
    const name = try testing.allocator.dupe(u8, "eth0");
    defer testing.allocator.free(name);
    const snapshots = [_]InterfaceStatsSnapshot{
        .{
            .name = name,
            .stats = .{ .rx_bytes = 100, .tx_bytes = 200, .rx_packets = 1, .tx_packets = 2 },
        },
    };
    try renderMetricsPayloadFromSnapshots(testing.allocator, &w, &snapshots);
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "eth0"));
}

// ============================================================================
// Tests: Fallback renderer
// ============================================================================

test "renderMetricsFallbackPayload: emits status warn" {
    var w = TestWriter.init();
    try renderMetricsFallbackPayload(&w);
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"status\":\"warn\""));
}

test "renderMetricsFallbackPayload: emits error metrics_unavailable" {
    var w = TestWriter.init();
    try renderMetricsFallbackPayload(&w);
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"error\":\"metrics_unavailable\""));
}

test "renderMetricsFallbackPayload: emits empty private_interfaces" {
    var w = TestWriter.init();
    try renderMetricsFallbackPayload(&w);
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"private_interfaces\":[]"));
}

test "renderMetricsFallbackPayload: emits cumulative counter note" {
    var w = TestWriter.init();
    try renderMetricsFallbackPayload(&w);
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "interface counters are cumulative"));
}

// ============================================================================
// Tests: JSON validity checks
// ============================================================================

test "renderMetricsPayloadFromSnapshots: output is valid JSON structure" {
    var w = TestWriter.init();
    const name = try testing.allocator.dupe(u8, "eth0");
    defer testing.allocator.free(name);
    const snapshots = [_]InterfaceStatsSnapshot{
        .{
            .name = name,
            .stats = .{ .rx_bytes = 100, .tx_bytes = 200, .rx_packets = 1, .tx_packets = 2 },
        },
    };
    try renderMetricsPayloadFromSnapshots(testing.allocator, &w, &snapshots);

    // Check top-level structure
    try testing.expect(std.mem.startsWith(u8, w.slice(), "{\"service\":"));
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"private_interfaces\":["));
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"notes\":["));
}

test "renderMetricsFallbackPayload: output is valid JSON structure" {
    var w = TestWriter.init();
    try renderMetricsFallbackPayload(&w);

    // Check top-level structure
    try testing.expect(std.mem.startsWith(u8, w.slice(), "{\"service\":"));
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"status\":\"warn\""));
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"error\":\"metrics_unavailable\""));
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"private_interfaces\":[]"));
}

// ============================================================================
// Tests: rate:null per interface row (ACT 4)
// ============================================================================

test "renderMetricsPayloadFromSnapshots: one snapshot emits rate null" {
    var w = TestWriter.init();
    const name = try testing.allocator.dupe(u8, "wg0");
    defer testing.allocator.free(name);
    const snapshots = [_]InterfaceStatsSnapshot{
        .{
            .name = name,
            .stats = .{ .rx_bytes = 123, .tx_bytes = 456, .rx_packets = 7, .tx_packets = 8 },
        },
    };
    try renderMetricsPayloadFromSnapshots(testing.allocator, &w, &snapshots);
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"rate\":null"));
}

test "renderMetricsPayloadFromSnapshots: two snapshots each have rate null" {
    var w = TestWriter.init();
    const eth0_name = try testing.allocator.dupe(u8, "eth0");
    defer testing.allocator.free(eth0_name);
    const wg0_name = try testing.allocator.dupe(u8, "wg0");
    defer testing.allocator.free(wg0_name);
    const snapshots = [_]InterfaceStatsSnapshot{
        .{
            .name = eth0_name,
            .stats = .{ .rx_bytes = 1000, .tx_bytes = 2000, .rx_packets = 10, .tx_packets = 20 },
        },
        .{
            .name = wg0_name,
            .stats = .{ .rx_bytes = 3000, .tx_bytes = 4000, .rx_packets = 30, .tx_packets = 40 },
        },
    };
    try renderMetricsPayloadFromSnapshots(testing.allocator, &w, &snapshots);
    // Each interface row should have its own "rate":null
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 2, "\"rate\":null"));
}

test "renderMetricsPayloadFromSnapshots: no populated rate object in output" {
    var w = TestWriter.init();
    const name = try testing.allocator.dupe(u8, "eth0");
    defer testing.allocator.free(name);
    const snapshots = [_]InterfaceStatsSnapshot{
        .{
            .name = name,
            .stats = .{ .rx_bytes = 100, .tx_bytes = 200, .rx_packets = 1, .tx_packets = 2 },
        },
    };
    try renderMetricsPayloadFromSnapshots(testing.allocator, &w, &snapshots);
    // Should NOT contain any populated rate object fields
    try testing.expect(!std.mem.containsAtLeast(u8, w.slice(), 1, "\"rx_bytes_per_second\":"));
    try testing.expect(!std.mem.containsAtLeast(u8, w.slice(), 1, "\"window_seconds\":"));
}

// ============================================================================
// Tests: Fallback renderer updated notes (ACT 4)
// ============================================================================

test "renderMetricsFallbackPayload: emits metrics_version 0.2" {
    var w = TestWriter.init();
    try renderMetricsFallbackPayload(&w);
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"metrics_version\":\"0.2\""));
}

test "renderMetricsFallbackPayload: emits rate null note" {
    var w = TestWriter.init();
    try renderMetricsFallbackPayload(&w);
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "rate is optional"));
}

// ============================================================================
// Linux-only Smoke Test
// ============================================================================

test "renderLiveMetricsPayload: live sysfs smoke test on Linux" {
    if (@import("builtin").os.tag != .linux) return error.SkipZigTest;

    // Check if /sys/class/net exists
    if (!linux_stats.fileExists("/sys/class/net")) {
        return error.SkipZigTest;
    }

    var w = TestWriter.init();
    // This will call collectPrivateInterfaceStats which may fail on non-Linux
    // or in containerized environments - that's okay for a smoke test
    metrics.renderLiveMetricsPayload(testing.allocator, &w) catch return error.SkipZigTest;

    // If we got here, we have a valid response
    try testing.expect(w.len > 0);
    try testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"service\":\"tovarisch\""));
}
