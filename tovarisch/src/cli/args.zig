const std = @import("std");
const http = @import("../http/server.zig");
const private_ip = @import("../net/private_ip.zig");

/// Errors that can occur during argument parsing.
pub const CliError = error{
    InvalidArguments,
    UnsupportedDeprecatedFlag,
};

/// Log mode for serve command.
pub const LogMode = enum {
    /// Normal mode: emit all startup and runtime logs.
    normal,
    /// Stat-only mode: suppress info logs, print compact interface stats periodically.
    statonly,
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
    ok: ServeConfig,
    usage,
};

/// Configuration returned after parsing serve command arguments.
/// Includes HTTP config and optional path to toml config file.
pub const ServeConfig = struct {
    /// HTTP server configuration.
    http_config: http.Config,
    /// Optional path to TOML config file for BFD runtime.
    config_path: ?[]const u8 = null,
};

/// Parse serve command arguments without starting the daemon.
/// Returns the parsed config or usage error.
pub fn parseServeArgs(args: []const []const u8, stderr: anytype) ServeParseResult {
    var http_config = http.defaultConfig();
    var dangerous_flag_present = false;
    var explicit_listen_address = false;
    var log_mode: LogMode = .normal;
    var stats_interval_seconds: u16 = 30;
    var config_path: ?[]const u8 = null;

    var i: usize = 0;
    while (i < args.len) : (i += 1) {
        const arg = args[i];

        if (std.mem.eql(u8, arg, "--config") and i + 1 < args.len) {
            config_path = args[i + 1];
            i += 1;
        } else if (std.mem.eql(u8, arg, "--listen") and i + 1 < args.len) {
            const addr = args[i + 1];
            if (std.mem.indexOfScalar(u8, addr, ':')) |colon_idx| {
                const host = addr[0..colon_idx];
                const port_str = addr[colon_idx + 1 ..];
                http_config.port = std.fmt.parseInt(u16, port_str, 10) catch {
                    stderr.writeAll("invalid port in --listen address\n") catch {};
                    return .usage;
                };
                http_config.address = host;
            } else {
                http_config.address = addr;
            }
            explicit_listen_address = true;
            i += 1;
        } else if (std.mem.eql(u8, arg, "--listen-private")) {
            http_config.address = "127.0.0.1";
            explicit_listen_address = true;
        } else if (std.mem.eql(u8, arg, "--listen-all-public-dangerous")) {
            // Only set 0.0.0.0 if no explicit --listen address was given
            if (!explicit_listen_address) {
                http_config.address = "0.0.0.0";
            }
            dangerous_flag_present = true;
        } else if (std.mem.eql(u8, arg, "--listen-all")) {
            stderr.writeAll("error: --listen-all is deprecated; use --listen-all-public-dangerous\n") catch {};
            return .usage;
        } else if (std.mem.eql(u8, arg, "--statonly")) {
            log_mode = .statonly;
        } else if (std.mem.eql(u8, arg, "--stats-interval") and i + 1 < args.len) {
            const interval = std.fmt.parseInt(u16, args[i + 1], 10) catch {
                stderr.writeAll("invalid --stats-interval value\n") catch {};
                return .usage;
            };
            if (interval == 0) {
                stderr.writeAll("error: --stats-interval must be >= 1\n") catch {};
                return .usage;
            }
            stats_interval_seconds = interval;
            i += 1;
        } else {
            stderr.print("unknown serve option: {s}\n", .{arg}) catch {};
            return .usage;
        }
    }

    // Store log_mode and stats_interval in config for runtime use
    http_config.log_mode = log_mode;
    http_config.stats_interval_seconds = stats_interval_seconds;

    // Validate bind address if an explicit host was set via --listen
    // (not if it was set by --listen-private or --listen-all-public-dangerous)
    const bind_class = classifyServeBindHost(http_config.address);
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

    return .{ .ok = .{
        .http_config = http_config,
        .config_path = config_path,
    } };
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

/// A writer that captures output into a fixed buffer for testing stderr content.
fn CapturingWriter(comptime capacity: usize) type {
    return struct {
        const Self = @This();
        const Overflow = error{BufferOverflow};

        buffer: [capacity]u8 = undefined,
        len: usize = 0,

        pub fn writeAll(self: *Self, data: []const u8) Overflow!void {
            if (self.len + data.len > capacity) return error.BufferOverflow;
            @memcpy(self.buffer[self.len..][0..data.len], data);
            self.len += data.len;
        }

        pub fn write(self: *Self, data: []const u8) Overflow!usize {
            if (self.len + data.len > capacity) return error.BufferOverflow;
            @memcpy(self.buffer[self.len..][0..data.len], data);
            self.len += data.len;
            return data.len;
        }

        pub fn print(self: *Self, comptime fmt: []const u8, args: anytype) Overflow!void {
            const slice = self.buffer[self.len..];
            const written = std.fmt.bufPrint(slice, fmt, args) catch return error.BufferOverflow;
            self.len += written.len;
        }

        pub fn writeByte(self: *Self, byte: u8) Overflow!void {
            if (self.len >= capacity) return error.BufferOverflow;
            self.buffer[self.len] = byte;
            self.len += 1;
        }

        pub fn flush(_: Self) error{}!void {}

        /// Check if the captured output contains the given substring.
        pub fn contains(self: *Self, needle: []const u8) bool {
            return std.mem.indexOf(u8, self.buffer[0..self.len], needle) != null;
        }

        /// Reset the buffer for reuse.
        pub fn reset(self: *Self) void {
            self.len = 0;
        }
    };
}

test "parseServeArgs defaults to loopback port 8317" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{}, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("127.0.0.1", parsed.ok.http_config.address);
    try std.testing.expectEqual(@as(u16, 8317), parsed.ok.http_config.port);
    try std.testing.expect(parsed.ok.config_path == null);
}

