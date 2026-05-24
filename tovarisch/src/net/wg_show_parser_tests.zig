// wg_show_parser_tests.zig — Unit tests for WireGuard `wg show` parser
//
// ACT: Tests for the wg_show_parser module.
// Fixtures use real `wg show` human-readable output format.

const std = @import("std");
const wg_show_parser = @import("wg_show_parser.zig");

const testing = std.testing;
const parseWgShowOutput = wg_show_parser.parseWgShowOutput;
const parseInterfaceHeader = wg_show_parser.parseInterfaceHeader;
const parseHandshakeAge = wg_show_parser.parseHandshakeAge;
const parseTransferBytes = wg_show_parser.parseTransferBytes;
const parseBytesValue = wg_show_parser.parseBytesValue;

// ============================================================================
// Tests: parseWgShowOutput - Real `wg show` format fixtures
// ============================================================================

test "parseWgShowOutput: one interface, one peer (real format)" {
    const input = "interface: wg0\n" ++
        "  public key: AbCdEf1234567890=\n" ++
        "  private key: (hidden)\n" ++
        "  listening port: 51820\n" ++
        "peer: XxYyZz9876543210=\n" ++
        "  endpoint: 192.168.1.100:51820\n" ++
        "  allowed ips: 10.0.0.2/32\n" ++
        "  latest handshake: 120 seconds ago\n" ++
        "  transfer: 1048576 bytes received, 524288 bytes sent\n";

    const result = try parseWgShowOutput(input);

    try testing.expectEqualSlices(u8, "wg0", result.interface);
    try testing.expectEqual(@as(u32, 1), result.peer_count);
    try testing.expectEqual(@as(u64, 120), result.latest_handshake_age_sec.?);
    try testing.expectEqual(@as(u64, 1048576), result.rx_bytes);
    try testing.expectEqual(@as(u64, 524288), result.tx_bytes);
}

test "parseWgShowOutput: multiple peers (real format)" {
    const input = "interface: wg0\n" ++
        "  public key: AAAAAAAAAAAAAAAAAAAAAAAA=\n" ++
        "  private key: (hidden)\n" ++
        "  listening port: 51820\n" ++
        "peer: peer1AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n" ++
        "  latest handshake: 30 seconds ago\n" ++
        "  transfer: 100 bytes received, 200 bytes sent\n" ++
        "peer: peer2AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n" ++
        "  latest handshake: 45 seconds ago\n" ++
        "  transfer: 300 bytes received, 400 bytes sent\n" ++
        "peer: peer3AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n" ++
        "  latest handshake: 60 seconds ago\n" ++
        "  transfer: 500 bytes received, 600 bytes sent\n";

    const result = try parseWgShowOutput(input);

    try testing.expectEqualSlices(u8, "wg0", result.interface);
    try testing.expectEqual(@as(u32, 3), result.peer_count);
    try testing.expectEqual(@as(u64, 30), result.latest_handshake_age_sec.?);
    try testing.expectEqual(@as(u64, 900), result.rx_bytes);
    try testing.expectEqual(@as(u64, 1200), result.tx_bytes);
}

test "parseWgShowOutput: never-handshaked peer (real format)" {
    const input = "interface: wg0\n" ++
        "  public key: AAAAAAAAAAAAAAAAAAAAAAAA=\n" ++
        "  private key: (hidden)\n" ++
        "peer: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\n" ++
        "  endpoint: 10.0.0.2:51820\n" ++
        "  allowed ips: 10.0.0.2/32\n" ++
        "  transfer: 0 bytes received, 0 bytes sent\n";

    const result = try parseWgShowOutput(input);

    try testing.expectEqualSlices(u8, "wg0", result.interface);
    try testing.expectEqual(@as(u32, 1), result.peer_count);
    try testing.expect(result.latest_handshake_age_sec == null);
    try testing.expectEqual(@as(u64, 0), result.rx_bytes);
    try testing.expectEqual(@as(u64, 0), result.tx_bytes);
}

test "parseWgShowOutput: endpoint and allowed IPs ignored (real format)" {
    const input = "interface: wg0\n" ++
        "  public key: AbCdEfGhIjKlMnOpQrStUvWxYz0123456789+=\n" ++
        "  private key: (hidden)\n" ++
        "  listening port: 51820\n" ++
        "peer: XxYyZzAaBbCcDdEeFfGgHhIiJjKkLlMmNnOoPp=\n" ++
        "  endpoint: 203.0.113.50:51820\n" ++
        "  allowed ips: 10.0.0.0/24, 192.168.100.0/24\n" ++
        "  latest handshake: 300 seconds ago\n" ++
        "  transfer: 999999 bytes received, 888888 bytes sent\n";

    const result = try parseWgShowOutput(input);

    try testing.expectEqualSlices(u8, "wg0", result.interface);
    try testing.expectEqual(@as(u32, 1), result.peer_count);
    try testing.expectEqual(@as(u64, 300), result.latest_handshake_age_sec.?);
    try testing.expectEqual(@as(u64, 999999), result.rx_bytes);
    try testing.expectEqual(@as(u64, 888888), result.tx_bytes);
}

