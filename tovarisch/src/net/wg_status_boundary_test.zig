// wg_status_boundary_test.zig — Tests for WireGuard status boundary
//
// Tests for parseWgDumpOutput and related parsing logic.

const std = @import("std");
const wg = @import("wg_status_boundary.zig");

// Re-export parseWgDumpOutput for tests
const parseWgDumpOutput = wg.parseWgDumpOutput;

// ============================================================================
// Tests for WireGuard Dump Parser
// ============================================================================

test "parseWgDumpOutput: interface name comes from parameter" {
    // Per-interface dump output doesn't include interface name
    const dump_output = "private_key_base64\t\n";
    const result = try parseWgDumpOutput(dump_output, "wg-kgb0");
    try std.testing.expectEqualStrings("wg-kgb0", result.interface);
}

test "parseWgDumpOutput: zero-peer interface preserves configured interface" {
    const dump_output = "private_key_base64\t\n";
    const result = try parseWgDumpOutput(dump_output, "wg-kgb0");
    try std.testing.expectEqualStrings("wg-kgb0", result.interface);
    try std.testing.expectEqual(@as(u32, 0), result.peer_count);
    try std.testing.expectEqual(@as(?u64, null), result.latest_handshake_epoch_sec);
}

test "parseWgDumpOutput: one peer with handshake" {
    // Format: peer_pubkey \t psk \t endpoint \t allowed_ips \t handshake_epoch \t rx \t tx \t keepalive
    const dump_output = "private_key_base64\t\n" ++
        "peer_pubkey_base64\tpsk_base64\t1.2.3.4:51820\t10.0.0.2/32\t1700000000\t1000\t2000\t25\n";
    const result = try parseWgDumpOutput(dump_output, "wg-kgb0");
    try std.testing.expectEqualStrings("wg-kgb0", result.interface);
    try std.testing.expectEqual(@as(u32, 1), result.peer_count);
    try std.testing.expectEqual(@as(?u64, 1700000000), result.latest_handshake_epoch_sec);
    try std.testing.expectEqual(@as(u64, 1000), result.rx_bytes);
    try std.testing.expectEqual(@as(u64, 2000), result.tx_bytes);
}

test "parseWgDumpOutput: multiple peers aggregate rx/tx and choose max handshake" {
    // First peer: older handshake
    // Second peer: newer handshake
    const dump_output = "private_key_base64\t\n" ++
        "peer1_pubkey\tpsk\t1.1.1.1:51820\t10.0.0.2/32\t1700000000\t1000\t2000\t25\n" ++
        "peer2_pubkey\tpsk\t2.2.2.2:51820\t10.0.0.3/32\t1700001000\t3000\t4000\t0\n";
    const result = try parseWgDumpOutput(dump_output, "wg-kgb0");
    try std.testing.expectEqualStrings("wg-kgb0", result.interface);
    try std.testing.expectEqual(@as(u32, 2), result.peer_count);
    // Should choose max handshake epoch
    try std.testing.expectEqual(@as(?u64, 1700001000), result.latest_handshake_epoch_sec);
    // Should aggregate rx/tx
    try std.testing.expectEqual(@as(u64, 4000), result.rx_bytes);
    try std.testing.expectEqual(@as(u64, 6000), result.tx_bytes);
}

test "parseWgDumpOutput: peer with no handshake (epoch 0)" {
    const dump_output = "private_key_base64\t\n" ++
        "peer_pubkey\tpsk\t1.2.3.4:51820\t10.0.0.2/32\t0\t1000\t2000\t25\n";
    const result = try parseWgDumpOutput(dump_output, "wg-kgb0");
    try std.testing.expectEqual(@as(u32, 1), result.peer_count);
    try std.testing.expectEqual(@as(?u64, null), result.latest_handshake_epoch_sec);
}

test "parseWgDumpOutput: empty output returns noInterface" {
    const result = try parseWgDumpOutput("", "wg-kgb0");
    try std.testing.expectEqualStrings("", result.interface);
    try std.testing.expectEqual(@as(u32, 0), result.peer_count);
}

test "parseWgDumpOutput: listen port parsed correctly" {
    const dump_output = "private_key_base64\tpublic_key_base64\t51820\t0\n";
    const result = try parseWgDumpOutput(dump_output, "wg-kgb0");
    try std.testing.expectEqual(@as(?u16, 51820), result.listen_port);
}

test "parseWgDumpOutput: non-wg0 interface names survive into status" {
    // Test that various interface names are preserved
    const dump_output = "private_key_base64\t\n";
    
    const result1 = try parseWgDumpOutput(dump_output, "wg0");
    try std.testing.expectEqualStrings("wg0", result1.interface);
    
    const result2 = try parseWgDumpOutput(dump_output, "wg100");
    try std.testing.expectEqualStrings("wg100", result2.interface);
    
    const result3 = try parseWgDumpOutput(dump_output, "wg-quick-test");
    try std.testing.expectEqualStrings("wg-quick-test", result3.interface);
}
