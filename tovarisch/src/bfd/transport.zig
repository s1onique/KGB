// transport.zig — BFD transport abstraction for UDP multihop
//
// Provides a transport interface that can be implemented with:
// - Real UDP socket for production
// - Fake/mock for unit tests
//
// UDP port 4784 is used for multihop BFD per RFC 5883.

const std = @import("std");
const packet = @import("packet.zig");

/// UDP destination port for multihop BFD (RFC 5883)
pub const MULTIHOP_PORT: u16 = packet.MULTIHOP_UDP_PORT;

/// Transport error types
pub const TransportError = error{
    /// Peer address is not configured
    UnknownPeer,
    /// Send failed (network issue)
    SendFailed,
    /// Port validation failed
    InvalidPort,
    /// Address parsing failed
    AddressParseFailed,
};

/// Transport interface for sending BFD packets.
/// Uses context-based approach where the transport wraps both
/// a function pointer and context for testability.
pub const Transport = struct {
    /// Send a BFD packet to a peer.
    sendPacket: *const fn (ctx: *anyopaque, peer_addr: []const u8, port: u16, bytes: []const u8) TransportError!void,
    /// Context for the send function (e.g., pointer to FakeTransport or RealTransport).
    ctx: *anyopaque,
};

// ============================================================================
// Socket Address Structures (Linux-specific, matching kernel wire format)
// ============================================================================

/// Address family constants
pub const AF_INET: c_int = 2;

/// Socket protocol constants
pub const SOCK_DGRAM: c_int = 2;
pub const IPPROTO_UDP: c_int = 17;

/// socklen_t type
pub const socklen_t = std.c.socklen_t;

/// sockaddr_in - IPv4 socket address structure (Linux wire format)
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

/// timeval for socket timeout
pub const timeval = extern struct {
    tv_sec: c_long,
    tv_usec: c_long,
};

/// Socket option constants
pub const SOL_SOCKET: c_int = 1;
pub const SO_RCVTIMEO: c_int = 20;

/// Real Linux UDP transport for production use.
/// Sends BFD multihop packets via UDP socket to destination port 4784.
pub const RealTransport = struct {
    const Self = @This();

    /// Send a BFD packet over UDP.
    /// Creates an ephemeral UDP socket, sends to peer_addr:port, and closes.
    pub fn sendPacket(peer_addr: []const u8, port: u16, bytes: []const u8) TransportError!void {
        if (port != MULTIHOP_PORT) {
            return TransportError.InvalidPort;
        }
        if (bytes.len != packet.CONTROL_PACKET_LEN) {
            return TransportError.SendFailed;
        }

        // Build sockaddr_in for the destination
        var addr: sockaddr_in = undefined;
        addr.sin_family = @as(c_ushort, @intCast(AF_INET));
        // Port in network byte order (big-endian) via @byteSwap builtin
        addr.sin_port = @byteSwap(port);
        @memset(addr.sin_zero[0..], 0);

        // Parse IPv4 address string into sin_addr
        try parseIPv4Address(peer_addr, &addr.sin_addr);

        // Create UDP socket (AF_INET, SOCK_DGRAM, IPPROTO_UDP)
        const sockfd = std.c.socket(AF_INET, SOCK_DGRAM, IPPROTO_UDP);
        if (sockfd < 0) {
            return TransportError.SendFailed;
        }
        defer _ = std.c.close(sockfd);

        // Send packet to peer
        const send_addr: *const std.c.sockaddr = @ptrCast(&addr);
        const addr_len: socklen_t = @sizeOf(sockaddr_in);
        const result = std.c.sendto(
            sockfd,
            @ptrCast(bytes.ptr),
            bytes.len,
            0, // flags
            send_addr,
            addr_len,
        );

        if (result < 0) {
            return TransportError.SendFailed;
        }

        return;
    }

    /// Create a Transport interface from real UDP implementation.
    pub fn interface() Transport {
        return Transport{
            .sendPacket = struct {
                fn send(ctx: *anyopaque, peer_addr: []const u8, port: u16, bytes: []const u8) TransportError!void {
                    _ = ctx; // unused for RealTransport
                    return Self.sendPacket(peer_addr, port, bytes);
                }
            }.send,
            .ctx = @ptrFromInt(0), // null context for RealTransport
        };
    }
};

