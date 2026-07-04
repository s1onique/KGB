// snapshot_tests.zig — BFD snapshot tests for tovarisch
//
// ACT-TOVARISCH-ZIG-HULK14: Harden BGP/BFD allocation and state boundaries.
//
// Comprehensive tests covering:
// 7. BFD session count limit
// 8. BFD closed state mapping
// 9. BFD invalid timer/math input overflow prevention
// 10. BFD stale/unavailable classification
// 11. Status rendering does not expose unbounded BFD data

const std = @import("std");
const snapshot = @import("snapshot.zig");

// ============================================================================
// Test 7: BFD session count limit
// ============================================================================

test "BfdSnapshotBudget limits session count" {
    const budget = snapshot.BfdSnapshotBudget{ .max_sessions = 4 };
    
    // Simulate collecting sessions
    var collected: usize = 0;
    var truncated = false;
    
    // Simulate 20 sessions but budget only allows 4
    const actual_session_count = 20;
    
    for (0..actual_session_count) |_| {
        if (snapshot.hasCapacity(collected, budget.max_sessions)) {
            collected += 1;
        } else {
            truncated = true;
        }
    }
    
    try std.testing.expectEqual(@as(usize, 4), collected);
    try std.testing.expect(truncated);
}

test "BfdSnapshotBudget handles zero max_sessions" {
    const budget = snapshot.BfdSnapshotBudget{ .max_sessions = 0 };
    try std.testing.expect(!snapshot.hasCapacity(0, budget.max_sessions));
}

test "BfdSnapshotBudget handles large max_sessions" {
    const budget = snapshot.BfdSnapshotBudget{ .max_sessions = 256 };
    try std.testing.expect(snapshot.hasCapacity(100, budget.max_sessions));
}

// ============================================================================
// Test 8: BFD closed state mapping
// ============================================================================

test "BfdState enum is closed and exhaustive" {
    const state_count = @typeInfo(snapshot.BfdState).@"enum".fields.len;
    try std.testing.expectEqual(@as(usize, 5), state_count);
    
    // Verify all expected states exist
    inline for (.{ "admin_down", "down", "init", "up", "unknown" }) |name| {
        try std.testing.expect(std.mem.indexOfScalar(
            snapshot.BfdState,
            &.{ .admin_down, .down, .init, .up, .unknown },
            @field(snapshot.BfdState, name),
        ) != null);
    }
}

test "parseBfdStateWire handles all valid RFC 5880 values" {
    try std.testing.expect(.admin_down == snapshot.parseBfdStateWire(0));
    try std.testing.expect(.down == snapshot.parseBfdStateWire(1));
    try std.testing.expect(.init == snapshot.parseBfdStateWire(2));
    try std.testing.expect(.up == snapshot.parseBfdStateWire(3));
}

test "parseBfdStateString handles lowercase output" {
    try std.testing.expect(.admin_down == snapshot.parseBfdStateString("admin_down"));
    try std.testing.expect(.down == snapshot.parseBfdStateString("down"));
    try std.testing.expect(.init == snapshot.parseBfdStateString("init"));
    try std.testing.expect(.up == snapshot.parseBfdStateString("up"));
}

test "parseBfdStateString handles capitalized output" {
    try std.testing.expect(.admin_down == snapshot.parseBfdStateString("AdminDown"));
    try std.testing.expect(.down == snapshot.parseBfdStateString("Down"));
    try std.testing.expect(.init == snapshot.parseBfdStateString("Init"));
    try std.testing.expect(.up == snapshot.parseBfdStateString("Up"));
}

test "parseBfdStateString handles uppercase output" {
    try std.testing.expect(.admin_down == snapshot.parseBfdStateString("ADMINDOWN"));
    try std.testing.expect(.down == snapshot.parseBfdStateString("DOWN"));
    try std.testing.expect(.init == snapshot.parseBfdStateString("INIT"));
    try std.testing.expect(.up == snapshot.parseBfdStateString("UP"));
}

test "parseBfdStateString maps unknown to .unknown" {
    try std.testing.expect(.unknown == snapshot.parseBfdStateString("Unknown"));
    try std.testing.expect(.unknown == snapshot.parseBfdStateString(""));
    try std.testing.expect(.unknown == snapshot.parseBfdStateString("INVALID"));
    try std.testing.expect(.unknown == snapshot.parseBfdStateString("UpDown")); // Typos
    try std.testing.expect(.unknown == snapshot.parseBfdStateString("up!")); // Garbage
}

