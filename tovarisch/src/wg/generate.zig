// wg/generate.zig — WireGuard config file generation
//
// Generates WireGuard server config files from WgConfig.
// Generates client config files for each enabled peer.
// This ACT generates files only - runtime mutation is out of scope.
//
// Key validation is delegated to peer.validateKey() to ensure
// consistent handling of padded keys (wg pubkey output).

const std = @import("std");
const config = @import("../config.zig");
const wg_config = @import("config.zig");
const peer = @import("peer.zig");

/// Errors that can occur during WireGuard config generation.
pub const GenerateError = error{
    /// Private key file does not exist or is not readable.
    PrivateKeyNotFound,
    /// Private key file has invalid content (not 44 base64 chars).
    InvalidPrivateKey,
    /// Public key file does not exist or is not readable.
    PublicKeyNotFound,
    /// Public key file has invalid content (not 44 base64 chars).
    InvalidPublicKey,
    /// Output directory cannot be created.
    OutputDirCreateFailed,
    /// Output file cannot be written.
    OutputFileWriteFailed,
    /// Output file permissions cannot be set.
    OutputPermissionFailed,
    /// Path too long for C APIs.
    PathTooLong,
    /// Out of memory.
    OutOfMemory,
};

/// Result of successful server config generation.
pub const ServerGenerateResult = struct {
    /// The generated interface name.
    interface: []const u8,
    /// The address in CIDR notation.
    address: []const u8,
    /// The UDP listen port.
    listen_port: u16,
    /// The path to the generated config file.
    output_path: []const u8,
    /// Number of peers added to the server config.
    peer_count: usize,

    /// Free resources owned by this result.
    /// Call this when done using the result.
    pub fn deinit(self: *const ServerGenerateResult, allocator: std.mem.Allocator) void {
        allocator.free(self.output_path);
    }
};

/// Result of successful client config generation for a single peer.
pub const ClientGenerateResult = struct {
    /// The peer name.
    peer_name: []const u8,
    /// The path to the generated client config file.
    output_path: []const u8,

    /// Free resources owned by this result.
    /// Call this when done using the result.
    pub fn deinit(self: *const ClientGenerateResult, allocator: std.mem.Allocator) void {
        allocator.free(self.output_path);
    }
};

/// Convert a Zig slice to a null-terminated C string for C path APIs.
fn toCString(path: []const u8, buf: *[4096]u8) GenerateError![*:0]const u8 {
    if (path.len >= buf.len) return GenerateError.PathTooLong;
    @memcpy(buf[0..path.len], path);
    buf[path.len] = 0;
    return @as([*:0]const u8, @ptrCast(buf));
}

/// Read the private key from a file.
/// Returns the key as a string slice (not null-terminated).
/// The key must be a valid WireGuard private key (44 base64 characters).
pub fn readPrivateKey(key_path: []const u8, allocator: std.mem.Allocator) GenerateError![]const u8 {
    var path_buf: [4096]u8 = undefined;
    const c_path = toCString(key_path, &path_buf) catch return GenerateError.PathTooLong;

    const fd = std.c.open(c_path, @bitCast(@as(u32, 0)));
    if (fd < 0) {
        return GenerateError.PrivateKeyNotFound;
    }
    defer _ = std.c.close(fd);

    // Read the key (WireGuard private keys are 44 base64 chars + newline).
    var key_buf: [64]u8 = undefined;
    const bytes_read = std.c.read(fd, &key_buf, key_buf.len);
    if (bytes_read < 0) {
        return GenerateError.PrivateKeyNotFound;
    }

    // Trim whitespace
    const key = std.mem.trim(u8, key_buf[0..@as(usize, @intCast(bytes_read))], " \t\r\n");

    // Validate key length (WireGuard private keys are 44 base64 chars)
    if (key.len != 44) {
        return GenerateError.InvalidPrivateKey;
    }

    // Duplicate the key for the caller (caller owns the memory)
    const owned_key = allocator.dupe(u8, key) catch return GenerateError.OutOfMemory;
    return owned_key;
}

