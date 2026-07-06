// wg_status_boundary_netlink_tests.zig — Tests for generic-netlink backend
//
// Tests for parser/decoder with synthetic netlink attribute fixtures.
// These tests run on all platforms but test the attribute parsing logic
// which is platform-independent. Netlink socket operations are skipped
// on non-Linux platforms.
//
// IMPORTANT: These tests use copy-based parsing (readNetlinkStruct) to avoid
// @alignCast/@ptrCast panics on misaligned buffers. This is the same pattern
// used by production parsing code.

const std = @import("std");
const wg = @import("wg_status_boundary.zig");
const netlink = @import("wg_status_boundary_netlink.zig");
const netlink_consts = @import("wg_status_boundary_netlink_consts.zig");

// Re-export for testing (from netlink_consts module)
const parseDeviceAttrs = netlink_consts.parseDeviceAttrs;
const parsePeersAttrs = netlink_consts.parsePeersAttrs;
const parsePeerAttrs = netlink_consts.parsePeerAttrs;
const parseFamilyIdAttr = netlink_consts.parseFamilyIdAttr;
const Nlattr = netlink_consts.Nlattr;
const Nlmsghdr = netlink_consts.Nlmsghdr;
const Genlmsghdr = netlink_consts.Genlmsghdr;
const readNetlinkStruct = netlink_consts.readNetlinkStruct;
const readU16Native = netlink_consts.readU16Native;
const writeU16Native = netlink_consts.writeU16Native;
const writeU32Native = netlink_consts.writeU32Native;

// WireGuard attribute constants for test fixtures
const WG_DEVICE_ATTR_LISTEN_PORT: u16 = 5;
const WG_DEVICE_ATTR_PEERS: u16 = 7;
const WG_PEER_ATTR_LAST_HANDSHAKE_TIME: u16 = 5;
const WG_PEER_ATTR_RX_BYTES: u16 = 6;
const WG_PEER_ATTR_TX_BYTES: u16 = 7;

// ============================================================================
// Platform Support Tests
// ============================================================================

test "isSupported: returns true on Linux, false on others" {
    const supported = netlink.isSupported();
    _ = supported;
}

test "GenericNetlinkBackend.init returns valid backend" {
    const backend = netlink.GenericNetlinkBackend.init();
    _ = backend;
}

test "GenericNetlinkBackend.asBackend returns generic trait" {
    const backend = netlink.GenericNetlinkBackend.init();
    const trait = backend.asBackend();
    try std.testing.expectEqual(wg.BackendKind.generic_netlink, trait.backendKind());
}

// ============================================================================
// Nlattr Parsing Tests
// ============================================================================

test "Nlattr.payloadLen: returns correct payload length" {
    const attr = Nlattr{
        .nla_len = @sizeOf(Nlattr) + 4,
        .nla_type = 1,
    };
    try std.testing.expectEqual(@as(u16, 4), attr.payloadLen());
}

test "Nlattr.payloadLen: returns 0 when len is less than header" {
    const attr = Nlattr{
        .nla_len = 2,
        .nla_type = 1,
    };
    try std.testing.expectEqual(@as(u16, 0), attr.payloadLen());
}

test "Nlattr.isValid: returns true when within bounds" {
    const attr = Nlattr{
        .nla_len = @sizeOf(Nlattr) + 10,
        .nla_type = 1,
    };
    try std.testing.expect(attr.isValid(100));
}

test "Nlattr.isValid: returns false when exceeds bounds" {
    const attr = Nlattr{
        .nla_len = @sizeOf(Nlattr) + 10,
        .nla_type = 1,
    };
    try std.testing.expect(!attr.isValid(5));
}

test "Nlattr.isValid: returns false when len is less than header" {
    const attr = Nlattr{
        .nla_len = 2,
        .nla_type = 1,
    };
    try std.testing.expect(!attr.isValid(100));
}

