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
// Tunnel Classification
// ============================================================================

/// Tunnel interface name prefixes that indicate tunnel-like interfaces.
/// These are name-based heuristics for Linux network interfaces:
/// - wg*   : WireGuard interfaces (digits only, e.g., wg0, wg1, wg42)
/// - tun*  : TUN (network tunnel) interfaces (OpenVPN, etc.)
/// - tap*  : TAP (ethernet tunnel) interfaces
/// - vpns* : OpenConnect / ocserv tunnel interfaces (e.g., vpns0, vpns1)
/// - sit*  : SIT (Simple Internet Transition) tunnel
/// - ip6tnl*: IPv6 tunnel interfaces
/// - gre*  : GRE tunnel interfaces
/// - ipip* : IP-in-IP tunnel interfaces
pub const tunnel_prefixes = [_][]const u8{
    "wg",
    "tun",
    "tap",
    "vpns",
    "sit",
    "ip6tnl",
    "gre",
    "ipip",
};

/// Checks if an interface name matches tunnel interface patterns.
///
/// This is a name-based heuristic classifier for Linux network interfaces.
/// It does NOT inspect interface flags, metadata, or actual tunnel configuration.
///
/// WireGuard naming patterns:
/// - "wg" alone is valid (bare WireGuard device)
/// - "wg" followed by digits (wg0, wg1, wg42) - standard WireGuard naming
/// - "wg-" prefix followed by anything (wg-kgb0, wg-tunnel, wg-custom) - KGB naming
///
/// This relaxed detection accepts both "wg0" and "wg-kgb0" patterns while
/// still excluding non-tunnel names like "wga", "wgh", "wg-peer1".
pub fn isTunnelInterface(iface: []const u8) bool {
    // Check WireGuard: must start with "wg"
    if (std.mem.startsWith(u8, iface, "wg")) {
        if (iface.len < 3) return true; // exactly "wg" is valid
        const c = iface[2];
        // Accept: digit (wg0, wg1) or hyphen (wg-kgb0, wg-tunnel)
        return (c >= '0' and c <= '9') or c == '-';
    }

    // Check other prefixes
    for (tunnel_prefixes[1..]) |prefix| {
        if (std.mem.startsWith(u8, iface, prefix)) return true;
    }
    return false;
}

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
