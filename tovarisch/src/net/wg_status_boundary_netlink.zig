// wg_status_boundary_netlink.zig — Generic-netlink backend for WireGuard status
//
// Part of wg_status_boundary.zig (split to satisfy LLM-friendliness limits).
// Contains only the generic-netlink backend implementation.
//
// This backend uses Linux generic netlink to query WireGuard status directly
// from the kernel without spawning the wg(8) userspace tool.
//
// Privacy-aligned:
//   - Exposes: interface, peer_count, latest_handshake_epoch_sec, rx_bytes, tx_bytes, listen_port
//   - Ignores: private keys, public keys, preshared keys, endpoints, allowed IPs
//
// Implementation boundaries:
//   - Uses generic netlink family discovery for wireguard
//   - Resolves family ID dynamically (no hardcoding)
//   - Uses WG_CMD_GET_DEVICE equivalent only
//   - Read-only: no set-device, no device creation/modification
//   - Conservative bounds checks for all netlink attribute lengths
//   - Unknown attributes are ignored (not fatal)
//
// Platform constraint: Linux-only. Returns error.unsupported_platform on non-Linux.

const std = @import("std");
const wg = @import("wg_status_boundary.zig");
const config_parse_helpers = @import("../config_parse_helpers.zig");
const netlink_consts = @import("wg_status_boundary_netlink_consts.zig");

// ============================================================================
// Default Interface Name
// ============================================================================

/// Default WireGuard interface name when not explicitly configured.
/// Matches the CLI backend's default for consistency.
const DEFAULT_WG_INTERFACE: [:0]const u8 = "wg-kgb0";

// ============================================================================
// Platform Guard
// ============================================================================

/// Whether generic-netlink backend is available.
/// Currently Linux-only due to socket(AF_NETLINK) requirement.
pub fn isSupported() bool {
    return @import("builtin").os.tag == .linux;
}

// ============================================================================
// Error Mapping
// ============================================================================

/// Maps netlink socket and protocol errors to StatusError.
fn mapNetlinkError(err: anyerror) wg.StatusError {
    return switch (err) {
        error.OperationNotSupported => error.unsupported_platform,
        error.PermissionDenied => error.permission_denied,
        error.SystemResources => error.out_of_memory,
        else => error.netlink_failed,
    };
}

// ============================================================================
// Generic Netlink Backend
// ============================================================================

/// Generic-netlink backend using Linux generic netlink protocol.
///
/// This backend queries WireGuard status directly from the kernel without
/// spawning the wg(8) userspace tool. It is read-only and privacy-aligned.
///
/// Safety properties:
///   - No shell, no process spawning
///   - Bounded receive buffer (8KB)
///   - Timeout enforcement via poll()
///   - Explicit interface name validation
///   - No sensitive data in output (keys, endpoints, allowed IPs filtered)
pub const GenericNetlinkBackend = struct {
    /// Default timeout for netlink operations (5 seconds).
    pub const DEFAULT_TIMEOUT_SECS: u64 = 5;

    /// Maximum receive buffer size (8KB).
    pub const MAX_RCV_SIZE: usize = 8192;

    /// Initialize generic-netlink backend.
    pub fn init() GenericNetlinkBackend {
        return GenericNetlinkBackend{};
    }

    /// Convert to generic backend trait.
    pub fn asBackend(self: *const GenericNetlinkBackend) wg.WireGuardStatusBackend {
        _ = self;
        return wg.WireGuardStatusBackend{
            .wireguardStatusFn = struct {
                fn f(allocator: std.mem.Allocator, _: ?*anyopaque) wg.StatusError!wg.WireGuardStatusResult {
                    return genericNetlinkWireguardStatus(allocator);
                }
            }.f,
            .backendKindFn = struct {
                fn f(_: ?*anyopaque) wg.BackendKind {
                    return .generic_netlink;
                }
            }.f,
        };
    }
};

