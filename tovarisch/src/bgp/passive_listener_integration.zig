// bgp/passive_listener_integration.zig — Passive listener integration
//
// Manages the passive BGP listener lifecycle for serve_integration.
// Handles creation, polling, and cleanup of the passive listener.

const serve_integration = @import("serve_integration.zig");
const passive_listener = @import("passive_listener.zig");
const logging = @import("../logging.zig");

// ============================================================================
// Passive Listener Management
// ============================================================================

/// Create and start the passive listener for a bundle.
/// When local_address is configured, creates a passive TCP/179 listener.
/// The listener is always-on once started.
///
/// IMPORTANT: The thread must be started with a pointer to the stable location
/// (bundle.passive_listener), not a temporary stack variable. This prevents the
/// thread from polling a dead stack address after the function returns.
pub fn createPassiveListener(
    bundle: *serve_integration.BgpServeBundle,
    stderr: anytype,
) void {
    // Passive listener is optional - only create if we have a local_address
    const local_addr = bundle.session_config.local_address orelse return;

    // Use peer_address as allowed_peer for security.
    // Only connections from the configured BGP peer are accepted.
    const listener_config = passive_listener.PassiveListenerConfig{
        .local_address = local_addr,
        .port = 179,
        .accept_timeout_ms = 500,
        .allowed_peer_address = bundle.session_config.peer_address,
    };

    var listener = passive_listener.createPassiveListener(listener_config) catch |e| {
        var log_buf = logging.BufferedWriter.init();
        logging.emit(.bgp_error, &log_buf, &.{
            .{ .name = "error", .value = logging.FieldValue{ .string = @errorName(e) } },
            .{ .name = "detail", .value = logging.FieldValue{ .string = "failed to create passive listener, continuing without passive accept" } },
        }) catch {};
        stderr.writeAll(log_buf.slice()) catch {};

        // Store failed listener so status can report bind failure.
        // This is required for the passive listener ACT - failures must not disappear.
        const failed_listener = passive_listener.PassiveListener{
            .config = listener_config,
            .state = .bind_failed,
            .error_message = @errorName(e),
        };
        bundle.passive_listener = failed_listener;
        return;
    };

    // Mark listener as bound (successful bind)
    listener.state = .bound;

    // CRITICAL: Store listener in bundle BEFORE starting thread.
    // The thread must be started with a pointer to the stable location.
    bundle.passive_listener = listener;

    // Now start the thread with a pointer to the bundle copy
    if (bundle.passive_listener) |*stored_listener| {
        passive_listener.startListenerThread(stored_listener) catch |e| {
            var log_buf = logging.BufferedWriter.init();
            logging.emit(.bgp_error, &log_buf, &.{
                .{ .name = "error", .value = logging.FieldValue{ .string = @errorName(e) } },
                .{ .name = "detail", .value = logging.FieldValue{ .string = "failed to start passive listener thread, continuing without passive accept" } },
            }) catch {};
            stderr.writeAll(log_buf.slice()) catch {};

            // Update the stored listener to report thread failure.
            stored_listener.state = .thread_failed;
            stored_listener.error_message = @errorName(e);
            return;
        };
    }

    var log_buf = logging.BufferedWriter.init();
    var listen_addr = passive_listener.getListenAddress(&listener);
    logging.emit(.bgp_connected, &log_buf, &.{
        .{ .name = "detail", .value = logging.FieldValue{ .string = "passive listener started on " } },
        .{ .name = "peer", .value = logging.FieldValue{ .string = &listen_addr } },
    }) catch {};
    stderr.writeAll(log_buf.slice()) catch {};
}

/// Check if there's a pending accepted connection from the passive listener.
pub fn hasPendingPassiveConnection(bundle: *const serve_integration.BgpServeBundle) bool {
    if (bundle.passive_listener) |*listener| {
        return passive_listener.hasPendingConnection(listener);
    }
    return false;
}

/// Pick up a pending accepted connection from the passive listener.
/// Returns the accepted transport ready for BGP session use.
pub fn acceptPassiveConnection(
    bundle: *serve_integration.BgpServeBundle,
) passive_listener.ListenerError!passive_listener.AcceptResult {
    if (bundle.passive_listener) |*listener| {
        return passive_listener.acceptConnection(listener);
    }
    return passive_listener.ListenerError.NoPendingConnection;
}

/// Stop and clean up the passive listener.
pub fn closePassiveListener(bundle: *serve_integration.BgpServeBundle) void {
    if (bundle.passive_listener) |*listener| {
        passive_listener.close(listener);
        bundle.passive_listener = null;
    }
}
