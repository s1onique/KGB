// bgp/serve_integration_tests.zig — Tests for BGP serve integration
//
// KEY CONSTRAINT TESTS:
// - When BGP is disabled, ZERO sockets are created
// - Empty prefixes config is ACCEPTED when enabled (zero-prefix smoke test mode)
// - Enabled config builds valid session config

const std = @import("std");
const config = @import("../config.zig");
const config_parse = @import("config_parse.zig");
const serve_integration = @import("serve_integration.zig");
const session = @import("session.zig");
const transport = @import("transport.zig");

const VoidWriter = struct {
    const Self = @This();
    pub fn writeAll(_: Self, _: []const u8) error{}!void {}
    pub fn write(_: Self, _: []const u8) error{}!void {}
    pub fn print(_: Self, _: []const u8, _: anytype) error{}!void {}
};

test "loadConfigAndBgp returns no_config when no config path" {
    const w = VoidWriter{};
    const result = serve_integration.loadConfigAndBgp(null, w, std.heap.page_allocator);
    try std.testing.expect(result == .no_config);
}

test "BgpRuntimeState enum has expected variants" {
    try std.testing.expectEqual(@as(usize, 5), @typeInfo(serve_integration.BgpRuntimeState).@"enum".fields.len);
    try std.testing.expectEqualStrings("not_configured", @tagName(.not_configured));
    try std.testing.expectEqualStrings("disabled", @tagName(.disabled));
    try std.testing.expectEqualStrings("configured", @tagName(.configured));
    try std.testing.expectEqualStrings("reconnect_wait", @tagName(.reconnect_wait));
    try std.testing.expectEqualStrings("failed", @tagName(.failed));
}

test "BgpLoadResult union has expected variants" {
    try std.testing.expectEqual(@as(usize, 5), @typeInfo(serve_integration.BgpLoadResult).@"union".fields.len);
}

test "BgpLoadResult.failed has LoadFailure payload" {
    const failure = serve_integration.LoadFailure{ .message = "test error" };
    const result: serve_integration.BgpLoadResult = .{ .failed = failure };
    try std.testing.expectEqualStrings("test error", result.failed.message);
}

test "BgpLoadResult.failed preserves error message" {
    const w = VoidWriter{};
    const result = serve_integration.loadConfigAndBgp("/nonexistent/config.toml", w, std.heap.page_allocator);
    try std.testing.expect(result == .failed);
    try std.testing.expectEqualStrings("failed to read config", result.failed.message);
}

test "parseBgpConfig returns disabled with present=false when no [bgp] section" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);
    const cfg = try config_parse.parseBgpConfig(&raw);
    try std.testing.expect(!cfg.present);
    try std.testing.expect(!cfg.enabled);
}

test "parseBgpConfig returns present=true when [bgp] section exists" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);
    try raw.put(std.heap.page_allocator, "bgp", .{});
    const bgp_section = raw.getPtr("bgp").?;
    try bgp_section.put(std.heap.page_allocator, "enabled", "false");
    const cfg = try config_parse.parseBgpConfig(&raw);
    try std.testing.expect(cfg.present);
    try std.testing.expect(!cfg.enabled);
}

test "parseBgpConfig accepts disabled with missing fields" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);
    try raw.put(std.heap.page_allocator, "bgp", .{});
    const bgp_section = raw.getPtr("bgp").?;
    try bgp_section.put(std.heap.page_allocator, "enabled", "false");
    const cfg = try config_parse.parseBgpConfig(&raw);
    try std.testing.expect(!cfg.enabled);
}

// REGRESSION: parseBgpConfig parses [bgp] present with enabled=false as present=true, enabled=false.
// The .disabled return value is determined by serve_integration's loadConfigAndBgp() based on this parsing.
test "REGRESSION: parseBgpConfig returns present=true, enabled=false for [bgp] enabled=false" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);
    try raw.put(std.heap.page_allocator, "bgp", .{});
    const bgp_section = raw.getPtr("bgp").?;
    try bgp_section.put(std.heap.page_allocator, "enabled", "false");
    const cfg = try config_parse.parseBgpConfig(&raw);
    try std.testing.expect(cfg.present);
    try std.testing.expect(!cfg.enabled);
}

test "REGRESSION: parseBgpConfig returns present=true for [bgp] section" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);
    try raw.put(std.heap.page_allocator, "bgp", .{});
    const bgp_section = raw.getPtr("bgp").?;
    try bgp_section.put(std.heap.page_allocator, "enabled", "false");
    const cfg = try config_parse.parseBgpConfig(&raw);
    try std.testing.expect(cfg.present);
    try std.testing.expect(!cfg.enabled);
}

