// tcp_transport.zig — Real TCP transport for BGP session
//
// ACT 3: Real TCP transport adapter for the BGP session state machine.
// This module provides real TCP socket transport that conforms to the
// existing Transport interface used by bgp/session.zig.
//
// NOT wired into production daemon runtime yet.
//
// receive semantics: non-blocking. Returns empty slice when no data available.
// send semantics: send errors (including EAGAIN) currently close the transport.
//                 TODO: handle EAGAIN without closing for better nonblocking behavior.
//
// Connect semantics: bounded non-blocking connect with configurable timeout.
// Uses O_NONBLOCK + poll() + SO_ERROR to bound connection attempts.

const std = @import("std");
const transport = @import("transport.zig");

// ============================================================================
// Socket Constants and Types
// ============================================================================

/// Address family for IPv4
pub const AF_INET: c_int = 2;

/// TCP socket type
pub const SOCK_STREAM: c_int = 1;

/// O_NONBLOCK flag for non-blocking socket
pub const O_NONBLOCK: c_int = 4;

/// POLLOUT flag for poll - socket is writable (connect completed or failed)
pub const POLLOUT: c_short = 0x004;

/// Default connect timeout in milliseconds.
/// This bounds TCP connection establishment to prevent indefinite hangs
/// on refused, filtered, or blackholed peers.
pub const default_connect_timeout_ms: u32 = 5000;

/// sockaddr_in - IPv4 socket address structure (network byte order)
pub const sockaddr_in = extern struct {
    sin_family: c_ushort,
    sin_port: c_ushort,
    sin_addr: in_addr,
    sin_zero: [8]u8,
};

/// in_addr - IPv4 address (4 bytes in network byte order)
pub const in_addr = extern struct {
    s_addr: c_uint,
};

/// socklen_t type
pub const socklen_t = std.c.socklen_t;

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
    /// Bounded by poll() with this timeout during nonblocking connect.
    connect_timeout_ms: u32,
};

// ============================================================================
// Helper Functions (for testing byte order without sockets)
// ============================================================================

/// Write IPv4 address bytes directly into sockaddr memory in network byte order.
/// This ensures correct memory layout on both little-endian and big-endian hosts.
/// 127.0.0.1 -> sin_addr bytes: 7f 00 00 01
pub fn writeIpv4ToSockaddr(addr: [4]u8) in_addr {
    var out: in_addr = undefined;
    const s_addr_bytes = std.mem.asBytes(&out.s_addr);
    // Write in big-endian (network byte order) directly to memory
    s_addr_bytes[0] = addr[0];
    s_addr_bytes[1] = addr[1];
    s_addr_bytes[2] = addr[2];
    s_addr_bytes[3] = addr[3];
    return out;
}

/// Write port number directly into sockaddr memory in network byte order.
/// Port 179 -> sin_port bytes: 00 b3 (big-endian)
pub fn writePortToSockaddr(port: u16) u16 {
    // Store in big-endian (network byte order)
    return @byteSwap(port);
}

/// Read IPv4 address from sockaddr memory.
pub fn readIpv4FromSockaddr(in: in_addr) [4]u8 {
    const s_addr_bytes = std.mem.asBytes(&in.s_addr);
    return [4]u8{
        s_addr_bytes[0],
        s_addr_bytes[1],
        s_addr_bytes[2],
        s_addr_bytes[3],
    };
}

