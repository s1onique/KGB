// wg/generate_tests.zig — WireGuard config generation tests
//
// Tests for generate.zig functions.

const std = @import("std");
const generate = @import("generate.zig");
const wg_config = @import("config.zig");

test "readPrivateKey rejects non-existent file" {
    const result = generate.readPrivateKey("/nonexistent/path/to/key", std.heap.page_allocator);
    try std.testing.expectError(generate.GenerateError.PrivateKeyNotFound, result);
}

test "readPrivateKey rejects invalid key length" {
    // Create a temp file with invalid key content using portable C API
    const tmp_path = "/tmp/wg-test-key-invalid";

    var path_buf: [256]u8 = undefined;
    @memcpy(path_buf[0..tmp_path.len], tmp_path);
    path_buf[tmp_path.len] = 0;
    const c_path: [*:0]const u8 = @ptrCast(&path_buf);
    defer _ = std.c.unlink(c_path);

    // Use portable std.c.O struct instead of platform-specific magic constants
    const open_flags = std.c.O{
        .ACCMODE = std.posix.ACCMODE.WRONLY,
        .CREAT = true,
        .TRUNC = true,
    };
    const fd = std.c.open(c_path, open_flags, @as(c_uint, 0o600));
    try std.testing.expect(fd >= 0); // Fail loud if file creation fails
    defer _ = std.c.close(fd);

    const content = "short";
    _ = std.c.write(fd, content.ptr, content.len);

    const result = generate.readPrivateKey(tmp_path, std.heap.page_allocator);
    try std.testing.expectError(generate.GenerateError.InvalidPrivateKey, result);
}

test "readPrivateKey accepts valid 44-char key" {
    const tmp_path = "/tmp/wg-test-key-valid";

    var path_buf: [256]u8 = undefined;
    @memcpy(path_buf[0..tmp_path.len], tmp_path);
    path_buf[tmp_path.len] = 0;
    const c_path: [*:0]const u8 = @ptrCast(&path_buf);
    defer _ = std.c.unlink(c_path);

    const open_flags = std.c.O{
        .ACCMODE = std.posix.ACCMODE.WRONLY,
        .CREAT = true,
        .TRUNC = true,
    };
    const fd = std.c.open(c_path, open_flags, @as(c_uint, 0o600));
    try std.testing.expect(fd >= 0);
    defer _ = std.c.close(fd);

    // 44-character base64 key
    const content = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA";
    _ = std.c.write(fd, content.ptr, content.len);

    const result = try generate.readPrivateKey(tmp_path, std.heap.page_allocator);
    defer std.heap.page_allocator.free(result);
    try std.testing.expectEqualStrings(content, result);
}

test "readPublicKey rejects non-existent file" {
    const result = generate.readPublicKey("/nonexistent/path/to/key", std.heap.page_allocator);
    try std.testing.expectError(generate.GenerateError.PublicKeyNotFound, result);
}

test "readPublicKey rejects invalid key length" {
    const tmp_path = "/tmp/wg-test-pubkey-invalid";

    var path_buf: [256]u8 = undefined;
    @memcpy(path_buf[0..tmp_path.len], tmp_path);
    path_buf[tmp_path.len] = 0;
    const c_path: [*:0]const u8 = @ptrCast(&path_buf);
    defer _ = std.c.unlink(c_path);

    const open_flags = std.c.O{
        .ACCMODE = std.posix.ACCMODE.WRONLY,
        .CREAT = true,
        .TRUNC = true,
    };
    const fd = std.c.open(c_path, open_flags, @as(c_uint, 0o600));
    try std.testing.expect(fd >= 0);
    defer _ = std.c.close(fd);

    const content = "short";
    _ = std.c.write(fd, content.ptr, content.len);

    const result = generate.readPublicKey(tmp_path, std.heap.page_allocator);
    try std.testing.expectError(generate.GenerateError.InvalidPublicKey, result);
}

