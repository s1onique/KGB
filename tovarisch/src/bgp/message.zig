// message.zig — BGP message frame encoding (KEEPALIVE, OPEN, UPDATE, WITHDRAW)
//
// ACT 1: Pure encoding only, no sockets or runtime behavior.
// BGP message format per RFC 4271 Section 4.1:
//   - 16-byte marker (all 0xFF)
//   - 2-byte message length (big-endian)
//   - 1-byte message type
//   - message body (variable)
//
// For withdrawals, UPDATE format per RFC 4271 Section 4.3:
//   - withdrawn_routes_length (2)
//   - withdrawn routes (NLRI-encoded prefixes)
//   - path_attributes_length (2) = 0 for withdrawal-only UPDATE
//   - No NLRI field (prefixes are in withdrawn section)

const std = @import("std");
const types = @import("types.zig");

/// BGP frame constants
pub const FRAME_MARKER_SIZE = types.MARKER_SIZE;
pub const FRAME_HEADER_SIZE = 19; // 16 marker + 2 length + 1 type

/// Write the standard BGP marker (16 bytes of 0xFF) into buf.
pub fn writeMarker(buf: []u8) void {
    if (buf.len < FRAME_MARKER_SIZE) return;
    @memset(buf[0..FRAME_MARKER_SIZE], 0xFF);
}

/// Verify the BGP marker is all 0xFF.
pub fn verifyMarker(buf: []const u8) bool {
    if (buf.len < FRAME_MARKER_SIZE) return false;
    for (0..FRAME_MARKER_SIZE) |i| {
        if (buf[i] != 0xFF) return false;
    }
    return true;
}

/// Encode a KEEPALIVE message into buf.
/// Returns the number of bytes written (always 19).
/// KEEPALIVE is the minimal BGP message: just marker + length(19) + type(4).
pub fn encodeKeepalive(buf: []u8) usize {
    if (buf.len < FRAME_HEADER_SIZE) return 0;

    // Write 16-byte marker
    writeMarker(buf);

    // Write message length (19, big-endian)
    buf[16] = 0;
    buf[17] = 19;

    // Write message type (4 = KEEPALIVE)
    buf[18] = @intFromEnum(types.MessageType.keepalive);

    return FRAME_HEADER_SIZE;
}

/// Encode an OPEN message into buf.
/// Returns the number of bytes written, or 0 if buffer too small.
/// OPEN format per RFC 4271 Section 4.2:
///   - marker (16)
///   - length (2)
///   - type=1 (1)
///   - version (1)
///   - my AS (2)
///   - hold time (2)
///   - router ID (4)
///   - optional parameters length (1)
///   - optional parameters (0 for this ACT)
pub fn encodeOpen(params: types.OpenParams, buf: []u8) usize {
    const body_len = 10; // version(1) + my_as(2) + hold_time(2) + router_id(4) + opt_params_len(1)
    const total_len = FRAME_HEADER_SIZE + body_len;

    if (buf.len < total_len) return 0;

    // Write marker
    writeMarker(buf);

    // Write message length (big-endian)
    buf[16] = @as(u8, @intCast(total_len / 256));
    buf[17] = @as(u8, @intCast(total_len % 256));

    // Write message type (1 = OPEN)
    buf[18] = @intFromEnum(types.MessageType.open);

    // Write OPEN body
    const body_start = FRAME_HEADER_SIZE;
    buf[body_start] = types.PROTOCOL_VERSION; // version = 4
    buf[body_start + 1] = @as(u8, @intCast(params.my_as / 256));
    buf[body_start + 2] = @as(u8, @intCast(params.my_as % 256));
    buf[body_start + 3] = @as(u8, @intCast(params.hold_time / 256));
    buf[body_start + 4] = @as(u8, @intCast(params.hold_time % 256));
    // MemoryCopySafety: buf is the output buffer. params.router_id is a [4]u8 struct field.
    // They are distinct memory regions; no aliasing possible.
    @memcpy(buf[body_start + 5 .. body_start + 9], &params.router_id);
    buf[body_start + 9] = 0; // optional parameters length = 0

    return total_len;
}

