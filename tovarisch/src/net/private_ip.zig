// private_ip.zig — Pure IPv4 address classification
//
// Classifies IPv4 text addresses into privacy/reserved categories.
// No allocator, no syscalls, purely deterministic.

const std = @import("std");

pub const IpClass = enum {
    private,
    carrier_nat,
    loopback,
    link_local,
    documentation,
    multicast,
    public,
    invalid,
};

/// Classify an IPv4 address from text representation.
/// Returns the appropriate IpClass based on RFC 5735/6598/6890 ranges.
///
/// Syntactically malformed input returns .invalid.
/// All other valid addresses return .public.
pub fn classifyIpv4Text(input: []const u8) IpClass {
    const octets = parseIpv4Octets(input) orelse return .invalid;
    return classifyIpv4Octets(octets);
}

/// Classify exactly four already-parsed IPv4 octets.
pub fn classifyIpv4Octets(octets: [4]u8) IpClass {
    const first = octets[0];
    const second = octets[1];

    // 10.0.0.0/8 — private
    if (first == 10) return .private;

    // 172.16.0.0/12 — private
    if (first == 172 and second >= 16 and second <= 31) return .private;

    // 192.168.0.0/16 — private
    if (first == 192 and second == 168) return .private;

    // 100.64.0.0/10 — carrier-grade NAT
    if (first == 100 and second >= 64 and second <= 127) return .carrier_nat;

    // 127.0.0.0/8 — loopback
    if (first == 127) return .loopback;

    // 169.254.0.0/16 — link-local
    if (first == 169 and second == 254) return .link_local;

    // 192.0.2.0/24 — documentation (TEST-NET-1)
    if (first == 192 and second == 0 and octets[2] == 2) return .documentation;

    // 198.51.100.0/24 — documentation (TEST-NET-2)
    if (first == 198 and second == 51 and octets[2] == 100) return .documentation;

    // 203.0.113.0/24 — documentation (TEST-NET-3)
    if (first == 203 and second == 0 and octets[2] == 113) return .documentation;

    // 224.0.0.0/4 — multicast
    if (first >= 224) return .multicast;

    // Everything else is public
    return .public;
}

/// Parse IPv4 text into 4 octets. Returns null on malformed input.
/// Expected format: "a.b.c.d" where each component is 0-255.
/// Does NOT allocate; parses in-place.
fn parseIpv4Octets(input: []const u8) ?[4]u8 {
    if (input.len == 0) return null;

    var octets: [4]u8 = undefined;
    var octet_idx: usize = 0;
    var pos: usize = 0;

    // State machine: reading digits for current octet
    var value: u32 = 0;
    var digits_seen: usize = 0;

    while (pos < input.len) {
        const c = input[pos];

        if (c == '.') {
            // End of current octet
            if (digits_seen == 0) return null; // empty octet before dot
            if (octet_idx >= 4) return null; // too many octets
            if (value > 255) return null; // octet overflow
            octets[octet_idx] = @intCast(value);
            octet_idx += 1;
            value = 0;
            digits_seen = 0;
            pos += 1;
            continue;
        }

        // Expect digit
        if (c < '0' or c > '9') return null;
        digits_seen += 1;
        if (digits_seen > 3) return null; // prevent overflow
        value = value * 10 + @as(u32, c - '0');
        pos += 1;
    }

    // Process final octet (no trailing dot)
    if (digits_seen == 0) return null; // empty final octet or empty string
    if (octet_idx >= 4) return null; // too many octets
    if (value > 255) return null; // octet overflow
    octets[octet_idx] = @intCast(value);
    octet_idx += 1;

    // Must have exactly 4 octets
    if (octet_idx != 4) return null;

    return octets;
}

