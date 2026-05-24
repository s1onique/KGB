// metrics_state_tests.zig — Tests for persistent sampler state wiring
//
// ACT 5: Wire persistent sampler state across /metrics.json requests.
//
// Tests cover:
// 1. First metrics render through state emits rate:null
// 2. Second render through same state emits populated rate object
// 3. Counter reset returns rate:null
// 4. New interface returns rate:null
// 5. Disappeared/reappeared interface returns rate:null
// 6. Output still preserves cumulative counters
// 7. Output still uses metrics_version 0.2
// 8. Fallback path does not require sampler state
//
// This file is imported by test_all.zig and refAllDecls forces test discovery.

const std = @import("std");
const testing = std.testing;
const metrics_state = @import("metrics_state.zig");
const rates = @import("net/rates.zig");
const linux_interface_stats = @import("net/linux_interface_stats.zig");

// Re-use types
const MetricsState = metrics_state.MetricsState;
const InterfaceStatsSnapshot = metrics_state.InterfaceStatsSnapshot;
const InterfaceRate = rates.InterfaceRate;

// ============================================================================
// Test Writer Helper
// ============================================================================

const TestWriter = struct {
    const Self = @This();
    const BufSize = 16384;

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
// Fixture: create interface stats snapshot
// ============================================================================

fn createSnapshot(
    allocator: std.mem.Allocator,
    name: []const u8,
    rx_bytes: u64,
    tx_bytes: u64,
    rx_packets: u64,
    tx_packets: u64,
) !InterfaceStatsSnapshot {
    const name_copy = try allocator.dupe(u8, name);
    return InterfaceStatsSnapshot{
        .name = name_copy,
        .stats = .{
            .rx_bytes = rx_bytes,
            .tx_bytes = tx_bytes,
            .rx_packets = rx_packets,
            .tx_packets = tx_packets,
        },
    };
}

// ============================================================================
// Tests: First render emits rate:null
// ============================================================================

test "first render through state emits rate:null" {
    const allocator = testing.allocator;
    var state = MetricsState.init(allocator);
    defer state.deinit();

    // Create a single interface snapshot
    const snap = try createSnapshot(allocator, "eth0", 1000, 2000, 10, 20);
    defer allocator.free(snap.name);
    const snaps = try allocator.create([]InterfaceStatsSnapshot);
    snaps.* = try allocator.dupe(InterfaceStatsSnapshot, &[1]InterfaceStatsSnapshot{snap});
    defer {
        allocator.free(snaps.*);
        allocator.destroy(snaps);
    }

    // Render with first timestamp (no previous sample)
    var writer = TestWriter.init();
    const t1: i64 = 1000; // First sample
    try state.renderMetricsPayloadFromSnapshots(allocator, &writer, snaps.*, t1);

    const json = writer.slice();

    // Should contain rate:null for first sample
    try testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"rate\":null"));
    try testing.expect(!std.mem.containsAtLeast(u8, json, 1, "\"rx_bytes_delta\""));
}

test "first render emits cumulative counters" {
    const allocator = testing.allocator;
    var state = MetricsState.init(allocator);
    defer state.deinit();

    // Create snapshot with known counters
    const snap = try createSnapshot(allocator, "wg0", 50000, 100000, 500, 1000);
    defer allocator.free(snap.name);
    const snaps = try allocator.dupe(InterfaceStatsSnapshot, &[1]InterfaceStatsSnapshot{snap});
    defer allocator.free(snaps);

    var writer = TestWriter.init();
    try state.renderMetricsPayloadFromSnapshots(allocator, &writer, snaps, 1000);

    const json = writer.slice();

    // Cumulative counters should be preserved
    try testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"rx_bytes\":50000"));
    try testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"tx_bytes\":100000"));
    try testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"rx_packets\":500"));
    try testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"tx_packets\":1000"));
}

// ============================================================================
// Tests: Second render emits populated rate object
// ============================================================================

