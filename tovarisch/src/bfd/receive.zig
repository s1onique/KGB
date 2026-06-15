// receive.zig — BFD UDP receive socket for multihop sessions
//
// Binds UDP port 4784 and provides a receive loop that decodes incoming
// BFD control packets and feeds them to the BfdRuntime. This enables
// the daemon to respond to BIRD's BFD session establishment.
//
// UDP receive is Linux-only (tovarisch targets Linux).

const std = @import("std");
const c = std.c;
const packet = @import("packet.zig");
const runtime = @import("runtime.zig");
const transport = @import("transport.zig");

/// Atomic-like stop signal for BFD receive loop.
/// Uses volatile bool pattern compatible with Zig 0.16.
pub const StopSignal = struct {
    const Self = @This();

    /// The stop flag (using atomic for cross-thread visibility).
    flag: u8 = 0,

    /// Store true to signal stop.
    pub fn store(self: *Self) void {
        @atomicStore(u8, &self.flag, 1, .monotonic);
    }

    /// Load the stop flag.
    pub fn load(self: *const Self) bool {
        return @atomicLoad(u8, &self.flag, .monotonic) != 0;
    }
};

/// UDP port for multihop BFD (RFC 5883)
pub const MULTIHOP_PORT: u16 = packet.MULTIHOP_UDP_PORT;

/// BFD receive socket errors
pub const ReceiveError = error{
    /// Failed to create UDP socket
    SocketCreateFailed,
    /// Failed to bind UDP socket to port
    BindFailed,
    /// Failed to set socket options
    SetsockoptFailed,
    /// Failed to receive data
    RecvFailed,
    /// Invalid packet received
    InvalidPacket,
};

/// sockaddr_in for UDP receive (same as transport.zig)
const sockaddr_in = extern struct {
    sin_family: c_ushort,
    sin_port: c_ushort,
    sin_addr: extern struct {
        s_addr: c_uint,
    },
    sin_zero: [8]u8,
};

/// Receive buffer size for one BFD packet (24 bytes + IP/UDP headers)
const RECV_BUFFER_SIZE: usize = 512;

/// Owned BFD packet received from network.
/// This struct owns all its data so it can be passed across function boundaries
/// without dangling pointers.
pub const ReceivedBfdPacket = struct {
    const Self = @This();

    /// The BFD control packet bytes (24 bytes per RFC 5880).
    bytes: [packet.CONTROL_PACKET_LEN]u8,
    /// Owned buffer for peer IP address string.
    peer_addr_buf: [16]u8,
    /// Length of peer address string.
    peer_addr_len: usize,

    /// Get the peer address as a slice.
    pub fn peerAddr(self: *const Self) []const u8 {
        return self.peer_addr_buf[0..self.peer_addr_len];
    }
};