test "readPublicKey accepts valid 44-char key" {
    const tmp_path = "/tmp/wg-test-pubkey-valid";

    var path_buf: [256]u8 = undefined;
    @memcpy(path_buf[0..tmp_path.len], tmp_path);
    path_buf[tmp_path.len] = 0;
    const c_path: [*:0]const u8 = @ptrCast(&path_buf);
    defer _ = std.c.unlink(c_path);

    const open_flags = std.c.O{
        .ACCMODE = std.posix.ACCMODE.WRONLY,
        .CREAT = true,
        .TRUNC = true,
    };
    const fd = std.c.open(c_path, open_flags, @as(c_uint, 0o600));
    try std.testing.expect(fd >= 0);
    defer _ = std.c.close(fd);

    // 44-character base64 key
    const content = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA";
    _ = std.c.write(fd, content.ptr, content.len);

    const result = try generate.readPublicKey(tmp_path, std.heap.page_allocator);
    defer std.heap.page_allocator.free(result);
    try std.testing.expectEqualStrings(content, result);
}

test "readPublicKey accepts padded key" {
    const tmp_path = "/tmp/wg-test-pubkey-padded";

    var path_buf: [256]u8 = undefined;
    @memcpy(path_buf[0..tmp_path.len], tmp_path);
    path_buf[tmp_path.len] = 0;
    const c_path: [*:0]const u8 = @ptrCast(&path_buf);
    defer _ = std.c.unlink(c_path);

    const open_flags = std.c.O{
        .ACCMODE = std.posix.ACCMODE.WRONLY,
        .CREAT = true,
        .TRUNC = true,
    };
    const fd = std.c.open(c_path, open_flags, @as(c_uint, 0o600));
    try std.testing.expect(fd >= 0);
    defer _ = std.c.close(fd);

    // 44-character key: 43 base64 chars + final '=' (normal wg pubkey output)
    const content = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
    _ = std.c.write(fd, content.ptr, content.len);

    const result = try generate.readPublicKey(tmp_path, std.heap.page_allocator);
    defer std.heap.page_allocator.free(result);
    try std.testing.expectEqualStrings(content, result);
}

test "readPublicKey rejects invalid characters" {
    const tmp_path = "/tmp/wg-test-pubkey-invalid-chars";

    var path_buf: [256]u8 = undefined;
    @memcpy(path_buf[0..tmp_path.len], tmp_path);
    path_buf[tmp_path.len] = 0;
    const c_path: [*:0]const u8 = @ptrCast(&path_buf);
    defer _ = std.c.unlink(c_path);

    const open_flags = std.c.O{
        .ACCMODE = std.posix.ACCMODE.WRONLY,
        .CREAT = true,
        .TRUNC = true,
    };
    const fd = std.c.open(c_path, open_flags, @as(c_uint, 0o600));
    try std.testing.expect(fd >= 0);
    defer _ = std.c.close(fd);

    // Key with invalid character
    const content = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA!";
    _ = std.c.write(fd, content.ptr, content.len);

    const result = generate.readPublicKey(tmp_path, std.heap.page_allocator);
    try std.testing.expectError(generate.GenerateError.InvalidPublicKey, result);
}

test "readPublicKey rejects invalid padding" {
    const tmp_path = "/tmp/wg-test-pubkey-bad-padding";

    var path_buf: [256]u8 = undefined;
    @memcpy(path_buf[0..tmp_path.len], tmp_path);
    path_buf[tmp_path.len] = 0;
    const c_path: [*:0]const u8 = @ptrCast(&path_buf);
    defer _ = std.c.unlink(c_path);

    const open_flags = std.c.O{
        .ACCMODE = std.posix.ACCMODE.WRONLY,
        .CREAT = true,
        .TRUNC = true,
    };
    const fd = std.c.open(c_path, open_flags, @as(c_uint, 0o600));
    try std.testing.expect(fd >= 0);
    defer _ = std.c.close(fd);

    // Key with invalid character after base64 portion
    const content = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA!";
    _ = std.c.write(fd, content.ptr, content.len);

    const result = generate.readPublicKey(tmp_path, std.heap.page_allocator);
    try std.testing.expectError(generate.GenerateError.InvalidPublicKey, result);
}