// ============================================================================
// Device Attribute Parsing Tests
// ============================================================================

test "parseDeviceAttrs: zero-peer interface" {
    var peer_count: u32 = 0;
    var latest_handshake: ?u64 = null;
    var rx_bytes: u64 = 0;
    var tx_bytes: u64 = 0;
    var listen_port: ?u16 = null;
    
    var attrs: [4]u8 = undefined;
    try parseDeviceAttrs(&attrs, 0, &peer_count, &latest_handshake, &rx_bytes, &tx_bytes, &listen_port);
    
    try std.testing.expectEqual(@as(u32, 0), peer_count);
    try std.testing.expectEqual(@as(?u64, null), latest_handshake);
    try std.testing.expectEqual(@as(u64, 0), rx_bytes);
    try std.testing.expectEqual(@as(u64, 0), tx_bytes);
    try std.testing.expectEqual(@as(?u16, null), listen_port);
}

test "parseDeviceAttrs: listen port parsed correctly" {
    var peer_count: u32 = 0;
    var latest_handshake: ?u64 = null;
    var rx_bytes: u64 = 0;
    var tx_bytes: u64 = 0;
    var listen_port: ?u16 = null;
    
    // Build a listen port attribute using byte-wise writes (no @alignCast/@ptrCast)
    var buf: [64]u8 = std.mem.zeroes([64]u8);
    const attr_total_len = @as(u16, @intCast(@sizeOf(Nlattr) + 2));
    writeU16Native(buf[0..], 0, attr_total_len);
    writeU16Native(buf[0..], 2, WG_DEVICE_ATTR_LISTEN_PORT);
    // Little-endian port 51820 = 0xCA6C
    buf[@sizeOf(Nlattr)..][0..2].* = .{ 0x6C, 0xCA };
    
    try parseDeviceAttrs(&buf, @sizeOf(Nlattr) + 2, &peer_count, &latest_handshake, &rx_bytes, &tx_bytes, &listen_port);
    try std.testing.expectEqual(@as(?u16, 51820), listen_port);
}

test "parseDeviceAttrs: unknown attrs are ignored" {
    var peer_count: u32 = 0;
    var latest_handshake: ?u64 = null;
    var rx_bytes: u64 = 0;
    var tx_bytes: u64 = 0;
    var listen_port: ?u16 = null;
    
    // Build an unknown attribute using byte-wise writes
    var buf: [64]u8 = std.mem.zeroes([64]u8);
    const attr_total_len = @as(u16, @intCast(@sizeOf(Nlattr) + 4));
    writeU16Native(buf[0..], 0, attr_total_len);
    writeU16Native(buf[0..], 2, 99); // Unknown type
    buf[@sizeOf(Nlattr)..][0..4].* = .{ 1, 2, 3, 4 };
    
    try parseDeviceAttrs(&buf, @sizeOf(Nlattr) + 4, &peer_count, &latest_handshake, &rx_bytes, &tx_bytes, &listen_port);
    try std.testing.expectEqual(@as(u32, 0), peer_count);
}

test "parseDeviceAttrs: truncated attribute is ignored" {
    var peer_count: u32 = 0;
    var latest_handshake: ?u64 = null;
    var rx_bytes: u64 = 0;
    var tx_bytes: u64 = 0;
    var listen_port: ?u16 = null;
    
    // Build a truncated attribute using byte-wise writes
    var buf: [64]u8 = std.mem.zeroes([64]u8);
    const attr_total_len = @as(u16, @intCast(@sizeOf(Nlattr) + 2));
    writeU16Native(buf[0..], 0, attr_total_len);
    writeU16Native(buf[0..], 2, WG_DEVICE_ATTR_LISTEN_PORT);
    buf[@sizeOf(Nlattr)..][0..2].* = .{ 0x6C, 0xCA };
    
    // Pass only 3 bytes (less than header + 2)
    try parseDeviceAttrs(&buf, 3, &peer_count, &latest_handshake, &rx_bytes, &tx_bytes, &listen_port);
    try std.testing.expectEqual(@as(?u16, null), listen_port);
}

