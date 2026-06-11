// bgp/status.zig — BGP status reporting for tovarisch
//
// Provides BGP status snapshot derivation for integration with the main status system.
// The BGP status module is stateless: it transforms an optional bundle pointer
// into a contract-valid check. Daemon owns the runtime, not the status module.
//
// KEY CONSTRAINT: This module does NOT perform network I/O.
// Status rendering is purely derived from the bundle's immutable config state.

const std = @import("std");
const serve_integration = @import("serve_integration.zig");
const passive_listener = @import("passive_listener.zig");

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
        advertised_prefix_count: usize,
        fsm_state: []const u8,
        peer_address: [4]u8,
        peer_as: u16,
        local_as: u16,
        last_error: ?[]const u8,
        messages_sent: u64,
        messages_received: u64,
        keepalives_sent: u64,
        keepalives_received: u64,
        passive_listener_state: passive_listener.ListenerState,
        passive_listener_error: ?[]const u8,
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

            if (std.mem.eql(u8, cfg.fsm_state, "established")) {
                if (cfg.advertised_prefix_count > 0) {
                    const prefix_label = if (cfg.advertised_prefix_count == 1) "prefix" else "prefixes";
                    const detail = std.fmt.bufPrint(detail_buf, "BGP established; {d} advertised {s}", .{
                        cfg.advertised_prefix_count, prefix_label,
                    }) catch {
                        return .{ .name = "bgp", .status = .ok, .detail = "BGP established" };
                    };
                    return .{ .name = "bgp", .status = .ok, .detail = detail };
                }
                return .{ .name = "bgp", .status = .ok, .detail = "BGP established" };
            }

            if (cfg.advertised_prefix_count == 0) {
                return .{ .name = "bgp", .status = .warn, .detail = "BGP configured with no advertised prefixes" };
            }

            const prefix_label = if (cfg.advertised_prefix_count == 1) "prefix" else "prefixes";
            const detail = std.fmt.bufPrint(detail_buf, "BGP configured; {d} advertised {s}", .{
                cfg.advertised_prefix_count, prefix_label,
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
            }};
        },
        .configured => {
            const sess_status = serve_integration.getSessionStatus(b);

            if (sess_status.state == .failed) {
                const err_msg = if (sess_status.last_error) |e| e.message else "session failed";
                return .{ .runtime_failed = .{ .message = err_msg } };
            }

            const fsm_state = @tagName(sess_status.state);
            var listener_state = passive_listener.ListenerState.disabled;
            var listener_error: ?[]const u8 = null;

            if (b.passive_listener) |*listener| {
                listener_state = listener.state;
                listener_error = listener.error_message;
            }

            return .{ .configured = .{
                .advertised_prefix_count = b.prefixes.len,
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
            }};
        },
        .failed => {
            const err_msg = if (b.last_error) |e| e else "configuration failed";
            return .{ .failed = .{ .message = err_msg } };
        },
    }
}
