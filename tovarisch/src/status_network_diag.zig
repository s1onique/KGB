// status_network_diag.zig — Network diagnostics status integration
//
// ACT: Add tovarisch WireGuard and XRay TCP underlay diagnostics

const std = @import("std");
const Io = std.Io;
const network_diag_config = @import("net/network_diag_config.zig");
const diag_event_ring = @import("net/diag_event_ring.zig");
const wg_dump_parser = @import("net/wg_dump_parser.zig");
const extended_interface_stats = @import("net/extended_interface_stats.zig");
const route_diag = @import("net/route_diag.zig");
const ss_parser = @import("net/ss_parser.zig");
const safe_command = @import("net/safe_command.zig");

// ============================================================================
// Types
// ============================================================================

/// Network diagnostics section status.
pub const NetworkDiagStatus = enum {
    ok,
    warning,
    @"error",
    unavailable,
    disabled,
};

/// WireGuard interface diagnostic output.
pub const WgInterfaceOutput = struct {
    name: []const u8,
    status: []const u8,
    peers: []WgPeerOutput,
};

/// WireGuard peer diagnostic output.
pub const WgPeerOutput = struct {
    public_key: []const u8,
    endpoint: []const u8,
    allowed_ips: []const u8,
    latest_handshake_at: ?[]const u8,
    latest_handshake_age_seconds: ?u64,
    transfer_rx_bytes: u64,
    transfer_tx_bytes: u64,
    transfer_rx_bytes_delta: ?i64,
    transfer_tx_bytes_delta: ?i64,
    persistent_keepalive_seconds: u16,
};

/// Interface diagnostic output.
pub const InterfaceOutput = struct {
    name: []const u8,
    operstate: []const u8,
    carrier: ?bool,
    rx_bytes: u64,
    tx_bytes: u64,
    rx_packets: u64,
    tx_packets: u64,
    rx_errors: u64,
    tx_errors: u64,
    rx_dropped: u64,
    tx_dropped: u64,
    rx_errors_delta: i64,
    tx_errors_delta: i64,
    rx_dropped_delta: i64,
    tx_dropped_delta: i64,
};

/// Route diagnostic output.
pub const RouteOutput = struct {
    target: []const u8,
    interface: []const u8,
    source: []const u8,
    gateway: ?[]const u8,
    status: []const u8,
};

/// TCP underlay socket output.
pub const TcpSocketOutput = struct {
    name: []const u8,
    state: []const u8,
    local: []const u8,
    remote: []const u8,
    rtt_ms: ?f64,
    rttvar_ms: ?f64,
    rto_ms: ?u64,
    retransmits: ?u64,
    unacked: ?u64,
    cwnd: ?u32,
    send_queue_bytes: ?u64,
    recv_queue_bytes: ?u64,
    status: []const u8,
};

/// Event output for JSON.
pub const EventOutput = struct {
    ts: []const u8,
    severity: []const u8,
    source: []const u8,
    message: []const u8,
    fields: ?[]const u8,
};

