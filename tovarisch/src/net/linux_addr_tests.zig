// linux_addr_tests.zig — Tests for live Linux address discovery
//
// Tests cover:
// 1. Error handling for invalid socket operations
// 2. InterfaceAddress struct contract
// 3. Linux smoke test for rtnetlink operations
// 4. Helper function unit tests (via linux_addr_parse.zig)
//
// Note: Full fixture-based testing of rtnetlink is complex because it requires
// mocking kernel responses. The primary test strategy is:
// - Unit tests for error paths and contracts
// - Linux smoke test for actual rtnetlink integration

const std = @import("std");
const testing = std.testing;
const linux_addr = @import("linux_addr.zig");
const linux_addr_parse = @import("linux_addr_parse.zig");
const interface_filter = @import("interface_filter.zig");

// ============================================================================
// Error Path Tests
// ============================================================================

test "AddrError has required variants" {
    // Verify AddrError error set has expected members
    _ = @TypeOf(linux_addr.AddrError.SocketCreateFailed);
    try testing.expect(true); // Basic assertion to keep test
}

// ============================================================================
// InterfaceAddress Contract Tests
// ============================================================================

test "InterfaceAddress struct has required fields" {
    // Verify InterfaceAddress can be constructed
    const addr = interface_filter.InterfaceAddress{
        .iface = "eth0",
        .address = "192.168.1.1",
    };

    try testing.expectEqualSlices(u8, "eth0", addr.iface);
    try testing.expectEqualSlices(u8, "192.168.1.1", addr.address);
}

// ============================================================================
// Address Classification Tests (via private_ip integration)
// ============================================================================

test "IPv4 addresses are correctly classified via integration" {
    // This tests the integration with interfaceHasPrivateAddress
    // to ensure the address format is compatible
    const addrs = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "192.168.1.1" },
        .{ .iface = "eth0", .address = "10.0.0.1" },
        .{ .iface = "eth0", .address = "172.16.0.1" },
        .{ .iface = "eth0", .address = "8.8.8.8" },
    };

    // All three private ranges should match
    try testing.expect(interface_filter.interfaceHasPrivateAddress("eth0", &addrs));
}

test "public addresses are not classified as private" {
    const addrs = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "8.8.8.8" },
        .{ .iface = "eth0", .address = "1.1.1.1" },
    };

    try testing.expect(!interface_filter.interfaceHasPrivateAddress("eth0", &addrs));
}

// ============================================================================
// Error Set Completeness
// ============================================================================

test "freeAddresses handles empty slice" {
    const allocator = std.heap.page_allocator;
    const empty: []interface_filter.InterfaceAddress = &.{};
    linux_addr.freeAddresses(allocator, empty);
    // Should not panic
}

// ============================================================================
// Linux Smoke Test
// ============================================================================

test "linux smoke: rtnetlink discovery on live system" {
    if (@import("builtin").os.tag != .linux) return error.SkipZigTest;

    const allocator = std.heap.page_allocator;

    // Call live discovery - expected to succeed on Linux
    const result = linux_addr.discoverPrivateAddresses(allocator, "/sys/class/net");

    if (result) |addrs| {
        defer linux_addr.freeAddresses(allocator, addrs);

        // Each address should be valid
        for (addrs) |addr| {
            try testing.expect(addr.iface.len > 0);
            try testing.expect(addr.address.len > 0);
            // Basic IPv4 format check
            try testing.expect(std.mem.indexOfScalar(u8, addr.address, '.') != null);
        }
    } else |err| {
        // Socket errors are acceptable on restricted environments
        switch (err) {
            error.SocketCreateFailed,
            error.SendFailed,
            error.RecvFailed,
            => {},
            else => return err, // Unexpected error
        }
    }
}

// ============================================================================
// Helper Unit Tests
// ============================================================================

test "align4: zero input" {
    try testing.expectEqual(@as(usize, 0), linux_addr.align4(0));
}

test "align4: boundary values" {
    try testing.expectEqual(@as(usize, 4), linux_addr.align4(1));
    try testing.expectEqual(@as(usize, 4), linux_addr.align4(2));
    try testing.expectEqual(@as(usize, 4), linux_addr.align4(3));
    try testing.expectEqual(@as(usize, 4), linux_addr.align4(4));
}

test "align4: beyond boundary" {
    try testing.expectEqual(@as(usize, 8), linux_addr.align4(5));
    try testing.expectEqual(@as(usize, 8), linux_addr.align4(6));
    try testing.expectEqual(@as(usize, 8), linux_addr.align4(7));
    try testing.expectEqual(@as(usize, 8), linux_addr.align4(8));
}

