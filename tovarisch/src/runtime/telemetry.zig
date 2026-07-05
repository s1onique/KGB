// telemetry.zig — Runtime telemetry - self-observed process metrics
//
// ACT-TOVARISCH-ZIG-HULK16: Migrate to canonical linux_read.zig boundary
//
// These are telemetry data, not health checks.
// The RSS value helps detect memory bloat in constrained leaf nodes.

const std = @import("std");
const linux_read = @import("../net/linux_read.zig");

// ============================================================================
// Constants
// ============================================================================

/// Maximum bytes to read from /proc/self/status
/// VmRSS line is typically under 100 bytes; entire file is under 4KB
const PROC_SELF_STATUS_MAX_BYTES: usize = 8192;

// ============================================================================
// Types
// ============================================================================

/// Runtime telemetry - self-observed process metrics.
pub const RuntimeTelemetry = struct {
    pid: u32,
    rss_kib: ?u64,
};

/// Telemetry availability state for structured reporting
pub const TelemetryAvailability = enum {
    available,
    unavailable_permission_denied,
    unavailable_missing,
    unavailable_unsupported_platform,
    unavailable_too_large,
    unavailable_malformed,
    unavailable_io_error,
    unavailable_unknown,
};

/// Telemetry result with structured availability
pub const TelemetryResult = struct {
    telemetry: RuntimeTelemetry,
    availability: TelemetryAvailability,
};

// ============================================================================
// Parsing
// ============================================================================

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

// ============================================================================
// PID
// ============================================================================

/// Returns the current process PID.
/// Uses std.c.getpid since we link libc anyway for socket support.
pub fn getCurrentPid() u32 {
    return @as(u32, @intCast(std.c.getpid()));
}

// ============================================================================
// Telemetry Collection
// ============================================================================

/// Gets runtime telemetry for the current process.
/// On Linux, reads /proc/self/status to get VmRSS via canonical linux_read boundary.
/// On other platforms, returns PID with null RSS.
/// Returns RuntimeTelemetry for backward compatibility.
pub fn getRuntimeTelemetry() RuntimeTelemetry {
    const result = getRuntimeTelemetryWithAllocator(std.heap.page_allocator);
    return result.telemetry;
}

/// Gets runtime telemetry with structured availability.
/// Returns TelemetryResult with both telemetry data and availability state.
pub fn getRuntimeTelemetryWithAvailability() TelemetryResult {
    return getRuntimeTelemetryWithAllocator(std.heap.page_allocator);
}

/// Gets runtime telemetry with a specific allocator.
/// On Linux, reads /proc/self/status to get VmRSS via canonical linux_read boundary.
/// On other platforms, returns PID with null RSS and unavailable state.
pub fn getRuntimeTelemetryWithAllocator(allocator: std.mem.Allocator) TelemetryResult {
    const pid = getCurrentPid();

    if (@import("builtin").os.tag == .linux) {
        const read_result = linux_read.linuxRead(
            allocator,
            "/proc/self/status",
            .proc_self,
            .{ .max_bytes = PROC_SELF_STATUS_MAX_BYTES },
        );

        switch (read_result) {
            .value => |content| {
                defer allocator.free(content);
                const rss_kib = parseVmRssKiB(content);
                return TelemetryResult{
                    .telemetry = RuntimeTelemetry{
                        .pid = pid,
                        .rss_kib = rss_kib,
                    },
                    .availability = if (rss_kib != null) .available else .available,
                };
            },
            .permission_denied => {
                return TelemetryResult{
                    .telemetry = RuntimeTelemetry{ .pid = pid, .rss_kib = null },
                    .availability = .unavailable_permission_denied,
                };
            },
            .missing => {
                return TelemetryResult{
                    .telemetry = RuntimeTelemetry{ .pid = pid, .rss_kib = null },
                    .availability = .unavailable_missing,
                };
            },
            .unsupported_platform => {
                return TelemetryResult{
                    .telemetry = RuntimeTelemetry{ .pid = pid, .rss_kib = null },
                    .availability = .unavailable_unsupported_platform,
                };
            },
            .too_large => {
                return TelemetryResult{
                    .telemetry = RuntimeTelemetry{ .pid = pid, .rss_kib = null },
                    .availability = .unavailable_too_large,
                };
            },
            .malformed => {
                return TelemetryResult{
                    .telemetry = RuntimeTelemetry{ .pid = pid, .rss_kib = null },
                    .availability = .unavailable_malformed,
                };
            },
            .io_error => {
                return TelemetryResult{
                    .telemetry = RuntimeTelemetry{ .pid = pid, .rss_kib = null },
                    .availability = .unavailable_io_error,
                };
            },
        }
    }

    // Non-Linux platforms: honest fallback
    return TelemetryResult{
        .telemetry = RuntimeTelemetry{ .pid = pid, .rss_kib = null },
        .availability = .unavailable_unsupported_platform,
    };
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

test "TelemetryResult has availability enum" {
    const result = TelemetryResult{
        .telemetry = RuntimeTelemetry{ .pid = 1234, .rss_kib = 1920 },
        .availability = .available,
    };
    try std.testing.expectEqual(TelemetryAvailability.available, result.availability);
    try std.testing.expectEqual(@as(?u64, 1920), result.telemetry.rss_kib);
}

test "PROC_SELF_STATUS_MAX_BYTES is reasonable" {
    try std.testing.expect(PROC_SELF_STATUS_MAX_BYTES >= 4096);
    try std.testing.expect(PROC_SELF_STATUS_MAX_BYTES <= 65536);
}
