// notification_decode.zig — BGP NOTIFICATION message decoding
//
// Decodes BGP NOTIFICATION error codes and subcodes into human-readable strings.
// References: RFC 4271 Section 4.5 and BGP IANA registry.
//
// Error codes and subcodes per RFC 4271:
//   1 = Message Header Error
//     1 = Not Synchronized
//     2 = Bad Message Length
//     3 = Bad Message Type
//   2 = OPEN Message Error
//     1 = Unsupported Version Number
//     2 = Bad Peer AS
//     3 = Bad BGP Identifier
//     4 = Unsupported Optional Parameter
//     5 = Authentication Failure (deprecated)
//     6 = Unacceptable Hold Time
//   3 = UPDATE Message Error
//     1 = Malformed Attribute List
//     2 = Unrecognized Well-Known Attribute
//     3 = Missing Well-Known Attribute
//     4 = Attribute Flags Error
//     5 = Attribute Length Error
//     6 = Invalid ORIGIN Attribute
//     7 = AS_ROUTE Loop (deprecated)
//     8 = Invalid NEXT_HOP Attribute
//     9 = Optional Attribute Error
//    10 = Invalid Network Field
//    11 = Malformed AS_PATH
//   4 = Hold Timer Expired
//     0 = No subcode
//   5 = Finite State Machine Error
//     1 = Unexpected Message
//     2 = Error Subcode
//   6 = Cease
//     0 = No subcode
//   7 = Routing Loop (deprecated)
//   8 = Notification (deprecated)

const std = @import("std");

/// BGP NOTIFICATION error code names
pub const ErrorCodeName = struct {
    code: u8,
    name: []const u8,
};

/// BGP NOTIFICATION error subcode names
pub const ErrorSubcodeName = struct {
    code: u8,
    subcode: u8,
    name: []const u8,
};

/// Get the human-readable name for an error code.
pub fn getErrorCodeName(code: u8) []const u8 {
    return switch (code) {
        1 => "Message Header Error",
        2 => "OPEN Message Error",
        3 => "UPDATE Message Error",
        4 => "Hold Timer Expired",
        5 => "Finite State Machine Error",
        6 => "Cease",
        7 => "Routing Loop",
        8 => "Notification",
        else => "Unknown Error",
    };
}

/// Get the human-readable name for an error subcode.
/// Pass the error code as context since subcodes are code-specific.
pub fn getErrorSubcodeName(code: u8, subcode: u8) []const u8 {
    switch (code) {
        1 => { // Message Header Error
            return switch (subcode) {
                1 => "Not Synchronized",
                2 => "Bad Message Length",
                3 => "Bad Message Type",
                else => "Unknown Subcode",
            };
        },
        2 => { // OPEN Message Error
            return switch (subcode) {
                1 => "Unsupported Version Number",
                2 => "Bad Peer AS",
                3 => "Bad BGP Identifier",
                4 => "Unsupported Optional Parameter",
                5 => "Authentication Failure",
                6 => "Unacceptable Hold Time",
                else => "Unknown Subcode",
            };
        },
        3 => { // UPDATE Message Error
            return switch (subcode) {
                1 => "Malformed Attribute List",
                2 => "Unrecognized Well-Known Attribute",
                3 => "Missing Well-Known Attribute",
                4 => "Attribute Flags Error",
                5 => "Attribute Length Error",
                6 => "Invalid ORIGIN Attribute",
                7 => "AS_ROUTE Loop",
                8 => "Invalid NEXT_HOP Attribute",
                9 => "Optional Attribute Error",
               10 => "Invalid Network Field",
               11 => "Malformed AS_PATH",
                else => "Unknown Subcode",
            };
        },
        4 => { // Hold Timer Expired
            // Hold Timer Expired subcode is always 0 per RFC 4271
            return "No subcode";
        },
        5 => { // Finite State Machine Error
            return switch (subcode) {
                1 => "Unexpected Message",
                2 => "Error Subcode",
                else => "Unknown Subcode",
            };
        },
        6 => { // Cease
            // Cease subcode is always 0 per RFC 4271
            return "No subcode";
        },
        else => return "Unknown Subcode",
    }
}

/// Format a NOTIFICATION as a human-readable string.
/// Writes to the provided buffer and returns a slice.
pub fn formatNotification(
    code: u8,
    subcode: u8,
    buf: *[64]u8,
) []const u8 {
    const code_name = getErrorCodeName(code);
    const subcode_name = getErrorSubcodeName(code, subcode);

    const result = std.fmt.bufPrint(buf, "{s} ({d}/{d}: {s})", .{
        code_name,
        code,
        subcode,
        subcode_name,
    }) catch {
        // Buffer too small - just return code name
        // MemoryCopySafety: buf is a caller-provided fixed buffer. code_name is a
        // compile-time constant string. They are distinct memory regions.
        @memcpy(buf[0..code_name.len], code_name);
        return buf[0..code_name.len];
    };

    return result;
}

// === Tests ===

test "getErrorCodeName returns correct names" {
    try std.testing.expectEqualStrings("Message Header Error", getErrorCodeName(1));
    try std.testing.expectEqualStrings("OPEN Message Error", getErrorCodeName(2));
    try std.testing.expectEqualStrings("UPDATE Message Error", getErrorCodeName(3));
    try std.testing.expectEqualStrings("Hold Timer Expired", getErrorCodeName(4));
    try std.testing.expectEqualStrings("Finite State Machine Error", getErrorCodeName(5));
    try std.testing.expectEqualStrings("Cease", getErrorCodeName(6));
    try std.testing.expectEqualStrings("Unknown Error", getErrorCodeName(99));
}

test "getErrorSubcodeName returns correct names for Hold Timer Expired" {
    try std.testing.expectEqualStrings("No subcode", getErrorSubcodeName(4, 0));
    try std.testing.expectEqualStrings("No subcode", getErrorSubcodeName(4, 1));
}

test "getErrorSubcodeName returns correct names for OPEN errors" {
    try std.testing.expectEqualStrings("Unsupported Version Number", getErrorSubcodeName(2, 1));
    try std.testing.expectEqualStrings("Bad Peer AS", getErrorSubcodeName(2, 2));
    try std.testing.expectEqualStrings("Unacceptable Hold Time", getErrorSubcodeName(2, 6));
}

test "getErrorSubcodeName returns correct names for UPDATE errors" {
    try std.testing.expectEqualStrings("Malformed Attribute List", getErrorSubcodeName(3, 1));
    try std.testing.expectEqualStrings("Malformed AS_PATH", getErrorSubcodeName(3, 11));
}

test "formatNotification formats correctly" {
    var buf: [64]u8 = undefined;
    const result = formatNotification(4, 0, &buf);
    try std.testing.expect(std.mem.containsAtLeast(u8, result, 1, "Hold Timer Expired"));
    try std.testing.expect(std.mem.containsAtLeast(u8, result, 1, "4"));
    try std.testing.expect(std.mem.containsAtLeast(u8, result, 1, "0"));
}

test "formatNotification formats Cease correctly" {
    var buf: [64]u8 = undefined;
    const result = formatNotification(6, 0, &buf);
    try std.testing.expect(std.mem.containsAtLeast(u8, result, 1, "Cease"));
    try std.testing.expect(std.mem.containsAtLeast(u8, result, 1, "6"));
}
