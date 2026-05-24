// metrics_tunnel_contract_tests.zig — Tunnel contract verification tests
//
// ACT: Metrics contract verification for tunnel_count consistency.
//
// This file verifies:
// 1. tunnel_count field exists in output
// 2. tunnel_count equals count of tunnel_interfaces entries
// 3. Tunnel interfaces have matching is_tunnel=true in private_interfaces
// 4. Zero tunnels edge case
//
// Tests are split from metrics_dto_tests.zig to maintain LLM-friendliness limits.

const std = @import("std");
const rates = @import("net/rates.zig");
const sampler = @import("net/interface_sampler.zig");
const metrics_dto = @import("metrics_dto.zig");
const telemetry = @import("runtime/telemetry.zig");

// Re-export types for convenience
const SampledInterface = metrics_dto.SampledInterface;
const renderSampledInterfacesPayload = metrics_dto.renderSampledInterfacesPayload;

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
    is_tunnel: bool,
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
        .is_tunnel = is_tunnel,
    };
}

// ============================================================================
// Tests: Tunnel contract verification - tunnel_count consistency
// ============================================================================

// Contract test: tunnel_count field exists in output.
test "tunnel contract: tunnel_count field exists" {
    const allocator = std.testing.allocator;

    const si = try makeTestSampledInterface(allocator, "eth0", 100, 200, 1, 2, 1000, null, false);
    defer allocator.free(si.sample.name);

    var w = TestWriter.init();
    const sampled = [_]SampledInterface{si};
    try renderSampledInterfacesPayload(&w, &sampled, testRuntime);

    const slice = w.slice();
    // tunnel_count field must be present (e.g., "tunnel_count":0 or "tunnel_count":1)
    try std.testing.expect(std.mem.containsAtLeast(u8, slice, 1, "\"tunnel_count\":"));
}

// Contract test: tunnel_count equals count of tunnel_interfaces entries.
test "tunnel contract: tunnel_count equals tunnel_interfaces array length" {
    const allocator = std.testing.allocator;

    // Three interfaces: wg0, wg1 are tunnels, eth0 is not
    const si1 = try makeTestSampledInterface(allocator, "wg0", 100, 200, 1, 2, 1000, null, true);
    defer allocator.free(si1.sample.name);

    const si2 = try makeTestSampledInterface(allocator, "wg1", 300, 400, 3, 4, 1000, null, true);
    defer allocator.free(si2.sample.name);

    const si3 = try makeTestSampledInterface(allocator, "eth0", 500, 600, 5, 6, 1000, null, false);
    defer allocator.free(si3.sample.name);

    var w = TestWriter.init();
    const sampled = [_]SampledInterface{ si1, si2, si3 };
    try renderSampledInterfacesPayload(&w, &sampled, testRuntime);

    const slice = w.slice();

    // Count the number of quoted names in tunnel_interfaces array
    const marker = "\"tunnel_interfaces\":[";
    const tunnel_array_start = std.mem.indexOf(u8, slice, marker) orelse return error.MissingField;
    const after_bracket = slice[tunnel_array_start + marker.len..];
    const array_end = std.mem.indexOf(u8, after_bracket, "]") orelse return error.MissingField;
    const tunnel_array_contents = after_bracket[0..array_end];
    // Count interface names by counting quotes and dividing by 2
    const actual_tunnel_count = std.mem.count(u8, tunnel_array_contents, "\"") / 2;

    // Extract tunnel_count value - it comes after "tunnel_count":
    const count_marker = "\"tunnel_count\":";
    const count_pos = std.mem.indexOf(u8, slice, count_marker) orelse return error.MissingField;
    const value_start = count_pos + count_marker.len;
    const value_str = slice[value_start..];

    // Parse the number - find end by looking for non-digit
    var num_buf: [16]u8 = undefined;
    var num_len: usize = 0;
    for (value_str) |c| {
        if (c >= '0' and c <= '9') {
            if (num_len >= num_buf.len) return error.BufferOverflow;
            num_buf[num_len] = c;
            num_len += 1;
        } else {
            break;
        }
    }
    if (num_len == 0) return error.MissingCountValue;
    const count_value = try std.fmt.parseInt(usize, num_buf[0..num_len], 10);

    try std.testing.expectEqual(actual_tunnel_count, count_value);
}