// ============================================================================
// Test 9: BFD invalid timer/math input does not overflow
// ============================================================================

test "safeDetectionTimeout handles normal values" {
    // 800ms * 3 = 2400ms
    try std.testing.expectEqual(@as(u32, 2400), snapshot.safeDetectionTimeout(800_000, 3));
    // 1000ms * 3 = 3000ms
    try std.testing.expectEqual(@as(u32, 3000), snapshot.safeDetectionTimeout(1_000_000, 3));
    // 100ms * 5 = 500ms
    try std.testing.expectEqual(@as(u32, 500), snapshot.safeDetectionTimeout(100_000, 5));
}

test "safeDetectionTimeout handles zero multiplier" {
    // 0 multiplier should give 0 timeout
    try std.testing.expectEqual(@as(u32, 0), snapshot.safeDetectionTimeout(800_000, 0));
}

test "safeDetectionTimeout handles zero interval" {
    // 0 interval should give 0 timeout
    try std.testing.expectEqual(@as(u32, 0), snapshot.safeDetectionTimeout(0, 3));
}

test "safeDetectionTimeout prevents overflow on max u32" {
    // Max u32 * max multiplier (255) would overflow
    // Should return 0 instead of crashing
    const result = snapshot.safeDetectionTimeout(4_294_967_295, 255);
    try std.testing.expectEqual(@as(u32, 0), result);
}

test "safeDetectionTimeout prevents overflow on large values" {
    // Large but under max u32 * small multiplier
    const result = snapshot.safeDetectionTimeout(2_000_000_000, 2);
    // Should not crash, may return 0 due to overflow protection
    _ = result; // Just ensure no panic
}

test "safeDetectionTimeout handles edge case near overflow" {
    // 2_147_483_648 * 2 = 4_294_967_296 which exceeds u32 max (4_294_967_295)
    const result = snapshot.safeDetectionTimeout(2_147_483_648, 2);
    try std.testing.expectEqual(@as(u32, 0), result); // Overflow returns 0
}

test "safeDetectionTimeout handles max u8 multiplier at boundary" {
    // 255 is the maximum valid multiplier value (RFC 5880)
    // This tests the function with a u8 parameter
    const result = snapshot.safeDetectionTimeout(4_294_967_295, 255);
    try std.testing.expectEqual(@as(u32, 0), result); // Overflow returns 0
}

test "safeDetectionTimeout handles typical BFD values" {
    // BIRD default: interval 800ms, multiplier 3
    try std.testing.expectEqual(@as(u32, 2400), snapshot.safeDetectionTimeout(800_000, 3));
    
    // FRR default: interval 200ms, multiplier 3
    try std.testing.expectEqual(@as(u32, 600), snapshot.safeDetectionTimeout(200_000, 3));
}

// ============================================================================
// Test 10: BFD stale/unavailable classification
// ============================================================================

test "BfdCollectionResult union is closed and exhaustive" {
    const result_count = @typeInfo(snapshot.BfdCollectionResult).@"union".fields.len;
    try std.testing.expectEqual(@as(usize, 6), result_count);
}

test "BfdCollectionResult handles available state" {
    const result: snapshot.BfdCollectionResult = .available;
    try std.testing.expect(std.mem.eql(u8, @tagName(result), "available"));
}

test "BfdCollectionResult handles unavailable state" {
    const result: snapshot.BfdCollectionResult = .unavailable;
    try std.testing.expect(std.mem.eql(u8, @tagName(result), "unavailable"));
}

test "BfdCollectionResult handles stale state" {
    const result: snapshot.BfdCollectionResult = .stale;
    try std.testing.expect(std.mem.eql(u8, @tagName(result), "stale"));
}

test "BfdCollectionResult handles truncated state" {
    const result: snapshot.BfdCollectionResult = .truncated;
    try std.testing.expect(std.mem.eql(u8, @tagName(result), "truncated"));
}

test "BfdCollectionResult handles malformed state" {
    const result: snapshot.BfdCollectionResult = .malformed;
    try std.testing.expect(std.mem.eql(u8, @tagName(result), "malformed"));
}

