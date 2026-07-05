// bgp/runtime.zig — BGP FSM runtime worker for tovarisch
//
// ACT runtime: BGP FSM loop runs in a joined thread, driving runSessionOnce.
// The thread handles reconnect scheduling when connection/session failures occur.
//
// Key behaviors:
// - Logs each FSM transition (open_sent, open_confirm, established, notification, error)
// - Sleeps between iterations to avoid hot-spinning
// - Schedules reconnect with exponential backoff on failure
// - Resets backoff after successful establishment
// - Thread failures are non-fatal (logs error and exits)
// - Bundle lifetime is owned by the caller (main thread) - thread does NOT free bundle
//
// Reconnect policy:
// - Initial retry: 1s, exponential up to 60s max
// - Backoff resets after successful establishment
// - Cleanup during reconnect_wait stops reconnect loop
//
// Thread safety:
// - bgpRuntimeThread waits for isCleanupRequested() before each iteration
// - cleanupBgpBundle signals cleanup_requested via atomic, then joins thread
// - All bundle state mutations happen in the runtime thread only
// - cleanup_requested is atomic.Bool for cross-thread signaling
// - Thread handle is stored in bundle.runtime_thread for join on cleanup
//
// Passive connection policy:
// - Passive inbound connection does NOT preempt an already-established active BGP session.
// - Established session wins and duplicate inbound socket is closed.
//
// References: RFC 4271 (BGP-4)

const std = @import("std");
const c = std.c;
const session = @import("session.zig");
const logging = @import("../logging.zig");
const serve_integration = @import("serve_integration.zig");
const passive_listener_integration = @import("passive_listener_integration.zig");
const tcp_transport = @import("tcp_transport.zig");
const clock = @import("clock.zig");
const serve_export_integration = @import("serve_export_integration.zig");

/// BGP FSM loop interval in milliseconds.
/// This is the sleep between runSessionOnce calls.
const BGP_LOOP_INTERVAL_MS: u64 = 100;

/// Maximum reconnect delay in milliseconds (1 minute).
const RECONNECT_MAX_MS: u64 = serve_integration.DEFAULT_RECONNECT_MAX_MS;

/// Format peer address as string for logging.
fn formatPeerAddr(addr: [4]u8, buf: *[32]u8) []const u8 {
    // Buffer is 32 bytes; IPv4 address needs at most 15 + null = 16 bytes.
    // This invariant is guaranteed by the fixed-size buffer.
    std.debug.assert(buf.len >= 16);
    const result = std.fmt.bufPrint(buf, "{}.{}.{}.{}", .{
        addr[0],
        addr[1],
        addr[2],
        addr[3],
    }) catch unreachable;
    return result;
}

/// Write a BGP log record directly to stdout using c.write.
/// This avoids contention with the main thread's buffered writer.
/// Thread-safe enough for daemon-lifetime NDJSON output.
fn bgpLogToStdout(bytes: []const u8) void {
    _ = c.write(1, bytes.ptr, bytes.len);
}