// Contract test: tunnel interfaces appear in private_interfaces with is_tunnel=true.
// Verifies cross-reference consistency between tunnel_interfaces array and the
// is_tunnel flag on corresponding private_interfaces entries.
test "tunnel contract: tunnel_interfaces entries have matching is_tunnel=true" {
    const allocator = std.testing.allocator;

    // Create mixed interfaces: tunnels (wg0, tun0) and non-tunnels (eth0)
    const si1 = try makeTestSampledInterface(allocator, "wg0", 100, 200, 1, 2, 1000, null, true);
    defer allocator.free(si1.sample.name);

    const si2 = try makeTestSampledInterface(allocator, "eth0", 300, 400, 3, 4, 1000, null, false);
    defer allocator.free(si2.sample.name);

    const si3 = try makeTestSampledInterface(allocator, "tun0", 500, 600, 5, 6, 1000, null, true);
    defer allocator.free(si3.sample.name);

    var w = TestWriter.init();
    const sampled = [_]SampledInterface{ si1, si2, si3 };
    try renderSampledInterfacesPayload(&w, &sampled, testRuntime);

    const slice = w.slice();

    // Find tunnel_interfaces array boundaries
    const marker = "\"tunnel_interfaces\":[";
    const tunnel_start = std.mem.indexOf(u8, slice, marker) orelse return error.MissingField;
    const after_bracket = slice[tunnel_start + marker.len..];
    const tunnel_end = std.mem.indexOf(u8, after_bracket, "]") orelse return error.MissingField;
    const tunnel_array = after_bracket[0..tunnel_end];

    // Extract tunnel interface names from tunnel_interfaces array
    var tunnel_names: [4][]const u8 = undefined;
    var tunnel_count: usize = 0;
    var pos: usize = 0;
    while (pos < tunnel_array.len) {
        const quote_start = std.mem.indexOf(u8, tunnel_array[pos..], "\"") orelse break;
        pos += quote_start + 1;
        const quote_end = std.mem.indexOf(u8, tunnel_array[pos..], "\"") orelse break;
        tunnel_names[tunnel_count] = tunnel_array[pos..][0..quote_end];
        tunnel_count += 1;
        pos += quote_end + 1;
    }

    // For each tunnel name, verify it appears in private_interfaces with is_tunnel=true
    // within the same JSON object (bounded by enclosing braces).
    for (tunnel_names[0..tunnel_count]) |tun_name| {
        // Find the private_interfaces entry for this name
        const entry_pattern = std.fmt.allocPrint(allocator, "\"name\":\"{s}\"", .{tun_name}) catch unreachable;
        defer allocator.free(entry_pattern);
        const entry_pos = std.mem.indexOf(u8, slice, entry_pattern) orelse return error.MissingTunnelEntry;

        // Bound the search to the enclosing object: find the opening { before entry
        // and its matching } after entry.
        const before_entry = slice[0..entry_pos];
        const open_brace_pos = std.mem.lastIndexOf(u8, before_entry, "{") orelse return error.MissingOpenBrace;
        const after_open = slice[open_brace_pos..];

        // Count braces to find matching close brace at depth 1
        var brace_depth: usize = 0;
        var obj_end: ?usize = null;
        for (after_open, 0..) |c, i| {
            if (c == '{') {
                brace_depth += 1;
            } else if (c == '}') {
                if (brace_depth == 1) {
                    obj_end = i;
                    break;
                }
                if (brace_depth > 0) brace_depth -= 1;
            }
        }
        const obj_slice = after_open[0..(obj_end orelse return error.MissingObjectEnd)];

        // Verify is_tunnel:true exists within this bounded object slice
        _ = std.mem.indexOf(u8, obj_slice, "\"is_tunnel\":true") orelse return error.MissingIsTunnel;
    }
}

// Contract test: zero tunnels - tunnel_count is 0, tunnel_interfaces is empty array.
test "tunnel contract: zero tunnels has empty tunnel_interfaces" {
    const allocator = std.testing.allocator;

    const si = try makeTestSampledInterface(allocator, "eth0", 100, 200, 1, 2, 1000, null, false);
    defer allocator.free(si.sample.name);

    var w = TestWriter.init();
    const sampled = [_]SampledInterface{si};
    try renderSampledInterfacesPayload(&w, &sampled, testRuntime);

    const slice = w.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, slice, 1, "\"tunnel_interfaces\":[]"));
    try std.testing.expect(std.mem.containsAtLeast(u8, slice, 1, "\"tunnel_count\":0"));
}
