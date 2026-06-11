// session_tests.zig — BGP session basic tests
//
// ACT 2: Basic tests for BGP session state machine components.
// Handshake tests are in session_handshake_tests.zig.

const std = @import("std");
const session = @import("session.zig");
const types = @import("types.zig");
const session_status = @import("session_status.zig");

// === Session Config Validation Tests ===

test "SessionConfig validation accepts zero hold_time" {
    const config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = null,
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 0,
        .keepalive_seconds = 0,
        .connect_timeout_ms = 5000,
        .prefixes = &.{types.Ipv4Prefix.init("10.0.0.0/8")},
        .same_as = true,
    };
    try std.testing.expect(try session.validateConfig(config) == {});
}

test "SessionConfig validation rejects AS out of range low" {
    const config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = null,
        .local_as = 0,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{types.Ipv4Prefix.init("10.0.0.0/8")},
        .same_as = true,
    };
    try std.testing.expectError(session.SessionErrorKind.InvalidConfig, session.validateConfig(config));
}

// === Session State Machine Tests ===

test "session stop transitions to stopped state" {
    const config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = null,
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{types.Ipv4Prefix.init("10.0.0.0/8")},
        .same_as = true,
    };
    var fake = session.FakeTransport.init(std.testing.allocator, &.{});
    defer fake.deinit();
    var sess = try session.init(config, &fake.toTransport());
    session.stop(&sess);
    try std.testing.expectEqual(session.SessionState.stopped, sess.status.state);
}

test "isTerminal works on stopped session" {
    const config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = null,
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{types.Ipv4Prefix.init("10.0.0.0/8")},
        .same_as = true,
    };
    var fake = session.FakeTransport.init(std.testing.allocator, &.{});
    defer fake.deinit();
    var sess = try session.init(config, &fake.toTransport());
    session.stop(&sess);
    try std.testing.expect(session.isTerminal(&sess));
}

// === Transport Tests ===

test "FakeTransport captures multiple sends" {
    var fake = session.FakeTransport.init(std.testing.allocator, &.{});
    defer fake.deinit();
    try fake.send(&[_]u8{ 1, 2, 3 });
    try fake.send(&[_]u8{ 4, 5, 6, 7 });
    const sent = fake.getSent();
    try std.testing.expectEqualSlices(u8, &.{ 1, 2, 3, 4, 5, 6, 7 }, sent);
}

test "FakeTransport returns responses in order" {
    const responses = &.{
        session.PeerResponse{ .recv_bytes = &.{ 10, 20 } },
        session.PeerResponse{ .recv_bytes = &.{ 30, 40, 50 } },
    };
    var fake = session.FakeTransport.init(std.testing.allocator, responses);
    defer fake.deinit();
    try std.testing.expectEqualSlices(u8, &.{ 10, 20 }, fake.recv());
    try std.testing.expectEqualSlices(u8, &.{ 30, 40, 50 }, fake.recv());
    try std.testing.expectEqualSlices(u8, &.{}, fake.recv());
}

// === Session Counters Tests ===

test "advertised_prefix_count is set on init" {
    const prefixes = &.{
        types.Ipv4Prefix.init("10.0.0.0/8"),
        types.Ipv4Prefix.init("192.168.0.0/16"),
    };
    const config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = null,
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = prefixes,
        .same_as = true,
    };
    var fake = session.FakeTransport.init(std.testing.allocator, &.{});
    defer fake.deinit();
    const sess = try session.init(config, &fake.toTransport());
    try std.testing.expectEqual(@as(usize, 2), sess.status.advertised_prefix_count);
}

// === Session Core Tests ===

test "Session state enum has expected values" {
    try std.testing.expectEqual(@as(usize, 7), @typeInfo(session.SessionState).@"enum".fields.len);
    try std.testing.expectEqualStrings("idle", @tagName(.idle));
    try std.testing.expectEqualStrings("established", @tagName(.established));
}

test "isEstablished returns false for idle state" {
    const status = session_status.initStatus(.{ 127, 0, 0, 1 }, 65001, 65002, .{ 10, 0, 0, 1 }, 0);
    try std.testing.expect(status.state != .established);
}

test "session init creates idle session" {
    const config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = null,
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{types.Ipv4Prefix.init("10.0.0.0/8")},
        .same_as = true,
    };
    var fake = session.FakeTransport.init(std.testing.allocator, &.{});
    defer fake.deinit();
    const sess = try session.init(config, &fake.toTransport());
    try std.testing.expectEqual(session.SessionState.idle, sess.status.state);
    try std.testing.expectEqual(@as(u16, 65001), sess.status.local_as);
    try std.testing.expectEqual(@as(u16, 65002), sess.status.peer_as);
}

test "SessionConfig validation rejects zero peer_port" {
    const config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 0,
        .local_address = null,
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{types.Ipv4Prefix.init("10.0.0.0/8")},
        .same_as = true,
    };
    try std.testing.expectError(session.SessionErrorKind.InvalidConfig, session.validateConfig(config));
}

