// update_frame_decode_tests.zig — UPDATE frame decode tests
//
// Extracted from frame_decode.zig to satisfy LLM-friendliness line limits.
// Tests for parseUpdateBody() function.

const std = @import("std");
const frame_decode = @import("frame_decode.zig");
const types = @import("types.zig");

test "parseUpdateBody decodes wire format correctly" {
    // Build an UPDATE frame matching message_wire_format_tests.zig structure
    // Header(19) + withdrawn_len(2) + attrs_len(2) + ORIGIN(4) + AS_PATH(3) + NEXT_HOP(7) + NLRI(2)
    // Total: 39 bytes for same_as with 10.0.0.0/8 prefix
    var buf: [1024]u8 = undefined;
    @memset(buf[0..16], 0xFF);
    buf[16] = 0;
    buf[17] = 39; // Total length
    buf[18] = 2; // UPDATE type
    
    // Withdrawn routes length = 0 (CRITICAL for route announcements)
    buf[19] = 0;
    buf[20] = 0;
    
    // Path attributes length = 14
    buf[21] = 0;
    buf[22] = 14;
    
    // ORIGIN attribute (transitive, type 1)
    buf[23] = 0x40;
    buf[24] = 1;
    buf[25] = 1;
    buf[26] = 0; // IGP
    
    // AS_PATH attribute (empty for same_as)
    buf[27] = 0x40;
    buf[28] = 2;
    buf[29] = 0;
    
    // NEXT_HOP attribute
    buf[30] = 0x40;
    buf[31] = 3;
    buf[32] = 4;
    buf[33] = 192;
    buf[34] = 168;
    buf[35] = 1;
    buf[36] = 1;
    
    // NLRI: 10.0.0.0/8
    buf[37] = 8; // prefix length
    buf[38] = 10; // first byte

    const frame = try frame_decode.decodeFrame(buf[0..39]);
    try std.testing.expect(frame_decode.isUpdate(frame));
    
    const update = frame_decode.parseUpdateBody(frame);
    try std.testing.expect(update != null);
    try std.testing.expectEqual(@as(u16, 0), update.?.withdrawn_routes_length);
    try std.testing.expectEqual(@as(u16, 14), update.?.path_attributes_length);
    try std.testing.expectEqual(@as(usize, 1), update.?.nlri_prefix_count);
    try std.testing.expect(update.?.nlri_byte_count > 0);
    try std.testing.expectEqual(@as(u16, 39), update.?.total_length);
}

test "parseUpdateBody detects multiple prefixes" {
    // Build UPDATE with 3 NLRI prefixes:
    // - 10.0.0.0/8: 2 NLRI bytes (1 length byte + 1 address byte)
    // - 192.168.0.0/16: 3 NLRI bytes (1 length byte + 2 address bytes)
    // - 172.16.0.0/12: 3 NLRI bytes (1 length byte + 2 address bytes)
    // Total NLRI: 8 bytes
    var buf: [1024]u8 = undefined;
    @memset(buf[0..16], 0xFF);
    buf[16] = 0;
    buf[17] = 47; // 19 + 2 + 2 + 14 + 8 = 47 (header + withdrawn + attrs_len + attrs + NLRI)
    buf[18] = 2; // UPDATE type
    
    buf[19] = 0; buf[20] = 0; // withdrawn_routes_length = 0
    buf[21] = 0; buf[22] = 14; // path_attributes_length = 14
    
    buf[23] = 0x40; buf[24] = 1; buf[25] = 1; buf[26] = 0; // ORIGIN
    buf[27] = 0x40; buf[28] = 2; buf[29] = 0; // AS_PATH (empty for same_as)
    buf[30] = 0x40; buf[31] = 3; buf[32] = 4; // NEXT_HOP header
    buf[33] = 192; buf[34] = 168; buf[35] = 1; buf[36] = 1; // NEXT_HOP value
    
    // NLRI: 3 prefixes
    buf[37] = 8; buf[38] = 10; // 10.0.0.0/8
    buf[39] = 16; buf[40] = 192; buf[41] = 168; // 192.168.0.0/16
    buf[42] = 12; buf[43] = 172; buf[44] = 16; // 172.16.0.0/12

    const frame = try frame_decode.decodeFrame(buf[0..47]);
    const update = frame_decode.parseUpdateBody(frame);
    try std.testing.expect(update != null);
    try std.testing.expectEqual(@as(usize, 3), update.?.nlri_prefix_count);
    try std.testing.expectEqual(@as(u16, 0), update.?.withdrawn_routes_length);
}

