// wg_status_boundary_netlink_consts.zig — Constants and types for WireGuard generic-netlink
//
// Part of wg_status_boundary_netlink.zig (split to satisfy LLM-friendliness limits).
// Contains: constants, data structures, helper functions, and parsing logic.

const std = @import("std");
const wg = @import("wg_status_boundary.zig");

// ============================================================================
// Generic Netlink Constants
// ============================================================================

/// Netlink socket protocol family.
pub const AF_NETLINK: c_int = 16;

/// Poll events: data available to read.
pub const POLLIN: c_short = 1;

/// Generic netlink controller family ID (CTRL_CMD_GETFAMILY response).
pub const GENERIC_NETLINK_CTRL_FAM_ID: u16 = 0x10;

/// Netlink message types.
pub const NLMSG_NOOP: u16 = 0x01;
pub const NLMSG_ERROR: u16 = 0x02;
pub const NLMSG_DONE: u16 = 0x03;
pub const NLMSG_OVERRUN: u16 = 0x04;

/// Netlink request flags.
pub const NLM_F_REQUEST: u32 = 0x001;
pub const NLM_F_ROOT: u32 = 0x100;
pub const NLM_F_MATCH: u32 = 0x200;
pub const NLM_F_DUMP: u32 = NLM_F_ROOT | NLM_F_MATCH;

/// Generic netlink controller commands.
pub const CTRL_CMD_UNSPEC: u8 = 0;
pub const CTRL_CMD_NEWFAMILY: u8 = 1;
pub const CTRL_CMD_DELFAMILY: u8 = 2;
pub const CTRL_CMD_GETFAMILY: u8 = 3;

/// Generic netlink controller attributes.
pub const CTRL_ATTR_FAMILY_ID: u16 = 1;
pub const CTRL_ATTR_FAMILY_NAME: u16 = 2;

/// WireGuard generic netlink family name.
pub const WG_GENL_NAME: [*:0]const u8 = "wireguard";

/// WireGuard generic netlink commands.
pub const WG_CMD_GET_DEVICE: u8 = 0;

/// WireGuard generic netlink version (from wireguard.h UAPI).
pub const WG_GENL_VERSION: u8 = 1;

/// WireGuard netlink attributes (nested).
pub const WG_DEVICE_ATTR_IFNAME: u16 = 2;
pub const WG_DEVICE_ATTR_LISTEN_PORT: u16 = 5;
pub const WG_DEVICE_ATTR_PEERS: u16 = 7;

/// WireGuard peer attributes.
pub const WG_PEER_ATTR_LAST_HANDSHAKE_TIME: u16 = 5;
pub const WG_PEER_ATTR_RX_BYTES: u16 = 6;
pub const WG_PEER_ATTR_TX_BYTES: u16 = 7;

// ============================================================================
// Netlink Data Structures
// ============================================================================

/// Netlink message header (nlmsghdr).
pub const Nlmsghdr = extern struct {
    nlmsg_len: u32,
    nlmsg_type: u16,
    nlmsg_flags: u16,
    nlmsg_seq: u32,
    nlmsg_pid: u32,
};

/// Generic netlink header (genlmsghdr).
pub const Genlmsghdr = extern struct {
    cmd: u8,
    version: u8,
    reserved: u16,
};

/// Netlink attribute header (nlattr).
pub const Nlattr = extern struct {
    nla_len: u16,
    nla_type: u16,

    /// Returns the payload length (total length minus header).
    pub fn payloadLen(self: *const Nlattr) u16 {
        return if (self.nla_len >= @sizeOf(Nlattr)) self.nla_len - @sizeOf(Nlattr) else 0;
    }

    /// Returns true if the attribute length is valid.
    pub fn isValid(self: *const Nlattr, total_len: usize) bool {
        return self.nla_len >= @sizeOf(Nlattr) and self.nla_len <= total_len;
    }

    /// Returns the payload as a u8 slice.
    pub fn payload(self: *const Nlattr) []const u8 {
        const payload_start = @sizeOf(Nlattr);
        const len = self.payloadLen();
        return @as([*]const u8, @ptrCast(self))[payload_start..][0..len];
    }
};

/// Netlink message header length in bytes.
pub const NLMSG_HDRLEN: usize = 16;

/// Generic netlink header length in bytes.
pub const GENL_HDRLEN: usize = 4;

/// Netlink attribute header length in bytes.
pub const NLA_HDRLEN: usize = 4;

