// prefix_file.zig — BIRD-style prefix-list parser
//
// ACT 1: Pure parsing only, no sockets or runtime behavior.
// Supports BIRD static route-list format:
//   route <IPv4 CIDR> reject;
// Blank lines and # comments are ignored.
// All other syntax is rejected (conservative parsing for security).

const std = @import("std");
const types = @import("types.zig");

/// Parse errors for prefix-list files
pub const ParseError = error{
    /// Line contains syntax that cannot be parsed
    SyntaxError,
    /// CIDR format is invalid
    InvalidCidr,
    /// IPv6 prefix found (not supported in this ACT)
    Ipv6NotSupported,
    /// Line missing required semicolon
    MissingSemicolon,
    /// Route type is not 'reject' (e.g., 'via')
    UnsupportedRouteType,
    /// Unknown BIRD directive
    UnknownDirective,
    /// Quoted string or potential injection detected
    PotentialInjection,
    /// IPv4 octet out of range (0..255)
    InvalidOctet,
    /// Prefix length out of range (0..32)
    InvalidPrefixLength,
};

/// Result of parsing a single prefix file
pub const ParseResult = struct {
    /// Parsed prefixes in order
    prefixes: []types.Ipv4Prefix,
    /// Number of lines that were skipped (blank or comment)
    skipped: usize,
    /// Number of lines that had parse errors
    errors: usize,
};

/// Skip leading whitespace and return remaining slice.
fn skipWhitespace(s: []const u8) []const u8 {
    var i: usize = 0;
    while (i < s.len and (s[i] == ' ' or s[i] == '\t')) : (i += 1) {
        // skip
    }
    return s[i..];
}

/// Check if line is a blank line (empty or whitespace only).
fn isBlankLine(line: []const u8) bool {
    return skipWhitespace(line).len == 0;
}

/// Check if line is a comment (starts with # after whitespace).
fn isComment(line: []const u8) bool {
    const trimmed = skipWhitespace(line);
    return trimmed.len > 0 and trimmed[0] == '#';
}

/// Parse a single route line and extract the CIDR.
/// Returns the CIDR string (without leading/trailing whitespace).
/// Returns error on any unexpected syntax.
fn extractCidrFromRouteLine(line: []const u8) ParseError![]const u8 {
    // Must start with "route" (case-sensitive)
    const trimmed = skipWhitespace(line);
    if (trimmed.len < 6) return ParseError.SyntaxError;
    if (!std.mem.eql(u8, trimmed[0..5], "route")) {
        return ParseError.UnknownDirective;
    }

    // Find the end of the CIDR (look for "reject;" pattern)
    // Look for "reject" keyword
    var reject_pos: usize = 0;
    for (trimmed, 0..) |c, i| {
        if (c == 'r' and i + 6 <= trimmed.len) {
            if (std.mem.eql(u8, trimmed[i..i+6], "reject")) {
                reject_pos = i;
                break;
            }
        }
    }

    if (reject_pos == 0) return ParseError.MissingSemicolon;

    // Check for "via" between "route" and "reject" - indicates non-reject route type
    const between_route_and_reject = trimmed[5..reject_pos];
    if (std.mem.indexOf(u8, between_route_and_reject, "via") != null) {
        return ParseError.UnsupportedRouteType;
    }

    // After "reject", we need semicolon
    const after_reject = trimmed[reject_pos + 6 ..];
    // Skip whitespace and look for semicolon
    var found_semicolon = false;
    for (after_reject) |c| {
        if (c == ';') {
            found_semicolon = true;
            break;
        }
        if (c != ' ' and c != '\t') {
            return ParseError.SyntaxError;
        }
    }
    if (!found_semicolon) return ParseError.MissingSemicolon;

    // Extract CIDR - it's everything between "route " and "reject"
    const after_route = trimmed[5..reject_pos];
    const cidr = skipWhitespace(after_route);

    if (cidr.len == 0) return ParseError.SyntaxError;

    // Check for injection attempts
    for (cidr) |c| {
        if (c == '"' or c == '\'') return ParseError.PotentialInjection;
    }

    // Verify CIDR contains slash
    if (std.mem.indexOf(u8, cidr, "/") == null) return ParseError.InvalidCidr;

    return cidr;
}

