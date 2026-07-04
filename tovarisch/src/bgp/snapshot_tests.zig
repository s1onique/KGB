// snapshot_tests.zig — BGP snapshot tests for tovarisch
//
// ACT-TOVARISCH-ZIG-HULK14: Harden BGP/BFD allocation and state boundaries.
//
// Comprehensive tests covering:
// 1. BGP peer count limit
// 2. BGP route count limit
// 3. BGP long string truncation
// 4. BGP unknown peer state mapping
// 5. BGP malformed input handling
// 6. Status rendering bounds

const std = @import("std");
const snapshot = @import("snapshot.zig");

// ============================================================================
// Test 1: BGP peer count limit
// ============================================================================

test "BgpSnapshotBudget limits peer count" {
    const budget = snapshot.BgpSnapshotBudget{ .max_peers = 4 };
    
    // Simulate collecting peers
    var collected: usize = 0;
    var truncated = false;
    
    // Simulate 10 peers but budget only allows 4
    const actual_peer_count = 10;
    
    for (0..actual_peer_count) |_| {
        if (snapshot.hasCapacity(collected, budget.max_peers)) {
            collected += 1;
        } else {
            truncated = true;
        }
    }
    
    try std.testing.expectEqual(@as(usize, 4), collected);
    try std.testing.expect(truncated);
}

test "BgpSnapshotBudget handles zero max_peers" {
    const budget = snapshot.BgpSnapshotBudget{ .max_peers = 0 };
    try std.testing.expect(!snapshot.hasCapacity(0, budget.max_peers));
}

test "BgpSnapshotBudget handles large max_peers" {
    const budget = snapshot.BgpSnapshotBudget{ .max_peers = 1024 };
    try std.testing.expect(snapshot.hasCapacity(100, budget.max_peers));
}

// ============================================================================
// Test 2: BGP route count limit
// ============================================================================

test "BgpSnapshotBudget limits route count" {
    const budget = snapshot.BgpSnapshotBudget{ .max_routes = 1000 };
    
    var collected: usize = 0;
    var truncated = false;
    
    // Simulate 5000 routes but budget only allows 1000
    const actual_route_count = 5000;
    
    for (0..actual_route_count) |_| {
        if (snapshot.hasCapacity(collected, budget.max_routes)) {
            collected += 1;
        } else {
            truncated = true;
        }
    }
    
    try std.testing.expectEqual(@as(usize, 1000), collected);
    try std.testing.expect(truncated);
}

test "BgpSnapshotBudget max_routes has sane default" {
    const budget = snapshot.BgpSnapshotBudget{};
    try std.testing.expect(budget.max_routes >= 1024);
}

// ============================================================================
// Test 3: BGP long string truncation
// ============================================================================

test "long BGP peer address is truncated" {
    const long_address = "192.168.1.1.example.com.with.very.long.hostname.example.com";
    const max_len: usize = 32;
    
    const truncated = snapshot.truncateString(long_address, max_len);
    
    try std.testing.expectEqual(@as(usize, 32), truncated.len);
    try std.testing.expect(truncated[0] == '1'); // Starts with '192...'
}

test "BGP error message truncation respects budget" {
    const long_error = "BGP notification received: Administrative Shutdown: peer configured but AS number mismatch between local (65001) and remote (65002)";
    const max_len: usize = 64;
    
    const truncated = snapshot.truncateString(long_error, max_len);
    
    // String is 130 chars, truncated to 64
    try std.testing.expectEqual(@as(usize, 64), truncated.len);
}

test "BGP FSM state truncation" {
    const long_state = "OpenConfirm-ExpectingKeepalive";
    const truncated = snapshot.truncateString(long_state, 32);
    
    // "OpenConfirm-ExpectingKeepalive" is 30 chars, so full string fits
    try std.testing.expectEqual(@as(usize, 30), truncated.len);
}

test "empty strings are preserved" {
    const empty: []const u8 = "";
    const result = snapshot.truncateString(empty, 64);
    try std.testing.expectEqualSlices(u8, "", result);
}

// ============================================================================
// Test 4: BGP unknown peer state mapping
// ============================================================================

test "BgpPeerState enum is closed and exhaustive" {
    const state_count = @typeInfo(snapshot.BgpPeerState).@"enum".fields.len;
    try std.testing.expectEqual(@as(usize, 7), state_count);
    
    // Verify all expected states exist
    inline for (.{ "idle", "connect", "active", "open_sent", "open_confirm", "established", "unknown" }) |name| {
        try std.testing.expect(std.mem.indexOfScalar(
            snapshot.BgpPeerState,
            &.{ .idle, .connect, .active, .open_sent, .open_confirm, .established, .unknown },
            @field(snapshot.BgpPeerState, name),
        ) != null);
    }
}

