// transport.zig — BGP transport interface
//
// ACT 2: Transport interface for BGP session testing.
// Abstracts the socket layer for testability.

const std = @import("std");

/// Transport send errors.
/// BGP FSM state transitions depend on successful writes.
/// Concrete variants preserve exact socket failure reason for observability.
pub const TransportError = error{
    /// Transport is closed
    Closed,
    /// Peer closed connection (zero-length send)
    ConnectionClosed,
    /// Memory allocation failed
    OutOfMemory,
    /// WouldBlock: socket non-blocking and buffer full (EAGAIN/EWOULDBLOCK)
    WouldBlock,
    /// ConnectionReset: peer sent RST (ECONNRESET)
    ConnectionReset,
    /// BrokenPipe: write on closed pipe/Socket (EPIPE)
    BrokenPipe,
    /// NotConnected: socket not connected (ENOTCONN)
    NotConnected,
    /// BadFileDescriptor: invalid socket fd (EBADF)
    BadFileDescriptor,
    /// SendFailed: unspecified send failure
    SendFailed,
};

/// Transport interface for BGP message send/receive.
/// This abstraction allows testing with fake transports and
/// production use with real TCP sockets.
pub const Transport = struct {
    /// Send bytes to the peer (returns error on failure)
    sendFn: *const fn (ctx: *anyopaque, data: []const u8) TransportError!void,
    /// Receive bytes from the peer (returns data or empty slice if no data)
    recvFn: *const fn (ctx: *anyopaque) []const u8,
    /// Close the transport
    closeFn: *const fn (ctx: *anyopaque) void,
    /// Context pointer
    ctx: *anyopaque,
};

/// Send data through transport.
pub fn transportSend(trans: *const Transport, data: []const u8) TransportError!void {
    return trans.sendFn(trans.ctx, data);
}

/// Receive data from transport.
pub fn transportRecv(trans: *const Transport) []const u8 {
    return trans.recvFn(trans.ctx);
}

/// Close transport.
pub fn transportClose(trans: *const Transport) void {
    trans.closeFn(trans.ctx);
}

// ============================================================================
// Fake Transport (for scripted testing)
// ============================================================================

/// Scripted peer response for testing BGP handshake.
pub const PeerResponse = struct {
    /// Bytes to return when session reads
    recv_bytes: []const u8,
    /// Expected sent bytes (validated after send)
    expected_sent: ?[]const u8 = null,
};

/// Fake transport for scripted BGP testing.
/// Scripts the full OPEN/KEEPALIVE/UPDATE handshake.
pub const FakeTransport = struct {
    const Self = @This();

    /// Allocator for memory operations
    allocator: std.mem.Allocator,
    /// Scripted responses (index advances as session reads)
    responses: []const PeerResponse,
    /// Current response index
    response_idx: usize,
    /// Captured sent bytes
    captured_sent: std.ArrayList(u8),
    /// All sent bytes concatenated (for validation)
    all_sent: std.ArrayList(u8),
    /// Whether transport is closed
    closed: bool,

    /// Create a fake transport with a scripted handshake.
    pub fn init(allocator: std.mem.Allocator, responses: []const PeerResponse) !Self {
        const captured = try std.ArrayList(u8).initCapacity(allocator, 0);
        const all = try std.ArrayList(u8).initCapacity(allocator, 0);
        return Self{
            .allocator = allocator,
            .responses = responses,
            .response_idx = 0,
            .captured_sent = captured,
            .all_sent = all,
            .closed = false,
        };
    }

    /// Send bytes (capture them).
    /// For fake transport, sends always succeed.
    pub fn send(self: *Self, data: []const u8) TransportError!void {
        try self.captured_sent.appendSlice(self.allocator, data);
        try self.all_sent.appendSlice(self.allocator, data);
    }

    /// Receive bytes (return scripted response).
    pub fn recv(self: *Self) []const u8 {
        if (self.response_idx >= self.responses.len) {
            return &[_]u8{};
        }
        const resp = self.responses[self.response_idx];
        self.response_idx += 1;
        return resp.recv_bytes;
    }

    /// Close the transport.
    pub fn close(self: *Self) void {
        self.closed = true;
    }

    /// Get captured sent bytes.
    pub fn getSent(self: *const Self) []const u8 {
        return self.captured_sent.items;
    }

    /// Get all sent bytes as a single slice.
    pub fn getAllSent(self: *const Self) []const u8 {
        return self.all_sent.items;
    }

    /// Get the last n bytes sent.
    /// Returns empty slice if fewer than n bytes were sent.
    pub fn lastSentBytes(self: *const Self, n: usize) []const u8 {
        const all = self.all_sent.items;
        if (all.len < n) return &[_]u8{};
        return all[all.len - n ..];
    }

    /// Deinit and free memory.
    pub fn deinit(self: *Self) void {
        self.captured_sent.deinit(self.allocator);
        self.all_sent.deinit(self.allocator);
    }

    /// Wrap this fake transport as a Transport interface.
    pub fn toTransport(self: *Self) Transport {
        return Transport{
            .sendFn = struct {
                fn send(ctx: *anyopaque, data: []const u8) TransportError!void {
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
