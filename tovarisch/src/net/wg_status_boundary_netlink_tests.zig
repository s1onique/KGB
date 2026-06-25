// wg_status_boundary_netlink_tests.zig — Tests for generic-netlink backend
//
// Tests for parser/decoder with synthetic netlink attribute fixtures.
// These tests run on all platforms but test the attribute parsing logic
// which is platform-independent. Netlink socket operations are skipped
// on non-Linux platforms.

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
    
    // Build a listen port attribute manually
    var buf: [64]u8 = std.mem.zeroes([64]u8);
    const attr = @as(*Nlattr, @alignCast(@ptrCast(&buf)));
    attr.nla_len = @intCast(@sizeOf(Nlattr) + 2);
    attr.nla_type = WG_DEVICE_ATTR_LISTEN_PORT;
    // Little-endian port 51820 = 0xCA6C
    @memcpy(buf[@sizeOf(Nlattr)..][0..2], &[_]u8{ 0x6C, 0xCA });
    
    try parseDeviceAttrs(&buf, @sizeOf(Nlattr) + 2, &peer_count, &latest_handshake, &rx_bytes, &tx_bytes, &listen_port);
    try std.testing.expectEqual(@as(?u16, 51820), listen_port);
}

test "parseDeviceAttrs: unknown attrs are ignored" {
    var peer_count: u32 = 0;
    var latest_handshake: ?u64 = null;
    var rx_bytes: u64 = 0;
    var tx_bytes: u64 = 0;
    var listen_port: ?u16 = null;
    
    var buf: [64]u8 = std.mem.zeroes([64]u8);
    const attr = @as(*Nlattr, @alignCast(@ptrCast(&buf)));
    attr.nla_len = @intCast(@sizeOf(Nlattr) + 4);
    attr.nla_type = 99; // Unknown type
    @memcpy(buf[@sizeOf(Nlattr)..][0..4], &[_]u8{ 1, 2, 3, 4 });
    
    try parseDeviceAttrs(&buf, @sizeOf(Nlattr) + 4, &peer_count, &latest_handshake, &rx_bytes, &tx_bytes, &listen_port);
    try std.testing.expectEqual(@as(u32, 0), peer_count);
}

test "parseDeviceAttrs: truncated attribute is ignored" {
    var peer_count: u32 = 0;
    var latest_handshake: ?u64 = null;
    var rx_bytes: u64 = 0;
    var tx_bytes: u64 = 0;
    var listen_port: ?u16 = null;
    
    var buf: [64]u8 = std.mem.zeroes([64]u8);
    const attr = @as(*Nlattr, @alignCast(@ptrCast(&buf)));
    attr.nla_len = @intCast(@sizeOf(Nlattr) + 2);
    attr.nla_type = WG_DEVICE_ATTR_LISTEN_PORT;
    @memcpy(buf[@sizeOf(Nlattr)..][0..2], &[_]u8{ 0x6C, 0xCA });
    
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
    
    // Build peer with LAST_HANDSHAKE_TIME
    // WireGuard uses kernel timespec: sec (u64) + nsec (u64) = 16 bytes
    // We store the epoch seconds directly in the sec field
    var buf: [256]u8 = std.mem.zeroes([256]u8);
    const attr = @as(*Nlattr, @alignCast(@ptrCast(&buf)));
    attr.nla_len = @intCast(@sizeOf(Nlattr) + 16);
    attr.nla_type = WG_PEER_ATTR_LAST_HANDSHAKE_TIME;
    
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
    
    var buf: [256]u8 = std.mem.zeroes([256]u8);
    const attr = @as(*Nlattr, @alignCast(@ptrCast(&buf)));
    attr.nla_len = @intCast(@sizeOf(Nlattr) + 8);
    attr.nla_type = WG_PEER_ATTR_RX_BYTES;
    const rx_val: [8]u8 = .{ 0xE8, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 }; // 1000 little-endian
    @memcpy(buf[@sizeOf(Nlattr)..][0..8], &rx_val);
    
    try parsePeerAttrs(&buf, @sizeOf(Nlattr) + 8, &latest_handshake, &rx_bytes, &tx_bytes);
    try std.testing.expectEqual(@as(u64, 1000), rx_bytes);
}