test "classifyIpv4Text: private ranges" {
    try std.testing.expectEqual(IpClass.private, classifyIpv4Text("10.0.0.0"));
    try std.testing.expectEqual(IpClass.private, classifyIpv4Text("10.255.255.255"));
    try std.testing.expectEqual(IpClass.private, classifyIpv4Text("10.42.123.99"));

    try std.testing.expectEqual(IpClass.private, classifyIpv4Text("172.16.0.0"));
    try std.testing.expectEqual(IpClass.private, classifyIpv4Text("172.31.255.255"));
    try std.testing.expectEqual(IpClass.private, classifyIpv4Text("172.20.50.100"));

    try std.testing.expectEqual(IpClass.private, classifyIpv4Text("192.168.0.0"));
    try std.testing.expectEqual(IpClass.private, classifyIpv4Text("192.168.255.255"));
    try std.testing.expectEqual(IpClass.private, classifyIpv4Text("192.168.1.1"));
}

test "classifyIpv4Text: carrier NAT range" {
    try std.testing.expectEqual(IpClass.carrier_nat, classifyIpv4Text("100.64.0.0"));
    try std.testing.expectEqual(IpClass.carrier_nat, classifyIpv4Text("100.127.255.255"));
    try std.testing.expectEqual(IpClass.carrier_nat, classifyIpv4Text("100.100.100.100"));
}

test "classifyIpv4Text: loopback range" {
    try std.testing.expectEqual(IpClass.loopback, classifyIpv4Text("127.0.0.0"));
    try std.testing.expectEqual(IpClass.loopback, classifyIpv4Text("127.255.255.255"));
    try std.testing.expectEqual(IpClass.loopback, classifyIpv4Text("127.0.0.1"));
}

test "classifyIpv4Text: link-local range" {
    try std.testing.expectEqual(IpClass.link_local, classifyIpv4Text("169.254.0.0"));
    try std.testing.expectEqual(IpClass.link_local, classifyIpv4Text("169.254.255.255"));
    try std.testing.expectEqual(IpClass.link_local, classifyIpv4Text("169.254.1.2"));
}

test "classifyIpv4Text: documentation ranges" {
    try std.testing.expectEqual(IpClass.documentation, classifyIpv4Text("192.0.2.0"));
    try std.testing.expectEqual(IpClass.documentation, classifyIpv4Text("192.0.2.255"));
    try std.testing.expectEqual(IpClass.documentation, classifyIpv4Text("198.51.100.0"));
    try std.testing.expectEqual(IpClass.documentation, classifyIpv4Text("198.51.100.255"));
    try std.testing.expectEqual(IpClass.documentation, classifyIpv4Text("203.0.113.0"));
    try std.testing.expectEqual(IpClass.documentation, classifyIpv4Text("203.0.113.255"));
}

test "classifyIpv4Text: multicast range" {
    try std.testing.expectEqual(IpClass.multicast, classifyIpv4Text("224.0.0.0"));
    try std.testing.expectEqual(IpClass.multicast, classifyIpv4Text("224.0.0.1"));
    try std.testing.expectEqual(IpClass.multicast, classifyIpv4Text("239.255.255.255"));
    try std.testing.expectEqual(IpClass.multicast, classifyIpv4Text("232.0.0.1"));
}

test "classifyIpv4Text: public ranges" {
    try std.testing.expectEqual(IpClass.public, classifyIpv4Text("8.8.8.8"));
    try std.testing.expectEqual(IpClass.public, classifyIpv4Text("1.1.1.1"));
    try std.testing.expectEqual(IpClass.public, classifyIpv4Text("93.184.216.34"));
    try std.testing.expectEqual(IpClass.public, classifyIpv4Text("223.255.255.255"));
}

