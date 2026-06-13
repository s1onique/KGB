// frame_decode.zig — BGP message frame decoding
//
// ACT 2: Decodes incoming BGP frames for session runtime and tests.
// This module validates frame structure but does NOT import learned routes.
// References: RFC 4271 Section 4.1

const std = @import("std");
const types = @import("types.zig");

/// Decode errors for BGP frame parsing
pub const DecodeError = error{
    /// Marker is not all 0xFF
    BadMarker,
    /// Message length < 19
    LengthTooShort,
    /// Message length > 4096
    LengthTooLong,
    /// Incomplete frame (buffer too short for declared length)
    IncompleteFrame,
    /// Unknown or unsupported message type
    UnknownMessageType,
};

/// BGP OPEN message body (minimally parsed)
pub const OpenBody = struct {
    /// BGP version (must be 4)
    version: u8,
    /// Peer's AS number
    peer_as: u16,
    /// Peer's hold time
    hold_time: u16,
    /// Peer's router ID
    router_id: [4]u8,
    /// Optional parameters length
    opt_params_len: u8,
};

/// BGP NOTIFICATION message body (minimally parsed)
pub const NotificationBody = struct {
    /// Error code
    error_code: u8,
    /// Error subcode
    error_subcode: u8,
};

/// Decoded BGP message frame
pub const Frame = struct {
    /// Message type
    msg_type: types.MessageType,
    /// Full message length (header + body)
    length: u16,
    /// Body slice (after 19-byte header, may be empty for KEEPALIVE)
    body: []const u8,
};

/// Verify the BGP marker is all 0xFF.
pub fn verifyMarker(buf: []const u8) bool {
    if (buf.len < types.MARKER_SIZE) return false;
    for (0..types.MARKER_SIZE) |i| {
        if (buf[i] != 0xFF) return false;
    }
    return true;
}

/// Decode a BGP frame header and validate structure.
/// Returns a Frame with the message type and body slice.
/// Does NOT fully parse UPDATE path attributes (import-nothing).
pub fn decodeFrame(buf: []const u8) DecodeError!Frame {
    // Must have at least the header (16 marker + 2 length + 1 type)
    if (buf.len < types.MIN_MESSAGE_LENGTH) {
        return DecodeError.LengthTooShort;
    }

    // Verify marker (first 16 bytes)
    if (!verifyMarker(buf)) {
        return DecodeError.BadMarker;
    }

    // Parse message length (bytes 16-17, big-endian)
    const length = @as(u16, buf[16]) * 256 + @as(u16, buf[17]);

    // Validate length bounds
    if (length < types.MIN_MESSAGE_LENGTH) {
        return DecodeError.LengthTooShort;
    }
    if (length > types.MAX_MESSAGE_LENGTH) {
        return DecodeError.LengthTooLong;
    }

    // Verify we have the complete frame
    if (buf.len < length) {
        return DecodeError.IncompleteFrame;
    }

    // Parse message type
    const type_val = buf[18];
    const msg_type: types.MessageType = switch (type_val) {
        1 => .open,
        2 => .update,
        3 => .notification,
        4 => .keepalive,
        else => return DecodeError.UnknownMessageType,
    };

    // Extract body slice (after 19-byte header)
    const body = buf[types.MIN_MESSAGE_LENGTH..length];

    return Frame{
        .msg_type = msg_type,
        .length = length,
        .body = body,
    };
}

/// Parse an OPEN message body minimally.
/// Returns OpenBody with version, peer AS, hold time, router ID.
/// Note: Does not parse optional parameters.
pub fn parseOpenBody(body: []const u8) DecodeError!OpenBody {
    // OPEN body minimum: version(1) + my_as(2) + hold_time(2) + router_id(4) + opt_params_len(1) = 10
    if (body.len < 10) {
        return DecodeError.IncompleteFrame;
    }

    const version = body[0];
    if (version != types.PROTOCOL_VERSION) {
        // Version must be 4 for BGP-4
        return DecodeError.UnknownMessageType;
    }

    const peer_as = @as(u16, body[1]) * 256 + @as(u16, body[2]);
    const hold_time = @as(u16, body[3]) * 256 + @as(u16, body[4]);
    const router_id = body[5..9].*;
    const opt_params_len = body[9];

    return OpenBody{
        .version = version,
        .peer_as = peer_as,
        .hold_time = hold_time,
        .router_id = router_id,
        .opt_params_len = opt_params_len,
    };
}

