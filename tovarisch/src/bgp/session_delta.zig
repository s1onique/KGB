// bgp/session_delta.zig — BGP session delta application
//
// ACT: Apply BGP export deltas after watched prefix reload (Phase 2)
//
// Provides delta application capability for BGP sessions.
// This module is separate from session.zig to satisfy LLM-friendliness limits.

const std = @import("std");
const types = @import("types.zig");
const message = @import("message.zig");
const session = @import("session.zig");

/// Result of applying a delta to a session.
pub const DeltaApplyResult = struct {
    /// Number of withdrawal UPDATE messages sent.
    withdrawals_sent: usize,
    /// Number of announcement UPDATE messages sent.
    announcements_sent: usize,
    /// Total prefixes in withdrawal messages.
    withdrawn_prefixes: usize,
    /// Total prefixes in announcement messages.
    announced_prefixes: usize,
};

/// Apply a prefix delta to an established BGP session.
///
/// Sends:
///   - BGP UPDATE withdrawals for removed prefixes
///   - BGP UPDATE announcements for added prefixes
///
/// Only applies to sessions in .established state.
/// Skips non-established sessions without error.
pub fn applyDelta(
    sess: *session.Session,
    removed: []const types.Ipv4Prefix,
    added: []const types.Ipv4Prefix,
) session.SessionErrorKind!DeltaApplyResult {
    var result = DeltaApplyResult{
        .withdrawals_sent = 0,
        .announcements_sent = 0,
        .withdrawn_prefixes = 0,
        .announced_prefixes = 0,
    };

    if (sess.status.state != .established) {
        return result;
    }

    if (removed.len > 0) {
        var offset: usize = 0;
        while (offset < removed.len) {
            const batch_end = @min(offset + session.MAX_PREFIXES_PER_UPDATE, removed.len);
            const batch = removed[offset..batch_end];
            const len = message.encodeWithdraw(batch, &sess.send_buf);
            if (len == 0) return session.SessionErrorKind.IoError;
            sess.send_pos = len;
            session.flushSend(sess) catch return session.SessionErrorKind.IoError;
            result.withdrawals_sent += 1;
            result.withdrawn_prefixes += batch.len;
            offset = batch_end;
        }
    }

    if (added.len > 0) {
        const next_hop = sess.config.local_address orelse sess.config.router_id;
        var offset: usize = 0;
        while (offset < added.len) {
            const batch_end = @min(offset + session.MAX_PREFIXES_PER_UPDATE, added.len);
            const batch = added[offset..batch_end];
            const len = message.encodeUpdate(types.UpdateParams{
                .next_hop = next_hop,
                .local_as = sess.config.local_as,
                .same_as = sess.config.same_as,
                .prefixes = batch,
            }, &sess.send_buf);
            if (len == 0) return session.SessionErrorKind.IoError;
            sess.send_pos = len;
            session.flushSend(sess) catch return session.SessionErrorKind.IoError;
            result.announcements_sent += 1;
            result.announced_prefixes += batch.len;
            offset = batch_end;
        }
    }

    return result;
}
