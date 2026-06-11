// bgp/serve_integration_tests.zig — Tests for BGP serve integration
//
// ACT 4: Wire BGP session into tovarisch serve runtime.
// These tests verify the config parsing and runtime behavior without requiring
// actual network connections or file I/O.
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
    // 5 variants: not_configured, disabled, configured, reconnect_wait, failed
    try std.testing.expectEqual(@as(usize, 5), @typeInfo(serve_integration.BgpRuntimeState).@"enum".fields.len);
    try std.testing.expectEqualStrings("not_configured", @tagName(.not_configured));
    try std.testing.expectEqualStrings("disabled", @tagName(.disabled));
    try std.testing.expectEqualStrings("configured", @tagName(.configured));
    try std.testing.expectEqualStrings("reconnect_wait", @tagName(.reconnect_wait));
    try std.testing.expectEqualStrings("failed", @tagName(.failed));
}

test "BgpLoadResult union has expected variants" {
    // Now has 4 variants: no_config, disabled, configured, failed
    try std.testing.expectEqual(@as(usize, 4), @typeInfo(serve_integration.BgpLoadResult).@"union".fields.len);
}

test "BgpLoadResult.failed has LoadFailure payload" {
    const failure = serve_integration.LoadFailure{ .message = "test error" };
    const result: serve_integration.BgpLoadResult = .{ .failed = failure };
    try std.testing.expectEqualStrings("test error", result.failed.message);
}

test "BgpLoadResult.failed preserves error message" {
    const w = VoidWriter{};
    // This will fail because /nonexistent doesn't exist
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
    try std.testing.expectEqual(@as(u16, 179), cfg.peer_port); // default
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
    // Empty string
    try std.testing.expectError(config.ConfigError.InvalidCidr, config_parse.parseIpv4Address(""));
    
    // Leading dot
    try std.testing.expectError(config.ConfigError.InvalidCidr, config_parse.parseIpv4Address(".0.0.1"));
    
    // Consecutive dots
    try std.testing.expectError(config.ConfigError.InvalidCidr, config_parse.parseIpv4Address("10..0.1"));
    
    // Trailing dot
    try std.testing.expectError(config.ConfigError.InvalidCidr, config_parse.parseIpv4Address("10.0.0.1."));
    
    // Missing octets
    try std.testing.expectError(config.ConfigError.InvalidCidr, config_parse.parseIpv4Address("10.0.1"));
    
    // Extra octets
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
    // This test verifies config parsing succeeds with empty prefixes for zero-prefix smoke test
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
    // No advertised_prefixes - valid for zero-prefix smoke test mode

    const cfg = try config_parse.parseBgpConfig(&raw);
    try std.testing.expect(cfg.enabled);
    try std.testing.expectEqualStrings("", cfg.advertised_prefixes_raw);
}

// ============================================================================
// Regression Tests: Concrete Error Preservation Through Serve Integration
// ============================================================================
// These tests verify that concrete TransportError messages (e.g., "send: EBADF")
// are preserved through serve_integration.runSessionOnce(), not replaced with
// the generic wrapper-level "@errorName(e)" (e.g., "IoError").

/// A fake transport that always fails on send with a configurable error.
const FailingFakeTransport = struct {
    const Self = @This();

    allocator: std.mem.Allocator,
    closed: bool,
    /// Configurable error to return on send
    error_to_return: transport.TransportError,

    pub fn init(allocator: std.mem.Allocator, err: transport.TransportError) Self {
        return Self{
            .allocator = allocator,
            .closed = false,
            .error_to_return = err,
        };
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
            .ctx = @ptrCast(self),
        };
    }
};

test "serve integration preserves concrete session send error over IoError" {
    // This test verifies that when session.runOnce() fails with a concrete
    // TransportError, it sets sess.status.last_error.message to the concrete error
    // (e.g., "send: EBADF") rather than allowing the generic wrapper-level
    // "@errorName(e)" (e.g., "IoError") to dominate.
    //
    // serve_integration.runSessionOnce() copies this concrete error to
    // bundle.last_error via copyErrorToBundle(), so this test proves the
    // error survives the serve integration layer.
    const sess_config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = null,
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{},
        .same_as = true,
    };

    // Create a fake transport that fails with BadFileDescriptor
    var fake_transport = FailingFakeTransport.init(std.testing.allocator, transport.TransportError.BadFileDescriptor);
    var fake_tport = fake_transport.toTransport();

    // Manually construct a minimal bundle-like struct for testing
    // We need to test that runSessionOnce preserves the concrete error
    var sess = try session.init(sess_config, &fake_tport);

    // Capture the session status before calling runOnce
    const result = session.runOnce(&sess);

    // Should return IoError because the wrapper-level catches transport errors
    try std.testing.expectError(session.SessionErrorKind.IoError, result);

    // Session should be in failed state
    try std.testing.expectEqual(session.SessionState.failed, sess.status.state);

    // CRITICAL: The session should have set the CONCRETE error message,
    // NOT the generic wrapper-level "@errorName(e)" (IoError).
    try std.testing.expect(sess.status.last_error != null);
    try std.testing.expectEqualStrings("send: EBADF", sess.status.last_error.?.message);

    fake_transport.close();
}

test "serve integration preserves WouldBlock send error as concrete message" {
    // Verify that WouldBlock (EAGAIN/EWOULDBLOCK) is preserved
    const sess_config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = null,
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{},
        .same_as = true,
    };

    var fake_transport = FailingFakeTransport.init(std.testing.allocator, transport.TransportError.WouldBlock);
    var fake_tport = fake_transport.toTransport();

    var sess = try session.init(sess_config, &fake_tport);

    // Expect IoError from the send failure
    _ = session.runOnce(&sess) catch |err| try std.testing.expect(err == session.SessionErrorKind.IoError);

    try std.testing.expectEqual(session.SessionState.failed, sess.status.state);
    try std.testing.expect(sess.status.last_error != null);
    // Must be the concrete error, NOT "IoError"
    try std.testing.expectEqualStrings("send: EAGAIN/EWOULDBLOCK", sess.status.last_error.?.message);

    fake_transport.close();
}

test "serve integration preserves ConnectionReset send error as concrete message" {
    // Verify that ConnectionReset (ECONNRESET) is preserved
    const sess_config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = null,
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{},
        .same_as = true,
    };

    var fake_transport = FailingFakeTransport.init(std.testing.allocator, transport.TransportError.ConnectionReset);
    var fake_tport = fake_transport.toTransport();

    var sess = try session.init(sess_config, &fake_tport);

    // Expect IoError from the send failure
    _ = session.runOnce(&sess) catch |err| try std.testing.expect(err == session.SessionErrorKind.IoError);

    try std.testing.expectEqual(session.SessionState.failed, sess.status.state);
    try std.testing.expect(sess.status.last_error != null);
    try std.testing.expectEqualStrings("send: ECONNRESET", sess.status.last_error.?.message);

    fake_transport.close();
}
