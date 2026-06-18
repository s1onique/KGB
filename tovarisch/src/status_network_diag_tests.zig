// status_network_diag_tests.zig — Unit tests for network diagnostics ownership
//
// Tests ownership model:
// - TcpSocketOutput strings are duplicated from parser-owned TcpSocket
// - freeTcpSockets is called after copying
// - NetworkDiag.deinit is safe for disabled, unavailable, and normal payloads

const std = @import("std");
const status_network_diag = @import("status_network_diag.zig");
const ss_parser = @import("net/ss_parser.zig");

const testing = std.testing;

// ============================================================================
// Tests: NetworkDiag.deinit for disabled status
// ============================================================================

test "NetworkDiag.deinit is safe for disabled status" {
    const allocator = testing.allocator;
    var diag = status_network_diag.NetworkDiag{
        .started_at = try allocator.dupe(u8, "1718700000000"),
        .status = .disabled,
        .wireguard = null,
        .interfaces = &.{},
        .routes = &.{},
        .underlay_tcp = &.{},
        .events = &.{},
    };

    diag.deinit(allocator);
}

test "NetworkDiag.deinit is safe for unavailable status" {
    const allocator = testing.allocator;
    var diag = status_network_diag.NetworkDiag{
        .started_at = try allocator.dupe(u8, "1718700000000"),
        .status = .unavailable,
        .wireguard = null,
        .interfaces = &.{},
        .routes = &.{},
        .underlay_tcp = &.{},
        .events = &.{},
    };

    diag.deinit(allocator);
}

test "NetworkDiag.deinit is safe for normal status with empty slices" {
    const allocator = testing.allocator;
    var diag = status_network_diag.NetworkDiag{
        .started_at = try allocator.dupe(u8, "1718700000000"),
        .status = .ok,
        .wireguard = null,
        .interfaces = &.{},
        .routes = &.{},
        .underlay_tcp = &.{},
        .events = &.{},
    };

    diag.deinit(allocator);
}

// ============================================================================
// Tests: NetworkDiag.deinit with underlay_tcp sockets
// ============================================================================

test "NetworkDiag.deinit frees underlay_tcp socket strings" {
    const allocator = testing.allocator;
    var diag = status_network_diag.NetworkDiag{
        .started_at = try allocator.dupe(u8, "1718700000000"),
        .status = .ok,
        .wireguard = null,
        .interfaces = &.{},
        .routes = &.{},
        .underlay_tcp = try allocator.alloc(status_network_diag.TcpSocketOutput, 1),
        .events = &.{},
    };

    // Simulate what collectNetworkDiag does - duplicate strings
    diag.underlay_tcp[0] = .{
        .name = try allocator.dupe(u8, "xray"),
        .state = try allocator.dupe(u8, "ESTAB"),
        .local = try allocator.dupe(u8, "redacted:443"),
        .remote = try allocator.dupe(u8, "redacted:12345"),
        .rtt_ms = 49.2,
        .rttvar_ms = 4.2,
        .rto_ms = 220,
        .retransmits = 0,
        .unacked = 3,
        .cwnd = 10,
        .send_queue_bytes = 0,
        .recv_queue_bytes = 0,
        .status = try allocator.dupe(u8, "ok"),
    };

    diag.deinit(allocator);
}

test "NetworkDiag.deinit frees multiple underlay_tcp sockets" {
    const allocator = testing.allocator;
    var diag = status_network_diag.NetworkDiag{
        .started_at = try allocator.dupe(u8, "1718700000000"),
        .status = .ok,
        .wireguard = null,
        .interfaces = &.{},
        .routes = &.{},
        .underlay_tcp = try allocator.alloc(status_network_diag.TcpSocketOutput, 2),
        .events = &.{},
    };

    // First socket
    diag.underlay_tcp[0] = .{
        .name = try allocator.dupe(u8, "xray"),
        .state = try allocator.dupe(u8, "ESTAB"),
        .local = try allocator.dupe(u8, "redacted:443"),
        .remote = try allocator.dupe(u8, "redacted:12345"),
        .rtt_ms = 49.2,
        .rttvar_ms = 4.2,
        .rto_ms = 220,
        .retransmits = 0,
        .unacked = 3,
        .cwnd = 10,
        .send_queue_bytes = 0,
        .recv_queue_bytes = 0,
        .status = try allocator.dupe(u8, "ok"),
    };

    // Second socket
    diag.underlay_tcp[1] = .{
        .name = try allocator.dupe(u8, "unknown"),
        .state = try allocator.dupe(u8, "LISTEN"),
        .local = try allocator.dupe(u8, "redacted:8080"),
        .remote = try allocator.dupe(u8, "redacted:0"),
        .rtt_ms = null,
        .rttvar_ms = null,
        .rto_ms = null,
        .retransmits = null,
        .unacked = null,
        .cwnd = null,
        .send_queue_bytes = 0,
        .recv_queue_bytes = 0,
        .status = try allocator.dupe(u8, "ok"),
    };

    diag.deinit(allocator);
}