/// BFD receive socket that binds UDP 4784 and receives packets.
pub const BfdReceiveSocket = struct {
    const Self = @This();

    /// UDP socket file descriptor
    fd: i32 = -1,

    /// Create a new BFD receive socket bound to UDP port 4784.
    pub fn bind(port: u16) ReceiveError!Self {
        if (port != MULTIHOP_PORT) {
            return ReceiveError.BindFailed;
        }

        // Create UDP socket
        const sockfd = c.socket(c.AF.INET, c.SOCK.DGRAM, c.IPPROTO.UDP);
        if (sockfd < 0) {
            return ReceiveError.SocketCreateFailed;
        }

        var self = Self{ .fd = sockfd };
        errdefer {
            _ = c.close(self.fd);
            self.fd = -1;
        }

        // Set SO_REUSEADDR to allow rebind if daemon restarts quickly
        const one: c_int = 1;
        if (c.setsockopt(self.fd, c.SOL.SOCKET, c.SO.REUSEADDR, &one, @sizeOf(c_int)) < 0) {
            return ReceiveError.SetsockoptFailed;
        }

        // Construct sockaddr_in for binding
        var addr: sockaddr_in = std.mem.zeroes(sockaddr_in);
        addr.sin_family = c.AF.INET;
        addr.sin_port = @byteSwap(port); // Network byte order (big-endian)
        addr.sin_addr.s_addr = 0; // INADDR_ANY - listen on all interfaces
        @memset(addr.sin_zero[0..], 0);

        // Bind to the UDP port
        const bind_result = c.bind(
            self.fd,
            @ptrCast(&addr),
            @sizeOf(sockaddr_in),
        );
        if (bind_result < 0) {
            return ReceiveError.BindFailed;
        }

        return self;
    }

    /// Close the socket.
    pub fn close(self: *Self) void {
        if (self.fd >= 0) {
            _ = c.close(self.fd);
            self.fd = -1;
        }
    }

    /// Receive one BFD packet and return the owned packet structure.
    /// Blocks until a packet is available or returns null on timeout.
    /// The returned ReceivedBfdPacket owns all its data (no dangling pointers).
    pub fn receiveOne(self: *Self) ReceiveError!?ReceivedBfdPacket {
        var buf: [RECV_BUFFER_SIZE]u8 = undefined;
        var src_addr: sockaddr_in = undefined;
        var addr_len: c.socklen_t = @sizeOf(sockaddr_in);

        const received = c.recvfrom(
            self.fd,
            &buf,
            buf.len,
            0,
            @ptrCast(&src_addr),
            &addr_len,
        );

        if (received < 0) {
            const errno_val = std.c._errno().*;
            // EAGAIN/EWOULDBLOCK means no data yet (non-blocking or timeout)
            if (errno_val == 11 or errno_val == 35) {
                return null;
            }
            return ReceiveError.RecvFailed;
        }

        // We need at least 24 bytes for a BFD control packet
        const pkt_len: usize = @intCast(received);
        if (pkt_len < packet.CONTROL_PACKET_LEN) {
            return null;
        }

        // Extract just the BFD control packet bytes
        var result = ReceivedBfdPacket{
            .bytes = undefined,
            .peer_addr_buf = undefined,
            .peer_addr_len = 0,
        };
        const payload_start = @min(@as(usize, @intCast(received)), packet.CONTROL_PACKET_LEN);
        @memcpy(result.bytes[0..payload_start], buf[0..payload_start]);

        // Parse IPv4 address from network byte order (big-endian).
        // sin_addr.s_addr is stored in network byte order per POSIX.
        const addr_be = std.mem.readInt(u32, @ptrCast(&src_addr.sin_addr.s_addr), .big);
        const b1: u8 = @truncate(addr_be >> 24);
        const b2: u8 = @truncate(addr_be >> 16);
        const b3: u8 = @truncate(addr_be >> 8);
        const b4: u8 = @truncate(addr_be);
        result.peer_addr_len = formatIPv4IntoBuf(&result.peer_addr_buf, b1, b2, b3, b4);

        return result;
    }

    /// Set socket to non-blocking mode.
    /// Uses platform-specific flag (NONBLOCK on Linux, O_NONBLOCK pattern for cross-platform).
    pub fn setNonBlocking(self: *Self) void {
        if (self.fd >= 0) {
            const flags = c.fcntl(self.fd, c.F.GETFL);
            // NONBLOCK is the Linux value; on other platforms this is skipped
            if (comptime @import("builtin").os.tag == .linux) {
                _ = c.fcntl(self.fd, c.F.SETFL, flags | 2048); // O_NONBLOCK = 2048 on Linux
            }
        }
    }

    /// Check if socket is valid (for cleanup coordination).
    pub fn isValid(self: *const Self) bool {
        return self.fd >= 0;
    }
};

/// Format IPv4 address bytes directly into a buffer.
/// Returns the length of the formatted string.
fn formatIPv4IntoBuf(buf: *[16]u8, b1: u8, b2: u8, b3: u8, b4: u8) usize {
    const formatted = std.fmt.bufPrint(buf, "{d}.{d}.{d}.{d}", .{
        b1, b2, b3, b4,
    }) catch unreachable;
    return formatted.len;
}

/// POLLIN event flag (from poll.h)
const POLLIN: c_short = 0x0001;