/// Complete network diagnostics status.
///
/// Ownership model:
/// - `started_at` is always allocator-owned (caller must free via deinit).
/// - All slices (interfaces, routes, underlay_tcp, events, wireguard) are allocator-owned.
/// - All string fields are allocator-owned and must be freed by deinit():
///   - TcpSocketOutput: name, state, local, remote, status (duplicated from parser)
///   - InterfaceOutput: name, operstate (duplicated from extended_interface_stats)
///   - RouteOutput: target, interface, source, gateway, status
///   - EventOutput: ts, severity, source, message, fields
///   - WireGuard structs: name, status, public_key, endpoint, allowed_ips, latest_handshake_at
pub const NetworkDiag = struct {
    started_at: []const u8,
    status: NetworkDiagStatus,
    wireguard: ?WireguardDiagSection,
    interfaces: []InterfaceOutput,
    routes: []RouteOutput,
    underlay_tcp: []TcpSocketOutput,
    events: []EventOutput,

    /// Deinitialize network diagnostics and free all owned memory.
    /// Safe for all NetworkDiag payloads (disabled, unavailable, normal).
    pub fn deinit(self: *NetworkDiag, allocator: std.mem.Allocator) void {
        allocator.free(self.started_at);
        for (self.underlay_tcp) |socket| {
            allocator.free(socket.name);
            allocator.free(socket.state);
            allocator.free(socket.local);
            allocator.free(socket.remote);
            allocator.free(socket.status);
        }
        allocator.free(self.underlay_tcp);
        for (self.interfaces) |iface| {
            allocator.free(iface.name);
            allocator.free(iface.operstate);
        }
        allocator.free(self.interfaces);
        for (self.routes) |route| {
            allocator.free(route.target);
            allocator.free(route.interface);
            allocator.free(route.source);
            if (route.gateway) |gw| allocator.free(gw);
            allocator.free(route.status);
        }
        allocator.free(self.routes);
        for (self.events) |event| {
            allocator.free(event.ts);
            allocator.free(event.severity);
            allocator.free(event.source);
            allocator.free(event.message);
            if (event.fields) |f| allocator.free(f);
        }
        allocator.free(self.events);
        if (self.wireguard) |wg| {
            for (wg.interfaces) |iface| {
                allocator.free(iface.name);
                allocator.free(iface.status);
                for (iface.peers) |peer| {
                    allocator.free(peer.public_key);
                    allocator.free(peer.endpoint);
                    allocator.free(peer.allowed_ips);
                    if (peer.latest_handshake_at) |ts| allocator.free(ts);
                }
                allocator.free(iface.peers);
            }
            allocator.free(wg.interfaces);
        }
        self.* = undefined;
    }
};

/// WireGuard diagnostics section.
pub const WireguardDiagSection = struct {
    interfaces: []WgInterfaceOutput,
    status: NetworkDiagStatus,
};

// ============================================================================
// Collection
// ============================================================================

fn wallClockMs() i64 {
    if (comptime @import("builtin").os.tag == .linux and @hasDecl(std.os.linux, "clock_gettime")) {
        var ts: std.os.linux.timespec = undefined;
        if (std.os.linux.clock_gettime(@enumFromInt(0), &ts) < 0) return 0;
        return ts.tv_sec * 1000 + @divTrunc(ts.tv_nsec, 1_000_000);
    }
    return 1718700000000;
}

pub fn formatTimestamp(allocator: std.mem.Allocator, ts: i64) ![]const u8 {
    return std.fmt.allocPrint(allocator, "{d}", .{ts});
}

