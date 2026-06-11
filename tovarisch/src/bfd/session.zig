// session.zig — BFD multihop session state machine
//
// Implements RFC 5880 asynchronous BFD for multihop sessions.
// This is the core session logic, decoupled from UDP transport.
//
// State Machine (RFC 5880 Section 6.8.4):
//
//   AdminDown --[admin down]--> AdminDown
//   AdminDown --[!admin down]--> Down (send poll if initiator)
//   Down --[rcv Init]--> Init
//   Down --[rcv Up, Poll]--> Up (send Final)
//   Down --[rcv Up, !Poll]--> Up
//   Init --[rcv Init or Up]--> Up
//   Init --[rcv Down or timeout]--> Down
//   Up --[rcv Down or timeout]--> Down
//   Up --[rcv Init]--> Init
//
// Detection: session goes Down if no valid BFD control packet received
// before detection timeout expires.

const std = @import("std");
const packet = @import("packet.zig");
const config = @import("config.zig");
const clock = @import("clock.zig");

/// Re-export commonly used types
pub const State = packet.State;
pub const ControlPacket = packet.ControlPacket;
pub const BfdConfig = config.BfdConfig;
pub const MonoTime = clock.MonoTime;
pub const Clock = clock.Clock;
pub const MULTIHOP_UDP_PORT = packet.MULTIHOP_UDP_PORT;
pub const PROTOCOL_VERSION = packet.PROTOCOL_VERSION;

/// BFD session state
pub const SessionState = State;

/// Session event
pub const Event = union(enum) {
    /// Start the session (begin polling)
    start,
    /// Stop the session (admin down)
    stop,
    /// Received a BFD control packet from the peer
    packetReceived: ControlPacket,
    /// Detection timer expired (no packet received in time)
    detectionTimeout,
    /// Transmit timer expired (time to send a packet)
    transmitTimeout,
};

/// Session statistics
pub const SessionStats = struct {
    /// Total packets transmitted
    packets_sent: u64 = 0,
    /// Total packets received
    packets_received: u64 = 0,
    /// Packets dropped (invalid or wrong discriminator)
    packets_dropped: u64 = 0,
    /// Session state transitions
    state_changes: u64 = 0,
    /// Detection timeouts (session went down)
    detection_timeouts: u64 = 0,
};

/// BFD multihop session
pub const Session = struct {
    /// Session configuration
    config: BfdConfig,
    /// Current session state
    state: SessionState = .down,
    /// Local discriminator (assigned on session start)
    local_discr: u32 = 0,
    /// Remote discriminator (learned from peer's first packet)
    remote_discr: u32 = 0,
    /// Remote detection multiplier (from peer's packet)
    remote_detect_mult: u8 = 3,
    /// Remote's required min RX interval (for our TX calculation)
    remote_required_min_rx_us: u32 = 0,
    /// Our required min RX interval (told to peer)
    local_required_min_rx_us: u32 = 0,
    /// Negotiated detection timeout in milliseconds
    detection_timeout_ms: u32 = 0,
    /// Last time we received a valid packet (monotonic ms)
    last_packet_recv_time: MonoTime = 0,
    /// Next scheduled transmit time (monotonic ms)
    next_transmit_time: MonoTime = 0,
    /// Session statistics
    stats: SessionStats = .{},
    /// Clock interface (for time operations)
    clock: Clock,
};

/// Initialize a new BFD session.
pub fn init(cfg: BfdConfig, c: Clock) Session {
    return Session{
        .config = cfg,
        .clock = c,
        .local_discr = cfg.local_discr,
        .local_required_min_rx_us = packet.msToUs(cfg.interval_ms),
        .detection_timeout_ms = config.defaultDetectionTimeout(cfg),
    };
}

/// Process an event on the session.
/// Returns the new state after processing the event.
pub fn processEvent(sess: *Session, event: Event) SessionState {
    switch (event) {
        .start => return handleStart(sess),
        .stop => return handleStop(sess),
        .packetReceived => |pkt| return handlePacketReceived(sess, pkt),
        .detectionTimeout => return handleDetectionTimeout(sess),
        .transmitTimeout => return handleTransmitTimeout(sess),
    }
}

fn handleStart(sess: *Session) SessionState {
    if (sess.state == .admin_down) return .admin_down;
    if (sess.local_discr == 0) {
        sess.local_discr = generateLocalDiscriminator();
        sess.config.local_discr = sess.local_discr;
    }
    sess.remote_discr = 0;
    sess.state = .down;
    sess.stats.state_changes += 1;
    sess.next_transmit_time = sess.clock.getMonoTimeMs();
    return .down;
}

fn handleStop(sess: *Session) SessionState {
    sess.state = .admin_down;
    sess.stats.state_changes += 1;
    return .admin_down;
}

