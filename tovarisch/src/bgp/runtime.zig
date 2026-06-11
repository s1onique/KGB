// bgp/runtime.zig — BGP FSM runtime worker for tovarisch
//
// ACT runtime: BGP FSM loop runs in a detached thread, driving runSessionOnce.
// The thread logs FSM transitions and sleeps between iterations.
//
// Key behaviors:
// - Logs each FSM transition (open_sent, open_confirm, established, notification, error)
// - Sleeps between iterations to avoid hot-spinning
// - Thread failures are non-fatal (logs error and exits)
// - Bundle lifetime is owned by the caller (main thread) - thread does NOT free bundle
//
// References: RFC 4271 (BGP-4)

const std = @import("std");
const c = std.c;
const session = @import("session.zig");
const logging = @import("../logging.zig");
const serve_integration = @import("serve_integration.zig");

/// BGP FSM loop interval in milliseconds.
/// This is the sleep between runSessionOnce calls.
const BGP_LOOP_INTERVAL_MS: u64 = 100;

/// Format peer address as string for logging.
fn formatPeerAddr(addr: [4]u8, buf: *[32]u8) []const u8 {
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
/// The thread logs FSM transitions and sleeps between iterations.
pub fn bgpRuntimeThread(bundle: *serve_integration.BgpServeBundle) void {
    // Log TCP connected event (TCP connection was already established at load time)
    {
        var log_buf = logging.BufferedWriter.init();
        var peer_addr_buf: [32]u8 = undefined;
        logging.emit(.bgp_connected, &log_buf, &.{
            .{ .name = "peer", .value = logging.FieldValue{ .string = formatPeerAddr(bundle.sess.config.peer_address, &peer_addr_buf) } },
        }) catch return;
        bgpLogToStdout(log_buf.slice());
    }

    // Main FSM loop - bounded and non-hot-spinning
    var previous_state: session.SessionState = .idle;
    var previous_keepalives_sent: u64 = 0;
    while (true) {
        const result = serve_integration.runSessionOnce(bundle);

        // Log FSM transitions when state changes
        const current_state = bundle.sess.status.state;
        if (current_state != previous_state) {
            var log_buf = logging.BufferedWriter.init();

            // Build detail with state info
            const detail = switch (current_state) {
                .open_sent => "OPEN sent",
                .open_confirm => "OPEN received, sent KEEPALIVE",
                .established => "session established",
                .failed => if (bundle.last_error) |e| e else "session failed",
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

        // Handle session termination
        switch (result) {
            .failed => {
                // Session failed - log error and exit thread
                var log_buf = logging.BufferedWriter.init();
                const err_msg = bundle.last_error orelse "unknown error";
                logging.emit(.bgp_error, &log_buf, &.{
                    .{ .name = "error", .value = logging.FieldValue{ .string = err_msg } },
                    .{ .name = "detail", .value = logging.FieldValue{ .string = "BGP runtime thread exiting" } },
                }) catch break;
                bgpLogToStdout(log_buf.slice());
                return;
            },
            .stopped => {
                // Session stopped cleanly - log and exit thread
                var log_buf = logging.BufferedWriter.init();
                logging.emit(.bgp_error, &log_buf, &.{
                    .{ .name = "detail", .value = logging.FieldValue{ .string = "session stopped cleanly" } },
                }) catch break;
                bgpLogToStdout(log_buf.slice());
                return;
            },
            .ok, .established => {},
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

/// Start the BGP runtime thread for a configured bundle.
/// Returns true if thread was spawned successfully.
/// Thread failures are non-fatal - caller should log and continue.
pub fn startBgpRuntimeThread(bundle: *serve_integration.BgpServeBundle, stderr: anytype) bool {
    if (std.Thread.spawn(.{}, bgpRuntimeThread, .{bundle})) |thread| {
        thread.detach();
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
