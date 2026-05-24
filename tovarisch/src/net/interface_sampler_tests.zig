// interface_sampler_tests.zig — Unit tests for interface_sampler.zig

const std = @import("std");
const rates = @import("rates.zig");
const sampler = @import("interface_sampler.zig");

test "first update returns null rates" {
    const allocator = std.testing.allocator;
    var s = sampler.InterfaceSampler.init(allocator);
    defer s.deinit();

    const current = &[_]rates.InterfaceCounterSample{
        .{
            .name = "wg0",
            .rx_bytes = 1000,
            .tx_bytes = 2000,
            .rx_packets = 10,
            .tx_packets = 20,
            .sampled_at_ms = 1000,
        },
    };

    const result = try s.update(current);
    defer {
        for (result) |si| allocator.free(si.sample.name);
        allocator.free(result);
    }

    try std.testing.expectEqual(@as(usize, 1), result.len);
    try std.testing.expectEqualStrings("wg0", result[0].sample.name);
    try std.testing.expect(result[0].rate == null);
}

test "second update computes rates" {
    const allocator = std.testing.allocator;
    var s = sampler.InterfaceSampler.init(allocator);
    defer s.deinit();

    const first = &[_]rates.InterfaceCounterSample{
        .{ .name = "wg0", .rx_bytes = 1000, .tx_bytes = 2000, .rx_packets = 10, .tx_packets = 20, .sampled_at_ms = 0 },
    };
    {
        const result = try s.update(first);
        defer { for (result) |si| allocator.free(si.sample.name); allocator.free(result); }
        try std.testing.expect(result[0].rate == null);
    }

    const second = &[_]rates.InterfaceCounterSample{
        .{ .name = "wg0", .rx_bytes = 31000, .tx_bytes = 62000, .rx_packets = 310, .tx_packets = 620, .sampled_at_ms = 30000 },
    };
    {
        const result = try s.update(second);
        defer { for (result) |si| allocator.free(si.sample.name); allocator.free(result); }
        try std.testing.expect(result[0].rate != null);
        const r = result[0].rate.?;
        try std.testing.expectEqual(@as(u64, 30), r.window_seconds);
        try std.testing.expectEqual(@as(u64, 30000), r.rx_bytes_delta);
        try std.testing.expectEqual(@as(u64, 1000), r.rx_bytes_per_second);
    }
}

test "packet deltas are calculated from stored packet counters" {
    const allocator = std.testing.allocator;
    var s = sampler.InterfaceSampler.init(allocator);
    defer s.deinit();

    // First update
    const first = &[_]rates.InterfaceCounterSample{
        .{ .name = "wg0", .rx_bytes = 1000, .tx_bytes = 2000, .rx_packets = 100, .tx_packets = 200, .sampled_at_ms = 0 },
    };
    {
        const result = try s.update(first);
        defer { for (result) |si| allocator.free(si.sample.name); allocator.free(result); }
        try std.testing.expect(result[0].rate == null);
    }

    // Second update after 10s
    const second = &[_]rates.InterfaceCounterSample{
        .{ .name = "wg0", .rx_bytes = 16000, .tx_bytes = 24000, .rx_packets = 160, .tx_packets = 260, .sampled_at_ms = 10000 },
    };
    {
        const result = try s.update(second);
        defer { for (result) |si| allocator.free(si.sample.name); allocator.free(result); }
        try std.testing.expect(result[0].rate != null);
        const r = result[0].rate.?;
        // Expected: +15000 bytes, +60 packets over 10s
        try std.testing.expectEqual(@as(u64, 10), r.window_seconds);
        try std.testing.expectEqual(@as(u64, 15000), r.rx_bytes_delta);
        try std.testing.expectEqual(@as(u64, 60), r.rx_packets_delta);
        try std.testing.expectEqual(@as(u64, 1500), r.rx_bytes_per_second);
        try std.testing.expectEqual(@as(u64, 6), r.rx_packets_per_second);
    }
}

