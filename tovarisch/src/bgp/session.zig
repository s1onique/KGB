// session.zig — BGP TCP session state machine
//
// ACT 2: Minimal BGP session state machine for tovarisch.
// Session timers extracted to session_timers.zig for LLM-friendliness.

const std = @import("std");
const types = @import("types.zig");
const message = @import("message.zig");
const frame_decode = @import("frame_decode.zig");
const session_status = @import("session_status.zig");
const transport = @import("transport.zig");
const clock = @import("clock.zig");
const notification_decode = @import("notification_decode.zig");
const timers = @import("session_timers.zig");
const diagnostics = @import("session_diagnostics.zig");
const session_recv = @import("session_recv.zig");

// Re-export types for convenience
pub const SessionState = timers.SessionState;
pub const SessionError = timers.SessionError;
pub const SessionStatus = timers.SessionStatus;
pub const Clock = timers.Clock;
pub const MonoTime = timers.MonoTime;
pub const RealClock = timers.RealClock;
pub const MockClock = timers.MockClock;
pub const EstablishedStepResult = timers.EstablishedStepResult;

// Re-export transport types
pub const Transport = transport.Transport;
pub const PeerResponse = transport.PeerResponse;
pub const FakeTransport = transport.FakeTransport;
pub const transportSend = transport.transportSend;
pub const transportRecv = transport.transportRecv;
pub const transportClose = transport.transportClose;
pub const TransportError = transport.TransportError;

// Re-export notification decode
pub const formatNotification = timers.formatNotification;
pub const getErrorCodeName = timers.getErrorCodeName;
pub const getErrorSubcodeName = timers.getErrorSubcodeName;

// Re-export diagnostics types
pub const UpdateInfo = diagnostics.UpdateInfo;
pub const UpdateDiagnostic = diagnostics.UpdateDiagnostic;

// Error Message Helpers
/// Map a transport error to a human-readable error message.
fn transportErrorMessage(err: TransportError) []const u8 {
    return switch (err) {
        TransportError.Closed => "transport closed",
        TransportError.ConnectionClosed => "connection closed",
        TransportError.OutOfMemory => "out of memory",
        TransportError.WouldBlock => "send: EAGAIN/EWOULDBLOCK",
        TransportError.ConnectionReset => "send: ECONNRESET",
        TransportError.BrokenPipe => "send: EPIPE",
        TransportError.NotConnected => "send: ENOTCONN",
        TransportError.BadFileDescriptor => "send: EBADF",
        TransportError.SendFailed => "send failed",
    };
}
// Session Configuration
pub const SessionConfig = struct {
    peer_address: [4]u8,
    peer_port: u16,
    local_address: ?[4]u8,
    local_as: u16,
    peer_as: u16,
    router_id: [4]u8,
    hold_time_seconds: u16,
    keepalive_seconds: u16,
    connect_timeout_ms: u32,
    prefixes: []const types.Ipv4Prefix,
    same_as: bool,
};

pub const SessionErrorKind = error{
    InvalidConfig,
    ConnectionFailed,
    ConnectionClosed,
    InvalidFrame,
    PeerNotification,
    InvalidState,
    IoError,
    DecodeError,
    HoldTimerExpired,
};
// UPDATE Batching Constants
/// Maximum BGP message size (RFC 4271 Section 4.1)
pub const MAX_BGP_MESSAGE_SIZE: usize = 4096;

/// Maximum NLRI bytes available per UPDATE after header and path attributes.
/// Header: 19 (marker+len+type), Withdrawn: 2, Path attrs: ~18 (ORIGIN+AS_PATH+NEXT_HOP)
const MAX_UPDATE_BODY_SIZE: usize = MAX_BGP_MESSAGE_SIZE - 19 - 2 - 18;

/// Maximum bytes per NLRI prefix entry (length byte + up to 4 bytes for IPv4)
const NLRI_PREFIX_MAX_BYTES: usize = 5;

/// Conservative max prefixes per UPDATE to stay within message size limits.
/// This allows for /8 prefixes (2 bytes) up to /32 prefixes (5 bytes).
pub const MAX_PREFIXES_PER_UPDATE: usize = MAX_UPDATE_BODY_SIZE / NLRI_PREFIX_MAX_BYTES;

