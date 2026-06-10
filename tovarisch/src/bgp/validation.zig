// validation.zig — BGP config validation helpers
//
// ACT 1: Pure validation for OPEN/UPDATE params, no sockets or runtime.
// All input is treated as untrusted.

const std = @import("std");

/// Validation errors for BGP config values
pub const ValidationError = error{
    /// ASN is not in 1..65535 range
    AsnOutOfRange,
    /// 32-bit ASN not supported in this ACT
    AsnTooLarge,
    /// Hold time must be 0 or >= 3
    InvalidHoldTime,
    /// Keepalive must be less than hold time when hold time is nonzero
    KeepaliveTooLarge,
    /// Invalid IPv4 address format
    InvalidIpv4Address,
    /// Invalid CIDR prefix length (must be 0..32)
    InvalidPrefixLength,
    /// Empty prefix list for UPDATE
    EmptyPrefixList,
};

/// Validate an AS number.
/// This ACT only supports 16-bit ASN (1..65535).
pub fn validateAsn(asn: u32) ValidationError!u16 {
    if (asn < 1 or asn > 65535) {
        return ValidationError.AsnOutOfRange;
    }
    return @as(u16, @intCast(asn));
}

/// Validate that an ASN is not a 32-bit ASN.
/// Returns error if asn > 65535.
pub fn validateAsnNot32Bit(asn: u32) ValidationError!void {
    if (asn > 65535) {
        return ValidationError.AsnTooLarge;
    }
}

/// Validate hold time (must be 0 or >= 3).
pub fn validateHoldTime(hold_time: u16) ValidationError!void {
    if (hold_time != 0 and hold_time < 3) {
        return ValidationError.InvalidHoldTime;
    }
}

/// Validate keepalive interval relative to hold time.
/// Keepalive must be less than hold time when hold time is nonzero.
pub fn validateKeepalive(keepalive: u16, hold_time: u16) ValidationError!void {
    if (hold_time != 0 and keepalive >= hold_time) {
        return ValidationError.KeepaliveTooLarge;
    }
}

/// Validate an IPv4 address (4 octets, each 0..255).
pub fn validateIpv4Address(addr: [4]u8) ValidationError!void {
    // All octets are u8, so they're already 0..255
    _ = addr;
}

/// Validate a prefix length (must be 0..32 for IPv4).
pub fn validatePrefixLength(len: u8) ValidationError!void {
    if (len > 32) {
        return ValidationError.InvalidPrefixLength;
    }
}

/// Validate that a prefix list is not empty (required for UPDATE).
pub fn validatePrefixList(prefixes_len: usize) ValidationError!void {
    if (prefixes_len == 0) {
        return ValidationError.EmptyPrefixList;
    }
}

/// Parse and validate an IPv4 CIDR string.
/// Returns the prefix (addr, len) on success.
pub fn parseAndValidateIpv4Cidr(cidr_str: []const u8) ValidationError!struct { addr: [4]u8, len: u8 } {
    const slash_idx = std.mem.indexOf(u8, cidr_str, "/") orelse return ValidationError.InvalidIpv4Address;

    // Parse address
    const addr_str = cidr_str[0..slash_idx];
    var addr: [4]u8 = .{ 0, 0, 0, 0 };
    var octet_idx: usize = 0;
    var pos: usize = 0;
    var octet_val: u32 = 0;

    while (pos <= addr_str.len) : (pos += 1) {
        const c = if (pos < addr_str.len) addr_str[pos] else '.';
        if (c == '.') {
            if (octet_idx >= 4) return ValidationError.InvalidIpv4Address;
            if (octet_val > 255) return ValidationError.InvalidIpv4Address;
            addr[octet_idx] = @as(u8, @intCast(octet_val));
            octet_idx += 1;
            octet_val = 0;
        } else if (c >= '0' and c <= '9') {
            octet_val = octet_val * 10 + (c - '0');
            if (octet_val > 255) return ValidationError.InvalidIpv4Address;
        } else {
            return ValidationError.InvalidIpv4Address;
        }
    }
    if (octet_idx != 4) return ValidationError.InvalidIpv4Address;

    // Parse prefix length
    const len_str = cidr_str[slash_idx + 1 ..];
    if (len_str.len == 0 or len_str.len > 2) return ValidationError.InvalidPrefixLength;
    var len: u8 = 0;
    for (len_str) |c| {
        if (c < '0' or c > '9') return ValidationError.InvalidPrefixLength;
        len = len * 10 + (c - '0');
    }
    if (len > 32) return ValidationError.InvalidPrefixLength;

    return .{ .addr = addr, .len = len };
}