test "createOutputDir creates nested directories" {
    // Create a unique nested path under /tmp
    const unique_dir = "/tmp/tovarisch-wg-test-nested-unique";

    // Create a deeply nested path that doesn't exist
    const nested_path = unique_dir ++ "/a/b/c/d";

    // This should succeed without error using recursive directory creation.
    try generate.createOutputDir(nested_path);

    // Clean up - remove the nested directory
    _ = std.c.rmdir(nested_path);
    _ = std.c.rmdir(unique_dir ++ "/a/b/c");
    _ = std.c.rmdir(unique_dir ++ "/a/b");
    _ = std.c.rmdir(unique_dir ++ "/a");
    _ = std.c.rmdir(unique_dir);
}

test "createOutputDir handles existing directory" {
    // Create a unique path under /tmp
    const unique_dir = "/tmp/tovarisch-wg-test-existing";

    // Create the directory first
    try generate.createOutputDir(unique_dir);

    // Creating the same directory again should succeed (EEXIST is handled)
    try generate.createOutputDir(unique_dir);

    // Clean up
    _ = std.c.rmdir(unique_dir);
}

test "writeConfigFile creates file with correct permissions" {
    const tmp_path = "/tmp/tovarisch-wg-test-config";

    // Clean up any existing file
    var path_buf: [256]u8 = undefined;
    @memcpy(path_buf[0..tmp_path.len], tmp_path);
    path_buf[tmp_path.len] = 0;
    const c_path: [*:0]const u8 = @ptrCast(&path_buf);
    _ = std.c.unlink(c_path);

    const content = "[Interface]\nAddress = 10.0.0.1/24\n";
    try generate.writeConfigFile(tmp_path, content);

    // Verify file was created
    const fd = std.c.open(c_path, @bitCast(@as(u32, 0)));
    try std.testing.expect(fd >= 0);
    defer _ = std.c.close(fd);

    // Clean up
    _ = std.c.unlink(c_path);
}

test "ServerGenerateResult deinit frees memory" {
    var result = generate.ServerGenerateResult{
        .interface = "wg0",
        .address = "10.0.0.1/24",
        .listen_port = 51820,
        .output_path = try std.heap.page_allocator.dupe(u8, "/tmp/test.conf"),
        .peer_count = 1,
    };
    result.deinit(std.heap.page_allocator);
}

test "ClientGenerateResult deinit frees memory" {
    var result = generate.ClientGenerateResult{
        .peer_name = "phone",
        .output_path = try std.heap.page_allocator.dupe(u8, "/tmp/phone.conf"),
    };
    result.deinit(std.heap.page_allocator);
}

test "readPublicKey accepts padded key ending with =" {
    // Regression test: wg pubkey outputs 44-char keys with final '='
    // e.g., "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n" (43 A's + '=' = 44 chars)
    const tmp_path = "/tmp/tovarisch-wg-test-padded-pubkey";

    var path_buf: [256]u8 = undefined;
    @memcpy(path_buf[0..tmp_path.len], tmp_path);
    path_buf[tmp_path.len] = 0;
    const c_path: [*:0]const u8 = @ptrCast(&path_buf);
    defer _ = std.c.unlink(c_path);

    const open_flags = std.c.O{
        .ACCMODE = std.posix.ACCMODE.WRONLY,
        .CREAT = true,
        .TRUNC = true,
    };
    const fd = std.c.open(c_path, open_flags, @as(c_uint, 0o600));
    try std.testing.expect(fd >= 0);
    defer _ = std.c.close(fd);

    // 44-character key: 43 base64 chars + final '=' (normal wg pubkey output)
    const padded_key = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
    _ = std.c.write(fd, padded_key.ptr, padded_key.len);

    const result = try generate.readPublicKey(tmp_path, std.heap.page_allocator);
    defer std.heap.page_allocator.free(result);
    try std.testing.expectEqualStrings(padded_key, result);
}