pub const RunResult = enum {
    ok,
    established,
    stopped,
    failed,
};
// Session struct
pub const Session = struct {
    config: SessionConfig,
    status: SessionStatus,
    trans: *const Transport,
    clock: Clock,
    peer_open: ?frame_decode.OpenBody,
    send_buf: [4096]u8,
    send_pos: usize,
    recv_buf: [4096]u8,
    recv_len: usize,
    negotiated_hold_time: u16,
    keepalive_interval_ms: u32,
    hold_timer_deadline: u64,
    pending_keepalive: bool,
    pending_keepalive_ms: u64, // Session-owned timestamp for keepalive scheduling
    notification_detail_buf: [64]u8, // Session-owned storage for NOTIFICATION error detail
    export_batch_index: usize = 0, // Current batch start index in prefixes array
    export_complete: bool = false, // All prefixes have been exported
    nlri_sent_count: usize = 0, // Total prefixes encoded across all batches
    last_update_diagnostic: UpdateDiagnostic = .none, // Captured before flush for structured logging
};

pub fn validateConfig(config: SessionConfig) SessionErrorKind!void {
    if (config.peer_port == 0) return SessionErrorKind.InvalidConfig;
    if (config.local_as < 1 or config.local_as > 65535) return SessionErrorKind.InvalidConfig;
    if (config.peer_as < 1 or config.peer_as > 65535) return SessionErrorKind.InvalidConfig;
    if (config.hold_time_seconds != 0 and config.hold_time_seconds < 3) return SessionErrorKind.InvalidConfig;
    if (config.hold_time_seconds != 0 and config.keepalive_seconds >= config.hold_time_seconds) return SessionErrorKind.InvalidConfig;
}

pub fn init(config: SessionConfig, trans: *const Transport) SessionErrorKind!Session {
    return initWithClock(config, trans, RealClock);
}

pub fn initWithClock(config: SessionConfig, trans: *const Transport, c: Clock) SessionErrorKind!Session {
    try validateConfig(config);
    return Session{
        .config = config,
        .status = session_status.initStatus(config.peer_address, config.local_as, config.peer_as, config.router_id, config.prefixes.len),
        .trans = trans,
        .clock = c,
        .peer_open = null,
        .send_buf = undefined,
        .send_pos = 0,
        .recv_buf = undefined,
        .recv_len = 0,
        .negotiated_hold_time = 0,
        .keepalive_interval_ms = 0,
        .hold_timer_deadline = 0,
        .pending_keepalive = false,
        .pending_keepalive_ms = 0,
        .notification_detail_buf = undefined,
        .nlri_sent_count = 0,
    };
}

pub fn getStatus(sess: *Session) SessionStatus {
    return sess.status;
}

pub fn isTerminal(sess: *Session) bool {
    return sess.status.state == .failed or sess.status.state == .stopped;
}

pub fn isEstablished(sess: *Session) bool {
    return sess.status.state == .established;
}

pub fn stop(sess: *Session) void {
    transportClose(sess.trans);
    sess.status.state = .stopped;
}

pub fn flushSend(sess: *Session) TransportError!void {
    if (sess.send_pos > 0) {
        try transportSend(sess.trans, sess.send_buf[0..sess.send_pos]);
        sess.send_pos = 0;
    }
}

// Re-export RecvResult from session_recv for backward compatibility
pub const RecvResult = session_recv.RecvResult;

/// Receive bytes into session buffer. Returns RecvResult with connection_closed flag.
fn recvIntoBuffer(sess: *Session) RecvResult {
    const data = transportRecv(sess.trans);
    if (data.len > 0 and sess.recv_len < sess.recv_buf.len) {
        const copy_len = @min(data.len, sess.recv_buf.len - sess.recv_len);
        @memcpy(sess.recv_buf[sess.recv_len .. sess.recv_len + copy_len], data[0..copy_len]);
        sess.recv_len += copy_len;
        return RecvResult{ .bytes_copied = copy_len, .connection_closed = false };
    }
    if (session_recv.transportIsClosed(sess.trans)) {
        return RecvResult{ .bytes_copied = 0, .connection_closed = true };
    }
    return RecvResult{ .bytes_copied = 0, .connection_closed = false };
}

