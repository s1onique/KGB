// private_interface_stats.zig — Live private interface stats pipeline
//
// ACT 5g: Wire live address discovery into private-interface stats pipeline.
//
// This module composes:
// - linux_interface_stats.collectInterfaceStats()
// - linux_addr.discoverPrivateAddresses()
// - interface_filter.filterPrivateInterfaceStats()
//
// It provides an end-to-end pipeline from sysfs stats + rtnetlink addresses
// to filtered private-interface stats. Does NOT wire /metrics.json.
//
// Scope:
// - Collect all interface stats from sysfs
// - Discover IPv4 private addresses via rtnetlink
// - Filter to interfaces with at least one private IPv4 address
// - Return owned filtered snapshots (caller frees via freeInterfaceStatsSnapshots)
//
// Non-goals:
// - No /metrics.json wiring (deferred to ACT 5h)
// - No IPv6 support (deferred per IPv4-only scope)
// - No bandwidth/rate calculation
// - No tunnel type detection

const std = @import("std");
const linux_interface_stats = @import("linux_interface_stats.zig");
const linux_addr = @import("linux_addr.zig");
const interface_filter = @import("interface_filter.zig");

// Re-export types for convenience
pub const InterfaceStatsSnapshot = linux_interface_stats.InterfaceStatsSnapshot;
pub const InterfaceAddress = interface_filter.InterfaceAddress;

// ============================================================================
// Error Types
// ============================================================================

/// Errors that can occur during private interface stats collection.
pub const CollectError = error{
    /// Failed to collect interface stats from sysfs
    StatsCollectionFailed,
    /// Failed to discover addresses via rtnetlink
    AddressDiscoveryFailed,
    /// Memory allocation failed
    OutOfMemory,
} || linux_addr.AddrError;

// ============================================================================
// Core Functions
// ============================================================================

/// Filters already-collected interface stats snapshots using already-discovered
/// private addresses.
///
/// This is a pure helper that composes the filtering boundary without live
/// collection. It gives cross-platform deterministic test coverage for the
/// composition logic.
///
/// Returns owned filtered snapshots. Caller must free via
/// `linux_interface_stats.freeInterfaceStatsSnapshots()`.
pub fn filterCollectedPrivateInterfaceStats(
    allocator: std.mem.Allocator,
    snapshots: []const linux_interface_stats.InterfaceStatsSnapshot,
    addresses: []const interface_filter.InterfaceAddress,
) error{OutOfMemory}![]linux_interface_stats.InterfaceStatsSnapshot {
    return try interface_filter.filterPrivateInterfaceStats(allocator, snapshots, addresses);
}

/// Collects interface stats for all interfaces that have at least one private
/// IPv4 address by composing live sysfs stats collection with live rtnetlink
/// address discovery.
///
/// Pipeline:
/// 1. Collect all interface stats from sysfs (collectInterfaceStats)
/// 2. Discover IPv4 private addresses via rtnetlink (discoverPrivateAddresses)
/// 3. Filter stats to only include interfaces with private addresses
///
/// Intermediate allocations are freed before returning. The caller owns the
/// returned filtered snapshots and must free them via
/// `linux_interface_stats.freeInterfaceStatsSnapshots()`.
///
/// Errors:
/// - StatsCollectionFailed if sysfs collection fails
/// - AddressDiscoveryFailed if rtnetlink fails
/// - OutOfMemory if any allocation fails
///
/// Note: Per-interface unreadable stats are already skipped by
/// collectInterfaceStats(). Address discovery failures propagate as errors
/// per ACT 5g scope (not silently treated as "no private interfaces").
pub fn collectPrivateInterfaceStats(
    allocator: std.mem.Allocator,
    sysfs_root: []const u8,
    ) CollectError![]linux_interface_stats.InterfaceStatsSnapshot {
    // Step 1: Collect all interface stats from sysfs
    const all_stats = linux_interface_stats.collectInterfaceStats(
        allocator,
        sysfs_root,
        .sysfs_net,
    ) catch |err| {
        switch (err) {
            error.OutOfMemory => return error.OutOfMemory,
            else => return error.StatsCollectionFailed,
        }
    };
    // Free intermediate stats on any subsequent failure
    errdefer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, all_stats);

    // Step 2: Discover private IPv4 addresses via rtnetlink
    // Note: sys_class_net param is currently unused by rtnetlink but kept
    // for API consistency with future sysfs fallback scenarios
    const addresses = linux_addr.discoverPrivateAddresses(
        allocator,
        sysfs_root,
    ) catch |err| {
        return switch (err) {
            error.SocketCreateFailed => error.AddressDiscoveryFailed,
            error.BindFailed => error.AddressDiscoveryFailed,
            error.SendFailed => error.AddressDiscoveryFailed,
            error.RecvFailed => error.AddressDiscoveryFailed,
            error.InvalidMessage => linux_addr.AddrError.InvalidMessage,
            error.InvalidAttribute => linux_addr.AddrError.InvalidAttribute,
            error.OutOfMemory => error.OutOfMemory,
            error.MissingInterfaceName => linux_addr.AddrError.MissingInterfaceName,
        };
    };
    // Free intermediate addresses on any subsequent failure
    errdefer linux_addr.freeAddresses(allocator, addresses);

    // Step 3: Filter stats to only include interfaces with private addresses
    const filtered = try interface_filter.filterPrivateInterfaceStats(
        allocator,
        all_stats,
        addresses,
    );

    // Step 4: Free intermediate resources before returning
    linux_interface_stats.freeInterfaceStatsSnapshots(allocator, all_stats);
    linux_addr.freeAddresses(allocator, addresses);

    return filtered;
}