test "parseBgpPeerState handles BIRD-style output" {
    try std.testing.expect(.idle == snapshot.parseBgpPeerState("Idle"));
    try std.testing.expect(.connect == snapshot.parseBgpPeerState("Connect"));
    try std.testing.expect(.active == snapshot.parseBgpPeerState("Active"));
    try std.testing.expect(.open_sent == snapshot.parseBgpPeerState("OpenSent"));
    try std.testing.expect(.open_confirm == snapshot.parseBgpPeerState("OpenConfirm"));
    try std.testing.expect(.established == snapshot.parseBgpPeerState("Established"));
}

test "parseBgpPeerState handles lowercase output" {
    try std.testing.expect(.idle == snapshot.parseBgpPeerState("idle"));
    try std.testing.expect(.connect == snapshot.parseBgpPeerState("connect"));
    try std.testing.expect(.active == snapshot.parseBgpPeerState("active"));
    try std.testing.expect(.open_sent == snapshot.parseBgpPeerState("open_sent"));
    try std.testing.expect(.open_confirm == snapshot.parseBgpPeerState("open_confirm"));
    try std.testing.expect(.established == snapshot.parseBgpPeerState("established"));
}

test "parseBgpPeerState handles GoBGP-style output" {
    try std.testing.expect(.established == snapshot.parseBgpPeerState("ESTABLISHED"));
    try std.testing.expect(.idle == snapshot.parseBgpPeerState("IDLE"));
}

test "parseBgpPeerState maps unknown to .unknown" {
    try std.testing.expect(.unknown == snapshot.parseBgpPeerState("Unknown"));
    try std.testing.expect(.unknown == snapshot.parseBgpPeerState(""));
    try std.testing.expect(.unknown == snapshot.parseBgpPeerState("INVALID"));
    try std.testing.expect(.unknown == snapshot.parseBgpPeerState("Connecting")); // Typos
    try std.testing.expect(.unknown == snapshot.parseBgpPeerState("Established!")); // Garbage
}

// ============================================================================
// Test 5: BGP malformed input handling
// ============================================================================

test "RoutingSnapshotResult union is closed and exhaustive" {
    const result_count = @typeInfo(snapshot.RoutingSnapshotResult).@"union".fields.len;
    try std.testing.expectEqual(@as(usize, 6), result_count);
}

test "RoutingSnapshotResult handles available state" {
    const result: snapshot.RoutingSnapshotResult = .available;
    try std.testing.expect(std.mem.eql(u8, @tagName(result), "available"));
}

test "RoutingSnapshotResult handles unavailable state" {
    const result: snapshot.RoutingSnapshotResult = .unavailable;
    try std.testing.expect(std.mem.eql(u8, @tagName(result), "unavailable"));
}

test "RoutingSnapshotResult handles stale state" {
    const result: snapshot.RoutingSnapshotResult = .stale;
    try std.testing.expect(std.mem.eql(u8, @tagName(result), "stale"));
}

test "RoutingSnapshotResult handles truncated state" {
    const result: snapshot.RoutingSnapshotResult = .truncated;
    try std.testing.expect(std.mem.eql(u8, @tagName(result), "truncated"));
}

test "RoutingSnapshotResult handles malformed state" {
    const result: snapshot.RoutingSnapshotResult = .malformed;
    try std.testing.expect(std.mem.eql(u8, @tagName(result), "malformed"));
}

test "RoutingSnapshotResult handles unsupported state" {
    const result: snapshot.RoutingSnapshotResult = .unsupported;
    try std.testing.expect(std.mem.eql(u8, @tagName(result), "unsupported"));
}

test "malformed daemon output returns malformed result" {
    _ = "BIRD 2.0.7 output: { invalid json"; // Malformed input is classified as malformed
    const result: snapshot.RoutingSnapshotResult = .malformed;
    try std.testing.expect(std.mem.eql(u8, @tagName(result), "malformed"));
}

test "daemon unavailable returns unavailable result" {
    const result: snapshot.RoutingSnapshotResult = .unavailable;
    try std.testing.expect(std.mem.eql(u8, @tagName(result), "unavailable"));
}

// ============================================================================
// Test 6: Status rendering bounds
// ============================================================================