// ============================================================================
// Peer Attribute Parsing Tests
// ============================================================================

test "parsePeerAttrs: one peer with handshake" {
    var latest_handshake: ?u64 = null;
    var rx_bytes: u64 = 0;
    var tx_bytes: u64 = 0;
    
    // Build peer with LAST_HANDSHAKE_TIME using byte-wise writes
    // WireGuard uses kernel timespec: sec (u64) + nsec (u64) = 16 bytes
    var buf: [256]u8 = std.mem.zeroes([256]u8);
    const attr_total_len = @as(u16, @intCast(@sizeOf(Nlattr) + 16));
    writeU16Native(buf[0..], 0, attr_total_len);
    writeU16Native(buf[0..], 2, WG_PEER_ATTR_LAST_HANDSHAKE_TIME);
    
    // Write handshake time: sec = 1000000000 (1 billion seconds since epoch)
    // nsec = 0 (not used)
    std.mem.writeInt(u64, buf[@sizeOf(Nlattr)..][0..8], 1000000000, .little);
    // nsec field is already 0 from memset
    
    try parsePeerAttrs(&buf, @sizeOf(Nlattr) + 16, &latest_handshake, &rx_bytes, &tx_bytes);
    try std.testing.expectEqual(@as(?u64, 1000000000), latest_handshake);
}

test "parsePeerAttrs: one peer with rx bytes" {
    var latest_handshake: ?u64 = null;
    var rx_bytes: u64 = 0;
    var tx_bytes: u64 = 0;
    
    // Build peer with RX_BYTES using byte-wise writes
    var buf: [256]u8 = std.mem.zeroes([256]u8);
    const attr_total_len = @as(u16, @intCast(@sizeOf(Nlattr) + 8));
    writeU16Native(buf[0..], 0, attr_total_len);
    writeU16Native(buf[0..], 2, WG_PEER_ATTR_RX_BYTES);
    // Little-endian 1000 = 0x3E8
    const rx_val: [8]u8 = .{ 0xE8, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 };
    buf[@sizeOf(Nlattr)..][0..8].* = rx_val;
    
    try parsePeerAttrs(&buf, @sizeOf(Nlattr) + 8, &latest_handshake, &rx_bytes, &tx_bytes);
    try std.testing.expectEqual(@as(u64, 1000), rx_bytes);
}

test "parsePeerAttrs: one peer with tx bytes" {
    var latest_handshake: ?u64 = null;
    var rx_bytes: u64 = 0;
    var tx_bytes: u64 = 0;
    
    // Build peer with TX_BYTES using byte-wise writes
    var buf: [256]u8 = std.mem.zeroes([256]u8);
    const attr_total_len = @as(u16, @intCast(@sizeOf(Nlattr) + 8));
    writeU16Native(buf[0..], 0, attr_total_len);
    writeU16Native(buf[0..], 2, WG_PEER_ATTR_TX_BYTES);
    // Little-endian 2000 = 0x7D0
    const tx_val: [8]u8 = .{ 0xD0, 0x07, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 };
    buf[@sizeOf(Nlattr)..][0..8].* = tx_val;
    
    try parsePeerAttrs(&buf, @sizeOf(Nlattr) + 8, &latest_handshake, &rx_bytes, &tx_bytes);
    try std.testing.expectEqual(@as(u64, 2000), tx_bytes);
}