/// Parse an IPv4 dotted-decimal address string into a in_addr.
/// Returns AddressParseFailed if the string is malformed.
pub fn parseIPv4Address(addr_str: []const u8, out: *in_addr) TransportError!void {
    // Parse "a.b.c.d" format
    var parts: [4]u8 = undefined;
    var part_idx: usize = 0;
    var pos: usize = 0;

    while (part_idx < 4 and pos < addr_str.len) {
        // Parse numeric part - first char must be a digit
        if (addr_str[pos] < '0' or addr_str[pos] > '9') {
            return TransportError.AddressParseFailed;
        }

        var value: u32 = 0;
        var has_digit = false;
        while (pos < addr_str.len and addr_str[pos] >= '0' and addr_str[pos] <= '9') {
            value = value * 10 + @as(u32, @intCast(addr_str[pos] - '0'));
            pos += 1;
            has_digit = true;
            if (value > 255) return TransportError.AddressParseFailed;
        }

        // Expect a dot after each part except the last
        if (part_idx < 3) {
            if (pos >= addr_str.len or addr_str[pos] != '.') {
                return TransportError.AddressParseFailed;
            }
            pos += 1; // Skip the dot
        } else {
            // Last part - no dot allowed after
            if (pos < addr_str.len and addr_str[pos] == '.') {
                return TransportError.AddressParseFailed;
            }
        }

        if (!has_digit) {
            return TransportError.AddressParseFailed;
        }

        parts[part_idx] = @as(u8, @intCast(value));
        part_idx += 1;
    }

    // We expect exactly 4 parts
    if (part_idx != 4) {
        return TransportError.AddressParseFailed;
    }

    // Store in network byte order (big-endian) in s_addr
    out.s_addr = std.mem.readInt(u32, &parts, .big);
}

/// Fake transport for testing.
/// Captures sent packets so tests can assert on them.
pub const FakeTransport = struct {
    const Self = @This();

    /// Captured packet data
    pub const CapturedPacket = struct {
        peer_addr: []const u8,
        port: u16,
        bytes: [packet.CONTROL_PACKET_LEN]u8,
    };

    /// Maximum number of captured packets
    pub const MaxCaptured: usize = 64;

    /// Captured packets storage
    captured: [MaxCaptured]CapturedPacket = undefined,
    captured_count: usize = 0,

    /// Known peer addresses (for validation)
    known_peers: []const []const u8 = &.{},

    /// Create a new fake transport with optional known peers.
    pub fn init(known_peers: []const []const u8) Self {
        var self = Self{};
        self.known_peers = known_peers;
        for (0..MaxCaptured) |_| {
            self.captured[self.captured_count] = .{
                .peer_addr = "",
                .port = 0,
                .bytes = [_]u8{0} ** packet.CONTROL_PACKET_LEN,
            };
            self.captured_count += 1;
        }
        self.captured_count = 0;
        return self;
    }

    /// Reset captured packets.
    pub fn reset(self: *Self) void {
        self.captured_count = 0;
        for (0..MaxCaptured) |_| {
            self.captured[self.captured_count] = .{
                .peer_addr = "",
                .port = 0,
                .bytes = [_]u8{0} ** packet.CONTROL_PACKET_LEN,
            };
            self.captured_count += 1;
        }
        self.captured_count = 0;
    }

    /// Send a BFD packet (captures for testing).
    pub fn sendPacket(self: *Self, peer_addr: []const u8, port: u16, bytes: []const u8) TransportError!void {
        if (self.known_peers.len > 0) {
            var found = false;
            for (self.known_peers) |kp| {
                if (std.mem.eql(u8, kp, peer_addr)) {
                    found = true;
                    break;
                }
            }
            if (!found) return TransportError.UnknownPeer;
        }

        if (port != MULTIHOP_PORT) return TransportError.InvalidPort;
        if (bytes.len != packet.CONTROL_PACKET_LEN) return TransportError.SendFailed;

        if (self.captured_count < MaxCaptured) {
            const idx = self.captured_count;
            self.captured[idx].peer_addr = peer_addr;
            self.captured[idx].port = port;
            @memcpy(self.captured[idx].bytes[0..packet.CONTROL_PACKET_LEN], bytes[0..packet.CONTROL_PACKET_LEN]);
            self.captured_count += 1;
        }
    }

    /// Get last captured packet.
    pub fn lastPacket(self: *const Self) ?*const CapturedPacket {
        if (self.captured_count == 0) return null;
        return &self.captured[self.captured_count - 1];
    }

    /// Get captured packet at index.
    pub fn getPacket(self: *const Self, index: usize) ?*const CapturedPacket {
        if (index >= self.captured_count) return null;
        return &self.captured[index];
    }
};