/// Read the public key from a file.
/// Returns the key as a string slice (not null-terminated).
/// The key must be a valid WireGuard public key (44 base64 characters, with optional padding).
/// Uses peer.validateKey() for consistent validation with wg pubkey output.
pub fn readPublicKey(key_path: []const u8, allocator: std.mem.Allocator) GenerateError![]const u8 {
    var path_buf: [4096]u8 = undefined;
    const c_path = toCString(key_path, &path_buf) catch return GenerateError.PathTooLong;

    const fd = std.c.open(c_path, @bitCast(@as(u32, 0)));
    if (fd < 0) {
        return GenerateError.PublicKeyNotFound;
    }
    defer _ = std.c.close(fd);

    // Read the key (WireGuard public keys are 44 base64 chars + newline, optionally with padding).
    var key_buf: [64]u8 = undefined;
    const bytes_read = std.c.read(fd, &key_buf, key_buf.len);
    if (bytes_read < 0) {
        return GenerateError.PublicKeyNotFound;
    }

    // Trim whitespace only (not padding)
    const key = std.mem.trim(u8, key_buf[0..@as(usize, @intCast(bytes_read))], " \t\r\n");

    // Validate using shared validator (handles padded keys from wg pubkey)
    peer.validateKey(key) catch |e| {
        switch (e) {
            peer.PeerConfigError.InvalidKey => return GenerateError.InvalidPublicKey,
            else => return GenerateError.InvalidPublicKey,
        }
    };

    // Duplicate the key for the caller (caller owns the memory)
    const owned_key = allocator.dupe(u8, key) catch return GenerateError.OutOfMemory;
    return owned_key;
}

/// Create the output directory recursively with mode 0700 (owner read/write/execute only).
/// Creates all parent directories as needed.
fn createOutputDir(output_dir: []const u8) GenerateError!void {
    // Iterate through the path and create each directory component
    // No trailing slashes - we slice to end of each component
    var i: usize = 0;
    while (i < output_dir.len) {
        // Find next path separator
        const remaining = output_dir[i..];
        const sep = std.mem.indexOfScalar(u8, remaining, '/');

        // Calculate end of this component (no trailing slash)
        const end = if (sep) |s| i + s else output_dir.len;

        // Skip empty components and the root "/" itself
        if (end > i + 1) {
            const component = output_dir[0..end];

            // Convert to null-terminated C string
            var path_buf: [4096]u8 = undefined;
            const c_path = toCString(component, &path_buf) catch return GenerateError.PathTooLong;

            // Try to create directory with 0700 permissions
            const result = std.c.mkdir(c_path, 0o700);
            if (result < 0) {
                const errno = std.c._errno().*;
                const e_exist = @intFromEnum(std.c.E.EXIST);

                if (errno != e_exist) {
                    return GenerateError.OutputDirCreateFailed;
                }
                // EEXIST is fine - directory already exists
            }
        }

        // Advance past this component
        if (sep) |_| {
            i = end + 1;
        } else {
            break;
        }
    }
}

/// Write the WireGuard config file with mode 0600 (owner read/write only).
fn writeConfigFile(output_path: []const u8, content: []const u8) GenerateError!void {
    var path_buf: [4096]u8 = undefined;
    const c_path = toCString(output_path, &path_buf) catch return GenerateError.PathTooLong;

    // Open file for writing, create if not exists, truncate if exists
    // Use portable std.c.O struct instead of platform-specific magic constants
    const open_flags = std.c.O{
        .ACCMODE = std.posix.ACCMODE.WRONLY,
        .CREAT = true,
        .TRUNC = true,
    };
    const fd = std.c.open(c_path, open_flags, @as(c_uint, 0o600));
    if (fd < 0) {
        return GenerateError.OutputFileWriteFailed;
    }
    defer _ = std.c.close(fd);

    // Write content
    const bytes_written = std.c.write(fd, content.ptr, content.len);
    if (bytes_written < 0 or @as(usize, @intCast(bytes_written)) != content.len) {
        return GenerateError.OutputFileWriteFailed;
    }

    // Set permissions to 0600 (owner read/write only)
    const perm_result = std.c.fchmod(fd, 0o600);
    if (perm_result < 0) {
        return GenerateError.OutputPermissionFailed;
    }
}