fn tryDecodeFrame(sess: *Session) ?frame_decode.Frame {
    if (sess.recv_len < types.MIN_MESSAGE_LENGTH) return null;
    const declared_len = @as(u16, sess.recv_buf[16]) * 256 + @as(u16, sess.recv_buf[17]);
    if (declared_len < types.MIN_MESSAGE_LENGTH or declared_len > types.MAX_MESSAGE_LENGTH) {
        sess.status.state = .failed;
        sess.status.last_error = SessionError{ .message = "invalid frame length", .notification_code = null, .notification_subcode = null };
        return null;
    }
    if (sess.recv_len < declared_len) return null;
    const frame = frame_decode.decodeFrame(sess.recv_buf[0..declared_len]) catch {
        sess.status.state = .failed;
        sess.status.last_error = SessionError{ .message = "malformed frame", .notification_code = null, .notification_subcode = null };
        return null;
    };
    if (sess.recv_len > declared_len) {
        // Use copyForwards because source starts at declared_len which is >= 0 (destination start).
        // This handles the overlapping case where both slices point into sess.recv_buf.
        std.mem.copyForwards(u8, sess.recv_buf[0 .. sess.recv_len - declared_len], sess.recv_buf[declared_len..sess.recv_len]);
    }
    sess.recv_len -= declared_len;
    return frame;
}

fn resetHoldTimer(sess: *Session) void {
    timers.resetHoldTimer(sess.negotiated_hold_time, sess.clock, &sess.hold_timer_deadline);
}

fn scheduleKeepalive(sess: *Session) void {
    timers.scheduleKeepalive(sess.keepalive_interval_ms, &sess.pending_keepalive, &sess.pending_keepalive_ms, sess.clock);
}

fn transitionToEstablished(sess: *Session) void {
    const peer_hold = if (sess.peer_open) |open| open.hold_time else sess.config.hold_time_seconds;
    timers.transitionToEstablished(
        sess.config.hold_time_seconds,
        peer_hold,
        sess.config.keepalive_seconds,
        &sess.negotiated_hold_time,
        &sess.keepalive_interval_ms,
        &sess.pending_keepalive,
        &sess.pending_keepalive_ms,
        &sess.hold_timer_deadline,
        sess.clock,
    );
}

pub fn stepEstablished(sess: *Session) EstablishedStepResult {
    return timers.stepEstablishedTimers(
        sess.negotiated_hold_time,
        sess.keepalive_interval_ms,
        sess.pending_keepalive,
        sess.hold_timer_deadline,
        sess.clock,
        &sess.send_buf,
        &sess.send_pos,
        &sess.status,
        &sess.pending_keepalive_ms,
    );
}

fn handleMessage(sess: *Session, frame: frame_decode.Frame) SessionErrorKind!RunResult {
    sess.status.messages_received += 1;
    switch (sess.status.state) {
        .open_sent => {
            if (frame_decode.isOpen(frame)) {
                const open_body = frame_decode.parseOpenBody(frame.body) catch {
                    sess.status.state = .failed;
                    sess.status.last_error = SessionError{
                        .message = "malformed OPEN",
                        .notification_code = null,
                        .notification_subcode = null,
                    };
                    return SessionErrorKind.DecodeError;
                };
                if (open_body.peer_as != sess.config.peer_as) {
                    sess.status.state = .failed;
                    sess.status.last_error = SessionError{
                        .message = "peer AS mismatch",
                        .notification_code = null,
                        .notification_subcode = null,
                    };
                    return SessionErrorKind.InvalidFrame;
                }
                sess.peer_open = open_body;
                sess.status.state = .open_confirm;
                sess.send_pos = message.encodeKeepalive(&sess.send_buf);
                sess.status.keepalives_sent += 1;
                sess.status.messages_sent += 1;
                return .ok;
            } else if (frame_decode.isNotification(frame)) {
                timers.handleNotificationError(frame, &sess.status, &sess.notification_detail_buf) catch return RunResult.failed;
                return RunResult.failed;
            }
            return .ok;
        },
        .open_confirm => {
            if (frame_decode.isKeepalive(frame)) {
                sess.status.keepalives_received += 1;
                sess.status.state = .established;
                transitionToEstablished(sess);
                return .established;
            } else if (frame_decode.isNotification(frame)) {
                timers.handleNotificationError(frame, &sess.status, &sess.notification_detail_buf) catch return RunResult.failed;
                return RunResult.failed;
            }
            return .ok;
        },
        .established => {
            resetHoldTimer(sess);
            if (frame_decode.isKeepalive(frame)) {
                sess.status.keepalives_received += 1;
            } else if (frame_decode.isNotification(frame)) {
                timers.handleNotificationError(frame, &sess.status, &sess.notification_detail_buf) catch return RunResult.failed;
                return RunResult.failed;
            }
            return .ok;
        },
        else => return SessionErrorKind.InvalidState,
    }
}