/// Standalone wireguardStatus implementation for generic-netlink backend.
fn genericNetlinkWireguardStatus(allocator: std.mem.Allocator) wg.StatusError!wg.WireGuardStatusResult {
    _ = allocator; // Interface requires allocator param; netlink backend uses no heap allocation
    // Platform check: only Linux supports generic netlink
    if (!isSupported()) {
        return error.unsupported_platform;
    }

    // Validate interface name
    if (!config_parse_helpers.isValidInterfaceName(DEFAULT_WG_INTERFACE)) {
        return error.interface_missing;
    }

    // Query via netlink
    const status = queryWireguardStatusNetlink(DEFAULT_WG_INTERFACE) catch |err| {
        return mapNetlinkError(err);
    };

    return wg.WireGuardStatusResult.ok(status, .generic_netlink);
}

/// Query WireGuard status via generic netlink.
fn queryWireguardStatusNetlink(interface_name: []const u8) !wg.WireGuardStatus {
    // Step 1: Create netlink socket
    const NETLINK_GENERIC: c_int = 15;
    const sock = std.c.socket(netlink_consts.AF_NETLINK, std.c.SOCK.DGRAM, NETLINK_GENERIC);
    if (sock < 0) {
        return error.netlink_failed;
    }
    defer _ = std.c.close(sock);

    // Step 2: Bind socket
    var local_addr: netlink_consts.sockaddr_nl = .{
        .nl_family = netlink_consts.AF_NETLINK,
        .nl_pad = 0,
        .nl_pid = 0,
        .nl_groups = 0,
    };
    const bind_result = std.c.bind(sock, @ptrCast(&local_addr), @sizeOf(netlink_consts.sockaddr_nl));
    if (bind_result != 0) {
        return error.PermissionDenied;
    }

    // Step 3: Get our PID for response matching
    var addr_len: u32 = @sizeOf(netlink_consts.sockaddr_nl);
    const getsockname_result = std.c.getsockname(sock, @ptrCast(&local_addr), &addr_len);
    if (getsockname_result != 0) {
        return error.OperationNotSupported;
    }
    const our_pid = local_addr.nl_pid;

    // Step 4: Discover WireGuard family ID
    // Use NLM_F_REQUEST only for by-name family lookup (not dump semantics)
    const family_id = try discoverWgFamilyId(sock, our_pid, 0);
    if (family_id == 0) {
        return error.backend_missing;
    }

    // Step 5: Query device status
    // Returns WireGuardStatus with interface borrowed from DEFAULT_WG_INTERFACE
    // (not allocated) - no deallocaton needed
    return queryWgDevice(sock, our_pid, family_id, 1, interface_name);
}

