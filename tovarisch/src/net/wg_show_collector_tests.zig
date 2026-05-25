// wg_show_collector_tests.zig — Unit tests for WireGuard command collector
//
// ACT: Tests for the wg_show_collector module.
// This file tests the parser integration and error handling paths.
// Process execution tests (fork/exec) require Linux integration testing
// with the `wg` binary available.

const std = @import("std");
const wg_show_collector = @import("wg_show_collector.zig");
const wg_show_parser = @import("wg_show_parser.zig");

const testing = std.testing;

// ============================================================================
// Tests: WgDiagnosticsOwned — ownership model
// ============================================================================

test "WgDiagnosticsOwned struct can be constructed with literal slices" {
    // Test that the struct layout works with inline construction
    const owned = wg_show_collector.WgDiagnosticsOwned{
        .diagnostics = .{
            .interface = "wg0",
            .peer_count = 1,
            .latest_handshake_age_sec = 120,
            .rx_bytes = 1000,
            .tx_bytes = 2000,
        },
        .stdout_buf = try testing.allocator.alloc(u8, 32),
    };
    defer {
        // Manual cleanup for test - verify deinit would work
        testing.allocator.free(owned.stdout_buf);
    }

    try testing.expectEqualSlices(u8, "wg0", owned.diagnostics.interface);
    try testing.expectEqual(@as(u32, 1), owned.diagnostics.peer_count);
    try testing.expectEqual(@as(u64, 120), owned.diagnostics.latest_handshake_age_sec.?);
    try testing.expectEqual(@as(u64, 1000), owned.diagnostics.rx_bytes);
    try testing.expectEqual(@as(u64, 2000), owned.diagnostics.tx_bytes);
    try testing.expectEqual(@as(usize, 32), owned.stdout_buf.len);
}

test "WgDiagnosticsOwned.deinit frees stdout_buf" {
    {
        var owned = wg_show_collector.WgDiagnosticsOwned{
            .diagnostics = .{
                .interface = "wg0",
                .peer_count = 0,
                .latest_handshake_age_sec = null,
                .rx_bytes = 0,
                .tx_bytes = 0,
            },
            .stdout_buf = try testing.allocator.alloc(u8, 256),
        };

        // Verify buffer was allocated
        try testing.expectEqual(@as(usize, 256), owned.stdout_buf.len);

        // Deinit should free the buffer
        owned.deinit(testing.allocator);

        // After deinit, fields are cleared
        try testing.expectEqual(@as(usize, 0), owned.stdout_buf.len);
    }
    // If we reach here without panic, deinit worked correctly
}

test "WgDiagnosticsOwned.deinit is idempotent" {
    var owned = wg_show_collector.WgDiagnosticsOwned{
        .diagnostics = .{
            .interface = "wg0",
            .peer_count = 0,
            .latest_handshake_age_sec = null,
            .rx_bytes = 0,
            .tx_bytes = 0,
        },
        .stdout_buf = try testing.allocator.alloc(u8, 64),
    };

    // Call deinit multiple times - should not crash
    owned.deinit(testing.allocator);
    owned.deinit(testing.allocator); // Second call should be safe
    owned.deinit(testing.allocator); // Third call too
}

test "WgDiagnosticsOwned interface slice points into stdout_buf (ownership proof)" {
    // This test demonstrates that interface is a slice of stdout_buf
    const interface_name = "wg0";

    var owned = wg_show_collector.WgDiagnosticsOwned{
        .diagnostics = .{
            .interface = undefined,
            .peer_count = 1,
            .latest_handshake_age_sec = null,
            .rx_bytes = 0,
            .tx_bytes = 0,
        },
        .stdout_buf = try testing.allocator.alloc(u8, 128),
    };
    defer owned.deinit(testing.allocator);

    // Simulate what the parser would do: point interface into stdout_buf
    @memcpy(owned.stdout_buf[0..interface_name.len], interface_name);
    owned.diagnostics.interface = owned.stdout_buf[0..interface_name.len];

    // Verify the slice is correct
    try testing.expectEqualSlices(u8, "wg0", owned.diagnostics.interface);

    // Verify it's actually pointing into stdout_buf (not a copy)
    // We do this by modifying stdout_buf and checking interface sees the change
    owned.stdout_buf[0] = 'W'; // Change 'w' to 'W' (capital)
    try testing.expectEqualSlices(u8, "Wg0", owned.diagnostics.interface);
}

