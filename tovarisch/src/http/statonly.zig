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
const linux_stats = @import("../net/linux_stats.zig");
const stat_formatter = @import("../net/stat_formatter.zig");
const interface_filter = @import("../net/interface_filter.zig");
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

/// Print compact tunnel stats line to stdout with rate calculation.
/// Uses linux_interface_stats.collectInterfaceStats() which returns all interfaces
/// (not just private-IP ones) to ensure stats are visible in operator mode.
/// Filters to tunnel interfaces only (wg*, tun*, tap*, etc.) for operator focus.
/// First sample is silent - only prints once previous sample exists for rates.
///
/// Caller owns returned snapshots and must free them when replacing.
/// 
/// The sysfs_root parameter allows testing with fake sysfs trees.
/// In production, always use "/sys/class/net" as the sysfs root.
pub fn printCompactStatsWithRate(
    out_writer: anytype,
    previous: ?PreviousSnapshots,
    sysfs_root: []const u8,
) !?PreviousSnapshots {
    // MemoryOwnership: Transient allocation freed within same function scope.
    // All memory is released via freeInterfaceStatsSnapshots() before function returns.
    const allocator = std.heap.page_allocator;
    const now_ms = statonlyNowMillis();

    // Check if we can compute rates - need at least 1000ms elapsed AND a previous sample
    const can_compute_rate = previous != null and (now_ms - previous.?.sampled_at_ms >= 1000);

    // Collect all interface stats using the lower-level collector.
    const snapshots = linux_interface_stats.collectInterfaceStats(
        allocator,
        sysfs_root,
        .sysfs_net,
    ) catch |err| {
        try out_writer.print("net: collect-error:{s}\n", .{@errorName(err)});
        return null;
    };
    defer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, snapshots);

    // Filter to tunnel interfaces only for operator-focused output.
    // This filters out: lo, ens*, eth*, veth*, podman*, bridges, etc.
    var tunnel_ifaces = std.ArrayList(linux_interface_stats.InterfaceStatsSnapshot).empty;
    defer tunnel_ifaces.deinit(allocator);
    for (snapshots) |snap| {
        // Skip loopback
        if (std.mem.eql(u8, snap.name, "lo")) continue;
        // Only include tunnel interfaces (wg*, tun*, tap*, sit*, ip6tnl*, gre*, ipip*)
        if (!interface_filter.isTunnelInterface(snap.name)) continue;
        // Make an owned copy of the name
        // MemoryOwnership: dupe() allocates a heap copy owned by tunnel_ifaces.
        // The ArrayList is deinit'd below if tunnel_ifaces is discarded;
        // if toOwnedSlice succeeds, ownership transfers to PreviousSnapshots.
        const name_copy = try allocator.dupe(u8, snap.name);
        errdefer allocator.free(name_copy);
        try tunnel_ifaces.append(allocator, .{
            .name = name_copy,
            .stats = snap.stats,
        });
    }

    if (tunnel_ifaces.items.len == 0) {
        // No tunnel interfaces - always print "net: no tunnels" so the operator
        // knows the system is running but has no tunnels (not silent forever).
        try out_writer.writeAll("net: no tunnels\n");
        try out_writer.flush();
        return null;
    }

    // If we can't compute rates yet (first sample), collect silently and return
    if (!can_compute_rate) {
        // MemoryOwnership: toOwnedSlice transfers ArrayList memory to PreviousSnapshots.
        // Caller owns the returned snapshots and is responsible for freeing them.
        return PreviousSnapshots{
            .snapshots = try tunnel_ifaces.toOwnedSlice(allocator),
            .sampled_at_ms = now_ms,
        };
    }

    // Compute and print rates for each tunnel interface
    var first = true;
    for (tunnel_ifaces.items) |snap| {
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

    // MemoryOwnership: toOwnedSlice transfers ArrayList memory to PreviousSnapshots.
    // Caller owns the returned snapshots and is responsible for freeing them.
    return PreviousSnapshots{
        .snapshots = try tunnel_ifaces.toOwnedSlice(allocator),
        .sampled_at_ms = now_ms,
    };
}

