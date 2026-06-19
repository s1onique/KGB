// network_diag_json.zig — JSON rendering for network diagnostics
//
// Extracted from status_network_diag.zig to satisfy LLM-friendliness line limits.
// Uses manual JSON formatting to support generic writers with print/writeAll methods.

const std = @import("std");
const status_network_diag = @import("../status_network_diag.zig");

/// Render a string value to JSON (with escaping for JSON special chars).
/// Escapes all control bytes (< 0x20, except \n\r\t which are handled explicitly) as \u00XX.
fn writeJsonString(writer: anytype, s: []const u8) !void {
    try writer.writeAll("\"");
    for (s) |c| {
        switch (c) {
            '"' => try writer.writeAll("\\\""),
            '\\' => try writer.writeAll("\\\\"),
            '\n' => try writer.writeAll("\\n"),
            '\r' => try writer.writeAll("\\r"),
            '\t' => try writer.writeAll("\\t"),
            0x00...0x08, 0x0b, 0x0c, 0x0e...0x1f => try writer.print("\\u00{x:0>2}", .{@as(u8, c)}),
            else => try writer.writeByte(c),
        }
    }
    try writer.writeAll("\"");
}

/// Render network diagnostics to JSON.
/// Works with any writer that has print and writeAll methods.
pub fn renderNetworkDiag(writer: anytype, diag: *const status_network_diag.NetworkDiag) !void {
    try writer.writeAll("{\"started_at\":");
    try writeJsonString(writer, diag.started_at);
    try writer.writeAll(",\"status\":");
    try writeJsonString(writer, @tagName(diag.status));

    // WireGuard section
    try writer.writeAll(",\"wireguard\":");
    if (diag.wireguard) |wg| {
        try writer.writeAll("{\"status\":");
        try writeJsonString(writer, @tagName(wg.status));
        try writer.writeAll(",\"interfaces\":[");
        for (wg.interfaces, 0..) |iface, iface_i| {
            if (iface_i > 0) try writer.writeAll(",");
            try writer.writeAll("{\"name\":");
            try writeJsonString(writer, iface.name);
            try writer.writeAll(",\"status\":");
            try writeJsonString(writer, iface.status);
            try writer.writeAll(",\"peers\":[");
            for (iface.peers, 0..) |peer, peer_i| {
                if (peer_i > 0) try writer.writeAll(",");
                try writer.writeAll("{\"public_key\":");
                try writeJsonString(writer, peer.public_key);
                try writer.writeAll(",\"endpoint\":");
                try writeJsonString(writer, peer.endpoint);
                try writer.writeAll(",\"allowed_ips\":");
                try writeJsonString(writer, peer.allowed_ips);
                if (peer.latest_handshake_at) |ts| {
                    try writer.writeAll(",\"latest_handshake_at\":");
                    try writeJsonString(writer, ts);
                }
                if (peer.latest_handshake_age_seconds) |age| {
                    try writer.writeAll(",\"latest_handshake_age_seconds\":");
                    try writer.print("{d}", .{age});
                }
                try writer.writeAll(",\"transfer_rx_bytes\":");
                try writer.print("{d}", .{peer.transfer_rx_bytes});
                try writer.writeAll(",\"transfer_tx_bytes\":");
                try writer.print("{d}", .{peer.transfer_tx_bytes});
                try writer.writeAll(",\"persistent_keepalive_seconds\":");
                try writer.print("{d}", .{peer.persistent_keepalive_seconds});
                try writer.writeAll("}");
            }
            try writer.writeAll("]}");
        }
        try writer.writeAll("]");
        try writer.writeAll("}");
    } else {
        try writer.writeAll("null");
    }

    // Interfaces section
    try writer.writeAll(",\"interfaces\":[");
    for (diag.interfaces, 0..) |iface, i| {
        if (i > 0) try writer.writeAll(",");
        try writer.writeAll("{\"name\":");
        try writeJsonString(writer, iface.name);
        try writer.writeAll(",\"operstate\":");
        try writeJsonString(writer, iface.operstate);
        if (iface.carrier) |c| {
            try writer.writeAll(",\"carrier\":");
            try writer.writeAll(if (c) "true" else "false");
        }
        try writer.writeAll(",\"rx_bytes\":");
        try writer.print("{d}", .{iface.rx_bytes});
        try writer.writeAll(",\"tx_bytes\":");
        try writer.print("{d}", .{iface.tx_bytes});
        try writer.writeAll(",\"rx_packets\":");
        try writer.print("{d}", .{iface.rx_packets});
        try writer.writeAll(",\"tx_packets\":");
        try writer.print("{d}", .{iface.tx_packets});
        try writer.writeAll(",\"rx_errors\":");
        try writer.print("{d}", .{iface.rx_errors});
        try writer.writeAll(",\"tx_errors\":");
        try writer.print("{d}", .{iface.tx_errors});
        try writer.writeAll(",\"rx_dropped\":");
        try writer.print("{d}", .{iface.rx_dropped});
        try writer.writeAll(",\"tx_dropped\":");
        try writer.print("{d}", .{iface.tx_dropped});
        try writer.writeAll(",\"rx_errors_delta\":");
        try writer.print("{d}", .{iface.rx_errors_delta});
        try writer.writeAll(",\"tx_errors_delta\":");
        try writer.print("{d}", .{iface.tx_errors_delta});
        try writer.writeAll(",\"rx_dropped_delta\":");
        try writer.print("{d}", .{iface.rx_dropped_delta});
        try writer.writeAll(",\"tx_dropped_delta\":");
        try writer.print("{d}", .{iface.tx_dropped_delta});
        try writer.writeAll("}");
    }
    try writer.writeAll("]");

    // Routes section
    try writer.writeAll(",\"routes\":[");
    for (diag.routes, 0..) |route, i| {
        if (i > 0) try writer.writeAll(",");
        try writer.writeAll("{\"target\":");
        try writeJsonString(writer, route.target);
        try writer.writeAll(",\"interface\":");
        try writeJsonString(writer, route.interface);
        try writer.writeAll(",\"source\":");
        try writeJsonString(writer, route.source);
        if (route.gateway) |gw| {
            try writer.writeAll(",\"gateway\":");
            try writeJsonString(writer, gw);
        }
        try writer.writeAll(",\"status\":");
        try writeJsonString(writer, route.status);
        try writer.writeAll("}");
    }
    try writer.writeAll("]");

    // Underlay TCP section
    try writer.writeAll(",\"underlay_tcp\":[");
    for (diag.underlay_tcp, 0..) |socket, i| {
        if (i > 0) try writer.writeAll(",");
        try writer.writeAll("{\"name\":");
        try writeJsonString(writer, socket.name);
        try writer.writeAll(",\"state\":");
        try writeJsonString(writer, socket.state);
        try writer.writeAll(",\"local\":");
        try writeJsonString(writer, socket.local);
        try writer.writeAll(",\"remote\":");
        try writeJsonString(writer, socket.remote);
        if (socket.rtt_ms) |rtt| {
            try writer.writeAll(",\"rtt_ms\":");
            try writer.print("{d}", .{@as(f64, rtt)});
        }
        if (socket.rttvar_ms) |rttvar| {
            try writer.writeAll(",\"rttvar_ms\":");
            try writer.print("{d}", .{@as(f64, rttvar)});
        }
        if (socket.rto_ms) |rto| {
            try writer.writeAll(",\"rto_ms\":");
            try writer.print("{d}", .{rto});
        }
        if (socket.retransmits) |retr| {
            try writer.writeAll(",\"retransmits\":");
            try writer.print("{d}", .{retr});
        }
        if (socket.unacked) |unack| {
            try writer.writeAll(",\"unacked\":");
            try writer.print("{d}", .{unack});
        }
        if (socket.cwnd) |cwnd| {
            try writer.writeAll(",\"cwnd\":");
            try writer.print("{d}", .{cwnd});
        }
        if (socket.send_queue_bytes) |sq| {
            try writer.writeAll(",\"send_queue_bytes\":");
            try writer.print("{d}", .{sq});
        }
        if (socket.recv_queue_bytes) |rq| {
            try writer.writeAll(",\"recv_queue_bytes\":");
            try writer.print("{d}", .{rq});
        }
        try writer.writeAll(",\"status\":");
        try writeJsonString(writer, socket.status);
        try writer.writeAll("}");
    }
    try writer.writeAll("]");

    // Events section
    try writer.writeAll(",\"events\":[");
    for (diag.events, 0..) |event, i| {
        if (i > 0) try writer.writeAll(",");
        try writer.writeAll("{\"ts\":");
        try writeJsonString(writer, event.ts);
        try writer.writeAll(",\"severity\":");
        try writeJsonString(writer, event.severity);
        try writer.writeAll(",\"source\":");
        try writeJsonString(writer, event.source);
        try writer.writeAll(",\"message\":");
        try writeJsonString(writer, event.message);
        if (event.fields) |f| {
            try writer.writeAll(",\"fields\":");
            try writeJsonString(writer, f);
        }
        try writer.writeAll("}");
    }
    try writer.writeAll("]");

    try writer.writeAll("}");
}