// Compile-time assertions: ensure protocol length constants match struct sizes.
// This prevents silent breakage if structs are modified.
comptime {
    std.debug.assert(NLMSG_HDRLEN == @sizeOf(Nlmsghdr));
    std.debug.assert(GENL_HDRLEN == @sizeOf(Genlmsghdr));
    std.debug.assert(NLA_HDRLEN == @sizeOf(Nlattr));
}

/// Write a native-endian u32 into a byte buffer at the given offset.
/// This avoids pointer casts that require alignment guarantees.
inline fn writeU32Native(buf: []u8, offset: usize, value: u32) void {
    std.mem.writeInt(u32, buf[offset..][0..4], value, .native);
}

/// Write a native-endian u16 into a byte buffer at the given offset.
/// This avoids pointer casts that require alignment guarantees.
inline fn writeU16Native(buf: []u8, offset: usize, value: u16) void {
    std.mem.writeInt(u16, buf[offset..][0..2], value, .native);
}

/// Read a native-endian u32 from a byte buffer at the given offset.
/// This avoids pointer casts that require alignment guarantees.
inline fn readU32Native(buf: []const u8, offset: usize) u32 {
    return std.mem.readInt(u32, buf[offset..][0..4], .native);
}

/// Read a native-endian u16 from a byte buffer at the given offset.
/// This avoids pointer casts that require alignment guarantees.
inline fn readU16Native(buf: []const u8, offset: usize) u16 {
    return std.mem.readInt(u16, buf[offset..][0..2], .native);
}

/// Netlink message builder for fixed-size messages.
/// Uses byte-wise native-endian writes to avoid alignment issues with arbitrary buffers.
pub fn buildNlmsgHeader(msg: []u8, msg_len: u32, msg_type: u16, flags: u32, seq: u32, pid: u32) void {
    std.debug.assert(msg.len >= NLMSG_HDRLEN);
    writeU32Native(msg, 0, msg_len);
    writeU16Native(msg, 4, msg_type);
    writeU16Native(msg, 6, @as(u16, @truncate(flags)));
    writeU32Native(msg, 8, seq);
    writeU32Native(msg, 12, pid);
}

/// Build generic netlink header in message buffer.
/// Uses byte-wise writes to avoid alignment issues.
pub fn buildGenlHeader(msg: []u8, cmd: u8, version: u8) void {
    std.debug.assert(msg.len >= GENL_HDRLEN);
    msg[0] = cmd;
    msg[1] = version;
    msg[2] = 0;
    msg[3] = 0;
}

/// Add a netlink attribute to a message buffer.
/// Uses byte-wise writes to avoid alignment issues.
/// Returns the aligned attribute length (including header and padding), or null if insufficient space.
pub fn addNlattr(buf: []u8, offset: usize, max_len: usize, attr_type: u16, payload: []const u8) ?usize {
    const attr_total_len = NLA_HDRLEN + payload.len;
    const aligned_len = (attr_total_len + 3) & ~@as(usize, 3);
    if (offset + aligned_len > max_len) return null;
    if (offset + attr_total_len > buf.len) return null;

    writeU16Native(buf, offset + 0, @intCast(attr_total_len));
    writeU16Native(buf, offset + 2, attr_type);
    // MemoryCopySafety: buf and payload are independent buffers — no overlap
    @memcpy(buf[offset + NLA_HDRLEN..][0..payload.len], payload);

    // Zero padding bytes so netlink message doesn't contain stack garbage
    const padding_start = offset + attr_total_len;
    const padding_end = offset + aligned_len;
    if (padding_end > padding_start) {
        @memset(buf[padding_start..padding_end], 0);
    }

    return aligned_len;
}

// ============================================================================
// Parsing Functions (exposed for testing)
// ============================================================================

/// Parse family ID from attributes (u16 to support family IDs > 255).
pub fn parseFamilyIdAttr(attrs_start: [*]u8, attrs_len: usize) !u16 {
    var offset: usize = 0;

    while (offset + @sizeOf(Nlattr) <= attrs_len) {
        const attr = @as(*const Nlattr, @alignCast(@ptrCast(attrs_start + offset)));

        if (!attr.isValid(attrs_len - offset)) {
            break;
        }

        if (attr.nla_type == CTRL_ATTR_FAMILY_ID) {
            const payload = attr.payload();
            if (payload.len >= 2) {
                return std.mem.readInt(u16, @as(*const [2]u8, @ptrCast(payload.ptr)), .little);
            }
        }

        offset = (offset + attr.nla_len + 3) & ~@as(usize, 3);
    }

    return error.backend_missing;
}

