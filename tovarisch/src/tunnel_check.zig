// tunnel_check.zig — Tunnel presence health check for status
//
// ACT-TOVARISCH-ZIG-HULK16: Migrate to caller-provided allocator
//
// This module provides tunnel presence detection for the status check.
// It does NOT validate:
// - WireGuard peer health
// - Latest handshake age
// - Route validity
// - Remote endpoint reachability
// - Tunnel traffic rates
//
// The check is interface-presence only, reporting ok when one or more
// tunnel-like interfaces are detected, warn when none are found.

const std = @import("std");
const interface_filter = @import("net/interface_filter.zig");
const linux_interfaces = @import("net/linux_interfaces.zig");
const status = @import("status.zig");

pub const Check = status.Check;
pub const CheckStatus = status.CheckStatus;

/// Default sysfs interface path used for tunnel detection.
pub const DEFAULT_SYSFS_NET_PATH = "/sys/class/net";

/// Static buffer for tunnel detail string.
///
/// NOTE: This buffer is safe for the current single-threaded status-rendering model.
/// The tunnel check is computed during getLocalChecks() which is called once per
/// status rendering. If status rendering becomes concurrent, this must be revisited
/// to use caller-owned buffers or owned status payload state.
var tunnel_detail_buf: [4096]u8 = undefined;

/// Performs a tunnel presence check by enumerating interfaces under sysfs
/// and checking for tunnel-like interface names.
///
/// This is a presence check only - it does NOT validate:
/// - WireGuard peer health or handshake status
/// - Route validity
/// - Remote endpoint reachability
/// - Tunnel traffic rates
///
/// Uses name-based classification from interface_filter.isTunnelInterface():
/// - wg* (WireGuard: wg, wg0, wg1, ...)
/// - tun* (TUN interfaces)
/// - tap* (TAP interfaces)
/// - sit* (SIT tunnels)
/// - ip6tnl* (IPv6 tunnels)
/// - gre* (GRE tunnels)
/// - ipip* (IP-in-IP tunnels)
///
/// Returns a Check with:
/// - "ok" + tunnel names when detected (e.g., "detected tunnel interfaces: wg0, tun0")
/// - "warn" + "no tunnel interfaces detected" when none found or sysfs unavailable
///
/// MemoryOwnership: Uses caller-provided allocator for interface list
/// Deinit: linux_interfaces.freeInterfaceList(allocator, ifaces)
pub fn getTunnelCheck(allocator: std.mem.Allocator, sysfs_net_path: []const u8) Check {
    var path_buf: [4096]u8 = undefined;
    const c_path = status.toCString(sysfs_net_path, &path_buf) orelse {
        // Path too long - treat as no tunnel detected (warn)
        return Check{
            .name = "tunnel",
            .status = .warn,
            .detail = "no tunnel interfaces detected",
        };
    };

    // Check if sysfs path exists
    if (std.c.access(c_path, std.c.F_OK) != 0) {
        // Path unavailable - treat as no tunnel detected (warn)
        return Check{
            .name = "tunnel",
            .status = .warn,
            .detail = "no tunnel interfaces detected",
        };
    }

    // Enumerate all network interfaces using caller-provided allocator
    const ifaces = linux_interfaces.listInterfaces(allocator, sysfs_net_path) catch {
        // Collection failed - treat as no tunnel detected (warn)
        return Check{
            .name = "tunnel",
            .status = .warn,
            .detail = "no tunnel interfaces detected",
        };
    };
    defer linux_interfaces.freeInterfaceList(allocator, ifaces);

    // Find all tunnel interfaces using fixed-size array (no heap allocation needed)
    // Maximum of 32 tunnel interfaces detected - sufficient for any realistic scenario
    var tunnel_names: [32][]const u8 = undefined;
    var tunnel_count: usize = 0;

    for (ifaces) |iface| {
        if (interface_filter.isTunnelInterface(iface) and tunnel_count < tunnel_names.len) {
            tunnel_names[tunnel_count] = iface;
            tunnel_count += 1;
        }
    }

    if (tunnel_count == 0) {
        return Check{
            .name = "tunnel",
            .status = .warn,
            .detail = "no tunnel interfaces detected",
        };
    }

    // Build the detail message with detected tunnel names using static buffer
    // Format: "detected tunnel interfaces: wg0, tun0, ..."
    const detail = buildTunnelDetail(&tunnel_detail_buf, tunnel_names[0..tunnel_count]);
    return Check{
        .name = "tunnel",
        .status = .ok,
        .detail = detail,
    };
}