test "classifyIpv4Text: invalid inputs" {
    // Empty
    try std.testing.expectEqual(IpClass.invalid, classifyIpv4Text(""));

    // Missing octets
    try std.testing.expectEqual(IpClass.invalid, classifyIpv4Text("1"));
    try std.testing.expectEqual(IpClass.invalid, classifyIpv4Text("1.2"));
    try std.testing.expectEqual(IpClass.invalid, classifyIpv4Text("1.2.3"));

    // Too many octets
    try std.testing.expectEqual(IpClass.invalid, classifyIpv4Text("1.2.3.4.5"));

    // Non-numeric
    try std.testing.expectEqual(IpClass.invalid, classifyIpv4Text("abc"));
    try std.testing.expectEqual(IpClass.invalid, classifyIpv4Text("1.abc.3.4"));
    try std.testing.expectEqual(IpClass.invalid, classifyIpv4Text("1.2.3.abc"));
    try std.testing.expectEqual(IpClass.invalid, classifyIpv4Text("1.2.3.4."));
    try std.testing.expectEqual(IpClass.invalid, classifyIpv4Text(".1.2.3.4"));

    // Octet overflow
    try std.testing.expectEqual(IpClass.invalid, classifyIpv4Text("256.0.0.0"));
    try std.testing.expectEqual(IpClass.invalid, classifyIpv4Text("0.256.0.0"));
    try std.testing.expectEqual(IpClass.invalid, classifyIpv4Text("0.0.256.0"));
    try std.testing.expectEqual(IpClass.invalid, classifyIpv4Text("0.0.0.256"));
    try std.testing.expectEqual(IpClass.invalid, classifyIpv4Text("999.999.999.999"));
    try std.testing.expectEqual(IpClass.invalid, classifyIpv4Text("1.2.3.999"));

    // Leading zeros edge case (should be valid as per std)
    try std.testing.expectEqual(IpClass.public, classifyIpv4Text("001.0.0.1"));
}

test "classifyIpv4Octets: boundary edges" {
    // Exact boundaries for private ranges
    try std.testing.expectEqual(IpClass.private, classifyIpv4Octets(.{ 10, 0, 0, 0 }));
    try std.testing.expectEqual(IpClass.private, classifyIpv4Octets(.{ 10, 255, 255, 255 }));
    try std.testing.expectEqual(IpClass.private, classifyIpv4Octets(.{ 172, 16, 0, 0 }));
    try std.testing.expectEqual(IpClass.private, classifyIpv4Octets(.{ 172, 31, 255, 255 }));
    try std.testing.expectEqual(IpClass.private, classifyIpv4Octets(.{ 192, 168, 0, 0 }));
    try std.testing.expectEqual(IpClass.private, classifyIpv4Octets(.{ 192, 168, 255, 255 }));

    // Carrier NAT boundaries
    try std.testing.expectEqual(IpClass.carrier_nat, classifyIpv4Octets(.{ 100, 64, 0, 0 }));
    try std.testing.expectEqual(IpClass.carrier_nat, classifyIpv4Octets(.{ 100, 127, 255, 255 }));
    try std.testing.expectEqual(IpClass.public, classifyIpv4Octets(.{ 100, 63, 255, 255 }));
    try std.testing.expectEqual(IpClass.public, classifyIpv4Octets(.{ 100, 128, 0, 0 }));

    // Loopback boundaries
    try std.testing.expectEqual(IpClass.loopback, classifyIpv4Octets(.{ 127, 0, 0, 0 }));
    try std.testing.expectEqual(IpClass.loopback, classifyIpv4Octets(.{ 127, 255, 255, 255 }));

    // Link-local boundaries
    try std.testing.expectEqual(IpClass.link_local, classifyIpv4Octets(.{ 169, 254, 0, 0 }));
    try std.testing.expectEqual(IpClass.link_local, classifyIpv4Octets(.{ 169, 254, 255, 255 }));

    // Multicast boundaries
    try std.testing.expectEqual(IpClass.multicast, classifyIpv4Octets(.{ 224, 0, 0, 0 }));
    try std.testing.expectEqual(IpClass.public, classifyIpv4Octets(.{ 223, 255, 255, 255 }));

    // Public just outside reserved ranges
    try std.testing.expectEqual(IpClass.public, classifyIpv4Octets(.{ 9, 255, 255, 255 }));
    try std.testing.expectEqual(IpClass.public, classifyIpv4Octets(.{ 11, 0, 0, 0 }));
    try std.testing.expectEqual(IpClass.public, classifyIpv4Octets(.{ 172, 15, 255, 255 }));
    try std.testing.expectEqual(IpClass.public, classifyIpv4Octets(.{ 172, 32, 0, 0 }));
}