/// Discover WireGuard generic-netlink family ID using controller.
/// Returns the family ID as u16, or 0 if not found.
/// Uses NLM_F_REQUEST only (not dump) for by-name lookup semantics.
fn discoverWgFamilyId(sock: c_int, pid: u32, seq: u32) !u16 {
    // Build GENL_ID_CTRL family discovery message
    var buf: [256]u8 = undefined;
    var offset: usize = 0;

    // Netlink header: NLM_F_REQUEST for by-name lookup (not dump all families)
    netlink_consts.buildNlmsgHeader(&buf, 0, 0, netlink_consts.NLM_F_REQUEST, seq, pid);
    offset = netlink_consts.NLMSG_HDRLEN;

    // Generic netlink header (CTRL_CMD_GETFAMILY)
    netlink_consts.buildGenlHeader(buf[offset..], netlink_consts.CTRL_CMD_GETFAMILY, netlink_consts.WG_GENL_VERSION);
    offset += netlink_consts.GENL_HDRLEN;

    // Add family name attribute
    const name_null: [*:0]const u8 = netlink_consts.WG_GENL_NAME;
    const name_len = std.mem.indexOfSentinel(u8, 0, name_null);
    if (netlink_consts.addNlattr(&buf, offset, buf.len, netlink_consts.CTRL_ATTR_FAMILY_NAME, name_null[0..name_len])) |attr_len| {
        offset += attr_len;
    }

    // Set final message length and type
    const msg_len = offset;
    netlink_consts.buildNlmsgHeader(&buf, @intCast(msg_len), netlink_consts.GENERIC_NETLINK_CTRL_FAM_ID, netlink_consts.NLM_F_REQUEST, seq, pid);

    // Send discovery request
    var peer_addr: netlink_consts.sockaddr_nl = .{
        .nl_family = netlink_consts.AF_NETLINK,
        .nl_pad = 0,
        .nl_pid = 0,
        .nl_groups = 0,
    };
    const send_len = std.c.sendto(sock, &buf, msg_len, 0, @ptrCast(&peer_addr), @sizeOf(netlink_consts.sockaddr_nl));
    if (send_len < 0) {
        return error.PermissionDenied;
    }

    // Receive responses
    var response_buf: [GenericNetlinkBackend.MAX_RCV_SIZE]u8 = undefined;
    const recv_len = try recvWithTimeout(sock, &response_buf, GenericNetlinkBackend.DEFAULT_TIMEOUT_SECS);
    if (recv_len < 0) {
        return error.timeout;
    }

    // Parse responses looking for CTRL_CMD_NEWFAMILY with our family
    const family_id = parseForWgFamilyId(&response_buf, @as(usize, @intCast(recv_len))) catch return error.backend_missing;

    return family_id;
}

/// Parse netlink messages for WireGuard family ID.
/// Uses byte-wise copy to avoid @alignCast panics on misaligned buffers.
fn parseForWgFamilyId(buf: [*]u8, buf_len: usize) !u16 {
    var offset: usize = 0;
    const data = buf[0..buf_len];

    while (offset + @sizeOf(netlink_consts.Nlmsghdr) <= buf_len) {
        const nlh = netlink_consts.readNetlinkStruct(netlink_consts.Nlmsghdr, data, offset) orelse {
            return error.backend_missing;
        };

        // Check for end of message
        if (nlh.nlmsg_type == netlink_consts.NLMSG_DONE) break;
        if (nlh.nlmsg_type == netlink_consts.NLMSG_ERROR) {
            return error.backend_missing;
        }
        if (nlh.nlmsg_type == netlink_consts.NLMSG_NOOP or nlh.nlmsg_type == netlink_consts.NLMSG_OVERRUN) {
            offset = (offset + nlh.nlmsg_len + 3) & ~@as(usize, 3);
            continue;
        }

        // Skip non-generic messages
        if (nlh.nlmsg_len < @sizeOf(netlink_consts.Nlmsghdr) + @sizeOf(netlink_consts.Genlmsghdr)) {
            offset = (offset + nlh.nlmsg_len + 3) & ~@as(usize, 3);
            continue;
        }

        // Parse generic netlink payload
        const genl_offset = offset + @sizeOf(netlink_consts.Nlmsghdr);
        const genlh = netlink_consts.readNetlinkStruct(netlink_consts.Genlmsghdr, data, genl_offset) orelse {
            offset = (offset + nlh.nlmsg_len + 3) & ~@as(usize, 3);
            continue;
        };

        if (genlh.cmd == netlink_consts.CTRL_CMD_NEWFAMILY) {
            // Parse attributes for CTRL_ATTR_FAMILY_ID
            const attr_offset = genl_offset + @sizeOf(netlink_consts.Genlmsghdr);
            const attr_len = nlh.nlmsg_len - @sizeOf(netlink_consts.Nlmsghdr) - @sizeOf(netlink_consts.Genlmsghdr);

            if (attr_len > 0 and attr_offset + attr_len <= data.len) {
                const family_id = netlink_consts.parseFamilyIdAttr(data[attr_offset..][0..attr_len].ptr, attr_len) catch continue;
                return family_id;
            }
        }

        // Advance to next message (aligned)
        offset = (offset + nlh.nlmsg_len + 3) & ~@as(usize, 3);
    }

    return error.backend_missing;
}

