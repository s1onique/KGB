// runtime.zig — BFD multihop runtime layer
//
// Manages BFD sessions over UDP multihop transport.
// Provides tick-based processing and receive path for BFD packets.

const std = @import("std");
const packet = @import("packet.zig");
const config = @import("config.zig");
const clock = @import("clock.zig");
const session = @import("session.zig");
const transport = @import("transport.zig");

/// Re-export commonly used types
pub const Session = session.Session;
pub const SessionState = session.SessionState;
pub const SessionStats = session.SessionStats;
pub const BfdConfig = config.BfdConfig;
pub const Clock = clock.Clock;

/// Maximum number of BFD peers supported
pub const MaxPeers: usize = 16;

/// Runtime error types
pub const RuntimeError = error{
    /// Session not found for peer
    SessionNotFound,
    /// Invalid packet received
    InvalidPacket,
    /// Too many peers configured
    TooManyPeers,
    /// Transport error
    TransportError,
};

/// Runtime-level BFD receive statistics.
/// These counters track packet flow through the receive path for diagnostics.
pub const RuntimeReceiveStats = struct {
    /// Packets received from network (before decode)
    received: u64 = 0,
    /// Packets successfully decoded
    decoded: u64 = 0,
    /// Packets dropped - invalid format
    dropped_invalid_packet: u64 = 0,
    /// Packets dropped - session not found
    dropped_session_not_found: u64 = 0,
    /// Packets dropped - bad discriminator
    dropped_bad_discriminator: u64 = 0,
    /// Packets with Your Discriminator = 0 accepted as initial startup
    accepted_initial_zero_your_discr: u64 = 0,
    /// Remote discriminator learned from packet
    remote_discriminator_learned: u64 = 0,
    /// Control packets sent in response
    control_packets_sent: u64 = 0,
};

/// Peer state for status reporting
pub const PeerStatus = struct {
    /// Peer address
    peer_addr: []const u8,
    /// Local discriminator
    local_discr: u32,
    /// Remote discriminator
    remote_discr: u32,
    /// Current session state
    state: SessionState,
    /// Desired transmit interval in ms
    interval_ms: u32,
    /// Detection multiplier
    multiplier: u8,
    /// Detection timeout in ms
    detection_timeout_ms: u32,
    /// Packets sent
    packets_sent: u64,
    /// Packets received
    packets_received: u64,
    /// Packets dropped
    packets_dropped: u64,
    /// Detection timeouts
    detection_timeouts: u64,
};

