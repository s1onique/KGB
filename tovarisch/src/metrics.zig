// metrics.zig — Metrics payload rendering for /metrics.json
//
// ACT 4a: Remove duplicated metrics JSON rendering by using metrics_dto.
//
// This module renders the v0 metrics JSON payload for the /metrics.json endpoint.
// It adapts live InterfaceStatsSnapshot values into metrics_dto.SampledInterface
// values, then delegates JSON serialization to metrics_dto.renderSampledInterfacesPayload().
//
// This ensures a single source of truth for the JSON schema, avoiding duplicated
// hand-rendered JSON that would make ACT 5 (sampler state wiring) riskier.
//
// v0.2 JSON shape (with rate field):
//
//   {
//     "service": "tovarisch",
//     "version": "0.1.1",
//     "metrics_version": "0.2",
//     "private_interfaces": [
//       {
//         "name": "eth0",
//         "rx_bytes": 123,
//         "tx_bytes": 456,
//         "rx_packets": 7,
//         "tx_packets": 8,
//         "rate": null
//       }
//     ],
//     "notes": [
//       "rate is optional (null until sampler state is wired)",
//       "interface counters are cumulative",
//       "IPv4 private interfaces only; IPv6 is deferred"
//     ]
//   }
//
// Fallback shape (on live collection failure):
//
//   {
//     "service": "tovarisch",
//     "version": "0.1.1",
//     "metrics_version": "0.2",
//     "status": "warn",
//     "private_interfaces": [],
//     "error": "metrics_unavailable",
//     "detail": "private interface stats unavailable",
//     "notes": [
//       "rate is optional (null until sampler state is wired)",
//       "interface counters are cumulative",
//       "IPv4 private interfaces only; IPv6 is deferred"
//     ]
//   }
//
// Non-goals:
// - No per-second rate calculation (deferred to ACT 5)
// - No sampler state wiring (deferred to ACT 5)
// - No tunnel detection
// - No IPv6 support (deferred)
// - No Prometheus format

const std = @import("std");
const private_interface_stats = @import("net/private_interface_stats.zig");
const linux_interface_stats = @import("net/linux_interface_stats.zig");
const metrics_dto = @import("metrics_dto.zig");

// Re-export types for convenience
pub const InterfaceStatsSnapshot = linux_interface_stats.InterfaceStatsSnapshot;
pub const CollectError = private_interface_stats.CollectError;
pub const SampledInterface = metrics_dto.SampledInterface;

// Service version constant (matches status.zig)
const service_version = "0.1.1";
const metrics_version = "0.2";

// ============================================================================
// Conversion: InterfaceStatsSnapshot -> SampledInterface
// ============================================================================

/// Converts a slice of InterfaceStatsSnapshot into a slice of SampledInterface.
/// Each SampledInterface has rate = null (no sampler state wired yet).
///
/// Ownership model:
/// - Caller owns the returned slice
/// - Each SampledInterface.sample.name is a duplicated string (caller frees)
/// - Caller must free via freeSampledInterfaces() after rendering
///
/// Returns owned sampled interfaces. Caller frees via freeSampledInterfaces().
pub fn sampledInterfacesFromSnapshots(
    allocator: std.mem.Allocator,
    snapshots: []const InterfaceStatsSnapshot,
) ![]SampledInterface {
    var sampled = try std.ArrayList(SampledInterface).initCapacity(allocator, snapshots.len);
    errdefer {
        // On failure, free any names we already duplicated
        for (sampled.items) |si| {
            allocator.free(si.sample.name);
        }
        sampled.deinit(allocator);
    }

    for (snapshots) |snap| {
        // Duplicate the name for the sampled interface (caller owns)
        const name = try allocator.dupe(u8, snap.name);
        errdefer allocator.free(name);

        const sample = metrics_dto.SampledInterface{
            .sample = .{
                .name = name,
                .rx_bytes = snap.stats.rx_bytes,
                .tx_bytes = snap.stats.tx_bytes,
                .rx_packets = snap.stats.rx_packets,
                .tx_packets = snap.stats.tx_packets,
                .sampled_at_ms = 0, // Placeholder: no timestamp from live collection yet
            },
            .rate = null, // No sampler state wired yet
        };

        try sampled.append(allocator, sample);
    }

    return sampled.toOwnedSlice(allocator);
}

/// Frees all memory associated with a slice of SampledInterface created by
/// sampledInterfacesFromSnapshots().
pub fn freeSampledInterfaces(allocator: std.mem.Allocator, sampled: []SampledInterface) void {
    metrics_dto.freeSampledInterfaces(allocator, sampled);
}

// ============================================================================
// Pure Renderer: renderMetricsPayloadFromSnapshots
// ============================================================================

/// Renders the metrics payload JSON from already-collected interface stats snapshots.
/// This is a pure function suitable for testing with fixture data.
///
/// Adapts snapshots to SampledInterface with rate = null, then delegates JSON
/// serialization to metrics_dto.renderSampledInterfacesPayload().
///
/// The caller owns the snapshots and must free them via
/// `linux_interface_stats.freeInterfaceStatsSnapshots()` after rendering.
pub fn renderMetricsPayloadFromSnapshots(
    allocator: std.mem.Allocator,
    writer: anytype,
    snapshots: []const InterfaceStatsSnapshot,
) !void {
    // Convert snapshots to sampled interfaces with rate = null
    const sampled = try sampledInterfacesFromSnapshots(allocator, snapshots);
    defer freeSampledInterfaces(allocator, sampled);

    // Delegate JSON serialization to DTO
    try metrics_dto.renderSampledInterfacesPayload(writer, sampled);
}

// ============================================================================
// Fallback Renderer: renderMetricsFallbackPayload
// ============================================================================

/// Renders the fallback warning payload when live metrics collection fails.
/// Returns HTTP 200 with a valid JSON payload indicating the warning state.
pub fn renderMetricsFallbackPayload(writer: anytype) !void {
    try writer.writeAll(
        "{\"service\":\"tovarisch\",\"version\":\"0.1.1\",\"metrics_version\":\"0.2\",\"status\":\"warn\",\"private_interfaces\":[],\"error\":\"metrics_unavailable\",\"detail\":\"private interface stats unavailable\",\"notes\":[\"rate is optional (null until sampler state is wired)\",\"interface counters are cumulative\",\"IPv4 private interfaces only; IPv6 is deferred\"]}",
    );
}

// ============================================================================
// Live Renderer: renderLiveMetricsPayload
// ============================================================================

/// Collects live private interface stats and renders them.
/// Uses collectPrivateInterfaceStats() for sysfs + rtnetlink collection.
/// Frees collected snapshots after rendering.
pub fn renderLiveMetricsPayload(
    allocator: std.mem.Allocator,
    writer: anytype,
) !void {
    const sysfs_root = "/sys/class/net";
    const snapshots = try private_interface_stats.collectPrivateInterfaceStats(allocator, sysfs_root);
    defer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, snapshots);

    try renderMetricsPayloadFromSnapshots(allocator, writer, snapshots);
}
