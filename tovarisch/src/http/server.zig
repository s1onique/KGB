const std = @import("std");
const c = std.c;
const routes = @import("routes.zig");
const logging = @import("../logging.zig");
const metrics_state = @import("../metrics_state.zig");
const heartbeat = @import("heartbeat.zig");

// ============================================================================
// Server State
// ============================================================================

/// Runtime state owned by the server for the duration of a serve session.
///
/// This struct owns the MetricsState that persists across HTTP requests
/// for rate calculation. It is initialized when the server starts and
/// deinitialized when the server exits.
///
/// Ownership model:
/// - Server owns ServerState (initialized in serve loop, deinitialized on exit)
/// - ServerState owns MetricsState (freed in deinit)
/// - MetricsState owns InterfaceSampler (freed in deinit)
pub const ServerState = struct {
    const Self = @This();

    allocator: std.mem.Allocator,
    metrics: metrics_state.MetricsState,

    /// Initialize server state with empty metrics sampler.
    pub fn init(allocator: std.mem.Allocator) Self {
        return .{
            .allocator = allocator,
            .metrics = metrics_state.MetricsState.init(allocator),
        };
    }

    /// Free all server-owned memory.
    pub fn deinit(self: *Self) void {
        self.metrics.deinit();
    }
};

// ============================================================================
// Server Configuration
// ============================================================================

/// Server configuration.
pub const Config = struct {
    /// Port to listen on.
    port: u16 = 8317,

    /// Address to bind to.
    address: []const u8 = "127.0.0.1",
};

// ============================================================================
// HTTP Server
// ============================================================================

/// HTTP server that listens and serves requests.
pub const Server = struct {
    const Self = @This();

    config: Config,
    listener_fd: i32 = -1,

    pub fn init(config: Config) Server {
        return .{
            .config = config,
            .listener_fd = -1,
        };
    }

    pub fn deinit(self: *Self) void {
        self.stop();
    }

    /// Start the server: create socket, bind, and listen.
    pub fn listen(self: *Self) !void {
        // Create socket
        const fd = c.socket(c.AF.INET, c.SOCK.STREAM, 0);
        if (fd < 0) {
            const errno_val = std.c._errno().*;
            std.debug.print("socket failed errno={d}\n", .{errno_val});
            return error.SocketCreateFailed;
        }
        self.listener_fd = @as(i32, fd);
        errdefer {
            _ = c.close(self.listener_fd);
            self.listener_fd = -1;
        }

        // Set SO_REUSEADDR
        const one: c_int = 1;
        const so_result = c.setsockopt(self.listener_fd, c.SOL.SOCKET, c.SO.REUSEADDR, &one, @sizeOf(c_int));
        if (so_result < 0) {
            const errno_val = std.c._errno().*;
            std.debug.print("setsockopt failed errno={d}\n", .{errno_val});
            return error.SetsockoptFailed;
        }

        // Construct sockaddr_in
        var addr: c.sockaddr.in = std.mem.zeroes(c.sockaddr.in);
        addr.family = c.AF.INET;
        addr.port = std.mem.nativeToBig(u16, self.config.port);
        addr.addr = std.mem.nativeToBig(u32, parseIpAddress(self.config.address));

        const bind_result = c.bind(
            self.listener_fd,
            @ptrCast(&addr),
            @sizeOf(c.sockaddr.in),
        );
        if (bind_result < 0) {
            const errno_val = std.c._errno().*;
            std.debug.print("bind failed errno={d}\n", .{errno_val});
            return error.BindFailed;
        }

        // Listen
        const listen_result = c.listen(self.listener_fd, 128);
        if (listen_result < 0) {
            const errno_val = std.c._errno().*;
            std.debug.print("listen failed errno={d}\n", .{errno_val});
            return error.ListenFailed;
        }
    }

    /// Stop the server.
    pub fn stop(self: *Self) void {
        if (self.listener_fd >= 0) {
            _ = c.close(self.listener_fd);
            self.listener_fd = -1;
        }
    }
};

/// Default server config for loopback-only.
pub fn defaultConfig() Config {
    return Config{
        .port = 8317,
        .address = "127.0.0.1",
    };
}

