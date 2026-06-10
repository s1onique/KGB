// session.zig — BGP TCP session state machine
//
// ACT 2: Minimal BGP session state machine for tovarisch.
// This module provides session logic but is NOT wired into the production
// daemon serve lifecycle yet.
//
// Session States (RFC 4271 Section 8.2):
//   Idle -> Connect -> OpenSent -> OpenConfirm -> Established
//   Established -> (any error) -> Failed/Stopped
//
// References: RFC 4271 Sections 4.1, 4.2, 8

const std = @import("std");
const types = @import("types.zig");
const message = @import("message.zig");
const frame_decode = @import("frame_decode.zig");
const session_status = @import("session_status.zig");
const transport = @import("transport.zig");

/// Re-export session types for convenience
pub const SessionState = session_status.SessionState;
pub const SessionError = session_status.SessionError;
pub const SessionStatus = session_status.SessionStatus;

// Re-export transport types
pub const Transport = transport.Transport;
pub const PeerResponse = transport.PeerResponse;
pub const FakeTransport = transport.FakeTransport;
pub const transportSend = transport.transportSend;
pub const transportRecv = transport.transportRecv;
pub const transportClose = transport.transportClose;

// ============================================================================
// Session Configuration and Errors
// ============================================================================

/// Session configuration for a single BGP peer.
pub const SessionConfig = struct {
    /// Peer's IPv4 address
    peer_address: [4]u8,
    /// Peer's BGP port (must be nonzero)
    peer_port: u16,
    /// Our local IPv4 address (null = let OS pick)
    local_address: ?[4]u8,
    /// Our local ASN (1..65535)
    local_as: u16,
    /// Peer's ASN (1..65535)
    peer_as: u16,
    /// Our router ID (IPv4-like ID)
    router_id: [4]u8,
    /// Hold time in seconds (0 or >= 3)
    hold_time_seconds: u16,
    /// Keepalive interval in seconds (< hold_time when hold time != 0)
    keepalive_seconds: u16,
    /// TCP connection timeout in milliseconds
    connect_timeout_ms: u32,
    /// Prefixes to advertise (must be non-empty)
    prefixes: []const types.Ipv4Prefix,
    /// If true, AS_PATH is empty (same-AS/iBGP style)
    same_as: bool,
};

/// Session errors
pub const SessionErrorKind = error{
    /// Config validation failed
    InvalidConfig,
    /// TCP connection failed
    ConnectionFailed,
    /// TCP connection closed by peer
    ConnectionClosed,
    /// Invalid/malformed frame received
    InvalidFrame,
    /// Peer sent NOTIFICATION
    PeerNotification,
    /// Session is not in a valid state for this operation
    InvalidState,
    /// I/O error during send/receive
    IoError,
    /// Frame decode error
    DecodeError,
};

/// Result of a session operation (used by runOnce)
pub const RunResult = enum {
    /// Operation completed successfully, session still running
    ok,
    /// Session transitioned to Established state
    established,
    /// Session stopped cleanly
    stopped,
    /// Session failed (see status.last_error for details)
    failed,
};

// ============================================================================
// Session
// ============================================================================

/// BGP session state.
/// This struct manages session state, configuration, and transport.
pub const Session = struct {
    /// Session configuration (borrowed, not owned)
    config: SessionConfig,
    /// Current session status (state, counters, errors)
    status: SessionStatus,
    /// Transport interface (borrowed, not owned)
    trans: *const Transport,
    /// Peer OPEN body (filled when OPEN received)
    peer_open: ?frame_decode.OpenBody,
    /// Send buffer for building messages
    send_buf: [4096]u8,
    /// Send buffer fill position
    send_pos: usize,
    /// Receive buffer
    recv_buf: [4096]u8,
    /// Bytes in receive buffer
    recv_len: usize,
};

/// Validate session configuration.
pub fn validateConfig(config: SessionConfig) SessionErrorKind!void {
    if (config.peer_port == 0) return SessionErrorKind.InvalidConfig;
    if (config.local_as < 1 or config.local_as > 65535) return SessionErrorKind.InvalidConfig;
    if (config.peer_as < 1 or config.peer_as > 65535) return SessionErrorKind.InvalidConfig;
    if (config.hold_time_seconds != 0 and config.hold_time_seconds < 3) return SessionErrorKind.InvalidConfig;
    if (config.hold_time_seconds != 0 and config.keepalive_seconds >= config.hold_time_seconds) return SessionErrorKind.InvalidConfig;
    if (config.prefixes.len == 0) return SessionErrorKind.InvalidConfig;
}

/// Create a new BGP session.
pub fn init(config: SessionConfig, trans: *const Transport) SessionErrorKind!Session {
    try validateConfig(config);
    return Session{
        .config = config,
        .status = session_status.initStatus(config.peer_address, config.local_as, config.peer_as, config.router_id, config.prefixes.len),
        .trans = trans,
        .peer_open = null,
        .send_buf = undefined,
        .send_pos = 0,
        .recv_buf = undefined,
        .recv_len = 0,
    };
}

/// Get current session status.
pub fn getStatus(sess: *Session) SessionStatus {
    return sess.status;
}

/// Check if session is in a terminal state.
pub fn isTerminal(sess: *Session) bool {
    return sess.status.state == .failed or sess.status.state == .stopped;
}

/// Check if session is established.
pub fn isEstablished(sess: *Session) bool {
    return sess.status.state == .established;
}

