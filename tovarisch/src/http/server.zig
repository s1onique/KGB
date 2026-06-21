const std = @import("std");
const c = std.c;
const routes = @import("routes.zig");
const logging = @import("../logging.zig");
const heartbeat = @import("heartbeat.zig");
const cli_args = @import("../cli/args.zig");
const statonly = @import("statonly.zig");
const bfd_status = @import("../bfd/status.zig");
const status = @import("../status.zig");
const serve_context = @import("serve_context.zig");
const tovarisch_config = @import("../config.zig");
const network_diag_config = @import("../net/network_diag_config.zig");

// Re-export ServeContext for external use
pub const ServeContext = serve_context.ServeContext;

// ============================================================================
// Server Configuration
// ============================================================================

/// Server configuration.
pub const Config = struct {
    /// Port to listen on.
    port: u16 = 8317,

    /// Address to bind to.
    address: []const u8 = "127.0.0.1",

    /// Log mode: normal or statonly.
    log_mode: cli_args.LogMode = .normal,

    /// Stats interval in seconds for statonly mode.
    stats_interval_seconds: u16 = 30,
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
fn writeLogRecord(out_writer: anytype, bytes: []const u8) !void {
    try out_writer.writeAll(bytes);

    // Flush if the writer supports it (not BufferedWriter).
    if (comptime @TypeOf(out_writer) == *logging.BufferedWriter) {
        // No-op: BufferedWriter doesn't have flush
    } else {
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

    log_buf.reset();
    try logging.emit(.uvb76_signal_ready, &log_buf, &.{
        .{ .name = "signal", .value = logging.FieldValue{ .string = "🚩📻" } },
        .{ .name = "message", .value = logging.FieldValue{ .string = "Listen to UVB-76 signals..." } },
    });
    try writeLogRecord(out_writer, log_buf.slice());
}

/// Emit startup logs only in normal mode (not statonly).
fn emitStartupLogsIfNormal(config: Config, out_writer: anytype) !void {
    if (config.log_mode == .normal) {
        try emitStartupLogs(config, out_writer);
    }
}

/// Log a critical error message.
fn logCritical(out_writer: anytype, comptime fmt: []const u8, args: anytype) void {
    out_writer.print("critical: " ++ fmt, args) catch {};
    out_writer.flush() catch {};
}

/// Simple serve function for CLI use (no persistent state).
pub fn serve(config: Config, out_writer: anytype) !void {
    var server = Server.init(config);
    defer server.deinit();

    try server.listen();
    try emitStartupLogs(config, out_writer);
}

/// Daemon-style serve loop for production CLI use with persistent state.
///
/// This function owns the ServeContext and passes it to route handlers.
/// The BFD runtime is null unless provided via config; status endpoint
/// handles null gracefully.
pub fn serveForever(config: Config, out_writer: anytype) !void {
    try serveForeverWithBfd(config, null, out_writer);
}

/// Daemon-style serve loop with optional BFD runtime for status integration.
///
/// When bfd_runtime is provided, it will be wired into the status endpoint
/// so that /status reports BFD session state.
pub fn serveForeverWithBfd(config: Config, bfd_runtime: ?*const bfd_status.BfdRuntime, out_writer: anytype) !void {
    try serveForeverWithContext(config, .{
        .bfd_runtime = bfd_runtime,
        .config_check = .no_config,
    }, out_writer);
}

/// Daemon-style serve loop with full runtime inputs for status integration.
///
/// This is the primary serve function that wires BFD runtime and config check
/// state into the status endpoint so that /status reports real serve-time state.
pub fn serveForeverWithContext(
    config: Config,
    inputs: status.RuntimeStatusInputs,
    out_writer: anytype,
) !void {
    try serveForeverWithContextAndLab(config, inputs, .{}, .{}, out_writer);
}

/// Daemon-style serve loop with full runtime inputs and lab config.
///
/// When lab_config.lab_mode is true, the /lab/probe endpoint is enabled.
pub fn serveForeverWithContextAndLab(
    config: Config,
    inputs: status.RuntimeStatusInputs,
    lab_config: tovarisch_config.LabConfig,
    network_diag_cfg: network_diag_config.NetworkDiagConfig,
    out_writer: anytype,
) !void {
    var server = Server.init(config);
    defer server.deinit();

    // Initialize serve context with full runtime inputs (BFD + config check + BGP bundle + lab config + network diag config).
    // MemoryOwnership: Startup-only one-time allocation at daemon init.
    // The ServeContext allocator is used once at serve startup, not per-request.
    // This is a single allocation that persists for daemon lifetime (acceptable).
    var serve_ctx = ServeContext.initWithContext(
        std.heap.page_allocator,
        inputs.bfd_runtime,
        inputs.config_check,
        inputs.bgp_result,
        lab_config,
        network_diag_cfg,
    );
    defer serve_ctx.deinit();

    try server.listen();

    // Emit startup logs only if not in statonly mode
    try emitStartupLogsIfNormal(config, out_writer);

    // Get opaque pointer to ServeContext for passing to route handlers.
    const ctx_ptr: *anyopaque = &serve_ctx;

    // Heartbeat thread: only spawn in normal mode.
    // In statonly mode, we skip heartbeat to keep output clean.
    var log_buf = logging.BufferedWriter.init();
    if (config.log_mode == .normal) {
        if (std.Thread.spawn(.{}, heartbeat.heartbeatThread, .{})) |thread| {
            thread.detach();
        } else |spawn_err| {
            log_buf.reset();
            logging.emit(.heartbeat_thread_start_failed, &log_buf, &.{
                .{ .name = "error", .value = logging.FieldValue{ .string = @errorName(spawn_err) } },
                .{ .name = "detail", .value = logging.FieldValue{ .string = "heartbeat spawn failed, continuing without heartbeat" } },
            }) catch {};
            writeLogRecord(out_writer, log_buf.slice()) catch {};
        }
    }

    // Branch based on log mode
    if (config.log_mode == .statonly) {
        try statonly.serveStatonlyWithStderr(server.listener_fd, ctx_ptr, config.stats_interval_seconds, out_writer);
    } else {
        try serveForeverNormal(server.listener_fd, ctx_ptr, &log_buf, out_writer);
    }
}

/// Normal mode accept loop with timeout for compatibility with statonly.
pub fn acceptOneNormal(listener_fd: i32, state: *anyopaque) !void {
    var client_addr: c.sockaddr = undefined;
    var client_len: c.socklen_t = @sizeOf(c.sockaddr);

    const conn_fd = c.accept(listener_fd, &client_addr, &client_len);
    if (conn_fd < 0) {
        const errno_val = std.c._errno().*;
        if (errno_val == 11 or errno_val == 35) {
            std.Thread.yield() catch {};
            return;
        }
        return error.AcceptFailed;
    }

    handleConnection(conn_fd, state);
}

/// Normal mode serve loop with structured JSON logging.
fn serveForeverNormal(
    listener_fd: i32,
    state_ptr: *anyopaque,
    log_buf: *logging.BufferedWriter,
    out_writer: anytype,
) !void {
    try logging.emit(.http_accept_loop_started, log_buf, &.{});
    try writeLogRecord(out_writer, log_buf.slice());

    // Blocking accept loop - stays alive until interrupted.
    while (true) {
        acceptOneNormal(listener_fd, state_ptr) catch |err| {
            log_buf.reset();
            try logging.emit(.http_accept_loop_error, log_buf, &.{
                .{ .name = "error", .value = logging.FieldValue{ .string = @errorName(err) } },
            });
            try writeLogRecord(out_writer, log_buf.slice());
        };
    }
}

