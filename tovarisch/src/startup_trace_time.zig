// startup_trace_time.zig — Cross-platform monotonic time for startup tracing.
///
/// Provides monoTimeNanos() for accurate duration measurement across Linux and macOS.

const std = @import("std");

/// Get current monotonic time in nanoseconds since some unspecified starting point.
///
/// Uses clock_gettime(CLOCK_MONOTONIC) for Linux or mach_absolute_time() for macOS.
/// Both are monotonic clocks not affected by system clock changes.
pub fn monoTimeNanos() i128 {
    const builtin = @import("builtin");

    // Linux: Use clock_gettime(CLOCK_MONOTONIC)
    if (builtin.os.tag == .linux) {
        if (@hasDecl(std.posix, "clock_gettime")) {
            var ts: std.posix.timespec = undefined;
            // CLOCK_MONOTONIC = 1 on Linux
            if (std.posix.clock_gettime(@enumFromInt(1), &ts)) {
                const sec_val: i128 = if (@hasDecl(std.posix.timespec, "tv_sec"))
                    @intCast(ts.tv_sec) else @intCast(ts.sec);
                const nsec_val: i128 = if (@hasDecl(std.posix.timespec, "tv_nsec"))
                    @intCast(ts.tv_nsec) else @intCast(ts.nsec);
                return sec_val * std.time.ns_per_s + nsec_val;
            }
        }
    }

    // macOS: Use mach_absolute_time() with timebase info
    if (builtin.os.tag == .macos or builtin.os.tag == .ios) {
        if (@hasDecl(std.c, "mach_absolute_time") and @hasDecl(std.c, "mach_timebase_info")) {
            var info: std.c.mach_timebase_info_data = undefined;
            _ = std.c.mach_timebase_info(&info);
            const absolute_time = std.c.mach_absolute_time();
            return @divTrunc(absolute_time * @as(i128, info.numer), @as(i128, info.denom));
        }
    }

    // Fallback: return wall clock time in ns (less accurate but better than 0)
    if (@hasDecl(std.c, "clock_gettime")) {
        var ts: std.c.timespec = undefined;
        if (std.c.clock_gettime(0, &ts) == 0) {
            return @as(i128, ts.tv_sec) * std.time.ns_per_s + @as(i128, ts.tv_nsec);
        }
    }

    return 0;
}