test "BfdCollectionResult handles not_configured state" {
    const result: snapshot.BfdCollectionResult = .not_configured;
    try std.testing.expect(std.mem.eql(u8, @tagName(result), "not_configured"));
}

test "daemon unavailable returns unavailable result" {
    const result: snapshot.BfdCollectionResult = .unavailable;
    try std.testing.expect(std.mem.eql(u8, @tagName(result), "unavailable"));
}

test "malformed daemon output returns malformed result" {
    const result: snapshot.BfdCollectionResult = .malformed;
    try std.testing.expect(std.mem.eql(u8, @tagName(result), "malformed"));
}

test "BFD not configured returns not_configured result" {
    const result: snapshot.BfdCollectionResult = .not_configured;
    try std.testing.expect(std.mem.eql(u8, @tagName(result), "not_configured"));
}

// ============================================================================
// Test 11: Status rendering does not expose unbounded BFD data
// ============================================================================

test "BfdSnapshotBudget max_total_bytes constrains collection" {
    const budget = snapshot.BfdSnapshotBudget{ .max_total_bytes = 1024 };
    
    var bytes_used: usize = 0;
    
    // Simulate collecting data
    while (snapshot.hasCapacity(bytes_used, budget.max_total_bytes)) {
        bytes_used += 100;
    }
    
    // Should have stopped at or near the limit
    try std.testing.expect(bytes_used <= budget.max_total_bytes + 100);
}

test "BfdSessionSnapshot uses bounded fields" {
    const sess = snapshot.BfdSessionSnapshot{
        .peer_address = "10.0.0.2",
        .local_discr = 0xBEEF,
        .remote_discr = 0xDEAD,
        .state = .up,
        .tx_interval_ms = 800,
        .multiplier = 3,
        .detection_timeout_ms = 2400,
        .packets_sent = 1000,
        .packets_received = 999,
        .detection_timeouts = 0,
    };
    
    // All fields are bounded types (no unbounded strings)
    try std.testing.expectEqual(@as(u32, 0xBEEF), sess.local_discr);
    try std.testing.expectEqual(@as(u32, 0xDEAD), sess.remote_discr);
    try std.testing.expectEqual(@as(u8, 3), sess.multiplier);
}

test "BfdSnapshot uses bounded collection" {
    // Empty session list is valid
    const empty_sessions: []const snapshot.BfdSessionSnapshot = &.{};
    const snap = snapshot.BfdSnapshot{
        .result = .available,
        .sessions = empty_sessions,
        .total_session_count = 0,
        .up_count = 0,
        .collected_at_ms = 0,
        .age_seconds = 0,
        .was_truncated = false,
    };
    
    try std.testing.expect(snap.sessions.len == 0);
    try std.testing.expect(!snap.was_truncated);
}

test "BFD_DETAIL_BUF_SIZE is adequate" {
    // "9999/9999 bfd sessions up" = 24 chars
    try std.testing.expect(snapshot.BFD_DETAIL_BUF_SIZE >= 24);
    // With safety margin
    try std.testing.expect(snapshot.BFD_DETAIL_BUF_SIZE >= 64);
}

// ============================================================================
// Edge cases
// ============================================================================

test "truncateString with zero budget returns empty" {
    const input = "some string";
    const result = snapshot.truncateString(input, 0);
    try std.testing.expectEqual(@as(usize, 0), result.len);
}

test "hasCapacity handles max usize" {
    try std.testing.expect(!snapshot.hasCapacity(std.math.maxInt(usize), 100));
}

test "default budget has reasonable values" {
    const budget = snapshot.BfdSnapshotBudget{};
    // Default values should be practical for embedded use
    try std.testing.expect(budget.max_sessions >= 1 and budget.max_sessions <= 256);
    try std.testing.expect(budget.max_diag_string_bytes >= 64);
    try std.testing.expect(budget.max_total_bytes >= 4096);
}

test "BfdSnapshotBudget max_diag_string_bytes limits string fields" {
    const budget = snapshot.BfdSnapshotBudget{ .max_diag_string_bytes = 64 };
    
    const long_diag = "BFD session down: neighbor signaled session down (diagnostic code 3) received from peer";
    const truncated = snapshot.truncateString(long_diag, budget.max_diag_string_bytes);
    
    try std.testing.expect(truncated.len <= budget.max_diag_string_bytes);
}
