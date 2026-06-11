// tcp_transport.zig — Real TCP transport for BGP session
//
// ACT 3: Real TCP transport adapter for the BGP session state machine.
// This module provides real TCP socket transport that conforms to the
// existing Transport interface used by bgp/session.zig.
//
// Helper functions have been extracted to tcp_transport_helpers.zig.

const std = @import("std");
const transport = @import("transport.zig");
const h = @import("tcp_transport_helpers.zig");

/// Re-export TransportError for convenience
pub const TransportError = h.TransportError;

// ============================================================================
// Configuration
// ============================================================================

/// Configuration for TcpTransport
pub const TcpTransportConfig = struct {
    /// Peer's IPv4 address
    peer_address: [4]u8,
    /// Peer's BGP port
    peer_port: u16,
    /// Our local IPv4 address (null = let OS pick)
    local_address: ?[4]u8,
    /// Connection timeout in milliseconds.
    connect_timeout_ms: u32,
};

// ============================================================================
// TcpTransport
// ============================================================================

/// Real TCP transport for BGP sessions.
/// Maintains a connected TCP socket and provides the Transport interface.
pub const TcpTransport = struct {
    const Self = @This();

    /// Connected socket file descriptor (-1 = closed)
    socket_fd: std.c.fd_t,
    /// Receive buffer for socket reads
    recv_buf: [4096]u8,
    /// Current fill position in recv_buf
    recv_len: usize,
    /// Whether transport is closed
    closed: bool,
    /// Peer address (for diagnostics)
    peer_address: [4]u8,
    /// Peer port
    peer_port: u16,

    /// Initialize a TCP transport and connect to peer with bounded timeout.
    pub fn connect(config: TcpTransportConfig) !Self {
        var self = Self{
            .socket_fd = -1,
            .recv_buf = undefined,
            .recv_len = 0,
            .closed = false,
            .peer_address = config.peer_address,
            .peer_port = config.peer_port,
        };

        // Create TCP socket
        self.socket_fd = std.c.socket(h.AF_INET, h.SOCK_STREAM, 0);
        if (self.socket_fd < 0) {
            return error.ConnectionFailed;
        }
        errdefer {
            if (self.socket_fd >= 0) {
                _ = std.c.close(self.socket_fd);
                self.socket_fd = -1;
            }
        }

        // Optionally bind to local address
        if (config.local_address) |local| {
            try h.bindToLocalAddress(self.socket_fd, local);
        }

        try h.setNonBlocking(self.socket_fd);

        // Build peer address
        var peer_addr: h.sockaddr_in = h.sockaddr_in{
            .sin_family = @as(c_ushort, @intCast(h.AF_INET)),
            .sin_port = h.writePortToSockaddr(config.peer_port),
            .sin_addr = h.writeIpv4ToSockaddr(config.peer_address),
            .sin_zero = undefined,
        };
        @memset(peer_addr.sin_zero[0..], 0);

        const addr_ptr: *const std.c.sockaddr = @ptrCast(&peer_addr);
        const addr_len: h.socklen_t = @sizeOf(h.sockaddr_in);
        const result = std.c.connect(self.socket_fd, addr_ptr, addr_len);
        if (result < 0) {
            const err = std.c._errno().*;
            if (err != @intFromEnum(std.c.E.INPROGRESS)) {
                return error.ConnectionFailed;
            }
            try h.waitConnectWritable(self.socket_fd, config.connect_timeout_ms);
            try h.checkSocketError(self.socket_fd);
        }

        return self;
    }

    /// Send bytes through the TCP socket.
    pub fn send(self: *Self, data: []const u8) TransportError!void {
        if (self.closed or self.socket_fd < 0) return TransportError.Closed;

        var offset: usize = 0;
        while (offset < data.len) {
            const remaining = data[offset..];
            const sent = std.c.send(self.socket_fd, @ptrCast(remaining.ptr), remaining.len, 0);
            if (sent < 0) {
                self.closed = true;
                const errno = std.c._errno().*;
                return h.mapSendError(errno);
            }
            if (sent == 0) {
                self.closed = true;
                return TransportError.ConnectionClosed;
            }
            offset += @as(usize, @intCast(sent));
        }
    }

    /// Receive bytes from the TCP socket.
    pub fn recv(self: *Self) []const u8 {
        if (self.closed or self.socket_fd < 0) return &[_]u8{};

        if (self.recv_len > 0) {
            const data = self.recv_buf[0..self.recv_len];
            self.recv_len = 0;
            return data;
        }

        const ready = h.waitForData(self.socket_fd, h.default_recv_timeout_ms);
        switch (ready) {
            .ready => {},
            .timeout => return &[_]u8{},
            .hung_up, .socket_error, .invalid => {
                self.closed = true;
                return &[_]u8{};
            },
        }

        const received = std.c.recv(self.socket_fd, @ptrCast(&self.recv_buf), self.recv_buf.len, 0);
        if (received < 0) {
            return &[_]u8{};
        }
        if (received == 0) {
            self.closed = true;
            return &[_]u8{};
        }

        self.recv_len = @as(usize, @intCast(received));
        return self.recv_buf[0..self.recv_len];
    }

    /// Close the TCP socket cleanly.
    pub fn close(self: *Self) void {
        if (self.socket_fd >= 0) {
            _ = std.c.close(self.socket_fd);
            self.socket_fd = -1;
        }
        self.closed = true;
        self.recv_len = 0;
    }

    /// Wrap this TCP transport as a Transport interface.
    pub fn toTransport(self: *Self) transport.Transport {
        return transport.Transport{
            .sendFn = struct {
                fn send(ctx: *anyopaque, data: []const u8) TransportError!void {
                    const tcp: *Self = @ptrCast(@alignCast(ctx));
                    return tcp.send(data);
                }
            }.send,
            .recvFn = struct {
                fn recv(ctx: *anyopaque) []const u8 {
                    const tcp: *Self = @ptrCast(@alignCast(ctx));
                    return tcp.recv();
                }
            }.recv,
            .closeFn = struct {
                fn close(ctx: *anyopaque) void {
                    const tcp: *Self = @ptrCast(@alignCast(ctx));
                    tcp.close();
                }
            }.close,
            .ctx = @ptrCast(self),
        };
    }
};
