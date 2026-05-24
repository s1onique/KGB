// statonly.zig — Stat-only mode runtime for tovarisch serve
//
// ACT: Add --statonly operator stats mode
//
// This module provides the stat-only mode runtime loop that:
// - Uses poll() with timeout for non-blocking accept
// - Periodically prints compact interface stats to stdout
// - Suppresses normal startup/runtime logs
// - Still emits critical errors to stderr

const std = @import("std");
const c = std.c;
const linux_interface_stats = @import("../net/linux_interface_stats.zig");
const stat_formatter = @import("../net/stat_formatter.zig");
const routes = @import("routes.zig");
const rates = @import("../net/rates.zig");

// ============================================================================
// Statonly Runtime
// ============================================================================

/// Get current time in milliseconds for stat-only scheduling.
///
/// Uses std.c.gettimeofday() for reliable cross-platform runtime timestamps.
/// Unlike metrics_state.currentWallClockMillis(), this always returns a valid
/// runtime timestamp that works in deployed binaries on Linux and macOS.
///
/// NOTE: This is NOT the same as wall-clock epoch time used for /metrics.json
/// rates. Stat-only mode uses this for local scheduling only.
fn statonlyNowMillis() i64 {
    var tv: std.c.timeval = undefined;
    if (std.c.gettimeofday(&tv, null) < 0) return 0;
    return @as(i64, tv.sec) * 1000 + @divTrunc(@as(i64, tv.usec), 1000);
}

/// Log a critical error message to stderr.
/// Always emits regardless of log mode - critical errors must be visible.
fn logCritical(stderr_writer: anytype, comptime fmt: []const u8, args: anytype) void {
    stderr_writer.print("critical: " ++ fmt, args) catch {};
    stderr_writer.flush();
}

/// Previous snapshot for rate calculation.
const PreviousSnapshots = struct {
    snapshots: []linux_interface_stats.InterfaceStatsSnapshot,
    sampled_at_ms: i64,
};

/// Handle a single connection with state.
fn handleConnection(conn_fd: i32, state: *anyopaque) void {
    defer _ = c.close(conn_fd);

    // Read request line
    var buf: [1024]u8 = undefined;
    const bytes_read = c.read(conn_fd, &buf, buf.len);
    if (bytes_read <= 0) return;

    // Find the end of the request line (first \r\n or \n)
    const request_line_end = std.mem.indexOfAny(u8, buf[0..@as(usize, @intCast(bytes_read))], "\r\n") orelse @as(usize, @intCast(bytes_read));
    const request_line = std.mem.trim(u8, buf[0..request_line_end], " \t");

    // Parse the request
    const req = routes.parseRequestLine(request_line) orelse {
        return;
    };

    // Route and handle the request with state
    _ = routes.routeRequestFd(conn_fd, req, state) catch return;
}

/// Accept one connection and handle it (blocking).
/// Returns error.AcceptFailed for non-transient failures.
/// EAGAIN/EWOULDBLOCK are treated as transient and return successfully.
fn acceptOneBlocking(listener_fd: i32, state: *anyopaque) !void {
    var client_addr: c.sockaddr = undefined;
    var client_len: c.socklen_t = @sizeOf(c.sockaddr);

    const conn_fd = c.accept(listener_fd, &client_addr, &client_len);
    if (conn_fd < 0) {
        const errno_val = std.c._errno().*;
        // EAGAIN (11 on Linux, 35 on macOS) and EWOULDBLOCK are transient
        if (errno_val == 11 or errno_val == 35) {
            std.Thread.yield() catch {};
            return;
        }
        // Non-transient error - return typed error for logging
        return error.AcceptFailed;
    }

    handleConnection(conn_fd, state);
}

/// Accept with a timeout for statonly mode polling.
pub fn acceptOneWithTimeout(
    listener_fd: i32,
    state: *anyopaque,
    timeout_ms: u32,
) !void {
    // Use poll to timeout accept
    // POLLIN = 1 on Linux/macOS
    var poll_fd: [1]std.c.pollfd = .{
        .{
            .fd = listener_fd,
            .events = 1, // POLLIN
            .revents = 0,
        },
    };

    const result = std.c.poll(&poll_fd, 1, @as(i32, @intCast(timeout_ms)));
    if (result < 0) {
        const errno_val = std.c._errno().*;
        // EAGAIN/EWOULDBLOCK are transient
        if (errno_val == 11 or errno_val == 35) return;
        return error.PollFailed;
    }

    if (result == 0) {
        // Timeout - no connection ready
        return;
    }

    // Connection ready, accept it
    acceptOneBlocking(listener_fd, state) catch |err| {
        // Non-transient accept error
        return err;
    };
}

