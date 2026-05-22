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

/// Manual IPv4 sockaddr_in structure for binding.
/// This works across platforms including macOS.
const SockaddrIn = extern struct {
    sin_len: u8 = @sizeOf(SockaddrIn),
    sin_family: u8,
    sin_port: u16,
    sin_addr: u32,
    sin_zero: [8]u8 = @splat(0),
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

    /// Start the server and listen for connections.
    /// This function blocks until stop() is called.
    pub fn listen(self: *Self) !void {
        // Create socket
        const fd = c.socket(c.AF.INET, c.SOCK.STREAM, 0);
        if (fd < 0) return error.SocketCreateFailed;
        self.listener_fd = @as(i32, fd);
        errdefer { _ = c.close(self.listener_fd); self.listener_fd = -1; }

        // Set SO_REUSEADDR
        const one: c_int = 1;
        const so_result = c.setsockopt(self.listener_fd, c.SOL.SOCKET, c.SO.REUSEADDR, &one, @sizeOf(c_int));
        if (so_result < 0) return error.SetsockoptFailed;

        // Construct sockaddr_in
        var addr = SockaddrIn{
            .sin_len = @sizeOf(SockaddrIn),
            .sin_family = c.AF.INET,
            .sin_port = @byteSwap(self.config.port),
            .sin_addr = parseIpAddress(self.config.address),
        };
        
        const bind_result = c.bind(self.listener_fd, @as(*c.sockaddr, @ptrFromInt(@intFromPtr(&addr))), @sizeOf(SockaddrIn));
        if (bind_result < 0) return error.BindFailed;

        // Listen
        const listen_result = c.listen(self.listener_fd, 128);
        if (listen_result < 0) return error.ListenFailed;

        // Accept connections
        while (true) {
            var client_addr: c.sockaddr = undefined;
            var client_len: c.socklen_t = @sizeOf(c.sockaddr);

            const conn_fd = c.accept(self.listener_fd, &client_addr, &client_len);
            if (conn_fd < 0) {
                // No connection ready, yield to allow other work
                std.Thread.yield() catch {};
                continue;
            }

            handleConnection(conn_fd);
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

    std.debug.print("Listening on {s}:{d}\n", .{ config.address, config.port });

    try server.listen();
}

/// Parse an IPv4 address string and return network byte order u32.
fn parseIpAddress(addr: []const u8) u32 {
    const octets = parseIpOctets(addr);
    // Network byte order: MSB first
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

test "parseIpAddress returns network byte order" {
    // 127.0.0.1 in network byte order (big-endian): MSB first
    // 127 = 0x7F, so 127.0.0.1 = 0x7F000001 = 2130706433
    const addr = parseIpAddress("127.0.0.1");
    try std.testing.expectEqual(@as(u32, 0x7F000001), addr);
}
