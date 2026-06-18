// ss_parser.zig — Parser for `ss -tin` TCP socket information
//
// ACT: Add tovarisch WireGuard and XRay TCP underlay diagnostics
// Parses TCP socket diagnostics from `ss -tin` output.
//
// The `ss -tin` output format includes TCP socket state, RTT, RTTVAR, RTO,
// cwnd, unacked bytes, send/receive queue information.
//
// Privacy constraints:
// - Addresses can be redacted (port retained if configured)
// - Process names may be included if available

const std = @import("std");

// ============================================================================
// Types
// ============================================================================

/// TCP socket state.
pub const TcpState = enum {
    ESTAB,
    SYN_SENT,
    SYN_RECV,
    FIN_WAIT1,
    FIN_WAIT2,
    TIME_WAIT,
    CLOSE,
    CLOSE_WAIT,
    LAST_ACK,
    LISTEN,
    CLOSING,
    UNKNOWN,
};

/// A single TCP socket's diagnostic data.
pub const TcpSocket = struct {
    /// Socket state.
    state: TcpState,
    /// Local address (may be redacted).
    local: ?[]const u8 = null,
    /// Remote address (may be redacted).
    remote: ?[]const u8 = null,
    /// Round-trip time in milliseconds.
    rtt_ms: ?f64 = null,
    /// RTT variance in milliseconds.
    rttvar_ms: ?f64 = null,
    /// Retransmission timeout in milliseconds.
    rto_ms: ?u64 = null,
    /// Congestion window.
    cwnd: ?u32 = null,
    /// Unacked bytes.
    unacked: ?u64 = null,
    /// Send queue bytes.
    send_queue_bytes: ?u64 = null,
    /// Receive queue bytes.
    recv_queue_bytes: ?u64 = null,
    /// Retransmit count/indicator.
    retransmits: ?u64 = null,
    /// Process name if available.
    process_name: ?[]const u8 = null,
    /// Socket status.
    status: SocketStatus = .ok,
};

/// Socket status.
pub const SocketStatus = enum {
    ok,
    warning,
    @"error",
    unavailable,
};

/// Parser errors.
pub const ParseError = error{
    NoData,
    MalformedOutput,
    InvalidNumber,
};

/// Configuration for parsing.
pub const ParseConfig = struct {
    /// Whether to redact addresses.
    redact_addresses: bool = true,
    /// Filter by remote port (0 = no filter).
    filter_remote_port: u16 = 0,
    /// Filter by process name prefix.
    filter_process: ?[]const u8 = null,
};

// ============================================================================
// Parser
// ============================================================================

/// Parse `ss -tin` output and extract TCP socket diagnostics.
///
/// Input format example:
///   State       Recv-Q   Send-Q   Local Address:Port   Peer Address:Port   Process
///   ESTAB       0        0        10.0.0.1:443         192.0.2.1:12345    users:(("xray",pid=1234,fd=5))
///   rtt:49.2/8.1    rttvar:4.2/2.1    pacing_rate 1000000.0    cwnd:10    retrans:0/123    unacked:3
pub fn parseSsTinOutput(allocator: std.mem.Allocator, input: []const u8, config: ParseConfig) ParseError![]TcpSocket {
    var sockets = std.ArrayList(TcpSocket).empty;
    var lines = std.mem.splitScalar(u8, input, '\n');

    // Skip header line
    _ = lines.next();

    while (lines.next()) |line| {
        const trimmed = std.mem.trim(u8, line, " \t\r\n");
        if (trimmed.len == 0) continue;

        const socket = parseSocketLine(allocator, trimmed, config) catch continue;

        // Apply filters
        if (config.filter_remote_port != 0) {
            if (socket.remote) |remote| {
                const port = extractPort(remote);
                if (port != config.filter_remote_port) continue;
            } else continue;
        }

        if (config.filter_process) |filter| {
            if (socket.process_name) |name| {
                if (!std.mem.startsWith(u8, name, filter)) continue;
            } else continue;
        }

        sockets.append(allocator, socket) catch {};
    }

    return sockets.toOwnedSlice(allocator) catch return error.MalformedOutput;
}

