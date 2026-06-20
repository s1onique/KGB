// status_network_diag_tcp.zig — TCP socket parsing from ss output
const std = @import("std");
const TcpSocketOutput = @import("status_network_diag_types.zig").TcpSocketOutput;
const ss_parser = @import("net/ss_parser.zig");

pub fn parseSockets(
    allocator: std.mem.Allocator,
    out: *std.ArrayList(TcpSocketOutput),
    ss_output: []const u8,
    redact_addresses: bool,
    filter_remote_port: u16,
) !usize {
    const cfg = ss_parser.ParseConfig{
        .redact_addresses = redact_addresses,
        .filter_remote_port = filter_remote_port,
    };

    // Parser failures are truthfulness bugs, not "no matching socket".
    // Propagate the error so caller can emit a proper parse_failed event.
    const sockets = try ss_parser.parseSsTinOutput(allocator, ss_output, cfg);
    defer ss_parser.freeTcpSockets(allocator, sockets);

    for (sockets) |s| {
        const name_str = if (s.process_name) |pn|
            try allocator.dupe(u8, pn)
        else
            try allocator.dupe(u8, "unknown");
        errdefer allocator.free(name_str);

        const state_str = try allocator.dupe(u8, @tagName(s.state));
        errdefer allocator.free(state_str);

        const local_str = try allocator.dupe(u8, s.local orelse "");
        errdefer allocator.free(local_str);

        const remote_str = try allocator.dupe(u8, s.remote orelse "");
        errdefer allocator.free(remote_str);

        const status_str = try allocator.dupe(u8, @tagName(s.status));
        errdefer allocator.free(status_str);

        try out.append(allocator, .{
            .name = name_str,
            .state = state_str,
            .local = local_str,
            .remote = remote_str,
            .rtt_ms = s.rtt_ms,
            .rttvar_ms = s.rttvar_ms,
            .rto_ms = s.rto_ms,
            .retransmits = s.retransmits,
            .unacked = s.unacked,
            .cwnd = s.cwnd,
            .send_queue_bytes = s.send_queue_bytes,
            .recv_queue_bytes = s.recv_queue_bytes,
            .status = status_str,
        });
    }

    return sockets.len;
}