/// Default tunnel check using caller-provided allocator.
pub fn getTunnelCheckWithAllocator(allocator: std.mem.Allocator) Check {
    return getTunnelCheck(allocator, DEFAULT_SYSFS_NET_PATH);
}

/// Builds a detail string listing detected tunnel interfaces.
fn buildTunnelDetail(buf: *[4096]u8, tunnel_names: []const []const u8) []const u8 {
    // Start with the prefix
    const prefix = "detected tunnel interfaces: ";
    // MemoryCopySafety: buf is a fixed [4096]u8 caller-owned buffer. prefix is a
    // compile-time constant string literal. They are distinct memory regions.
    @memcpy(buf[0..prefix.len], prefix);
    var pos = prefix.len;

    for (tunnel_names, 0..) |name, i| {
        if (i > 0) {
            // Add comma and space between names
            if (pos + 2 > buf.len) break;
            buf[pos] = ',';
            buf[pos + 1] = ' ';
            pos += 2;
        }

        // Add the interface name
        if (pos + name.len > buf.len) {
            // Truncate if buffer too small
            // MemoryCopySafety: buf[pos..] is a caller-owned sub-slice. name is a
            // parameter slice from tunnel_names iteration. They are distinct.
            @memcpy(buf[pos..], name[0..@min(name.len, buf.len - pos)]);
            pos += @min(name.len, buf.len - pos);
            break;
        }
        // MemoryCopySafety: buf is a fixed [4096]u8 buffer. name is a parameter
        // slice from tunnel_names iteration. They are distinct memory regions.
        @memcpy(buf[pos..][0..name.len], name);
        pos += name.len;
    }

    return buf[0..pos];
}

// --- Tests ---

test "buildTunnelDetail with single tunnel" {
    var buf: [4096]u8 = undefined;
    const tunnel_names = [_][]const u8{"wg0"};
    const result = buildTunnelDetail(&buf, tunnel_names[0..]);
    try std.testing.expectEqualStrings("detected tunnel interfaces: wg0", result);
}

test "buildTunnelDetail with multiple tunnels" {
    var buf: [4096]u8 = undefined;
    const tunnel_names = [_][]const u8{ "wg0", "tun1", "tap0" };
    const detail = buildTunnelDetail(&buf, tunnel_names[0..]);
    try std.testing.expectEqualStrings("detected tunnel interfaces: wg0, tun1, tap0", detail);
}

test "buildTunnelDetail with empty list" {
    var buf: [4096]u8 = undefined;
    const tunnel_names: [0][]const u8 = .{};
    const detail = buildTunnelDetail(&buf, tunnel_names[0..]);
    try std.testing.expectEqualStrings("detected tunnel interfaces: ", detail);
}