/// Parse a NOTIFICATION message body minimally.
/// Returns NotificationBody with error code and subcode.
pub fn parseNotificationBody(body: []const u8) DecodeError!NotificationBody {
    // NOTIFICATION body minimum: error_code(1) + error_subcode(1) = 2
    if (body.len < 2) {
        return DecodeError.IncompleteFrame;
    }

    return NotificationBody{
        .error_code = body[0],
        .error_subcode = body[1],
    };
}

/// Check if a frame is a KEEPALIVE (type 4, body length 0).
pub fn isKeepalive(frame: Frame) bool {
    return frame.msg_type == .keepalive and frame.body.len == 0;
}

/// Check if a frame is an OPEN (type 1).
pub fn isOpen(frame: Frame) bool {
    return frame.msg_type == .open;
}

/// Check if a frame is an UPDATE (type 2).
pub fn isUpdate(frame: Frame) bool {
    return frame.msg_type == .update;
}

/// Check if a frame is a NOTIFICATION (type 3).
pub fn isNotification(frame: Frame) bool {
    return frame.msg_type == .notification;
}

/// Decoded UPDATE message body metadata.
/// Used for structural validation of sent UPDATE frames.
pub const UpdateBody = struct {
    /// withdrawn_routes_length (MUST be 0 for route announcements)
    withdrawn_routes_length: u16,
    /// total_path_attributes_length (MUST be > 0)
    path_attributes_length: u16,
    /// Number of NLRI prefixes detected
    nlri_prefix_count: usize,
    /// Total NLRI bytes (after path attributes)
    nlri_byte_count: usize,
    /// Full message length
    total_length: u16,
};

/// Decode an UPDATE frame's body metadata.
/// Returns UpdateBody with key structural fields for validation.
/// 
/// This decodes the UPDATE wire format per RFC 4271 Section 4.3:
///   - withdrawn_routes_length (2 bytes)
///   - path_attributes_length (2 bytes)
///   - path attributes (variable)
///   - NLRI prefixes (variable, no length byte per RFC 4271)
///
/// Used for test verification and debug instrumentation.
pub fn parseUpdateBody(frame: Frame) ?UpdateBody {
    const body = frame.body;
    if (body.len < 4) return null; // Need at least 2+2 bytes for withdrawn_len + attrs_len

    // Withdrawn routes length (bytes 0-1, big-endian)
    const withdrawn_len = @as(u16, body[0]) * 256 + @as(u16, body[1]);

    // Path attributes length (bytes 2-3, big-endian)
    const attrs_len = @as(u16, body[2]) * 256 + @as(u16, body[3]);

    // NLRI starts after the fixed fields and path attributes in the body:
    // body = [withdrawn_len(2) + withdrawn_routes(withdrawn_len) + attrs_len(2) + path_attrs(attrs_len) + NLRI]
    // The fixed header (19 bytes) is already excluded from body.
    // NLRI offset within body = 2 (withdrawn_len field) + withdrawn_len + 2 (attrs_len field) + attrs_len
    const nlri_start_in_body = 2 + withdrawn_len + 2 + attrs_len;
    if (body.len < 4 + withdrawn_len + attrs_len) return null;
    if (body.len < nlri_start_in_body) return null;

    // Count NLRI prefixes
    var nlri_offset = nlri_start_in_body;
    var prefix_count: usize = 0;
    var nlri_bytes: usize = 0;
    while (nlri_offset < body.len) {
        const prefix_len_bits = body[nlri_offset];
        nlri_offset += 1; // Skip length byte
        
        if (prefix_len_bits == 0) break; // Safety: no more prefixes
        
        const prefix_len_bytes = (prefix_len_bits + 7) / 8;
        if (nlri_offset + prefix_len_bytes > body.len) break;
        
        nlri_offset += prefix_len_bytes;
        prefix_count += 1;
        nlri_bytes += 1 + prefix_len_bytes;
    }

    return UpdateBody{
        .withdrawn_routes_length = withdrawn_len,
        .path_attributes_length = attrs_len,
        .nlri_prefix_count = prefix_count,
        .nlri_byte_count = nlri_bytes,
        .total_length = frame.length,
    };
}

// === Tests ===

test "Frame rejects bad marker" {
    var buf: [19]u8 = undefined;
    @memset(&buf, 0x00); // Bad marker - all zeros
    buf[16] = 0;
    buf[17] = 19;
    buf[18] = 4; // KEEPALIVE type

    try std.testing.expectError(DecodeError.BadMarker, decodeFrame(&buf));
}

test "Frame rejects good marker with bad length" {
    var buf: [19]u8 = undefined;
    @memset(buf[0..16], 0xFF); // Good marker
    buf[16] = 0;
    buf[17] = 18; // Length 18 is too short (min is 19)
    buf[18] = 4;

    try std.testing.expectError(DecodeError.LengthTooShort, decodeFrame(&buf));
}

