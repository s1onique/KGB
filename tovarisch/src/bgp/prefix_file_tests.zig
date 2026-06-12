// prefix_file_tests.zig — Tests for prefix_file.zig
//
// Tests for BIRD-style prefix-list parser with dual format support.

const std = @import("std");
const prefix_file = @import("prefix_file.zig");
const types = @import("types.zig");

// === Original tests ===

test "ignores blank lines" {
    const result = try prefix_file.parse("", std.testing.allocator);
    defer std.testing.allocator.free(result.prefixes);
    try std.testing.expectEqual(@as(usize, 0), result.prefixes.len);
    try std.testing.expectEqual(@as(usize, 0), result.skipped);
    
    const result2 = try prefix_file.parse("\n", std.testing.allocator);
    defer std.testing.allocator.free(result2.prefixes);
    try std.testing.expectEqual(@as(usize, 0), result2.prefixes.len);
    try std.testing.expectEqual(@as(usize, 1), result2.skipped);
}

test "ignores # comments" {
    const result = try prefix_file.parse("# This is a comment\n", std.testing.allocator);
    defer std.testing.allocator.free(result.prefixes);
    try std.testing.expectEqual(@as(usize, 0), result.prefixes.len);
    try std.testing.expectEqual(@as(usize, 1), result.skipped);
}

test "rejects invalid CIDR" {
    _ = prefix_file.parse("route invalid reject;\n", std.testing.allocator) catch |err| {
        try std.testing.expect(err == prefix_file.ParseError.InvalidCidr or err == prefix_file.ParseError.SyntaxError);
        return;
    };
    unreachable;
}

test "rejects IPv6" {
    _ = prefix_file.parse("route 2001:db8::/32 reject;\n", std.testing.allocator) catch |err| {
        try std.testing.expect(err == prefix_file.ParseError.Ipv6NotSupported);
        return;
    };
    unreachable;
}

test "rejects missing semicolon" {
    _ = prefix_file.parse("route 10.0.0.0/8 reject\n", std.testing.allocator) catch |err| {
        try std.testing.expect(err == prefix_file.ParseError.MissingSemicolon);
        return;
    };
    unreachable;
}

test "rejects via routes" {
    _ = prefix_file.parse("route 192.168.229.66/32 via 198.168.229.65 reject;\n", std.testing.allocator) catch |err| {
        try std.testing.expect(err == prefix_file.ParseError.UnsupportedRouteType or err == prefix_file.ParseError.SyntaxError);
        return;
    };
    unreachable;
}

test "rejects unknown BIRD directives" {
    _ = prefix_file.parse("protocol bgp Test { }\n", std.testing.allocator) catch |err| {
        try std.testing.expect(err == prefix_file.ParseError.InvalidCidr);
        return;
    };
    unreachable;
}

test "rejects quoted/injection-like input" {
    _ = prefix_file.parse("route 10.0.0.0/8 \"reject;\"\n", std.testing.allocator) catch |err| {
        try std.testing.expect(err == prefix_file.ParseError.PotentialInjection or err == prefix_file.ParseError.SyntaxError);
        return;
    };
    unreachable;
}

// === New tests for dual format support ===

test "accepts bare CIDR lines" {
    const result = try prefix_file.parse("10.149.149.0/24\n", std.testing.allocator);
    defer std.testing.allocator.free(result.prefixes);
    try std.testing.expectEqual(@as(usize, 1), result.prefixes.len);
    try std.testing.expectEqual(@as(u8, 24), result.prefixes[0].len);
}

test "accepts bare CIDR lines with whitespace" {
    const result = try prefix_file.parse("  192.168.0.0/16  \n", std.testing.allocator);
    defer std.testing.allocator.free(result.prefixes);
    try std.testing.expectEqual(@as(usize, 1), result.prefixes.len);
    try std.testing.expectEqual(@as(u8, 16), result.prefixes[0].len);
}

test "accepts BIRD route with blackhole" {
    const result = try prefix_file.parse("route 23.192.0.0/11 blackhole;\n", std.testing.allocator);
    defer std.testing.allocator.free(result.prefixes);
    try std.testing.expectEqual(@as(usize, 1), result.prefixes.len);
    try std.testing.expectEqual(@as(u8, 11), result.prefixes[0].len);
}