test "SessionConfig validation accepts empty prefixes for zero-prefix smoke test" {
    const config = session.SessionConfig{
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
    // Zero prefixes is valid - allows OPEN/KEEPALIVE-only smoke test without route advertisement
    try std.testing.expect(try session.validateConfig(config) == {});
}

test "SessionConfig validation rejects invalid hold_time" {
    const config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = null,
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 2,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{types.Ipv4Prefix.init("10.0.0.0/8")},
        .same_as = true,
    };
    try std.testing.expectError(session.SessionErrorKind.InvalidConfig, session.validateConfig(config));
}

test "getStatus returns current status" {
    const config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = null,
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{types.Ipv4Prefix.init("10.0.0.0/8")},
        .same_as = true,
    };
    var fake = session.FakeTransport.init(std.testing.allocator, &.{});
    defer fake.deinit();
    var sess = try session.init(config, &fake.toTransport());
    const status = session.getStatus(&sess);
    try std.testing.expectEqual(session.SessionState.idle, status.state);
    try std.testing.expectEqualSlices(u8, &.{ 127, 0, 0, 1 }, &status.peer_address);
    try std.testing.expectEqual(@as(u16, 65001), status.local_as);
    try std.testing.expectEqual(@as(u16, 65002), status.peer_as);
}

// === Transport Send Error Tests ===

/// A fake transport that always fails on send.
const FailingFakeTransport = struct {
    const Self = @This();

    allocator: std.mem.Allocator,
    closed: bool,

    pub fn init(allocator: std.mem.Allocator) Self {
        return Self{
            .allocator = allocator,
            .closed = false,
        };
    }

    /// Always returns error to simulate failed TCP send.
    pub fn send(self: *Self, data: []const u8) session.TransportError!void {
        _ = data;
        _ = self;
        return session.TransportError.SendFailed;
    }

    pub fn recv(self: *Self) []const u8 {
        _ = self;
        return &[_]u8{};
    }

    pub fn close(self: *Self) void {
        self.closed = true;
    }

    pub fn toTransport(self: *Self) session.Transport {
        return session.Transport{
            .sendFn = struct {
                fn send(ctx: *anyopaque, data: []const u8) session.TransportError!void {
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

test "failed OPEN send leaves session in failed state, not open_sent" {
    const config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = null,
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{types.Ipv4Prefix.init("10.0.0.0/8")},
        .same_as = true,
    };

    var fake = FailingFakeTransport.init(std.testing.allocator);
    var sess = try session.init(config, &fake.toTransport());

    // Attempt to run from idle - OPEN send will fail
    const result = session.runOnce(&sess);

    // Should return error because send failed
    try std.testing.expectError(session.SessionErrorKind.IoError, result);

    // Session should be in failed state, NOT open_sent
    try std.testing.expectEqual(session.SessionState.failed, sess.status.state);

    // messages_sent should still be 0 - OPEN was never sent
    try std.testing.expectEqual(@as(u64, 0), sess.status.messages_sent);

    // last_error should be set
    try std.testing.expect(sess.status.last_error != null);
}

test "successful OPEN send transitions to open_sent with captured frame" {
    const config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = null,
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{types.Ipv4Prefix.init("10.0.0.0/8")},
        .same_as = true,
    };

    var fake = session.FakeTransport.init(std.testing.allocator, &.{});
    defer fake.deinit();
    var sess = try session.init(config, &fake.toTransport());

    // Run from idle - should send OPEN
    const result = try session.runOnce(&sess);
    try std.testing.expectEqual(session.RunResult.ok, result);

    // Session should be in open_sent state
    try std.testing.expectEqual(session.SessionState.open_sent, sess.status.state);

    // messages_sent should be 1
    try std.testing.expectEqual(@as(u64, 1), sess.status.messages_sent);

    // Fake transport should have captured a non-empty BGP OPEN frame
    const sent = fake.getSent();
    try std.testing.expect(sent.len > 0);

    // BGP OPEN starts with 0xFF marker (RFC 4271 Section 4.1)
    try std.testing.expectEqual(@as(u8, 0xFF), sent[0]);
    try std.testing.expectEqual(@as(u8, 0xFF), sent[1]);
    try std.testing.expectEqual(@as(u8, 0xFF), sent[2]);
    try std.testing.expectEqual(@as(u8, 0xFF), sent[3]);
    try std.testing.expectEqual(@as(u8, 0xFF), sent[4]);
    try std.testing.expectEqual(@as(u8, 0xFF), sent[5]);
    try std.testing.expectEqual(@as(u8, 0xFF), sent[6]);
    try std.testing.expectEqual(@as(u8, 0xFF), sent[7]);
    try std.testing.expectEqual(@as(u8, 0xFF), sent[8]);
    try std.testing.expectEqual(@as(u8, 0xFF), sent[9]);
    try std.testing.expectEqual(@as(u8, 0xFF), sent[10]);
    try std.testing.expectEqual(@as(u8, 0xFF), sent[11]);
    try std.testing.expectEqual(@as(u8, 0xFF), sent[12]);
    try std.testing.expectEqual(@as(u8, 0xFF), sent[13]);
    try std.testing.expectEqual(@as(u8, 0xFF), sent[14]);
    try std.testing.expectEqual(@as(u8, 0xFF), sent[15]);

    // Message type should be OPEN (1)
    try std.testing.expectEqual(@as(u8, 1), sent[18]);
}