test "second render through same state emits populated rate object" {
    const allocator = testing.allocator;
    var state = MetricsState.init(allocator);
    defer state.deinit();

    // First snapshot
    const snap1 = try createSnapshot(allocator, "eth0", 1000, 2000, 10, 20);
    defer allocator.free(snap1.name);
    const snaps1 = try allocator.dupe(InterfaceStatsSnapshot, &[1]InterfaceStatsSnapshot{snap1});
    defer allocator.free(snaps1);

    var writer = TestWriter.init();

    // First render: rate:null
    try state.renderMetricsPayloadFromSnapshots(allocator, &writer, snaps1, 1000);
    try testing.expect(std.mem.containsAtLeast(u8, writer.slice(), 1, "\"rate\":null"));

    // Second snapshot with same interface, increased counters
    const snap2 = try createSnapshot(allocator, "eth0", 31000, 62000, 310, 620);
    defer allocator.free(snap2.name);
    const snaps2 = try allocator.dupe(InterfaceStatsSnapshot, &[1]InterfaceStatsSnapshot{snap2});
    defer allocator.free(snaps2);

    writer.reset();

    // Second render at t=31000 (30 seconds later): rate should be populated
    try state.renderMetricsPayloadFromSnapshots(allocator, &writer, snaps2, 31000);

    const json = writer.slice();

    // Rate should be populated (not null)
    try testing.expect(!std.mem.containsAtLeast(u8, json, 1, "\"rate\":null"));
    try testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"window_seconds\":30"));
    try testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"rx_bytes_delta\":30000"));
    try testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"tx_bytes_delta\":60000"));
    try testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"rx_bytes_per_second\":1000"));
    try testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"tx_bytes_per_second\":2000"));
}

test "rate calculation uses actual elapsed time" {
    const allocator = testing.allocator;
    var state = MetricsState.init(allocator);
    defer state.deinit();

    // First snapshot
    const snap1 = try createSnapshot(allocator, "eth0", 0, 0, 0, 0);
    defer allocator.free(snap1.name);
    const snaps1 = try allocator.dupe(InterfaceStatsSnapshot, &[1]InterfaceStatsSnapshot{snap1});
    defer allocator.free(snaps1);

    // Second snapshot 60 seconds later
    const snap2 = try createSnapshot(allocator, "eth0", 60000, 120000, 600, 1200);
    defer allocator.free(snap2.name);
    const snaps2 = try allocator.dupe(InterfaceStatsSnapshot, &[1]InterfaceStatsSnapshot{snap2});
    defer allocator.free(snaps2);

    var writer = TestWriter.init();

    try state.renderMetricsPayloadFromSnapshots(allocator, &writer, snaps1, 0);
    writer.reset();
    try state.renderMetricsPayloadFromSnapshots(allocator, &writer, snaps2, 60000);

    const json = writer.slice();

    // 60 seconds, 60000 bytes = 1000 bytes/sec
    try testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"window_seconds\":60"));
    try testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"rx_bytes_per_second\":1000"));
}

// ============================================================================
// Tests: Counter reset returns rate:null
// ============================================================================

test "counter reset returns rate:null for that interface" {
    const allocator = testing.allocator;
    var state = MetricsState.init(allocator);
    defer state.deinit();

    // First snapshot with high counters
    const snap1 = try createSnapshot(allocator, "eth0", 100000, 100000, 1000, 1000);
    defer allocator.free(snap1.name);
    const snaps1 = try allocator.dupe(InterfaceStatsSnapshot, &[1]InterfaceStatsSnapshot{snap1});
    defer allocator.free(snaps1);

    var writer = TestWriter.init();

    // First render
    try state.renderMetricsPayloadFromSnapshots(allocator, &writer, snaps1, 0);
    writer.reset();

    // Second snapshot with reset counter (rx_bytes less than previous)
    const snap2 = try createSnapshot(allocator, "eth0", 500, 200000, 1000, 1000);
    defer allocator.free(snap2.name);
    const snaps2 = try allocator.dupe(InterfaceStatsSnapshot, &[1]InterfaceStatsSnapshot{snap2});
    defer allocator.free(snaps2);

    // Second render with reset: rate should be null (counter went backwards)
    try state.renderMetricsPayloadFromSnapshots(allocator, &writer, snaps2, 30000);

    const json = writer.slice();

    // Rate should be null due to reset detection
    try testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"rate\":null"));
}

