// args_explicit_listen_tests.zig — Tests for explicit_listen tracking in CLI argument parsing
//
// These tests verify that --listen and --listen-private correctly set the explicit_listen
// flag so that config file [server].listen does not override CLI-provided values.

const std = @import("std");
const cli_args = @import("args.zig");

// Test writer that discards all output
const VoidWriter = struct {
    const Self = @This();

    pub fn writeAll(_: Self, _: []const u8) error{}!void {}
    pub fn write(_: Self, _: []const u8) error{}!usize { return 0; }
    pub fn print(_: Self, _: []const u8, _: anytype) error{}!void {}
    pub fn writeByte(_: Self, _: u8) error{}!void {}
    pub fn flush(_: Self) error{}!void {}
};

test "parseServeArgs with --listen sets explicit_listen true" {
    const w = VoidWriter{};
    const parsed = cli_args.parseServeArgs(&.{ "--listen", "10.149.149.1:8317" }, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expect(parsed.ok.explicit_listen);
}

test "parseServeArgs with --listen-private sets explicit_listen true" {
    const w = VoidWriter{};
    const parsed = cli_args.parseServeArgs(&.{"--listen-private"}, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expect(parsed.ok.explicit_listen);
}

test "parseServeArgs defaults to loopback port 8317" {
    const w = VoidWriter{};
    const parsed = cli_args.parseServeArgs(&.{}, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("127.0.0.1", parsed.ok.http_config.address);
    try std.testing.expectEqual(@as(u16, 8317), parsed.ok.http_config.port);
    try std.testing.expect(parsed.ok.config_path == null);
    try std.testing.expect(!parsed.ok.explicit_listen);
}