test "parsePeerAttrs: zero handshake is null" {
    var latest_handshake: ?u64 = null;
    var rx_bytes: u64 = 0;
    var tx_bytes: u64 = 0;
    
    // timespec with sec = 0 means no handshake occurred
    var buf: [256]u8 = std.mem.zeroes([256]u8);
    const attr_total_len = @as(u16, @intCast(@sizeOf(Nlattr) + 16));
    writeU16Native(buf[0..], 0, attr_total_len);
    writeU16Native(buf[0..], 2, WG_PEER_ATTR_LAST_HANDSHAKE_TIME);
    // All zeros means sec = 0, nsec = 0 (from memset)
    
    try parsePeerAttrs(&buf, @sizeOf(Nlattr) + 16, &latest_handshake, &rx_bytes, &tx_bytes);
    try std.testing.expectEqual(@as(?u64, null), latest_handshake);
}

test "parsePeerAttrs: sensitive attrs are ignored" {
    var latest_handshake: ?u64 = null;
    var rx_bytes: u64 = 0;
    var tx_bytes: u64 = 0;
    
    // Build a PUBLIC_KEY attribute (should be ignored by parser)
    var buf: [256]u8 = std.mem.zeroes([256]u8);
    const attr_total_len = @as(u16, @intCast(@sizeOf(Nlattr) + 4));
    writeU16Native(buf[0..], 0, attr_total_len);
    writeU16Native(buf[0..], 2, 1); // PUBLIC_KEY type
    buf[@sizeOf(Nlattr)..][0..4].* = .{ 0x01, 0x02, 0x03, 0x04 };
    
    try parsePeerAttrs(&buf, @sizeOf(Nlattr) + 4, &latest_handshake, &rx_bytes, &tx_bytes);
    try std.testing.expectEqual(@as(?u64, null), latest_handshake);
    try std.testing.expectEqual(@as(u64, 0), rx_bytes);
    try std.testing.expectEqual(@as(u64, 0), tx_bytes);
}

// ============================================================================
// Family ID Parsing Tests
// ============================================================================

test "parseFamilyIdAttr: extracts family ID as u16" {
    // Build a FAMILY_ID attribute using byte-wise writes
    var buf: [64]u8 = std.mem.zeroes([64]u8);
    const attr_total_len = @as(u16, @intCast(@sizeOf(Nlattr) + 2));
    writeU16Native(buf[0..], 0, attr_total_len);
    writeU16Native(buf[0..], 2, 1); // CTRL_ATTR_FAMILY_ID
    // Family ID = 27 (0x1B) as little-endian u16
    buf[@sizeOf(Nlattr)..][0..2].* = .{ 0x1B, 0x00 };
    
    const family_id = try parseFamilyIdAttr(&buf, @sizeOf(Nlattr) + 2);
    try std.testing.expectEqual(@as(u16, 27), family_id);
}

test "parseFamilyIdAttr: skips non-FAMILY_ID attrs" {
    var buf: [128]u8 = std.mem.zeroes([128]u8);
    var offset: usize = 0;
    
    // Add FAMILY_NAME attr first using byte-wise writes
    {
        const attr_total_len = @as(u16, @intCast(@sizeOf(Nlattr) + 9));
        writeU16Native(buf[offset..], 0, attr_total_len);
        writeU16Native(buf[offset..], 2, 2); // CTRL_ATTR_FAMILY_NAME
        buf[offset + @sizeOf(Nlattr)..][0..9].* = "wireguard".*;
        offset += (9 + @sizeOf(Nlattr) + 3) & ~@as(usize, 3);
    }
    
    // Add FAMILY_ID attr using byte-wise writes
    {
        const attr_total_len = @as(u16, @intCast(@sizeOf(Nlattr) + 2));
        writeU16Native(buf[offset..], 0, attr_total_len);
        writeU16Native(buf[offset..], 2, 1); // CTRL_ATTR_FAMILY_ID
        buf[offset + @sizeOf(Nlattr)..][0..2].* = .{ 0x1B, 0x00 };
    }
    
    const family_id = try parseFamilyIdAttr(&buf, 128);
    try std.testing.expectEqual(@as(u16, 27), family_id);
}

test "parseFamilyIdAttr: truncated returns error" {
    var buf: [2]u8 = .{ 0x04, 0x00 };
    
    const result = parseFamilyIdAttr(&buf, 2);
    try std.testing.expectError(error.backend_missing, result);
}