/// Stop the session cleanly.
pub fn stop(sess: *Session) void {
    transportClose(sess.trans);
    sess.status.state = .stopped;
}

/// Flush any buffered send data.
fn flushSend(sess: *Session) void {
    if (sess.send_pos > 0) {
        transportSend(sess.trans, sess.send_buf[0..sess.send_pos]);
        sess.send_pos = 0;
    }
}

/// Receive data into buffer.
fn recvIntoBuffer(sess: *Session) void {
    const data = transportRecv(sess.trans);
    if (data.len > 0 and sess.recv_len < sess.recv_buf.len) {
        const copy_len = @min(data.len, sess.recv_buf.len - sess.recv_len);
        @memcpy(sess.recv_buf[sess.recv_len..sess.recv_len + copy_len], data[0..copy_len]);
        sess.recv_len += copy_len;
    }
}

/// Try to decode one frame from receive buffer.
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
        @memcpy(sess.recv_buf[0..sess.recv_len - declared_len], sess.recv_buf[declared_len..sess.recv_len]);
    }
    sess.recv_len -= declared_len;
    return frame;
}

/// Handle a decoded message.
fn handleMessage(sess: *Session, frame: frame_decode.Frame) SessionErrorKind!RunResult {
    sess.status.messages_received += 1;
    switch (sess.status.state) {
        .open_sent => {
            if (frame_decode.isOpen(frame)) {
                const open_body = frame_decode.parseOpenBody(frame.body) catch {
                    sess.status.state = .failed;
                    sess.status.last_error = SessionError{ .message = "malformed OPEN", .notification_code = null, .notification_subcode = null };
                    return SessionErrorKind.DecodeError;
                };
                if (open_body.peer_as != sess.config.peer_as) {
                    sess.status.state = .failed;
                    sess.status.last_error = SessionError{ .message = "peer AS mismatch", .notification_code = null, .notification_subcode = null };
                    return SessionErrorKind.InvalidFrame;
                }
                sess.peer_open = open_body;
                sess.status.state = .open_confirm;
                sess.send_pos = message.encodeKeepalive(&sess.send_buf);
                sess.status.keepalives_sent += 1;
                sess.status.messages_sent += 1;
                return .ok;
            } else if (frame_decode.isNotification(frame)) {
                const notif = frame_decode.parseNotificationBody(frame.body) catch {
                    sess.status.state = .failed;
                    return SessionErrorKind.PeerNotification;
                };
                sess.status.last_notification_code = notif.error_code;
                sess.status.last_notification_subcode = notif.error_subcode;
                sess.status.state = .failed;
                sess.status.last_error = SessionError{ .message = "peer NOTIFICATION", .notification_code = notif.error_code, .notification_subcode = notif.error_subcode };
                return RunResult.failed;
            }
            return .ok;
        },
        .open_confirm => {
            if (frame_decode.isKeepalive(frame)) {
                sess.status.keepalives_received += 1;
                sess.status.state = .established;
                const next_hop = sess.config.local_address orelse sess.config.router_id;
                sess.send_pos = message.encodeUpdate(types.UpdateParams{
                    .next_hop = next_hop,
                    .local_as = sess.config.local_as,
                    .same_as = sess.config.same_as,
                    .prefixes = sess.config.prefixes,
                }, &sess.send_buf);
                if (sess.send_pos > 0) {
                    sess.status.updates_sent += 1;
                    sess.status.messages_sent += 1;
                }
                return .established;
            } else if (frame_decode.isNotification(frame)) {
                const notif = frame_decode.parseNotificationBody(frame.body) catch {
                    sess.status.state = .failed;
                    return SessionErrorKind.PeerNotification;
                };
                sess.status.last_notification_code = notif.error_code;
                sess.status.last_notification_subcode = notif.error_subcode;
                sess.status.state = .failed;
                sess.status.last_error = SessionError{ .message = "peer NOTIFICATION", .notification_code = notif.error_code, .notification_subcode = notif.error_subcode };
                return RunResult.failed;
            }
            return .ok;
        },
        .established => {
            if (frame_decode.isKeepalive(frame)) {
                sess.status.keepalives_received += 1;
                return .ok;
            } else if (frame_decode.isUpdate(frame)) {
                return .ok; // Import-nothing
            } else if (frame_decode.isNotification(frame)) {
                const notif = frame_decode.parseNotificationBody(frame.body) catch {
                    sess.status.state = .failed;
                    return SessionErrorKind.PeerNotification;
                };
                sess.status.last_notification_code = notif.error_code;
                sess.status.last_notification_subcode = notif.error_subcode;
                sess.status.state = .failed;
                sess.status.last_error = SessionError{ .message = "peer NOTIFICATION", .notification_code = notif.error_code, .notification_subcode = notif.error_subcode };
                return RunResult.failed;
            }
            return .ok;
        },
        else => return SessionErrorKind.InvalidState,
    }
}

/// Run one iteration of the session state machine.
pub fn runOnce(sess: *Session) SessionErrorKind!RunResult {
    switch (sess.status.state) {
        .idle => {
            sess.status.state = .open_sent;
            sess.send_pos = message.encodeOpen(types.OpenParams{
                .my_as = sess.config.local_as,
                .hold_time = sess.config.hold_time_seconds,
                .router_id = sess.config.router_id,
            }, &sess.send_buf);
            sess.status.messages_sent += 1;
            flushSend(sess);
            return .ok;
        },
        .open_sent, .open_confirm, .established => {
            flushSend(sess);
            recvIntoBuffer(sess);
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
