// wg_status_boundary_netlink_alignment_tests.zig — Alignment safety regression tests
//
// These tests verify that netlink builder functions work correctly with
// arbitrarily-aligned byte buffers, proving the @ptrCast/@alignCast panic
// is resolved for serialization code.
//
// Tests run on all platforms. Safe to execute anywhere.

const std = @import("std");
const netlink = @import("wg_status_boundary_netlink.zig");
const netlink_consts = @import("wg_status_boundary_netlink_consts.zig");

const buildNlmsgHeader = netlink_consts.buildNlmsgHeader;
const buildGenlHeader = netlink_consts.buildGenlHeader;
const addNlattr = netlink_consts.addNlattr;

test "buildNlmsgHeader works with misaligned buffer (offset 1)" {
    var raw: [64]u8 = undefined;
    const msg = raw[1..]; // deliberately misaligned (offset 1)

    buildNlmsgHeader(msg, 16, 0, netlink_consts.NLM_F_REQUEST, 1, 0);

    // Verify fields are written correctly using byte-wise reads
    try std.testing.expectEqual(@as(u32, 16), std.mem.readInt(u32, msg[0..4], .native));
    try std.testing.expectEqual(@as(u16, 0), std.mem.readInt(u16, msg[4..6], .native));
    try std.testing.expectEqual(@as(u16, netlink_consts.NLM_F_REQUEST), std.mem.readInt(u16, msg[6..8], .native));
    try std.testing.expectEqual(@as(u32, 1), std.mem.readInt(u32, msg[8..12], .native));
    try std.testing.expectEqual(@as(u32, 0), std.mem.readInt(u32, msg[12..16], .native));
}

test "buildNlmsgHeader works with misaligned buffer (offset 3)" {
    var raw: [64]u8 = undefined;
    const msg = raw[3..]; // deliberately misaligned (offset 3)

    buildNlmsgHeader(msg, 32, 0x10, netlink_consts.NLM_F_REQUEST | netlink_consts.NLM_F_DUMP, 42, 12345);

    // Verify all fields
    try std.testing.expectEqual(@as(u32, 32), std.mem.readInt(u32, msg[0..4], .native));
    try std.testing.expectEqual(@as(u16, 0x10), std.mem.readInt(u16, msg[4..6], .native));
    try std.testing.expectEqual(@as(u16, netlink_consts.NLM_F_REQUEST | netlink_consts.NLM_F_DUMP), std.mem.readInt(u16, msg[6..8], .native));
    try std.testing.expectEqual(@as(u32, 42), std.mem.readInt(u32, msg[8..12], .native));
    try std.testing.expectEqual(@as(u32, 12345), std.mem.readInt(u32, msg[12..16], .native));
}

test "buildGenlHeader works with misaligned buffer" {
    var raw: [64]u8 = undefined;
    const msg = raw[1..]; // deliberately misaligned

    buildGenlHeader(msg, netlink_consts.CTRL_CMD_GETFAMILY, netlink_consts.WG_GENL_VERSION);

    try std.testing.expectEqual(@as(u8, netlink_consts.CTRL_CMD_GETFAMILY), msg[0]);
    try std.testing.expectEqual(@as(u8, netlink_consts.WG_GENL_VERSION), msg[1]);
    try std.testing.expectEqual(@as(u8, 0), msg[2]);
    try std.testing.expectEqual(@as(u8, 0), msg[3]);
}

test "addNlattr works with misaligned buffer" {
    var raw: [64]u8 = std.mem.zeroes([64]u8);
    const msg = raw[1..]; // deliberately misaligned

    const payload: []const u8 = "test";
    const attr_len = addNlattr(msg, 0, msg.len, 5, payload);

    try std.testing.expect(attr_len != null);
    const attr_total_len = netlink_consts.NLA_HDRLEN + payload.len;
    try std.testing.expectEqual(@as(u16, attr_total_len), std.mem.readInt(u16, msg[0..2], .native));
    try std.testing.expectEqual(@as(u16, 5), std.mem.readInt(u16, msg[2..4], .native));
    try std.testing.expectEqualSlices(u8, payload, msg[netlink_consts.NLA_HDRLEN..][0..payload.len]);
}

test "buildNlmsgHeader + buildGenlHeader + addNlattr as full sequence" {
    var raw: [256]u8 = undefined;
    @memset(&raw, 0);
    const msg = raw[1..64]; // deliberately misaligned

    // Build netlink + genl header
    buildNlmsgHeader(msg, 0, 0, netlink_consts.NLM_F_REQUEST, 1, 0);
    const genl_offset = netlink_consts.NLMSG_HDRLEN;
    buildGenlHeader(msg[genl_offset..], netlink_consts.CTRL_CMD_GETFAMILY, netlink_consts.WG_GENL_VERSION);
    var offset = netlink_consts.NLMSG_HDRLEN + netlink_consts.GENL_HDRLEN;

    // Add attribute
    const name: []const u8 = "wireguard";
    if (addNlattr(msg, offset, msg.len, netlink_consts.CTRL_ATTR_FAMILY_NAME, name)) |attr_len| {
        offset += attr_len;
    }

    // Update message length
    buildNlmsgHeader(msg, @intCast(offset), netlink_consts.GENERIC_NETLINK_CTRL_FAM_ID, netlink_consts.NLM_F_REQUEST, 1, 0);

    // Verify
    try std.testing.expectEqual(@as(u32, @intCast(offset)), std.mem.readInt(u32, msg[0..4], .native));
    try std.testing.expectEqual(@as(u16, netlink_consts.GENERIC_NETLINK_CTRL_FAM_ID), std.mem.readInt(u16, msg[4..6], .native));
    try std.testing.expectEqual(@as(u8, netlink_consts.CTRL_CMD_GETFAMILY), msg[genl_offset + 0]);
    try std.testing.expectEqual(@as(u8, netlink_consts.WG_GENL_VERSION), msg[genl_offset + 1]);
}