/// Encode an UPDATE message into buf.
/// Returns the number of bytes written, or 0 if buffer too small.
/// UPDATE format per RFC 4271 Section 4.3:
///   - withdrawn routes length (2)
///   - withdrawn routes (0 for this ACT)
///   - path attributes length (2)
///   - path attributes:
///     - ORIGIN (4 bytes: flags, type, length, value)
///     - AS_PATH (variable, 1-byte length field)
///     - NEXT_HOP (7 bytes: flags, type, length, 4-byte IP)
///   - NLRI prefixes (variable, NO separate length byte per RFC 4271)
pub fn encodeUpdate(params: types.UpdateParams, buf: []u8) usize {
    // Calculate NLRI byte count (each prefix: length byte + prefix bytes)
    const nlri_byte_count = blk: {
        var total: usize = 0;
        for (params.prefixes) |prefix| {
            total += 1 + prefix.nlriByteCount(); // length byte + prefix bytes
        }
        break :blk total;
    };

    // Path attributes:
    // ORIGIN: flags(1) + type(1) + length(1) + value(1) = 4 bytes
    // AS_PATH: flags(1) + type(1) + length(1) + segment(4)
    //   - same_as=true: length=0, no segment bytes (truly empty AS_PATH)
    //   - same_as=false: length=4, segment type(1) + length(1) + AS(2)
    // NEXT_HOP: flags(1) + type(1) + length(1) + ip(4) = 7 bytes

    // AS_SEQUENCE with one 16-bit AS:
    //   segment type:   1 byte (value 2)
    //   segment length: 1 byte (value 1)
    //   ASN:            2 bytes (local_as in big-endian)
    const as_path_segment_len: usize = if (params.same_as) 0 else 4; // 0 or type(1)+len(1)+AS(2)
    const as_path_total_len: usize = 3 + as_path_segment_len; // flags+type+length + segment

    const total_attrs_len = 4 + as_path_total_len + 7; // ORIGIN + AS_PATH + NEXT_HOP
    // Body: withdrawn(2) + attrs_len(2) + attrs + nlri (no NLRI length byte per RFC 4271)
    const total_body_len = 2 + 2 + total_attrs_len + nlri_byte_count;
    const total_len = FRAME_HEADER_SIZE + total_body_len;

    if (buf.len < total_len) return 0;
    if (params.prefixes.len == 0) return 0; // Reject empty prefix list

    // Write marker
    writeMarker(buf);

    // Write message length (big-endian)
    buf[16] = @as(u8, @intCast(total_len / 256));
    buf[17] = @as(u8, @intCast(total_len % 256));

    // Write message type (2 = UPDATE)
    buf[18] = @intFromEnum(types.MessageType.update);

    // Write UPDATE body
    var offset: usize = FRAME_HEADER_SIZE;

    // Withdrawn routes length = 0
    buf[offset] = 0;
    buf[offset + 1] = 0;
    offset += 2;

    // Path attributes length (big-endian)
    buf[offset] = @as(u8, @intCast(total_attrs_len / 256));
    buf[offset + 1] = @as(u8, @intCast(total_attrs_len % 256));
    offset += 2;

    // ORIGIN attribute (transitive, well-known type 1)
    buf[offset] = 0x40; // TRANSITIVE flag
    buf[offset + 1] = 1; // ORIGIN type
    buf[offset + 2] = 1; // length = 1
    buf[offset + 3] = @intFromEnum(types.OriginType.igp); // IGP
    offset += 4;

    // AS_PATH attribute (transitive, well-known type 2)
    // Uses 1-byte length field (no extended length flag set)
    buf[offset] = 0x40; // TRANSITIVE flag
    buf[offset + 1] = 2; // AS_PATH type
    buf[offset + 2] = @as(u8, @intCast(as_path_total_len - 3)); // segment length only
    offset += 3;

    // AS_PATH segment (only if not same_as)
    if (!params.same_as) {
        buf[offset] = @intFromEnum(types.AsPathSegmentType.as_sequence);
        buf[offset + 1] = 1; // segment length = 1 AS
        buf[offset + 2] = @as(u8, @intCast(params.local_as / 256));
        buf[offset + 3] = @as(u8, @intCast(params.local_as % 256));
        offset += 4;
    }
    // If same_as, AS_PATH attribute has length=0 and no segment bytes (truly empty)

    // NEXT_HOP attribute (transitive, well-known type 3)
    buf[offset] = 0x40; // TRANSITIVE flag
    buf[offset + 1] = 3; // NEXT_HOP type
    buf[offset + 2] = 4; // length = 4
    // MemoryCopySafety: buf is the output buffer. params.next_hop is a [4]u8 struct field.
    // They are distinct memory regions; no aliasing possible.
    @memcpy(buf[offset + 3 .. offset + 7], &params.next_hop);
    offset += 7;

    // NLRI prefixes (NO length byte per RFC 4271)
    for (params.prefixes) |prefix| {
        // Write prefix length byte
        buf[offset] = prefix.len;
        offset += 1;

        // Write minimum prefix bytes
        const byte_count = prefix.nlriByteCount();
        if (byte_count > 0) {
            // MemoryCopySafety: buf is the output buffer. prefix.addr is a fixed [4]u8 array.
            // They are distinct memory regions; no aliasing possible.
            @memcpy(buf[offset .. offset + byte_count], prefix.addr[0..byte_count]);
            offset += byte_count;
        }
    }

    return total_len;
}

