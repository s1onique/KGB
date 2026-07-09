/// Startup phase timing and readiness trace for tovarisch.
///
/// This module provides structured startup phase instrumentation to make the
/// gap between process start and HTTP readiness diagnosable.
///
/// Background: Systemd's "Started" line is process-start semantics unless the
/// unit uses Type=notify. For Type=simple services (default), systemd considers
/// startup complete when the process forks/execs, NOT when HTTP is listening.
///
/// The canonical application readiness event is `startup_ready`, emitted after
/// the HTTP accept loop is ready to accept connections.
///
/// For production diagnosis of startup gaps:
/// ```
/// journalctl -u tovarisch.service -n 200 -o short-iso | \
///   grep -E 'startup_phase|startup_ready|http_server_listening'
/// ```
///
/// See docs/operations/startup-readiness.md for full operational guidance.

const std = @import("std");
const logging = @import("logging.zig");
const startup_trace_time = @import("startup_trace_time.zig");

// ============================================================================
// Timestamp Helper
// ============================================================================

/// Alias for cross-platform monotonic time.
const monoTimeNanos = startup_trace_time.monoTimeNanos;

/// Startup phases instrumented during tovarisch initialization.
/// Each phase represents a distinct synchronous initialization step.
pub const StartupPhase = enum {
    /// Config file loading and parsing (if config path provided).
    config_load,
    /// Runtime status context initialization.
    runtime_init,
    /// WireGuard status subsystem initialization.
    wg_status_init,
    /// BFD configuration loading and loop startup.
    bfd_start,
    /// BGP configuration loading (not FSM startup).
    bgp_load,
    /// BGP FSM thread startup (separate from bgp_load).
    bgp_runtime_start,
    /// Lab and network diagnostics configuration parsing.
    lab_and_net_diag_config_parse,
    /// HTTP server socket creation and bind.
    http_bind,
    /// HTTP accept loop startup.
    http_accept_loop,
};

/// Default threshold in milliseconds before a phase is considered "slow".
/// Phases exceeding this will emit startup_phase_slow.
pub const DEFAULT_SLOW_THRESHOLD_MS: u64 = 5000;

/// PhaseGuard: RAII-style timer for a single startup phase.
///
/// Created via StartupTracer.begin(), emits startup_phase_started on creation
/// and startup_phase_finished (or startup_phase_slow) on drop.
pub const PhaseGuard = struct {
    const Self = @This();

    tracer: *StartupTracer,
    phase: StartupPhase,
    started_ns: i128,
    emitted_started: bool = false,

    /// Emit startup_phase_started event.
    pub fn emitStarted(self: *Self, out_writer: anytype) !void {
        var log_buf = logging.BufferedWriter.init();
        try logging.emit(.startup_phase_started, &log_buf, &.{
            .{ .name = "phase", .value = logging.FieldValue{ .string = @tagName(self.phase) } },
        });
        try out_writer.writeAll(log_buf.slice());
        self.emitted_started = true;
    }

    /// Emit startup_phase_finished or startup_phase_slow event.
    pub fn finish(self: *Self, out_writer: anytype) !void {
        const elapsed_ns = monoTimeNanos() - self.started_ns;
        const elapsed_ms = @divTrunc(elapsed_ns, std.time.ns_per_ms);
        const is_slow = elapsed_ms >= self.tracer.slow_threshold_ms;

        var log_buf = logging.BufferedWriter.init();
        if (is_slow) {
            try logging.emit(.startup_phase_slow, &log_buf, &.{
                .{ .name = "phase", .value = logging.FieldValue{ .string = @tagName(self.phase) } },
                .{ .name = "duration_ms", .value = logging.FieldValue{ .integer = @intCast(elapsed_ms) } },
                .{ .name = "threshold_ms", .value = logging.FieldValue{ .integer = @intCast(self.tracer.slow_threshold_ms) } },
            });
        } else {
            try logging.emit(.startup_phase_finished, &log_buf, &.{
                .{ .name = "phase", .value = logging.FieldValue{ .string = @tagName(self.phase) } },
                .{ .name = "duration_ms", .value = logging.FieldValue{ .integer = @intCast(elapsed_ms) } },
            });
        }
        try out_writer.writeAll(log_buf.slice());
    }
};

/// StartupTracer: tracks startup phases and emits timing events.
///
/// Usage:
/// ```
/// var tracer = StartupTracer.init();
/// defer tracer.deinit();
///
/// {
///     var guard = tracer.begin(.config_load);
///     try guard.emitStarted(stdout);
///     // ... do config loading work ...
///     try guard.finish(stdout);
/// }
/// ```
///
/// When the tracer goes out of scope, no cleanup is needed (no allocations).
pub const StartupTracer = struct {
    const Self = @This();

    /// Monotonic timestamp when the tracer was created (process entry).
    /// Used to compute total startup duration for startup_ready.
    service_started_ns: i128,

    /// Threshold in ms above which a phase is considered "slow".
    slow_threshold_ms: u64,

    /// Initialize a new tracer with default slow threshold.
    pub fn init() Self {
        return Self{
            .service_started_ns = monoTimeNanos(),
            .slow_threshold_ms = DEFAULT_SLOW_THRESHOLD_MS,
        };
    }

    /// Initialize a tracer with custom slow threshold.
    pub fn initWithThreshold(slow_threshold_ms: u64) Self {
        return Self{
            .service_started_ns = monoTimeNanos(),
            .slow_threshold_ms = slow_threshold_ms,
        };
    }

    /// Begin timing a startup phase. Returns a PhaseGuard that will emit
    /// timing events when dropped.
    pub fn begin(self: *Self, phase: StartupPhase) PhaseGuard {
        return PhaseGuard{
            .tracer = self,
            .phase = phase,
            .started_ns = monoTimeNanos(),
            .emitted_started = false,
        };
    }

    /// Emit the canonical startup_ready event.
    ///
    /// This should be called exactly once, after the HTTP accept loop is ready.
    /// It computes the total startup duration from process entry to readiness.
    ///
    /// NOTE: This does NOT send systemd READY=1 via sd_notify(). Adding
    /// Type=notify support requires a separate follow-up ACT:
    /// ACT-TOVARISCH-SYSTEMD-NOTIFY-READY01
    pub fn ready(self: *Self, out_writer: anytype, kind: []const u8, bind_address: []const u8, port: u16) !void {
        const total_ns = monoTimeNanos() - self.service_started_ns;
        const total_ms = @divTrunc(total_ns, std.time.ns_per_ms);

        var log_buf = logging.BufferedWriter.init();
        try logging.emit(.startup_ready, &log_buf, &.{
            .{ .name = "ready_kind", .value = logging.FieldValue{ .string = kind } },
            .{ .name = "startup_duration_ms", .value = logging.FieldValue{ .integer = @intCast(total_ms) } },
            .{ .name = "bind_address", .value = logging.FieldValue{ .string = bind_address } },
            .{ .name = "port", .value = logging.FieldValue{ .integer = port } },
        });
        try out_writer.writeAll(log_buf.slice());
    }
};
