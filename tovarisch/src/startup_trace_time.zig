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
                const sec_val: i128 = if (@hasField(std.posix.timespec, "tv_sec"))
                    @intCast(ts.tv_sec)
                else
                    @intCast(ts.sec);
                const nsec_val: i128 = if (@hasField(std.posix.timespec, "tv_nsec"))
                    @intCast(ts.tv_nsec)
                else
                    @intCast(ts.nsec);
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

    // Fallback: return wall clock time in ns (less accurate but better than 0).
    // Zig 0.16 types clock_gettime's clock ID as clockid_t;
    // use the named value corresponding to the previous CLOCK_REALTIME value 0.
    if (@hasDecl(std.c, "clock_gettime")) {
        var ts: std.c.timespec = undefined;
        if (std.c.clock_gettime(.REALTIME, &ts) == 0) {
            // Mirror the defensive @hasField pattern used above for std.posix.timespec.
            // std.c.timespec uses .sec/.nsec on Linux/macOS/BSD in Zig 0.16.
            const sec_val: i128 = if (@hasField(std.c.timespec, "tv_sec"))
                @intCast(ts.tv_sec)
            else
                @intCast(ts.sec);
            const nsec_val: i128 = if (@hasField(std.c.timespec, "tv_nsec"))
                @intCast(ts.tv_nsec)
            else
                @intCast(ts.nsec);
            return sec_val * std.time.ns_per_s + nsec_val;
        }
    }

    return 0;
}
