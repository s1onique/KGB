const std = @import("std");
const http = @import("../http/server.zig");

/// Errors that can occur during argument parsing.
pub const CliError = error{
    InvalidArguments,
    UnsupportedDeprecatedFlag,
};

/// Result of parsing serve command arguments.
pub const ServeParseResult = union(enum) {
    ok: http.Config,
    usage,
};

/// Parse serve command arguments without starting the daemon.
/// Returns the parsed config or usage error.
pub fn parseServeArgs(args: []const []const u8, stderr: anytype) ServeParseResult {
    var config = http.defaultConfig();

    var i: usize = 0;
    while (i < args.len) : (i += 1) {
        const arg = args[i];

        if (std.mem.eql(u8, arg, "--listen") and i + 1 < args.len) {
            const addr = args[i + 1];
            if (std.mem.indexOfScalar(u8, addr, ':')) |colon_idx| {
                const host = addr[0..colon_idx];
                const port_str = addr[colon_idx + 1 ..];
                config.port = std.fmt.parseInt(u16, port_str, 10) catch {
                    stderr.writeAll("invalid port in --listen address\n") catch {};
                    return .usage;
                };
                config.address = host;
            } else {
                config.address = addr;
            }
            i += 1;
        } else if (std.mem.eql(u8, arg, "--listen-private")) {
            config.address = "127.0.0.1";
        } else if (std.mem.eql(u8, arg, "--listen-all-public-dangerous")) {
            config.address = "0.0.0.0";
        } else if (std.mem.eql(u8, arg, "--listen-all")) {
            stderr.writeAll("error: --listen-all is deprecated; use --listen-all-public-dangerous\n") catch {};
            return .usage;
        } else {
            stderr.print("unknown serve option: {s}\n", .{arg}) catch {};
            return .usage;
        }
    }

    return .{ .ok = config };
}

// --- Tests for serve argument parsing ---

const VoidWriter = struct {
    const Self = @This();

    pub fn writeAll(_: Self, _: []const u8) error{}!void {}
    pub fn write(_: Self, _: []const u8) error{}!void {}
    pub fn print(_: Self, _: []const u8, _: anytype) error{}!void {}
    pub fn writeByte(_: Self, _: u8) error{}!void {}
    pub fn flush(_: Self) error{}!void {}
};

test "parseServeArgs defaults to loopback port 8317" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{}, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("127.0.0.1", parsed.ok.address);
    try std.testing.expectEqual(@as(u16, 8317), parsed.ok.port);
}

test "parseServeArgs with --listen sets address and port" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{ "--listen", "127.0.0.1:9999" }, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("127.0.0.1", parsed.ok.address);
    try std.testing.expectEqual(@as(u16, 9999), parsed.ok.port);
}

test "parseServeArgs with --listen-private sets loopback" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{"--listen-private"}, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("127.0.0.1", parsed.ok.address);
}

test "parseServeArgs with --listen-all-public-dangerous sets 0.0.0.0" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{"--listen-all-public-dangerous"}, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("0.0.0.0", parsed.ok.address);
}

test "parseServeArgs with deprecated --listen-all returns usage" {
    const w = VoidWriter{};
    try std.testing.expect(parseServeArgs(&.{"--listen-all"}, w) == .usage);
}

test "parseServeArgs with unknown option returns usage" {
    const w = VoidWriter{};
    try std.testing.expect(parseServeArgs(&.{"--unknown"}, w) == .usage);
}