test "packet counter reset is detected and rate is null" {
    const allocator = std.testing.allocator;
    var s = sampler.InterfaceSampler.init(allocator);
    defer s.deinit();

    // First update
    const first = &[_]rates.InterfaceCounterSample{
        .{ .name = "wg0", .rx_bytes = 100000, .tx_bytes = 100000, .rx_packets = 1000, .tx_packets = 1000, .sampled_at_ms = 0 },
    };
    {
        const result = try s.update(first);
        defer { for (result) |si| allocator.free(si.sample.name); allocator.free(result); }
    }

    // Second update: packets reset, bytes increase normally
    const second = &[_]rates.InterfaceCounterSample{
        .{ .name = "wg0", .rx_bytes = 200000, .tx_bytes = 200000, .rx_packets = 5, .tx_packets = 2000, .sampled_at_ms = 30000 },
    };
    {
        const result = try s.update(second);
        defer { for (result) |si| allocator.free(si.sample.name); allocator.free(result); }
        // rx_packets went from 1000 to 5, so rate should be null
        try std.testing.expect(result[0].rate == null);
    }
}

test "multiple interfaces compute independently" {
    const allocator = std.testing.allocator;
    var s = sampler.InterfaceSampler.init(allocator);
    defer s.deinit();

    const first = &[_]rates.InterfaceCounterSample{
        .{ .name = "wg0", .rx_bytes = 1000, .tx_bytes = 2000, .rx_packets = 10, .tx_packets = 20, .sampled_at_ms = 0 },
        .{ .name = "eth0", .rx_bytes = 5000, .tx_bytes = 10000, .rx_packets = 50, .tx_packets = 100, .sampled_at_ms = 0 },
    };
    {
        const result = try s.update(first);
        defer { for (result) |si| allocator.free(si.sample.name); allocator.free(result); }
        try std.testing.expectEqual(@as(usize, 2), result.len);
        try std.testing.expect(result[0].rate == null);
        try std.testing.expect(result[1].rate == null);
    }

    const second = &[_]rates.InterfaceCounterSample{
        .{ .name = "eth0", .rx_bytes = 65000, .tx_bytes = 130000, .rx_packets = 650, .tx_packets = 1300, .sampled_at_ms = 30000 },
        .{ .name = "wg0", .rx_bytes = 31000, .tx_bytes = 62000, .rx_packets = 310, .tx_packets = 620, .sampled_at_ms = 30000 },
    };
    {
        const result = try s.update(second);
        defer { for (result) |si| allocator.free(si.sample.name); allocator.free(result); }
        try std.testing.expectEqual(@as(usize, 2), result.len);
        try std.testing.expect(result[0].rate != null);
        try std.testing.expect(result[1].rate != null);
        try std.testing.expectEqual(@as(u64, 2000), result[0].rate.?.rx_bytes_per_second);
        try std.testing.expectEqual(@as(u64, 1000), result[1].rate.?.rx_bytes_per_second);
    }
}

test "new interface on second update has null rate" {
    const allocator = std.testing.allocator;
    var s = sampler.InterfaceSampler.init(allocator);
    defer s.deinit();

    const first = &[_]rates.InterfaceCounterSample{
        .{ .name = "wg0", .rx_bytes = 1000, .tx_bytes = 2000, .rx_packets = 10, .tx_packets = 20, .sampled_at_ms = 0 },
    };
    {
        const result = try s.update(first);
        defer { for (result) |si| allocator.free(si.sample.name); allocator.free(result); }
        try std.testing.expectEqual(@as(usize, 1), result.len);
    }

    const second = &[_]rates.InterfaceCounterSample{
        .{ .name = "wg0", .rx_bytes = 31000, .tx_bytes = 62000, .rx_packets = 310, .tx_packets = 620, .sampled_at_ms = 30000 },
        .{ .name = "eth0", .rx_bytes = 5000, .tx_bytes = 10000, .rx_packets = 50, .tx_packets = 100, .sampled_at_ms = 30000 },
    };
    {
        const result = try s.update(second);
        defer { for (result) |si| allocator.free(si.sample.name); allocator.free(result); }
        try std.testing.expectEqual(@as(usize, 2), result.len);
        try std.testing.expect(result[0].rate != null);
        try std.testing.expect(result[1].rate == null);
    }
}

