// bgp/status.zig — BGP status reporting for tovarisch
//
// Provides BGP status snapshot derivation for integration with the main status system.
// The BGP status module is stateless: it transforms an optional bundle pointer
// into a contract-valid check. Daemon owns the runtime, not the status module.
//
// KEY CONSTRAINT: This module does NOT perform network I/O.
// Status rendering is purely derived from the bundle's immutable config state.
//
// Budget Contract:
// - BgpSnapshotBudget constrains the size of BGP diagnostic data
// - BgpPeerState is a closed enum ensuring exhaustive FSM state handling

const std = @import("std");
const serve_integration = @import("serve_integration.zig");
const passive_listener = @import("passive_listener.zig");
const session_status = @import("session_status.zig");
const snapshot = @import("snapshot.zig");

/// BGP check status enum (mirrors status.CheckStatus).
pub const CheckStatus = enum {
    ok,
    warn,
    @"error",
    unknown,
};

/// BGP status check result (mirrors status.Check).
pub const BgpCheck = struct {
    name: []const u8,
    status: CheckStatus,
    detail: []const u8,
};

/// Reconnect wait info for status reporting.
pub const ReconnectWait = struct {
    backoff_ms: u64,
    peer_address: [4]u8,
    last_error: ?[]const u8,
    reconnect_count: u64,
    last_socket_error: ?[]const u8,
};

/// BGP status state for status reporting.
pub const BgpStatusState = union(enum) {
    no_config,
    not_configured,
    disabled,
    configured: Configured,
    failed: Failure,
    runtime_failed: Failure,
    reconnect_wait: ReconnectWait,

    pub const Configured = struct {
        /// Number of prefixes configured for advertisement
        configured_prefix_count: usize,
        /// Number of UPDATE messages sent (actual exports)
        updates_sent: u64,
        /// Total prefixes encoded into UPDATE NLRI across this session/export run
        nlri_sent_count: usize,
        /// FSM state as closed enum (not stringly typed).
        /// This ensures exhaustive handling of all BGP peer states.
        fsm_state: snapshot.BgpPeerState,
        peer_address: [4]u8,
        /// Peer's AS number (supports 4-byte ASNs per RFC 6793).
        /// Widened from u16 to u32 per HULK17R.
        peer_as: u32,
        /// Our local AS number (supports 4-byte ASNs per RFC 6793).
        /// Widened from u16 to u32 per HULK17R.
        local_as: u32,
        last_error: ?[]const u8,
        messages_sent: u64,
        messages_received: u64,
        keepalives_sent: u64,
        keepalives_received: u64,
        passive_listener_state: passive_listener.ListenerState,
        passive_listener_error: ?[]const u8,

        // Reconnect diagnostics persisted after recovery
        // These survive into established state so labs can verify reconnect_count.
        /// Total reconnect attempts since startup.
        /// Persists into established state for lab verification.
        reconnect_count: u64 = 0,
        /// Last TCP socket error message.
        /// Persists into established state for diagnostics.
        last_socket_error: ?[]const u8 = null,

        // Export reload diagnostics
        /// Currently exported prefix count (daemon-owned current set)
        exported_prefix_count: usize = 0,
        /// Last reload success flag
        last_reload_success: bool = false,
        /// Last reload error message
        last_reload_error: ?[]const u8 = null,
        /// Last delta added count
        last_delta_added_count: usize = 0,
        /// Last delta removed count
        last_delta_removed_count: usize = 0,
        /// Last delta unchanged count
        last_delta_unchanged_count: usize = 0,
        /// Last apply error message
        last_apply_error: ?[]const u8 = null,
    };

    pub const Failure = struct {
        message: []const u8,
    };
};

const BGP_DETAIL_BUF_SIZE: usize = 64;

pub fn buildBgpCheckInto(
    state: BgpStatusState,
    detail_buf: *[BGP_DETAIL_BUF_SIZE]u8,
) BgpCheck {
    switch (state) {
        .no_config => return .{
            .name = "bgp",
            .status = .warn,
            .detail = "BGP not configured",
        },
        .not_configured => return .{
            .name = "bgp",
            .status = .warn,
            .detail = "BGP not configured",
        },
        .disabled => return .{
            .name = "bgp",
            .status = .ok,
            .detail = "BGP disabled by config",
        },
        .configured => |cfg| {
            if (cfg.passive_listener_state == .thread_failed or cfg.passive_listener_state == .bind_failed) {
                const detail = std.fmt.bufPrint(detail_buf, "{s}", .{"passive listener failed"}) catch {
                    return .{ .name = "bgp", .status = .warn, .detail = "BGP active (passive listener failed)" };
                };
                return .{ .name = "bgp", .status = .warn, .detail = detail };
            }

            if (cfg.fsm_state == .established) {
                // Established FSM always outranks zero-prefix warning.
                // Live BGP connectivity is more important than prefix advertisement.
                if (cfg.configured_prefix_count > 0) {
                    const prefix_label = if (cfg.configured_prefix_count == 1) "prefix" else "prefixes";
                    const detail = std.fmt.bufPrint(detail_buf, "BGP established; {d} configured {s}", .{
                        cfg.configured_prefix_count, prefix_label,
                    }) catch {
                        return .{ .name = "bgp", .status = .ok, .detail = "BGP established" };
                    };
                    return .{ .name = "bgp", .status = .ok, .detail = detail };
                }
                return .{ .name = "bgp", .status = .ok, .detail = "BGP established" };
            } else if (cfg.configured_prefix_count == 0) {
                // Zero-prefix warning only applies when BGP is NOT established.
                return .{ .name = "bgp", .status = .warn, .detail = "BGP configured with no configured prefixes" };
            }

            const prefix_label = if (cfg.configured_prefix_count == 1) "prefix" else "prefixes";
            const detail = std.fmt.bufPrint(detail_buf, "BGP configured; {d} configured {s}", .{
                cfg.configured_prefix_count, prefix_label,
            }) catch {
                return .{ .name = "bgp", .status = .ok, .detail = "BGP configured" };
            };
            return .{ .name = "bgp", .status = .ok, .detail = detail };
        },
        .failed => |fail| return .{ .name = "bgp", .status = .@"error", .detail = fail.message },
        .runtime_failed => |fail| return .{ .name = "bgp", .status = .@"error", .detail = fail.message },
        .reconnect_wait => |rw| {
            const detail = std.fmt.bufPrint(detail_buf, "BGP reconnecting in {d}ms", .{rw.backoff_ms}) catch {
                return .{ .name = "bgp", .status = .warn, .detail = "BGP reconnecting" };
            };
            return .{ .name = "bgp", .status = .warn, .detail = detail };
        },
    }
}

