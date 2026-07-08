// idle_telemetry.zig — Idle memory attribution tick counters
//
// Tracks tick counts for background loops to help correlate anonymous
// heap growth with specific subsystems.
//
// Tick counters are per-u64 atomics using @atomicRmw/@atomicLoad for
// safe cross-thread access from BGP/BFD/heartbeat threads.

const std = @import("std");
const telemetry = @import("telemetry.zig");

/// Per-subsystem tick counters using actual atomics.
/// Incremented from different threads: BGP FSM, BFD transmit, BFD receive, heartbeat.
/// Uses @atomicRmw(.Add) for increment and @atomicLoad for snapshot reads.
var bgp_fsm_ticks: u64 = 0;
var bfd_transmit_ticks: u64 = 0;
var bfd_receive_ticks: u64 = 0;
var heartbeat_ticks: u64 = 0;

/// Increment BGP FSM tick counter.
/// Called from bgp/runtime.zig on each FSM loop iteration.
pub fn incrementBgpFsmTicks() void {
    _ = @atomicRmw(u64, &bgp_fsm_ticks, .Add, 1, .monotonic);
}

/// Increment BFD transmit tick counter.
/// Called from bfd/transmit.zig on each tick iteration.
pub fn incrementBfdTransmitTicks() void {
    _ = @atomicRmw(u64, &bfd_transmit_ticks, .Add, 1, .monotonic);
}

/// Increment BFD receive tick counter.
/// Called from bfd/receive.zig on each poll iteration.
pub fn incrementBfdReceiveTicks() void {
    _ = @atomicRmw(u64, &bfd_receive_ticks, .Add, 1, .monotonic);
}

/// Increment heartbeat tick counter.
/// Called from heartbeat on each 30s interval.
pub fn incrementHeartbeatTicks() void {
    _ = @atomicRmw(u64, &heartbeat_ticks, .Add, 1, .monotonic);
}

/// Get current tick counter snapshot using atomic loads.
pub fn getTickCounters() telemetry.TickCounters {
    return .{
        .bgp_fsm_ticks = @atomicLoad(u64, &bgp_fsm_ticks, .monotonic),
        .bfd_transmit_ticks = @atomicLoad(u64, &bfd_transmit_ticks, .monotonic),
        .bfd_receive_ticks = @atomicLoad(u64, &bfd_receive_ticks, .monotonic),
        .heartbeat_ticks = @atomicLoad(u64, &heartbeat_ticks, .monotonic),
    };
}

/// Reset counters for testing - must only be used in test contexts.
pub fn resetForTesting() void {
    @atomicStore(u64, &bgp_fsm_ticks, 0, .monotonic);
    @atomicStore(u64, &bfd_transmit_ticks, 0, .monotonic);
    @atomicStore(u64, &bfd_receive_ticks, 0, .monotonic);
    @atomicStore(u64, &heartbeat_ticks, 0, .monotonic);
}

// ============================================================================
// Tests
// ============================================================================

test "idle_telemetry counters increment atomically" {
    resetForTesting();
    // Test that increment functions work
    incrementBgpFsmTicks();
    incrementBgpFsmTicks();
    incrementBfdTransmitTicks();
    incrementBfdReceiveTicks();
    incrementHeartbeatTicks();

    const snapshot = getTickCounters();
    try std.testing.expectEqual(@as(u64, 2), snapshot.bgp_fsm_ticks);
    try std.testing.expectEqual(@as(u64, 1), snapshot.bfd_transmit_ticks);
    try std.testing.expectEqual(@as(u64, 1), snapshot.bfd_receive_ticks);
    try std.testing.expectEqual(@as(u64, 1), snapshot.heartbeat_ticks);
}

test "TickCounters default values" {
    const counters = telemetry.TickCounters{};
    try std.testing.expectEqual(@as(u64, 0), counters.bgp_fsm_ticks);
    try std.testing.expectEqual(@as(u64, 0), counters.bfd_transmit_ticks);
    try std.testing.expectEqual(@as(u64, 0), counters.bfd_receive_ticks);
    try std.testing.expectEqual(@as(u64, 0), counters.heartbeat_ticks);
}