/// Generate a WireGuard server config file from WgConfig.
/// Adds Peer blocks for each enabled peer.
/// Returns a ServerGenerateResult with safe summary fields (no secrets).
/// Does NOT log the private key.
pub fn generateServerConfig(
    cfg: wg_config.WgConfig,
    peers: []const wg_config.WgPeer,
    allocator: std.mem.Allocator,
) GenerateError!ServerGenerateResult {
    // Read the private key
    const private_key = readPrivateKey(cfg.private_key_file, allocator) catch |e| return e;
    defer allocator.free(private_key);

    // Build the output path: output_dir/interface.conf
    const output_path = std.fmt.allocPrint(
        allocator,
        "{s}/{s}.conf",
        .{ cfg.output_dir, cfg.interface },
    ) catch return GenerateError.OutOfMemory;
    errdefer allocator.free(output_path);

    // Create output directory
    try createOutputDir(cfg.output_dir);

    // Build the WireGuard config content
    var content = std.ArrayList(u8).empty;
    defer content.deinit(allocator);

    // [Interface] section
    try content.appendSlice(allocator, "[Interface]\n");
    try content.appendSlice(allocator, "Address = ");
    try content.appendSlice(allocator, cfg.address);
    try content.append(allocator, '\n');
    try content.appendSlice(allocator, "ListenPort = ");
    try content.print(allocator, "{d}\n", .{cfg.listen_port});
    try content.appendSlice(allocator, "PrivateKey = ");
    try content.appendSlice(allocator, private_key);
    try content.append(allocator, '\n');
    try content.appendSlice(allocator, "SaveConfig = false\n");

    // [Peer] sections for enabled peers
    var peer_count: usize = 0;
    for (peers) |p| {
        if (!p.enabled) continue;

        // Read the peer's public key - fail if missing or invalid
        const peer_pub_key = try readPublicKey(p.public_key_file, allocator);
        defer allocator.free(peer_pub_key);

        try content.append(allocator, '\n');
        try content.appendSlice(allocator, "[Peer]\n");
        try content.appendSlice(allocator, "# ");
        try content.appendSlice(allocator, p.name);
        try content.append(allocator, '\n');
        try content.appendSlice(allocator, "PublicKey = ");
        try content.appendSlice(allocator, peer_pub_key);
        try content.append(allocator, '\n');
        try content.appendSlice(allocator, "AllowedIPs = ");
        try content.appendSlice(allocator, p.allowed_ips);
        try content.append(allocator, '\n');

        peer_count += 1;
    }

    // Write the config file
    try writeConfigFile(output_path, content.items);

    return ServerGenerateResult{
        .interface = cfg.interface,
        .address = cfg.address,
        .listen_port = cfg.listen_port,
        .output_path = output_path,
        .peer_count = peer_count,
    };
}

/// Generate a WireGuard client config file for a single peer.
/// Uses the peer's private key and the server's public key.
/// Returns a ClientGenerateResult with safe summary fields (no secrets).
/// Does NOT log any private keys.
pub fn generateClientConfig(
    cfg: wg_config.WgConfig,
    peer_config: *const wg_config.WgPeer,
    allocator: std.mem.Allocator,
) GenerateError!ClientGenerateResult {
    // Read the peer's private key
    const peer_private_key = readPrivateKey(peer_config.private_key_file, allocator) catch |e| return e;
    defer allocator.free(peer_private_key);

    // Read the server's public key (required for client config)
    const server_public_key = readPublicKey(cfg.public_key_file, allocator) catch |e| return e;
    defer allocator.free(server_public_key);

    // Build the output path
    const output_path = try allocator.dupe(u8, peer_config.client_output_file);
    errdefer allocator.free(output_path);

    // Create output directory (extract parent directory)
    const parent_dir = std.fs.path.dirname(peer_config.client_output_file) orelse ".";
    try createOutputDir(parent_dir);

    // Build the WireGuard client config content
    var content = std.ArrayList(u8).empty;
    defer content.deinit(allocator);

    // [Interface] section
    try content.appendSlice(allocator, "[Interface]\n");
    try content.appendSlice(allocator, "Address = ");
    try content.appendSlice(allocator, peer_config.address);
    try content.append(allocator, '\n');
    try content.appendSlice(allocator, "PrivateKey = ");
    try content.appendSlice(allocator, peer_private_key);
    try content.append(allocator, '\n');

    // [Peer] section
    try content.append(allocator, '\n');
    try content.appendSlice(allocator, "[Peer]\n");
    try content.appendSlice(allocator, "PublicKey = ");
    try content.appendSlice(allocator, server_public_key);
    try content.append(allocator, '\n');
    try content.appendSlice(allocator, "AllowedIPs = ");
    try content.appendSlice(allocator, cfg.client_allowed_ips);
    try content.append(allocator, '\n');

    // Endpoint (optional)
    if (peer_config.endpoint) |endpoint| {
        try content.appendSlice(allocator, "Endpoint = ");
        try content.appendSlice(allocator, endpoint);
        try content.append(allocator, '\n');
    }

    // PersistentKeepalive (optional)
    if (peer_config.persistent_keepalive) |keepalive| {
        if (keepalive > 0) {
            try content.appendSlice(allocator, "PersistentKeepalive = ");
            try content.print(allocator, "{d}\n", .{keepalive});
        }
    }

    // Write the config file
    try writeConfigFile(output_path, content.items);

    return ClientGenerateResult{
        .peer_name = peer_config.name,
        .output_path = output_path,
    };
}