test "parseServeArgs with --config sets config_path" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{ "--config", "/etc/kgb/tovarisch.conf" }, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("/etc/kgb/tovarisch.conf", parsed.ok.config_path.?);
}

test "parseServeArgs with --listen sets address and port" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{ "--listen", "127.0.0.1:9999" }, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("127.0.0.1", parsed.ok.http_config.address);
    try std.testing.expectEqual(@as(u16, 9999), parsed.ok.http_config.port);
}

test "parseServeArgs with --listen-private sets loopback" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{"--listen-private"}, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("127.0.0.1", parsed.ok.http_config.address);
}

test "parseServeArgs with --listen-all-public-dangerous sets 0.0.0.0" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{"--listen-all-public-dangerous"}, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("0.0.0.0", parsed.ok.http_config.address);
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
    try std.testing.expectEqualStrings("10.0.0.1", parsed.ok.http_config.address);
}

test "parseServeArgs accepts RFC1918 private 172.16.x.x" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{ "--listen", "172.16.0.1:8317" }, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("172.16.0.1", parsed.ok.http_config.address);
}

test "parseServeArgs accepts RFC1918 private 192.168.x.x" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{ "--listen", "192.168.1.1:8317" }, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("192.168.1.1", parsed.ok.http_config.address);
}

test "parseServeArgs accepts carrier NAT 100.64.x.x" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{ "--listen", "100.64.0.1:8317" }, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("100.64.0.1", parsed.ok.http_config.address);
}

test "parseServeArgs accepts link-local 169.254.x.x" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{ "--listen", "169.254.0.1:8317" }, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("169.254.0.1", parsed.ok.http_config.address);
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
    try std.testing.expectEqualStrings("8.8.8.8", parsed.ok.http_config.address);
}

test "parseServeArgs rejects 0.0.0.0 without dangerous flag" {
    const w = VoidWriter{};
    try std.testing.expect(parseServeArgs(&.{ "--listen", "0.0.0.0:8317" }, w) == .usage);
}

test "parseServeArgs accepts 0.0.0.0 with dangerous flag" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{ "--listen", "0.0.0.0:8317", "--listen-all-public-dangerous" }, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("0.0.0.0", parsed.ok.http_config.address);
}

test "parseServeArgs accepts 1.1.1.1 with dangerous flag" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{ "--listen", "1.1.1.1:8317", "--listen-all-public-dangerous" }, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqualStrings("1.1.1.1", parsed.ok.http_config.address);
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

// --- Stderr content tests for bind rejection safety messages ---

test "parseServeArgs rejects public IP and mentions safety phrase" {
    var w = CapturingWriter(256){};
    try std.testing.expect(parseServeArgs(&.{ "--listen", "8.8.8.8:8317" }, &w) == .usage);
    try std.testing.expect(w.contains("refusing to bind to public IP address"));
    try std.testing.expect(w.contains("--listen-all-public-dangerous"));
}

test "parseServeArgs rejects 0.0.0.0 and mentions wildcard phrase" {
    var w = CapturingWriter(256){};
    try std.testing.expect(parseServeArgs(&.{ "--listen", "0.0.0.0:8317" }, &w) == .usage);
    try std.testing.expect(w.contains("0.0.0.0 bind requires --listen-all-public-dangerous"));
}

test "parseServeArgs deprecated --listen-all mentions dangerous flag" {
    var w = CapturingWriter(256){};
    try std.testing.expect(parseServeArgs(&.{"--listen-all"}, &w) == .usage);
    try std.testing.expect(w.contains("--listen-all-public-dangerous"));
}

// --- Tests for statonly mode ---

test "parseServeArgs with --statonly sets statonly mode" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{"--statonly"}, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expect(parsed.ok.http_config.log_mode == .statonly);
}

test "parseServeArgs --statonly with --listen" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{ "--statonly", "--listen", "127.0.0.1:8317" }, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expect(parsed.ok.http_config.log_mode == .statonly);
    try std.testing.expectEqualStrings("127.0.0.1", parsed.ok.http_config.address);
}

test "parseServeArgs --statonly with --stats-interval" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{ "--statonly", "--stats-interval", "10" }, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expect(parsed.ok.http_config.log_mode == .statonly);
    try std.testing.expectEqual(@as(u16, 10), parsed.ok.http_config.stats_interval_seconds);
}

test "parseServeArgs --statonly --stats-interval combined" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{ "--statonly", "--stats-interval", "30" }, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqual(@as(u16, 30), parsed.ok.http_config.stats_interval_seconds);
}

test "parseServeArgs invalid --stats-interval returns usage" {
    const w = VoidWriter{};
    try std.testing.expect(parseServeArgs(&.{ "--statonly", "--stats-interval", "abc" }, w) == .usage);
}

test "parseServeArgs defaults log_mode to normal" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{}, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expect(parsed.ok.http_config.log_mode == .normal);
}

test "parseServeArgs defaults stats_interval to 30" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{}, w);
    try std.testing.expect(parsed == .ok);
    try std.testing.expectEqual(@as(u16, 30), parsed.ok.http_config.stats_interval_seconds);
}

test "parseServeArgs rejects --stats-interval 0" {
    var w = CapturingWriter(256){};
    try std.testing.expect(parseServeArgs(&.{ "--statonly", "--stats-interval", "0" }, &w) == .usage);
    try std.testing.expect(w.contains("--stats-interval must be >= 1"));
}
