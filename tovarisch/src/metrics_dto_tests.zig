// metrics_dto_tests.zig — Unit tests for metrics_dto.zig
//
// ACT 3: Tests for metrics JSON DTO rendering with optional interface rates.

const std = @import("std");
const rates = @import("net/rates.zig");
const sampler = @import("net/interface_sampler.zig");
const metrics_dto = @import("metrics_dto.zig");
const telemetry = @import("runtime/telemetry.zig");

// Re-export types for convenience
const SampledInterface = metrics_dto.SampledInterface;
const renderSampledInterfacesPayload = metrics_dto.renderSampledInterfacesPayload;
const writeJsonString = metrics_dto.writeJsonString;

// Test runtime telemetry - use known values for deterministic tests
const testRuntime = telemetry.RuntimeTelemetry{ .pid = 1234, .rss_kib = 1920 };

// ============================================================================
// Test Writer Helper
// ============================================================================

const TestWriter = struct {
    const Self = @This();
    const BufSize: usize = 8192;

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
};

// Helper to create a test SampledInterface with owned name
fn makeTestSampledInterface(
    allocator: std.mem.Allocator,
    name: []const u8,
    rx_bytes: u64,
    tx_bytes: u64,
    rx_packets: u64,
    tx_packets: u64,
    sampled_at_ms: i64,
    rate: ?rates.InterfaceRate,
) !sampler.SampledInterface {
    const owned_name = try allocator.dupe(u8, name);
    return .{
        .sample = .{
            .name = owned_name,
            .rx_bytes = rx_bytes,
            .tx_bytes = tx_bytes,
            .rx_packets = rx_packets,
            .tx_packets = tx_packets,
            .sampled_at_ms = sampled_at_ms,
        },
        .rate = rate,
    };
}

// ============================================================================
// Tests: Zero interfaces
// ============================================================================

test "renderSampledInterfacesPayload: zero interfaces emits service" {
    var w = TestWriter.init();
    const sampled: [0]SampledInterface = .{};
    try renderSampledInterfacesPayload(&w, &sampled, testRuntime);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"service\":\"tovarisch\""));
}

test "renderSampledInterfacesPayload: zero interfaces emits metrics_version 0.3" {
    var w = TestWriter.init();
    const sampled: [0]SampledInterface = .{};
    try renderSampledInterfacesPayload(&w, &sampled, testRuntime);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"metrics_version\":\"0.3\""));
}

test "renderSampledInterfacesPayload: zero interfaces emits empty array" {
    var w = TestWriter.init();
    const sampled: [0]SampledInterface = .{};
    try renderSampledInterfacesPayload(&w, &sampled, testRuntime);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"private_interfaces\":[]"));
}

test "renderSampledInterfacesPayload: emits rate is null note" {
    var w = TestWriter.init();
    const sampled: [0]SampledInterface = .{};
    try renderSampledInterfacesPayload(&w, &sampled, testRuntime);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "rate is null until a previous sample exists"));
}

// ============================================================================
// Tests: One interface without rate
// ============================================================================

test "renderSampledInterfacesPayload: one interface without rate emits rate:null" {
    const allocator = std.testing.allocator;
    const si = try makeTestSampledInterface(allocator, "wg0", 1000, 2000, 10, 20, 1000, null);
    defer allocator.free(si.sample.name);

    var w = TestWriter.init();
    const sampled = [_]SampledInterface{si};
    try renderSampledInterfacesPayload(&w, &sampled, testRuntime);

    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"wg0\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"rx_bytes\":1000"));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"tx_bytes\":2000"));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"rx_packets\":10"));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"tx_packets\":20"));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"rate\":null"));
}

// ============================================================================
// Tests: One interface with rate
// ============================================================================