pub fn collectNetworkDiag(
    allocator: std.mem.Allocator,
    cfg: network_diag_config.NetworkDiagConfig,
) !NetworkDiag {
    if (!cfg.enabled) {
        const started_at = try formatTimestamp(allocator, wallClockMs());
        return NetworkDiag{
            .started_at = started_at,
            .status = .disabled,
            .wireguard = null,
            .interfaces = &.{},
            .routes = &.{},
            .underlay_tcp = &.{},
            .events = &.{},
        };
    }

    var interfaces: std.ArrayList(InterfaceOutput) = .{ .items = &.{}, .capacity = 0 };
    var routes: std.ArrayList(RouteOutput) = .{ .items = &.{}, .capacity = 0 };
    var underlay_tcp: std.ArrayList(TcpSocketOutput) = .{ .items = &.{}, .capacity = 0 };
    var overall_status: NetworkDiagStatus = .ok;
    var underlay_available = true;

    if (cfg.wireguard.enabled) {
        for (cfg.wireguard.interfaces) |iface| {
            const stats = extended_interface_stats.readExtendedInterfaceStats(
                allocator, "/sys/class/net", iface,
            ) catch continue;
            defer allocator.free(stats.operstate);

            const name_str = try allocator.dupe(u8, stats.name);
            errdefer allocator.free(name_str);
            const operstate_str = try allocator.dupe(u8, stats.operstate);
            errdefer allocator.free(operstate_str);

            try interfaces.append(allocator, InterfaceOutput{
                .name = name_str,
                .operstate = operstate_str,
                .carrier = stats.carrier,
                .rx_bytes = stats.basic.rx_bytes,
                .tx_bytes = stats.basic.tx_bytes,
                .rx_packets = stats.basic.rx_packets,
                .tx_packets = stats.basic.tx_packets,
                .rx_errors = stats.errors.rx_errors,
                .tx_errors = stats.errors.tx_errors,
                .rx_dropped = stats.errors.rx_dropped,
                .tx_dropped = stats.errors.tx_dropped,
                .rx_errors_delta = if (stats.deltas) |d| d.rx_errors_delta else 0,
                .tx_errors_delta = if (stats.deltas) |d| d.tx_errors_delta else 0,
                .rx_dropped_delta = if (stats.deltas) |d| d.rx_dropped_delta else 0,
                .tx_dropped_delta = if (stats.deltas) |d| d.tx_dropped_delta else 0,
            });
        }
    }

    if (cfg.underlay_tcp.enabled and cfg.underlay_tcp.commands_enabled) {
        const cmd_result = safe_command.runSsTin(allocator, .{}) catch {
            underlay_available = false;
            overall_status = .unavailable;
            const started_at = try formatTimestamp(allocator, wallClockMs());
            return NetworkDiag{
                .started_at = started_at,
                .status = .unavailable,
                .wireguard = null,
                .interfaces = try interfaces.toOwnedSlice(allocator),
                .routes = try routes.toOwnedSlice(allocator),
                .underlay_tcp = &.{},
                .events = &.{},
            };
        };
        defer allocator.free(cmd_result.stdout);
        defer allocator.free(cmd_result.stderr);

        if (cmd_result.exit_code != 0) underlay_available = false;

        if (underlay_available and !cmd_result.stdout_truncated) {
            const ss_cfg = ss_parser.ParseConfig{
                .redact_addresses = cfg.underlay_tcp.redact_addresses,
                .filter_remote_port = if (cfg.underlay_tcp.remote_ports.len > 0)
                    cfg.underlay_tcp.remote_ports[0] else 0,
            };
            const sockets = ss_parser.parseSsTinOutput(allocator, cmd_result.stdout, ss_cfg) catch &.{};
            defer ss_parser.freeTcpSockets(allocator, sockets);

            for (sockets) |socket| {
                const name_str = if (socket.process_name) |pn|
                    try allocator.dupe(u8, pn) else
                    try allocator.dupe(u8, "unknown");
                errdefer allocator.free(name_str);

                const state_str = try allocator.dupe(u8, @tagName(socket.state));
                errdefer allocator.free(state_str);

                const local_str = try allocator.dupe(u8, socket.local orelse "");
                errdefer allocator.free(local_str);

                const remote_str = try allocator.dupe(u8, socket.remote orelse "");
                errdefer allocator.free(remote_str);

                const status_str = try allocator.dupe(u8, @tagName(socket.status));
                errdefer allocator.free(status_str);

                try underlay_tcp.append(allocator, .{
                    .name = name_str, .state = state_str,
                    .local = local_str, .remote = remote_str,
                    .rtt_ms = socket.rtt_ms, .rttvar_ms = socket.rttvar_ms,
                    .rto_ms = socket.rto_ms, .retransmits = socket.retransmits,
                    .unacked = socket.unacked, .cwnd = socket.cwnd,
                    .send_queue_bytes = socket.send_queue_bytes,
                    .recv_queue_bytes = socket.recv_queue_bytes,
                    .status = status_str,
                });
            }
        }
    }

    if (interfaces.items.len == 0 and cfg.wireguard.enabled) overall_status = .warning;
    if (!underlay_available) overall_status = .unavailable;

    const started_at = try formatTimestamp(allocator, wallClockMs());
    return NetworkDiag{
        .started_at = started_at,
        .status = overall_status,
        .wireguard = null,
        .interfaces = try interfaces.toOwnedSlice(allocator),
        .routes = try routes.toOwnedSlice(allocator),
        .underlay_tcp = try underlay_tcp.toOwnedSlice(allocator),
        .events = &.{},
    };
}

// Re-export JSON rendering for backward compatibility.
pub const renderNetworkDiag = @import("net/network_diag_json.zig").renderNetworkDiag;
