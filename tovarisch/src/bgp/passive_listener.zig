// bgp/passive_listener.zig — Passive BGP TCP/179 listener
//
// ACT: Implement always-on passive BGP TCP/179 listener alongside active connect.
// The passive listener binds to the configured local_address on port 179 and
// accepts incoming BGP peer connections.
//
// Thread Safety:
// - Passive listener runs in its own thread.
// - cleanup_requested signals stop via atomic u8.
// - Listener thread joins on cleanup.
// - Accepted connections are handed to the BGP runtime for session management.
// - has_pending_accept is atomic u8 for cross-thread signaling.

const std = @import("std");
const c = std.c;
const tcp_transport_helpers = @import("tcp_transport_helpers.zig");

// ============================================================================
// Passive Listener Configuration
// ============================================================================

/// Configuration for passive BGP listener.
pub const PassiveListenerConfig = struct {
    /// Local address to bind to.
    local_address: [4]u8,
    /// Port to listen on (default 179).
    port: u16 = 179,
    /// Accept timeout in milliseconds (poll timeout for incoming connections).
    accept_timeout_ms: u32 = 1000,
    /// Allowed peer address - only connections from this peer are accepted.
    /// If null, any peer is accepted (use with caution).
    allowed_peer_address: ?[4]u8 = null,
};

/// Result of accept operation.
pub const AcceptResult = struct {
    /// Accepted TCP socket file descriptor (-1 = invalid).
    socket_fd: std.c.fd_t,
    /// Peer address.
    peer_address: [4]u8,
    /// Peer port.
    peer_port: u16,
};

/// Passive listener state for status reporting.
pub const ListenerState = enum {
    /// Listener is not created (no local_address configured).
    disabled,
    /// Listener is bound and listening.
    bound,
    /// Listener thread failed to start.
    thread_failed,
    /// Bind failed (address in use, permission denied, etc.).
    bind_failed,
};

// ============================================================================
// Passive Listener
// ============================================================================

/// Passive BGP listener that accepts incoming connections.
/// Runs in a thread, listening on local_address:port for incoming BGP peers.
///
/// THREAD SAFETY:
/// - Single-producer/single-consumer: listener thread writes accepted_* fields,
///   runtime thread reads them only after has_pending_accept=1 is observed.
/// - The has_pending_accept flag is the synchronization point - release/acquire
///   ordering on this flag ensures proper visibility of accepted_* fields.
/// - accepted_fd, accepted_peer_address, accepted_peer_port are only accessed
///   when has_pending_accept=1, which provides implicit synchronization.
pub const PassiveListener = struct {
    const Self = @This();

    /// Listening socket file descriptor (-1 = not listening)
    listen_fd: std.c.fd_t = -1,
    /// Configuration
    config: PassiveListenerConfig,
    /// Atomic flag for cleanup signaling (1 = stop requested)
    cleanup_requested: u8 = 0,
    /// Thread handle (null if not started or already joined)
    thread: ?std.Thread = null,
    /// Accepted socket file descriptor.
    /// THREAD SAFETY: Written only by listener thread; read by runtime thread
    /// only after has_pending_accept=1 is observed (release/acquire ordering).
    accepted_fd: std.c.fd_t = -1,
    /// Accepted peer address.
    /// THREAD SAFETY: Written only by listener thread; read by runtime thread
    /// only after has_pending_accept=1 is observed (release/acquire ordering).
    accepted_peer_address: [4]u8 = .{ 0, 0, 0, 0 },
    /// Accepted peer port.
    /// THREAD SAFETY: Written only by listener thread; read by runtime thread
    /// only after has_pending_accept=1 is observed (release/acquire ordering).
    accepted_peer_port: u16 = 0,
    /// Atomic flag: 1 = has pending connection.
    /// This is the synchronization point between listener and runtime threads.
    has_pending_accept: u8 = 0,
    /// Current listener state for status reporting.
    state: ListenerState = .disabled,
    /// Last error message if bind or thread failed.
    error_message: ?[]const u8 = null,
};

/// Listener errors.
pub const ListenerError = error{
    /// Failed to create socket
    SocketCreationFailed,
    /// Failed to bind to address
    BindFailed,
    /// Failed to listen
    ListenFailed,
    /// Failed to set non-blocking
    NonBlockingFailed,
    /// Listener is not open
    NotListening,
    /// No pending connection
    NoPendingConnection,
    /// Thread spawn failed
    ThreadSpawnFailed,
};

/// Create a new passive listener with configuration.
pub fn createPassiveListener(config: PassiveListenerConfig) ListenerError!PassiveListener {
    var self = PassiveListener{
        .listen_fd = -1,
        .config = config,
        .cleanup_requested = 0,
        .thread = null,
        .accepted_fd = -1,
        .has_pending_accept = 0,
        .state = .disabled,
        .error_message = null,
    };

    self.listen_fd = std.c.socket(tcp_transport_helpers.AF_INET, tcp_transport_helpers.SOCK_STREAM, 0);
    if (self.listen_fd < 0) {
        self.state = .bind_failed;
        self.error_message = "socket creation failed";
        return ListenerError.SocketCreationFailed;
    }

    errdefer {
        if (self.listen_fd >= 0) {
            _ = std.c.close(self.listen_fd);
            self.listen_fd = -1;
        }
    }

    const SOL_SOCKET: c_int = 1;
    const SO_REUSEADDR: c_int = 2;
    var reuseaddr: c_int = 1;
    _ = std.c.setsockopt(self.listen_fd, SOL_SOCKET, SO_REUSEADDR, @ptrCast(&reuseaddr), @sizeOf(c_int));

    var addr: tcp_transport_helpers.sockaddr_in = .{
        .sin_family = @as(c_ushort, @intCast(tcp_transport_helpers.AF_INET)),
        .sin_port = tcp_transport_helpers.writePortToSockaddr(self.config.port),
        .sin_addr = tcp_transport_helpers.writeIpv4ToSockaddr(self.config.local_address),
        .sin_zero = undefined,
    };
    @memset(addr.sin_zero[0..], 0);

    const addr_ptr: *const std.c.sockaddr = @ptrCast(&addr);
    const addr_len: tcp_transport_helpers.socklen_t = @sizeOf(tcp_transport_helpers.sockaddr_in);
    if (std.c.bind(self.listen_fd, addr_ptr, addr_len) < 0) {
        self.state = .bind_failed;
        self.error_message = "bind failed";
        return ListenerError.BindFailed;
    }

    if (std.c.listen(self.listen_fd, 1) < 0) {
        self.state = .bind_failed;
        self.error_message = "listen failed";
        return ListenerError.ListenFailed;
    }

    setNonBlocking(self.listen_fd) catch {
        self.state = .bind_failed;
        self.error_message = "non-blocking mode failed";
        return ListenerError.NonBlockingFailed;
    };
    self.state = .bound;
    return self;
}