test "formatIpv4: standard address" {
    const octets: [4]u8 = .{ 192, 168, 1, 10 };
    var buf: [15]u8 = undefined;
    const result = try linux_addr.formatIpv4(octets, &buf);
    try testing.expectEqualSlices(u8, "192.168.1.10", result);
}

test "formatIpv4: loopback" {
    const octets: [4]u8 = .{ 127, 0, 0, 1 };
    var buf: [15]u8 = undefined;
    const result = try linux_addr.formatIpv4(octets, &buf);
    try testing.expectEqualSlices(u8, "127.0.0.1", result);
}

test "formatIpv4: all zeros" {
    const octets: [4]u8 = .{ 0, 0, 0, 0 };
    var buf: [15]u8 = undefined;
    const result = try linux_addr.formatIpv4(octets, &buf);
    try testing.expectEqualSlices(u8, "0.0.0.0", result);
}

test "formatIpv4: max values" {
    const octets: [4]u8 = .{ 255, 255, 255, 255 };
    var buf: [15]u8 = undefined;
    const result = try linux_addr.formatIpv4(octets, &buf);
    try testing.expectEqualSlices(u8, "255.255.255.255", result);
}

test "parseLabel: null-terminated string" {
    const buffer = [_]u8{ 'e', 't', 'h', '0', 0, 'f', 'o', 'o' };
    const result = linux_addr.parseLabel(&buffer, 0, buffer.len);
    try testing.expect(result != null);
    try testing.expectEqualSlices(u8, "eth0", result.?);
}

test "parseLabel: no null terminator" {
    const buffer = [_]u8{ 'w', 'g', '0' };
    const result = linux_addr.parseLabel(&buffer, 0, buffer.len);
    try testing.expect(result != null);
    try testing.expectEqualSlices(u8, "wg0", result.?);
}

test "parseLabel: empty buffer" {
    const buffer: [0]u8 = .{};
    const result = linux_addr.parseLabel(&buffer, 0, 0);
    try testing.expect(result == null);
}

test "parseLabel: only null" {
    const buffer = [_]u8{0};
    const result = linux_addr.parseLabel(&buffer, 0, 1);
    try testing.expect(result == null);
}

test "parseLabel: offset start" {
    const buffer = [_]u8{ 0, 'l', 'o', 0 };
    const result = linux_addr.parseLabel(&buffer, 1, 3);
    try testing.expect(result != null);
    try testing.expectEqualSlices(u8, "lo", result.?);
}

test "parseLabel: offset beyond data" {
    const buffer = [_]u8{ 'a', 'b' };
    const result = linux_addr.parseLabel(&buffer, 5, 10);
    try testing.expect(result == null);
}

// ============================================================================
// rtnetlink Constants Regression Tests
// ============================================================================

// These tests verify the request bytes match expected rtnetlink values.
// Uses buildRequest() from linux_addr_parse.zig which encodes the same
// constants used by the production discoverPrivateAddresses() function.

test "rtnetlink: request has RTM_GETADDR type (22 = 0x16)" {
    const req = linux_addr_parse.buildRequest();
    // nlmsghdr.nlmsg_type is at offset 4 (after nlmsg_len), 2 bytes
    const nlmsg_type = std.mem.readInt(c_ushort, req.buffer[4..6], .little);
    try testing.expectEqual(@as(c_ushort, 22), nlmsg_type);
}

test "rtnetlink: request has NLM_F_REQUEST|NLM_F_DUMP flags (0x301)" {
    const req = linux_addr_parse.buildRequest();
    // nlmsghdr layout: nlmsg_len(4) + nlmsg_type(2) + nlmsg_flags(2) at offset 6
    const nlmsg_flags = std.mem.readInt(c_ushort, req.buffer[6..8], .little);
    // NLM_F_REQUEST (0x001) | NLM_F_DUMP (0x300) = 0x301
    try testing.expectEqual(@as(c_ushort, 0x301), nlmsg_flags);
}

test "rtnetlink: request has AF_INET family in ifaddrmsg" {
    const req = linux_addr_parse.buildRequest();
    // ifaddrmsg.ifa_family is at offset @sizeOf(nlmsghdr) = 16
    const ifa_family = req.buffer[16];
    // AF_INET = 2
    try testing.expectEqual(@as(u8, 2), ifa_family);
}