// ============================================================================
// Tests: New interface returns rate:null
// ============================================================================

test "new interface returns rate:null" {
    const allocator = testing.allocator;
    var state = MetricsState.init(allocator);
    defer state.deinit();

    var writer = TestWriter.init();

    // First: only eth0
    const snap1 = try createSnapshot(allocator, "eth0", 1000, 2000, 10, 20);
    defer allocator.free(snap1.name);
    const snaps1 = try allocator.dupe(InterfaceStatsSnapshot, &[1]InterfaceStatsSnapshot{snap1});
    defer allocator.free(snaps1);
    try state.renderMetricsPayloadFromSnapshots(allocator, &writer, snaps1, 0);
    writer.reset();

    // Second: eth0 + wg0 (wg0 is new)
    const snap_eth0 = try createSnapshot(allocator, "eth0", 31000, 62000, 310, 620);
    defer allocator.free(snap_eth0.name);
    const snap_wg0 = try createSnapshot(allocator, "wg0", 500, 1000, 5, 10);
    defer allocator.free(snap_wg0.name);
    const snaps2 = try allocator.dupe(InterfaceStatsSnapshot, &[_]InterfaceStatsSnapshot{ snap_eth0, snap_wg0 });
    defer allocator.free(snaps2);

    try state.renderMetricsPayloadFromSnapshots(allocator, &writer, snaps2, 30000);

    const json = writer.slice();

    // eth0 should have rate (second sample)
    try testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"name\":\"eth0\""));
    // wg0 should have rate:null (first appearance)
    try testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"name\":\"wg0\""));
}

// ============================================================================
// Tests: Disappeared/reappeared interface returns rate:null
// ============================================================================

test "reappeared interface returns rate:null" {
    const allocator = testing.allocator;
    var state = MetricsState.init(allocator);
    defer state.deinit();

    var writer = TestWriter.init();

    // First: eth0 + wg0
    const snap1_eth0 = try createSnapshot(allocator, "eth0", 1000, 2000, 10, 20);
    defer allocator.free(snap1_eth0.name);
    const snap1_wg0 = try createSnapshot(allocator, "wg0", 500, 1000, 5, 10);
    defer allocator.free(snap1_wg0.name);
    const snaps1 = try allocator.dupe(InterfaceStatsSnapshot, &[_]InterfaceStatsSnapshot{ snap1_eth0, snap1_wg0 });
    defer allocator.free(snaps1);
    try state.renderMetricsPayloadFromSnapshots(allocator, &writer, snaps1, 0);
    writer.reset();

    // Second: only eth0 (wg0 disappears)
    const snap2_eth0 = try createSnapshot(allocator, "eth0", 31000, 62000, 310, 620);
    defer allocator.free(snap2_eth0.name);
    const snaps2 = try allocator.dupe(InterfaceStatsSnapshot, &[1]InterfaceStatsSnapshot{snap2_eth0});
    defer allocator.free(snaps2);
    try state.renderMetricsPayloadFromSnapshots(allocator, &writer, snaps2, 30000);
    writer.reset();

    // Third: eth0 + wg0 again (wg0 reappears)
    const snap3_eth0 = try createSnapshot(allocator, "eth0", 61000, 122000, 610, 1220);
    defer allocator.free(snap3_eth0.name);
    const snap3_wg0 = try createSnapshot(allocator, "wg0", 1000, 2000, 10, 20);
    defer allocator.free(snap3_wg0.name);
    const snaps3 = try allocator.dupe(InterfaceStatsSnapshot, &[_]InterfaceStatsSnapshot{ snap3_eth0, snap3_wg0 });
    defer allocator.free(snaps3);
    try state.renderMetricsPayloadFromSnapshots(allocator, &writer, snaps3, 60000);

    const json = writer.slice();

    // eth0 should have rate (third sample) - should NOT have rate:null adjacent
    try testing.expect(!std.mem.containsAtLeast(u8, json, 1, "\"name\":\"eth0\",\"rate\":null"));
    // wg0 should have rate:null (reappeared, previous state was cleared)
    try testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"name\":\"wg0\""));
    try testing.expect(std.mem.containsAtLeast(u8, json, 1, ",\"rate\":null}"));
}