/// BFD runtime managing one or more multihop sessions
pub const BfdRuntime = struct {
    const Self = @This();

    /// Session configurations
    configs: [MaxPeers]BfdConfig = undefined,
    config_count: usize = 0,

    /// Active BFD sessions (parallel to configs)
    sessions: [MaxPeers]Session = undefined,

    /// Transport interface for sending packets
    transport: transport.Transport,

    /// Transport context (for fake transport ownership)
    transport_ctx: ?*transport.TransportContext = null,

    /// Clock interface for timers
    clock: Clock,

    /// Session storage (for cross-session operations)
    session_storage: *[MaxPeers]Session = undefined,

    /// Create a new BFD runtime with transport and clock.
    pub fn init(trans: transport.Transport, c: Clock) Self {
        return Self{
            .transport = trans,
            .transport_ctx = null,
            .clock = c,
            .session_storage = undefined,
        };
    }

    /// Create a new BFD runtime with transport, clock, and transport context.
    /// The transport_ctx must outlive the runtime.
    pub fn initWithContext(trans: transport.Transport, c: Clock, ctx: *transport.TransportContext) Self {
        return Self{
            .transport = trans,
            .transport_ctx = ctx,
            .clock = c,
            .session_storage = undefined,
        };
    }

    /// Add a peer configuration.
    pub fn addPeer(self: *Self, cfg: BfdConfig) RuntimeError!void {
        if (self.config_count >= MaxPeers) {
            return RuntimeError.TooManyPeers;
        }

        // Check for duplicate peer address
        for (0..self.config_count) |i| {
            if (std.mem.eql(u8, self.configs[i].peer_addr, cfg.peer_addr)) {
                return RuntimeError.TooManyPeers;
            }
        }

        self.configs[self.config_count] = cfg;
        self.sessions[self.config_count] = session.init(cfg, self.clock);
        self.session_storage = &self.sessions;
        self.config_count += 1;
    }

    /// Get session index by peer address.
    fn getSessionIndex(self: *const Self, peer_addr: []const u8) ?usize {
        for (0..self.config_count) |i| {
            if (std.mem.eql(u8, self.configs[i].peer_addr, peer_addr)) {
                return i;
            }
        }
        return null;
    }

    /// Get session by peer address.
    pub fn getSession(self: *Self, peer_addr: []const u8) ?*Session {
        if (self.getSessionIndex(peer_addr)) |idx| {
            return &self.sessions[idx];
        }
        return null;
    }

    /// Get configuration by peer address.
    pub fn getConfig(self: *const Self, peer_addr: []const u8) ?*const BfdConfig {
        for (0..self.config_count) |i| {
            if (std.mem.eql(u8, self.configs[i].peer_addr, peer_addr)) {
                return &self.configs[i];
            }
        }
        return null;
    }

    /// Tick the runtime - process transmit and detection timers.
    pub fn tick(self: *Self) RuntimeError!void {
        for (0..self.config_count) |i| {
            const sess = &self.sessions[i];
            const cfg = &self.configs[i];

            if (session.isDetectionExpired(sess)) {
                _ = session.processEvent(sess, .detectionTimeout);
            }

            if (session.isTransmitDue(sess)) {
                const tx_pkt = session.buildTransmitPacket(sess);
                var buf: [packet.CONTROL_PACKET_LEN]u8 = undefined;
                _ = packet.encode(tx_pkt, &buf);

                // Send the BFD control packet
                self.transport.sendPacket(self.transport.ctx, cfg.peer_addr, transport.MULTIHOP_PORT, &buf) 
                    catch |send_err| {
                    // Diagnostic: send failed - RealTransport already logged detailed info above
                    std.debug.print("[BFD] bfd_control_packet_send_failed to={s} reason={s}\n", .{ cfg.peer_addr, @errorName(send_err) });
                    return RuntimeError.TransportError;
                };

                // Diagnostic: send succeeded
                std.debug.print("[BFD] bfd_control_packet_sent to={s}\n", .{cfg.peer_addr});

                _ = session.processEvent(sess, .transmitTimeout);
            }
        }
    }

    /// Receive a BFD packet from a peer.
    pub fn receivePacket(self: *Self, peer_addr: []const u8, bytes: []const u8) RuntimeError!void {
        const idx = self.getSessionIndex(peer_addr) orelse return RuntimeError.SessionNotFound;

        const pkt = packet.decode(bytes) 
            catch return RuntimeError.InvalidPacket;

        const sess = &self.sessions[idx];
        _ = session.processEvent(sess, .{ .packetReceived = pkt });
    }

    /// Start all sessions.
    pub fn startAll(self: *Self) void {
        for (0..self.config_count) |i| {
            _ = session.processEvent(&self.sessions[i], .start);
        }
    }

    /// Get peer status for all configured peers.
    pub fn getPeerStatuses(self: *const Self, allocator: std.mem.Allocator) ![]const PeerStatus {
        const statuses = try allocator.alloc(PeerStatus, self.config_count);
        for (0..self.config_count) |i| {
            const sess = &self.sessions[i];
            const cfg = &self.configs[i];
            
            statuses[i] = PeerStatus{
                .peer_addr = cfg.peer_addr,
                .local_discr = sess.local_discr,
                .remote_discr = sess.remote_discr,
                .state = sess.state,
                .interval_ms = cfg.interval_ms,
                .multiplier = cfg.multiplier,
                .detection_timeout_ms = sess.detection_timeout_ms,
                .packets_sent = sess.stats.packets_sent,
                .packets_received = sess.stats.packets_received,
                .packets_dropped = sess.stats.packets_dropped,
                .detection_timeouts = sess.stats.detection_timeouts,
            };
        }
        
        return statuses;
    }

    /// Count of configured peers.
    pub fn peerCount(self: *const Self) usize {
        return self.config_count;
    }

    /// Count of peers in Up state.
    pub fn upCount(self: *const Self) usize {
        // Defensive: if config_count is 0 or looks invalid, return 0
        if (self.config_count == 0) return 0;
        if (self.config_count > MaxPeers) return 0;
        
        var count: usize = 0;
        for (0..self.config_count) |i| {
            if (self.sessions[i].state == .up) {
                count += 1;
            }
        }
        return count;
    }

    /// Check if any peers are configured.
    pub fn hasPeers(self: *const Self) bool {
        return self.config_count > 0;
    }
};