test "getTunnelCheck returns warn for nonexistent path" {
    // Use std.heap.FixedBufferAllocator for Zig 0.16
    var buf: [256]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const allocator = fba.allocator();

    const check = getTunnelCheck(allocator, "/nonexistent/path/that/does/not/exist");
    try std.testing.expectEqualStrings("tunnel", check.name);
    try std.testing.expectEqual(CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("no tunnel interfaces detected", check.detail);
}

test "getTunnelCheck returns warn for path too long" {
    var buf: [256]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const allocator = fba.allocator();

    // Create a path longer than 4096 characters
    var long_path: [4097]u8 = undefined;
    @memset(&long_path, 'a');
    long_path[4096] = 0;

    const check = getTunnelCheck(allocator, long_path[0..4096]);
    try std.testing.expectEqualStrings("tunnel", check.name);
    try std.testing.expectEqual(CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("no tunnel interfaces detected", check.detail);
}

test "getTunnelCheckWithAllocator returns correct name" {
    var buf: [256]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const check = getTunnelCheckWithAllocator(fba.allocator());
    try std.testing.expectEqualStrings("tunnel", check.name);
}

test "DEFAULT_SYSFS_NET_PATH is correct" {
    try std.testing.expectEqualStrings("/sys/class/net", DEFAULT_SYSFS_NET_PATH);
}

test "tunnel check detail format when tunnels present" {
    // This test verifies the detail format matches expected JSON output
    // When tunnel interfaces are detected, detail should include "detected tunnel interfaces:"
    var buf: [4096]u8 = undefined;
    const tunnel_names = [_][]const u8{"wg0", "wg1"};
    const detail = buildTunnelDetail(&buf, tunnel_names[0..]);

    // Verify it starts with the expected prefix
    try std.testing.expect(std.mem.startsWith(u8, detail, "detected tunnel interfaces:"));
    // Verify both interface names are present
    try std.testing.expect(std.mem.containsAtLeast(u8, detail, 1, "wg0"));
    try std.testing.expect(std.mem.containsAtLeast(u8, detail, 1, "wg1"));
    // Verify they're separated by comma
    try std.testing.expect(std.mem.containsAtLeast(u8, detail, 1, ","));
}

test "getTunnelCheck with fake sysfs containing wg0 returns ok" {
    var buf: [256]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const allocator = fba.allocator();

    // Create a temporary directory with a fake "wg0" interface entry
    const test_dir = "/tmp/tovarisch_test_tunnel_12345";
    var path_buf: [4096]u8 = undefined;
    const c_path = status.toCString(test_dir, &path_buf) orelse return error.SkipZigTest;
    _ = std.c.mkdir(c_path, 0o755);

    defer _ = std.c.rmdir(c_path);

    // Create fake interface directories (just need entries in the directory)
    const wg0_path = std.fmt.bufPrint(&path_buf, "{s}/wg0", .{test_dir}) catch return error.SkipZigTest;
    var wg0_path_buf: [4096]u8 = undefined;
    const wg0_c_path = status.toCString(wg0_path, &wg0_path_buf) orelse return error.SkipZigTest;
    _ = std.c.mkdir(wg0_c_path, 0o755);
    defer _ = std.c.rmdir(wg0_c_path);

    // Run getTunnelCheck with our fake sysfs directory
    const check = getTunnelCheck(allocator, test_dir);

    // Assert the check has correct values
    try std.testing.expectEqualStrings("tunnel", check.name);
    try std.testing.expectEqual(CheckStatus.ok, check.status);
    // Detail should contain the detected tunnel interface name
    try std.testing.expect(std.mem.startsWith(u8, check.detail, "detected tunnel interfaces:"));
    try std.testing.expect(std.mem.containsAtLeast(u8, check.detail, 1, "wg0"));
}

test "getTunnelCheck with multiple fake tunnels returns ok with all names" {
    var alloc_buf: [4096]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&alloc_buf);
    const allocator = fba.allocator();

    // Create a temporary directory with multiple fake tunnel interfaces
    const test_dir = "/tmp/tovarisch_test_multi_tunnel_12345";
    var path_buf: [4096]u8 = undefined;
    const c_path = status.toCString(test_dir, &path_buf) orelse return error.SkipZigTest;
    _ = std.c.mkdir(c_path, 0o755);

    defer _ = std.c.rmdir(c_path);

    // Create multiple fake tunnel interfaces
    const ifaces = [_][]const u8{ "wg0", "tun1", "tap0" };
    for (ifaces) |iface| {
        var iface_path: [4096]u8 = undefined;
        const full_path = std.fmt.bufPrint(&iface_path, "{s}/{s}", .{ test_dir, iface }) catch continue;
        var iface_path_buf: [4096]u8 = undefined;
        const iface_c_path = status.toCString(full_path, &iface_path_buf) orelse continue;
        _ = std.c.mkdir(iface_c_path, 0o755);
    }

    // Run getTunnelCheck with our fake sysfs directory
    const check = getTunnelCheck(allocator, test_dir);

    // Assert the check has correct values
    try std.testing.expectEqualStrings("tunnel", check.name);
    try std.testing.expectEqual(CheckStatus.ok, check.status);
    // Detail should contain all detected tunnel interface names
    try std.testing.expect(std.mem.startsWith(u8, check.detail, "detected tunnel interfaces:"));
    try std.testing.expect(std.mem.containsAtLeast(u8, check.detail, 1, "wg0"));
    try std.testing.expect(std.mem.containsAtLeast(u8, check.detail, 1, "tun1"));
    try std.testing.expect(std.mem.containsAtLeast(u8, check.detail, 1, "tap0"));
}