// ============================================================================
// Tests: Constants and Types
// ============================================================================

test "MAX_OUTPUT_SIZE is bounded (8192 bytes)" {
    // Verify the constant exists and is reasonable
    try testing.expect(wg_show_collector.MAX_OUTPUT_SIZE > 1024);
    try testing.expect(wg_show_collector.MAX_OUTPUT_SIZE <= 65536);
}

test "CollectError error set is complete" {
    // Verify error set variants can be used
    try testing.expect(wg_show_collector.CollectError.CommandNotFound == error.CommandNotFound);
    try testing.expect(wg_show_collector.CollectError.CommandFailed == error.CommandFailed);
    try testing.expect(wg_show_collector.CollectError.PipeFailed == error.PipeFailed);
    try testing.expect(wg_show_collector.CollectError.ForkFailed == error.ForkFailed);
    try testing.expect(wg_show_collector.CollectError.ExecFailed == error.ExecFailed);
    try testing.expect(wg_show_collector.CollectError.OutputTruncated == error.OutputTruncated);
    try testing.expect(wg_show_collector.CollectError.MalformedOutput == error.MalformedOutput);
}

// ============================================================================
// Tests: WgDiagnosticsResult type
// ============================================================================

test "WgDiagnosticsResult is optional WgInterface" {
    // Verify the type alias works correctly
    const null_result: wg_show_collector.WgDiagnosticsResult = null;
    try testing.expect(null_result == null);

    const valid_result: wg_show_collector.WgDiagnosticsResult = .{
        .interface = "wg0",
        .peer_count = 2,
        .latest_handshake_age_sec = 60,
        .rx_bytes = 5000,
        .tx_bytes = 3000,
    };
    try testing.expect(valid_result != null);
    try testing.expectEqualSlices(u8, "wg0", valid_result.?.interface);
    try testing.expectEqual(@as(u32, 2), valid_result.?.peer_count);
}

// ============================================================================
// Tests: Error mapping via mapCommandOutputForTest
// ============================================================================

test "valid stdout passed to parser" {
    const fixture = "interface: wg0\n" ++
        "  public key: (hidden)\n" ++
        "  private key: (hidden)\n" ++
        "peer: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx=\n" ++
        "  endpoint: 192.168.1.100:51820\n" ++
        "  allowed ips: 10.0.0.2/32\n" ++
        "  latest handshake: 45 seconds ago\n" ++
        "  transfer: 1234567 bytes received, 987654 bytes sent\n";

    const result = wg_show_collector.mapCommandOutputForTest(fixture, 0);
    try testing.expect(result != error.CommandNotFound);
    try testing.expect(result != error.CommandFailed);
    try testing.expect(result != error.OutputTruncated);
    try testing.expect(result != error.MalformedOutput);

    const diag = try result;
    try testing.expectEqualSlices(u8, "wg0", diag.interface);
    try testing.expectEqual(@as(u32, 1), diag.peer_count);
    try testing.expectEqual(@as(u64, 45), diag.latest_handshake_age_sec.?);
    try testing.expectEqual(@as(u64, 1234567), diag.rx_bytes);
    try testing.expectEqual(@as(u64, 987654), diag.tx_bytes);
}

test "missing command maps to CommandNotFound" {
    // exit_code 127 is the standard "command not found" exit code
    try testing.expectError(error.CommandNotFound, wg_show_collector.mapCommandOutputForTest("", 127));
}

test "non-zero exit maps to CommandFailed" {
    // Any non-zero exit that isn't 127 indicates command failure
    try testing.expectError(error.CommandFailed, wg_show_collector.mapCommandOutputForTest("some error output", 1));
    try testing.expectError(error.CommandFailed, wg_show_collector.mapCommandOutputForTest("permission denied", 2));
    try testing.expectError(error.CommandFailed, wg_show_collector.mapCommandOutputForTest("", 255));
}

