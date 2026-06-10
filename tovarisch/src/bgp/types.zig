// types.zig — BGP protocol types and constants
//
// ACT 1: Pure encoding/parsing only, no sockets or runtime behavior.
// References: RFC 4271 (BGP-4), RFC 1997 (AS_PATH), RFC 2918 (ORIGIN)

const std = @import("std");

/// BGP protocol version
pub const PROTOCOL_VERSION: u8 = 4;

/// BGP message type codes (RFC 4271 Section 4.1)
pub const MessageType = enum(u8) {
    open = 1,
    update = 2,
    notification = 3,
    keepalive = 4,
};

/// BGP marker size (16 bytes of 0xFF)
pub const MARKER_SIZE: usize = 16;

/// BGP minimum message length (19 bytes for KEEPALIVE)
pub const MIN_MESSAGE_LENGTH: usize = 19;

/// BGP maximum message length (4KB for this ACT)
pub const MAX_MESSAGE_LENGTH: usize = 4096;

/// BGP port (not used in this ACT, but documented for future)
pub const BGP_PORT: u16 = 179;

/// ORIGIN attribute values (RFC 4271 Section 5.1.1)
pub const OriginType = enum(u8) {
    igp = 0,
    egp = 1,
    incomplete = 2,
};

/// AS_PATH path segment types (RFC 4271 Section 5.1.2)
pub const AsPathSegmentType = enum(u8) {
    as_set = 1,
    as_sequence = 2,
};

/// BGP attribute flags (RFC 4271 Section 4.3)
pub const AttributeFlags = struct {
    optional: u1 = 0,
    transitive: u1 = 1,
    partial: u1 = 0,
    extended_length: u1 = 0,
};

/// Parse error for Ipv4Prefix
pub const PrefixParseError = error{
    /// CIDR string is malformed
    InvalidFormat,
    /// IPv4 address has invalid octet value (>255)
    InvalidOctet,
    /// Prefix length out of range (must be 0..32)
    InvalidPrefixLength,
};

/// IPv4 prefix representation
pub const Ipv4Prefix = struct {
    /// 4-byte network address (network order)
    addr: [4]u8,
    /// Prefix length in bits (0..32)
    len: u8,

    /// Parse a CIDR string with full validation.
    /// Returns error on malformed input.
    pub fn parse(cidr_str: []const u8) PrefixParseError!Ipv4Prefix {
        const slash_idx = std.mem.indexOf(u8, cidr_str, "/") orelse return PrefixParseError.InvalidFormat;
        if (slash_idx == 0) return PrefixParseError.InvalidFormat;

        const addr_str = cidr_str[0..slash_idx];
        const len_str = cidr_str[slash_idx + 1 ..];

        // Parse IPv4 address
        var addr: [4]u8 = .{ 0, 0, 0, 0 };
        var octet_idx: usize = 0;
        var pos: usize = 0;
        var octet_val: u32 = 0;

        while (pos <= addr_str.len) : (pos += 1) {
            const c = if (pos < addr_str.len) addr_str[pos] else '.';
            if (c == '.') {
                if (octet_idx >= 4) return PrefixParseError.InvalidFormat;
                if (octet_val > 255) return PrefixParseError.InvalidOctet;
                addr[octet_idx] = @as(u8, @intCast(octet_val));
                octet_idx += 1;
                octet_val = 0;
            } else if (c >= '0' and c <= '9') {
                octet_val = octet_val * 10 + (c - '0');
                if (octet_val > 255) return PrefixParseError.InvalidOctet;
            } else {
                return PrefixParseError.InvalidFormat;
            }
        }
        if (octet_idx != 4) return PrefixParseError.InvalidFormat;

        // Parse prefix length
        if (len_str.len == 0 or len_str.len > 2) return PrefixParseError.InvalidPrefixLength;
        var len: u8 = 0;
        for (len_str) |c| {
            if (c < '0' or c > '9') return PrefixParseError.InvalidPrefixLength;
            len = len * 10 + (c - '0');
        }
        if (len > 32) return PrefixParseError.InvalidPrefixLength;

        return Ipv4Prefix{ .addr = addr, .len = len };
    }

    /// Unsafe initializer for test/known-good input only.
    /// Do NOT use on untrusted input - use parse() instead.
    pub fn init(cidr_str: []const u8) Ipv4Prefix {
        return parse(cidr_str) catch undefined;
    }

    /// Number of bytes needed to encode this prefix in NLRI
    pub fn nlriByteCount(self: Ipv4Prefix) usize {
        if (self.len == 0) return 0;
        return (self.len + 7) / 8;
    }
};

/// BGP OPEN message parameters
pub const OpenParams = struct {
    /// My autonomous system number (1..65535 for this ACT)
    my_as: u16,
    /// Hold time in seconds (0 or >= 3)
    hold_time: u16,
    /// BGP router ID (4-byte IPv4 address)
    router_id: [4]u8,
};

/// BGP UPDATE message parameters
pub const UpdateParams = struct {
    /// Local/source IPv4 address (used for NEXT_HOP)
    next_hop: [4]u8,
    /// Local AS number (used in AS_PATH)
    local_as: u16,
    /// If true, AS_PATH is empty (same-AS/iBGP style)
    /// If false, AS_PATH contains local_as (different-AS/eBGP style)
    same_as: bool,
    /// Prefixes to advertise
    prefixes: []const Ipv4Prefix,
};

test "Ipv4Prefix init and nlriByteCount" {
    const prefix = Ipv4Prefix.init("23.192.0.0/11");
    try std.testing.expect(prefix.len == 11);
    try std.testing.expect(prefix.nlriByteCount() == 2);
}