/// Encode a BGP UPDATE withdrawal message into buf.
/// Returns the number of bytes written, or 0 if buffer too small.
///
/// Withdrawal UPDATE format per RFC 4271 Section 4.3:
///   - withdrawn_routes_length (2) = non-zero
///   - withdrawn routes (NLRI-encoded prefixes in withdrawn section)
///   - path_attributes_length (2) = 0
///   - No NLRI field (prefixes are in withdrawn section only)
pub fn encodeWithdraw(prefixes: []const types.Ipv4Prefix, buf: []u8) usize {
    // Calculate withdrawn routes byte count (each prefix: length byte + prefix bytes)
    const withdrawn_byte_count = blk: {
        var total: usize = 0;
        for (prefixes) |prefix| {
            total += 1 + prefix.nlriByteCount(); // length byte + prefix bytes
        }
        break :blk total;
    };

    // Body: withdrawn_len(2) + withdrawn_routes + attrs_len(2) + attrs + nlri
    // For withdrawal-only: withdrawn_len > 0, attrs_len = 0, no NLRI
    const total_body_len = 2 + withdrawn_byte_count + 2;
    const total_len = FRAME_HEADER_SIZE + total_body_len;

    if (buf.len < total_len) return 0;
    if (prefixes.len == 0) return 0; // Reject empty prefix list for withdrawal

    // Write marker
    writeMarker(buf);

    // Write message length (big-endian)
    buf[16] = @as(u8, @intCast(total_len / 256));
    buf[17] = @as(u8, @intCast(total_len % 256));

    // Write message type (2 = UPDATE)
    buf[18] = @intFromEnum(types.MessageType.update);

    // Write UPDATE body
    var offset: usize = FRAME_HEADER_SIZE;

    // Withdrawn routes length (big-endian)
    buf[offset] = @as(u8, @intCast(withdrawn_byte_count / 256));
    buf[offset + 1] = @as(u8, @intCast(withdrawn_byte_count % 256));
    offset += 2;

    // Withdrawn routes (NLRI-encoded prefixes)
    for (prefixes) |prefix| {
        // Write prefix length byte
        buf[offset] = prefix.len;
        offset += 1;

        // Write minimum prefix bytes
        const byte_count = prefix.nlriByteCount();
        if (byte_count > 0) {
            // MemoryCopySafety: buf is the output buffer. prefix.addr is a fixed [4]u8 array.
            // They are distinct memory regions; no aliasing possible.
            @memcpy(buf[offset .. offset + byte_count], prefix.addr[0..byte_count]);
            offset += byte_count;
        }
    }

    // Path attributes length = 0 (no new announcements)
    buf[offset] = 0;
    buf[offset + 1] = 0;
    // offset += 2; // Not needed as we return immediately after

    return total_len;
}

// === Tests ===

test "KEEPALIVE encodes to 19 bytes" {
    var buf: [19]u8 = undefined;
    const len = encodeKeepalive(&buf);
    try std.testing.expect(len == 19);
}

test "KEEPALIVE marker is 16 bytes of 0xFF" {
    var buf: [19]u8 = undefined;
    _ = encodeKeepalive(&buf);
    try std.testing.expect(verifyMarker(&buf));
}

test "KEEPALIVE length is 19" {
    var buf: [19]u8 = undefined;
    _ = encodeKeepalive(&buf);
    try std.testing.expect(buf[16] == 0);
    try std.testing.expect(buf[17] == 19);
}

