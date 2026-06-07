// packet.zig — BFD control packet encode/decode per RFC 5880
//
// BFD Control Packet format (24 bytes):
//   0               1               2               3
//   0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
//  +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//  | Vers |  Reserved   | Sta(P)    |     Flags     |  Detect Mult  |
//  +-------------------------------+-------------------------------+
//  |           Length              |          My Discriminator     |
//  +-------------------------------+-------------------------------+
//  |        Your Discriminator     |        Desired Min TX Int     |
//  +-------------------------------+-------------------------------+
//  |       Required Min RX Int     |   Required Min Echo RX Int    |
//  +-------------------------------+-------------------------------+
//
// Multihop BFD uses UDP destination port 4784 (RFC 5883).

const std = @import("std");

/// BFD protocol version (RFC 5880 Section 4.1)
pub const PROTOCOL_VERSION: u3 = 1;

/// UDP destination port for multihop BFD (RFC 5883)
pub const MULTIHOP_UDP_PORT: u16 = 4784;

/// UDP destination port for single-hop BFD (for reference)
pub const SINGLEHOP_UDP_PORT: u16 = 3784;

/// UDP destination port for BFD echo (for reference)
pub const ECHO_UDP_PORT: u16 = 3785;

/// Minimum BFD control packet length (24 bytes)
pub const CONTROL_PACKET_LEN: usize = 24;

/// BFD session state (RFC 5880 Section 6.8.1)
pub const State = enum(u2) {
    admin_down = 0,
    down = 1,
    init = 2,
    up = 3,
};

/// BFD diagnostic code (RFC 5880 Section 6.8.2)
pub const Diagnostic = enum(u5) {
    no_diagnostic = 0,
    control_detection_time_expired = 1,
    echo_function_failed = 2,
    neighbor_signaled_session_down = 3,
    forwarding_plane_reset = 4,
    path_down = 5,
    concatenated_path_down = 6,
    admin_down = 7,
    reverse_concatenated_path_down = 8,
};

/// BFD control packet flags (RFC 5880 Section 6.8.3)
/// In the wire format, flags are mapped as:
///   bit 5: Poll (P)
///   bit 4: Final (F)
///   bit 3: Control Plane Independent (C)
///   bit 2: Authentication Present (A)
///   bit 1: Demand (D)
///   bit 0: Multipoint (M)
pub const Flags = struct {
    poll: u1 = 0,
    final: u1 = 0,
    control_plane_independent: u1 = 0,
    auth_present: u1 = 0,
    demand: u1 = 0,
    multipoint: u1 = 0,
};

/// Encode flags to wire format (bits 5-0 of byte 1).
/// RFC 5880: bit 5=Poll, bit 4=Final, bit 3=C, bit 2=A, bit 1=Demand, bit 0=Multipoint
pub fn encodeFlags(flags: Flags) u8 {
    return (@as(u8, flags.poll) << 5) |
        (@as(u8, flags.final) << 4) |
        (@as(u8, flags.control_plane_independent) << 3) |
        (@as(u8, flags.auth_present) << 2) |
        (@as(u8, flags.demand) << 1) |
        @as(u8, flags.multipoint);
}

/// Decode flags from wire format (bits 5-0 of byte 1).
pub fn decodeFlags(v: u8) Flags {
    return Flags{
        .poll = @intCast((v >> 5) & 1),
        .final = @intCast((v >> 4) & 1),
        .control_plane_independent = @intCast((v >> 3) & 1),
        .auth_present = @intCast((v >> 2) & 1),
        .demand = @intCast((v >> 1) & 1),
        .multipoint = @intCast(v & 1),
    };
}

/// BFD Control Packet (RFC 5880 Section 6.8)
pub const ControlPacket = struct {
    /// Protocol version (must be 1)
    version: u3 = PROTOCOL_VERSION,
    /// Diagnostic code
    diag: Diagnostic = .no_diagnostic,
    /// Session state
    state: State = .down,
    /// Control flags
    flags: Flags = .{},
    /// Detection time multiplier
    detect_mult: u8 = 3,
    /// Packet length in bytes (always CONTROL_PACKET_LEN = 24)
    length: u8 = CONTROL_PACKET_LEN,
    /// My discriminator (local session identifier)
    my_discr: u32 = 0,
    /// Your discriminator (remote session identifier, 0 if unknown)
    your_discr: u32 = 0,
    /// Desired minimum transmit interval (microseconds)
    desired_min_tx_interval: u32 = 0,
    /// Required minimum receive interval (microseconds)
    required_min_rx_interval: u32 = 0,
    /// Required minimum echo receive interval (microseconds, 0 = not supported)
    required_min_echo_rx_interval: u32 = 0,
};