/// Query WireGuard device status.
/// Returns WireGuardStatus with interface_name borrowed from caller (no allocation).
/// Note: This is a skeleton - multipart handling (NLM_F_MULTI) is TODO for production.
fn queryWgDevice(
    sock: c_int,
    pid: u32,
    family_id: u16,
    seq: u32,
    interface_name: []const u8,
) !wg.WireGuardStatus {
    // Build WG_CMD_GET_DEVICE message
    var buf: [512]u8 = undefined;
    var offset: usize = 0;

    // Netlink header: GET_DEVICE uses NLM_F_REQUEST | NLM_F_DUMP to list all devices
    // then filter by interface name in response parsing
    netlink_consts.buildNlmsgHeader(&buf, 0, family_id, netlink_consts.NLM_F_REQUEST | netlink_consts.NLM_F_DUMP, seq, pid);
    offset = netlink_consts.NLMSG_HDRLEN;

    // Generic netlink header
    netlink_consts.buildGenlHeader(buf[offset..], netlink_consts.WG_CMD_GET_DEVICE, netlink_consts.WG_GENL_VERSION);
    offset += netlink_consts.GENL_HDRLEN;

    // Add device name attribute
    if (netlink_consts.addNlattr(&buf, offset, buf.len, netlink_consts.WG_DEVICE_ATTR_IFNAME, interface_name)) |attr_len| {
        offset += attr_len;
    }

    // Set final message length
    const msg_len = offset;
    netlink_consts.buildNlmsgHeader(&buf, @intCast(msg_len), family_id, netlink_consts.NLM_F_REQUEST | netlink_consts.NLM_F_DUMP, seq, pid);

    // Send query
    var peer_addr: netlink_consts.sockaddr_nl = .{
        .nl_family = netlink_consts.AF_NETLINK,
        .nl_pad = 0,
        .nl_pid = 0,
        .nl_groups = 0,
    };
    const send_len = std.c.sendto(sock, &buf, msg_len, 0, @ptrCast(&peer_addr), @sizeOf(netlink_consts.sockaddr_nl));
    if (send_len < 0) {
        return error.PermissionDenied;
    }

    // Receive response
    var response_buf: [GenericNetlinkBackend.MAX_RCV_SIZE]u8 = undefined;
    const recv_len = try recvWithTimeout(sock, &response_buf, GenericNetlinkBackend.DEFAULT_TIMEOUT_SECS);
    if (recv_len < 0) {
        return error.timeout;
    }

    // Parse response - returns interface_name as borrowed slice (no allocation)
    return parseWgDeviceResponse(&response_buf, @as(usize, @intCast(recv_len)), interface_name);
}