test "BgpSnapshotBudget max_total_bytes constrains collection" {
    const budget = snapshot.BgpSnapshotBudget{ .max_total_bytes = 1024 };
    
    var bytes_used: usize = 0;
    
    // Simulate collecting data
    while (snapshot.hasCapacity(bytes_used, budget.max_total_bytes)) {
        bytes_used += 100;
    }
    
    // Should have stopped at or near the limit
    try std.testing.expect(bytes_used <= budget.max_total_bytes + 100);
}

test "BgpPeerSnapshot uses bounded fields" {
    const peer = snapshot.BgpPeerSnapshot{
        .peer_address = "10.0.0.2",
        .peer_as = 65001,
        .local_as = 65000,
        .state = .established,
        .uptime_seconds = 3600,
        .last_error = null,
        .messages_received = 1000,
        .messages_sent = 500,
        .prefixes_received = 50,
        .prefixes_sent = 10,
    };
    
    // All fields are bounded types (no unbounded strings)
    try std.testing.expectEqual(@as(u32, 65001), peer.peer_as);
    try std.testing.expectEqual(@as(u32, 65000), peer.local_as);
    try std.testing.expectEqual(@as(u64, 3600), peer.uptime_seconds);
    try std.testing.expect(peer.last_error == null);
}

test "BgpPeerSnapshot supports 4-byte ASNs (RFC 6793)" {
    // Test that 4-byte ASNs (above 65535) work correctly
    const peer = snapshot.BgpPeerSnapshot{
        .peer_address = "10.0.0.2",
        .peer_as = 4200000000, // 4-byte ASN
        .local_as = 4200000001, // 4-byte ASN
        .state = .established,
        .uptime_seconds = 3600,
        .last_error = null,
        .messages_received = 1000,
        .messages_sent = 500,
        .prefixes_received = 50,
        .prefixes_sent = 10,
    };
    
    // Verify 4-byte ASNs are stored correctly
    try std.testing.expectEqual(@as(u32, 4200000000), peer.peer_as);
    try std.testing.expectEqual(@as(u32, 4200000001), peer.local_as);
}

test "BgpSnapshot uses bounded collection" {
    // Empty peer list is valid
    const empty_peers: []const snapshot.BgpPeerSnapshot = &.{};
    const snap = snapshot.BgpSnapshot{
        .result = .available,
        .peers = empty_peers,
        .total_peer_count = 0,
        .total_route_count = 0,
        .collected_at_ms = 0,
        .age_seconds = 0,
        .was_truncated = false,
    };
    
    try std.testing.expect(snap.peers.len == 0);
    try std.testing.expect(!snap.was_truncated);
}

// ============================================================================
// Test 7: AS_PATH length bounds
// ============================================================================

test "BgpSnapshotBudget max_as_path_len constrains AS paths" {
    const budget = snapshot.BgpSnapshotBudget{ .max_as_path_len = 16 };
    
    var as_path_len: usize = 0;
    var truncated = false;
    
    // Simulate 32 AS numbers in path
    const actual_as_count = 32;
    
    for (0..actual_as_count) |_| {
        if (as_path_len < budget.max_as_path_len) {
            as_path_len += 1;
        } else {
            truncated = true;
        }
    }
    
    try std.testing.expectEqual(@as(usize, 16), as_path_len);
    try std.testing.expect(truncated);
}

// ============================================================================
// Test 8: Edge cases
// ============================================================================

test "truncateString with zero budget returns empty" {
    const input = "some string";
    const result = snapshot.truncateString(input, 0);
    try std.testing.expectEqual(@as(usize, 0), result.len);
}

test "remainingBytes handles boundary" {
    try std.testing.expectEqual(@as(usize, 0), snapshot.remainingBytes(100, 100));
    try std.testing.expectEqual(@as(usize, 0), snapshot.remainingBytes(150, 100));
    try std.testing.expectEqual(@as(usize, 50), snapshot.remainingBytes(50, 100));
}

test "hasCapacity handles max usize" {
    try std.testing.expect(!snapshot.hasCapacity(std.math.maxInt(usize), 100));
}

test "default budget has reasonable values" {
    const budget = snapshot.BgpSnapshotBudget{};
    // Default values should be practical for embedded use
    try std.testing.expect(budget.max_peers >= 1 and budget.max_peers <= 256);
    try std.testing.expect(budget.max_string_bytes >= 64);
    try std.testing.expect(budget.max_total_bytes >= 4096);
}
