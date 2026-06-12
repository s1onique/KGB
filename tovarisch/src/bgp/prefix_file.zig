// prefix_file.zig — BIRD-style prefix-list parser
//
// Supports both BIRD static route-list format and bare CIDR lines:
//   route <IPv4 CIDR> reject;
//   route <IPv4 CIDR> blackhole;
//   10.149.149.0/24
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
    /// Memory allocation failed
    OutOfMemory,
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

/// Trim trailing whitespace from a slice.
fn trimTrailingWhitespace(s: []const u8) []const u8 {
    var end = s.len;
    while (end > 0 and (s[end - 1] == ' ' or s[end - 1] == '\t')) {
        end -= 1;
    }
    return s[0..end];
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
/// Supports both BIRD static route syntax and bare CIDR lines.
fn extractCidrFromRouteLine(line: []const u8) ParseError![]const u8 {
    const trimmed = skipWhitespace(line);
    if (trimmed.len == 0) return ParseError.SyntaxError;

    // Check if this is a BIRD route line (starts with "route")
    if (trimmed.len >= 5 and std.mem.eql(u8, trimmed[0..5], "route")) {
        return extractCidrFromBirdRouteLine(trimmed);
    }

    // Otherwise, treat as bare CIDR line
    return extractCidrFromBareLine(trimmed);
}

/// Extract CIDR from a BIRD static route line: "route <CIDR> <action>;"
/// Supported actions: "reject", "blackhole"
/// No allocation - uses std.mem.tokenizeScalar for pure parsing.
fn extractCidrFromBirdRouteLine(trimmed: []const u8) ParseError![]const u8 {
    // Tokenize by whitespace using std.mem.tokenizeScalar (no allocation)
    var tokenizer = std.mem.tokenizeScalar(u8, trimmed, ' ');
    var token_count: usize = 0;
    var cidr: []const u8 = undefined;
    var action_token: []const u8 = undefined;
    var has_semicolon = false;

    while (tokenizer.next()) |token| {
        token_count += 1;
        if (token_count == 1) {
            // token[0] must be "route"
            if (!std.mem.eql(u8, token, "route")) {
                return ParseError.SyntaxError;
            }
        } else if (token_count == 2) {
            // token[1] is CIDR
            cidr = token;
            // Check for injection attempts in CIDR
            for (cidr) |c| {
                if (c == '"' or c == '\'') return ParseError.PotentialInjection;
            }
        } else if (token_count == 3) {
            // token[2] is action (reject or blackhole, possibly with trailing semicolon)
            action_token = token;
            // Check for trailing semicolon
            if (action_token.len > 0 and action_token[action_token.len - 1] == ';') {
                has_semicolon = true;
            }
        } else if (token_count == 4) {
            // token[3] - must be semicolon if attached semicolon not present
            if (!has_semicolon and !std.mem.eql(u8, token, ";")) {
                return ParseError.SyntaxError;
            }
        } else {
            // More than 4 tokens - extra tokens not allowed
            return ParseError.SyntaxError;
        }
    }

    // Must have at least 3 tokens (route, CIDR, action)
    if (token_count < 3) {
        return ParseError.MissingSemicolon;
    }

    // Normalize action: remove trailing semicolon if present
    var action = action_token;
    if (action.len > 0 and action[action.len - 1] == ';') {
        action = action[0 .. action.len - 1];
    }

    // Require action is "reject" or "blackhole"
    if (!std.mem.eql(u8, action, "reject") and !std.mem.eql(u8, action, "blackhole")) {
        // Check for "via" which is unsupported
        if (std.mem.indexOf(u8, action_token, "via") != null) {
            return ParseError.UnsupportedRouteType;
        }
        return ParseError.SyntaxError;
    }

    // Validate token count based on semicolon position
    if (has_semicolon) {
        // Attached semicolon: must have exactly 3 tokens
        if (token_count != 3) {
            return ParseError.SyntaxError;
        }
    } else {
        // Separate semicolon: must have exactly 4 tokens (route, CIDR, action, ;)
        if (token_count != 4) {
            return ParseError.MissingSemicolon;
        }
    }

    // Verify CIDR contains slash
    if (std.mem.indexOf(u8, cidr, "/") == null) return ParseError.InvalidCidr;

    return cidr;
}

/// Extract CIDR from a bare CIDR line (e.g., "10.149.149.0/24")
fn extractCidrFromBareLine(trimmed: []const u8) ParseError![]const u8 {
    // Trim trailing whitespace
    const cidr = trimTrailingWhitespace(trimmed);

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