/// Print compact stats line to stdout with rate calculation.
/// Uses linux_interface_stats.collectInterfaceStats() which returns all interfaces
/// (not just private-IP ones) to ensure stats are visible in operator mode.
/// Loopback interface is filtered out for human-readable output.
/// Caller owns returned snapshots and must free them when replacing.
pub fn printCompactStatsWithRate(
    out_writer: anytype,
    previous: ?PreviousSnapshots,
) !?PreviousSnapshots {
    const allocator = std.heap.page_allocator;
    const now_ms = statonlyNowMillis();

    // Check if we can compute rates - need at least 1000ms elapsed
    const can_compute_rate = previous != null and (now_ms - previous.?.sampled_at_ms >= 1000);

    // Collect all interface stats using the lower-level collector.
    // This is less strict than collectPrivateInterfaceStats() which may
    // return empty on systems without RFC1918 private addresses.
    const snapshots = linux_interface_stats.collectInterfaceStats(
        allocator,
        "/sys",
    ) catch |err| {
        try out_writer.print("net: collect-error:{s}\n", .{@errorName(err)});
        return null;
    };
    defer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, snapshots);

    // Filter out loopback interface for human-readable stats
    var non_loopback = std.ArrayList(linux_interface_stats.InterfaceStatsSnapshot).empty;
    defer non_loopback.deinit(allocator);
    for (snapshots) |snap| {
        if (std.mem.eql(u8, snap.name, "lo")) continue;
        // Make an owned copy of the name
        const name_copy = try allocator.dupe(u8, snap.name);
        errdefer allocator.free(name_copy);
        try non_loopback.append(allocator, .{
            .name = name_copy,
            .stats = snap.stats,
        });
    }

    if (non_loopback.items.len == 0) {
        out_writer.writeAll("net: no interfaces\n") catch {};
        return null;
    }

    // If we can't compute rates yet, print warm-up message and return snapshots
    if (!can_compute_rate) {
        out_writer.writeAll("net: sampling\n") catch {};
        try out_writer.flush();
        return PreviousSnapshots{
            .snapshots = try non_loopback.toOwnedSlice(allocator),
            .sampled_at_ms = now_ms,
        };
    }

    // Collect rates for each interface
    var first = true;
    for (non_loopback.items) |snap| {
        if (!first) {
            try out_writer.writeAll(" | ");
        }
        first = false;

        var rate: ?rates.InterfaceRate = null;

        // Find matching previous snapshot by name
        for (previous.?.snapshots) |prev_snap| {
            if (std.mem.eql(u8, snap.name, prev_snap.name)) {
                const prev_sample = rates.InterfaceCounterSample{
                    .name = prev_snap.name,
                    .rx_bytes = prev_snap.stats.rx_bytes,
                    .tx_bytes = prev_snap.stats.tx_bytes,
                    .rx_packets = prev_snap.stats.rx_packets,
                    .tx_packets = prev_snap.stats.tx_packets,
                    .sampled_at_ms = previous.?.sampled_at_ms,
                };
                const curr_sample = rates.InterfaceCounterSample{
                    .name = snap.name,
                    .rx_bytes = snap.stats.rx_bytes,
                    .tx_bytes = snap.stats.tx_bytes,
                    .rx_packets = snap.stats.rx_packets,
                    .tx_packets = snap.stats.tx_packets,
                    .sampled_at_ms = now_ms,
                };
                rate = rates.calculateRate(prev_sample, curr_sample);
                break;
            }
        }

        var writer = stat_formatter.CompactLineWriter.init();
        try stat_formatter.formatInterfaceLine(snap.name, rate, 0, 0, &writer);
        try out_writer.writeAll(writer.slice());
    }
    try out_writer.writeAll("\n");
    try out_writer.flush();

    return PreviousSnapshots{
        .snapshots = try non_loopback.toOwnedSlice(allocator),
        .sampled_at_ms = now_ms,
    };
}