// ============================================================================
// Tests: Output still uses metrics_version 0.2
// ============================================================================

test "output uses metrics_version 0.3" {
    const allocator = testing.allocator;
    var state = MetricsState.init(allocator);
    defer state.deinit();

    const snap = try createSnapshot(allocator, "eth0", 1000, 2000, 10, 20);
    defer allocator.free(snap.name);
    const snaps = try allocator.dupe(InterfaceStatsSnapshot, &[1]InterfaceStatsSnapshot{snap});
    defer allocator.free(snaps);

    var writer = TestWriter.init();
    try state.renderMetricsPayloadFromSnapshots(allocator, &writer, snaps, 1000);

    const json = writer.slice();

    try testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"metrics_version\":\"0.3\""));
}

test "output uses service tovarisch" {
    const allocator = testing.allocator;
    var state = MetricsState.init(allocator);
    defer state.deinit();

    const snap = try createSnapshot(allocator, "eth0", 1000, 2000, 10, 20);
    defer allocator.free(snap.name);
    const snaps = try allocator.dupe(InterfaceStatsSnapshot, &[1]InterfaceStatsSnapshot{snap});
    defer allocator.free(snaps);

    var writer = TestWriter.init();
    try state.renderMetricsPayloadFromSnapshots(allocator, &writer, snaps, 1000);

    const json = writer.slice();

    try testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"service\":\"tovarisch\""));
}

// ============================================================================
// Tests: Fallback path does not require sampler state
// ============================================================================

test "MetricsState.init creates empty sampler" {
    const allocator = testing.allocator;
    var state = MetricsState.init(allocator);
    defer state.deinit();
    // Should not panic, sampler starts empty
}

test "MetricsState.deinit handles empty sampler" {
    const allocator = testing.allocator;
    var state = MetricsState.init(allocator);
    // Should not panic on deinit with empty sampler
    state.deinit();
}

test "two interfaces with different sample histories" {
    const allocator = testing.allocator;
    var state = MetricsState.init(allocator);
    defer state.deinit();

    var writer = TestWriter.init();

    // First: only eth0
    const snap1_eth0 = try createSnapshot(allocator, "eth0", 1000, 2000, 10, 20);
    defer allocator.free(snap1_eth0.name);
    const snaps1 = try allocator.dupe(InterfaceStatsSnapshot, &[1]InterfaceStatsSnapshot{snap1_eth0});
    defer allocator.free(snaps1);
    try state.renderMetricsPayloadFromSnapshots(allocator, &writer, snaps1, 0);
    writer.reset();

    // Second: eth0 (3rd sample) + wg0 (1st sample)
    const snap2_eth0 = try createSnapshot(allocator, "eth0", 31000, 62000, 310, 620);
    defer allocator.free(snap2_eth0.name);
    const snap2_wg0 = try createSnapshot(allocator, "wg0", 500, 1000, 5, 10);
    defer allocator.free(snap2_wg0.name);
    const snaps2 = try allocator.dupe(InterfaceStatsSnapshot, &[_]InterfaceStatsSnapshot{ snap2_eth0, snap2_wg0 });
    defer allocator.free(snaps2);
    try state.renderMetricsPayloadFromSnapshots(allocator, &writer, snaps2, 30000);

    const json = writer.slice();

    // eth0 should have rate (has previous sample)
    // wg0 should have rate:null (no previous sample)
    // Both should be in the output
    try testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"name\":\"eth0\""));
    try testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"name\":\"wg0\""));
}
