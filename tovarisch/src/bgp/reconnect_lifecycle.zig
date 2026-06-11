// bgp/reconnect_lifecycle.zig — BGP reconnect/backoff lifecycle management
//
// Extracted from serve_integration.zig for LLM-friendliness.
// Handles exponential backoff, reconnect scheduling, and cleanup signaling.
//
// Reconnect policy:
// - Initial retry: 1s, exponential up to 60s max
// - Backoff resets after successful establishment
// - No hot loop (bounded backoff)

const std = @import("std");
const session = @import("session.zig");
const tcp_transport = @import("tcp_transport.zig");
const transport = @import("transport.zig");
const clock = @import("clock.zig");

// ============================================================================
// Reconnect Configuration
// ============================================================================

/// Default initial reconnect delay in milliseconds.
pub const DEFAULT_RECONNECT_INITIAL_MS: u64 = 1000;

/// Default maximum reconnect delay in milliseconds (1 minute).
pub const DEFAULT_RECONNECT_MAX_MS: u64 = 60_000;

/// Default backoff multiplier (doubling delay each retry).
pub const DEFAULT_RECONNECT_MULTIPLIER: u64 = 2;

// ============================================================================
// Reconnect Lifecycle Interface
// ============================================================================

/// Minimal interface for reconnect lifecycle management.
/// Implement this in your bundle struct to use reconnect capabilities.
pub const ReconnectLifecycle = struct {
    /// Get current backoff delay in milliseconds.
    getBackoffMs: *const fn () u64,
    /// Set backoff delay in milliseconds.
    setBackoffMs: *const fn (u64) void,
    /// Get reconnect deadline (monotonic time).
    getReconnectDeadline: *const fn () clock.MonoTime,
    /// Set reconnect deadline (monotonic time).
    setReconnectDeadline: *const fn (clock.MonoTime) void,
    /// Get runtime state.
    getState: *const fn () ReconnectState,
    /// Set runtime state.
    setState: *const fn (ReconnectState) void,
    /// Get cleanup requested flag.
    isCleanupRequested: *const fn () bool,
    /// Set cleanup requested flag.
    setCleanupRequested: *const fn (bool) void,
    /// Get last error message.
    getLastError: *const fn () ?[]const u8,
    /// Set last error message.
    setLastError: *const fn (?[]const u8) void,
};

/// Reconnect states that can be managed by this lifecycle.
pub const ReconnectState = enum {
    /// Normal running state
    configured,
    /// Waiting for reconnect deadline
    reconnect_wait,
};

// ============================================================================
// Backoff Computation
// ============================================================================

/// Compute the next backoff delay using exponential backoff.
/// Returns the new delay in milliseconds, capped at max_delay.
pub fn computeNextBackoff(
    current_ms: u64,
    max_delay_ms: u64,
) u64 {
    if (current_ms == 0) {
        return DEFAULT_RECONNECT_INITIAL_MS;
    }
    const next = current_ms * DEFAULT_RECONNECT_MULTIPLIER;
    if (next > max_delay_ms) {
        return max_delay_ms;
    }
    return next;
}

/// Schedule a reconnect by setting backoff state and deadline.
/// Uses the provided clock for time operations.
pub fn scheduleReconnect(
    backoff_ms: *u64,
    reconnect_deadline: *clock.MonoTime,
    state: *ReconnectState,
    clock_interface: clock.Clock,
    max_delay_ms: u64,
) void {
    // Compute next backoff delay
    backoff_ms.* = computeNextBackoff(backoff_ms.*, max_delay_ms);

    // Set deadline
    const now = clock_interface.getMonoTimeMs();
    reconnect_deadline.* = now + backoff_ms.*;
    state.* = .reconnect_wait;
}

/// Check if reconnect deadline has elapsed.
pub fn isReconnectReady(
    state: ReconnectState,
    reconnect_deadline: clock.MonoTime,
    clock_interface: clock.Clock,
) bool {
    if (state != .reconnect_wait) {
        return false;
    }
    const now = clock_interface.getMonoTimeMs();
    return now >= reconnect_deadline;
}

/// Reset backoff after successful connection.
pub fn resetBackoff(backoff_ms: *u64, reconnect_deadline: *clock.MonoTime) void {
    backoff_ms.* = 0;
    reconnect_deadline.* = 0;
}