fn handlePacketReceived(sess: *Session, pkt: ControlPacket) SessionState {
    sess.stats.packets_received += 1;
    if (pkt.version != PROTOCOL_VERSION) return sess.state;
    if (pkt.flags.auth_present == 1) {
        sess.stats.packets_dropped += 1;
        return sess.state;
    }
    if (sess.remote_discr == 0 and pkt.my_discr != 0) {
        sess.remote_discr = pkt.my_discr;
    }
    // RFC 5880 Section 6.8.4: Your Discriminator is 0 on initial discovery packets.
    // BIRD and other implementations send Down packets with YourDiscr=0 when they
    // don't know our discriminator yet. Accept these from configured peers.
    // Drop only if YourDiscr is nonzero AND doesn't match our local discriminator.
    if (pkt.your_discr != 0 and pkt.your_discr != sess.local_discr) {
        sess.stats.packets_dropped += 1;
        return sess.state;
    }
    if (pkt.required_min_rx_interval > 0) {
        sess.remote_required_min_rx_us = pkt.required_min_rx_interval;
        sess.remote_detect_mult = pkt.detect_mult;
        sess.detection_timeout_ms = config.calculateDetectionTimeout(
            sess.remote_required_min_rx_us,
            sess.remote_detect_mult,
        );
    }
    sess.last_packet_recv_time = sess.clock.getMonoTimeMs();
    const new_state = nextState(sess.state, pkt.state);
    if (new_state != sess.state) {
        sess.state = new_state;
        sess.stats.state_changes += 1;
    }
    return sess.state;
}

fn handleDetectionTimeout(sess: *Session) SessionState {
    if (sess.state != .up and sess.state != .init) return sess.state;
    sess.state = .down;
    sess.stats.state_changes += 1;
    sess.stats.detection_timeouts += 1;
    sess.remote_discr = 0;
    return .down;
}

fn handleTransmitTimeout(sess: *Session) SessionState {
    if (sess.state == .admin_down) return .admin_down;
    const tx_interval = sess.local_required_min_rx_us / 1000;
    sess.next_transmit_time = sess.clock.getMonoTimeMs() + tx_interval;
    return sess.state;
}

/// Calculate the next state based on current state and received packet state.
/// This is the core RFC 5880 Section 6.8.4 state machine logic.
fn nextState(current: SessionState, received: SessionState) SessionState {
    switch (current) {
        .admin_down => return .admin_down,
        .down => switch (received) {
            .init => return .init,
            .up => return .up,
            else => return .down,
        },
        .init => switch (received) {
            .init, .up => return .up,
            .down => return .down,
            else => return .init,
        },
        .up => switch (received) {
            .down => return .down,
            .init => return .init,
            else => return .up,
        },
    }
}

/// Build a BFD control packet for transmission.
pub fn buildTransmitPacket(sess: *Session) ControlPacket {
    sess.stats.packets_sent += 1;
    return ControlPacket{
        .state = sess.state,
        .diag = .no_diagnostic,
        .detect_mult = sess.config.multiplier,
        .length = packet.CONTROL_PACKET_LEN,
        .my_discr = sess.local_discr,
        .your_discr = sess.remote_discr,
        .desired_min_tx_interval = sess.remote_required_min_rx_us,
        .required_min_rx_interval = sess.local_required_min_rx_us,
        .required_min_echo_rx_interval = 0,
    };
}

/// Check if detection timer has expired.
pub fn isDetectionExpired(sess: *Session) bool {
    if (sess.remote_discr == 0) return false;
    if (sess.state != .up and sess.state != .init) return false;
    const elapsed = sess.clock.getMonoTimeMs() -% sess.last_packet_recv_time;
    return elapsed >= sess.detection_timeout_ms;
}

/// Check if it's time to transmit.
pub fn isTransmitDue(sess: *Session) bool {
    if (sess.state == .admin_down) return false;
    // In Down state, suppress transmit only if we've sent at least one packet
    // and are still waiting for the peer's response.
    // This prevents the BFD startup deadlock where tovarisch doesn't respond
    // to BIRD's initial packet because it thinks it's "waiting".
    if (sess.state == .down and sess.stats.packets_sent > 0 and sess.remote_discr == 0) {
        return false;
    }
    return sess.clock.getMonoTimeMs() >= sess.next_transmit_time;
}

/// Generate a local discriminator (pseudo-random for uniqueness).
/// Uses PID and address-based mixing since wall-clock time APIs are limited.
fn generateLocalDiscriminator() u32 {
    const pid = std.c.getpid();
    // Use PID with some mixing from pointer address for uniqueness
    const pid_val = @as(u32, @intCast(pid));
    const addr = @as(usize, @intFromPtr(&pid_val));
    // Mix using lower bits of address
    const mix = @as(u32, @truncate(addr));
    return pid_val ^ (mix *% 31);
}