/// BGP FSM runtime worker.
/// This function runs in a detached thread, driving the BGP session state machine.
/// The thread handles FSM transitions and reconnect scheduling.
/// Thread exits when isCleanupRequested() returns true.
pub fn bgpRuntimeThread(bundle: *serve_integration.BgpServeBundle) void {
    // Use real clock for production time tracking
    const clock_interface = clock.RealClock;

    // Log TCP connected event (TCP connection was already established at load time)
    {
        var log_buf = logging.BufferedWriter.init();
        var peer_addr_buf: [32]u8 = undefined;
        logging.emit(.bgp_connected, &log_buf, &.{
            .{ .name = "peer", .value = logging.FieldValue{ .string = formatPeerAddr(bundle.sess.config.peer_address, &peer_addr_buf) } },
        }) catch return;
        bgpLogToStdout(log_buf.slice());
    }

    // Main FSM loop - bounded and non-hot-spinning with reconnect support
    var previous_state: session.SessionState = .idle;
    var previous_keepalives_sent: u64 = 0;
    while (true) {
        // Check for cleanup request first - this is the safe cleanup coordination point
        if (serve_integration.isCleanupRequested(bundle)) {
            var log_buf = logging.BufferedWriter.init();
            logging.emit(.bgp_error, &log_buf, &.{
                .{ .name = "detail", .value = logging.FieldValue{ .string = "cleanup requested, exiting" } },
            }) catch break;
            bgpLogToStdout(log_buf.slice());
            return;
        }

        // Handle reconnect wait state
        if (bundle.state == .reconnect_wait) {
            if (serve_integration.isReconnectReady(bundle, clock_interface)) {
                // Deadline elapsed, attempt reconnect
                serve_integration.doReconnect(bundle) catch |reconnect_err| {
                    // Reconnect failed, schedule next attempt with backoff
                    var log_buf = logging.BufferedWriter.init();
                    logging.emit(.bgp_error, &log_buf, &.{
                        .{ .name = "error", .value = logging.FieldValue{ .string = @errorName(reconnect_err) } },
                        .{ .name = "detail", .value = logging.FieldValue{ .string = "reconnect failed, scheduling retry" } },
                    }) catch break;
                    bgpLogToStdout(log_buf.slice());

                    serve_integration.scheduleReconnect(bundle, clock_interface, RECONNECT_MAX_MS);
                    continue;
                };

                // Reconnect successful, log and continue FSM
                var log_buf = logging.BufferedWriter.init();
                logging.emit(.bgp_connected, &log_buf, &.{
                    .{ .name = "detail", .value = logging.FieldValue{ .string = "reconnected after failure" } },
                }) catch break;
                bgpLogToStdout(log_buf.slice());
            } else {
                // Still waiting for deadline, sleep and check again
                var ts: c.timespec = .{
                    .sec = @intCast(BGP_LOOP_INTERVAL_MS / 1000),
                    .nsec = @intCast((BGP_LOOP_INTERVAL_MS % 1000) * 1_000_000),
                };
                _ = c.nanosleep(&ts, null);
                continue;
            }
        }

        // Check for pending passive connection ONLY if session is not already established.
        // Passive inbound connection does NOT preempt an already-established active BGP session.
        // This ensures established sessions are never displaced by incoming connections.
        if (passive_listener_integration.hasPendingPassiveConnection(bundle)) {
            const current_session_state = bundle.sess.status.state;

            // Only process passive connection if we're not in established state
            if (current_session_state != .established) {
                const accept_result = passive_listener_integration.acceptPassiveConnection(bundle) catch {
                    // Failed to accept, continue with normal session
                    var log_buf = logging.BufferedWriter.init();
                    logging.emit(.bgp_error, &log_buf, &.{
                        .{ .name = "detail", .value = logging.FieldValue{ .string = "failed to accept passive connection" } },
                    }) catch break;
                    bgpLogToStdout(log_buf.slice());
                    continue;
                };

                // Log passive connection accepted
                {
                    var log_buf = logging.BufferedWriter.init();
                    var peer_addr_buf: [32]u8 = undefined;
                    logging.emit(.bgp_connected, &log_buf, &.{
                        .{ .name = "detail", .value = logging.FieldValue{ .string = "passive connection accepted" } },
                        .{ .name = "peer", .value = logging.FieldValue{ .string = formatPeerAddr(accept_result.peer_address, &peer_addr_buf) } },
                    }) catch break;
                    bgpLogToStdout(log_buf.slice());
                }

                // Close current transport and switch to passive
                bundle.tcp.close();
                bundle.tcp = tcp_transport.TcpTransport.fromPassiveSocket(
                    accept_result.socket_fd,
                    accept_result.peer_address,
                    accept_result.peer_port,
                );
                bundle.trans = bundle.tcp.toTransport();
                bundle.sess.trans = &bundle.trans;

                // Reset session state for fresh BGP handshake
                bundle.sess.status.state = .idle;
                bundle.sess.recv_len = 0;
                bundle.sess.send_pos = 0;
                bundle.sess.peer_open = null;
                bundle.sess.negotiated_hold_time = 0;
                bundle.sess.keepalive_interval_ms = 0;
                bundle.sess.hold_timer_deadline = 0;
                bundle.sess.pending_keepalive = false;
                bundle.sess.pending_keepalive_ms = 0;
                bundle.sess.status.last_error = null;
                bundle.sess.status.last_notification_code = null;
                bundle.sess.status.last_notification_subcode = null;

                // Update session config peer address
                bundle.session_config.peer_address = accept_result.peer_address;
                bundle.session_config.peer_port = accept_result.peer_port;

                // Reset to configured state (not reconnect_wait)
                bundle.state = .configured;
                bundle.last_error = null;
                previous_state = .idle;
            } else {
                // Session is established - close the incoming passive socket without switching.
                // Established active sessions take precedence over incoming connections.
                if (passive_listener_integration.acceptPassiveConnection(bundle)) |accept_result| {
                    if (accept_result.socket_fd >= 0) {
                        _ = std.c.close(accept_result.socket_fd);
                    }
                } else |_| {}
            }
        }

        // Run one FSM iteration
        const result = serve_integration.runSessionOnce(bundle);

        // Check for prefix file changes and apply delta if watcher is configured.
        // This handles the event -> debounce -> reload -> delta -> UPDATE path.
        const now_ms = clock_interface.getMonoTimeMs();
        _ = serve_export_integration.applyPrefixReloadIfWatched(bundle, now_ms);

        // Log FSM transitions when state changes
        const current_state = bundle.sess.status.state;
        if (current_state != previous_state) {
            var log_buf = logging.BufferedWriter.init();

            // Build detail with state info
            const detail = switch (current_state) {
                .open_sent => "OPEN sent",
                .open_confirm => "OPEN received, sent KEEPALIVE",
                .established => "session established",
                .failed => getConcreteErrorMessage(bundle),
                else => @tagName(current_state),
            };

            // Emit specific event (comptime-known)
            switch (current_state) {
                .open_sent => {
                    logging.emit(.bgp_open_sent, &log_buf, &.{
                        .{ .name = "state", .value = logging.FieldValue{ .string = @tagName(current_state) } },
                        .{ .name = "detail", .value = logging.FieldValue{ .string = detail } },
                    }) catch break;
                },
                .open_confirm => {
                    logging.emit(.bgp_open_received, &log_buf, &.{
                        .{ .name = "state", .value = logging.FieldValue{ .string = @tagName(current_state) } },
                        .{ .name = "detail", .value = logging.FieldValue{ .string = detail } },
                    }) catch break;
                },
                .established => {
                    logging.emit(.bgp_established, &log_buf, &.{
                        .{ .name = "state", .value = logging.FieldValue{ .string = @tagName(current_state) } },
                        .{ .name = "detail", .value = logging.FieldValue{ .string = detail } },
                    }) catch break;
                },
                else => {
                    logging.emit(.bgp_error, &log_buf, &.{
                        .{ .name = "state", .value = logging.FieldValue{ .string = @tagName(current_state) } },
                        .{ .name = "detail", .value = logging.FieldValue{ .string = detail } },
                    }) catch break;
                },
            }

            bgpLogToStdout(log_buf.slice());
            previous_state = current_state;
        }

        // Log keepalive only when counter increases (counter-driven, not every loop)
        const current_keepalives = bundle.sess.status.keepalives_sent;
        if (current_keepalives > previous_keepalives_sent) {
            var log_buf = logging.BufferedWriter.init();
            logging.emit(.bgp_keepalive_sent, &log_buf, &.{
                .{ .name = "count", .value = logging.FieldValue{ .integer = @as(i64, @intCast(current_keepalives)) } },
            }) catch break;
            bgpLogToStdout(log_buf.slice());
            previous_keepalives_sent = current_keepalives;
        }

        // Handle session termination and success
        switch (result) {
            .established => {
                // Session established - reset backoff for future failures
                serve_integration.resetBackoff(bundle);
                // Continue running session
            },
            .failed => {
                // Session failed - schedule reconnect instead of exiting
                var log_buf = logging.BufferedWriter.init();
                const err_msg = getConcreteErrorMessage(bundle);
                logging.emit(.bgp_error, &log_buf, &.{
                    .{ .name = "error", .value = logging.FieldValue{ .string = err_msg } },
                    .{ .name = "detail", .value = logging.FieldValue{ .string = "session failed, scheduling reconnect" } },
                }) catch break;
                bgpLogToStdout(log_buf.slice());

                // Schedule reconnect with backoff
                serve_integration.scheduleReconnect(bundle, clock_interface, RECONNECT_MAX_MS);
                // Loop will handle reconnect_wait state on next iteration
            },
            .stopped => {
                // Session stopped cleanly - schedule reconnect
                var log_buf = logging.BufferedWriter.init();
                logging.emit(.bgp_error, &log_buf, &.{
                    .{ .name = "detail", .value = logging.FieldValue{ .string = "session stopped, scheduling reconnect" } },
                }) catch break;
                bgpLogToStdout(log_buf.slice());

                serve_integration.scheduleReconnect(bundle, clock_interface, RECONNECT_MAX_MS);
            },
            .ok => {
                // Session running normally
            },
        }

        // Sleep between iterations to avoid hot-spinning
        // Use nanosleep for Zig 0.16 portable blocking sleep
        var ts: c.timespec = .{
            .sec = @intCast(BGP_LOOP_INTERVAL_MS / 1000),
            .nsec = @intCast((BGP_LOOP_INTERVAL_MS % 1000) * 1_000_000),
        };
        _ = c.nanosleep(&ts, null);
    }
}