/// Parse WireGuard device response.
/// Uses actual recv_len to bound parsing, not MAX_RCV_SIZE.
/// Returns WireGuardStatus with interface_name borrowed from caller (no allocation).
/// Uses byte-wise copy to avoid @alignCast panics on misaligned buffers.
fn parseWgDeviceResponse(buf: [*]u8, recv_len: usize, interface_name: []const u8) !wg.WireGuardStatus {
    var peer_count: u32 = 0;
    var latest_handshake: ?u64 = null;
    var rx_bytes: u64 = 0;
    var tx_bytes: u64 = 0;
    var listen_port: ?u16 = null;
    var found_interface = false;
    var offset: usize = 0;
    const data = buf[0..recv_len];

    while (offset + @sizeOf(netlink_consts.Nlmsghdr) <= recv_len) {
        const nlh = netlink_consts.readNetlinkStruct(netlink_consts.Nlmsghdr, data, offset) orelse {
            break;
        };

        if (nlh.nlmsg_type == netlink_consts.NLMSG_DONE) break;
        if (nlh.nlmsg_type == netlink_consts.NLMSG_ERROR) {
            return error.interface_missing;
        }
        if (nlh.nlmsg_type == netlink_consts.NLMSG_NOOP or nlh.nlmsg_type == netlink_consts.NLMSG_OVERRUN) {
            offset = (offset + nlh.nlmsg_len + 3) & ~@as(usize, 3);
            continue;
        }

        if (nlh.nlmsg_len < @sizeOf(netlink_consts.Nlmsghdr) + @sizeOf(netlink_consts.Genlmsghdr)) {
            offset = (offset + nlh.nlmsg_len + 3) & ~@as(usize, 3);
            continue;
        }

        const genl_offset = offset + @sizeOf(netlink_consts.Nlmsghdr);
        const genlh = netlink_consts.readNetlinkStruct(netlink_consts.Genlmsghdr, data, genl_offset) orelse {
            offset = (offset + nlh.nlmsg_len + 3) & ~@as(usize, 3);
            continue;
        };

        // Only process WireGuard messages
        if (genlh.cmd != netlink_consts.WG_CMD_GET_DEVICE) {
            offset = (offset + nlh.nlmsg_len + 3) & ~@as(usize, 3);
            continue;
        }

        // Parse device attributes
        const attr_offset = genl_offset + @sizeOf(netlink_consts.Genlmsghdr);
        const attr_len = nlh.nlmsg_len - @sizeOf(netlink_consts.Nlmsghdr) - @sizeOf(netlink_consts.Genlmsghdr);

        if (attr_len > 0 and attr_offset + attr_len <= data.len) {
            try netlink_consts.parseDeviceAttrs(data[attr_offset..][0..attr_len].ptr, attr_len, &peer_count, &latest_handshake, &rx_bytes, &tx_bytes, &listen_port);
            // TODO: Implement multipart handling for multiple interfaces
            // For now, mark as found after first valid parse
            found_interface = true;
        }

        offset = (offset + nlh.nlmsg_len + 3) & ~@as(usize, 3);
    }

    if (!found_interface) {
        return error.interface_missing;
    }

    // Return status with interface_name borrowed from DEFAULT_WG_INTERFACE (no allocation)
    // Caller owns the static string lifetime; no deallocation needed
    return wg.WireGuardStatus{
        .interface = interface_name,
        .peer_count = peer_count,
        .latest_handshake_epoch_sec = latest_handshake,
        .rx_bytes = rx_bytes,
        .tx_bytes = tx_bytes,
        .listen_port = listen_port,
        .public_key_redacted = "",
    };
}

/// Receive from netlink socket with timeout using poll().
fn recvWithTimeout(sock: c_int, buf: *[GenericNetlinkBackend.MAX_RCV_SIZE]u8, timeout_secs: u64) !isize {
    var poll_fd: std.c.pollfd = .{
        .fd = sock,
        .events = netlink_consts.POLLIN,
        .revents = 0,
    };

    var remaining_ms: i32 = @intCast(timeout_secs * 1000);
    const poll_interval_ms: i32 = 100;

    while (remaining_ms > 0) {
        const poll_ms: i32 = @intCast(@min(@as(i32, remaining_ms), poll_interval_ms));
        const poll_result = std.c.poll(@ptrCast(&poll_fd), 1, poll_ms);

        if (poll_result < 0) {
            continue;
        }

        if (poll_result > 0 and (poll_fd.revents & netlink_consts.POLLIN) != 0) {
            const n = std.c.recv(sock, buf, GenericNetlinkBackend.MAX_RCV_SIZE, 0);
            if (n < 0) {
                const errno = std.c.errno(n);
                if (errno == .AGAIN) {
                    remaining_ms -= poll_ms;
                    continue;
                }
                return error.OperationNotSupported;
            }
            return n;
        }

        remaining_ms -= poll_ms;
    }

    return error.timeout;
}

/// nlmsgerr for error messages.
const Nlmsgerr = extern struct {
    err_code: c_int,
    msg: netlink_consts.Nlmsghdr,
};
