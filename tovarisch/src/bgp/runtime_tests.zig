// bgp/runtime_tests.zig — Tests for bgp/runtime.zig functions
//
// Tests for formatPeerAddr and related utilities.

const std = @import("std");
const runtime = @import("runtime.zig");

test "formatPeerAddr formats valid IPv4 address" {
    var buf: [32]u8 = undefined;
    const addr = [4]u8{ 192, 168, 1, 1 };

    const result = try runtime.formatPeerAddr(addr, &buf);

    try std.testing.expectEqualStrings("192.168.1.1", result);
}

test "formatPeerAddr formats all-zeros address" {
    var buf: [32]u8 = undefined;
    const addr = [4]u8{ 0, 0, 0, 0 };

    const result = try runtime.formatPeerAddr(addr, &buf);

    try std.testing.expectEqualStrings("0.0.0.0", result);
}

test "formatPeerAddr formats broadcast address" {
    var buf: [32]u8 = undefined;
    const addr = [4]u8{ 255, 255, 255, 255 };

    const result = try runtime.formatPeerAddr(addr, &buf);

    try std.testing.expectEqualStrings("255.255.255.255", result);
}

test "formatPeerAddr formats loopback address" {
    var buf: [32]u8 = undefined;
    const addr = [4]u8{ 127, 0, 0, 1 };

    const result = try runtime.formatPeerAddr(addr, &buf);

    try std.testing.expectEqualStrings("127.0.0.1", result);
}
