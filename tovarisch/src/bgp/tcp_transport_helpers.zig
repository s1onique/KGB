// tcp_transport_helpers.zig — TCP transport helper functions
//
// Helper functions for TcpTransport including socket setup, polling,
// connection verification, and errno mapping.
//
// This file is extracted from tcp_transport.zig to satisfy LLM-friendliness
// line limits.

const std = @import("std");
const transport = @import("transport.zig");

/// Re-export TransportError for convenience
pub const TransportError = transport.TransportError;

// ============================================================================
// Socket Constants
// ============================================================================

/// Address family for IPv4
pub const AF_INET: c_int = 2;

/// TCP socket type
pub const SOCK_STREAM: c_int = 1;

/// O_NONBLOCK flag for non-blocking socket
pub const O_NONBLOCK: c_int = 4;

/// POLLOUT flag for poll - socket is writable (connect completed or failed)
pub const POLLOUT: c_short = 0x004;

/// POLLHUP = 0x010 (peer closed), POLLERR = 0x008 (error), POLLIN = 0x001 (read), POLLNVAL = 0x020 (invalid)
const POLLHUP: c_short = 0x010;
const POLLERR: c_short = 0x008;
const POLLIN: c_short = 0x001;
const POLLNVAL: c_short = 0x020;

/// Default connect timeout in milliseconds.
pub const default_connect_timeout_ms: u32 = 5000;

/// Default receive timeout in milliseconds.
pub const default_recv_timeout_ms: u32 = 100;

// ============================================================================
// Socket Address Structures
// ============================================================================

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
// Byte Order Helpers (for testing without sockets)
// ============================================================================

/// Write IPv4 address bytes directly into sockaddr memory in network byte order.
pub fn writeIpv4ToSockaddr(addr: [4]u8) in_addr {
    var out: in_addr = undefined;
    const s_addr_bytes = std.mem.asBytes(&out.s_addr);
    s_addr_bytes[0] = addr[0];
    s_addr_bytes[1] = addr[1];
    s_addr_bytes[2] = addr[2];
    s_addr_bytes[3] = addr[3];
    return out;
}

/// Write port number directly into sockaddr memory in network byte order.
pub fn writePortToSockaddr(port: u16) u16 {
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
// Helper Functions
// ============================================================================

/// Map errno to concrete TransportError variant.
/// Preserves exact socket failure reason for observability.
pub fn mapSendError(errno: c_int) TransportError {
    // EAGAIN/EWOULDBLOCK: socket buffer full (non-blocking)
    if (errno == @intFromEnum(std.c.E.AGAIN)) return TransportError.WouldBlock;
    // ECONNRESET: peer sent RST
    if (errno == @intFromEnum(std.c.E.CONNRESET)) return TransportError.ConnectionReset;
    // EPIPE: write on closed pipe/Socket
    if (errno == @intFromEnum(std.c.E.PIPE)) return TransportError.BrokenPipe;
    // ENOTCONN: socket not connected
    if (errno == @intFromEnum(std.c.E.NOTCONN)) return TransportError.NotConnected;
    // EBADF: invalid socket fd
    if (errno == @intFromEnum(std.c.E.BADF)) return TransportError.BadFileDescriptor;
    // Fallback: unspecified send failure
    return TransportError.SendFailed;
}

/// Bind socket to local address if specified.
pub fn bindToLocalAddress(sockfd: std.c.fd_t, local_addr: [4]u8) !void {
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
pub fn setNonBlocking(sockfd: std.c.fd_t) !void {
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

/// Result of waiting for socket data.
pub const DataReady = enum(u3) {
    ready,
    timeout,
    hung_up,
    socket_error,
    invalid,
};

/// Wait for data with bounded timeout. Returns DataReady to allow proper error handling.
pub fn waitForData(sockfd: std.c.fd_t, timeout_ms: u32) DataReady {
    var poll_fd: [1]std.c.pollfd = .{
        .{ .fd = sockfd, .events = POLLIN, .revents = 0 },
    };
    const r = std.c.poll(&poll_fd, 1, @as(i32, @intCast(timeout_ms)));
    if (r < 0) return .socket_error;
    if (r == 0) return .timeout;
    const revents = poll_fd[0].revents;
    if ((revents & POLLNVAL) != 0) return .invalid;
    if ((revents & POLLERR) != 0) return .socket_error;
    if ((revents & POLLHUP) != 0) return .hung_up;
    if ((revents & POLLIN) != 0) return .ready;
    return .timeout;
}

/// Wait for writable with timeout. POLLOUT detects connect completion.
pub fn waitConnectWritable(sockfd: std.c.fd_t, timeout_ms: u32) !void {
    var poll_fd: [1]std.c.pollfd = .{.{ .fd = sockfd, .events = POLLOUT, .revents = 0 }};
    const r = std.c.poll(&poll_fd, 1, @as(i32, @intCast(timeout_ms)));
    if (r < 0) return error.PollFailed;
    if (r == 0) return error.Timeout;
    const revents = poll_fd[0].revents;
    if ((revents & POLLOUT) != 0 or (revents & POLLERR) != 0 or (revents & POLLHUP) != 0) return;
    return error.PollFailed;
}

/// Check SO_ERROR on a connected socket to determine if connect succeeded.
pub fn checkSocketError(sockfd: std.c.fd_t) !void {
    const SOL_SOCKET: c_int = 1;
    const SO_ERROR: c_int = 4;

    var error_val: c_int = 0;
    var error_len: socklen_t = @sizeOf(c_int);

    const result = std.c.getsockopt(sockfd, SOL_SOCKET, SO_ERROR, @ptrCast(&error_val), &error_len);
    if (result < 0) {
        return checkConnectionViaSend(sockfd);
    }

    if (error_val != 0) {
        return error.ConnectionFailed;
    }
}

/// Fallback connection verification using nonblocking send.
fn checkConnectionViaSend(sockfd: std.c.fd_t) !void {
    const result = std.c.send(sockfd, "", 0, 0);
    if (result < 0) {
        const err = std.c._errno().*;
        if (err == @intFromEnum(std.c.E.AGAIN)) {
            return; // Connection is good, just waiting on buffer space
        }
        return error.ConnectionFailed;
    }
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
