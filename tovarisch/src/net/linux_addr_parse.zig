// linux_addr_parse.zig — rtnetlink message parsing helpers
//
// Helper functions for parsing NETLINK_ROUTE messages received from
// the Linux kernel. These are extracted from linux_addr.zig to keep
// that file under the LLM-friendly size limit.
//
// Provides:
// - align4(): 4-byte alignment for netlink message iteration
// - formatIpv4(): IPv4 octets to string conversion
// - parseLabel(): null-terminated string extraction from netlink buffer

const std = @import("std");

/// Align a length to 4 bytes for netlink message alignment.
/// Netlink attributes and messages must be aligned to 4-byte boundaries.
pub fn align4(len: usize) usize {
    return (len + 3) & ~@as(usize, 3);
}

/// Format an IPv4 address into a caller-provided buffer.
/// Returns a slice into the buffer on success.
pub fn formatIpv4(octets: [4]u8, buf: []u8) ![]const u8 {
    return std.fmt.bufPrint(buf, "{}.{}.{}.{}", .{
        octets[0],
        octets[1],
        octets[2],
        octets[3],
    });
}

/// Parse a null-terminated string from a buffer at a given offset.
/// Returns the string if found and non-empty.
///
/// Used to extract interface labels (e.g., "eth0", "wg0") from
/// IFA_LABEL netlink attributes.
pub fn parseLabel(buffer: []const u8, start_offset: usize, end_offset: usize) ?[]const u8 {
    // Check bounds before slicing
    if (start_offset >= buffer.len or end_offset > buffer.len) return null;
    if (start_offset >= end_offset) return null;
    const data_len = end_offset - start_offset;
    if (data_len < 1) return null;

    // Find null terminator or use full data
    const data = buffer[start_offset..end_offset];
    const null_pos = std.mem.indexOfScalar(u8, data, 0);
    const label_len = if (null_pos) |pos| pos else data_len;

    if (label_len == 0) return null;
    return data[0..label_len];
}

// ============================================================================
// Tests
// ============================================================================

const testing = std.testing;

test "align4: zero input" {
    try testing.expectEqual(@as(usize, 0), align4(0));
}

test "align4: boundary values" {
    try testing.expectEqual(@as(usize, 4), align4(1));
    try testing.expectEqual(@as(usize, 4), align4(2));
    try testing.expectEqual(@as(usize, 4), align4(3));
    try testing.expectEqual(@as(usize, 4), align4(4));
}

test "align4: beyond boundary" {
    try testing.expectEqual(@as(usize, 8), align4(5));
    try testing.expectEqual(@as(usize, 8), align4(6));
    try testing.expectEqual(@as(usize, 8), align4(7));
    try testing.expectEqual(@as(usize, 8), align4(8));
}

test "align4: larger values" {
    try testing.expectEqual(@as(usize, 12), align4(9));
    try testing.expectEqual(@as(usize, 12), align4(12));
    try testing.expectEqual(@as(usize, 16), align4(13));
}

test "formatIpv4: standard address" {
    const octets: [4]u8 = .{ 192, 168, 1, 10 };
    var buf: [15]u8 = undefined;
    const result = try formatIpv4(octets, &buf);
    try testing.expectEqualSlices(u8, "192.168.1.10", result);
}

test "formatIpv4: loopback" {
    const octets: [4]u8 = .{ 127, 0, 0, 1 };
    var buf: [15]u8 = undefined;
    const result = try formatIpv4(octets, &buf);
    try testing.expectEqualSlices(u8, "127.0.0.1", result);
}

test "formatIpv4: all zeros" {
    const octets: [4]u8 = .{ 0, 0, 0, 0 };
    var buf: [15]u8 = undefined;
    const result = try formatIpv4(octets, &buf);
    try testing.expectEqualSlices(u8, "0.0.0.0", result);
}

test "formatIpv4: max values" {
    const octets: [4]u8 = .{ 255, 255, 255, 255 };
    var buf: [15]u8 = undefined;
    const result = try formatIpv4(octets, &buf);
    try testing.expectEqualSlices(u8, "255.255.255.255", result);
}

test "formatIpv4: handles all private ranges" {
    var buf: [15]u8 = undefined;

    // 10.0.0.1
    const oct1: [4]u8 = .{ 10, 0, 0, 1 };
    try testing.expectEqualSlices(u8, "10.0.0.1", try formatIpv4(oct1, &buf));

    // 172.16.0.1
    const oct2: [4]u8 = .{ 172, 16, 0, 1 };
    try testing.expectEqualSlices(u8, "172.16.0.1", try formatIpv4(oct2, &buf));

    // 192.168.1.1
    const oct3: [4]u8 = .{ 192, 168, 1, 1 };
    try testing.expectEqualSlices(u8, "192.168.1.1", try formatIpv4(oct3, &buf));
}

test "parseLabel: null-terminated string" {
    const buffer = [_]u8{ 'e', 't', 'h', '0', 0, 'f', 'o', 'o' };
    const result = parseLabel(&buffer, 0, buffer.len);
    try testing.expect(result != null);
    try testing.expectEqualSlices(u8, "eth0", result.?);
}

test "parseLabel: no null terminator" {
    const buffer = [_]u8{ 'w', 'g', '0' };
    const result = parseLabel(&buffer, 0, buffer.len);
    try testing.expect(result != null);
    try testing.expectEqualSlices(u8, "wg0", result.?);
}

test "parseLabel: empty buffer" {
    const buffer: [0]u8 = .{};
    const result = parseLabel(&buffer, 0, 0);
    try testing.expect(result == null);
}

test "parseLabel: only null" {
    const buffer = [_]u8{0};
    const result = parseLabel(&buffer, 0, 1);
    try testing.expect(result == null);
}

test "parseLabel: offset start" {
    const buffer = [_]u8{ 0, 'l', 'o', 0 };
    const result = parseLabel(&buffer, 1, 3);
    try testing.expect(result != null);
    try testing.expectEqualSlices(u8, "lo", result.?);
}

test "parseLabel: offset beyond data" {
    const buffer = [_]u8{ 'a', 'b' };
    const result = parseLabel(&buffer, 5, 10);
    try testing.expect(result == null);
}