// Extract BGP status state from BgpLoadResult union.
// This preserves load result information (failed, not_configured, disabled)
// that would be lost if we only carry ?*BgpServeBundle.
pub fn statusStateFromLoadResult(result: serve_integration.BgpLoadResult) BgpStatusState {
    switch (result) {
        .no_config => return .no_config,
        .not_configured => return .not_configured,
        .disabled => return .disabled,
        .configured => |bundle| {
            // Delegate to bundle-based derivation for runtime state
            return deriveStatusStateFromBundle(bundle);
        },
        .failed => |load_err| return .{ .failed = .{ .message = load_err.message } },
    }
}

/// Map from internal SessionState to closed BgpPeerState for status reporting.
/// This ensures all external-facing BGP state uses the bounded snapshot contract.
pub fn mapSessionStateToBgpPeerState(sess_state: session_status.SessionState) snapshot.BgpPeerState {
    return switch (sess_state) {
        .idle => .idle,
        .connect => .connect,
        // SessionState has no 'active' state - BGP FSM differs from snapshot enum.
        .open_sent => .open_sent,
        .open_confirm => .open_confirm,
        .established => .established,
        // Internal states that shouldn't reach status (failed/stopped)
        // are mapped to unknown for safety.
        .failed, .stopped => .unknown,
    };
}

pub fn deriveStatusStateFromBundle(bundle: ?*serve_integration.BgpServeBundle) BgpStatusState {
    if (bundle == null) return .no_config;

    const b = bundle.?;
    switch (b.state) {
        .not_configured => return .not_configured,
        .disabled => return .disabled,
        .reconnect_wait => {
            const sess_status = serve_integration.getSessionStatus(b);
            return .{ .reconnect_wait = .{
                .backoff_ms = b.backoff_ms,
                .peer_address = sess_status.peer_address,
                .last_error = b.last_error,
                .reconnect_count = b.reconnect_count,
                .last_socket_error = b.last_socket_error,
            }};
        },
        .configured => {
            const sess_status = serve_integration.getSessionStatus(b);

            if (sess_status.state == .failed) {
                const err_msg = if (sess_status.last_error) |e| e.message else "session failed";
                return .{ .runtime_failed = .{ .message = err_msg } };
            }

            // Map to closed enum - ensures exhaustive handling of all FSM states.
            const fsm_state = mapSessionStateToBgpPeerState(sess_status.state);
            var listener_state = passive_listener.ListenerState.disabled;
            var listener_error: ?[]const u8 = null;

            if (b.passive_listener) |*listener| {
                listener_state = listener.state;
                listener_error = listener.error_message;
            }

            return .{ .configured = .{
                .configured_prefix_count = b.prefixes.len,
                .updates_sent = sess_status.updates_sent,
                .nlri_sent_count = sess_status.nlri_sent_count,
                .fsm_state = fsm_state,
                .peer_address = sess_status.peer_address,
                .peer_as = sess_status.peer_as,
                .local_as = sess_status.local_as,
                .last_error = if (sess_status.last_error) |e| e.message else null,
                .messages_sent = sess_status.messages_sent,
                .messages_received = sess_status.messages_received,
                .keepalives_sent = sess_status.keepalives_sent,
                .keepalives_received = sess_status.keepalives_received,
                .passive_listener_state = listener_state,
                .passive_listener_error = listener_error,
                // Persist reconnect diagnostics into established state for lab verification.
                // This ensures reconnect_count survives recovery to configured/established.
                .reconnect_count = b.reconnect_count,
                .last_socket_error = b.last_socket_error,
                // Export reload diagnostics from daemon-owned export state
                .exported_prefix_count = b.export_state.exportedCount(),
                .last_reload_success = b.export_state.last_reload_success,
                .last_reload_error = b.export_state.last_reload_error,
                .last_delta_added_count = b.export_state.last_delta_added_count,
                .last_delta_removed_count = b.export_state.last_delta_removed_count,
                .last_delta_unchanged_count = b.export_state.last_delta_unchanged_count,
                .last_apply_error = b.export_state.last_apply_error,
            }};
        },
        .failed => {
            const err_msg = if (b.last_error) |e| e else "configuration failed";
            return .{ .failed = .{ .message = err_msg } };
        },
    }
}