/// Parse a single socket line.
fn parseSocketLine(allocator: std.mem.Allocator, line: []const u8, config: ParseConfig) ParseError!TcpSocket {
    // Parse state (first field)
    const state_end = std.mem.indexOfScalar(u8, line, ' ') orelse return error.MalformedOutput;
    const state_str = std.mem.trim(u8, line[0..state_end], " \t");
    const state = parseTcpState(state_str);

    const rest = std.mem.trim(u8, line[state_end..], " \t");

    // Parse Recv-Q and Send-Q
    var recv_q: u64 = 0;
    var send_q: u64 = 0;

    var fields = std.mem.splitScalar(u8, rest, ' ');
    var field_count: usize = 0;
    while (fields.next()) |field| : (field_count += 1) {
        const trimmed_field = std.mem.trim(u8, field, " \t");
        if (trimmed_field.len == 0) continue;

        if (field_count == 0) {
            recv_q = std.fmt.parseInt(u64, trimmed_field, 10) catch 0;
        } else if (field_count == 1) {
            send_q = std.fmt.parseInt(u64, trimmed_field, 10) catch 0;
        } else break;
    }

    // Parse Local Address:Port and Peer Address:Port
    var local: ?[]const u8 = null;
    var remote: ?[]const u8 = null;
    var process_name: ?[]const u8 = null;

    // Find local and remote addresses
    // Format: Local Address:Port  Peer Address:Port
    var addr_fields = std.mem.splitScalar(u8, rest, ' ');
    var found_local = false;
    var found_remote = false;

    while (addr_fields.next()) |field| {
        const trimmed_field = std.mem.trim(u8, field, " \t");
        if (trimmed_field.len == 0) continue;

        // Check if this looks like an address:port
        if (std.mem.containsAtLeast(u8, trimmed_field, 1, ":") and !found_local) {
            local = if (config.redact_addresses) (redactAddress(allocator, trimmed_field) catch null) else trimmed_field;
            found_local = true;
        } else if (found_local and !found_remote) {
            if (std.mem.containsAtLeast(u8, trimmed_field, 1, ":") or
                std.mem.containsAtLeast(u8, trimmed_field, 1, "("))
            {
                remote = if (config.redact_addresses) (redactAddress(allocator, trimmed_field) catch null) else trimmed_field;
                found_remote = true;
            }
        }

        // Check for process info
        if (std.mem.containsAtLeast(u8, trimmed_field, 1, "(")) {
            process_name = extractProcessName(trimmed_field);
        }
    }

    // Parse TCP metrics from the rest of the line
    var rtt_ms: ?f64 = null;
    var rttvar_ms: ?f64 = null;
    var rto_ms: ?u64 = null;
    var cwnd: ?u32 = null;
    var unacked: ?u64 = null;
    var retransmits: ?u64 = null;

    // Look for metrics patterns
    // rtt:49.2/8.1  rttvar:4.2/2.1  cwnd:10  retrans:0/123  unacked:3
    if (std.mem.indexOf(u8, line, "rtt:")) |idx| {
        const rtt_section = line[idx..];
        rtt_ms = parseRttMetric(rtt_section);
    }

    if (std.mem.indexOf(u8, line, "rttvar:")) |idx| {
        const rttvar_section = line[idx..];
        rttvar_ms = parseRttVarMetric(rttvar_section);
    }

    if (std.mem.indexOf(u8, line, "rto:")) |idx| {
        const rto_section = line[idx..];
        rto_ms = parseRtoMetric(rto_section);
    }

    if (std.mem.indexOf(u8, line, "cwnd:")) |idx| {
        const cwnd_section = line[idx..];
        cwnd = parseCwndMetric(cwnd_section);
    }

    if (std.mem.indexOf(u8, line, "retrans:")) |idx| {
        const retrans_section = line[idx..];
        retransmits = parseRetransMetric(retrans_section);
    }

    if (std.mem.indexOf(u8, line, "unacked:")) |idx| {
        const unacked_section = line[idx..];
        unacked = parseUnackedMetric(unacked_section);
    }

    // Determine status
    var status: SocketStatus = .ok;
    if (retransmits != null and retransmits.? > 0) {
        status = .warning;
    }
    if (unacked != null and unacked.? > 100) {
        status = .warning;
    }
    if (rto_ms != null and rto_ms.? > 1000) {
        status = .warning;
    }

    return TcpSocket{
        .state = state,
        .local = local,
        .remote = remote,
        .rtt_ms = rtt_ms,
        .rttvar_ms = rttvar_ms,
        .rto_ms = rto_ms,
        .cwnd = cwnd,
        .unacked = unacked,
        .send_queue_bytes = send_q,
        .recv_queue_bytes = recv_q,
        .retransmits = retransmits,
        .process_name = process_name,
        .status = status,
    };
}