test "parseWgShowOutput: malformed/partial output" {
    const input = "interface: wg0\n";

    const result = try parseWgShowOutput(input);

    try testing.expectEqualSlices(u8, "wg0", result.interface);
    try testing.expectEqual(@as(u32, 0), result.peer_count);
    try testing.expectEqual(@as(u64, 0), result.rx_bytes);
    try testing.expectEqual(@as(u64, 0), result.tx_bytes);
}

test "parseWgShowOutput: empty input returns error" {
    const result = parseWgShowOutput("");
    try testing.expect(result == error.NoInterface);
}

test "parseWgShowOutput: whitespace-only input returns error" {
    const result = parseWgShowOutput("   \n   \n   ");
    try testing.expect(result == error.NoInterface);
}

test "parseWgShowOutput: large transfer numbers" {
    const input = "interface: wg0\n" ++
        "  public key: AAAAAAAAAAAAAAAAAAAAAAAA=\n" ++
        "peer: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\n" ++
        "  latest handshake: 1 seconds ago\n" ++
        "  transfer: 10737418240 bytes received, 5368709120 bytes sent\n";

    const result = try parseWgShowOutput(input);

    try testing.expectEqual(@as(u64, 10737418240), result.rx_bytes);
    try testing.expectEqual(@as(u64, 5368709120), result.tx_bytes);
}

test "parseWgShowOutput: interface name with numbers" {
    const input = "interface: wg100\n" ++
        "  public key: AAAAAAAAAAAAAAAAAAAAAAAA=\n" ++
        "peer: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\n" ++
        "  latest handshake: 50 seconds ago\n" ++
        "  transfer: 100 bytes received, 200 bytes sent\n";

    const result = try parseWgShowOutput(input);

    try testing.expectEqualSlices(u8, "wg100", result.interface);
    try testing.expectEqual(@as(u32, 1), result.peer_count);
}

// ============================================================================
// Tests: parseInterfaceHeader
// ============================================================================

test "parseInterfaceHeader: real interface format" {
    const result = parseInterfaceHeader("interface: wg0");
    try testing.expect(result != null);
    try testing.expectEqualSlices(u8, "wg0", result.?);
}

test "parseInterfaceHeader: interface with number" {
    const result = parseInterfaceHeader("interface: wg100");
    try testing.expect(result != null);
    try testing.expectEqualSlices(u8, "wg100", result.?);
}

test "parseInterfaceHeader: empty string" {
    try testing.expect(parseInterfaceHeader("") == null);
}

// ============================================================================
// Tests: parseHandshakeAge
// ============================================================================

test "parseHandshakeAge: standard age" {
    try testing.expectEqual(@as(u64, 120), parseHandshakeAge("latest handshake: 120 seconds ago"));
}

test "parseHandshakeAge: zero age" {
    try testing.expectEqual(@as(u64, 0), parseHandshakeAge("latest handshake: 0 seconds ago"));
}

test "parseHandshakeAge: not handshake line" {
    try testing.expect(parseHandshakeAge("endpoint: 1.2.3.4:51820") == null);
}

// ============================================================================
// Tests: parseTransferBytes
// ============================================================================

test "parseTransferBytes: both directions" {
    const result = parseTransferBytes("transfer: 1000 bytes received, 2000 bytes sent");
    try testing.expect(result != null);
    try testing.expectEqual(@as(u64, 1000), result.?.rx);
    try testing.expectEqual(@as(u64, 2000), result.?.tx);
}

test "parseTransferBytes: only received" {
    const result = parseTransferBytes("transfer: 500 bytes received");
    try testing.expect(result != null);
    try testing.expectEqual(@as(u64, 500), result.?.rx);
    try testing.expectEqual(@as(u64, 0), result.?.tx);
}

test "parseTransferBytes: only sent" {
    const result = parseTransferBytes("transfer: 300 bytes sent");
    try testing.expect(result != null);
    try testing.expectEqual(@as(u64, 0), result.?.rx);
    try testing.expectEqual(@as(u64, 300), result.?.tx);
}

test "parseTransferBytes: not transfer line" {
    try testing.expect(parseTransferBytes("endpoint: 1.2.3.4:51820") == null);
}

// ============================================================================
// Tests: parseBytesValue
// ============================================================================

test "parseBytesValue: standard value" {
    try testing.expectEqual(@as(u64, 1234), parseBytesValue("1234 bytes received"));
}

test "parseBytesValue: zero value" {
    try testing.expectEqual(@as(u64, 0), parseBytesValue("0 bytes received"));
}

test "parseBytesValue: no number" {
    try testing.expectEqual(@as(u64, 0), parseBytesValue("bytes received"));
}