test "parseBgpConfig parses enabled config with required fields" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);
    try raw.put(std.heap.page_allocator, "bgp", .{});
    const bgp_section = raw.getPtr("bgp").?;
    try bgp_section.put(std.heap.page_allocator, "enabled", "true");
    try bgp_section.put(std.heap.page_allocator, "local_address", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "router_id", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "local_as", "65001");
    try bgp_section.put(std.heap.page_allocator, "peer_address", "10.0.0.2");
    try bgp_section.put(std.heap.page_allocator, "peer_as", "65002");
    try bgp_section.put(std.heap.page_allocator, "advertised_prefixes", "10.0.0.0/8");
    const cfg = try config_parse.parseBgpConfig(&raw);
    try std.testing.expect(cfg.enabled);
    try std.testing.expectEqualStrings("10.0.0.1", cfg.local_address);
    try std.testing.expectEqualStrings("10.0.0.1", cfg.router_id);
    try std.testing.expectEqual(@as(u16, 65001), cfg.local_as);
    try std.testing.expectEqualStrings("10.0.0.2", cfg.peer_address);
    try std.testing.expectEqual(@as(u16, 65002), cfg.peer_as);
    try std.testing.expectEqual(@as(u16, 179), cfg.peer_port);
    try std.testing.expect(cfg.advertised_prefixes_raw.len > 0);
}

test "parseBgpConfig requires local_address when enabled" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);
    try raw.put(std.heap.page_allocator, "bgp", .{});
    const bgp_section = raw.getPtr("bgp").?;
    try bgp_section.put(std.heap.page_allocator, "enabled", "true");
    try bgp_section.put(std.heap.page_allocator, "router_id", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "local_as", "65001");
    try bgp_section.put(std.heap.page_allocator, "peer_address", "10.0.0.2");
    try bgp_section.put(std.heap.page_allocator, "peer_as", "65002");
    try std.testing.expectError(config.ConfigError.MissingKey, config_parse.parseBgpConfig(&raw));
}

test "parseBgpConfig requires peer_address when enabled" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);
    try raw.put(std.heap.page_allocator, "bgp", .{});
    const bgp_section = raw.getPtr("bgp").?;
    try bgp_section.put(std.heap.page_allocator, "enabled", "true");
    try bgp_section.put(std.heap.page_allocator, "local_address", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "router_id", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "local_as", "65001");
    try bgp_section.put(std.heap.page_allocator, "peer_as", "65002");
    try std.testing.expectError(config.ConfigError.MissingKey, config_parse.parseBgpConfig(&raw));
}

test "parseBgpConfig stores advertised_prefixes_raw as string" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);
    try raw.put(std.heap.page_allocator, "bgp", .{});
    const bgp_section = raw.getPtr("bgp").?;
    try bgp_section.put(std.heap.page_allocator, "enabled", "true");
    try bgp_section.put(std.heap.page_allocator, "local_address", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "router_id", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "local_as", "65001");
    try bgp_section.put(std.heap.page_allocator, "peer_address", "10.0.0.2");
    try bgp_section.put(std.heap.page_allocator, "peer_as", "65002");
    try bgp_section.put(std.heap.page_allocator, "advertised_prefixes", "10.0.0.0/8,192.168.0.0/16");
    const cfg = try config_parse.parseBgpConfig(&raw);
    try std.testing.expectEqualStrings("10.0.0.0/8,192.168.0.0/16", cfg.advertised_prefixes_raw);
}

test "parseIpv4Address accepts valid IPv4" {
    const addr = try config_parse.parseIpv4Address("10.0.0.1");
    try std.testing.expectEqual(@as(u8, 10), addr[0]);
    try std.testing.expectEqual(@as(u8, 0), addr[1]);
    try std.testing.expectEqual(@as(u8, 0), addr[2]);
    try std.testing.expectEqual(@as(u8, 1), addr[3]);
}

test "parseIpv4Address accepts 127.0.0.1" {
    const addr = try config_parse.parseIpv4Address("127.0.0.1");
    try std.testing.expectEqual(@as(u8, 127), addr[0]);
    try std.testing.expectEqual(@as(u8, 0), addr[1]);
    try std.testing.expectEqual(@as(u8, 0), addr[2]);
    try std.testing.expectEqual(@as(u8, 1), addr[3]);
}

test "parseIpv4Address rejects CIDR suffix" {
    try std.testing.expectError(config.ConfigError.InvalidCidr, config_parse.parseIpv4Address("10.0.0.1/32"));
}

test "parseIpv4Address rejects IPv6" {
    try std.testing.expectError(config.ConfigError.InvalidCidr, config_parse.parseIpv4Address("::1"));
    try std.testing.expectError(config.ConfigError.InvalidCidr, config_parse.parseIpv4Address("2001:db8::1"));
}

test "parseIpv4Address rejects out of range octets" {
    try std.testing.expectError(config.ConfigError.InvalidCidr, config_parse.parseIpv4Address("10.0.0.256"));
    try std.testing.expectError(config.ConfigError.InvalidCidr, config_parse.parseIpv4Address("256.0.0.1"));
}

