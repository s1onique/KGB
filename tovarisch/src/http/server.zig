const std = @import("std");
const c = std.c;
const routes = @import("routes.zig");
const logging = @import("../logging.zig");

/// Server configuration.
pub const Config = struct {
    /// Port to listen on.
    port: u16 = 8317,

    /// Address to bind to.
    address: []const u8 = "127.0.0.1",
};

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

/// Handle a single connection.
fn handleConnection(conn_fd: i32) void {
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

    // Route and handle the request
    _ = routes.routeRequestFd(conn_fd, req) catch return;
}

/// Accept one connection and handle it (blocking).
/// Returns error.AcceptFailed for non-transient failures.
/// EAGAIN/EWOULDBLOCK are treated as transient and return successfully.
fn acceptOneBlocking(server: *Server) !void {
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

    handleConnection(conn_fd);
}

/// Emit startup log events after successful server listen.
fn emitStartupLogs(config: Config, out_writer: anytype) !void {
    var log_buf = logging.BufferedWriter.init();
    try logging.emit(.http_server_listening, &log_buf, &.{
        .{ .name = "bind_address", .value = logging.FieldValue{ .string = config.address } },
        .{ .name = "port", .value = logging.FieldValue{ .integer = config.port } },
    });
    try out_writer.writeAll(log_buf.slice());

    // Emit UVB-76 signal ready event
    log_buf.reset();
    try logging.emit(.uvb76_signal_ready, &log_buf, &.{
        .{ .name = "signal", .value = logging.FieldValue{ .string = "🚩📻" } },
        .{ .name = "message", .value = logging.FieldValue{ .string = "Listen to UVB-76 signals..." } },
    });
    try out_writer.writeAll(log_buf.slice());
}

/// Simple serve function for CLI use.
pub fn serve(config: Config, out_writer: anytype) !void {
    var server = Server.init(config);
    defer server.deinit();

    try server.listen();
    try emitStartupLogs(config, out_writer);
}

/// Daemon-style serve loop for production CLI use.
pub fn serveForever(config: Config, out_writer: anytype) !void {
    var server = Server.init(config);
    defer server.deinit();

    try server.listen();
    try emitStartupLogs(config, out_writer);

    // Structured JSON log: accept loop started
    var log_buf = logging.BufferedWriter.init();
    try logging.emit(.http_accept_loop_started, &log_buf, &.{});
    try out_writer.writeAll(log_buf.slice());

    // Blocking accept loop - stays alive until interrupted.
    while (true) {
        acceptOneBlocking(&server) catch |err| {
            // Build log record in buffer, then write to output
            log_buf.reset();
            try logging.emit(.http_accept_loop_error, &log_buf, &.{
                .{ .name = "error", .value = logging.FieldValue{ .string = @errorName(err) } },
            });
            try out_writer.writeAll(log_buf.slice());
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
