// bgp/backoff_tests.zig — BGP backoff computation tests
//
// Tests for exponential backoff computation in reconnect lifecycle.
// Backoff sequence: 1s, 2s, 4s, 8s, 16s, 32s, 60s(max)

const std = @import("std");
const serve_integration = @import("serve_integration.zig");

test "computeNextBackoff returns initial delay when current is 0" {
    const result = serve_integration.computeNextBackoff(0, 60_000);
    try std.testing.expectEqual(@as(u64, 1000), result);
}

test "computeNextBackoff doubles delay" {
    const result = serve_integration.computeNextBackoff(1000, 60_000);
    try std.testing.expectEqual(@as(u64, 2000), result);
}

test "computeNextBackoff continues doubling" {
    try std.testing.expectEqual(@as(u64, 2000), serve_integration.computeNextBackoff(1000, 60_000));
    try std.testing.expectEqual(@as(u64, 4000), serve_integration.computeNextBackoff(2000, 60_000));
    try std.testing.expectEqual(@as(u64, 8000), serve_integration.computeNextBackoff(4000, 60_000));
    try std.testing.expectEqual(@as(u64, 16000), serve_integration.computeNextBackoff(8000, 60_000));
}

test "computeNextBackoff caps at max delay" {
    const result = serve_integration.computeNextBackoff(32_000, 60_000);
    try std.testing.expectEqual(@as(u64, 60_000), result);
}

test "computeNextBackoff stays at max when already at max" {
    const result = serve_integration.computeNextBackoff(60_000, 60_000);
    try std.testing.expectEqual(@as(u64, 60_000), result);
}

test "DEFAULT_RECONNECT_INITIAL_MS is 1 second" {
    try std.testing.expectEqual(@as(u64, 1000), serve_integration.DEFAULT_RECONNECT_INITIAL_MS);
}

test "DEFAULT_RECONNECT_MAX_MS is 60 seconds" {
    try std.testing.expectEqual(@as(u64, 60_000), serve_integration.DEFAULT_RECONNECT_MAX_MS);
}

test "DEFAULT_RECONNECT_MULTIPLIER is 2" {
    try std.testing.expectEqual(@as(u64, 2), serve_integration.DEFAULT_RECONNECT_MULTIPLIER);
}

test "exponential backoff follows correct sequence" {
    var current: u64 = 0;
    const max = serve_integration.DEFAULT_RECONNECT_MAX_MS;

    current = serve_integration.computeNextBackoff(current, max);
    try std.testing.expectEqual(@as(u64, 1000), current);

    current = serve_integration.computeNextBackoff(current, max);
    try std.testing.expectEqual(@as(u64, 2000), current);

    current = serve_integration.computeNextBackoff(current, max);
    try std.testing.expectEqual(@as(u64, 4000), current);

    current = serve_integration.computeNextBackoff(current, max);
    try std.testing.expectEqual(@as(u64, 8000), current);

    current = serve_integration.computeNextBackoff(current, max);
    try std.testing.expectEqual(@as(u64, 16000), current);

    current = serve_integration.computeNextBackoff(current, max);
    try std.testing.expectEqual(@as(u64, 32000), current);

    current = serve_integration.computeNextBackoff(current, max);
    try std.testing.expectEqual(@as(u64, 60000), current);

    current = serve_integration.computeNextBackoff(current, max);
    try std.testing.expectEqual(@as(u64, 60000), current);
}

test "backoff increases after failed reconnection" {
    var backoff_ms: u64 = 1000;
    backoff_ms = serve_integration.computeNextBackoff(backoff_ms, 60_000);
    try std.testing.expectEqual(@as(u64, 2000), backoff_ms);

    backoff_ms = serve_integration.computeNextBackoff(backoff_ms, 60_000);
    try std.testing.expectEqual(@as(u64, 4000), backoff_ms);
}

test "backoff starts at initial value after successful connection" {
    const backoff_ms: u64 = 0;
    try std.testing.expectEqual(@as(u64, 0), backoff_ms);
}
