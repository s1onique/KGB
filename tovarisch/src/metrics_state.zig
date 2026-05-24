// metrics_state.zig — Persistent sampler state for live /metrics.json rates
//
// ACT 5: Wire persistent sampler state across HTTP requests.
//
// This module provides the runtime state that persists across requests:
// - InterfaceSampler keeps previous samples for rate calculation
// - One timestamp per collection cycle (not per interface)
// - DTO is the single JSON serializer for success payloads
//
// Ownership model:
// - Server owns MetricsState (initialized in serve loop, deinitialized on exit)
// - MetricsState owns InterfaceSampler (freed in deinit)
// - Sampler owns map keys for previous samples
// - Sampler.update() returns caller-owned sampled interfaces (caller frees)
//
// Clock source: std.os.linux.clock_gettime(CLOCK_REALTIME) — wall-clock seconds + nanoseconds
// Rationale: Provides real wall-clock time with sub-second precision.
//   CLOCK_REALTIME gives Unix epoch time. We convert to milliseconds for rate calculation.
//   Rates become available only when elapsed whole seconds are positive (>= 1000ms).

const std = @import("std");
const interface_sampler = @import("net/interface_sampler.zig");
const metrics_dto = @import("metrics_dto.zig");
const rates = @import("net/rates.zig");
const linux_interface_stats = @import("net/linux_interface_stats.zig");
const private_interface_stats = @import("net/private_interface_stats.zig");
const telemetry = @import("runtime/telemetry.zig");

// Re-export types for convenience
pub const SampledInterface = interface_sampler.SampledInterface;
pub const InterfaceSampler = interface_sampler.InterfaceSampler;

// ============================================================================
// Timestamp Helper
// ============================================================================

/// Get current wall-clock time in milliseconds since Unix epoch.
///
/// Uses std.os.linux.clock_gettime(CLOCK_REALTIME) for wall-clock time.
/// Converts seconds + nanoseconds to milliseconds.
///
/// Returns a positive value representing milliseconds since Unix epoch.
fn currentWallClockMillis() i64 {
    // On Linux, use clock_gettime for real timestamps.
    // On non-Linux (macOS, etc.), return 0 and let tests inject timestamps.
    // Note: cross-platform builds may include std.os.linux but we only want it on native Linux.
    // Note: Zig dev builds (e.g., 0.16.0-dev) use tv_sec/tv_nsec; stable 0.16.0 uses sec/nsec.
    if (comptime @import("builtin").os.tag == .linux and @hasDecl(std.os.linux, "clock_gettime")) {
        var ts: std.os.linux.timespec = undefined;
        if (std.os.linux.clock_gettime(@enumFromInt(0), &ts) < 0) return 0;  // CLOCK_REALTIME = 0
        // Convert to milliseconds: seconds * 1000 + nanoseconds / 1_000_000
        // Use u128 to avoid overflow when multiplying seconds by 1000
        // Detect field names: stable Zig 0.16.0 uses .sec/.nsec; dev builds use .tv_sec/.tv_nsec
        const sec_val: u128 = if (@hasDecl(std.os.linux.timespec, "tv_sec"))
            @intCast(ts.tv_sec) else @intCast(ts.sec);
        const nsec_val: u128 = if (@hasDecl(std.os.linux.timespec, "tv_nsec"))
            @intCast(ts.tv_nsec) else @intCast(ts.nsec);
        return @as(i64, @intCast(sec_val * 1000 + nsec_val / 1_000_000));
    }
    // Fallback for non-Linux: return 0 (tests inject explicit timestamps)
    return 0;
}

// ============================================================================
// Metrics State
// ============================================================================