/// Get the concrete error message for logging and status.
/// Preferred lookup order:
/// 1. bundle.last_error (bundle-owned, includes concrete session errors)
/// 2. bundle.sess.status.last_error.message (session-owned fallback)
/// 3. "unknown error" fallback
fn getConcreteErrorMessage(bundle: *serve_integration.BgpServeBundle) []const u8 {
    return bundle.last_error orelse
        if (bundle.sess.status.last_error) |session_err| session_err.message else
        "unknown error";
}

/// Start the BGP runtime thread for a configured bundle.
/// Returns true if thread was spawned successfully.
/// Thread failures are non-fatal - caller should log and continue.
///
/// Thread is stored in bundle.runtime_thread for join on cleanup.
/// cleanupBgpBundle() will join this thread before destroying bundle.
pub fn startBgpRuntimeThread(bundle: *serve_integration.BgpServeBundle, stderr: anytype) bool {
    if (std.Thread.spawn(.{}, bgpRuntimeThread, .{bundle})) |thread| {
        // Store thread handle for join on cleanup (NOT detached)
        bundle.runtime_thread = thread;
        return true;
    } else |spawn_err| {
        // Log error, continue serving HTTP (non-fatal)
        var log_buf = logging.BufferedWriter.init();
        logging.emit(.bgp_error, &log_buf, &.{
            .{ .name = "error", .value = logging.FieldValue{ .string = @errorName(spawn_err) } },
            .{ .name = "detail", .value = logging.FieldValue{ .string = "BGP runtime thread spawn failed, continuing without BGP" } },
        }) catch {};
        stderr.writeAll(log_buf.slice()) catch {};
        return false;
    }
}
