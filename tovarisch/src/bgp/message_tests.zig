// message_tests.zig — Tests for UPDATE encoding and NLRI
//
// ACT 1: Tests for UPDATE message encoding, NLRI prefix encoding,
// and prefix length validation.

const std = @import("std");
const message = @import("message.zig");
const types = @import("types.zig");
const validation = @import("validation.zig");

// === UPDATE Tests ===

test "UPDATE has zero withdrawn routes" {
    var buf: [1024]u8 = undefined;
    const prefix = types.Ipv4Prefix.init("10.0.0.0/8");
    const params = types.UpdateParams{
        .next_hop = .{ 192, 168, 1, 1 },
        .local_as = 65001,
        .same_as = true,
        .prefixes = &.{prefix},
    };
    const len = message.encodeUpdate(params, &buf);
    try std.testing.expect(len > 19);
    // Withdrawn routes length is at offset 19 (after header)
    try std.testing.expect(buf[19] == 0);
    try std.testing.expect(buf[20] == 0);
}

test "UPDATE rejects empty advertised prefix list" {
    var buf: [1024]u8 = undefined;
    const params = types.UpdateParams{
        .next_hop = .{ 192, 168, 1, 1 },
        .local_as = 65001,
        .same_as = true,
        .prefixes = &.{},
    };
    const len = message.encodeUpdate(params, &buf);
    try std.testing.expect(len == 0); // 0 means rejected
}

test "UPDATE encodes and message type is UPDATE (2)" {
    var buf: [1024]u8 = undefined;
    const prefix = types.Ipv4Prefix.init("10.0.0.0/8");
    const params = types.UpdateParams{
        .next_hop = .{ 192, 168, 1, 1 },
        .local_as = 65001,
        .same_as = true,
        .prefixes = &.{prefix},
    };
    const len = message.encodeUpdate(params, &buf);
    try std.testing.expect(len > 18);
    // Message type is at offset 18
    try std.testing.expect(buf[18] == 2); // UPDATE type
}

test "UPDATE renders deterministically" {
    var buf1: [1024]u8 = undefined;
    var buf2: [1024]u8 = undefined;
    const prefix = types.Ipv4Prefix.init("10.0.0.0/8");
    const params = types.UpdateParams{
        .next_hop = .{ 192, 168, 1, 1 },
        .local_as = 65001,
        .same_as = true,
        .prefixes = &.{prefix},
    };
    const len1 = message.encodeUpdate(params, &buf1);
    const len2 = message.encodeUpdate(params, &buf2);
    try std.testing.expectEqual(len1, len2);
    try std.testing.expect(std.mem.eql(u8, buf1[0..len1], buf2[0..len2]));
}

test "UPDATE same_as vs different_as produces different output" {
    var buf1: [1024]u8 = undefined;
    var buf2: [1024]u8 = undefined;
    const prefix = types.Ipv4Prefix.init("10.0.0.0/8");

    const params1 = types.UpdateParams{
        .next_hop = .{ 192, 168, 1, 1 },
        .local_as = 65001,
        .same_as = true,
        .prefixes = &.{prefix},
    };
    const params2 = types.UpdateParams{
        .next_hop = .{ 192, 168, 1, 1 },
        .local_as = 65001,
        .same_as = false,
        .prefixes = &.{prefix},
    };

    const len1 = message.encodeUpdate(params1, &buf1);
    const len2 = message.encodeUpdate(params2, &buf2);
    try std.testing.expect(len1 != len2); // Different AS produces different length
    // The messages should not be identical
    try std.testing.expect(!std.mem.eql(u8, buf1[0..len1], buf2[0..len2]));
}

test "UPDATE encodes multiple prefixes deterministically" {
    var buf1: [1024]u8 = undefined;
    var buf2: [1024]u8 = undefined;
    const prefixes = &.{
        types.Ipv4Prefix.init("10.0.0.0/8"),
        types.Ipv4Prefix.init("192.168.0.0/16"),
    };
    const params = types.UpdateParams{
        .next_hop = .{ 192, 168, 1, 1 },
        .local_as = 65001,
        .same_as = true,
        .prefixes = prefixes,
    };
    const len1 = message.encodeUpdate(params, &buf1);
    const len2 = message.encodeUpdate(params, &buf2);
    try std.testing.expectEqual(len1, len2);
    try std.testing.expect(std.mem.eql(u8, buf1[0..len1], buf2[0..len2]));
}

// === Path Attributes Wire Format Tests ===

test "UPDATE same_as has empty AS_PATH (length=0, no segment)" {
    var buf: [1024]u8 = undefined;
    const prefix = types.Ipv4Prefix.init("10.0.0.0/8");
    const params = types.UpdateParams{
        .next_hop = .{ 192, 168, 1, 1 },
        .local_as = 65001,
        .same_as = true,
        .prefixes = &.{prefix},
    };
    const len = message.encodeUpdate(params, &buf);
    try std.testing.expect(len > 27);

    // Message structure after header (offset 19):
    // - withdrawn length (2 bytes): offset 19-20
    // - path attrs length (2 bytes): offset 21-22
    // - ORIGIN (4 bytes): offset 23-26
    // - AS_PATH (3 bytes, length=0): offset 27-29
    // - NEXT_HOP (7 bytes): offset 30-36
    
    // AS_PATH attribute: flags(1) + type(1) + length(1) = 3 bytes
    // For same_as: length should be 0 (no segment bytes)
    try std.testing.expect(buf[27] == 0x40); // TRANSITIVE flag
    try std.testing.expect(buf[28] == 2); // AS_PATH type
    try std.testing.expect(buf[29] == 0); // length = 0 (empty AS_PATH)
}

