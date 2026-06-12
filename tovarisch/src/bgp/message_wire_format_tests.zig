// message_wire_format_tests.zig — BGP UPDATE wire format regression tests
//
// Regression: BIRD was interpreting prefixes as withdrawn routes.
// Evidence: BIRD show output shows "Import withdraws: 15810 ignored"
// while tovarisch status shows "BGP established; 15810 configured prefixes"
//
// Root cause: If prefixes were in withdrawn routes section (withdrawn_routes_length > 0),
// BIRD reads them as route withdrawals, not announcements.
//
// Fix verification: These tests prove prefixes are in NLRI section, not withdrawn routes.

const std = @import("std");
const message = @import("message.zig");
const types = @import("types.zig");

// === UPDATE Wire Format Regression Tests ===

test "UPDATE wire format: prefixes in NLRI not withdrawn routes" {
    // Regression test: ensure prefixes are NOT in withdrawn routes section.
    // Wire format per RFC 4271 Section 4.3:
    //   withdrawn_routes_length (2) | withdrawn routes | path attributes length (2) | path attributes | NLRI
    //
    // If prefixes were incorrectly placed in withdrawn section, BIRD would interpret
    // them as withdrawals ("Import withdraws: 15810 ignored" in BIRD show).
    var buf: [1024]u8 = undefined;
    const prefix = types.Ipv4Prefix.init("10.0.0.0/8");
    const params = types.UpdateParams{
        .next_hop = .{ 192, 168, 1, 1 },
        .local_as = 65001,
        .same_as = true,
        .prefixes = &.{prefix},
    };
    const len = message.encodeUpdate(params, &buf);

    try std.testing.expect(len > 23);

    // Verify marker is all 0xFF
    var marker_ok = true;
    for (0..16) |i| {
        if (buf[i] != 0xFF) marker_ok = false;
    }
    try std.testing.expect(marker_ok);

    // Verify message type is UPDATE (2)
    try std.testing.expect(buf[18] == 2);

    // CRITICAL: withdrawn_routes_length MUST be 0 for route announcement
    // If this is non-zero, BIRD interprets subsequent bytes as withdrawn prefixes
    try std.testing.expect(buf[19] == 0);
    try std.testing.expect(buf[20] == 0);

    // Path attributes length must be > 0 (at least ORIGIN + AS_PATH + NEXT_HOP)
    const attrs_len = @as(usize, buf[21]) * 256 + @as(usize, buf[22]);
    try std.testing.expect(attrs_len > 0);

    // Verify ORIGIN attribute (type 1) is present at offset 23
    try std.testing.expect(buf[23] == 0x40); // TRANSITIVE flag
    try std.testing.expect(buf[24] == 1); // ORIGIN type
    try std.testing.expect(buf[25] == 1); // length = 1
    try std.testing.expect(buf[26] == 0); // IGP origin value

    // Verify AS_PATH attribute (type 2) follows ORIGIN
    try std.testing.expect(buf[27] == 0x40); // TRANSITIVE flag
    try std.testing.expect(buf[28] == 2); // AS_PATH type
    try std.testing.expect(buf[29] == 0); // empty AS_PATH for same_as

    // Verify NEXT_HOP attribute (type 3) follows AS_PATH
    try std.testing.expect(buf[30] == 0x40); // TRANSITIVE flag
    try std.testing.expect(buf[31] == 3); // NEXT_HOP type
    try std.testing.expect(buf[32] == 4); // length = 4 (IPv4 address)

    // NEXT_HOP value should match our next_hop parameter
    try std.testing.expect(buf[33] == 192);
    try std.testing.expect(buf[34] == 168);
    try std.testing.expect(buf[35] == 1);
    try std.testing.expect(buf[36] == 1);

    // NLRI starts at offset 37 (after all path attributes)
    // For 10.0.0.0/8: prefix length byte (8) + first byte of address (10)
    // This is NOT the withdrawn section - it's the NLRI section
    try std.testing.expect(buf[37] == 8); // prefix length for /8
    try std.testing.expect(buf[38] == 10); // first byte of 10.0.0.0

    // The withdrawn section is 0 bytes (confirmed by bytes 19-20 = 0)
    // Therefore bytes 37+ are definitively in NLRI, NOT withdrawn routes
}