test "renderSampledInterfacesPayload: one interface with rate" {
    const allocator = std.testing.allocator;
    const rate = rates.InterfaceRate{
        .window_seconds = 30,
        .rx_bytes_delta = 30000,
        .tx_bytes_delta = 60000,
        .rx_packets_delta = 300,
        .tx_packets_delta = 600,
        .rx_bytes_per_second = 1000,
        .tx_bytes_per_second = 2000,
        .rx_packets_per_second = 10,
        .tx_packets_per_second = 20,
    };
    const si = try makeTestSampledInterface(allocator, "eth0", 31000, 62000, 310, 620, 30000, rate);
    defer allocator.free(si.sample.name);

    var w = TestWriter.init();
    const sampled = [_]SampledInterface{si};
    try renderSampledInterfacesPayload(&w, &sampled, testRuntime);

    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"eth0\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"rate\":{"));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"window_seconds\":30"));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"rx_bytes_delta\":30000"));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"tx_bytes_delta\":60000"));
}

// ============================================================================
// Tests: Two interfaces mixed
// ============================================================================

test "renderSampledInterfacesPayload: two interfaces one with rate one without" {
    const allocator = std.testing.allocator;

    const rate = rates.InterfaceRate{
        .window_seconds = 30,
        .rx_bytes_delta = 30000,
        .tx_bytes_delta = 60000,
        .rx_packets_delta = 300,
        .tx_packets_delta = 600,
        .rx_bytes_per_second = 1000,
        .tx_bytes_per_second = 2000,
        .rx_packets_per_second = 10,
        .tx_packets_per_second = 20,
    };
    const si1 = try makeTestSampledInterface(allocator, "wg0", 31000, 62000, 310, 620, 30000, rate);
    defer allocator.free(si1.sample.name);

    const si2 = try makeTestSampledInterface(allocator, "eth0", 1000, 2000, 10, 20, 1000, null);
    defer allocator.free(si2.sample.name);

    var w = TestWriter.init();
    const sampled = [_]SampledInterface{ si1, si2 };
    try renderSampledInterfacesPayload(&w, &sampled, testRuntime);

    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"wg0\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"name\":\"eth0\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"rate\":{"));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"rate\":null"));
}

// ============================================================================
// Tests: JSON structure validation
// ============================================================================

test "renderSampledInterfacesPayload: output is valid JSON structure" {
    const allocator = std.testing.allocator;
    const si = try makeTestSampledInterface(allocator, "eth0", 100, 200, 1, 2, 1000, null);
    defer allocator.free(si.sample.name);

    var w = TestWriter.init();
    const sampled = [_]SampledInterface{si};
    try renderSampledInterfacesPayload(&w, &sampled, testRuntime);

    try std.testing.expect(std.mem.startsWith(u8, w.slice(), "{\"service\":"));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"private_interfaces\":["));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"notes\":["));
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "}"));
}

// ============================================================================
// Tests: Runtime telemetry in output
// ============================================================================

test "renderSampledInterfacesPayload: emits runtime pid" {
    var w = TestWriter.init();
    const sampled: [0]SampledInterface = .{};
    try renderSampledInterfacesPayload(&w, &sampled, testRuntime);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"pid\":1234"));
}

test "renderSampledInterfacesPayload: emits runtime rss_kib" {
    var w = TestWriter.init();
    const sampled: [0]SampledInterface = .{};
    try renderSampledInterfacesPayload(&w, &sampled, testRuntime);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"rss_kib\":1920"));
}

test "renderSampledInterfacesPayload: emits runtime block" {
    var w = TestWriter.init();
    const sampled: [0]SampledInterface = .{};
    try renderSampledInterfacesPayload(&w, &sampled, testRuntime);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "\"runtime\":{"));
}

test "renderSampledInterfacesPayload: emits runtime RSS best-effort note" {
    var w = TestWriter.init();
    const sampled: [0]SampledInterface = .{};
    try renderSampledInterfacesPayload(&w, &sampled, testRuntime);
    try std.testing.expect(std.mem.containsAtLeast(u8, w.slice(), 1, "runtime RSS is best-effort platform telemetry"));
}

// ============================================================================
// Tests: JSON string escaping
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
    }{ .buf = &buf, .len = &len };

    try writeJsonString(&writer, "eth\n0");
    try std.testing.expectEqualSlices(u8, "eth\\n0", buf[0..len]);
}
