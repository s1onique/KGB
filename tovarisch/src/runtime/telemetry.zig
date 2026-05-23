const std = @import("std");

/// Runtime telemetry - self-observed process metrics.
/// These are telemetry data, not health checks.
/// The RSS value helps detect memory bloat in constrained leaf nodes.
pub const RuntimeTelemetry = struct {
    pid: u32,
    rss_kib: ?u64,
};

/// Parses VmRSS value from Linux /proc/self/status content.
/// Returns the RSS in KiB units, or null if not found/invalid.
///
/// Example line: "VmRSS:\t    1920 kB"
pub fn parseVmRssKiB(content: []const u8) ?u64 {
    // Look for "VmRSS:" prefix
    const prefix = "VmRSS:";
    const prefix_idx = std.mem.indexOf(u8, content, prefix);
    if (prefix_idx == null) return null;
    
    // Start after prefix
    const value_start = prefix_idx.? + prefix.len;
    // Find end of line (newline or end of content)
    const suffix = content[value_start..];
    const newline_idx = std.mem.indexOfScalar(u8, suffix, '\n');
    // If no newline found, use rest of content
    const line = if (newline_idx) |idx| 
        content[value_start..value_start + idx]
    else 
        suffix;
    
    // Trim whitespace and parse
    const value_part = std.mem.trim(u8, line, " \t");
    // Remove "kB" suffix if present
    const clean = if (std.mem.endsWith(u8, value_part, "kB"))
        std.mem.trim(u8, value_part[0..value_part.len - 2], " ")
    else
        value_part;
    return std.fmt.parseInt(u64, clean, 10) catch null;
}

/// Returns the current process PID.
/// Uses std.c.getpid since we link libc anyway for socket support.
pub fn getCurrentPid() u32 {
    return @as(u32, @intCast(std.c.getpid()));
}

/// Gets runtime telemetry for the current process.
/// On Linux, reads /proc/self/status to get VmRSS.
/// On other platforms, returns PID with null RSS.
pub fn getRuntimeTelemetry() RuntimeTelemetry {
    return .{
        .pid = getCurrentPid(),
        .rss_kib = getVmRssKiB(),
    };
}

/// Platform-specific VmRSS retrieval.
/// On Linux: reads /proc/self/status
/// On other platforms: returns null (honest fallback)
fn getVmRssKiB() ?u64 {
    if (@import("builtin").os.tag == .linux) {
        return linuxGetVmRssKiB();
    }
    return null;
}

fn linuxGetVmRssKiB() ?u64 {
    // /proc/self/status is always available for the current process
    // Use std.c.open with libc linking (we already link libc for sockets)
    // Use struct initializer syntax for the O type (O.ACCMODE = .RDONLY)
    const path: [*:0]const u8 = "/proc/self/status";
    const flags = std.os.linux.O{ .ACCMODE = std.posix.ACCMODE.RDONLY };
    const fd = std.c.open(path, flags, @as(c_uint, 0));
    if (fd < 0) return null;
    defer _ = std.c.close(fd);

    // Read into a fixed buffer to avoid heap allocation
    var buf: [4096]u8 = undefined;
    const bytes_read = std.c.read(fd, &buf, buf.len);
    if (bytes_read == 0) return null;

    const content = buf[0..@as(usize, @intCast(bytes_read))];
    return parseVmRssKiB(content);
}

// --- Tests ---

test "parseVmRssKiB extracts value from VmRSS line" {
    const sample = "Name:\ttovarisch\nVmRSS:\t    1920 kB\n";
    try std.testing.expectEqual(@as(?u64, 1920), parseVmRssKiB(sample));
}

test "parseVmRssKiB handles various spacing" {
    const sample = "VmRSS:\t1920 kB";
    try std.testing.expectEqual(@as(?u64, 1920), parseVmRssKiB(sample));
}

test "parseVmRssKiB returns null when not found" {
    const sample = "Name:\ttovarisch\nState:\tRunning";
    try std.testing.expectEqual(@as(?u64, null), parseVmRssKiB(sample));
}

test "parseVmRssKiB handles missing kB suffix" {
    const sample = "VmRSS:\t1920";
    try std.testing.expectEqual(@as(?u64, 1920), parseVmRssKiB(sample));
}

test "parseVmRssKiB handles empty content" {
    try std.testing.expectEqual(@as(?u64, null), parseVmRssKiB(""));
}

test "parseVmRssKiB ignores non-VmRSS lines" {
    const sample = "VmSize:\t    8192 kB\nVmRSS:\t    1920 kB\nVmData:\t    1024 kB";
    try std.testing.expectEqual(@as(?u64, 1920), parseVmRssKiB(sample));
}

test "RuntimeTelemetry struct has correct fields" {
    const t = RuntimeTelemetry{ .pid = 1234, .rss_kib = 1920 };
    try std.testing.expectEqual(@as(u32, 1234), t.pid);
    try std.testing.expectEqual(@as(?u64, 1920), t.rss_kib);
}

test "RuntimeTelemetry supports null rss_kib" {
    const t = RuntimeTelemetry{ .pid = 1234, .rss_kib = null };
    try std.testing.expectEqual(@as(u32, 1234), t.pid);
    try std.testing.expectEqual(@as(?u64, null), t.rss_kib);
}