test "parseUpdateBody returns null for malformed body" {
    // Build a truncated UPDATE frame (body is too short to contain full attrs)
    var buf: [25]u8 = undefined;
    @memset(buf[0..16], 0xFF);
    buf[16] = 0;
    buf[17] = 25; // Declared length
    buf[18] = 2; // UPDATE type
    
    // Withdrawn routes length = 0
    buf[19] = 0; buf[20] = 0;
    // Path attributes length = 14 (but body only has 6 bytes total!)
    buf[21] = 0; buf[22] = 14;
    // Only 2 more bytes in body
    buf[23] = 0; buf[24] = 0;
    
    const frame = try frame_decode.decodeFrame(buf[0..25]);
    try std.testing.expectEqual(@as(usize, 6), frame.body.len);
    
    // Should return null due to body too short for declared attrs_len
    const update = frame_decode.parseUpdateBody(frame);
    try std.testing.expect(update == null);
}

test "parseUpdateBody handles empty AS_PATH correctly" {
    // Verify AS_PATH length field is 0 for same_as configuration
    var buf: [1024]u8 = undefined;
    @memset(buf[0..16], 0xFF);
    buf[16] = 0;
    buf[17] = 39;
    buf[18] = 2;
    buf[19] = 0; buf[20] = 0;
    buf[21] = 0; buf[22] = 14; // AS_PATH length = 0 (empty)
    buf[23] = 0x40; buf[24] = 1; buf[25] = 1; buf[26] = 0; // ORIGIN
    buf[27] = 0x40; buf[28] = 2; buf[29] = 0; // AS_PATH: type=2, length=0
    buf[30] = 0x40; buf[31] = 3; buf[32] = 4; // NEXT_HOP
    buf[33] = 10; buf[34] = 0; buf[35] = 0; buf[36] = 1;
    buf[37] = 8; buf[38] = 10; // NLRI

    const frame = try frame_decode.decodeFrame(buf[0..39]);
    const update = frame_decode.parseUpdateBody(frame);
    try std.testing.expect(update != null);
    try std.testing.expectEqual(@as(u16, 0), update.?.withdrawn_routes_length);
    try std.testing.expectEqual(@as(u16, 14), update.?.path_attributes_length);
}

test "parseUpdateBody handles non-empty AS_PATH correctly" {
    // Verify AS_PATH length field is 4 for different_as configuration
    var buf: [1024]u8 = undefined;
    @memset(buf[0..16], 0xFF);
    buf[16] = 0;
    buf[17] = 43; // 19 + 2 + 2 + 18 + 2 = 43
    buf[18] = 2;
    buf[19] = 0; buf[20] = 0;
    buf[21] = 0; buf[22] = 18; // AS_PATH length = 4 (has segment)
    buf[23] = 0x40; buf[24] = 1; buf[25] = 1; buf[26] = 0; // ORIGIN
    buf[27] = 0x40; buf[28] = 2; buf[29] = 4; // AS_PATH: type=2, length=4
    // AS_PATH segment: type=2 (AS_SEQUENCE), len=1, AS=65001
    buf[30] = 2; buf[31] = 1; buf[32] = 0xFD; buf[33] = 0xE9;
    buf[34] = 0x40; buf[35] = 3; buf[36] = 4; // NEXT_HOP
    buf[37] = 10; buf[38] = 0; buf[39] = 0; buf[40] = 1;
    buf[41] = 8; buf[42] = 10; // NLRI

    const frame = try frame_decode.decodeFrame(buf[0..43]);
    const update = frame_decode.parseUpdateBody(frame);
    try std.testing.expect(update != null);
    try std.testing.expectEqual(@as(u16, 18), update.?.path_attributes_length);
    try std.testing.expectEqual(@as(usize, 1), update.?.nlri_prefix_count);
}
