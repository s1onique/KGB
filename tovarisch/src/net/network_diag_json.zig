// network_diag_json.zig — JSON rendering for network diagnostics
//
// Extracted from status_network_diag.zig to satisfy LLM-friendliness line limits.

const std = @import("std");
const status_network_diag = @import("../status_network_diag.zig");

/// Render network diagnostics to JSON.
pub fn renderNetworkDiag(writer: anytype, diag: *const status_network_diag.NetworkDiag) !void {
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