/// Parse device-level attributes.
pub fn parseDeviceAttrs(
    attrs_start: [*]u8,
    attrs_len: usize,
    peer_count: *u32,
    latest_handshake: *?u64,
    rx_bytes: *u64,
    tx_bytes: *u64,
    listen_port: *?u16,
) !void {
    var offset: usize = 0;

    while (offset + @sizeOf(Nlattr) <= attrs_len) {
        const attr = @as(*const Nlattr, @alignCast(@ptrCast(attrs_start + offset)));

        if (!attr.isValid(attrs_len - offset)) {
            break;
        }

        switch (attr.nla_type) {
            WG_DEVICE_ATTR_LISTEN_PORT => {
                const payload = attr.payload();
                if (payload.len >= 2) {
                    listen_port.* = std.mem.readInt(u16, @as(*const [2]u8, @ptrCast(payload.ptr)), .little);
                }
            },
            WG_DEVICE_ATTR_PEERS => {
                const payload = attr.payload();
                try parsePeersAttrs(@constCast(payload.ptr), payload.len, peer_count, latest_handshake, rx_bytes, tx_bytes);
            },
            else => {},
        }

        offset = (offset + attr.nla_len + 3) & ~@as(usize, 3);
    }
}

/// Parse peer attributes from nested container.
pub fn parsePeersAttrs(
    peers_start: [*]u8,
    peers_len: usize,
    peer_count: *u32,
    latest_handshake: *?u64,
    rx_bytes: *u64,
    tx_bytes: *u64,
) !void {
    var offset: usize = 0;

    while (offset + @sizeOf(Nlattr) <= peers_len) {
        const peer_attr = @as(*const Nlattr, @alignCast(@ptrCast(peers_start + offset)));

        if (!peer_attr.isValid(peers_len - offset)) {
            break;
        }

        if (peer_attr.nla_type == 0) {
            const payload = peer_attr.payload();
            try parsePeerAttrs(@constCast(payload.ptr), payload.len, latest_handshake, rx_bytes, tx_bytes);
            peer_count.* += 1;
        }

        offset = (offset + peer_attr.nla_len + 3) & ~@as(usize, 3);
    }
}

/// Parse single peer attributes.
pub fn parsePeerAttrs(
    peer_start: [*]u8,
    peer_len: usize,
    latest_handshake: *?u64,
    rx_bytes: *u64,
    tx_bytes: *u64,
) !void {
    var offset: usize = 0;

    while (offset + @sizeOf(Nlattr) <= peer_len) {
        const attr = @as(*const Nlattr, @alignCast(@ptrCast(peer_start + offset)));

        if (!attr.isValid(peer_len - offset)) {
            break;
        }

        switch (attr.nla_type) {
            WG_PEER_ATTR_LAST_HANDSHAKE_TIME => {
                // WireGuard uses kernel timespec: sec (u64) + nsec (u64) = 16 bytes
                const payload = attr.payload();
                if (payload.len >= 16) {
                    const handshake_sec = std.mem.readInt(u64, @as(*const [8]u8, @ptrCast(payload.ptr)), .little);
                    // nsec = payload[8..16] (not used, just validating 16-byte length)
                    if (handshake_sec > 0) {
                        if (latest_handshake.* == null or handshake_sec > latest_handshake.*.?) {
                            latest_handshake.* = handshake_sec;
                        }
                    }
                }
            },
            WG_PEER_ATTR_RX_BYTES => {
                const payload = attr.payload();
                if (payload.len >= 8) {
                    rx_bytes.* +|= std.mem.readInt(u64, @as(*const [8]u8, @ptrCast(payload.ptr)), .little);
                }
            },
            WG_PEER_ATTR_TX_BYTES => {
                const payload = attr.payload();
                if (payload.len >= 8) {
                    tx_bytes.* +|= std.mem.readInt(u64, @as(*const [8]u8, @ptrCast(payload.ptr)), .little);
                }
            },
            else => {},
        }

        offset = (offset + attr.nla_len + 3) & ~@as(usize, 3);
    }
}

/// Netlink socket address (defined for cross-platform compatibility).
pub const sockaddr_nl = extern struct {
    nl_family: u16,
    nl_pad: u16,
    nl_pid: u32,
    nl_groups: u32,
};