test "disappeared interface is removed" {
    const allocator = std.testing.allocator;
    var s = sampler.InterfaceSampler.init(allocator);
    defer s.deinit();

    const first = &[_]rates.InterfaceCounterSample{
        .{ .name = "wg0", .rx_bytes = 1000, .tx_bytes = 2000, .rx_packets = 10, .tx_packets = 20, .sampled_at_ms = 0 },
        .{ .name = "eth0", .rx_bytes = 5000, .tx_bytes = 10000, .rx_packets = 50, .tx_packets = 100, .sampled_at_ms = 0 },
    };
    {
        const result = try s.update(first);
        defer { for (result) |si| allocator.free(si.sample.name); allocator.free(result); }
        try std.testing.expectEqual(@as(usize, 2), result.len);
    }

    const second = &[_]rates.InterfaceCounterSample{
        .{ .name = "wg0", .rx_bytes = 31000, .tx_bytes = 62000, .rx_packets = 310, .tx_packets = 620, .sampled_at_ms = 30000 },
    };
    {
        const result = try s.update(second);
        defer { for (result) |si| allocator.free(si.sample.name); allocator.free(result); }
        try std.testing.expectEqual(@as(usize, 1), result.len);
        try std.testing.expectEqualStrings("wg0", result[0].sample.name);
    }

    const third = &[_]rates.InterfaceCounterSample{
        .{ .name = "wg0", .rx_bytes = 61000, .tx_bytes = 102000, .rx_packets = 610, .tx_packets = 1020, .sampled_at_ms = 60000 },
        .{ .name = "eth0", .rx_bytes = 5000, .tx_bytes = 10000, .rx_packets = 50, .tx_packets = 100, .sampled_at_ms = 60000 },
    };
    {
        const result = try s.update(third);
        defer { for (result) |si| allocator.free(si.sample.name); allocator.free(result); }
        try std.testing.expectEqual(@as(usize, 2), result.len);
        try std.testing.expect(result[0].rate != null);
        try std.testing.expect(result[1].rate == null);
    }
}

test "counter reset affects only that interface" {
    const allocator = std.testing.allocator;
    var s = sampler.InterfaceSampler.init(allocator);
    defer s.deinit();

    const first = &[_]rates.InterfaceCounterSample{
        .{ .name = "wg0", .rx_bytes = 100000, .tx_bytes = 100000, .rx_packets = 1000, .tx_packets = 1000, .sampled_at_ms = 0 },
        .{ .name = "eth0", .rx_bytes = 50000, .tx_bytes = 50000, .rx_packets = 500, .tx_packets = 500, .sampled_at_ms = 0 },
    };
    {
        const result = try s.update(first);
        defer { for (result) |si| allocator.free(si.sample.name); allocator.free(result); }
    }

    const second = &[_]rates.InterfaceCounterSample{
        .{ .name = "wg0", .rx_bytes = 500, .tx_bytes = 200000, .rx_packets = 5, .tx_packets = 2000, .sampled_at_ms = 30000 },
        .{ .name = "eth0", .rx_bytes = 95000, .tx_bytes = 95000, .rx_packets = 950, .tx_packets = 950, .sampled_at_ms = 30000 },
    };
    {
        const result = try s.update(second);
        defer { for (result) |si| allocator.free(si.sample.name); allocator.free(result); }
        try std.testing.expectEqual(@as(usize, 2), result.len);
        try std.testing.expect(result[0].rate == null);
        try std.testing.expect(result[1].rate != null);
        try std.testing.expectEqual(@as(u64, 45000), result[1].rate.?.rx_bytes_delta);
    }
}

