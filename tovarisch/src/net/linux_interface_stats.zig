// linux_interface_stats.zig — Composition: enumerate interfaces + read stats
//
// ACT 5d: Combine sysfs interface enumeration with readInterfaceStats().
// Does NOT filter private interfaces, does NOT wire /metrics.json.
//
// Scope:
// - List interface names under an injectable /sys/class/net-style root
// - Read statistics for each interface
// - Skip interfaces with missing/unreadable/malformed stats
// - Return owned InterfaceStatsSnapshot list with allocator-owned names

const std = @import("std");
const linux_stats = @import("linux_stats.zig");
const linux_interfaces = @import("linux_interfaces.zig");

// ============================================================================
// Types
// ============================================================================

pub const InterfaceStatsSnapshot = struct {
    name: []const u8,
    stats: linux_stats.InterfaceStats,
};

// ============================================================================
// Collect Interface Stats
// ============================================================================

/// Collects statistics for all network interfaces that have readable stats.
///
/// Calls `listInterfaces()` to enumerate interface names, then attempts to
/// read statistics for each using `readInterfaceStats()`. Interfaces with
/// missing, unreadable, or malformed stats are skipped silently.
///
/// Returns an owned slice of snapshots. The caller must free the result
/// via `freeInterfaceStatsSnapshots()`.
///
/// Errors:
/// - `RootDirMissing` / `RootDirUnreadable` from `listInterfaces()` if
///   the sysfs root is inaccessible.
/// - `OutOfMemory` if allocation fails (partial results are cleaned up).
pub fn collectInterfaceStats(
    allocator: std.mem.Allocator,
    sysfs_root: []const u8,
) (linux_interfaces.ListError || error{OutOfMemory})![]InterfaceStatsSnapshot {
    // Enumerate all interface names
    const names = try linux_interfaces.listInterfaces(allocator, sysfs_root);
    errdefer linux_interfaces.freeInterfaceList(allocator, names);

    // Collect snapshots only for interfaces with readable stats
    var snapshots = std.ArrayList(InterfaceStatsSnapshot).empty;
    errdefer {
        // Free all allocated names in snapshots already collected
        for (snapshots.items) |snap| allocator.free(snap.name);
        snapshots.deinit(allocator);
    }

    for (names) |name| {
        // Attempt to read stats for this interface
        const stats = linux_stats.readInterfaceStats(allocator, sysfs_root, name) catch {
            // Skip interfaces with missing/unreadable/malformed stats
            continue;
        };

        // Make an owned copy of the interface name
        const name_copy = try allocator.dupe(u8, name);
        errdefer allocator.free(name_copy);

        try snapshots.append(allocator, InterfaceStatsSnapshot{
            .name = name_copy,
            .stats = stats,
        });
    }

    // Free the temporary interface name list (names are copied into snapshots)
    linux_interfaces.freeInterfaceList(allocator, names);

    return try snapshots.toOwnedSlice(allocator);
}

/// Frees a list of interface stats snapshots returned by collectInterfaceStats().
///
/// This frees each snapshot's name and the snapshot slice itself.
pub fn freeInterfaceStatsSnapshots(
    allocator: std.mem.Allocator,
    snapshots: []InterfaceStatsSnapshot,
) void {
    for (snapshots) |snap| allocator.free(snap.name);
    allocator.free(snapshots);
}
