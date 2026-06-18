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
pub const NetworkDiag = struct {
    started_at: []const u8,
    status: NetworkDiagStatus,
    wireguard: ?WireguardDiagSection,
    interfaces: []InterfaceOutput,
    routes: []RouteOutput,
    underlay_tcp: []TcpSocketOutput,
    events: []EventOutput,
};

/// WireGuard diagnostics section.
pub const WireguardDiagSection = struct {
    interfaces: []WgInterfaceOutput,
    status: NetworkDiagStatus,
};

// ============================================================================
// Collection
// ============================================================================

/// Get wall clock time in milliseconds.
fn wallClockMs() i64 {
    if (comptime @import("builtin").os.tag == .linux and @hasDecl(std.os.linux, "clock_gettime")) {
        var ts: std.os.linux.timespec = undefined;
        if (std.os.linux.clock_gettime(@enumFromInt(0), &ts) < 0) return 0;
        return ts.tv_sec * 1000 + @divTrunc(ts.tv_nsec, 1_000_000);
    }
    return 1718700000000; // Test timestamp
}

/// Collect network diagnostics based on configuration.
pub fn collectNetworkDiag(
    allocator: std.mem.Allocator,
    cfg: network_diag_config.NetworkDiagConfig,
) !NetworkDiag {
    // If diagnostics disabled, return minimal structure
    if (!cfg.enabled) {
        return NetworkDiag{
            .started_at = formatTimestamp(wallClockMs()),
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

    // Collect interface stats
    if (cfg.wireguard.enabled) {
        for (cfg.wireguard.interfaces) |iface| {
            const stats = extended_interface_stats.readExtendedInterfaceStats(
                allocator,
                "/sys/class/net",
                iface,
            ) catch continue;

            try interfaces.append(allocator, InterfaceOutput{
                .name = stats.name,
                .operstate = stats.operstate,
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

    // Collect TCP underlay if enabled
    if (cfg.underlay_tcp.enabled and cfg.underlay_tcp.commands_enabled) {
        const cmd_result = safe_command.runSsTin(allocator, .{}) catch {
            // Command unavailable - continue with empty underlay_tcp
            underlay_available = false;
            overall_status = .unavailable;
            return NetworkDiag{
                .started_at = formatTimestamp(wallClockMs()),
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

        if (cmd_result.exit_code != 0) {
            underlay_available = false;
        }

        if (underlay_available and !cmd_result.stdout_truncated) {
            const ss_cfg = ss_parser.ParseConfig{
                .redact_addresses = cfg.underlay_tcp.redact_addresses,
                .filter_remote_port = if (cfg.underlay_tcp.remote_ports.len > 0)
                    cfg.underlay_tcp.remote_ports[0] else 0,
            };
            const sockets = ss_parser.parseSsTinOutput(cmd_result.stdout, ss_cfg) catch &.{};
            for (sockets) |socket| {
                try underlay_tcp.append(allocator, .{
                    .name = socket.process_name orelse "unknown",
                    .state = @tagName(socket.state),
                    .local = socket.local orelse "",
                    .remote = socket.remote orelse "",
                    .rtt_ms = socket.rtt_ms,
                    .rttvar_ms = socket.rttvar_ms,
                    .rto_ms = socket.rto_ms,
                    .retransmits = socket.retransmits,
                    .unacked = socket.unacked,
                    .cwnd = socket.cwnd,
                    .send_queue_bytes = socket.send_queue_bytes,
                    .recv_queue_bytes = socket.recv_queue_bytes,
                    .status = @tagName(socket.status),
                });
            }
        }
    }

    // Determine overall status
    if (interfaces.items.len == 0 and cfg.wireguard.enabled) {
        overall_status = .warning;
    }
    if (!underlay_available) {
        overall_status = .unavailable;
    }

    return NetworkDiag{
        .started_at = formatTimestamp(wallClockMs()),
        .status = overall_status,
        .wireguard = null, // WireGuard detailed output requires more complex state
        .interfaces = try interfaces.toOwnedSlice(allocator),
        .routes = try routes.toOwnedSlice(allocator),
        .underlay_tcp = try underlay_tcp.toOwnedSlice(allocator),
        .events = &.{},
    };
}

/// Format timestamp as ISO 8601 (simplified).
fn formatTimestamp(ts: i64) []const u8 {
    // Simplified: return Unix timestamp in milliseconds
    var buf: [32]u8 = undefined;
    const len = std.fmt.bufPrint(&buf, "{d}", .{ts}) catch "0";
    return len;
}

// ============================================================================
// JSON Rendering
// ============================================================================

/// Render network diagnostics to JSON.
pub fn renderNetworkDiag(writer: anytype, diag: *const NetworkDiag) !void {
    var jw = std.json.Stringify{ .writer = writer };

    try jw.beginObject();
    try jw.objectField("started_at");
    try jw.write(diag.started_at);
    try jw.objectField("status");
    try jw.write(@tagName(diag.status));

    // WireGuard section
    try jw.objectField("wireguard");
    if (diag.wireguard) |wg| {
        try jw.beginObject();
        try jw.objectField("status");
        try jw.write(@tagName(wg.status));
        try jw.objectField("interfaces");
        try jw.beginArray();
        for (wg.interfaces) |iface| {
            try jw.beginObject();
            try jw.objectField("name");
            try jw.write(iface.name);
            try jw.objectField("status");
            try jw.write(iface.status);
            try jw.objectField("peers");
            try jw.beginArray();
            for (iface.peers) |peer| {
                try jw.beginObject();
                try jw.objectField("public_key");
                try jw.write(peer.public_key);
                try jw.objectField("endpoint");
                try jw.write(peer.endpoint);
                try jw.objectField("allowed_ips");
                try jw.write(peer.allowed_ips);
                if (peer.latest_handshake_at) |ts| {
                    try jw.objectField("latest_handshake_at");
                    try jw.write(ts);
                }
                if (peer.latest_handshake_age_seconds) |age| {
                    try jw.objectField("latest_handshake_age_seconds");
                    try jw.write(age);
                }
                try jw.objectField("transfer_rx_bytes");
                try jw.write(peer.transfer_rx_bytes);
                try jw.objectField("transfer_tx_bytes");
                try jw.write(peer.transfer_tx_bytes);
                try jw.objectField("persistent_keepalive_seconds");
                try jw.write(peer.persistent_keepalive_seconds);
                try jw.endObject();
            }
            try jw.endArray();
            try jw.endObject();
        }
        try jw.endArray();
        try jw.endObject();
    } else {
        try jw.write(null);
    }

    // Interfaces section
    try jw.objectField("interfaces");
    try jw.beginArray();
    for (diag.interfaces) |iface| {
        try jw.beginObject();
        try jw.objectField("name"); try jw.write(iface.name);
        try jw.objectField("operstate"); try jw.write(iface.operstate);
        if (iface.carrier) |c| { try jw.objectField("carrier"); try jw.write(c); }
        inline for (.{ "rx_bytes", "tx_bytes", "rx_packets", "tx_packets", "rx_errors", "tx_errors", "rx_dropped", "tx_dropped" }) |field| {
            try jw.objectField(field); try jw.write(@field(iface, field));
        }
        inline for (.{ "rx_errors_delta", "tx_errors_delta", "rx_dropped_delta", "tx_dropped_delta" }) |field| {
            try jw.objectField(field); try jw.write(@field(iface, field));
        }
        try jw.endObject();
    }
    try jw.endArray();

    // Routes section
    try jw.objectField("routes");
    try jw.beginArray();
    for (diag.routes) |route| {
        try jw.beginObject();
        try jw.objectField("target");
        try jw.write(route.target);
        try jw.objectField("interface");
        try jw.write(route.interface);
        try jw.objectField("source");
        try jw.write(route.source);
        if (route.gateway) |gw| {
            try jw.objectField("gateway");
            try jw.write(gw);
        }
        try jw.objectField("status");
        try jw.write(route.status);
        try jw.endObject();
    }
    try jw.endArray();

    // Underlay TCP section
    try jw.objectField("underlay_tcp");
    try jw.beginArray();
    for (diag.underlay_tcp) |socket| {
        try jw.beginObject();
        try jw.objectField("name");
        try jw.write(socket.name);
        try jw.objectField("state");
        try jw.write(socket.state);
        try jw.objectField("local");
        try jw.write(socket.local);
        try jw.objectField("remote");
        try jw.write(socket.remote);
        if (socket.rtt_ms) |rtt| {
            try jw.objectField("rtt_ms");
            try jw.write(rtt);
        }
        if (socket.rttvar_ms) |rttvar| {
            try jw.objectField("rttvar_ms");
            try jw.write(rttvar);
        }
        if (socket.rto_ms) |rto| {
            try jw.objectField("rto_ms");
            try jw.write(rto);
        }
        if (socket.retransmits) |retr| {
            try jw.objectField("retransmits");
            try jw.write(retr);
        }
        if (socket.unacked) |unack| {
            try jw.objectField("unacked");
            try jw.write(unack);
        }
        if (socket.cwnd) |cwnd| {
            try jw.objectField("cwnd");
            try jw.write(cwnd);
        }
        if (socket.send_queue_bytes) |sq| {
            try jw.objectField("send_queue_bytes");
            try jw.write(sq);
        }
        if (socket.recv_queue_bytes) |rq| {
            try jw.objectField("recv_queue_bytes");
            try jw.write(rq);
        }
        try jw.objectField("status");
        try jw.write(socket.status);
        try jw.endObject();
    }
    try jw.endArray();

    // Events section
    try jw.objectField("events");
    try jw.beginArray();
    for (diag.events) |event| {
        try jw.beginObject();
        try jw.objectField("ts");
        try jw.write(event.ts);
        try jw.objectField("severity");
        try jw.write(event.severity);
        try jw.objectField("source");
        try jw.write(event.source);
        try jw.objectField("message");
        try jw.write(event.message);
        if (event.fields) |f| {
            try jw.objectField("fields");
            try jw.write(f);
        }
        try jw.endObject();
    }
    try jw.endArray();

    try jw.endObject();
}
