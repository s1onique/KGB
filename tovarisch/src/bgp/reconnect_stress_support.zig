// bgp/reconnect_stress_support.zig — production-neutral types and constants for
// reconnect stress testing and proof harnesses.
//
// This module contains NO test declarations and NO std.testing dependencies.
// It provides shared types used by both production proofs and test harnesses.
//
// Production use: reconnect memory proof harnesses and regression tests.
// Test use: stress test infrastructure (wrapped by test files).

const session = @import("session.zig");
const types = @import("types.zig");

/// Number of reconnect generations for stress test (declared here for tests below)
pub const STRESS_TEST_GENERATIONS: u64 = 10_000;

/// Creates a minimal session config for testing.
pub fn makeTestSessionConfig() session.SessionConfig {
    return .{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = .{ 127, 0, 0, 1 },
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{types.Ipv4Prefix.init("10.0.0.0/8")},
        .same_as = true,
    };
}

/// Results from the reconnect stress test.
pub const ReconnectStressResults = struct {
    generations: u64,
    baseline_live_bytes: u64,
    final_live_bytes: u64,
    peak_live_bytes: u64,
    peak_active_timers: u32,
    peak_active_sockets: u32,
    total_reconnect_count: u64,
    allocations_leaked: bool,
    resources_at_baseline: bool,
};