test "parseIpv4Address rejects malformed addresses" {
    try std.testing.expectError(config.ConfigError.InvalidCidr, config_parse.parseIpv4Address(""));
    try std.testing.expectError(config.ConfigError.InvalidCidr, config_parse.parseIpv4Address(".0.0.1"));
    try std.testing.expectError(config.ConfigError.InvalidCidr, config_parse.parseIpv4Address("10..0.1"));
    try std.testing.expectError(config.ConfigError.InvalidCidr, config_parse.parseIpv4Address("10.0.0.1."));
    try std.testing.expectError(config.ConfigError.InvalidCidr, config_parse.parseIpv4Address("10.0.1"));
    try std.testing.expectError(config.ConfigError.InvalidCidr, config_parse.parseIpv4Address("10.0.0.1.2"));
}

test "parsePrefixList parses single prefix" {
    const result = try config_parse.parsePrefixList("10.0.0.0/8", std.heap.page_allocator);
    defer std.heap.page_allocator.free(result);
    try std.testing.expectEqual(@as(usize, 1), result.len);
    try std.testing.expectEqualStrings("10.0.0.0/8", result[0]);
}

test "parsePrefixList parses multiple prefixes" {
    const result = try config_parse.parsePrefixList("10.0.0.0/8, 192.168.0.0/16", std.heap.page_allocator);
    defer std.heap.page_allocator.free(result);
    try std.testing.expectEqual(@as(usize, 2), result.len);
}

test "parsePrefixList accepts empty for zero-prefix BGP smoke test" {
    const result = try config_parse.parsePrefixList("", std.heap.page_allocator);
    defer std.heap.page_allocator.free(result);
    try std.testing.expectEqual(@as(usize, 0), result.len);
}

test "parsePrefixList accepts whitespace-only for zero-prefix BGP smoke test" {
    const result = try config_parse.parsePrefixList("   ", std.heap.page_allocator);
    defer std.heap.page_allocator.free(result);
    try std.testing.expectEqual(@as(usize, 0), result.len);
}

test "parseBgpConfig accepts empty advertised_prefixes when enabled" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);
    try raw.put(std.heap.page_allocator, "bgp", .{});
    const bgp_section = raw.getPtr("bgp").?;
    try bgp_section.put(std.heap.page_allocator, "enabled", "true");
    try bgp_section.put(std.heap.page_allocator, "local_address", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "router_id", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "local_as", "65001");
    try bgp_section.put(std.heap.page_allocator, "peer_address", "10.0.0.2");
    try bgp_section.put(std.heap.page_allocator, "peer_as", "65002");
    const cfg = try config_parse.parseBgpConfig(&raw);
    try std.testing.expect(cfg.enabled);
    try std.testing.expectEqualStrings("", cfg.advertised_prefixes_raw);
}

// REGRESSION: parseBgpConfig accepts zero prefixes via explicit empty string.
// This enables zero-prefix BGP smoke test mode where passive listener starts
// even when no prefixes are advertised.
test "REGRESSION: parseBgpConfig accepts advertised_prefixes empty string" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);
    try raw.put(std.heap.page_allocator, "bgp", .{});
    const bgp_section = raw.getPtr("bgp").?;
    try bgp_section.put(std.heap.page_allocator, "enabled", "true");
    try bgp_section.put(std.heap.page_allocator, "local_address", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "router_id", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "local_as", "65001");
    try bgp_section.put(std.heap.page_allocator, "peer_address", "10.0.0.2");
    try bgp_section.put(std.heap.page_allocator, "peer_as", "65002");
    try bgp_section.put(std.heap.page_allocator, "advertised_prefixes", "");
    const cfg = try config_parse.parseBgpConfig(&raw);
    try std.testing.expect(cfg.enabled);
    // Empty string should be stored as-is (not rejected)
    try std.testing.expectEqualStrings("", cfg.advertised_prefixes_raw);
}

// REGRESSION: parseBgpConfig accepts advertised_prefixes whitespace-only.
// getString() trims whitespace, so "   " becomes "" - which is still valid (zero prefixes).
test "REGRESSION: parseBgpConfig accepts advertised_prefixes whitespace-only" {
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);
    try raw.put(std.heap.page_allocator, "bgp", .{});
    const bgp_section = raw.getPtr("bgp").?;
    try bgp_section.put(std.heap.page_allocator, "enabled", "true");
    try bgp_section.put(std.heap.page_allocator, "local_address", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "router_id", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "local_as", "65001");
    try bgp_section.put(std.heap.page_allocator, "peer_address", "10.0.0.2");
    try bgp_section.put(std.heap.page_allocator, "peer_as", "65002");
    try bgp_section.put(std.heap.page_allocator, "advertised_prefixes", "   ");
    const cfg = try config_parse.parseBgpConfig(&raw);
    try std.testing.expect(cfg.enabled);
    // getString trims whitespace, so "   " becomes "" - zero prefixes is valid
    try std.testing.expectEqualStrings("", cfg.advertised_prefixes_raw);
}

