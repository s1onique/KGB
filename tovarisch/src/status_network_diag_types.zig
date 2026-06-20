// status_network_diag_types.zig — Public types for network diagnostics
const std = @import("std");

pub const NetworkDiagStatus = enum {
    ok,
    warning,
    @"error",
    unavailable,
    disabled,
};

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

pub const EventOutput = struct {
    ts: []const u8,
    severity: []const u8,
    source: []const u8,
    message: []const u8,
    fields: ?[]const u8,
};

pub const RouteOutput = struct {
    target: []const u8,
    interface: []const u8,
    source: []const u8,
    gateway: ?[]const u8,
    status: []const u8,
};

pub const WireguardPeerOutput = struct {
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

pub const WireguardInterfaceOutput = struct {
    name: []const u8,
    status: []const u8,
    peers: []WireguardPeerOutput,
};

pub const NetworkDiag = struct {
    started_at: []const u8,
    status: NetworkDiagStatus,
    wireguard: ?struct {
        interfaces: []WireguardInterfaceOutput,
        status: NetworkDiagStatus,
    },
    interfaces: []InterfaceOutput,
    routes: []RouteOutput,
    underlay_tcp: []TcpSocketOutput,
    events: []EventOutput,

    pub fn deinit(self: *NetworkDiag, allocator: std.mem.Allocator) void {
        allocator.free(self.started_at);
        for (self.underlay_tcp) |s| {
            allocator.free(s.name);
            allocator.free(s.state);
            allocator.free(s.local);
            allocator.free(s.remote);
            allocator.free(s.status);
        }
        allocator.free(self.underlay_tcp);

        for (self.interfaces) |i| {
            allocator.free(i.name);
            allocator.free(i.operstate);
        }
        allocator.free(self.interfaces);

        for (self.routes) |r| {
            allocator.free(r.target);
            allocator.free(r.interface);
            allocator.free(r.source);
            if (r.gateway) |g| allocator.free(g);
            allocator.free(r.status);
        }
        allocator.free(self.routes);

        for (self.events) |e| {
            allocator.free(e.ts);
            allocator.free(e.severity);
            allocator.free(e.source);
            allocator.free(e.message);
            if (e.fields) |f| allocator.free(f);
        }
        allocator.free(self.events);

        if (self.wireguard) |wg| {
            for (wg.interfaces) |iface| {
                allocator.free(iface.name);
                allocator.free(iface.status);
                for (iface.peers) |p| {
                    allocator.free(p.public_key);
                    allocator.free(p.endpoint);
                    allocator.free(p.allowed_ips);
                    if (p.latest_handshake_at) |ts| allocator.free(ts);
                }
                allocator.free(iface.peers);
            }
            allocator.free(wg.interfaces);
        }

        self.* = undefined;
    }
};