test "accepts BIRD route with reject" {
    const result = try prefix_file.parse("route 64.233.160.0/19 reject;\n", std.testing.allocator);
    defer std.testing.allocator.free(result.prefixes);
    try std.testing.expectEqual(@as(usize, 1), result.prefixes.len);
    try std.testing.expectEqual(@as(u8, 19), result.prefixes[0].len);
}

test "accepts BIRD route with extra whitespace before action" {
    const result = try prefix_file.parse("route 23.192.0.0/11   reject;\n", std.testing.allocator);
    defer std.testing.allocator.free(result.prefixes);
    try std.testing.expectEqual(@as(usize, 1), result.prefixes.len);
    try std.testing.expectEqual(@as(u8, 11), result.prefixes[0].len);
}

test "accepts BIRD route with space before semicolon" {
    const result = try prefix_file.parse("route 23.192.0.0/11 reject ;\n", std.testing.allocator);
    defer std.testing.allocator.free(result.prefixes);
    try std.testing.expectEqual(@as(usize, 1), result.prefixes.len);
    try std.testing.expectEqual(@as(u8, 11), result.prefixes[0].len);
}

test "accepts BIRD route with blackhole and space before semicolon" {
    const result = try prefix_file.parse("route 23.192.0.0/11 blackhole ;\n", std.testing.allocator);
    defer std.testing.allocator.free(result.prefixes);
    try std.testing.expectEqual(@as(usize, 1), result.prefixes.len);
    try std.testing.expectEqual(@as(u8, 11), result.prefixes[0].len);
}

test "rejects BIRD route with extra tokens" {
    _ = prefix_file.parse("route 23.192.0.0/11 reject extra;\n", std.testing.allocator) catch |err| {
        try std.testing.expect(err == prefix_file.ParseError.SyntaxError);
        return;
    };
    unreachable;
}

test "rejects BIRD route missing action" {
    _ = prefix_file.parse("route 23.192.0.0/11\n", std.testing.allocator) catch |err| {
        try std.testing.expect(err == prefix_file.ParseError.MissingSemicolon);
        return;
    };
    unreachable;
}

test "accepts mixed BIRD and bare CIDR formats" {
    const content = 
        \\route 23.192.0.0/11 reject;
        \\route 64.233.160.0/19 blackhole;
        \\10.149.149.0/24
        \\# This is a comment
        \\
        \\192.168.0.0/16
    ;
    const result = try prefix_file.parse(content, std.testing.allocator);
    defer std.testing.allocator.free(result.prefixes);
    try std.testing.expectEqual(@as(usize, 4), result.prefixes.len);
    try std.testing.expectEqual(@as(u8, 11), result.prefixes[0].len);
    try std.testing.expectEqual(@as(u8, 19), result.prefixes[1].len);
    try std.testing.expectEqual(@as(u8, 24), result.prefixes[2].len);
    try std.testing.expectEqual(@as(u8, 16), result.prefixes[3].len);
    try std.testing.expect(result.skipped >= 1);
}

test "accepts duplicate prefixes (no deduplication)" {
    const content = 
        \\10.0.0.0/8
        \\10.0.0.0/8
        \\route 10.0.0.0/8 reject;
    ;
    const result = try prefix_file.parse(content, std.testing.allocator);
    defer std.testing.allocator.free(result.prefixes);
    try std.testing.expectEqual(@as(usize, 3), result.prefixes.len);
}

test "rejects bare CIDR with invalid format" {
    _ = prefix_file.parse("invalid-cidr\n", std.testing.allocator) catch |err| {
        try std.testing.expect(err == prefix_file.ParseError.InvalidCidr);
        return;
    };
    unreachable;
}

test "rejects bare CIDR with injection attempt" {
    _ = prefix_file.parse("10.0.0.0/8 \"extra\"\n", std.testing.allocator) catch |err| {
        try std.testing.expect(err == prefix_file.ParseError.PotentialInjection);
        return;
    };
    unreachable;
}

test "rejects bare CIDR with via-like content" {
    _ = prefix_file.parse("via 10.0.0.0/8\n", std.testing.allocator) catch |err| {
        try std.testing.expect(err == prefix_file.ParseError.InvalidCidr);
        return;
    };
    unreachable;
}