pub fn runOnce(sess: *Session) SessionErrorKind!RunResult {
    switch (sess.status.state) {
        .idle => {
            sess.send_pos = message.encodeOpen(types.OpenParams{
                .my_as = sess.config.local_as,
                .hold_time = sess.config.hold_time_seconds,
                .router_id = sess.config.router_id,
            }, &sess.send_buf);
            flushSend(sess) catch |err| {
                sess.status.state = .failed;
                sess.status.last_error = SessionError{
                    .message = transportErrorMessage(err),
                    .notification_code = null,
                    .notification_subcode = null,
                };
                return SessionErrorKind.IoError;
            };
            sess.status.state = .open_sent;
            sess.status.messages_sent += 1;
            return .ok;
        },
        .open_sent, .open_confirm, .established => {
            var pending_update: ?struct { prefixes: usize, batch_end: usize } = null;
            sess.last_update_diagnostic = .none;
            if (sess.status.state == .established) {
                const step_result = stepEstablished(sess);
                switch (step_result) {
                    .hold_timer_expired => {
                        sess.status.state = .failed;
                        sess.status.last_error = SessionError{
                            .message = "local hold timer expired",
                            .notification_code = null,
                            .notification_subcode = null,
                        };
                        return RunResult.failed;
                    },
                    .keepalive_sent => {
                        flushSend(sess) catch |err| {
                            sess.status.state = .failed;
                            sess.status.last_error = SessionError{
                                .message = transportErrorMessage(err),
                                .notification_code = null,
                                .notification_subcode = null,
                            };
                            return SessionErrorKind.IoError;
                        };
                    },
                    .ok => {},
                }
                if (sess.config.prefixes.len > 0 and sess.send_pos == 0 and !sess.export_complete) {
                    const next_hop = sess.config.local_address orelse sess.config.router_id;
                    const batch_start = sess.export_batch_index;
                    const batch_end = @min(batch_start + MAX_PREFIXES_PER_UPDATE, sess.config.prefixes.len);
                    const batch = sess.config.prefixes[batch_start..batch_end];
                    sess.send_pos = message.encodeUpdate(types.UpdateParams{
                        .next_hop = next_hop,
                        .local_as = sess.config.local_as,
                        .same_as = sess.config.same_as,
                        .prefixes = batch,
                    }, &sess.send_buf);
                    if (sess.send_pos > 0) {
                        pending_update = .{ .prefixes = batch.len, .batch_end = batch_end };
                        sess.status.messages_sent += 1;
                        // Capture UpdateDiagnostic before flush for structured logging
                        diagnostics.captureUpdateDiagnostic(sess, &sess.send_buf, sess.send_pos, batch_end, sess.config.prefixes.len);
                    }
                }
            }
            flushSend(sess) catch |err| {
                sess.status.state = .failed;
                sess.status.last_error = SessionError{
                    .message = transportErrorMessage(err),
                    .notification_code = null,
                    .notification_subcode = null,
                };
                return SessionErrorKind.IoError;
            };
            if (pending_update) |update| {
                sess.status.updates_sent += 1;
                sess.nlri_sent_count += update.prefixes;
                sess.status.nlri_sent_count = sess.nlri_sent_count; // Sync to status
                sess.export_batch_index = update.batch_end;
                if (update.batch_end >= sess.config.prefixes.len) {
                    sess.export_complete = true;
                }
            }
            const recv_result = recvIntoBuffer(sess);
            if (recv_result.connection_closed) {
                sess.status.state = .failed;
                sess.status.last_error = SessionError{
                    .message = "TCP connection closed by peer",
                    .notification_code = null,
                    .notification_subcode = null,
                };
                return SessionErrorKind.ConnectionClosed;
            }
            while (tryDecodeFrame(sess)) |frame| {
                const msg_result = handleMessage(sess, frame) catch |err| {
                    if (err == SessionErrorKind.InvalidFrame and sess.status.state == .failed) {
                        return RunResult.failed;
                    }
                    return err;
                };
                if (msg_result == .established) return RunResult.established;
                if (msg_result == .failed) return RunResult.failed;
            }
            return .ok;
        },
        .failed, .stopped => {
            return if (sess.status.state == .stopped) RunResult.stopped else RunResult.failed;
        },
        .connect => {
            sess.status.state = .open_sent;
            return .ok;
        },
    }
}