test "Frame rejects length above max" {
    var buf: [100]u8 = undefined;
    @memset(buf[0..16], 0xFF);
    // Set length to 4097 (exceeds MAX_MESSAGE_LENGTH of 4096)
    buf[16] = 0x10;
    buf[17] = 0x01;
    buf[18] = 4;

    try std.testing.expectError(DecodeError.LengthTooLong, decodeFrame(&buf));
}

test "Frame recognizes KEEPALIVE" {
    var buf: [19]u8 = undefined;
    @memset(buf[0..16], 0xFF);
    buf[16] = 0;
    buf[17] = 19;
    buf[18] = 4; // KEEPALIVE

    const frame = try decodeFrame(&buf);
    try std.testing.expect(isKeepalive(frame));
    try std.testing.expectEqual(@as(usize, 0), frame.body.len);
}

test "Frame parses OPEN minimally" {
    // Build a valid OPEN frame
    var buf: [29]u8 = undefined;
    @memset(buf[0..16], 0xFF);
    buf[16] = 0;
    buf[17] = 29; // Total length
    buf[18] = 1; // OPEN type
    buf[19] = 4; // version
    // peer AS = 0xFEEE = 65262 (big-endian: high byte 0xFE, low byte 0xEE)
    buf[20] = 0xFE;
    buf[21] = 0xEE;
    buf[22] = 0;
    buf[23] = 180; // hold time
    buf[24] = 10;
    buf[25] = 0;
    buf[26] = 0;
    buf[27] = 1; // router ID
    buf[28] = 0; // opt params len

    const frame = try decodeFrame(&buf);
    try std.testing.expect(isOpen(frame));

    const open_body = try parseOpenBody(frame.body);
    try std.testing.expectEqual(@as(u8, 4), open_body.version);
    try std.testing.expectEqual(@as(u16, 65262), open_body.peer_as); // 0xFE * 256 + 0xEE = 65262
    try std.testing.expectEqual(@as(u16, 180), open_body.hold_time);
    try std.testing.expectEqualSlices(u8, &.{ 10, 0, 0, 1 }, &open_body.router_id);
    try std.testing.expectEqual(@as(u8, 0), open_body.opt_params_len);
}

test "Frame parses NOTIFICATION code/subcode" {
    // Build a NOTIFICATION frame: header + error_code + error_subcode
    var buf: [21]u8 = undefined;
    @memset(buf[0..16], 0xFF);
    buf[16] = 0;
    buf[17] = 21; // Total length
    buf[18] = 3; // NOTIFICATION type
    buf[19] = 6; // Cease error code
    buf[20] = 0; // no subcode

    const frame = try decodeFrame(&buf);
    try std.testing.expect(isNotification(frame));

    const notif = try parseNotificationBody(frame.body);
    try std.testing.expectEqual(@as(u8, 6), notif.error_code);
    try std.testing.expectEqual(@as(u8, 0), notif.error_subcode);
}

test "Frame rejects incomplete frame" {
    var buf: [19]u8 = undefined;
    @memset(buf[0..16], 0xFF);
    buf[16] = 0;
    buf[17] = 29; // Declares 29 bytes
    buf[18] = 1; // OPEN type
    // But only 19 bytes provided (incomplete for 29-byte OPEN)

    try std.testing.expectError(DecodeError.IncompleteFrame, decodeFrame(&buf));
}

test "Frame rejects unknown message type" {
    var buf: [19]u8 = undefined;
    @memset(buf[0..16], 0xFF);
    buf[16] = 0;
    buf[17] = 19;
    buf[18] = 99; // Unknown type

    try std.testing.expectError(DecodeError.UnknownMessageType, decodeFrame(&buf));
}

test "isUpdate and isNotification work correctly" {
    var buf: [19]u8 = undefined;
    @memset(buf[0..16], 0xFF);
    buf[16] = 0;
    buf[17] = 19;

    buf[18] = 2; // UPDATE
    var frame = try decodeFrame(&buf);
    try std.testing.expect(isUpdate(frame));
    try std.testing.expect(!isNotification(frame));

    buf[18] = 3; // NOTIFICATION
    frame = try decodeFrame(&buf);
    try std.testing.expect(!isUpdate(frame));
    try std.testing.expect(isNotification(frame));
}

// Note: UPDATE-specific decode tests have been moved to update_frame_decode_tests.zig
// to satisfy LLM-friendliness line limits.
