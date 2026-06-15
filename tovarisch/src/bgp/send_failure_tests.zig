// send_failure_tests.zig — BGP send failure propagation tests
//
// ACT: Preserve exact BGP TCP send failure reason.
// Tests prove that concrete TransportError variants propagate from
// transport → session → status.last_error.message.
//
// These tests are extracted from session_tests.zig to satisfy
// LLM-friendliness line limits.

const std = @import("std");
const session = @import("session.zig");
const types = @import("types.zig");

// ============================================================================
// Test Helpers
// ============================================================================

/// A fake transport that fails on send with a configurable error.
const FailingFakeTransport = struct {
    const Self = @This();

    allocator: std.mem.Allocator,
    closed: bool,
    /// Configurable error to return on send
    error_to_return: session.TransportError,

    pub fn init(allocator: std.mem.Allocator, err: session.TransportError) Self {
        return Self{
            .allocator = allocator,
            .closed = false,
            .error_to_return = err,
        };
    }

    /// Returns configurable error to simulate specific TCP send failure.
    pub fn send(self: *Self, data: []const u8) session.TransportError!void {
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

/// Creates a minimal session config for testing.
fn makeTestConfig() session.SessionConfig {
    return session.SessionConfig{
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
}

// ============================================================================
// Send Failure Propagation Tests
// ============================================================================

test "failed OPEN send leaves session in failed state, not open_sent" {
    var fake = FailingFakeTransport.init(std.testing.allocator, session.TransportError.SendFailed);
    var sess = try session.init(makeTestConfig(), &fake.toTransport());

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
    try std.testing.expectEqualStrings("send failed", sess.status.last_error.?.message);
}

test "ConnectionReset send failure preserves ECONNRESET in error message" {
    var fake = FailingFakeTransport.init(std.testing.allocator, session.TransportError.ConnectionReset);
    var sess = try session.init(makeTestConfig(), &fake.toTransport());

    const result = session.runOnce(&sess);

    try std.testing.expectError(session.SessionErrorKind.IoError, result);
    try std.testing.expectEqual(session.SessionState.failed, sess.status.state);
    try std.testing.expect(sess.status.last_error != null);
    try std.testing.expectEqualStrings("send: ECONNRESET", sess.status.last_error.?.message);
}

test "BrokenPipe send failure preserves EPIPE in error message" {
    var fake = FailingFakeTransport.init(std.testing.allocator, session.TransportError.BrokenPipe);
    var sess = try session.init(makeTestConfig(), &fake.toTransport());

    const result = session.runOnce(&sess);

    try std.testing.expectError(session.SessionErrorKind.IoError, result);
    try std.testing.expectEqual(session.SessionState.failed, sess.status.state);
    try std.testing.expect(sess.status.last_error != null);
    try std.testing.expectEqualStrings("send: EPIPE", sess.status.last_error.?.message);
}

test "NotConnected send failure preserves ENOTCONN in error message" {
    var fake = FailingFakeTransport.init(std.testing.allocator, session.TransportError.NotConnected);
    var sess = try session.init(makeTestConfig(), &fake.toTransport());

    const result = session.runOnce(&sess);

    try std.testing.expectError(session.SessionErrorKind.IoError, result);
    try std.testing.expectEqual(session.SessionState.failed, sess.status.state);
    try std.testing.expect(sess.status.last_error != null);
    try std.testing.expectEqualStrings("send: ENOTCONN", sess.status.last_error.?.message);
}

test "BadFileDescriptor send failure preserves EBADF in error message" {
    var fake = FailingFakeTransport.init(std.testing.allocator, session.TransportError.BadFileDescriptor);
    var sess = try session.init(makeTestConfig(), &fake.toTransport());

    const result = session.runOnce(&sess);

    try std.testing.expectError(session.SessionErrorKind.IoError, result);
    try std.testing.expectEqual(session.SessionState.failed, sess.status.state);
    try std.testing.expect(sess.status.last_error != null);
    try std.testing.expectEqualStrings("send: EBADF", sess.status.last_error.?.message);
}

test "WouldBlock send failure preserves EAGAIN in error message" {
    var fake = FailingFakeTransport.init(std.testing.allocator, session.TransportError.WouldBlock);
    var sess = try session.init(makeTestConfig(), &fake.toTransport());

    const result = session.runOnce(&sess);

    try std.testing.expectError(session.SessionErrorKind.IoError, result);
    try std.testing.expectEqual(session.SessionState.failed, sess.status.state);
    try std.testing.expect(sess.status.last_error != null);
    try std.testing.expectEqualStrings("send: EAGAIN/EWOULDBLOCK", sess.status.last_error.?.message);
}