test "malformed stdout maps to MalformedOutput" {
    // Empty output has no interface
    try testing.expectError(error.MalformedOutput, wg_show_collector.mapCommandOutputForTest("", 0));

    // Single line without space/colon (no valid interface name)
    try testing.expectError(error.MalformedOutput, wg_show_collector.mapCommandOutputForTest("invalid-only-no-separator", 0));
}

test "oversized stdout maps to OutputTruncated" {
    // Create output larger than MAX_OUTPUT_SIZE
    const oversized = try testing.allocator.alloc(u8, wg_show_collector.MAX_OUTPUT_SIZE + 1);
    defer testing.allocator.free(oversized);
    @memset(oversized, 'x');

    try testing.expectError(error.OutputTruncated, wg_show_collector.mapCommandOutputForTest(oversized, 0));
}

// ============================================================================
// Tests: Parser integration tests
// ============================================================================

test "Valid wg show output parses correctly (parser integration)" {
    // This test verifies that the parser integration works correctly.
    // The actual collectWgDiagnosticsOwned() function would be tested in
    // an integration test that has access to a real `wg` binary.
    const fixture = "interface: wg0\n" ++
        "  public key: (hidden)\n" ++
        "  private key: (hidden)\n" ++
        "peer: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx=\n" ++
        "  endpoint: 192.168.1.100:51820\n" ++
        "  allowed ips: 10.0.0.2/32\n" ++
        "  latest handshake: 45 seconds ago\n" ++
        "  transfer: 1234567 bytes received, 987654 bytes sent\n";

    const result = wg_show_parser.parseWgShowOutput(fixture);
    try testing.expect(result != error.NoInterface);

    const diag = try result;
    try testing.expectEqualSlices(u8, "wg0", diag.interface);
    try testing.expectEqual(@as(u32, 1), diag.peer_count);
    try testing.expectEqual(@as(u64, 45), diag.latest_handshake_age_sec.?);
    try testing.expectEqual(@as(u64, 1234567), diag.rx_bytes);
    try testing.expectEqual(@as(u64, 987654), diag.tx_bytes);
}

test "Malformed output returns NoInterface error (parser integration)" {
    // Empty input simulates the case where wg shows nothing
    const result = wg_show_parser.parseWgShowOutput("");
    try testing.expect(result == error.NoInterface);
}

test "Partial output returns valid struct with zeroed values (parser integration)" {
    // Partial output (no peers, no handshakes) should still parse
    const fixture = "interface: wg0\n";

    const result = wg_show_parser.parseWgShowOutput(fixture);
    try testing.expect(result != error.NoInterface);

    const diag = try result;
    try testing.expectEqualSlices(u8, "wg0", diag.interface);
    try testing.expectEqual(@as(u32, 0), diag.peer_count);
    try testing.expect(diag.latest_handshake_age_sec == null);
    try testing.expectEqual(@as(u64, 0), diag.rx_bytes);
    try testing.expectEqual(@as(u64, 0), diag.tx_bytes);
}

// ============================================================================
// Tests: Buffer size validation
// ============================================================================

test "MAX_OUTPUT_SIZE is large enough for typical wg show output" {
    // Real wg show output for a typical setup with 3 peers is under 2KB.
    // 8KB provides 4x headroom for larger configurations.
    const typical_output_size = 2048;
    try testing.expect(wg_show_collector.MAX_OUTPUT_SIZE >= typical_output_size * 2);
}

test "MAX_OUTPUT_SIZE prevents unbounded growth" {
    // Verify that the buffer limit is enforced
    // This is a compile-time check that MAX_OUTPUT_SIZE is reasonable
    const max_size = wg_show_collector.MAX_OUTPUT_SIZE;
    try testing.expect(max_size >= 4096); // Minimum 4KB
    try testing.expect(max_size <= 32768); // Maximum 32KB
}

// ============================================================================
// Tests: Privacy-aligned data handling
// ============================================================================

