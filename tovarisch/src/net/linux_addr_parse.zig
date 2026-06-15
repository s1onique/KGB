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

// ============================================================================
// Test Helpers (for unit testing rtnetlink request construction)
// ============================================================================

// Note: These helpers use the same constant values as linux_addr.zig.
// They exist here so tests can verify request bytes without exposing
// the private constants directly.

/// Builds a netlink request buffer for RTM_GETADDR.
/// Returns bytes for testing the request structure.
pub fn buildRequest() struct { buffer: [64]u8, len: usize } {
    const nlmsg_len = 20; // @sizeOf(nlmsghdr) + @sizeOf(ifaddrmsg) = 16 + 4
    const RTM_GETADDR: c_uint = 22;
    const NLM_F_REQUEST: c_uint = 0x001;
    const NLM_F_ROOT: c_uint = 0x100;
    const NLM_F_MATCH: c_uint = 0x200;
    const AF_INET: c_int = 2;

    var result: [64]u8 = undefined;
    var req_buf: [nlmsg_len]u8 = undefined;

    // nlmsghdr: nlmsg_len(4) + nlmsg_type(2) + nlmsg_flags(2) + nlmsg_seq(4) + nlmsg_pid(4)
    std.mem.writeInt(c_uint, req_buf[0..4], @intCast(nlmsg_len), .little);
    std.mem.writeInt(c_ushort, req_buf[4..6], @intCast(RTM_GETADDR), .little);
    std.mem.writeInt(c_ushort, req_buf[6..8], @intCast(NLM_F_REQUEST | NLM_F_ROOT | NLM_F_MATCH), .little);
    std.mem.writeInt(c_uint, req_buf[8..12], 1, .little);
    std.mem.writeInt(c_uint, req_buf[12..16], 0, .little);

    // ifaddrmsg: ifa_family(1) + ifa_prefixlen(1) + ifa_flags(1) + ifa_scope(1) + ifa_index(4)
    std.mem.writeInt(c_uint, req_buf[16..20], 0, .little); // ifa_index = 0
    req_buf[16] = @intCast(AF_INET); // IPv4 only (overwrites first byte of ifa_index)
    req_buf[17] = 0; // ifa_prefixlen
    req_buf[18] = 0; // ifa_flags
    req_buf[19] = 0; // ifa_scope

    // MemoryCopySafety: result is a local [64]u8 buffer. req_buf is a local
    // [nlmsg_len]u8. Both are on the stack; distinct regions; no aliasing.
    @memcpy(result[0..nlmsg_len], &req_buf);
    return .{ .buffer = result, .len = nlmsg_len };
}