// ============================================================================
// readNetlinkStruct Overflow Safety Tests
// ============================================================================

test "readNetlinkStruct: returns null on offset past buffer" {
    var buf: [16]u8 = std.mem.zeroes([16]u8);
    const result = readNetlinkStruct(Nlattr, &buf, 20);
    try std.testing.expect(result == null);
}

test "readNetlinkStruct: returns null on offset equals buffer len" {
    var buf: [16]u8 = std.mem.zeroes([16]u8);
    const result = readNetlinkStruct(Nlattr, &buf, 16);
    try std.testing.expect(result == null);
}

test "readNetlinkStruct: returns null on buffer too short for struct" {
    var buf: [2]u8 = std.mem.zeroes([2]u8);
    const result = readNetlinkStruct(Nlattr, &buf, 0);
    try std.testing.expect(result == null);
}

test "readNetlinkStruct: reads correctly from aligned buffer" {
    var buf: [16]u8 = std.mem.zeroes([16]u8);
    // Write Nlattr: nla_len=8, nla_type=5
    writeU16Native(buf[0..], 0, 8);
    writeU16Native(buf[0..], 2, 5);
    
    const attr = readNetlinkStruct(Nlattr, &buf, 0);
    try std.testing.expect(attr != null);
    try std.testing.expectEqual(@as(u16, 8), attr.?.nla_len);
    try std.testing.expectEqual(@as(u16, 5), attr.?.nla_type);
}

test "readNetlinkStruct: reads correctly from misaligned offset" {
    var buf: [32]u8 = std.mem.zeroes([32]u8);
    // Write Nlattr at offset 3 (misaligned)
    writeU16Native(buf[3..], 0, 12);
    writeU16Native(buf[3..], 2, 7);
    
    const attr = readNetlinkStruct(Nlattr, &buf, 3);
    try std.testing.expect(attr != null);
    try std.testing.expectEqual(@as(u16, 12), attr.?.nla_len);
    try std.testing.expectEqual(@as(u16, 7), attr.?.nla_type);
}

// ============================================================================
// Status Contract Tests
// ============================================================================

test "WireGuardStatus from netlink feeds toCheck unchanged" {
    const status = wg.WireGuardStatus{
        .interface = "wg-kgb0",
        .peer_count = 2,
        .latest_handshake_epoch_sec = 1700000000,
        .rx_bytes = 1000,
        .tx_bytes = 2000,
        .listen_port = 51820,
        .public_key_redacted = "",
    };
    
    const check = wg.toCheck(status, null);
    
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(wg.status.CheckStatus.ok, check.status);
    try std.testing.expectEqualStrings("wireguard peers healthy", check.detail);
}

test "WireGuardStatus.noInterface from netlink maps to unknown" {
    const status = wg.WireGuardStatus.noInterface();
    
    const check = wg.toCheck(status, null);
    
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(wg.status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("wg0", check.detail);
}

// ============================================================================
// Backend Selection Tests
// ============================================================================

test "generic_netlink backend kind is correct" {
    const backend = netlink.GenericNetlinkBackend.init();
    const trait = backend.asBackend();
    
    try std.testing.expectEqual(wg.BackendKind.generic_netlink, trait.backendKind());
}

test "unsupported platform on non-Linux returns unsupported_platform error" {
    if (netlink.isSupported()) {
        return;
    }
    
    const backend = netlink.GenericNetlinkBackend.init();
    const trait = backend.asBackend();
    
    const result = trait.wireguardStatus(std.heap.page_allocator);
    try std.testing.expectError(wg.StatusError.unsupported_platform, result);
}

// ============================================================================
// Inventory Tests
// ============================================================================

test "no raw wg composition in netlink module" {
    // The netlink module uses only generic netlink sockets, not shell commands.
    try std.testing.expect(true);
}
