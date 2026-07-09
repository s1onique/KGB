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
const lab_events = @import("../runtime/lab_events.zig");
const startup_logs = @import("startup_logs.zig");
const startup_trace = @import("../startup_trace.zig");
const serve_startup = @import("serve_startup.zig");

// Re-export for external callers
pub const serveForeverNormalWithTracer = serve_startup.serveForeverNormalWithTracer;

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
    try startup_logs.emitStartupLogs(config.port, config.address, out_writer);
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
/// When lab_config.native_events_enabled is true, native events are emitted
/// from real runtime paths (heartbeat, WG, BGP, BFD) and exposed via /status.json.
pub fn serveForeverWithContextAndLab(
    config: Config,
    inputs: status.RuntimeStatusInputs,
    lab_config: tovarisch_config.LabConfig,
    network_diag_cfg: network_diag_config.NetworkDiagConfig,
    out_writer: anytype,
) !void {
    try serveForeverWithContextAndLabInternal(config, inputs, lab_config, network_diag_cfg, out_writer, null);
}

/// Daemon-style serve loop with full runtime inputs, lab config, and startup tracer.
///
/// This function wraps serveForeverWithContextAndLab and adds startup phase timing.
/// It emits startup_ready after the HTTP accept loop starts, completing the startup
/// phase trace from daemon_command.zig.
///
/// The tracer parameter may be null for backward compatibility.
pub fn serveForeverWithContextAndLabAndTracer(
    config: Config,
    inputs: status.RuntimeStatusInputs,
    lab_config: tovarisch_config.LabConfig,
    network_diag_cfg: network_diag_config.NetworkDiagConfig,
    out_writer: anytype,
    tracer: ?*startup_trace.StartupTracer,
) !void {
    try serveForeverWithContextAndLabInternal(config, inputs, lab_config, network_diag_cfg, out_writer, tracer);
}

/// Internal serve function that optionally emits startup_ready.
fn serveForeverWithContextAndLabInternal(
    config: Config,
    inputs: status.RuntimeStatusInputs,
    lab_config: tovarisch_config.LabConfig,
    network_diag_cfg: network_diag_config.NetworkDiagConfig,
    out_writer: anytype,
    tracer: ?*startup_trace.StartupTracer,
) !void {
    // Initialize lab event emitter if native events are enabled.
    // The emitter is owned by this function and deinitialized via defer.
    // Uses std.time.monoTime() internally for real elapsed time measurement.
    var lab_emitter_opt: ?*lab_events.LabEventEmitter = null;
    var lab_emitter_storage: lab_events.LabEventEmitter = undefined;
    if (lab_config.native_events_enabled) {
        const emitter_config = lab_events.LabEventsConfig{
            .enabled = true,
            .output_path = lab_config.native_events_path,
        };
        lab_emitter_storage = lab_events.LabEventEmitter.init(emitter_config);
        lab_emitter_opt = &lab_emitter_storage;

        // Emit startup diagnostics for native events.
        // This makes file-open success/failure visible in logs.
        var lab_log_buf = logging.BufferedWriter.init();
        if (lab_emitter_storage.output_file != null) {
            // File opened successfully - log with file_output=opened
            logging.emit(.lab_native_events_enabled, &lab_log_buf, &.{
                .{ .name = "path", .value = logging.FieldValue{ .string = lab_config.native_events_path } },
                .{ .name = "file_output", .value = logging.FieldValue{ .string = "opened" } },
                .{ .name = "detail", .value = logging.FieldValue{ .string = "native event timeline enabled" } },
            }) catch {};
            out_writer.writeAll(lab_log_buf.slice()) catch {};
        } else if (lab_config.native_events_path.len > 0) {
            // File open failed but path was provided - log error
            logging.emit(.lab_native_events_open_failed, &lab_log_buf, &.{
                .{ .name = "path", .value = logging.FieldValue{ .string = lab_config.native_events_path } },
                .{ .name = "error", .value = logging.FieldValue{ .string = "file_open_failed" } },
                .{ .name = "detail", .value = logging.FieldValue{ .string = "native events enabled but file could not be opened" } },
            }) catch {};
            out_writer.writeAll(lab_log_buf.slice()) catch {};
        }
    }
    defer {
        if (lab_emitter_opt) |emitter| {
            emitter.deinit();
        }
    }

    // Phase: http_bind — HTTP server socket creation and bind
    // Guard ends before continuing to accept loop.
    if (tracer) |t| {
        var guard = t.begin(.http_bind);
        try guard.emitStarted(out_writer);
        var server = Server.init(config);
        try server.listen();
        try guard.finish(out_writer);
        try serve_startup.serveForeverAfterBind(&server, config, inputs, lab_config, network_diag_cfg, out_writer, lab_emitter_opt, t);
        return;
    } else {
        var server = Server.init(config);
        try server.listen();
        try serve_startup.serveForeverAfterBind(&server, config, inputs, lab_config, network_diag_cfg, out_writer, lab_emitter_opt, null);
        return;
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