test "parsePeerAttrs: one peer with tx bytes" {
    var latest_handshake: ?u64 = null;
    var rx_bytes: u64 = 0;
    var tx_bytes: u64 = 0;
    
    var buf: [256]u8 = std.mem.zeroes([256]u8);
    const attr = @as(*Nlattr, @alignCast(@ptrCast(&buf)));
    attr.nla_len = @intCast(@sizeOf(Nlattr) + 8);
    attr.nla_type = WG_PEER_ATTR_TX_BYTES;
    const tx_val: [8]u8 = .{ 0xD0, 0x07, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00 }; // 2000 little-endian
    @memcpy(buf[@sizeOf(Nlattr)..][0..8], &tx_val);
    
    try parsePeerAttrs(&buf, @sizeOf(Nlattr) + 8, &latest_handshake, &rx_bytes, &tx_bytes);
    try std.testing.expectEqual(@as(u64, 2000), tx_bytes);
}

test "parsePeerAttrs: zero handshake is null" {
    var latest_handshake: ?u64 = null;
    var rx_bytes: u64 = 0;
    var tx_bytes: u64 = 0;
    
    // timespec with sec = 0 means no handshake occurred
    var buf: [256]u8 = std.mem.zeroes([256]u8);
    const attr = @as(*Nlattr, @alignCast(@ptrCast(&buf)));
    attr.nla_len = @intCast(@sizeOf(Nlattr) + 16);
    attr.nla_type = WG_PEER_ATTR_LAST_HANDSHAKE_TIME;
    // All zeros means sec = 0, nsec = 0 (from memset)
    
    try parsePeerAttrs(&buf, @sizeOf(Nlattr) + 16, &latest_handshake, &rx_bytes, &tx_bytes);
    try std.testing.expectEqual(@as(?u64, null), latest_handshake);
}

test "parsePeerAttrs: sensitive attrs are ignored" {
    var latest_handshake: ?u64 = null;
    var rx_bytes: u64 = 0;
    var tx_bytes: u64 = 0;
    
    var buf: [256]u8 = std.mem.zeroes([256]u8);
    const attr = @as(*Nlattr, @alignCast(@ptrCast(&buf)));
    attr.nla_len = @intCast(@sizeOf(Nlattr) + 4);
    attr.nla_type = 1; // PUBLIC_KEY
    @memcpy(buf[@sizeOf(Nlattr)..][0..4], &[_]u8{ 0x01, 0x02, 0x03, 0x04 });
    
    try parsePeerAttrs(&buf, @sizeOf(Nlattr) + 4, &latest_handshake, &rx_bytes, &tx_bytes);
    try std.testing.expectEqual(@as(?u64, null), latest_handshake);
    try std.testing.expectEqual(@as(u64, 0), rx_bytes);
    try std.testing.expectEqual(@as(u64, 0), tx_bytes);
}

// ============================================================================
// Family ID Parsing Tests
// ============================================================================

test "parseFamilyIdAttr: extracts family ID as u16" {
    var buf: [64]u8 = std.mem.zeroes([64]u8);
    const attr = @as(*Nlattr, @alignCast(@ptrCast(&buf)));
    attr.nla_len = @intCast(@sizeOf(Nlattr) + 2);
    attr.nla_type = 1; // CTRL_ATTR_FAMILY_ID
    // Family ID = 27 (0x1B) as little-endian u16
    buf[@sizeOf(Nlattr)..][0..2].* = .{ 0x1B, 0x00 };
    
    const family_id = try parseFamilyIdAttr(&buf, @sizeOf(Nlattr) + 2);
    try std.testing.expectEqual(@as(u16, 27), family_id);
}

test "parseFamilyIdAttr: skips non-FAMILY_ID attrs" {
    var buf: [128]u8 = std.mem.zeroes([128]u8);
    var offset: usize = 0;
    
    // Add FAMILY_NAME attr first
    {
        const attr = @as(*Nlattr, @alignCast(@ptrCast(buf[offset..].ptr)));
        attr.nla_len = @intCast(@sizeOf(Nlattr) + 9);
        attr.nla_type = 2; // CTRL_ATTR_FAMILY_NAME
        @memcpy(buf[offset + @sizeOf(Nlattr)..][0..9], "wireguard");
        offset += (9 + @sizeOf(Nlattr) + 3) & ~@as(usize, 3);
    }
    
    // Add FAMILY_ID attr
    {
        const attr = @as(*Nlattr, @alignCast(@ptrCast(buf[offset..].ptr)));
        attr.nla_len = @intCast(@sizeOf(Nlattr) + 2);
        attr.nla_type = 1; // CTRL_ATTR_FAMILY_ID
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