/// Wrapper that implements Transport using a FakeTransport instance.
/// This allows the FakeTransport to be used where a Transport is expected.
/// The wrapper OWNS the FakeTransport to ensure stable memory.
pub const FakeTransportWrapper = struct {
    const Self = @This();

    /// Owned fake transport
    fake: FakeTransport,

    /// Initialize with a fake transport.
    pub fn init(fake: FakeTransport) Self {
        return Self{ .fake = fake };
    }

    /// Send via the wrapped fake transport.
    /// Static function that accepts FakeTransportWrapper as context.
    pub fn send(ctx: *anyopaque, peer_addr: []const u8, port: u16, bytes: []const u8) TransportError!void {
        const wrapper = @as(*Self, @ptrCast(@alignCast(ctx)));
        return wrapper.fake.sendPacket(peer_addr, port, bytes);
    }

    /// Convert to Transport interface.
    pub fn toTransport(self: *Self) Transport {
        return Transport{
            .sendPacket = Self.send,
            .ctx = @ptrCast(self),
        };
    }
};

/// Transport context that owns the underlying transport data.
/// This allows stable pointers across function boundaries.
pub const TransportContext = struct {
    const Self = @This();

    /// Wrapper for fake transport (if fake)
    fake_wrapper: ?FakeTransportWrapper = null,
    /// Marker to identify type
    is_fake: bool = false,

    /// Create a context for a fake transport.
    pub fn initFake(fake: FakeTransport) Self {
        return Self{
            .fake_wrapper = FakeTransportWrapper.init(fake),
            .is_fake = true,
        };
    }

    /// Get the Transport interface for this context.
    /// Passes the fake_wrapper pointer as ctx since that's what FakeTransportWrapper.send expects.
    pub fn toTransport(self: *Self) Transport {
        return Transport{
            .sendPacket = FakeTransportWrapper.send,
            .ctx = @ptrCast(&self.fake_wrapper.?),
        };
    }
};

/// Create a fake transport interface for a specific fake instance.
/// Returns both the Transport interface and the context that owns it.
pub fn makeFakeTransportInterface(fake: FakeTransport) struct { trans: Transport, ctx: *TransportContext } {
    var ctx = std.heap.page_allocator.create(TransportContext) catch unreachable;
    ctx.* = TransportContext.initFake(fake);
    return .{
        .trans = ctx.toTransport(),
        .ctx = ctx,
    };
}

test "FakeTransport captures sent packets" {
    var fake = FakeTransport.init(&.{});
    fake.reset();

    const test_bytes = [_]u8{0x20, 0x40, 0x03, 24, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0x0C, 0x35, 0, 0, 0x0C, 0x35, 0, 0, 0, 0, 0};

    try fake.sendPacket("10.0.0.2", MULTIHOP_PORT, &test_bytes);

    try std.testing.expectEqual(@as(usize, 1), fake.captured_count);
    
    const pkt = fake.lastPacket().?;
    try std.testing.expectEqualStrings("10.0.0.2", pkt.peer_addr);
    try std.testing.expectEqual(@as(u16, MULTIHOP_PORT), pkt.port);
}

test "FakeTransport validates multihop port" {
    var fake = FakeTransport.init(&.{});
    
    const test_bytes = [_]u8{0x20, 0x40, 0x03, 24, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0x0C, 0x35, 0, 0, 0x0C, 0x35, 0, 0, 0, 0, 0};

    try std.testing.expectError(TransportError.InvalidPort, fake.sendPacket("10.0.0.2", 1234, &test_bytes));
}

test "FakeTransport rejects unknown peers when configured" {
    var fake = FakeTransport.init(&.{ "10.0.0.2", "10.0.0.3" });
    
    const test_bytes = [_]u8{0x20, 0x40, 0x03, 24, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0x0C, 0x35, 0, 0, 0x0C, 0x35, 0, 0, 0, 0, 0};

    try fake.sendPacket("10.0.0.2", MULTIHOP_PORT, &test_bytes);
    try std.testing.expectEqual(@as(usize, 1), fake.captured_count);

    try std.testing.expectError(TransportError.UnknownPeer, fake.sendPacket("192.168.1.1", MULTIHOP_PORT, &test_bytes));
}
