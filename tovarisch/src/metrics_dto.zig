// metrics_dto.zig — JSON DTO/serialization for sampled interface metrics
//
// ACT 3: Metrics JSON DTO with optional interface rates.
//
// This module provides JSON serialization for SampledInterface data,
// including optional calculated rates. It is intentionally isolated from
// HTTP server wiring: pure rendering functions only.
//
// Purpose:
//   - Render interface metrics with optional rates to JSON
//   - Provide contract tests for the JSON shape
//   - Enable future HTTP wiring without changing the serialization logic
//
// Non-goals:
//   - No sysfs reading (belongs in linux_stats.zig)
//   - No live sampler state (belongs in interface_sampler.zig)
//   - No HTTP server wiring (future ACT)
//
// JSON shape for a sampled interface with rate:
//   {
//     "name": "eth0",
//     "rx_bytes": 123,
//     "tx_bytes": 456,
//     "rx_packets": 7,
//     "tx_packets": 8,
//     "rate": {
//       "window_seconds": 30,
//       "rx_bytes_delta": 30000,
//       "tx_bytes_delta": 60000,
//       "rx_packets_delta": 300,
//       "tx_packets_delta": 600,
//       "rx_bytes_per_second": 1000,
//       "tx_bytes_per_second": 2000,
//       "rx_packets_per_second": 10,
//       "tx_packets_per_second": 20
//     }
//   }
//
// JSON shape for a sampled interface without rate (first sample):
//   {
//     "name": "eth0",
//     "rx_bytes": 123,
//     "tx_bytes": 456,
//     "rx_packets": 7,
//     "tx_packets": 8,
//     "rate": null
//   }
//
// Top-level payload shape:
//   {
//     "service": "tovarisch",
//     "version": "0.1.1",
//     "metrics_version": "0.3",
//     "runtime": { "pid": 123, "rss_kib": 1920 },
//     "private_interfaces": [ ... ],
//     "notes": [ ... ]
//   }
//
// Version 0.3 adds runtime telemetry (pid, rss_kib) - process telemetry, not interface metrics.

const std = @import("std");
const rates = @import("net/rates.zig");
const sampler = @import("net/interface_sampler.zig");
const telemetry = @import("runtime/telemetry.zig");

// Re-export types for convenience
pub const SampledInterface = sampler.SampledInterface;
pub const InterfaceRate = rates.InterfaceRate;

// Version constants
const service_version = "0.1.1";
const metrics_version = "0.3";

// ============================================================================
// JSON String Escaping
// ============================================================================

/// Escapes special JSON characters in a string and writes to the writer.
/// Handles: " -> \", \ -> \\, \n -> \\n, \r -> \\r, \t -> \\t
pub fn writeJsonString(writer: anytype, s: []const u8) !void {
    for (s) |c| {
        switch (c) {
            '"' => try writer.writeAll("\\\""),
            '\\' => try writer.writeAll("\\\\"),
            '\n' => try writer.writeAll("\\n"),
            '\r' => try writer.writeAll("\\r"),
            '\t' => try writer.writeAll("\\t"),
            else => try writer.writeByte(c),
        }
    }
}

// ============================================================================
// Pure Renderers: renderSampledInterfacesPayload
// ============================================================================

/// Renders the sampled interface metrics payload JSON.
/// Does not take ownership. Caller retains responsibility for freeing sampled
/// interfaces and their name strings.
///
/// The notes indicate that rate is optional and IPv4-only for now.
pub fn renderSampledInterfacesPayload(
    writer: anytype,
    sampled: []const SampledInterface,
    runtime: telemetry.RuntimeTelemetry,
) !void {
    // Service and version header
    try writer.writeAll("{\"service\":\"tovarisch\",\"version\":\"");
    try writer.writeAll(service_version);
    try writer.writeAll("\",\"metrics_version\":\"");
    try writer.writeAll(metrics_version);
    try writer.writeAll("\",\"runtime\":{");

    // Render runtime telemetry
    try writer.print("\"pid\":{d}", .{runtime.pid});
    if (runtime.rss_kib) |rss| {
        try writer.print(",\"rss_kib\":{d}", .{rss});
    } else {
        try writer.writeAll(",\"rss_kib\":null");
    }

    try writer.writeAll("},\"private_interfaces\":[");

    // Render each interface
    for (sampled, 0..) |si, i| {
        if (i > 0) try writer.writeAll(",");
        try renderSampledInterface(writer, si);
    }

    // Notes footer - include runtime RSS note
    try writer.writeAll("],\"notes\":[\"rate is null until a previous sample exists\",\"interface counters are cumulative\",\"IPv4 private interfaces only; IPv6 is deferred\",\"runtime RSS is best-effort platform telemetry\"]}");
}

/// Renders a single SampledInterface as JSON.
/// The rate field is always present: null if unavailable, object if calculated.
fn renderSampledInterface(writer: anytype, si: SampledInterface) !void {
    try writer.writeAll("{\"name\":\"");
    try writeJsonString(writer, si.sample.name);
    try writer.print(
        "\",\"rx_bytes\":{d},\"tx_bytes\":{d},\"rx_packets\":{d},\"tx_packets\":{d}",
        .{
            si.sample.rx_bytes,
            si.sample.tx_bytes,
            si.sample.rx_packets,
            si.sample.tx_packets,
        },
    );

    // Rate field is always present - null if unavailable, object if calculated
    if (si.rate) |r| {
        try writer.print(
            ",\"rate\":{{\"window_seconds\":{d},\"rx_bytes_delta\":{d},\"tx_bytes_delta\":{d},\"rx_packets_delta\":{d},\"tx_packets_delta\":{d},\"rx_bytes_per_second\":{d},\"tx_bytes_per_second\":{d},\"rx_packets_per_second\":{d},\"tx_packets_per_second\":{d}}}",
            .{
                r.window_seconds,
                r.rx_bytes_delta,
                r.tx_bytes_delta,
                r.rx_packets_delta,
                r.tx_packets_delta,
                r.rx_bytes_per_second,
                r.tx_bytes_per_second,
                r.rx_packets_per_second,
                r.tx_packets_per_second,
            },
        );
    } else {
        try writer.writeAll(",\"rate\":null");
    }

    try writer.writeAll("}");
}

/// Convenience helper: frees all memory associated with a slice of SampledInterface.
/// Caller-owned sampled results can be freed after rendering.
pub fn freeSampledInterfaces(allocator: std.mem.Allocator, sampled: []SampledInterface) void {
    for (sampled) |si| {
        allocator.free(si.sample.name);
    }
    allocator.free(sampled);
}