/// Start the passive listener thread.
pub fn startListenerThread(self: *PassiveListener) ListenerError!void {
    if (std.Thread.spawn(.{}, listenerThread, .{self})) |thread| {
        self.thread = thread;
    } else |_| {
        self.state = .thread_failed;
        self.error_message = "thread spawn failed";
        return ListenerError.ThreadSpawnFailed;
    }
}

/// Listener thread main function.
fn listenerThread(self: *PassiveListener) void {
    while (true) {
        if (@atomicLoad(u8, &self.cleanup_requested, .acquire) != 0) {
            return;
        }

        var poll_fd: [1]std.c.pollfd = .{
            .{ .fd = self.listen_fd, .events = 0x001, .revents = 0 }, // POLLIN
        };
        const r = std.c.poll(&poll_fd, 1, @as(i32, @intCast(self.config.accept_timeout_ms)));
        if (r < 0) continue;
        if (r == 0) continue;

        const revents = poll_fd[0].revents;
        if ((revents & 0x001) == 0) continue; // POLLIN

        var client_addr: tcp_transport_helpers.sockaddr_in = undefined;
        var addr_len: tcp_transport_helpers.socklen_t = @sizeOf(tcp_transport_helpers.sockaddr_in);
        const client_fd = std.c.accept(self.listen_fd, @ptrCast(&client_addr), &addr_len);
        if (client_fd < 0) continue;

        const peer_addr = tcp_transport_helpers.readIpv4FromSockaddr(client_addr.sin_addr);
        const peer_port = tcp_transport_helpers.readPortFromSockaddr(client_addr.sin_port);

        if (self.config.allowed_peer_address) |allowed| {
            if (!std.mem.eql(u8, &peer_addr, &allowed)) {
                _ = std.c.close(client_fd);
                continue;
            }
        }

        if (@atomicLoad(u8, &self.has_pending_accept, .acquire) != 0) {
            _ = std.c.close(client_fd);
            continue;
        }

        self.accepted_fd = client_fd;
        self.accepted_peer_address = peer_addr;
        self.accepted_peer_port = peer_port;
        @atomicStore(u8, &self.has_pending_accept, 1, .release);
    }
}

/// Check if there's a pending accepted connection.
pub fn hasPendingConnection(self: *const PassiveListener) bool {
    return @atomicLoad(u8, &self.has_pending_accept, .acquire) != 0;
}

/// Pick up the pending accepted connection.
pub fn acceptConnection(self: *PassiveListener) ListenerError!AcceptResult {
    if (!hasPendingConnection(self)) {
        return ListenerError.NoPendingConnection;
    }

    @atomicStore(u8, &self.has_pending_accept, 0, .release);

    const fd = self.accepted_fd;
    if (fd < 0) {
        return ListenerError.NoPendingConnection;
    }

    self.accepted_fd = -1;
    return AcceptResult{
        .socket_fd = fd,
        .peer_address = self.accepted_peer_address,
        .peer_port = self.accepted_peer_port,
    };
}

/// Signal the listener to stop.
pub fn requestStop(self: *PassiveListener) void {
    @atomicStore(u8, &self.cleanup_requested, 1, .release);
}

/// Stop the listener and close the listening socket.
pub fn close(self: *PassiveListener) void {
    requestStop(self);

    if (self.thread) |t| {
        t.join();
        self.thread = null;
    }

    if (self.listen_fd >= 0) {
        _ = std.c.close(self.listen_fd);
        self.listen_fd = -1;
    }

    if (self.accepted_fd >= 0) {
        _ = std.c.close(self.accepted_fd);
        self.accepted_fd = -1;
    }
}

/// Get the bound listen address as a formatted string.
pub fn getListenAddress(self: *const PassiveListener) [32]u8 {
    var buf: [32]u8 = undefined;
    const addr = tcp_transport_helpers.fmtPeerAddress(self.config.local_address);
    const port = self.config.port;
    const written = std.fmt.bufPrint(&buf, "{}:{}", .{ addr, port }) catch unreachable;
    _ = written;
    return buf;
}

/// Set socket to non-blocking mode.
fn setNonBlocking(sockfd: std.c.fd_t) !void {
    const F_GETFL: c_int = 3;
    const F_SETFL: c_int = 4;
    const zero: c_int = 0;

    const flags = std.c.fcntl(sockfd, F_GETFL, zero);
    if (flags < 0) return error.FcntlFailed;

    const new_flags: c_int = flags | tcp_transport_helpers.O_NONBLOCK;
    if (std.c.fcntl(sockfd, F_SETFL, new_flags) < 0) {
        return error.FcntlFailed;
    }
}