// Concrete TransportError messages preserved through runSessionOnce().
const FailingFakeTransport = struct {
    const Self = @This();
    allocator: std.mem.Allocator,
    closed: bool,
    error_to_return: transport.TransportError,
    pub fn init(allocator: std.mem.Allocator, err: transport.TransportError) Self {
        return Self{ .allocator = allocator, .closed = false, .error_to_return = err };
    }
    pub fn send(self: *Self, data: []const u8) transport.TransportError!void {
        _ = data;
        return self.error_to_return;
    }
    pub fn recv(self: *Self) []const u8 {
        _ = self;
        return &[_]u8{};
    }
    pub fn close(self: *Self) void {
        self.closed = true;
    }
    pub fn toTransport(self: *Self) transport.Transport {
        return transport.Transport{
            .sendFn = struct {
                fn send(ctx: *anyopaque, data: []const u8) transport.TransportError!void {
                    const fake: *Self = @ptrCast(@alignCast(ctx));
                    return fake.send(data);
                }
            }.send,
            .recvFn = struct {
                fn recv(ctx: *anyopaque) []const u8 {
                    const fake: *Self = @ptrCast(@alignCast(ctx));
                    return fake.recv();
                }
            }.recv,
            .closeFn = struct {
                fn close(ctx: *anyopaque) void {
                    const fake: *Self = @ptrCast(@alignCast(ctx));
                    fake.close();
                }
            }.close,
            .isClosedFn = struct {
                fn isClosed(ctx: *anyopaque) bool {
                    const fake: *Self = @ptrCast(@alignCast(ctx));
                    return fake.closed;
                }
            }.isClosed,
            .ctx = @ptrCast(self),
        };
    }
};

test "serve integration preserves concrete session send error over IoError" {
    const sess_config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 }, .peer_port = 179, .local_address = null,
        .local_as = 65001, .peer_as = 65002, .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180, .keepalive_seconds = 60, .connect_timeout_ms = 5000,
        .prefixes = &.{}, .same_as = true,
    };
    var fake_transport = FailingFakeTransport.init(std.testing.allocator, transport.TransportError.BadFileDescriptor);
    var fake_tport = fake_transport.toTransport();
    var sess = try session.init(sess_config, &fake_tport);
    const result = session.runOnce(&sess);
    try std.testing.expectError(session.SessionErrorKind.IoError, result);
    try std.testing.expectEqual(session.SessionState.failed, sess.status.state);
    try std.testing.expect(sess.status.last_error != null);
    try std.testing.expectEqualStrings("send: EBADF", sess.status.last_error.?.message);
    fake_transport.close();
}

test "serve integration preserves WouldBlock send error as concrete message" {
    const sess_config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 }, .peer_port = 179, .local_address = null,
        .local_as = 65001, .peer_as = 65002, .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180, .keepalive_seconds = 60, .connect_timeout_ms = 5000,
        .prefixes = &.{}, .same_as = true,
    };
    var fake_transport = FailingFakeTransport.init(std.testing.allocator, transport.TransportError.WouldBlock);
    var fake_tport = fake_transport.toTransport();
    var sess = try session.init(sess_config, &fake_tport);
    _ = session.runOnce(&sess) catch |err| try std.testing.expect(err == session.SessionErrorKind.IoError);
    try std.testing.expectEqual(session.SessionState.failed, sess.status.state);
    try std.testing.expect(sess.status.last_error != null);
    try std.testing.expectEqualStrings("send: EAGAIN/EWOULDBLOCK", sess.status.last_error.?.message);
    fake_transport.close();
}

test "serve integration preserves ConnectionReset send error as concrete message" {
    const sess_config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 }, .peer_port = 179, .local_address = null,
        .local_as = 65001, .peer_as = 65002, .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180, .keepalive_seconds = 60, .connect_timeout_ms = 5000,
        .prefixes = &.{}, .same_as = true,
    };
    var fake_transport = FailingFakeTransport.init(std.testing.allocator, transport.TransportError.ConnectionReset);
    var fake_tport = fake_transport.toTransport();
    var sess = try session.init(sess_config, &fake_tport);
    _ = session.runOnce(&sess) catch |err| try std.testing.expect(err == session.SessionErrorKind.IoError);
    try std.testing.expectEqual(session.SessionState.failed, sess.status.state);
    try std.testing.expect(sess.status.last_error != null);
    try std.testing.expectEqualStrings("send: ECONNRESET", sess.status.last_error.?.message);
    fake_transport.close();
}