test "UPDATE wire format: complete byte-level structure for same_as" {
    // Comprehensive byte-level verification of UPDATE wire format.
    var buf: [1024]u8 = undefined;
    const prefix = types.Ipv4Prefix.init("10.0.0.0/8");
    const params = types.UpdateParams{
        .next_hop = .{ 192, 168, 1, 1 },
        .local_as = 65001,
        .same_as = true,
        .prefixes = &.{prefix},
    };
    const len = message.encodeUpdate(params, &buf);

    // Message structure breakdown:
    // - Header: 19 bytes (16 marker + 2 length + 1 type)
    // - Withdrawn routes length field: 2 bytes (value = 0)
    // - Path attributes length field: 2 bytes (value = 14)
    // - Path attributes: ORIGIN(4) + AS_PATH(3, empty) + NEXT_HOP(7) = 14 bytes
    // - NLRI: prefix_len_byte(1) + prefix_bytes(1) = 2 bytes
    // Total: 19 + 2 + 2 + 14 + 2 = 39 bytes
    const expected_len = 39;
    try std.testing.expectEqual(@as(usize, expected_len), len);

    // Bytes 0-15: marker (all 0xFF)
    for (0..16) |i| {
        try std.testing.expectEqual(@as(u8, 0xFF), buf[i]);
    }

    // Bytes 16-17: message length (big-endian 0x00, 0x27 = 39)
    try std.testing.expectEqual(@as(u8, 0x00), buf[16]);
    try std.testing.expectEqual(@as(u8, 0x27), buf[17]);

    // Byte 18: message type (2 = UPDATE)
    try std.testing.expectEqual(@as(u8, 2), buf[18]);

    // Bytes 19-20: withdrawn routes length = 0 (MUST be 0 for announcements)
    try std.testing.expectEqual(@as(u8, 0), buf[19]);
    try std.testing.expectEqual(@as(u8, 0), buf[20]);

    // Bytes 21-22: path attributes length = 14 (big-endian 0x00, 0x0E)
    try std.testing.expectEqual(@as(u8, 0), buf[21]);
    try std.testing.expectEqual(@as(u8, 14), buf[22]);

    // Bytes 23-26: ORIGIN attribute
    try std.testing.expectEqual(@as(u8, 0x40), buf[23]);
    try std.testing.expectEqual(@as(u8, 1), buf[24]); // ORIGIN type
    try std.testing.expectEqual(@as(u8, 1), buf[25]); // length = 1
    try std.testing.expectEqual(@as(u8, 0), buf[26]); // IGP origin

    // Bytes 27-29: AS_PATH attribute (empty for same_as)
    try std.testing.expectEqual(@as(u8, 0x40), buf[27]);
    try std.testing.expectEqual(@as(u8, 2), buf[28]); // AS_PATH type
    try std.testing.expectEqual(@as(u8, 0), buf[29]); // empty AS_PATH

    // Bytes 30-36: NEXT_HOP attribute
    try std.testing.expectEqual(@as(u8, 0x40), buf[30]);
    try std.testing.expectEqual(@as(u8, 3), buf[31]); // NEXT_HOP type
    try std.testing.expectEqual(@as(u8, 4), buf[32]); // length = 4
    try std.testing.expectEqual(@as(u8, 192), buf[33]);
    try std.testing.expectEqual(@as(u8, 168), buf[34]);
    try std.testing.expectEqual(@as(u8, 1), buf[35]);
    try std.testing.expectEqual(@as(u8, 1), buf[36]);

    // Bytes 37-38: NLRI prefix (NOT withdrawn!)
    try std.testing.expectEqual(@as(u8, 8), buf[37]); // prefix length /8
    try std.testing.expectEqual(@as(u8, 10), buf[38]); // first byte of 10.0.0.0
}

test "UPDATE wire format: complete byte-level structure for different_as" {
    // Comprehensive byte-level verification with AS_PATH segment.
    var buf: [1024]u8 = undefined;
    const prefix = types.Ipv4Prefix.init("10.0.0.0/8");
    const params = types.UpdateParams{
        .next_hop = .{ 192, 168, 1, 1 },
        .local_as = 65001,
        .same_as = false, // Include AS in AS_PATH
        .prefixes = &.{prefix},
    };
    const len = message.encodeUpdate(params, &buf);

    // Message structure breakdown:
    // - Header: 19 bytes
    // - Withdrawn routes length field: 2 bytes (value = 0)
    // - Path attributes length field: 2 bytes (value = 18)
    // - Path attributes: ORIGIN(4) + AS_PATH(7, with segment) + NEXT_HOP(7) = 18 bytes
    // - NLRI: 2 bytes
    // Total: 19 + 2 + 2 + 18 + 2 = 43 bytes
    const expected_len = 43;
    try std.testing.expectEqual(@as(usize, expected_len), len);

    // Withdrawn routes length = 0
    try std.testing.expectEqual(@as(u8, 0), buf[19]);
    try std.testing.expectEqual(@as(u8, 0), buf[20]);

    // Path attributes length = 18 (big-endian 0x00, 0x12)
    try std.testing.expectEqual(@as(u8, 0), buf[21]);
    try std.testing.expectEqual(@as(u8, 18), buf[22]);

    // ORIGIN at offset 23-26
    try std.testing.expectEqual(@as(u8, 0x40), buf[23]);
    try std.testing.expectEqual(@as(u8, 1), buf[24]); // ORIGIN type
    try std.testing.expectEqual(@as(u8, 1), buf[25]); // length = 1
    try std.testing.expectEqual(@as(u8, 0), buf[26]); // IGP

    // AS_PATH at offset 27-33 (7 bytes with segment)
    try std.testing.expectEqual(@as(u8, 0x40), buf[27]);
    try std.testing.expectEqual(@as(u8, 2), buf[28]); // AS_PATH type
    try std.testing.expectEqual(@as(u8, 4), buf[29]); // segment length = 4

    // AS_PATH segment: type=2 (AS_SEQUENCE), len=1, AS=65001
    try std.testing.expectEqual(@as(u8, 2), buf[30]); // AS_SEQUENCE
    try std.testing.expectEqual(@as(u8, 1), buf[31]); // segment length = 1
    try std.testing.expectEqual(@as(u8, 0xFD), buf[32]); // 65001 high byte
    try std.testing.expectEqual(@as(u8, 0xE9), buf[33]); // 65001 low byte

    // NEXT_HOP at offset 34-40
    try std.testing.expectEqual(@as(u8, 0x40), buf[34]);
    try std.testing.expectEqual(@as(u8, 3), buf[35]); // NEXT_HOP type
    try std.testing.expectEqual(@as(u8, 4), buf[36]); // length = 4
    try std.testing.expectEqual(@as(u8, 192), buf[37]);
    try std.testing.expectEqual(@as(u8, 168), buf[38]);
    try std.testing.expectEqual(@as(u8, 1), buf[39]);
    try std.testing.expectEqual(@as(u8, 1), buf[40]);

    // NLRI at offset 41-42
    try std.testing.expectEqual(@as(u8, 8), buf[41]); // prefix length /8
    try std.testing.expectEqual(@as(u8, 10), buf[42]); // first byte of 10.0.0.0
}