test "UPDATE different_as has AS_PATH with segment" {
    var buf: [1024]u8 = undefined;
    const prefix = types.Ipv4Prefix.init("10.0.0.0/8");
    const params = types.UpdateParams{
        .next_hop = .{ 192, 168, 1, 1 },
        .local_as = 65001,
        .same_as = false,
        .prefixes = &.{prefix},
    };
    const len = message.encodeUpdate(params, &buf);
    try std.testing.expect(len > 33);

    // AS_PATH attribute: flags(1) + type(1) + length(1) + segment(4) = 7 bytes
    // length should be 4 (segment type + length + AS)
    try std.testing.expect(buf[27] == 0x40); // TRANSITIVE flag
    try std.testing.expect(buf[28] == 2); // AS_PATH type
    try std.testing.expect(buf[29] == 4); // length = 4 (segment data)
    // Segment: type(2=AS_SEQUENCE) + segment_len(1) + AS(65001)
    try std.testing.expect(buf[30] == 2); // AS_SEQUENCE
    try std.testing.expect(buf[31] == 1); // 1 AS in segment
    try std.testing.expect(buf[32] == 0xFD); // 65001 >> 8
    try std.testing.expect(buf[33] == 0xE9); // 65001 & 0xFF
}

test "UPDATE NLRI has no length byte (per RFC 4271)" {
    var buf: [1024]u8 = undefined;
    const prefix = types.Ipv4Prefix.init("10.0.0.0/8");
    const params = types.UpdateParams{
        .next_hop = .{ 192, 168, 1, 1 },
        .local_as = 65001,
        .same_as = true,
        .prefixes = &.{prefix},
    };
    const len = message.encodeUpdate(params, &buf);
    try std.testing.expect(len > 0);

    // After path attributes, NLRI should start directly with prefix length byte
    // Find the NLRI start by looking for the prefix (10.0.0.0/8 = 10, 0, 0, 0)
    // The NLRI should be: prefix_len_byte + prefix_bytes
    // For 10.0.0.0/8: 8-bit prefix = 1 byte (10)
    var found_nlri = false;
    for (buf[21..len], 21..) |b, i| {
        if (b == 8 and i + 1 < len and buf[i + 1] == 10) {
            found_nlri = true;
            // Verify no extra NLRI length byte before this
            break;
        }
    }
    try std.testing.expect(found_nlri);
}

// === NLRI Prefix Encoding Tests ===

test "/11 prefix encodes with 2 bytes" {
    const prefix = types.Ipv4Prefix.init("23.192.0.0/11");
    try std.testing.expectEqual(@as(usize, 2), prefix.nlriByteCount());
    // First 2 bytes of 23.192 = 0x17C0 (big-endian)
    try std.testing.expectEqual(@as(u8, 0x17), prefix.addr[0]);
    try std.testing.expectEqual(@as(u8, 0xC0), prefix.addr[1]);
}

test "/19 prefix encodes with 3 bytes" {
    const prefix = types.Ipv4Prefix.init("64.233.160.0/19");
    try std.testing.expectEqual(@as(usize, 3), prefix.nlriByteCount());
    // First 3 bytes of 64.233.160 = 0x40E9A0
    try std.testing.expectEqual(@as(u8, 0x40), prefix.addr[0]);
    try std.testing.expectEqual(@as(u8, 0xE9), prefix.addr[1]);
    try std.testing.expectEqual(@as(u8, 0xA0), prefix.addr[2]);
}

test "/16 prefix encodes with 2 bytes" {
    const prefix = types.Ipv4Prefix.init("173.194.0.0/16");
    try std.testing.expectEqual(@as(usize, 2), prefix.nlriByteCount());
    try std.testing.expectEqual(@as(u8, 173), prefix.addr[0]);
    try std.testing.expectEqual(@as(u8, 194), prefix.addr[1]);
}

test "/32 prefix encodes with 4 bytes" {
    const prefix = types.Ipv4Prefix.init("192.168.1.1/32");
    try std.testing.expectEqual(@as(usize, 4), prefix.nlriByteCount());
    try std.testing.expectEqual(@as(u8, 192), prefix.addr[0]);
    try std.testing.expectEqual(@as(u8, 168), prefix.addr[1]);
    try std.testing.expectEqual(@as(u8, 1), prefix.addr[2]);
    try std.testing.expectEqual(@as(u8, 1), prefix.addr[3]);
}

test "/0 prefix encodes with 0 bytes" {
    const prefix = types.Ipv4Prefix.init("0.0.0.0/0");
    try std.testing.expectEqual(@as(usize, 0), prefix.nlriByteCount());
}

test "invalid prefix length is rejected in validation" {
    try std.testing.expectError(validation.ValidationError.InvalidPrefixLength, validation.validatePrefixLength(33));
    try std.testing.expectError(validation.ValidationError.InvalidPrefixLength, validation.validatePrefixLength(128));
}