/// Default poll timeout in milliseconds (50ms = 20 polls/second max)
/// This balances BFD responsiveness (typically 800ms+ intervals) with CPU savings.
/// BFD packets arrive at most once per configured interval; 50ms is 1/16th of the
/// minimum typical BFD interval (800ms), providing good responsiveness without busy-spin.
pub const DEFAULT_POLL_TIMEOUT_MS: c_int = 50;

/// BFD receive loop state passed to the receive thread.
/// Includes the stop signal and runtime reference.
/// Socket ownership: the loop state OWNS the socket; bundle does NOT copy it.
pub const BfdReceiveLoopState = struct {
    /// Pointer to the BFD runtime to feed packets into.
    runtime: *runtime.BfdRuntime,
    /// UDP socket for receiving packets (owned by this state).
    socket: BfdReceiveSocket,
    /// Stop signal (set to true to stop the loop).
    stop: *StopSignal,
    /// Local address this daemon is listening on (for discriminator learning).
    local_addr: []const u8,
    /// Heap-allocated flag to track if state needs cleanup.
    /// Set to true when this state should be freed.
    needs_cleanup: bool = true,
};

/// BFD receive loop function.
/// Runs in a separate thread, receives BFD packets, and feeds them to the runtime.
/// This enables the daemon to respond to peer's BFD session establishment.
///
/// Uses poll() with bounded timeout before recvfrom() to prevent CPU spin on EAGAIN.
/// This ensures the daemon remains responsive to BFD packets while not consuming
/// excessive CPU when no packets are arriving.
pub fn bfdReceiveLoop(state: *BfdReceiveLoopState) void {
    // Emit diagnostics to stderr for production debugging
    std.debug.print("[BFD] bfd_receive_loop_started addr={s}\n", .{state.local_addr});
    bfdReceiveLoopWithTimeout(state, DEFAULT_POLL_TIMEOUT_MS);
}

/// BFD receive loop with configurable poll timeout.
/// Exposed for testing with shorter timeouts.
pub fn bfdReceiveLoopWithTimeout(state: *BfdReceiveLoopState, poll_timeout_ms: c_int) void {
    // Set socket to non-blocking so we can check the stop flag
    state.socket.setNonBlocking();

    // Verify stop_signal is false at startup - this catches initialization bugs
    // where raw heap allocation left the flag undefined.
    // If this assertion fails, it means BfdServeBundle was not properly initialized.
    if (state.stop.load()) {
        std.debug.print("[BFD] ERROR: receive loop started with stop_signal already set - bundle initialization bug\n", .{});
        state.socket.close();
        return;
    }

    // Prepare pollfd for the receive socket
    // Use array of 1 pollfd for c.poll compatibility (expects [*]pollfd)
    var pfd_arr: [1]c.pollfd = .{
        .{
            .fd = state.socket.fd,
            .events = POLLIN,
            .revents = 0,
        },
    };

    while (!state.stop.load()) {
        // Use poll() with bounded timeout to avoid CPU spin on EAGAIN.
        // This is the key fix: instead of busy-polling with yield(), we block
        // on poll() until data is available or timeout expires.
        const poll_result = c.poll(&pfd_arr, 1, poll_timeout_ms);

        if (poll_result < 0) {
            // Poll error - this shouldn't happen for UDP socket
            // Yield to avoid tight loop on persistent errors
            std.Thread.yield() catch {};
            continue;
        }

        if (poll_result == 0) {
            // Timeout - no data available, loop back to check stop flag
            continue;
        }

        // revents is non-zero, data should be available
        // Check for POLLIN - if not set, could be POLLERR/POLLHUP
        if (pfd_arr[0].revents & POLLIN == 0) {
            // Not POLLIN - could be POLLERR or POLLHUP, try recv to drain
            const result = state.socket.receiveOne() catch {
                continue;
            };
            _ = result;
            continue;
        }

        // Receive one packet (data is confirmed available by poll)
        const result = state.socket.receiveOne() catch {
            // On error, yield and retry (socket error, not EAGAIN since poll said ready)
            std.Thread.yield() catch {};
            continue;
        };

        const packet_opt = result orelse {
            // No packet (shouldn't happen if poll said data available)
            // but handle gracefully
            continue;
        };

        const pkt = packet_opt.bytes;
        const peer_addr = packet_opt.peerAddr();

        // Diagnostic: BFD packet received from peer
        std.debug.print("[BFD] bfd_receive_packet from={s} size={d}\n", .{ peer_addr, pkt.len });

        // Feed the packet to the runtime.
        // RFC 5880 Section 6.8.4: Your Discriminator = 0 is valid for initial
        // discovery packets. The session handles discriminator learning - we just
        // pass the packet through. handleDiscriminatorLearning() was incorrectly
        // returning SessionNotFound for new sessions before the session could process
        // the packet and learn the remote discriminator.
        state.runtime.receivePacket(peer_addr, &pkt) catch |err| {
            // Failed to process packet - log reason
            std.debug.print("[BFD] bfd_receive_packet_dropped from={s} reason={s}\n", .{ peer_addr, @errorName(err) });
            continue;
        };

        // Diagnostic: packet accepted and processed
        std.debug.print("[BFD] bfd_receive_packet_accepted from={s}\n", .{peer_addr});
    }

    // Clean up socket on exit
    std.debug.print("[BFD] bfd_receive_loop_stopped\n", .{});
    state.socket.close();
}

