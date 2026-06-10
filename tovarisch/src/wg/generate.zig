// wg/generate.zig — WireGuard config file generation
//
// Generates WireGuard server config files from WgConfig.
// This ACT generates files only - runtime mutation is out of scope.

const std = @import("std");
const config = @import("../config.zig");
const wg_config = @import("config.zig");

/// Errors that can occur during WireGuard config generation.
pub const GenerateError = error{
    /// Private key file does not exist or is not readable.
    PrivateKeyNotFound,
    /// Private key file has invalid content (not 44 base64 chars).
    InvalidPrivateKey,
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

/// Result of successful config generation.
pub const GenerateResult = struct {
    /// The generated interface name.
    interface: []const u8,
    /// The address in CIDR notation.
    address: []const u8,
    /// The UDP listen port.
    listen_port: u16,
    /// The path to the generated config file.
    output_path: []const u8,

    /// Free resources owned by this result.
    /// Call this when done using the result.
    pub fn deinit(self: *const GenerateResult, allocator: std.mem.Allocator) void {
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

    // Get file size
    var stat_buf: std.c.Stat = undefined;
    const stat_result = std.c.fstat(fd, &stat_buf);
    if (stat_result < 0) {
        return GenerateError.PrivateKeyNotFound;
    }

    // Read the key (WireGuard private keys are 44 base64 chars + newline)
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

/// Create the output directory with mode 0700 (owner read/write/execute only).
fn createOutputDir(output_dir: []const u8) GenerateError!void {
    var path_buf: [4096]u8 = undefined;
    const c_path = toCString(output_dir, &path_buf) catch return GenerateError.PathTooLong;

    // Try to create directory with 0700 permissions
    const result = std.c.mkdir(c_path, 0o700);
    if (result < 0) {
        const errno = std.c._errno().*;
        const e_exist = @intFromEnum(std.c.E.EXIST);
        const e_noent = @intFromEnum(std.c.E.NOENT);

        if (errno == e_exist) {
            // Directory already exists - that's fine
            return;
        }
        if (errno == e_noent) {
            // Parent directory doesn't exist - try to create parent dirs
            return GenerateError.OutputDirCreateFailed;
        }
        // Other error
        return GenerateError.OutputDirCreateFailed;
    }
}

/// Write the WireGuard config file with mode 0600 (owner read/write only).
fn writeConfigFile(output_path: []const u8, content: []const u8) GenerateError!void {
    var path_buf: [4096]u8 = undefined;
    const c_path = toCString(output_path, &path_buf) catch return GenerateError.PathTooLong;

    // Open file for writing, create if not exists, truncate if exists
    const open_flags: std.c.O = @bitCast(@as(u32, 1) | @as(u32, 0x0200) | @as(u32, 0x0400));
    const fd = std.c.open(c_path, open_flags, @as(u32, 0o600));
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
/// Returns a GenerateResult with safe summary fields (no secrets).
/// Does NOT log the private key.
pub fn generateConfig(
    cfg: wg_config.WgConfig,
    allocator: std.mem.Allocator,
) GenerateError!GenerateResult {
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

    // Write the config file
    try writeConfigFile(output_path, content.items);

    return GenerateResult{
        .interface = cfg.interface,
        .address = cfg.address,
        .listen_port = cfg.listen_port,
        .output_path = output_path,
    };
}

// --- Tests ---

test "readPrivateKey rejects non-existent file" {
    const result = readPrivateKey("/nonexistent/path/to/key", std.heap.page_allocator);
    try std.testing.expectError(GenerateError.PrivateKeyNotFound, result);
}

test "readPrivateKey rejects invalid key length" {
    // Create a temp file with invalid key content using C API
    const tmp_path = "/tmp/wg-test-key-invalid";
    defer _ = std.c.unlink(tmp_path.ptr);

    var path_buf: [256]u8 = undefined;
    @memcpy(path_buf[0..tmp_path.len], tmp_path);
    path_buf[tmp_path.len] = 0;
    const c_path: [*:0]const u8 = @ptrCast(&path_buf);

    // Create and write to the file
    const fd = std.c.open(c_path, @bitCast(@as(u32, 0x0201)), @as(u32, 0o600)); // O_WRONLY | O_CREAT
    if (fd >= 0) {
        defer _ = std.c.close(fd);
        const content = "short";
        _ = std.c.write(fd, content.ptr, content.len);
    }

    const result = readPrivateKey(tmp_path, std.heap.page_allocator);
    try std.testing.expectError(GenerateError.InvalidPrivateKey, result);
}