/// Persistent metrics state for the HTTP server runtime.
///
/// This struct owns the InterfaceSampler that tracks previous samples across
/// requests for rate calculation. It is initialized when the server starts
/// and deinitialized when the server exits.
///
/// Usage:
///   var state = MetricsState.init(allocator);
///   defer state.deinit();
///   // On each /metrics.json request:
///   try state.renderMetricsPayload(allocator, writer);
///
/// Thread-safety: Not thread-safe. Assumes single-threaded HTTP server.
pub const MetricsState = struct {
    const Self = @This();

    allocator: std.mem.Allocator,
    sampler: InterfaceSampler,

    /// Initialize metrics state with an empty sampler.
    pub fn init(allocator: std.mem.Allocator) Self {
        return .{
            .allocator = allocator,
            .sampler = InterfaceSampler.init(allocator),
        };
    }

    /// Free all sampler-owned memory.
    /// Safe to call even if no requests were processed.
    pub fn deinit(self: *Self) void {
        self.sampler.deinit();
    }

    /// Collect live interface stats, update sampler, and render metrics payload.
    ///
    /// Flow:
    /// 1. Collect current private interface stats from sysfs
    /// 2. Get current wall-clock timestamp
    /// 3. Convert to InterfaceCounterSample with current timestamp
    /// 4. Feed to sampler.update() for rate calculation
    /// 5. Render sampler output with DTO (single JSON serializer)
    /// 6. Free intermediate allocations
    ///
    /// On collection failure, falls back to warning payload (rate: null semantics preserved).
    pub fn renderMetricsPayload(
        self: *Self,
        allocator: std.mem.Allocator,
        writer: anytype,
        sysfs_root: []const u8,
    ) !void {
        // Step 1: Collect current private interface stats
        const snapshots = private_interface_stats.collectPrivateInterfaceStats(
            allocator,
            sysfs_root,
        ) catch {
            // Fallback: render warning payload (rate: null semantics preserved)
            return self.renderFallbackPayload(writer);
        };
        defer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, snapshots);

        // Step 2: Get current timestamp (one per collection cycle)
        const now_ms = currentWallClockMillis();

        // Step 3: Convert snapshots to counter samples
        const samples = try self.snapshotsToCounterSamples(allocator, snapshots, now_ms);
        defer {
            // Free counter samples (names duplicated in conversion)
            for (samples) |s| allocator.free(s.name);
            allocator.free(samples);
        }

        // Step 4: Update sampler and get sampled results
        const sampled = try self.sampler.update(samples);
        // Note: sampled names are owned by caller; we free via metrics_dto helper
        defer metrics_dto.freeSampledInterfaces(allocator, sampled);

        // Step 5: Get runtime telemetry
        const runtime = telemetry.getRuntimeTelemetry();

        // Step 6: Render with DTO (single JSON serializer)
        try metrics_dto.renderSampledInterfacesPayload(writer, sampled, runtime);
    }

    /// Convert InterfaceStatsSnapshot to InterfaceCounterSample with timestamp.
    ///
    /// Each sample gets the same timestamp (now_ms) for consistency.
    /// Names are duplicated for the counter samples (caller must free).
    fn snapshotsToCounterSamples(
        self: *Self,
        allocator: std.mem.Allocator,
        snapshots: []const linux_interface_stats.InterfaceStatsSnapshot,
        now_ms: i64,
    ) ![]rates.InterfaceCounterSample {
        _ = self;
        var samples = try std.ArrayList(rates.InterfaceCounterSample).initCapacity(
            allocator,
            snapshots.len,
        );
        errdefer {
            for (samples.items) |s| allocator.free(s.name);
            samples.deinit(allocator);
        }

        for (snapshots) |snap| {
            const name = try allocator.dupe(u8, snap.name);
            errdefer allocator.free(name);

            try samples.append(allocator, .{
                .name = name,
                .rx_bytes = snap.stats.rx_bytes,
                .tx_bytes = snap.stats.tx_bytes,
                .rx_packets = snap.stats.rx_packets,
                .tx_packets = snap.stats.tx_packets,
                .sampled_at_ms = now_ms,
            });
        }

        return try samples.toOwnedSlice(allocator);
    }

    /// Render fallback warning payload.
    /// Used when live collection fails.
    fn renderFallbackPayload(self: *Self, writer: anytype) !void {
        _ = self;
        // Fallback payload with runtime telemetry
        const runtime = telemetry.getRuntimeTelemetry();
        try writer.writeAll("{\"service\":\"tovarisch\",\"version\":\"0.1.1\",\"metrics_version\":\"0.3\",\"status\":\"warn\",\"runtime\":{");
        try writer.print("\"pid\":{d}", .{runtime.pid});
        if (runtime.rss_kib) |rss| {
            try writer.print(",\"rss_kib\":{d}", .{rss});
        } else {
            try writer.writeAll(",\"rss_kib\":null");
        }
        try writer.writeAll("},\"private_interfaces\":[],\"error\":\"metrics_unavailable\",\"detail\":\"private interface stats unavailable\",\"notes\":[\"rate is null until a previous sample exists\",\"interface counters are cumulative\",\"IPv4 private interfaces only; IPv6 is deferred\",\"runtime RSS is best-effort platform telemetry\"]}");
    }

    /// Render metrics from already-collected snapshots using the persistent sampler.
    ///
    /// This is fixture-based for testing, but still updates MetricsState sampler state.
    /// Each call updates the sampler with the given samples, enabling rate calculation
    /// on subsequent calls.
    ///
    /// Tests can inject explicit timestamps via the now_ms parameter to achieve
    /// deterministic rate calculation tests.
    pub fn renderMetricsPayloadFromSnapshots(
        self: *Self,
        allocator: std.mem.Allocator,
        writer: anytype,
        snapshots: []const linux_interface_stats.InterfaceStatsSnapshot,
        now_ms: i64,
    ) !void {
        // Convert snapshots to counter samples
        const samples = try self.snapshotsToCounterSamples(allocator, snapshots, now_ms);
        defer {
            for (samples) |s| allocator.free(s.name);
            allocator.free(samples);
        }

        // Update sampler and get sampled results
        const sampled = try self.sampler.update(samples);
        defer metrics_dto.freeSampledInterfaces(allocator, sampled);

        // Get runtime telemetry
        const runtime = telemetry.getRuntimeTelemetry();

        // Render with DTO
        try metrics_dto.renderSampledInterfacesPayload(writer, sampled, runtime);
    }
};

// Re-export for convenience
pub const InterfaceStatsSnapshot = linux_interface_stats.InterfaceStatsSnapshot;

// ============================================================================
// Tests
// ============================================================================

test "currentWallClockMillis returns positive timestamp on Linux" {
    // On Linux, the timestamp should be positive (after Jan 1, 1970).
    // On non-Linux (macOS), the function returns 0 (fallback behavior).
    const ts = currentWallClockMillis();
    if (comptime @import("builtin").os.tag == .linux) {
        // On Linux, we expect a positive timestamp
        try std.testing.expect(ts > 0);
    }
    // On non-Linux, the function may return 0 (expected fallback behavior)
}