// === Tests ===

test "validateAsn accepts 1..65535" {
    try std.testing.expectEqual(@as(u16, 1), try validateAsn(1));
    try std.testing.expectEqual(@as(u16, 65535), try validateAsn(65535));
    try std.testing.expectEqual(@as(u16, 65001), try validateAsn(65001));
}

test "validateAsn rejects 0 and >65535" {
    try std.testing.expectError(ValidationError.AsnOutOfRange, validateAsn(0));
    try std.testing.expectError(ValidationError.AsnOutOfRange, validateAsn(65536));
    try std.testing.expectError(ValidationError.AsnOutOfRange, validateAsn(100000));
}

test "validateAsnNot32Bit rejects >65535" {
    try validateAsnNot32Bit(65001);
    try std.testing.expectError(ValidationError.AsnTooLarge, validateAsnNot32Bit(65536));
    try std.testing.expectError(ValidationError.AsnTooLarge, validateAsnNot32Bit(4200000000));
}

test "validateHoldTime accepts 0 and >=3" {
    try validateHoldTime(0);
    try validateHoldTime(3);
    try validateHoldTime(180);
}

test "validateHoldTime rejects 1 and 2" {
    try std.testing.expectError(ValidationError.InvalidHoldTime, validateHoldTime(1));
    try std.testing.expectError(ValidationError.InvalidHoldTime, validateHoldTime(2));
}

test "validateKeepalive accepts valid keepalive" {
    try validateKeepalive(30, 90);
    try validateKeepalive(60, 180);
    try validateKeepalive(1, 3);
}

test "validateKeepalive rejects keepalive >= hold_time" {
    try std.testing.expectError(ValidationError.KeepaliveTooLarge, validateKeepalive(90, 90));
    try std.testing.expectError(ValidationError.KeepaliveTooLarge, validateKeepalive(100, 90));
}

test "validateKeepalive accepts 0 hold_time (no keepalive needed)" {
    try validateKeepalive(0, 0);
    try validateKeepalive(60, 0);
}

test "validatePrefixLength accepts 0..32" {
    try validatePrefixLength(0);
    try validatePrefixLength(16);
    try validatePrefixLength(32);
}

test "validatePrefixLength rejects >32" {
    try std.testing.expectError(ValidationError.InvalidPrefixLength, validatePrefixLength(33));
    try std.testing.expectError(ValidationError.InvalidPrefixLength, validatePrefixLength(128));
}

test "validatePrefixList accepts non-empty" {
    try validatePrefixList(1);
    try validatePrefixList(100);
}

test "validatePrefixList rejects empty" {
    try std.testing.expectError(ValidationError.EmptyPrefixList, validatePrefixList(0));
}

test "parseAndValidateIpv4Cidr accepts valid CIDR" {
    const result = try parseAndValidateIpv4Cidr("10.0.0.1/32");
    try std.testing.expect(result.addr[0] == 10);
    try std.testing.expect(result.addr[1] == 0);
    try std.testing.expect(result.addr[2] == 0);
    try std.testing.expect(result.addr[3] == 1);
    try std.testing.expect(result.len == 32);

    const result2 = try parseAndValidateIpv4Cidr("192.168.1.0/24");
    try std.testing.expect(result2.len == 24);
}

test "parseAndValidateIpv4Cidr rejects invalid input" {
    try std.testing.expectError(ValidationError.InvalidIpv4Address, parseAndValidateIpv4Cidr("10.0.0.1")); // no slash
    try std.testing.expectError(ValidationError.InvalidIpv4Address, parseAndValidateIpv4Cidr("256.0.0.1/32")); // invalid octet
    try std.testing.expectError(ValidationError.InvalidPrefixLength, parseAndValidateIpv4Cidr("10.0.0.1/33")); // invalid prefix
    try std.testing.expectError(ValidationError.InvalidPrefixLength, parseAndValidateIpv4Cidr("10.0.0.1/")); // empty prefix
}