/// Parse an IPv4 address string and return host-order u32.
fn parseIpAddress(addr: []const u8) u32 {
    const octets = parseIpOctets(addr);
    return (@as(u32, octets[0]) << 24) | (@as(u32, octets[1]) << 16) | (@as(u32, octets[2]) << 8) | @as(u32, octets[3]);
}

/// Parse an IPv4 address string like "127.0.0.1" into octets.
fn parseIpOctets(addr: []const u8) [4]u8 {
    var octets: [4]u8 = .{ 0, 0, 0, 0 };
    var idx: usize = 0;

    var start: usize = 0;
    for (addr, 0..) |ch, i| {
        if (ch == '.') {
            if (idx < 4) {
                const octet_str = addr[start..i];
                octets[idx] = @intCast(parseU8(octet_str));
                idx += 1;
                start = i + 1;
            }
        }
    }

    if (idx < 4) {
        const octet_str = addr[start..];
        octets[idx] = @intCast(parseU8(octet_str));
    }

    return octets;
}

/// Parse a string to u32.
fn parseU8(s: []const u8) u32 {
    var result: u32 = 0;
    for (s) |ch| {
        if (ch >= '0' and ch <= '9') {
            result = result * 10 + (ch - '0');
        }
    }
    return result;
}

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
fn acceptOneBlocking(server: *Server, state: *anyopaque) !void {
    var client_addr: c.sockaddr = undefined;
    var client_len: c.socklen_t = @sizeOf(c.sockaddr);

    const conn_fd = c.accept(server.listener_fd, &client_addr, &client_len);
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

/// Write a log record to the output and flush.
///
/// Flushes all writer types except *logging.BufferedWriter (which has no
/// flush method). All other writers reaching the else branch must provide flush().
/// This ensures NDJSON records are visible immediately.
fn writeLogRecord(out_writer: anytype, bytes: []const u8) !void {
    try out_writer.writeAll(bytes);

    // Flush if the writer supports it. This handles:
    // - Zig 0.16 BufferedWriter (e.g., Io.File.Writer with buffer)
    // - Raw file writers (flush is a no-op on unbuffered)
    // - Test writers (flush is a no-op on VoidWriter/CaptureWriter)
    //
    // Comptime branch: only the valid check is compiled based on writer type.
    if (comptime @TypeOf(out_writer) == *logging.BufferedWriter) {
        // No-op: BufferedWriter doesn't have flush, this branch is dead code
        // but compiles because the writer type is checked at comptime
    } else {
        // Writer types reaching this branch must provide flush().
        out_writer.flush() catch {};
    }
}

/// Emit startup log events after successful server listen.
fn emitStartupLogs(config: Config, out_writer: anytype) !void {
    var log_buf = logging.BufferedWriter.init();
    try logging.emit(.http_server_listening, &log_buf, &.{
        .{ .name = "bind_address", .value = logging.FieldValue{ .string = config.address } },
        .{ .name = "port", .value = logging.FieldValue{ .integer = config.port } },
    });
    try writeLogRecord(out_writer, log_buf.slice());

    // Emit UVB-76 signal ready event
    log_buf.reset();
    try logging.emit(.uvb76_signal_ready, &log_buf, &.{
        .{ .name = "signal", .value = logging.FieldValue{ .string = "🚩📻" } },
        .{ .name = "message", .value = logging.FieldValue{ .string = "Listen to UVB-76 signals..." } },
    });
    try writeLogRecord(out_writer, log_buf.slice());
}

/// Simple serve function for CLI use (no persistent state).
pub fn serve(config: Config, out_writer: anytype) !void {
    var server = Server.init(config);
    defer server.deinit();

    try server.listen();
    try emitStartupLogs(config, out_writer);
}

/// Daemon-style serve loop for production CLI use with persistent state.
pub fn serveForever(config: Config, out_writer: anytype) !void {
    var server = Server.init(config);
    defer server.deinit();

    // Initialize server state with metrics sampler
    var state = ServerState.init(std.heap.page_allocator);
    defer state.deinit();

    try server.listen();
    try emitStartupLogs(config, out_writer);

    // Structured JSON log: accept loop started
    var log_buf = logging.BufferedWriter.init();
    try logging.emit(.http_accept_loop_started, &log_buf, &.{});
    try writeLogRecord(out_writer, log_buf.slice());

    // Get opaque pointer to MetricsState for passing to route handlers.
    // handleMetrics() casts this back to *metrics_state.MetricsState.
    const state_ptr: *anyopaque = &state.metrics;

    // NOTE: Heartbeat thread is DISABLED for v0.
    // See: docs/security/accepted-risks.md (R-009)
    // Diagnostic command available: `tovarisch thread-smoke`

    // TEMPORARY DIAGNOSTIC: Heartbeat thread spawn with zero logging.
    // Set TOVARISCH_ENABLE_HEARTBEAT_THREAD_UNSAFE=1 to enable.
    // This is a diagnostic probe to isolate the serve-context spawn crash.
    // - Writes raw stderr markers to trace execution flow
    // - Avoids logging.emit entirely to test whether failure logging is the crash source
    // - Uses std.c.write directly to bypass BufferedWriter/logging path
    if (c.getenv("TOVARISCH_ENABLE_HEARTBEAT_THREAD_UNSAFE") != null) {
        const before_msg = "DIAG: before heartbeat spawn\n";
        _ = c.write(2, before_msg.ptr, before_msg.len);
        const spawn_result = std.Thread.spawn(
            .{ .stack_size = 65536 },
            heartbeat.heartbeatThread,
            .{},
        );
        const after_msg = "DIAG: after heartbeat spawn\n";
        _ = c.write(2, after_msg.ptr, after_msg.len);

        if (spawn_result) |thread| {
            const success_msg = "DIAG: heartbeat spawn succeeded, detaching\n";
            _ = c.write(2, success_msg.ptr, success_msg.len);
            thread.detach();
        } else |spawn_err| {
            // Do NOT call logging.emit here - that was the suspected panic path.
            // Write raw stderr only with no formatting.
            const err_name = @errorName(spawn_err);
            const fail_msg = "DIAG: heartbeat spawn failed, continuing without heartbeat\n";
            _ = c.write(2, fail_msg.ptr, fail_msg.len);
            _ = c.write(2, err_name.ptr, err_name.len);
            _ = c.write(2, "\n", 1);
        }
    }

    // Blocking accept loop - stays alive until interrupted.
    while (true) {
        acceptOneBlocking(&server, state_ptr) catch |err| {
            // Build log record in buffer, then write to output
            log_buf.reset();
            try logging.emit(.http_accept_loop_error, &log_buf, &.{
                .{ .name = "error", .value = logging.FieldValue{ .string = @errorName(err) } },
            });
            try writeLogRecord(out_writer, log_buf.slice());
        };
    }
}

// --- Tests ---

test "Config has sensible defaults" {
    const cfg = Config{};
    try std.testing.expectEqual(@as(u16, 8317), cfg.port);
    try std.testing.expectEqualStrings("127.0.0.1", cfg.address);
}

test "defaultConfig uses loopback" {
    const cfg = defaultConfig();
    try std.testing.expectEqual(@as(u16, 8317), cfg.port);
    try std.testing.expectEqualStrings("127.0.0.1", cfg.address);
}

test "parseIpOctets parses 127.0.0.1" {
    const octets = parseIpOctets("127.0.0.1");
    try std.testing.expect(octets[0] == 127);
    try std.testing.expect(octets[1] == 0);
    try std.testing.expect(octets[2] == 0);
    try std.testing.expect(octets[3] == 1);
}

test "parseIpAddress returns host-order u32" {
    const addr = parseIpAddress("127.0.0.1");
    try std.testing.expectEqual(@as(u32, 0x7F000001), addr);
}

test "ServerState.init creates empty sampler" {
    const allocator = std.testing.allocator;
    var state = ServerState.init(allocator);
    defer state.deinit();
    // State should initialize without error; sampler is empty
}

test "ServerState.deinit handles empty sampler" {
    const allocator = std.testing.allocator;
    var state = ServerState.init(allocator);
    // Should not panic on deinit with empty sampler
    state.deinit();
}