test "Endpoint data is not exposed in parsed output" {
    // Verify that endpoint information doesn't leak into the result
    const fixture = "interface: wg0\n" ++
        "peer: xxxxx\n" ++
        "  endpoint: 203.0.113.50:51820\n" ++
        "  latest handshake: 120 seconds ago\n" ++
        "  transfer: 1000 bytes received, 2000 bytes sent\n";

    const result = try wg_show_parser.parseWgShowOutput(fixture);

    // The result should NOT contain endpoint information
    // We only expose: interface, peer_count, handshake_age, rx_bytes, tx_bytes
    try testing.expectEqualSlices(u8, "wg0", result.interface);
    try testing.expectEqual(@as(u32, 1), result.peer_count);
    try testing.expectEqual(@as(u64, 120), result.latest_handshake_age_sec.?);
    try testing.expectEqual(@as(u64, 1000), result.rx_bytes);
    try testing.expectEqual(@as(u64, 2000), result.tx_bytes);
}

test "Public key data is not exposed in parsed output" {
    // Verify that public key information doesn't leak into the result
    const fixture = "interface: wg0\n" ++
        "  public key: AbCdEfGhIjKlMnOpQrStUvWxYz0123456789+=\n" ++
        "  private key: (hidden)\n" ++
        "peer: XxYyZzAaBbCcDdEeFfGgHhIiJjKkLlMmNnOoPp=\n" ++
        "  latest handshake: 300 seconds ago\n" ++
        "  transfer: 5000 bytes received, 6000 bytes sent\n";

    const result = try wg_show_parser.parseWgShowOutput(fixture);

    // Public key should not be in any output field
    try testing.expectEqualSlices(u8, "wg0", result.interface);
    try testing.expectEqual(@as(u32, 1), result.peer_count);
    // No endpoint, public key, or private key in output
    try testing.expect(result.interface.len == "wg0".len);
}

test "Allowed IPs data is not exposed in parsed output" {
    // Verify that allowed IPs don't affect parsed output
    const fixture = "interface: wg0\n" ++
        "peer: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\n" ++
        "  allowed ips: 10.0.0.0/24, 192.168.100.0/24, 172.16.0.0/16\n" ++
        "  latest handshake: 60 seconds ago\n" ++
        "  transfer: 800 bytes received, 900 bytes sent\n";

    const result = try wg_show_parser.parseWgShowOutput(fixture);

    // Allowed IPs should be ignored by the parser
    try testing.expectEqualSlices(u8, "wg0", result.interface);
    try testing.expectEqual(@as(u32, 1), result.peer_count);
    try testing.expectEqual(@as(u64, 60), result.latest_handshake_age_sec.?);
    try testing.expectEqual(@as(u64, 800), result.rx_bytes);
    try testing.expectEqual(@as(u64, 900), result.tx_bytes);
}

// ============================================================================
// Tests: Large peer count handling
// ============================================================================

test "Multiple peers aggregate correctly" {
    const fixture = "interface: wg0\n" ++
        "peer: peer1AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n" ++
        "  latest handshake: 10 seconds ago\n" ++
        "  transfer: 100 bytes received, 200 bytes sent\n" ++
        "peer: peer2AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n" ++
        "  latest handshake: 20 seconds ago\n" ++
        "  transfer: 300 bytes received, 400 bytes sent\n" ++
        "peer: peer3AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n" ++
        "  latest handshake: 30 seconds ago\n" ++
        "  transfer: 500 bytes received, 600 bytes sent\n" ++
        "peer: peer4AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n" ++
        "  latest handshake: 40 seconds ago\n" ++
        "  transfer: 700 bytes received, 800 bytes sent\n" ++
        "peer: peer5AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n" ++
        "  latest handshake: 50 seconds ago\n" ++
        "  transfer: 900 bytes received, 1000 bytes sent\n";

    const result = try wg_show_parser.parseWgShowOutput(fixture);

    try testing.expectEqual(@as(u32, 5), result.peer_count);
    // Latest handshake should be 10s (the most recent)
    try testing.expectEqual(@as(u64, 10), result.latest_handshake_age_sec.?);
    // Aggregate bytes: rx=100+300+500+700+900=2500, tx=200+400+600+800+1000=3000
    try testing.expectEqual(@as(u64, 2500), result.rx_bytes);
    try testing.expectEqual(@as(u64, 3000), result.tx_bytes);
}