/// Parse TCP state string to enum.
fn parseTcpState(state_str: []const u8) TcpState {
    const upper = std.mem.trim(u8, state_str, " \t\r\n");
    inline for (@typeInfo(TcpState).@"enum".fields) |field| {
        if (std.mem.eql(u8, upper, field.name)) {
            return @enumFromInt(field.value);
        }
    }
    return .UNKNOWN;
}

/// Extract port from an address:port string.
fn extractPort(addr: []const u8) u16 {
    const colon_idx = std.mem.indexOfScalar(u8, addr, ':');
    if (colon_idx == null) return 0;
    const port_str = std.mem.trim(u8, addr[colon_idx.? + 1 ..], " \t");
    return std.fmt.parseInt(u16, port_str, 10) catch 0;
}

/// Redact an address, keeping only the port.
/// Returns an allocator-owned string that caller may retain.
fn redactAddress(allocator: std.mem.Allocator, addr: []const u8) ![]const u8 {
    const colon_idx = std.mem.indexOfScalar(u8, addr, ':');
    if (colon_idx == null) {
        return try allocator.dupe(u8, "redacted");
    }
    const port = addr[colon_idx.?..];
    return std.fmt.allocPrint(allocator, "redacted{s}", .{port});
}

/// Extract process name from ss output.
fn extractProcessName(field: []const u8) ?[]const u8 {
    // Format: (("processname",pid=1234,fd=5))
    const open_idx = std.mem.indexOf(u8, field, "((");
    const close_idx = std.mem.indexOf(u8, field, "))");

    if (open_idx == null or close_idx == null) return null;
    return field[open_idx.? + 2 .. close_idx.?];
}

/// Parse rtt metric: rtt:49.2/8.1
fn parseRttMetric(section: []const u8) ?f64 {
    const colon_idx = std.mem.indexOfScalar(u8, section, ':');
    if (colon_idx == null) return null;

    const value_part = section[colon_idx.? + 1..];
    const slash_idx = std.mem.indexOfScalar(u8, value_part, '/');

    if (slash_idx == null) return null;
    const rtt_str = std.mem.trim(u8, value_part[0..slash_idx.?], " \t");

    return std.fmt.parseFloat(f64, rtt_str) catch null;
}

/// Parse rttvar metric: rttvar:4.2/2.1
fn parseRttVarMetric(section: []const u8) ?f64 {
    const colon_idx = std.mem.indexOfScalar(u8, section, ':');
    if (colon_idx == null) return null;

    const value_part = section[colon_idx.? + 1..];
    const slash_idx = std.mem.indexOfScalar(u8, value_part, '/');

    if (slash_idx == null) return null;
    const rttvar_str = std.mem.trim(u8, value_part[0..slash_idx.?], " \t");

    return std.fmt.parseFloat(f64, rttvar_str) catch null;
}

/// Parse rto metric: rto:220
fn parseRtoMetric(section: []const u8) ?u64 {
    const colon_idx = std.mem.indexOfScalar(u8, section, ':');
    if (colon_idx == null) return null;

    const value_part = std.mem.trim(u8, section[colon_idx.? + 1..], " \t");
    var end_idx: usize = 0;
    for (value_part, 0..) |c, i| {
        if (c < '0' or c > '9') {
            end_idx = i;
            break;
        }
        end_idx = value_part.len;
    }

    return std.fmt.parseInt(u64, value_part[0..end_idx], 10) catch null;
}