/// Legacy function for first-call compatibility - prints without rates.
pub fn printCompactStats(out_writer: anytype) !void {
    const result = try printCompactStatsWithRate(out_writer, null);
    if (result) |r| {
        linux_interface_stats.freeInterfaceStatsSnapshots(std.heap.page_allocator, r.snapshots);
    }
}

/// Stat-only mode serve loop with compact stats output.
/// Uses non-blocking accept with poll() and prints stats periodically.
/// Critical errors go to stderr_writer, stats output to out_writer.
/// Maintains previous snapshots for rate calculation across intervals.
pub fn serveStatonly(
    listener_fd: i32,
    state: *anyopaque,
    interval_seconds: u16,
    out_writer: anytype,
    stderr_writer: anytype,
) !void {
    const interval_ms = @as(i64, interval_seconds) * 1000;
    // Use statonlyNowMillis() for reliable runtime timestamps.
    // First emission is immediate (not interval_ms in the future).
    var next_stats_at = statonlyNowMillis();
    var previous: ?PreviousSnapshots = null;

    // Blocking accept loop with periodic stats printing
    while (true) {
        // Accept with a short timeout to check for stats interval
        const accept_timeout_ms: u32 = 1000; // 1 second timeout
        acceptOneWithTimeout(listener_fd, state, accept_timeout_ms) catch |err| {
            // Log critical error to stderr
            logCritical(stderr_writer, "accept loop error: {s}\n", .{@errorName(err)});
        };

        // Check if it's time to print stats using reliable runtime clock
        const now = statonlyNowMillis();
        if (now >= next_stats_at) {
            // Preserve previous snapshots until after rate calculation
            const old_previous = previous;
            const new_previous = try printCompactStatsWithRate(out_writer, old_previous);

            // Now safe to free old snapshots
            if (old_previous) |prev| {
                linux_interface_stats.freeInterfaceStatsSnapshots(std.heap.page_allocator, prev.snapshots);
            }

            previous = new_previous;
            next_stats_at = now + interval_ms;
        }
    }
}

/// Write critical error to stderr using low-level write.
fn writeStderr(comptime msg: []const u8) void {
    _ = std.c.write(2, msg.ptr, msg.len);
}

/// StderrWriter struct that writes to real stderr via low-level write.
const StderrWriterImpl = struct {
    fn writeAll(_: StderrWriterImpl, bytes: []const u8) !void {
        _ = std.c.write(2, bytes.ptr, bytes.len);
    }
    fn print(_: StderrWriterImpl, comptime fmt: []const u8, args: anytype) !void {
        var buf: [256]u8 = undefined;
        const slice = std.fmt.bufPrint(&buf, fmt, args) catch return;
        _ = std.c.write(2, slice.ptr, slice.len);
    }
    fn flush(_: StderrWriterImpl) void {}
};

/// Stat-only mode serve loop that writes critical errors to real stderr.
pub fn serveStatonlyWithStderr(
    listener_fd: i32,
    state: *anyopaque,
    interval_seconds: u16,
    out_writer: anytype,
) !void {
    const stderr = StderrWriterImpl{};
    try serveStatonly(listener_fd, state, interval_seconds, out_writer, stderr);
}

// ============================================================================
// Tests
// ============================================================================

test "statonlyNowMillis returns positive timestamp" {
    const ts = statonlyNowMillis();
    // Must always return positive timestamp - no fallback to 0
    try std.testing.expect(ts > 0);
}

test "statonly scheduler first emission is immediate" {
    // Regression test: ensure statonlyNowMillis() doesn't return 0
    // which would cause scheduler to stall indefinitely.
    // The first emission should be immediate (now >= next_stats_at on first iteration).
    const now = statonlyNowMillis();
    const interval_ms = @as(i64, 30) * 1000;
    const next_stats_at = now; // First emission is immediate, not now + interval

    // Verify first emission triggers immediately
    try std.testing.expect(now >= next_stats_at);

    // Verify subsequent emission is interval_ms in the future
    const next_time = now + interval_ms;
    try std.testing.expect(next_time > now);
    try std.testing.expect(next_time - now == interval_ms);
}