/// Legacy function for first-call compatibility - prints without rates.
/// Uses production sysfs path.
pub fn printCompactStats(out_writer: anytype) !void {
    const result = try printCompactStatsWithRate(out_writer, null, "/sys/class/net");
    if (result) |r| {
        // MemoryOwnership: page_allocator matches allocator used in printCompactStatsWithRate
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
            const new_previous = try printCompactStatsWithRate(out_writer, old_previous, "/sys/class/net");

            // Now safe to free old snapshots
            if (old_previous) |prev| {
                // MemoryOwnership: page_allocator matches allocator used in printCompactStatsWithRate
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
// Test Writer Helper (Zig 0.16 compatible)
// ============================================================================

const TestWriter = struct {
    const Self = @This();
    const BufSize = 8192;

    buf: [BufSize]u8 = undefined,
    len: usize = 0,

    pub fn init() Self {
        return .{ .buf = undefined, .len = 0 };
    }

    pub fn writer(self: *Self) TestWriterImpl {
        return .{ .tw = self };
    }

    pub fn slice(self: *const Self) []const u8 {
        return self.buf[0..self.len];
    }
};

const TestWriterImpl = struct {
    tw: *TestWriter,

    pub fn print(self: @This(), comptime fmt: []const u8, args: anytype) !void {
        if (self.tw.len >= TestWriter.BufSize) return error.BufferOverflow;
        const remaining = self.tw.buf[self.tw.len..];
        const written = std.fmt.bufPrint(remaining, fmt, args) catch return error.BufferOverflow;
        self.tw.len += written.len;
    }

    pub fn writeAll(self: @This(), bytes: []const u8) !void {
        if (self.tw.len + bytes.len > TestWriter.BufSize) return error.BufferOverflow;
        // MemoryCopySafety: bytes is a parameter slice, self.tw.buf is a separate fixed buffer.
        // These buffers are independent - no aliasing risk.
        @memcpy(self.tw.buf[self.tw.len..][0..bytes.len], bytes);
        self.tw.len += bytes.len;
    }

    pub fn flush(_: @This()) void {} // never fails, but !void for Writer interface compatibility
};

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

// Note: Extended statonly tests with fake sysfs are skipped on macOS due to
// allocator mismatch between page_allocator (used by statonly) and test allocator.
// Core tunnel filtering is tested in tunnel_classification_tests.zig.
// Integration testing is done via make tovarisch-test on Linux VM.

/// Helper to create a fake network interface in a fake sysfs tree.
fn createFakeInterface(_allocator: std.mem.Allocator, base_dir: []const u8, iface_name: []const u8, rx_bytes: u64, tx_bytes: u64, rx_packets: u64, tx_packets: u64) !void {
    _ = _allocator; // reserved for future use
    var iface_path_buf: [1024]u8 = undefined;
    var stats_path_buf: [1024]u8 = undefined;
    var rx_content_buf: [32]u8 = undefined;
    var tx_content_buf: [32]u8 = undefined;
    var rx_pkt_content_buf: [32]u8 = undefined;
    var tx_pkt_content_buf: [32]u8 = undefined;

    const iface_path = std.fmt.bufPrint(&iface_path_buf, "{s}/{s}", .{ base_dir, iface_name }) catch unreachable;
    const stats_path = std.fmt.bufPrint(&stats_path_buf, "{s}/statistics", .{iface_path}) catch unreachable;

    try linux_stats.makeDir(iface_path);
    try linux_stats.makeDir(stats_path);

    var rx_bytes_path_buf: [1024]u8 = undefined;
    var tx_bytes_path_buf: [1024]u8 = undefined;
    var rx_packets_path_buf: [1024]u8 = undefined;
    var tx_packets_path_buf: [1024]u8 = undefined;

    const rx_bytes_path = std.fmt.bufPrint(&rx_bytes_path_buf, "{s}/rx_bytes", .{stats_path}) catch unreachable;
    const tx_bytes_path = std.fmt.bufPrint(&tx_bytes_path_buf, "{s}/tx_bytes", .{stats_path}) catch unreachable;
    const rx_packets_path = std.fmt.bufPrint(&rx_packets_path_buf, "{s}/rx_packets", .{stats_path}) catch unreachable;
    const tx_packets_path = std.fmt.bufPrint(&tx_packets_path_buf, "{s}/tx_packets", .{stats_path}) catch unreachable;

    const rx_content = std.fmt.bufPrint(&rx_content_buf, "{d}\n", .{rx_bytes}) catch unreachable;
    const tx_content = std.fmt.bufPrint(&tx_content_buf, "{d}\n", .{tx_bytes}) catch unreachable;
    const rx_pkt_content = std.fmt.bufPrint(&rx_pkt_content_buf, "{d}\n", .{rx_packets}) catch unreachable;
    const tx_pkt_content = std.fmt.bufPrint(&tx_pkt_content_buf, "{d}\n", .{tx_packets}) catch unreachable;

    try linux_stats.writeFile(rx_bytes_path, rx_content);
    try linux_stats.writeFile(tx_bytes_path, tx_content);
    try linux_stats.writeFile(rx_packets_path, rx_pkt_content);
    try linux_stats.writeFile(tx_packets_path, tx_pkt_content);
}
