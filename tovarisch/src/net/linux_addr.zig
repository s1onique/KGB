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

const std = @import("std");
const private_ip = @import("private_ip.zig");
const interface_filter = @import("interface_filter.zig");

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
// Helper Functions
// ============================================================================

/// Align a length to 4 bytes for netlink message alignment.
pub fn align4(len: usize) usize {
    return (len + 3) & ~@as(usize, 3);
}

/// Format an IPv4 address into a caller-provided buffer.
/// Returns a slice into the buffer on success.
pub fn formatIpv4(octets: [4]u8, buf: []u8) ![]const u8 {
    return std.fmt.bufPrint(buf, "{}.{}.{}.{}", .{
        octets[0],
        octets[1],
        octets[2],
        octets[3],
    });
}

/// Parse a null-terminated string from a buffer at a given offset.
/// Returns the string if found and non-empty.
pub fn parseLabel(buffer: []const u8, start_offset: usize, end_offset: usize) ?[]const u8 {
    // Check bounds before slicing
    if (start_offset >= buffer.len or end_offset > buffer.len) return null;
    if (start_offset >= end_offset) return null;
    const data_len = end_offset - start_offset;
    if (data_len < 1) return null;

    // Find null terminator or use full data
    const data = buffer[start_offset..end_offset];
    const null_pos = std.mem.indexOfScalar(u8, data, 0);
    const label_len = if (null_pos) |pos| pos else data_len;

    if (label_len == 0) return null;
    return data[0..label_len];
}

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
    const nlmsg_len = @sizeOf(nlmsghdr) + @sizeOf(ifaddrmsg);
    var request: [nlmsg_len]u8 = undefined;

    const nlhdr = @as(*nlmsghdr, @ptrCast(@alignCast(&request)));
    nlhdr.nlmsg_len = @intCast(nlmsg_len);
    nlhdr.nlmsg_type = @intCast(RTM_GETADDR);
    nlhdr.nlmsg_flags = @intCast(NLM_F_REQUEST | NLM_F_DUMP);
    nlhdr.nlmsg_seq = 1;
    nlhdr.nlmsg_pid = 0;

    // Fill ifaddrmsg - request all families (AF_UNSPEC), all prefixes
    const addrmsg_ptr = @as(*ifaddrmsg, @ptrCast(@constCast(&request[@sizeOf(nlmsghdr)..])));
    addrmsg_ptr.ifa_family = 0; // AF_UNSPEC = all families
    addrmsg_ptr.ifa_prefixlen = 0;
    addrmsg_ptr.ifa_flags = 0;
    addrmsg_ptr.ifa_scope = 0;
    addrmsg_ptr.ifa_index = 0;

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

            const nlhdr_ptr = @as(*const nlmsghdr, @ptrCast(@alignCast(&buffer[offset])));
            const response_len = @as(usize, @intCast(nlhdr_ptr.nlmsg_len));

            if (response_len < @sizeOf(nlmsghdr) or response_len > msg_len - offset) break;

            // NLMSG_DONE terminates the multipart message
            if (nlhdr_ptr.nlmsg_type == NLMSG_DONE) {
                done = true;
                break;
            }

            if (nlhdr_ptr.nlmsg_type == RTM_NEWADDR) {
                // Parse the ifaddrmsg header
                const addrmsg_offset = offset + @sizeOf(nlmsghdr);
                if (addrmsg_offset + @sizeOf(ifaddrmsg) > msg_len) {
                    offset += align4(response_len);
                    continue;
                }

                const addrmsg_hdr = @as(*const ifaddrmsg, @ptrCast(@alignCast(&buffer[addrmsg_offset])));
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

                    const attr_hdr = @as(*const rtattr, @ptrCast(@alignCast(&buffer[attr_pos])));
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

                        const attr_hdr = @as(*const rtattr, @ptrCast(@alignCast(&buffer[attr_pos])));
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