test "UPDATE NLRI position proves prefixes are NOT withdrawals" {
    // This test directly addresses the BIRD issue.
    // Bug scenario: buf[19]=high, buf[20]=low (withdrawn_len != 0)
    //   Prefix bytes immediately follow - BIRD reads as withdrawn
    // Fixed scenario: buf[19]=0, buf[20]=0 (withdrawn_len == 0)
    //   Path attributes follow, then NLRI prefixes - BIRD reads as announcements
    var buf: [1024]u8 = undefined;
    const prefix = types.Ipv4Prefix.init("10.0.0.0/8");
    const params = types.UpdateParams{
        .next_hop = .{ 192, 168, 1, 1 },
        .local_as = 65001,
        .same_as = true,
        .prefixes = &.{prefix},
    };
    const len = message.encodeUpdate(params, &buf);
    try std.testing.expect(len > 37);

    // Withdrawn routes length MUST be 0
    const withdrawn_len = @as(usize, buf[19]) * 256 + @as(usize, buf[20]);
    try std.testing.expectEqual(@as(usize, 0), withdrawn_len);

    // Path attributes length MUST be > 0
    const attrs_len = @as(usize, buf[21]) * 256 + @as(usize, buf[22]);
    try std.testing.expect(attrs_len > 0);

    // NLRI starts at: header(19) + withdrawn_len(2) + attrs_len_field(2) + attrs
    const nlri_offset = 19 + 2 + 2 + attrs_len;

    // The prefix should be at nlri_offset, NOT at offset 21
    try std.testing.expect(buf[nlri_offset] == 8); // prefix length
    try std.testing.expect(buf[nlri_offset + 1] == 10); // first byte of address

    // Verify the prefix is NOT at the path attributes length position (offset 21)
    // offset 21 is the path attributes length field, not the prefix
    try std.testing.expect(buf[21] == 0); // path attrs length high byte (14 = 0x0E)
    try std.testing.expect(buf[22] == 14); // path attrs length low byte
}

test "UPDATE multiple prefixes count in NLRI not withdrawn" {
    // Verify multiple prefixes are all in NLRI, none in withdrawn.
    var buf: [1024]u8 = undefined;
    const prefixes = &.{
        types.Ipv4Prefix.init("10.0.0.0/8"),
        types.Ipv4Prefix.init("192.168.0.0/16"),
        types.Ipv4Prefix.init("172.16.0.0/12"),
    };
    const params = types.UpdateParams{
        .next_hop = .{ 192, 168, 1, 1 },
        .local_as = 65001,
        .same_as = true,
        .prefixes = prefixes,
    };
    const len = message.encodeUpdate(params, &buf);
    try std.testing.expect(len > 23);

    // Withdrawn routes length MUST be 0 (all 3 prefixes are announcements)
    try std.testing.expect(buf[19] == 0);
    try std.testing.expect(buf[20] == 0);

    // Calculate NLRI offset
    const attrs_len = @as(usize, buf[21]) * 256 + @as(usize, buf[22]);
    const nlri_offset = 19 + 2 + 2 + attrs_len;

    // Count prefixes in NLRI section
    var nlri_count: usize = 0;
    var offset = nlri_offset;
    while (offset < len) {
        const prefix_len = buf[offset];
        offset += 1; // Skip length byte
        const byte_count: usize = if (prefix_len == 0) 0 else (prefix_len + 7) / 8;
        offset += byte_count;
        nlri_count += 1;
    }

    // All 3 prefixes should be in NLRI
    try std.testing.expectEqual(@as(usize, 3), nlri_count);
}