/// Parse a CIDR string into an Ipv4Prefix.
/// Returns error on invalid format or IPv6.
fn parseCidr(cidr: []const u8) ParseError!types.Ipv4Prefix {
    // Reject IPv6 indicators
    if (std.mem.indexOf(u8, cidr, ":") != null) {
        return ParseError.Ipv6NotSupported;
    }

    const slash_idx = std.mem.indexOf(u8, cidr, "/") orelse return ParseError.InvalidCidr;
    if (slash_idx == 0) return ParseError.InvalidCidr;

    // Parse address part
    const addr_str = cidr[0..slash_idx];
    var addr: [4]u8 = .{ 0, 0, 0, 0 };
    var octet_idx: usize = 0;
    var pos: usize = 0;
    var octet_val: u32 = 0;

    while (pos <= addr_str.len) : (pos += 1) {
        const c = if (pos < addr_str.len) addr_str[pos] else '.';
        if (c == '.') {
            if (octet_idx >= 4) return ParseError.InvalidCidr;
            if (octet_val > 255) return ParseError.InvalidOctet;
            addr[octet_idx] = @as(u8, @intCast(octet_val));
            octet_idx += 1;
            octet_val = 0;
        } else if (c >= '0' and c <= '9') {
            octet_val = octet_val * 10 + (c - '0');
            if (octet_val > 255) return ParseError.InvalidOctet;
        } else {
            return ParseError.InvalidCidr;
        }
    }
    if (octet_idx != 4) return ParseError.InvalidCidr;

    // Parse prefix length
    const len_str = cidr[slash_idx + 1 ..];
    if (len_str.len == 0 or len_str.len > 2) return ParseError.InvalidPrefixLength;
    var len: u8 = 0;
    for (len_str) |c| {
        if (c < '0' or c > '9') return ParseError.InvalidPrefixLength;
        len = len * 10 + (c - '0');
    }
    if (len > 32) return ParseError.InvalidPrefixLength;

    return types.Ipv4Prefix{ .addr = addr, .len = len };
}

/// Parse a prefix-list file content.
/// Returns a ParseResult with all parsed prefixes.
/// FAIL-CLOSED: Returns error on any malformed line (blank/comment lines are OK).
/// The result's prefixes slice is allocated from the provided allocator.
pub fn parse(content: []const u8, allocator: std.mem.Allocator) anyerror!ParseResult {
    var prefixes = std.ArrayList(types.Ipv4Prefix).empty;
    errdefer prefixes.deinit(allocator);

    var skipped: usize = 0;

    var pos: usize = 0;
    while (pos < content.len) {
        // Find line end (LF or CR)
        var line_end = pos;
        while (line_end < content.len and content[line_end] != '\n' and content[line_end] != '\r') {
            line_end += 1;
        }

        // Extract line (without trailing CR/LF)
        const line = content[pos..line_end];

        if (isBlankLine(line)) {
            skipped += 1;
        } else if (isComment(line)) {
            skipped += 1;
        } else {
            // Try to parse as route line - FAIL on any error
            const cidr = try extractCidrFromRouteLine(line);
            const prefix = try parseCidr(cidr);
            try prefixes.append(allocator, prefix);
        }

        // Move to next line (skip any CR/LF characters)
        pos = line_end;
        while (pos < content.len and (content[pos] == '\r' or content[pos] == '\n')) pos += 1;
    }

    const owned = try prefixes.toOwnedSlice(allocator);
    return ParseResult{
        .prefixes = owned,
        .skipped = skipped,
        .errors = 0,
    };
}

// === Tests ===

test "ignores blank lines" {
    // Empty string has no lines, so skipped = 0
    const result = try parse("", std.testing.allocator);
    defer std.testing.allocator.free(result.prefixes);
    try std.testing.expectEqual(@as(usize, 0), result.prefixes.len);
    try std.testing.expectEqual(@as(usize, 0), result.skipped);
    
    // String with just a newline is a blank line
    const result2 = try parse("\n", std.testing.allocator);
    defer std.testing.allocator.free(result2.prefixes);
    try std.testing.expectEqual(@as(usize, 0), result2.prefixes.len);
    try std.testing.expectEqual(@as(usize, 1), result2.skipped);
}

test "ignores # comments" {
    const result = try parse("# This is a comment\n", std.testing.allocator);
    defer std.testing.allocator.free(result.prefixes);
    try std.testing.expectEqual(@as(usize, 0), result.prefixes.len);
    try std.testing.expectEqual(@as(usize, 1), result.skipped);
}

test "rejects invalid CIDR" {
    _ = parse("route invalid reject;\n", std.testing.allocator) catch |err| {
        try std.testing.expect(err == ParseError.InvalidCidr or err == ParseError.SyntaxError);
        return;
    };
    unreachable; // should have errored
}

test "rejects IPv6" {
    _ = parse("route 2001:db8::/32 reject;\n", std.testing.allocator) catch |err| {
        try std.testing.expect(err == ParseError.Ipv6NotSupported);
        return;
    };
    unreachable; // should have errored
}

test "rejects missing semicolon" {
    _ = parse("route 10.0.0.0/8 reject\n", std.testing.allocator) catch |err| {
        try std.testing.expect(err == ParseError.MissingSemicolon);
        return;
    };
    unreachable; // should have errored
}

test "rejects via routes" {
    _ = parse("route 192.168.229.66/32 via 198.168.229.65 reject;\n", std.testing.allocator) catch |err| {
        try std.testing.expect(err == ParseError.UnsupportedRouteType or err == ParseError.UnknownDirective);
        return;
    };
    unreachable; // should have errored
}

test "rejects unknown BIRD directives" {
    _ = parse("protocol bgp Test { }\n", std.testing.allocator) catch |err| {
        try std.testing.expect(err == ParseError.UnknownDirective);
        return;
    };
    unreachable; // should have errored
}

test "rejects quoted/injection-like input" {
    _ = parse("route 10.0.0.0/8 \"reject;\"\n", std.testing.allocator) catch |err| {
        try std.testing.expect(err == ParseError.PotentialInjection or err == ParseError.SyntaxError);
        return;
    };
    unreachable; // should have errored
}
