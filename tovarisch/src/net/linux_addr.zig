// linux_addr.zig — Live Linux IPv4 address discovery via rtnetlink
//
// ACT 5f: Enumerate IPv4 addresses on network interfaces using rtnetlink.
//
// This module reads live interface addresses from the Linux kernel via
// NETLINK_ROUTE sockets. It provides IPv4 address-to-interface mappings
// for private-interface filtering.
//
// Scope:
// - NETLINK_ROUTE socket creation
// - RTM_GETADDR request/response
// - IPv4 address parsing (no IPv6 per deferral)
// - Returns InterfaceAddress structs for interface_filter consumption
//
// Non-goals:
// - No IPv6 support (deferred to future ACT)
// - No /metrics.json wiring
// - No sysfs fallback
//
// Alignment Doctrine:
// - Raw []u8 network/kernel buffers are byte-aligned.
// - Do NOT @alignCast them into extern structs.
// - Copy bytes into aligned local structs using readStruct() instead.

const std = @import("std");
const private_ip = @import("private_ip.zig");
const interface_filter = @import("interface_filter.zig");
const linux_addr_parse = @import("linux_addr_parse.zig");

// Re-export parser helpers for convenience
pub const align4 = linux_addr_parse.align4;
pub const formatIpv4 = linux_addr_parse.formatIpv4;
pub const parseLabel = linux_addr_parse.parseLabel;

// ============================================================================
// Alignment-Safe Struct Reading
// ============================================================================

/// Reads a struct from a byte buffer by copying bytes into a properly aligned
/// local variable. This avoids panics from @alignCast when the byte offset
/// is not aligned for the extern struct.
///
/// This pattern is required for network/kernel byte buffers that may arrive
/// at arbitrary byte offsets.
fn readStruct(comptime T: type, bytes: []const u8) AddrError!T {
    if (bytes.len < @sizeOf(T)) return error.InvalidMessage;

    var value: T = undefined;
    @memcpy(std.mem.asBytes(&value), bytes[0..@sizeOf(T)]);
    return value;
}

// ============================================================================
// C Socket Definitions
// ============================================================================

// Netlink socket family (AF_NETLINK = 16)
const AF_NETLINK: c_int = 16;

// Netlink protocol family
const NETLINK_ROUTE: c_int = 0;

// Socket type for netlink - SOCK_RAW = 3 on Linux
const SOCK_RAW: c_int = 3;

// rtnetlink message types
const RTM_GETADDR: c_uint = 20; // 0x14
const RTM_NEWADDR: c_uint = 20; // Same value, response type
const NLMSG_DONE: c_uint = 3;

// RTM flags
const NLM_F_REQUEST: c_uint = 0x001;
const NLM_F_DUMP: c_uint = 0x003; // NLM_F_ROOT | NLM_F_MATCH

// Address family
const AF_INET: c_int = 2;
const AF_UNSPEC: c_int = 0;

// ============================================================================
// C Structures (packed for netlink)
// ============================================================================

// Netlink message header (nlmsghdr)
const nlmsghdr = extern struct {
    nlmsg_len: c_uint,
    nlmsg_type: c_ushort,
    nlmsg_flags: c_ushort,
    nlmsg_seq: c_uint,
    nlmsg_pid: c_uint,
};

// ifaddrmsg (rtnetlink address message)
const ifaddrmsg = extern struct {
    ifa_family: u8,
    ifa_prefixlen: u8,
    ifa_flags: u8,
    ifa_scope: u8,
    ifa_index: c_uint,
};

// rtattr (routing attribute)
const rtattr = extern struct {
    rta_len: c_ushort,
    rta_type: c_ushort,
};

// Attribute type constants
const IFA_ADDRESS: c_ushort = 1;
const IFA_LOCAL: c_ushort = 2;
const IFA_LABEL: c_ushort = 3;

// sockaddr_nl - Linux netlink address structure
const sockaddr_nl = extern struct {
    nl_family: c_ushort,
    nl_pad: c_short,
    nl_pid: c_uint,
    nl_groups: c_uint,
};

// ============================================================================
// Errors
// ============================================================================

pub const AddrError = error{
    SocketCreateFailed,
    BindFailed,
    SendFailed,
    RecvFailed,
    InvalidMessage,
    InvalidAttribute,
    OutOfMemory,
    MissingInterfaceName,
};

