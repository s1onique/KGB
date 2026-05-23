const std = @import("std");
const c = std.c;
const routes = @import("routes.zig");

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
    /// Does NOT enter the accept loop - that is handled by pollOnce.
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

        // Construct sockaddr_in using standard nested struct from c.sockaddr
        // c.sockaddr.in is the correct cross-platform approach for both Linux and macOS
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

/// Simple serve function for CLI use.
/// Creates a server with the given config and listens until interrupted.
pub fn serve(config: Config) !void {
    var server = Server.init(config);
    defer server.deinit();

    try server.listen();
    std.debug.print("Listening on {s}:{d}\n", .{ config.address, config.port });
}

/// Accept one connection and handle it (blocking).
/// This is the simple accept loop for production CLI use.
fn acceptOneBlocking(server: *Server) !void {
    var client_addr: c.sockaddr = undefined;
    var client_len: c.socklen_t = @sizeOf(c.sockaddr);

    const conn_fd = c.accept(server.listener_fd, &client_addr, &client_len);
    if (conn_fd < 0) {
        const errno_val = std.c._errno().*;
        // EAGAIN (11 on Linux, 35 on macOS) means no connection ready yet
        // EWOULDBLOCK is the same value on most platforms
        if (errno_val == 11 or errno_val == 35) {
            std.Thread.yield() catch {};
            return;
        }
        // For other errors, just retry
        std.Thread.yield() catch {};
        return;
    }

    handleConnection(conn_fd);
}

/// Daemon-style serve loop for production CLI use.
/// This is the correct CLI entry point - the process stays alive
/// until interrupted by a signal.
pub fn serveForever(config: Config) !void {
    var server = Server.init(config);
    defer server.deinit();

    try server.listen();
    std.debug.print("Listening on {s}:{d}\n", .{ config.address, config.port });
    std.debug.print("Listen to UVB-76 signals...\n", .{});
    std.debug.print("Entering accept loop\n", .{});

    // Blocking accept loop - stays alive until interrupted.
    // This is the correct daemon behavior.
    while (true) {
        acceptOneBlocking(&server) catch |err| {
            std.debug.print("accept loop error: {}\n", .{err});
        };
    }
}

/// Parse an IPv4 address string and return host-order u32.
/// Callers must convert to network byte order via nativeToBig before use.
fn parseIpAddress(addr: []const u8) u32 {
    const octets = parseIpOctets(addr);
    // Host byte order: first octet is MSB
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

    // Parse the last octet
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
        // Invalid request, just close
        return;
    };

    // Route and handle the request
    _ = routes.routeRequestFd(conn_fd, req) catch return;
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
    // 127.0.0.1 in host byte order: 127 << 24 | 0 << 16 | 0 << 8 | 1 = 0x7F000001
    // Callers must use nativeToBig for network byte order storage.
    const addr = parseIpAddress("127.0.0.1");
    try std.testing.expectEqual(@as(u32, 0x7F000001), addr);
}