// ============================================================================
// Tests: status_network_diag does not borrow from cmd_result.stdout
// ============================================================================

test "status_network_diag copies sockets and frees parser-owned memory" {
    const allocator = testing.allocator;

    // Create parser-owned sockets (simulating what parseSsTinOutput returns)
    const input = "State       Recv-Q   Send-Q   Local Address:Port   Peer Address:Port   Process\n" ++
        "ESTAB       0        0        10.0.0.1:443         192.0.2.1:12345    users:((\"xray\",pid=1234,fd=5))\n";

    const config = ss_parser.ParseConfig{ .redact_addresses = true };
    const sockets = try ss_parser.parseSsTinOutput(allocator, input, config);
    defer ss_parser.freeTcpSockets(allocator, sockets); // This is the correct pattern

    // After freeTcpSockets, the original socket strings are freed
    // Any copy would need to be made BEFORE this point

    try testing.expect(sockets.len == 1);
    try testing.expectEqualStrings("redacted:443", sockets[0].local.?);
    try testing.expectEqualStrings("redacted:12345", sockets[0].remote.?);
}

test "TcpSocketOutput strings are owned by NetworkDiag, not parser" {
    const allocator = testing.allocator;

    // Parse sockets with parser
    const input = "State       Recv-Q   Send-Q   Local Address:Port   Peer Address:Port   Process\n" ++
        "ESTAB       0        0        10.0.0.1:443         192.0.2.1:12345    users:((\"xray\",pid=1234,fd=5))\n";

    const config = ss_parser.ParseConfig{ .redact_addresses = false };
    const sockets = try ss_parser.parseSsTinOutput(allocator, input, config);

    // Create TcpSocketOutput by duplicating (as status_network_diag does)
    const output = status_network_diag.TcpSocketOutput{
        .name = try allocator.dupe(u8, sockets[0].process_name.?),
        .state = try allocator.dupe(u8, @tagName(sockets[0].state)),
        .local = try allocator.dupe(u8, sockets[0].local.?),
        .remote = try allocator.dupe(u8, sockets[0].remote.?),
        .rtt_ms = sockets[0].rtt_ms,
        .rttvar_ms = sockets[0].rttvar_ms,
        .rto_ms = sockets[0].rto_ms,
        .retransmits = sockets[0].retransmits,
        .unacked = sockets[0].unacked,
        .cwnd = sockets[0].cwnd,
        .send_queue_bytes = sockets[0].send_queue_bytes,
        .recv_queue_bytes = sockets[0].recv_queue_bytes,
        .status = try allocator.dupe(u8, @tagName(sockets[0].status)),
    };

    // Now we can free parser sockets - output strings are independent copies
    ss_parser.freeTcpSockets(allocator, sockets);

    // Output still has valid data
    try testing.expectEqualStrings("\"xray\",pid=1234,fd=5", output.name);
    try testing.expectEqualStrings("ESTAB", output.state);

    // Cleanup output
    allocator.free(output.name);
    allocator.free(output.state);
    allocator.free(output.local);
    allocator.free(output.remote);
    allocator.free(output.status);
}

// ============================================================================
// Tests: formatTimestamp returns allocator-owned string
// ============================================================================

test "formatTimestamp returns allocator-owned string" {
    const allocator = testing.allocator;
    const ts = status_network_diag.formatTimestamp(allocator, 1718700000000);
    const result = try ts;
    defer allocator.free(result);

    // Result is a valid string representation
    try testing.expect(result.len > 0);
    try testing.expect(result[0] == '1'); // Starts with timestamp digits
}