test "output order follows current input order" {
    const allocator = std.testing.allocator;
    var s = sampler.InterfaceSampler.init(allocator);
    defer s.deinit();

    const first = &[_]rates.InterfaceCounterSample{
        .{ .name = "wg0", .rx_bytes = 1000, .tx_bytes = 2000, .rx_packets = 10, .tx_packets = 20, .sampled_at_ms = 0 },
        .{ .name = "eth0", .rx_bytes = 5000, .tx_bytes = 10000, .rx_packets = 50, .tx_packets = 100, .sampled_at_ms = 0 },
        .{ .name = "tun0", .rx_bytes = 100, .tx_bytes = 200, .rx_packets = 1, .tx_packets = 2, .sampled_at_ms = 0 },
    };
    {
        const result = try s.update(first);
        defer { for (result) |si| allocator.free(si.sample.name); allocator.free(result); }
        try std.testing.expectEqual(@as(usize, 3), result.len);
        try std.testing.expectEqualStrings("wg0", result[0].sample.name);
        try std.testing.expectEqualStrings("eth0", result[1].sample.name);
        try std.testing.expectEqualStrings("tun0", result[2].sample.name);
    }

    const second = &[_]rates.InterfaceCounterSample{
        .{ .name = "tun0", .rx_bytes = 100100, .tx_bytes = 200200, .rx_packets = 1001, .tx_packets = 2002, .sampled_at_ms = 30000 },
        .{ .name = "wg0", .rx_bytes = 31000, .tx_bytes = 62000, .rx_packets = 310, .tx_packets = 620, .sampled_at_ms = 30000 },
        .{ .name = "eth0", .rx_bytes = 65000, .tx_bytes = 130000, .rx_packets = 650, .tx_packets = 1300, .sampled_at_ms = 30000 },
    };
    {
        const result = try s.update(second);
        defer { for (result) |si| allocator.free(si.sample.name); allocator.free(result); }
        try std.testing.expectEqual(@as(usize, 3), result.len);
        try std.testing.expectEqualStrings("tun0", result[0].sample.name);
        try std.testing.expectEqualStrings("wg0", result[1].sample.name);
        try std.testing.expectEqualStrings("eth0", result[2].sample.name);
    }
}

test "sampler duplicates names safely - result survives caller buffer mutation" {
    const allocator = std.testing.allocator;
    var s = sampler.InterfaceSampler.init(allocator);
    defer s.deinit();

    // First update with stack/mutable name buffer
    var name_buffer: [32]u8 = undefined;
    @memcpy(name_buffer[0..3], "wg0");
    const nameSlice = name_buffer[0..3];

    const first = &[_]rates.InterfaceCounterSample{
        .{ .name = nameSlice, .rx_bytes = 1000, .tx_bytes = 2000, .rx_packets = 10, .tx_packets = 20, .sampled_at_ms = 0 },
    };

    const result1 = try s.update(first);
    defer { for (result1) |si| allocator.free(si.sample.name); allocator.free(result1); }

    try std.testing.expectEqualStrings("wg0", result1[0].sample.name);
    @memcpy(name_buffer[0..3], "XXX"); // Mutate caller buffer
    try std.testing.expectEqualStrings("wg0", result1[0].sample.name); // Result survived

    // Second update with fresh literal - must still match and compute valid rate
    const second = &[_]rates.InterfaceCounterSample{
        .{ .name = "wg0", .rx_bytes = 31000, .tx_bytes = 62000, .rx_packets = 310, .tx_packets = 620, .sampled_at_ms = 30000 },
    };
    const result2 = try s.update(second);
    defer { for (result2) |si| allocator.free(si.sample.name); allocator.free(result2); }

    // This proves the map key is independent from caller's buffer
    try std.testing.expect(result2[0].rate != null);
    const r = result2[0].rate.?;
    try std.testing.expectEqual(@as(u64, 1000), r.rx_bytes_per_second);
}

test "deinit does not leak under Zig test allocator" {
    const allocator = std.testing.allocator;
    var s = sampler.InterfaceSampler.init(allocator);
    defer s.deinit();

    const first = &[_]rates.InterfaceCounterSample{
        .{ .name = "wg0", .rx_bytes = 1000, .tx_bytes = 2000, .rx_packets = 10, .tx_packets = 20, .sampled_at_ms = 0 },
        .{ .name = "eth0", .rx_bytes = 5000, .tx_bytes = 10000, .rx_packets = 50, .tx_packets = 100, .sampled_at_ms = 0 },
    };
    {
        const result = try s.update(first);
        defer { for (result) |si| allocator.free(si.sample.name); allocator.free(result); }
    }

    const second = &[_]rates.InterfaceCounterSample{
        .{ .name = "wg0", .rx_bytes = 31000, .tx_bytes = 62000, .rx_packets = 310, .tx_packets = 620, .sampled_at_ms = 30000 },
        .{ .name = "eth0", .rx_bytes = 65000, .tx_bytes = 130000, .rx_packets = 650, .tx_packets = 1300, .sampled_at_ms = 30000 },
    };
    {
        const result = try s.update(second);
        defer { for (result) |si| allocator.free(si.sample.name); allocator.free(result); }
    }
}