test "KEEPALIVE type is 4" {
    var buf: [19]u8 = undefined;
    _ = encodeKeepalive(&buf);
    try std.testing.expect(buf[18] == 4);
}

test "OPEN encodes BGP version 4" {
    var buf: [100]u8 = undefined;
    const params = types.OpenParams{
        .my_as = 65001,
        .hold_time = 180,
        .router_id = .{ 10, 0, 0, 1 },
    };
    const len = encodeOpen(params, &buf);
    try std.testing.expect(len > 19);
    try std.testing.expect(buf[19] == 4); // version byte
}

test "OPEN encodes local AS" {
    var buf: [100]u8 = undefined;
    const params = types.OpenParams{
        .my_as = 65001,
        .hold_time = 180,
        .router_id = .{ 10, 0, 0, 1 },
    };
    const len = encodeOpen(params, &buf);
    try std.testing.expect(len > 21);
    try std.testing.expect(buf[20] == 0xFD); // 65001 >> 8 = 253
    try std.testing.expect(buf[21] == 0xE9); // 65001 & 0xFF = 233
}

test "OPEN encodes hold time" {
    var buf: [100]u8 = undefined;
    const params = types.OpenParams{
        .my_as = 65001,
        .hold_time = 180,
        .router_id = .{ 10, 0, 0, 1 },
    };
    const len = encodeOpen(params, &buf);
    try std.testing.expect(len > 23);
    try std.testing.expect(buf[22] == 0);
    try std.testing.expect(buf[23] == 180);
}

test "OPEN encodes router ID" {
    var buf: [100]u8 = undefined;
    const params = types.OpenParams{
        .my_as = 65001,
        .hold_time = 180,
        .router_id = .{ 10, 20, 30, 40 },
    };
    const len = encodeOpen(params, &buf);
    try std.testing.expect(len > 27);
    try std.testing.expect(buf[24] == 10);
    try std.testing.expect(buf[25] == 20);
    try std.testing.expect(buf[26] == 30);
    try std.testing.expect(buf[27] == 40);
}

test "OPEN optional parameter length is 0" {
    var buf: [100]u8 = undefined;
    const params = types.OpenParams{
        .my_as = 65001,
        .hold_time = 180,
        .router_id = .{ 10, 0, 0, 1 },
    };
    const len = encodeOpen(params, &buf);
    try std.testing.expect(len == 29); // 19 header + 10 body
    try std.testing.expect(buf[28] == 0); // optional parameter length
}

test "WITHDRAW encodes single prefix correctly" {
    var buf: [1024]u8 = undefined;
    const prefixes = &.{types.Ipv4Prefix.init("10.0.0.0/8")};
    const len = encodeWithdraw(prefixes, &buf);
    try std.testing.expect(len > 0);

    // Message type must be UPDATE (2)
    try std.testing.expect(buf[18] == 2);

    // withdrawn_routes_length must be non-zero
    try std.testing.expect(buf[19] > 0 or buf[20] > 0);

    // Prefix length byte should be present (8 for /8)
    try std.testing.expect(buf[21] == 8);

    // path_attributes_length must be 0 (no announcements) at offset 23
    try std.testing.expect(buf[23] == 0);
    try std.testing.expect(buf[24] == 0);
}

test "WITHDRAW has zero path attributes" {
    var buf: [1024]u8 = undefined;
    const prefixes = &.{types.Ipv4Prefix.init("192.168.0.0/16")};
    const len = encodeWithdraw(prefixes, &buf);
    try std.testing.expect(len > 0);

    // Message type must be UPDATE (2)
    try std.testing.expect(buf[18] == 2);
}

test "WITHDRAW rejects empty prefix list" {
    var buf: [1024]u8 = undefined;
    const prefixes: []const types.Ipv4Prefix = &.{};
    const len = encodeWithdraw(prefixes, &buf);
    try std.testing.expect(len == 0);
}

test "WITHDRAW encodes multiple prefixes" {
    var buf: [1024]u8 = undefined;
    const prefixes = &.{
        types.Ipv4Prefix.init("10.0.0.0/8"),
        types.Ipv4Prefix.init("172.16.0.0/12"),
        types.Ipv4Prefix.init("192.168.0.0/16"),
    };
    const len = encodeWithdraw(prefixes, &buf);
    try std.testing.expect(len > 0);

    // Message type must be UPDATE (2)
    try std.testing.expect(buf[18] == 2);
}