/// Parse cwnd metric: cwnd:10
fn parseCwndMetric(section: []const u8) ?u32 {
    const colon_idx = std.mem.indexOfScalar(u8, section, ':');
    if (colon_idx == null) return null;

    const value_part = std.mem.trim(u8, section[colon_idx.? + 1..], " \t");
    return std.fmt.parseInt(u32, value_part, 10) catch null;
}

/// Parse retrans metric: retrans:0/123
fn parseRetransMetric(section: []const u8) ?u64 {
    const colon_idx = std.mem.indexOfScalar(u8, section, ':');
    if (colon_idx == null) return null;

    const value_part = std.mem.trim(u8, section[colon_idx.? + 1..], " \t");
    const slash_idx = std.mem.indexOfScalar(u8, value_part, '/');

    if (slash_idx != null) {
        // Format: retrans:0/123 (current/total or sent/retrans)
        const retrans_str = std.mem.trim(u8, value_part[slash_idx.? + 1..], " \t");
        return std.fmt.parseInt(u64, retrans_str, 10) catch null;
    }

    return std.fmt.parseInt(u64, value_part, 10) catch null;
}

/// Parse unacked metric: unacked:3
fn parseUnackedMetric(section: []const u8) ?u64 {
    const colon_idx = std.mem.indexOfScalar(u8, section, ':');
    if (colon_idx == null) return null;

    const value_part = std.mem.trim(u8, section[colon_idx.? + 1..], " \t");
    return std.fmt.parseInt(u64, value_part, 10) catch null;
}

/// Free TCP sockets and all nested allocations.
pub fn freeTcpSockets(allocator: std.mem.Allocator, sockets: []TcpSocket) void {
    for (sockets) |socket| {
        if (socket.local) |l| allocator.free(l);
        if (socket.remote) |r| allocator.free(r);
        if (socket.process_name) |p| allocator.free(p);
    }
    allocator.free(sockets);
}

// ============================================================================
// Tests
// ============================================================================

test "parseTcpState returns correct states" {
    try std.testing.expectEqual(TcpState.ESTAB, parseTcpState("ESTAB"));
    try std.testing.expectEqual(TcpState.LISTEN, parseTcpState("LISTEN"));
    try std.testing.expectEqual(TcpState.UNKNOWN, parseTcpState("INVALID"));
}

test "extractPort parses port correctly" {
    try std.testing.expectEqual(@as(u16, 443), extractPort("192.0.2.1:443"));
    try std.testing.expectEqual(@as(u16, 12345), extractPort("10.0.0.1:12345"));
    try std.testing.expectEqual(@as(u16, 0), extractPort("invalid"));
}

test "redactAddress keeps port" {
    const allocator = std.testing.allocator;
    const redacted = try redactAddress(allocator, "192.0.2.1:443");
    defer allocator.free(redacted);
    try std.testing.expectEqualStrings("redacted:443", redacted);
}

test "extractProcessName parses correctly" {
    // The actual format in ss output is: users:(("processname",pid=1234,fd=5))
    const result = extractProcessName("((\"xray\",pid=1234,fd=5))");
    try std.testing.expect(result != null);
    try std.testing.expectEqualStrings("\"xray\",pid=1234,fd=5", result.?);
}

test "parseRttMetric extracts rtt" {
    const result = parseRttMetric("rtt:49.2/8.1  other");
    try std.testing.expect(result != null);
    try std.testing.expect(result.? > 49.0 and result.? < 50.0);
}

test "parseCwndMetric extracts cwnd" {
    const result = parseCwndMetric("cwnd:10");
    try std.testing.expect(result != null);
    try std.testing.expectEqual(@as(u32, 10), result.?);
}

test "parseRetransMetric extracts retransmit count" {
    const result = parseRetransMetric("retrans:0/123");
    try std.testing.expect(result != null);
    try std.testing.expectEqual(@as(u64, 123), result.?);
}

test "parseUnackedMetric extracts unacked" {
    const result = parseUnackedMetric("unacked:3");
    try std.testing.expect(result != null);
    try std.testing.expectEqual(@as(u64, 3), result.?);
}
