// session_timers.zig — BGP session timer logic
//
// Timer abstractions for BGP KEEPALIVE/hold timers.
// Extracted from session.zig to satisfy LLM-friendliness line limits.

const std = @import("std");
const types = @import("types.zig");
const message = @import("message.zig");
const frame_decode = @import("frame_decode.zig");
const session_status = @import("session_status.zig");
const clock_mod = @import("clock.zig");
const notification_decode = @import("notification_decode.zig");

// Re-export types for convenience
pub const SessionState = session_status.SessionState;
pub const SessionError = session_status.SessionError;
pub const SessionStatus = session_status.SessionStatus;
pub const Clock = clock_mod.Clock;
pub const MonoTime = clock_mod.MonoTime;

// Re-export clock types
pub const RealClock = clock_mod.RealClock;
pub const MockClock = clock_mod.MockClock;

// Re-export notification decode
pub const formatNotification = notification_decode.formatNotification;
pub const getErrorCodeName = notification_decode.getErrorCodeName;
pub const getErrorSubcodeName = notification_decode.getErrorSubcodeName;

// Result of stepEstablished for fine-grained state tracking.
pub const EstablishedStepResult = enum {
    ok,
    keepalive_sent,
    hold_timer_expired,
};

// === Timer Functions ===

/// Calculate keepalive interval from negotiated hold time.
/// Per RFC 4271: Keepalive interval = min(keepalive_seconds, hold_time / 3).
/// Returns 0 if hold_time is 0 (disabled).
pub fn calcKeepaliveInterval(config_keepalive: u16, negotiated_hold: u16) u32 {
    if (negotiated_hold == 0) return 0;
    const keepalive_from_hold = @divFloor(@as(u32, negotiated_hold), 3);
    const configured = @as(u32, config_keepalive);
    if (configured > 0 and configured < keepalive_from_hold) {
        return configured * 1000;
    }
    return keepalive_from_hold * 1000;
}

/// Reset the hold timer to current time + negotiated hold time.
/// If negotiated hold time is 0, the hold timer is disabled.
pub fn resetHoldTimer(
    negotiated_hold_time: u16,
    clock: Clock,
    hold_timer_deadline: *u64,
) void {
    if (negotiated_hold_time == 0) {
        hold_timer_deadline.* = 0;
        return;
    }
    const now = clock.getMonoTimeMs();
    const hold_ms = @as(u64, negotiated_hold_time) * 1000;
    hold_timer_deadline.* = now + hold_ms;
}

/// Check if the hold timer has expired.
pub fn isHoldTimerExpired(
    negotiated_hold_time: u16,
    hold_timer_deadline: u64,
    clock: Clock,
) bool {
    if (negotiated_hold_time == 0) return false;
    if (hold_timer_deadline == 0) return false;
    const now = clock.getMonoTimeMs();
    return now >= hold_timer_deadline;
}

/// Step the established-state session timer logic.
/// Returns EstablishedStepResult indicating what happened.
pub fn stepEstablishedTimers(
    negotiated_hold_time: u16,
    keepalive_interval_ms: u32,
    pending_keepalive: bool,
    hold_timer_deadline: u64,
    clock: Clock,
    send_buf: *[4096]u8,
    send_pos: *usize,
    status: *session_status.SessionStatus,
    pending_keepalive_ms: *u64,
) EstablishedStepResult {
    if (negotiated_hold_time == 0) return .ok;

    const now = clock.getMonoTimeMs();

    // Check hold timer expiry FIRST
    if (hold_timer_deadline > 0 and now >= hold_timer_deadline) {
        return .hold_timer_expired;
    }

    // Check if keepalive is due
    if (pending_keepalive) {
        const elapsed = now -% pending_keepalive_ms.*;
        if (elapsed >= keepalive_interval_ms) {
            send_pos.* = message.encodeKeepalive(send_buf);
            status.keepalives_sent += 1;
            status.messages_sent += 1;
            pending_keepalive_ms.* = now;
            return .keepalive_sent;
        }
    }

    return .ok;
}

/// Mark that a keepalive should be sent.
pub fn scheduleKeepalive(
    keepalive_interval_ms: u32,
    pending_keepalive: *bool,
    pending_keepalive_ms: *u64,
    clock: Clock,
) void {
    if (keepalive_interval_ms == 0) return;
    pending_keepalive.* = true;
    pending_keepalive_ms.* = clock.getMonoTimeMs();
}

/// Transition session to established state.
/// Calculates negotiated hold time and keepalive interval.
pub fn transitionToEstablished(
    local_hold: u16,
    peer_hold: u16,
    config_keepalive: u16,
    negotiated_hold_time: *u16,
    keepalive_interval_ms: *u32,
    pending_keepalive: *bool,
    pending_keepalive_ms: *u64,
    hold_timer_deadline: *u64,
    clock: Clock,
) void {
    negotiated_hold_time.* = @min(local_hold, peer_hold);
    keepalive_interval_ms.* = calcKeepaliveInterval(config_keepalive, negotiated_hold_time.*);
    resetHoldTimer(negotiated_hold_time.*, clock, hold_timer_deadline);
    scheduleKeepalive(keepalive_interval_ms.*, pending_keepalive, pending_keepalive_ms, clock);
}

/// Handle NOTIFICATION message and set error details.
/// The notif_buf must be caller-owned (e.g., session-owned) storage.
pub fn handleNotificationError(
    frame: frame_decode.Frame,
    status: *session_status.SessionStatus,
    notif_buf: *[64]u8,
) error{PeerNotification}!void {
    const notif = frame_decode.parseNotificationBody(frame.body) catch {
        status.state = .failed;
        status.last_error = SessionError{
            .message = "peer NOTIFICATION (malformed)",
            .notification_code = null,
            .notification_subcode = null,
        };
        return error.PeerNotification;
    };
    status.last_notification_code = notif.error_code;
    status.last_notification_subcode = notif.error_subcode;
    status.state = .failed;

    const notif_detail = notification_decode.formatNotification(
        notif.error_code,
        notif.error_subcode,
        notif_buf,
    );
    status.last_error = SessionError{
        .message = notif_detail,
        .notification_code = notif.error_code,
        .notification_subcode = notif.error_subcode,
    };
}
