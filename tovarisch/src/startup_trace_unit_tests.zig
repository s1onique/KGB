// startup_trace_unit_tests.zig — Unit tests for startup tracing core types.

const std = @import("std");
const startup_trace = @import("startup_trace.zig");
const logging = @import("logging.zig");

test "StartupTracer.init() sets service_started_ns" {
    const tracer = startup_trace.StartupTracer.init();
    try std.testing.expect(tracer.service_started_ns > 0);
    try std.testing.expect(tracer.slow_threshold_ms == startup_trace.DEFAULT_SLOW_THRESHOLD_MS);
}

test "StartupTracer.initWithThreshold() sets custom threshold" {
    const tracer = startup_trace.StartupTracer.initWithThreshold(10000);
    try std.testing.expect(tracer.slow_threshold_ms == 10000);
}

test "PhaseGuard.begin() captures phase and start time" {
    var tracer = startup_trace.StartupTracer.init();
    const guard = tracer.begin(.config_load);
    try std.testing.expect(guard.phase == .config_load);
    try std.testing.expect(guard.started_ns > 0);
}

test "PhaseGuard emits startup_phase_started" {
    var tracer = startup_trace.StartupTracer.init();
    var guard = tracer.begin(.bfd_start);
    var log_buf = logging.BufferedWriter.init();
    try guard.emitStarted(&log_buf);
    const output = log_buf.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"event\":\"startup_phase_started\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"phase\":\"bfd_start\""));
}

test "PhaseGuard emits startup_phase_finished for fast phases" {
    var tracer = startup_trace.StartupTracer.initWithThreshold(5000);
    var guard = tracer.begin(.http_bind);
    var log_buf = logging.BufferedWriter.init();
    try guard.emitStarted(&log_buf);
    log_buf.reset();
    try guard.finish(&log_buf);
    const output = log_buf.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"event\":\"startup_phase_finished\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"phase\":\"http_bind\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"duration_ms\":"));
    try std.testing.expect(!std.mem.containsAtLeast(u8, output, 1, "startup_phase_slow"));
}

test "PhaseGuard emits startup_phase_slow for slow phases" {
    var tracer = startup_trace.StartupTracer.initWithThreshold(100);
    var guard = tracer.begin(.wg_status_init);
    var ts: std.c.timespec = .{ .sec = 0, .nsec = 200_000_000 };
    _ = std.c.nanosleep(&ts, null);
    var log_buf = logging.BufferedWriter.init();
    try guard.finish(&log_buf);
    const output = log_buf.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"event\":\"startup_phase_slow\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"phase\":\"wg_status_init\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"threshold_ms\":100"));
}

test "StartupTracer.ready() emits startup_ready" {
    var tracer = startup_trace.StartupTracer.init();
    var ts: std.c.timespec = .{ .sec = 0, .nsec = 50_000_000 };
    _ = std.c.nanosleep(&ts, null);
    var log_buf = logging.BufferedWriter.init();
    try tracer.ready(&log_buf, "http_accept_loop", "10.149.149.1", 8317);
    const output = log_buf.slice();
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"event\":\"startup_ready\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"ready_kind\":\"http_accept_loop\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"bind_address\":\"10.149.149.1\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, output, 1, "\"port\":8317"));
}

test "Phase names are stable snake_case" {
    try std.testing.expectEqualStrings("config_load", @tagName(.config_load));
    try std.testing.expectEqualStrings("runtime_init", @tagName(.runtime_init));
    try std.testing.expectEqualStrings("wg_status_init", @tagName(.wg_status_init));
    try std.testing.expectEqualStrings("bfd_start", @tagName(.bfd_start));
    try std.testing.expectEqualStrings("http_bind", @tagName(.http_bind));
    try std.testing.expectEqualStrings("http_accept_loop", @tagName(.http_accept_loop));
}

test "PhaseGuard emits log records ending with newline for NDJSON" {
    var tracer = startup_trace.StartupTracer.init();
    var guard = tracer.begin(.bfd_start);
    var log_buf = logging.BufferedWriter.init();
    try guard.emitStarted(&log_buf);
    const started_output = log_buf.slice();
    try std.testing.expect(started_output[started_output.len - 1] == '\n');
    log_buf.reset();
    try guard.finish(&log_buf);
    const finished_output = log_buf.slice();
    try std.testing.expect(finished_output[finished_output.len - 1] == '\n');
}

test "Multiple phases can be timed sequentially" {
    var tracer = startup_trace.StartupTracer.init();
    {
        var guard = tracer.begin(.config_load);
        var log_buf = logging.BufferedWriter.init();
        try guard.emitStarted(&log_buf);
        log_buf.reset();
        try guard.finish(&log_buf);
        try std.testing.expect(std.mem.containsAtLeast(u8, log_buf.slice(), 1, "\"phase\":\"config_load\""));
    }
    {
        var guard = tracer.begin(.bfd_start);
        var log_buf = logging.BufferedWriter.init();
        try guard.emitStarted(&log_buf);
        log_buf.reset();
        try guard.finish(&log_buf);
        try std.testing.expect(std.mem.containsAtLeast(u8, log_buf.slice(), 1, "\"phase\":\"bfd_start\""));
    }
    {
        var log_buf = logging.BufferedWriter.init();
        try tracer.ready(&log_buf, "http_accept_loop", "127.0.0.1", 8317);
        try std.testing.expect(std.mem.containsAtLeast(u8, log_buf.slice(), 1, "\"event\":\"startup_ready\""));
    }
}