// ============================================================================
// Core Discovery Function
// ============================================================================

/// Discovers private IPv4 addresses on network interfaces using rtnetlink.
///
/// Uses a NETLINK_ROUTE socket to query kernel for interface addresses.
/// Returns an allocator-owned list of InterfaceAddress structs containing
/// IPv4 address to interface name mappings.
///
/// This function only returns private IPv4 addresses (RFC1918: 10.x.x.x,
/// 172.16.x.x-172.31.x.x, 192.168.x.x). Public, loopback, link-local,
/// multicast, and IPv6 addresses are excluded per the IPv4-only scope.
///
/// Errors:
/// - Socket creation or I/O failures
/// - OutOfMemory for allocator operations
pub fn discoverPrivateAddresses(
    allocator: std.mem.Allocator,
    sys_class_net: []const u8,
) AddrError![]interface_filter.InterfaceAddress {
    _ = sys_class_net; // Currently unused; rtnetlink doesn't use sysfs

    // Create NETLINK_ROUTE socket with AF_NETLINK family
    const sock = std.c.socket(AF_NETLINK, SOCK_RAW, NETLINK_ROUTE);
    if (sock < 0) return error.SocketCreateFailed;
    defer _ = std.c.close(sock);

    // Build netlink request for RTM_GETADDR
    // Build local aligned structs and copy into request buffer to avoid
    // alignment issues when casting from byte arrays.
    const nlmsg_len = @sizeOf(nlmsghdr) + @sizeOf(ifaddrmsg);
    var request: [nlmsg_len]u8 = undefined;

    var hdr = nlmsghdr{
        .nlmsg_len = @intCast(nlmsg_len),
        .nlmsg_type = @intCast(RTM_GETADDR),
        .nlmsg_flags = @intCast(NLM_F_REQUEST | NLM_F_DUMP),
        .nlmsg_seq = 1,
        .nlmsg_pid = 0,
    };

    var msg = ifaddrmsg{
        .ifa_family = 0, // AF_UNSPEC = all families
        .ifa_prefixlen = 0,
        .ifa_flags = 0,
        .ifa_scope = 0,
        .ifa_index = 0,
    };

    @memcpy(request[0..@sizeOf(nlmsghdr)], std.mem.asBytes(&hdr));
    @memcpy(request[@sizeOf(nlmsghdr)..][0..@sizeOf(ifaddrmsg)], std.mem.asBytes(&msg));

    // Send request to kernel
    var nl_addr: sockaddr_nl = undefined;
    nl_addr.nl_family = AF_NETLINK;
    nl_addr.nl_pad = 0;
    nl_addr.nl_pid = 0;
    nl_addr.nl_groups = 0;

    const send_result = std.c.sendto(
        sock,
        @ptrCast(&request),
        request.len,
        0,
        @as(*const std.c.sockaddr, @ptrCast(&nl_addr)),
        @sizeOf(sockaddr_nl),
    );
    if (send_result < 0) return error.SendFailed;

    // Collect addresses
    var addresses = std.ArrayList(interface_filter.InterfaceAddress).empty;
    errdefer {
        for (addresses.items) |addr_| {
            allocator.free(addr_.iface);
            allocator.free(addr_.address);
        }
        addresses.deinit(allocator);
    }

    // Receive responses
    var buffer: [16384]u8 = undefined;
    var done = false;

    while (!done) {
        const recv_result = std.c.recv(sock, @ptrCast(&buffer), buffer.len, 0);
        if (recv_result < 0) return error.RecvFailed;
        if (recv_result == 0) break;

        const msg_len = @as(usize, @intCast(recv_result));
        var offset: usize = 0;

        while (offset < msg_len) {
            if (offset + @sizeOf(nlmsghdr) > msg_len) break;

            const nlhdr = try readStruct(nlmsghdr, buffer[offset..msg_len]);
            const response_len = @as(usize, @intCast(nlhdr.nlmsg_len));

            if (response_len < @sizeOf(nlmsghdr) or response_len > msg_len - offset) break;

            // NLMSG_DONE terminates the multipart message
            if (nlhdr.nlmsg_type == NLMSG_DONE) {
                done = true;
                break;
            }

            if (nlhdr.nlmsg_type == RTM_NEWADDR) {
                // Parse the ifaddrmsg header
                const addrmsg_offset = offset + @sizeOf(nlmsghdr);
                if (addrmsg_offset + @sizeOf(ifaddrmsg) > msg_len) {
                    offset += align4(response_len);
                    continue;
                }

                const addrmsg_hdr = try readStruct(ifaddrmsg, buffer[addrmsg_offset..msg_len]);
                _ = addrmsg_hdr.ifa_index; // Available if needed for logging

                // Only process IPv4 addresses (AF_INET)
                if (addrmsg_hdr.ifa_family != AF_INET) {
                    offset += align4(response_len);
                    continue;
                }

                // Parse attributes
                const attr_offset = addrmsg_offset + @sizeOf(ifaddrmsg);
                const msg_end = offset + response_len;
                var attr_pos = attr_offset;

                var address_octets: [4]u8 = undefined;
                var has_address = false;
                var interface_name: ?[]const u8 = null;

                while (attr_pos < msg_end) {
                    if (attr_pos + @sizeOf(rtattr) > msg_end) break;

                    const attr_hdr = try readStruct(rtattr, buffer[attr_pos..msg_end]);
                    const attr_full_len = @as(usize, @intCast(attr_hdr.rta_len));
                    const attr_type = @as(c_ushort, @intCast(attr_hdr.rta_type));

                    if (attr_full_len < @sizeOf(rtattr) or attr_full_len > msg_end - attr_pos) break;

                    // Attribute data starts after the rtattr header
                    const data_start = attr_pos + @sizeOf(rtattr);
                    const data_end = attr_pos + attr_full_len;

                    if (attr_type == IFA_LABEL and data_end > data_start) {
                        // Parse interface label
                        if (parseLabel(&buffer, data_start, data_end)) |label| {
                            interface_name = label;
                        }
                    }

                    if (attr_type == IFA_ADDRESS and data_end >= data_start + 4) {
                        @memcpy(address_octets[0..4], buffer[data_start .. data_start + 4]);
                        has_address = true;
                    }

                    attr_pos += align4(attr_full_len);
                }

                // Fallback: try IFA_LOCAL if no IFA_ADDRESS found
                if (!has_address) {
                    attr_pos = attr_offset;
                    while (attr_pos < msg_end) {
                        if (attr_pos + @sizeOf(rtattr) > msg_end) break;

                        const attr_hdr = try readStruct(rtattr, buffer[attr_pos..msg_end]);
                        const attr_full_len = @as(usize, @intCast(attr_hdr.rta_len));
                        const attr_type = @as(c_ushort, @intCast(attr_hdr.rta_type));

                        if (attr_full_len < @sizeOf(rtattr) or attr_full_len > msg_end - attr_pos) break;

                        const data_start = attr_pos + @sizeOf(rtattr);
                        const data_end = attr_pos + attr_full_len;

                        if (attr_type == IFA_LOCAL and data_end >= data_start + 4) {
                            @memcpy(address_octets[0..4], buffer[data_start .. data_start + 4]);
                            has_address = true;
                            break;
                        }

                        attr_pos += align4(attr_full_len);
                    }
                }

                // Classify address and filter for private
                if (has_address and interface_name != null) {
                    const classification = private_ip.classifyIpv4Octets(address_octets);

                    if (classification == .private) {
                        const iface_name = interface_name.?;
                        var addr_buf: [15]u8 = undefined;
                        const addr_str = formatIpv4(address_octets, &addr_buf) catch return error.InvalidMessage;

                        const iface_copy = try allocator.dupe(u8, iface_name);
                        errdefer allocator.free(iface_copy);

                        const addr_copy = try allocator.dupe(u8, addr_str);
                        errdefer allocator.free(addr_copy);

                        try addresses.append(allocator, .{
                            .iface = iface_copy,
                            .address = addr_copy,
                        });
                    }
                }
            }

            // Advance by aligned response message length, not request length
            offset += align4(response_len);
        }
    }

    return try addresses.toOwnedSlice(allocator);
}

/// Frees addresses returned by discoverPrivateAddresses.
pub fn freeAddresses(allocator: std.mem.Allocator, addresses: []interface_filter.InterfaceAddress) void {
    for (addresses) |addr| {
        allocator.free(addr.iface);
        allocator.free(addr.address);
    }
    allocator.free(addresses);
}