/// Encode a BFD control packet into a byte buffer.
/// Returns the number of bytes written (always CONTROL_PACKET_LEN = 24).
pub fn encode(pkt: ControlPacket, buf: []u8) usize {
    if (buf.len < CONTROL_PACKET_LEN) return 0;

    // Byte 0: Version (3) + Diagnostic (5)
    const diag_val: u5 = @intFromEnum(pkt.diag);
    buf[0] = (@as(u8, pkt.version) << 5) | @as(u8, diag_val);

    // Byte 1: State (2) + Flags (6)
    const state_bits: u8 = @intFromEnum(pkt.state);
    buf[1] = (state_bits << 6) | encodeFlags(pkt.flags);

    // Byte 2: Detect Mult
    buf[2] = pkt.detect_mult;

    // Byte 3: Length (always 24 for basic control packet)
    buf[3] = pkt.length;

    // Bytes 4-7: My Discriminator (big-endian)
    std.mem.writeInt(u32, buf[4..8], pkt.my_discr, .big);

    // Bytes 8-11: Your Discriminator (big-endian)
    std.mem.writeInt(u32, buf[8..12], pkt.your_discr, .big);

    // Bytes 12-15: Desired Min TX Interval (big-endian)
    std.mem.writeInt(u32, buf[12..16], pkt.desired_min_tx_interval, .big);

    // Bytes 16-19: Required Min RX Interval (big-endian)
    std.mem.writeInt(u32, buf[16..20], pkt.required_min_rx_interval, .big);

    // Bytes 20-23: Required Min Echo RX Interval (big-endian)
    std.mem.writeInt(u32, buf[20..24], pkt.required_min_echo_rx_interval, .big);

    return CONTROL_PACKET_LEN;
}

/// Decode a BFD control packet from a byte buffer.
/// Returns error.InvalidPacket if the packet is too short or has invalid version.
pub fn decode(buf: []const u8) error{InvalidPacket}!ControlPacket {
    if (buf.len < CONTROL_PACKET_LEN) return error.InvalidPacket;

    // Byte 0: Version (bits 7-5) + Diagnostic (bits 4-0)
    const version = (buf[0] >> 5) & 0x07;
    if (version != PROTOCOL_VERSION) return error.InvalidPacket;

    const diag_val = @as(u5, @truncate(buf[0]));
    const diag: Diagnostic = @enumFromInt(diag_val);

    // Byte 1: State (bits 7-6) + Flags (bits 5-0)
    const state_val = (buf[1] >> 6) & 0x03;
    const state: State = @enumFromInt(state_val);
    const flags = decodeFlags(buf[1] & 0x3F);

    // Byte 2: Detect Mult
    const detect_mult = buf[2];

    // Byte 3: Length
    const length = buf[3];
    if (length < CONTROL_PACKET_LEN) return error.InvalidPacket;

    // Bytes 4-7: My Discriminator
    const my_discr = std.mem.readInt(u32, buf[4..8], .big);

    // Bytes 8-11: Your Discriminator
    const your_discr = std.mem.readInt(u32, buf[8..12], .big);

    // Bytes 12-15: Desired Min TX Interval
    const desired_min_tx_interval = std.mem.readInt(u32, buf[12..16], .big);

    // Bytes 16-19: Required Min RX Interval
    const required_min_rx_interval = std.mem.readInt(u32, buf[16..20], .big);

    // Bytes 20-23: Required Min Echo RX Interval
    const required_min_echo_rx_interval = std.mem.readInt(u32, buf[20..24], .big);

    return ControlPacket{
        .version = @as(u3, @intCast(version)),
        .diag = diag,
        .state = state,
        .flags = flags,
        .detect_mult = detect_mult,
        .length = length,
        .my_discr = my_discr,
        .your_discr = your_discr,
        .desired_min_tx_interval = desired_min_tx_interval,
        .required_min_rx_interval = required_min_rx_interval,
        .required_min_echo_rx_interval = required_min_echo_rx_interval,
    };
}

/// Convert microseconds to milliseconds (rounded up).
pub fn usToMs(us: u32) u32 {
    return (us + 999) / 1000;
}

/// Convert milliseconds to microseconds.
pub fn msToUs(ms: u32) u32 {
    return ms * 1000;
}