/// Read port from sockaddr memory.
pub fn readPortFromSockaddr(port: u16) u16 {
    return @byteSwap(port);
}

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
    /// Uses nonblocking connect + poll() + SO_ERROR to bound connection attempts.
    /// Returns error on connection failure or timeout.
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
        self.socket_fd = std.c.socket(AF_INET, SOCK_STREAM, 0);
        if (self.socket_fd < 0) {
            return error.ConnectionFailed;
        }
        // Guaranteed socket cleanup on all error paths
        errdefer {
            if (self.socket_fd >= 0) {
                _ = std.c.close(self.socket_fd);
                self.socket_fd = -1;
            }
        }

        // Optionally bind to local address
        if (config.local_address) |local| {
            try bindToLocalAddress(self.socket_fd, local);
        }

        // Set socket to non-blocking mode BEFORE connect.
        // This is critical: connect returns immediately with EINPROGRESS
        // on nonblocking sockets, allowing us to poll for completion.
        try setNonBlocking(self.socket_fd);

        // Build peer address - write bytes directly to ensure correct memory layout
        var peer_addr: sockaddr_in = sockaddr_in{
            .sin_family = @as(c_ushort, @intCast(AF_INET)),
            .sin_port = writePortToSockaddr(config.peer_port),
            .sin_addr = writeIpv4ToSockaddr(config.peer_address),
            .sin_zero = undefined,
        };
        @memset(peer_addr.sin_zero[0..], 0);

        // Connect to peer - nonblocking so this returns immediately
        const addr_ptr: *const std.c.sockaddr = @ptrCast(&peer_addr);
        const addr_len: socklen_t = @sizeOf(sockaddr_in);
        const result = std.c.connect(self.socket_fd, addr_ptr, addr_len);
        if (result < 0) {
            const err = std.c._errno().*;
            // EINPROGRESS means the connect is in progress - this is expected
            // for nonblocking sockets. We need to poll for completion.
            if (err != @intFromEnum(std.c.E.INPROGRESS)) {
                return error.ConnectionFailed;
            }
            // Connect is in progress - poll for writability with timeout
            try waitConnectWritable(self.socket_fd, config.connect_timeout_ms);

            // After poll returns, check SO_ERROR to see if connect succeeded or failed.
            // On macOS, SO_ERROR may return -1 or unexpected values for successful connects,
            // so we also check for immediate success via getsockopt error.
            try checkSocketError(self.socket_fd);
        }

        return self;
    }

    /// Send bytes through the TCP socket.
    /// Handles partial sends. Marks closed on any send error.
    /// Note: Currently EAGAIN closes transport. TODO: handle EAGAIN without closing.
    pub fn send(self: *Self, data: []const u8) void {
        if (self.closed or self.socket_fd < 0) return;

        var offset: usize = 0;
        while (offset < data.len) {
            const remaining = data[offset..];
            const sent = std.c.send(self.socket_fd, @ptrCast(remaining.ptr), remaining.len, 0);
            if (sent < 0) {
                // Error occurred - currently mark closed on any error
                // TODO: distinguish EAGAIN/EWOULDBLOCK for retry without closing
                self.closed = true;
                return;
            }
            if (sent == 0) {
                // Zero send means peer closed connection
                self.closed = true;
                return;
            }
            offset += @as(usize, @intCast(sent));
        }
    }

    /// Receive bytes from the TCP socket.
    /// Non-blocking: returns empty slice when no data available.
    /// Marks closed only on peer close (recv==0) or fatal error.
    pub fn recv(self: *Self) []const u8 {
        if (self.closed or self.socket_fd < 0) return &[_]u8{};

        // Return buffered data first
        if (self.recv_len > 0) {
            const data = self.recv_buf[0..self.recv_len];
            self.recv_len = 0;
            return data;
        }

        // Try recv
        const received = std.c.recv(self.socket_fd, @ptrCast(&self.recv_buf), self.recv_buf.len, 0);
        if (received < 0) {
            // Error occurred - for non-blocking, this means try again later
            return &[_]u8{};
        }
        if (received == 0) {
            // Peer closed connection
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
                fn send(ctx: *anyopaque, data: []const u8) void {
                    const tcp: *Self = @ptrCast(@alignCast(ctx));
                    tcp.send(data);
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

// ============================================================================
// Helper Functions
// ============================================================================

/// Bind socket to local address if specified.
fn bindToLocalAddress(sockfd: std.c.fd_t, local_addr: [4]u8) !void {
    var addr: sockaddr_in = sockaddr_in{
        .sin_family = @as(c_ushort, @intCast(AF_INET)),
        .sin_port = writePortToSockaddr(0), // Let OS pick ephemeral port
        .sin_addr = writeIpv4ToSockaddr(local_addr),
        .sin_zero = undefined,
    };
    @memset(addr.sin_zero[0..], 0);

    const addr_ptr: *const std.c.sockaddr = @ptrCast(&addr);
    const addr_len: socklen_t = @sizeOf(sockaddr_in);
    if (std.c.bind(sockfd, addr_ptr, addr_len) < 0) {
        return error.BindFailed;
    }
}

/// Set socket to non-blocking mode.
/// F_GETFL = 3, F_SETFL = 4
fn setNonBlocking(sockfd: std.c.fd_t) !void {
    const F_GETFL: c_int = 3;
    const F_SETFL: c_int = 4;
    const zero: c_int = 0;

    // Get current flags
    const flags = std.c.fcntl(sockfd, F_GETFL, zero);
    if (flags < 0) return error.FcntlFailed;

    // Set non-blocking flag
    const new_flags: c_int = flags | O_NONBLOCK;
    if (std.c.fcntl(sockfd, F_SETFL, new_flags) < 0) {
        return error.FcntlFailed;
    }
}

/// POLLHUP flag - peer closed connection (but connect may still have succeeded)
const POLLHUP: c_short = 0x020;

/// POLLERR flag - error condition on socket
const POLLERR: c_short = 0x008;

/// Wait for socket to be writable (connect completed or failed) with timeout.
/// Uses poll() with POLLOUT to detect when the nonblocking connect completes.
/// Returns error.Timeout on poll timeout, error.PollFailed on poll error.
fn waitConnectWritable(sockfd: std.c.fd_t, timeout_ms: u32) !void {
    var poll_fd: [1]std.c.pollfd = .{
        .{ .fd = sockfd, .events = POLLOUT, .revents = 0 },
    };

    const poll_result = std.c.poll(&poll_fd, 1, @as(i32, @intCast(timeout_ms)));
    if (poll_result < 0) {
        return error.PollFailed;
    }
    if (poll_result == 0) {
        // Timeout - connection did not complete within the timeout period
        return error.Timeout;
    }
    // poll_result > 0 means socket had an event
    // Check if POLLOUT is set (connect completed or succeeded) OR
    // POLLERR/POLLHUP indicate completion (even if failed)
    const revents = poll_fd[0].revents;
    if ((revents & POLLOUT) != 0 or (revents & POLLERR) != 0 or (revents & POLLHUP) != 0) {
        // Connect completed (success or failure) - caller should check SO_ERROR
        return;
    }
    return error.PollFailed;
}

/// Check SO_ERROR on a connected socket to determine if connect succeeded.
/// Called after poll() returns - meaning the connect operation completed.
/// Returns error.ConnectionFailed if the underlying connect failed.
fn checkSocketError(sockfd: std.c.fd_t) !void {
    const SOL_SOCKET: c_int = 1;
    const SO_ERROR: c_int = 4;

    var error_val: c_int = 0;
    var error_len: socklen_t = @sizeOf(c_int);

    // Get SO_ERROR socket option to check connect result
    const result = std.c.getsockopt(sockfd, SOL_SOCKET, SO_ERROR, @ptrCast(&error_val), &error_len);
    if (result < 0) {
        // On macOS, getsockopt may fail for successful connects.
        // In that case, try a zero-length send to verify the connection works.
        // If send succeeds or returns EAGAIN, connection is good.
        return checkConnectionViaSend(sockfd);
    }

    // error_val contains the connect error code:
    // 0 = success, ECONNREFUSED = refused, ETIMEDOUT = timeout, etc.
    if (error_val != 0) {
        return error.ConnectionFailed;
    }
}

/// Fallback connection verification using nonblocking send.
/// On some platforms (macOS), getsockopt SO_ERROR may not work as expected.
/// This verifies the connection is actually usable by attempting a send.
fn checkConnectionViaSend(sockfd: std.c.fd_t) !void {
    // Try a zero-length send to check if connection is alive
    const result = std.c.send(sockfd, "", 0, 0);
    if (result < 0) {
        const err = std.c._errno().*;
        // EAGAIN means socket is nonblocking and buffer is full
        // which indicates the connection IS established but can't send right now
        // Note: macOS uses EAGAIN for both EAGAIN and EWOULDBLOCK
        if (err == @intFromEnum(std.c.E.AGAIN)) {
            return; // Connection is good, just waiting on buffer space
        }
        return error.ConnectionFailed;
    }
    // send succeeded or returned 0 (both mean connection is alive)
}

/// Convert [4]u8 to IPv4 address string for debugging.
pub fn fmtPeerAddress(addr: [4]u8) [15]u8 {
    var buf: [15]u8 = undefined;
    const written = std.fmt.bufPrint(&buf, "{}.{}.{}.{}", .{
        addr[0],
        addr[1],
        addr[2],
        addr[3],
    }) catch unreachable;
    _ = written;
    return buf;
}
