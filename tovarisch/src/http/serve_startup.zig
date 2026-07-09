// serve_startup.zig — Startup-aware HTTP serve loop functions.
//
// Contains serveForeverNormalWithTracer which emits startup_ready after HTTP accept loop starts.
// Uses the real acceptOneNormal from server.zig to handle connections properly.
// Also contains serveForeverAfterBind which handles post-bind initialization.

const std = @import("std");
const logging = @import("../logging.zig");
const startup_logs = @import("startup_logs.zig");
const startup_trace = @import("../startup_trace.zig");
const heartbeat = @import("heartbeat.zig");
const statonly = @import("statonly.zig");
const status = @import("../status.zig");
const tovarisch_config = @import("../config.zig");
const network_diag_config = @import("../net/network_diag_config.zig");
const lab_events = @import("../runtime/lab_events.zig");
const server = @import("server.zig");

/// Normal mode serve loop with structured JSON logging and optional startup tracer.
/// The accept loop uses the real acceptOneNormal from server.zig which calls handleConnection.
pub fn serveForeverNormalWithTracer(
    listener_fd: i32,
    state_ptr: *anyopaque,
    log_buf: *logging.BufferedWriter,
    out_writer: anytype,
    tracer: ?*startup_trace.StartupTracer,
    bind_address: []const u8,
    port: u16,
) !void {
    // Phase: http_accept_loop — HTTP accept loop is now running
    // Guard ends before infinite loop so phase_finished emits during startup.
    if (tracer) |t| {
        var guard = t.begin(.http_accept_loop);
        try guard.emitStarted(out_writer);
        try logging.emit(.http_accept_loop_started, log_buf, &.{});
        try startup_logs.writeLogRecord(out_writer, log_buf.slice());
        try t.ready(out_writer, "http_accept_loop", bind_address, port);
        try guard.finish(out_writer);
    } else {
        try logging.emit(.http_accept_loop_started, log_buf, &.{});
        try startup_logs.writeLogRecord(out_writer, log_buf.slice());
    }

    // Now enter infinite accept loop - phases have already finished
    while (true) {
        server.acceptOneNormal(listener_fd, state_ptr) catch |err| {
            log_buf.reset();
            try logging.emit(.http_accept_loop_error, log_buf, &.{
                .{ .name = "error", .value = logging.FieldValue{ .string = @errorName(err) } },
            });
            try startup_logs.writeLogRecord(out_writer, log_buf.slice());
        };
    }
}

/// Internal serve function - called after HTTP bind is complete.
/// Separated to allow http_bind phase to finish before entering the infinite accept loop.
/// The tracer parameter enables http_accept_loop phase instrumentation and startup_ready emission.
pub fn serveForeverAfterBind(
    server_ptr: *server.Server,
    config: server.Config,
    inputs: status.RuntimeStatusInputs,
    lab_config: tovarisch_config.LabConfig,
    network_diag_cfg: network_diag_config.NetworkDiagConfig,
    out_writer: anytype,
    lab_emitter_opt: ?*lab_events.LabEventEmitter,
    tracer: ?*startup_trace.StartupTracer,
) !void {
    defer server_ptr.deinit();

    // Initialize serve context with full runtime inputs (BFD + config check + BGP bundle + lab config + network diag config).
    // MemoryOwnership: Startup-only one-time allocation at daemon init.
    // The ServeContext allocator is used once at serve startup, not per-request.
    // This is a single allocation that persists for daemon lifetime (acceptable).
    var serve_ctx = server.ServeContext.initWithContext(
        std.heap.page_allocator,
        inputs.bfd_runtime,
        inputs.config_check,
        inputs.bgp_result,
        lab_config,
        network_diag_cfg,
    );
    // Wire lab event emitter into serve context for /status exposure
    serve_ctx.lab_event_emitter = lab_emitter_opt;
    defer serve_ctx.deinit();

    // NOTE: server.listen() was already called by the caller before passing server here.
    // We do NOT call listen() again to avoid double-bind errors.

    // Emit startup logs only if not in statonly mode
    try startup_logs.emitStartupLogsIfNormal(config.log_mode, config.port, config.address, out_writer);

    // Get opaque pointer to ServeContext for passing to route handlers.
    const ctx_ptr: *anyopaque = &serve_ctx;

    // Heartbeat thread: only spawn in normal mode.
    // In statonly mode, we skip heartbeat to keep output clean.
    // When lab emitter is available, use heartbeatThreadWithEvents for native event emission.
    // Skip heartbeat if lab_config.disable_heartbeat is true (lab runtime toggle).
    var log_buf = logging.BufferedWriter.init();
    if (config.log_mode == .normal and !lab_config.disable_heartbeat) {
        if (lab_emitter_opt) |emitter| {
            if (std.Thread.spawn(.{}, heartbeat.heartbeatThreadWithEvents, .{emitter})) |thread| {
                thread.detach();
            } else |spawn_err| {
                log_buf.reset();
                logging.emit(.heartbeat_thread_start_failed, &log_buf, &.{
                    .{ .name = "error", .value = logging.FieldValue{ .string = @errorName(spawn_err) } },
                    .{ .name = "detail", .value = logging.FieldValue{ .string = "heartbeat spawn failed, continuing without heartbeat" } },
                }) catch {};
                startup_logs.writeLogRecord(out_writer, log_buf.slice()) catch {};
            }
        } else {
            if (std.Thread.spawn(.{}, heartbeat.heartbeatThread, .{})) |thread| {
                thread.detach();
            } else |spawn_err| {
                log_buf.reset();
                logging.emit(.heartbeat_thread_start_failed, &log_buf, &.{
                    .{ .name = "error", .value = logging.FieldValue{ .string = @errorName(spawn_err) } },
                    .{ .name = "detail", .value = logging.FieldValue{ .string = "heartbeat spawn failed, continuing without heartbeat" } },
                }) catch {};
                startup_logs.writeLogRecord(out_writer, log_buf.slice()) catch {};
            }
        }
    } else if (lab_config.disable_heartbeat) {
        // Log when heartbeat is disabled via lab runtime toggle
        log_buf.reset();
        logging.emit(.heartbeat_thread_start_failed, &log_buf, &.{
            .{ .name = "error", .value = logging.FieldValue{ .string = "disabled" } },
            .{ .name = "detail", .value = logging.FieldValue{ .string = "heartbeat disabled via lab_config.disable_heartbeat" } },
        }) catch {};
        startup_logs.writeLogRecord(out_writer, log_buf.slice()) catch {};
    }

    // Branch based on log mode
    if (config.log_mode == .statonly) {
        try statonly.serveStatonlyWithStderr(server_ptr.listener_fd, ctx_ptr, config.stats_interval_seconds, out_writer);
    } else {
        // Pass tracer to enable http_accept_loop phase and startup_ready emission
        try serveForeverNormalWithTracer(server_ptr.listener_fd, ctx_ptr, &log_buf, out_writer, tracer, config.address, config.port);
    }
}
