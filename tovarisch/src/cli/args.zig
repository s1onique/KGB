const std = @import("std");
const http = @import("../http/server.zig");
const private_ip = @import("../net/private_ip.zig");

/// Errors that can occur during argument parsing.
pub const CliError = error{
    InvalidArguments,
    UnsupportedDeprecatedFlag,
};

/// Result of validating a serve bind host.
pub const BindValidation = enum {
    /// Host is safe for binding without dangerous flag.
    safe,
    /// Host is 0.0.0.0, requires dangerous flag.
    wildcard_requires_dangerous,
    /// Host is explicitly public, requires dangerous flag.
    public_requires_dangerous,
    /// Host is not a valid IPv4 address (likely a hostname).
    non_ipv4,
};

/// Classify a bind host for serve command validation.
/// Returns whether the host is safe, requires dangerous flag, or is non-IPv4.
fn classifyServeBindHost(host: []const u8) BindValidation {
    // Special-case 0.0.0.0 - always requires dangerous flag
    if (std.mem.eql(u8, host, "0.0.0.0")) {
        return .wildcard_requires_dangerous;
    }

    const ip_class = private_ip.classifyIpv4Text(host);

    switch (ip_class) {
        .invalid => {
            // Not a valid IPv4 address - likely a hostname
            // Allow it through; hostname resolution is a runtime concern
            return .non_ipv4;
        },
        .public => {
            // Explicitly public IP - dangerous without flag
            return .public_requires_dangerous;
        },
        else => {
            // loopback, private, carrier_nat, link_local, documentation, multicast
            // All are considered safe for local/constrained environments
            return .safe;
        },
    }
}

/// Result of parsing serve command arguments.
pub const ServeParseResult = union(enum) {
    ok: http.Config,
    usage,
};

/// Parse serve command arguments without starting the daemon.
/// Returns the parsed config or usage error.
pub fn parseServeArgs(args: []const []const u8, stderr: anytype) ServeParseResult {
    var config = http.defaultConfig();
    var dangerous_flag_present = false;
    var explicit_listen_address = false;

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
            explicit_listen_address = true;
            i += 1;
        } else if (std.mem.eql(u8, arg, "--listen-private")) {
            config.address = "127.0.0.1";
            explicit_listen_address = true;
        } else if (std.mem.eql(u8, arg, "--listen-all-public-dangerous")) {
            // Only set 0.0.0.0 if no explicit --listen address was given
            if (!explicit_listen_address) {
                config.address = "0.0.0.0";
            }
            dangerous_flag_present = true;
        } else if (std.mem.eql(u8, arg, "--listen-all")) {
            stderr.writeAll("error: --listen-all is deprecated; use --listen-all-public-dangerous\n") catch {};
            return .usage;
        } else {
            stderr.print("unknown serve option: {s}\n", .{arg}) catch {};
            return .usage;
        }
    }

    // Validate bind address if an explicit host was set via --listen
    // (not if it was set by --listen-private or --listen-all-public-dangerous)
    const bind_class = classifyServeBindHost(config.address);
    switch (bind_class) {
        .safe, .non_ipv4 => {
            // Safe or hostname - allow through
        },
        .public_requires_dangerous => {
            if (!dangerous_flag_present) {
                stderr.writeAll("error: refusing to bind to public IP address without --listen-all-public-dangerous\n") catch {};
                stderr.writeAll("hint: use --listen 127.0.0.1 for loopback or a private RFC1918 address\n") catch {};
                return .usage;
            }
        },
        .wildcard_requires_dangerous => {
            // 0.0.0.0 is technically public (0.0.0.0/0) but we want explicit dangerous flag
            if (!dangerous_flag_present) {
                stderr.writeAll("error: 0.0.0.0 bind requires --listen-all-public-dangerous\n") catch {};
                return .usage;
            }
        },
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

// --- Tests for bind address validation ---

test "parseServeArgs accepts RFC1918 private 10.x.x.x" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{ "--listen", "10.0.0.1:8317" }, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("10.0.0.1", parsed.ok.address);
}

test "parseServeArgs accepts RFC1918 private 172.16.x.x" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{ "--listen", "172.16.0.1:8317" }, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("172.16.0.1", parsed.ok.address);
}

test "parseServeArgs accepts RFC1918 private 192.168.x.x" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{ "--listen", "192.168.1.1:8317" }, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("192.168.1.1", parsed.ok.address);
}

test "parseServeArgs accepts carrier NAT 100.64.x.x" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{ "--listen", "100.64.0.1:8317" }, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("100.64.0.1", parsed.ok.address);
}

test "parseServeArgs accepts link-local 169.254.x.x" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{ "--listen", "169.254.0.1:8317" }, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("169.254.0.1", parsed.ok.address);
}

test "parseServeArgs rejects public IPv4 without dangerous flag" {
    const w = VoidWriter{};
    // 8.8.8.8 is a public DNS server
    try std.testing.expect(parseServeArgs(&.{ "--listen", "8.8.8.8:8317" }, w) == .usage);
}

test "parseServeArgs accepts public IPv4 with dangerous flag" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{ "--listen", "8.8.8.8:8317", "--listen-all-public-dangerous" }, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("8.8.8.8", parsed.ok.address);
}

test "parseServeArgs rejects 0.0.0.0 without dangerous flag" {
    const w = VoidWriter{};
    try std.testing.expect(parseServeArgs(&.{ "--listen", "0.0.0.0:8317" }, w) == .usage);
}

test "parseServeArgs accepts 0.0.0.0 with dangerous flag" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{ "--listen", "0.0.0.0:8317", "--listen-all-public-dangerous" }, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("0.0.0.0", parsed.ok.address);
}

test "parseServeArgs accepts 1.1.1.1 with dangerous flag" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{ "--listen", "1.1.1.1:8317", "--listen-all-public-dangerous" }, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("1.1.1.1", parsed.ok.address);
}

test "parseServeArgs with other public IP rejected without dangerous flag" {
    const w = VoidWriter{};
    // 93.184.216.34 is example.com
    try std.testing.expect(parseServeArgs(&.{ "--listen", "93.184.216.34:8317" }, w) == .usage);
}

// --- Tests for deprecated --listen-all ---

test "parseServeArgs with --listen-all still returns usage" {
    const w = VoidWriter{};
    try std.testing.expect(parseServeArgs(&.{"--listen-all"}, w) == .usage);
}
