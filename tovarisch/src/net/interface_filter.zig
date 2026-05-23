// interface_filter.zig — Private-interface filtering for collected stats
//
// ACT 5e: Filter interface stats snapshots to only include interfaces
// that have at least one private IP address.
//
// This module is fixture-backed with an injectable address source.
// It does NOT implement live Linux address discovery (rtnetlink, etc).
//
// Scope:
// - Address-source abstraction (InterfaceAddress struct)
// - interfaceHasPrivateAddress() predicate
// - filterPrivateInterfaceStats() filtering function
// - Reuses existing private_ip.zig classification logic
//
// Non-goals:
// - No live address discovery
// - No rtnetlink
// - No /proc/net/fib_trie parsing
// - No shell command parsing (ip addr)
// - No /metrics.json wiring

const std = @import("std");
const linux_interface_stats = @import("linux_interface_stats.zig");
const private_ip = @import("private_ip.zig");

// ============================================================================
// Types
// ============================================================================

/// Represents a network interface address mapping.
/// This is the injectable address-source abstraction for fixture-testing.
pub const InterfaceAddress = struct {
    /// Interface name (e.g., "eth0", "wg0")
    iface: []const u8,
    /// IP address as a string (e.g., "192.168.1.10", "fd00::1")
    address: []const u8,
};

// ============================================================================
// Predicate
// ============================================================================

/// Checks if an interface has at least one private address.
///
/// Behavior:
/// - Returns true if any address with matching `iface` is classified private
///   by existing private_ip.zig logic.
/// - Returns false if there are no addresses for the interface.
/// - Returns false for malformed addresses.
/// - Loopback and link-local are NOT considered private per private_ip semantics.
///
/// This is a pure predicate with no allocator requirements.
pub fn interfaceHasPrivateAddress(
    iface: []const u8,
    addresses: []const InterfaceAddress,
) bool {
    for (addresses) |addr| {
        // Match by interface name
        if (!std.mem.eql(u8, addr.iface, iface)) continue;

        // Classify the address using existing private_ip logic
        const classification = private_ip.classifyIpv4Text(addr.address);

        // Only .private is considered for inclusion
        if (classification == .private) return true;
    }

    // No addresses found or no private address found
    return false;
}

// ============================================================================
// Filtering
// ============================================================================

/// Filters interface stats snapshots to only include interfaces
/// that have at least one private IP address.
///
/// Behavior:
/// - Returns owned snapshot copies where `name` has at least one private address.
/// - Input snapshots are NOT mutated or freed.
/// - Result must be freed via freeInterfaceStatsSnapshots().
/// - Malformed addresses are ignored and do not include the interface.
///
/// Errors:
/// - `OutOfMemory` if allocation fails (partial results are cleaned up).
pub fn filterPrivateInterfaceStats(
    allocator: std.mem.Allocator,
    snapshots: []const linux_interface_stats.InterfaceStatsSnapshot,
    addresses: []const InterfaceAddress,
) error{OutOfMemory}![]linux_interface_stats.InterfaceStatsSnapshot {
    var result = std.ArrayList(linux_interface_stats.InterfaceStatsSnapshot).empty;
    errdefer {
        // Free all allocated names in snapshots already collected
        for (result.items) |snap| allocator.free(snap.name);
        result.deinit(allocator);
    }

    for (snapshots) |snap| {
        // Check if this interface has a private address
        if (!interfaceHasPrivateAddress(snap.name, addresses)) continue;

        // Make an owned copy of the snapshot (name copy only; stats are value type)
        const name_copy = try allocator.dupe(u8, snap.name);
        errdefer allocator.free(name_copy);

        try result.append(allocator, linux_interface_stats.InterfaceStatsSnapshot{
            .name = name_copy,
            .stats = snap.stats,
        });
    }

    return try result.toOwnedSlice(allocator);
}