/// Handle discriminator learning for new sessions.
///
/// When BIRD initiates a BFD session, it sends packets with Your Discriminator = 0
/// (meaning it doesn't know our discriminator yet). We need to:
///
/// 1. Find or create a session for this peer
/// 2. Assign a local discriminator if we don't have one
/// 3. Learn the peer's discriminator from the packet's My Discriminator field
///
/// This allows BIRD to learn our discriminator from subsequent packets.
fn handleDiscriminatorLearning(rt: *runtime.BfdRuntime, peer_addr: []const u8, your_discr: u32) runtime.RuntimeError!void {
    // If we already have a session for this peer with a local discriminator, nothing to do
    if (rt.getSession(peer_addr)) |sess| {
        if (sess.local_discr != 0) {
            // Session already initialized, check if we need to learn peer's discriminator
            if (sess.remote_discr == 0 and your_discr == 0) {
                // This case is handled in session.handlePacketReceived
                return;
            }
            return;
        }
    }

    // Try to get or create session for this peer
    if (rt.getSession(peer_addr)) |_| {
        // Session exists but needs initialization - trigger start
        // The runtime should already have been started
        return;
    }

    // No session for this peer - this is a new connection from an unknown peer
    // For now, we only handle configured peers
    return runtime.RuntimeError.SessionNotFound;
}

// ============================================================================
// Tests (Linux-only, use FakeTransport for cross-platform testing)
// ============================================================================

test "BfdReceiveSocket bind validates port" {
    // This test only works on Linux where we can actually bind
    if (comptime @import("builtin").os.tag != .linux) {
        return error.SkipZigTest;
    }

    // Try to bind to the BFD port
    var socket = BfdReceiveSocket.bind(MULTIHOP_PORT) catch {
        return error.SkipZigTest;
    };
    defer socket.close();

    try std.testing.expect(socket.fd >= 0);
}

test "formatIPv4IntoBuf formats correctly" {
    var buf: [16]u8 = undefined;
    // 10.149.149.1 = 12 characters
    const len = formatIPv4IntoBuf(&buf, 10, 149, 149, 1);
    try std.testing.expectEqual(@as(usize, 12), len);
    const result = buf[0..len];
    try std.testing.expectEqualStrings("10.149.149.1", result);
}

test "formatIPv4IntoBuf handles localhost" {
    var buf: [16]u8 = undefined;
    // 127.0.0.1
    const len = formatIPv4IntoBuf(&buf, 127, 0, 0, 1);
    try std.testing.expectEqual(@as(usize, 9), len);
    const result = buf[0..len];
    try std.testing.expectEqualStrings("127.0.0.1", result);
}
