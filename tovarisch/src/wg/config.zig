// wg/config.zig — WireGuard configuration module
//
// This module provides WireGuard-specific configuration types and helpers.
// It re-exports WgConfig from the parent config.zig for convenience.

const std = @import("std");
const config = @import("../config.zig");
const peer = @import("peer.zig");

/// Re-export WgConfig for convenience in wg submodules.
pub const WgConfig = config.WgConfig;

/// Re-export ConfigError for convenience.
pub const ConfigError = config.ConfigError;

/// Re-export parseWgConfig for convenience.
pub const parseWgConfig = config.parseWgConfig;

/// Re-export WgPeer for convenience.
pub const WgPeer = peer.WgPeer;

/// Re-export PeerConfigError for convenience.
pub const PeerConfigError = peer.PeerConfigError;

/// Re-export parsePeerConfig for convenience.
pub const parsePeerConfig = peer.parsePeerConfig;

/// Parse all [wg.peer.<name>] sections from raw config.
/// Returns a list of all parsed peers.
/// Fails loudly if any enabled peer has malformed config.
/// Disabled peers with malformed config are skipped silently.
pub fn parseAllPeerConfigs(
    raw: *const config.RawConfig,
    allocator: std.mem.Allocator,
) (config.ConfigError || error{OutOfMemory})![]WgPeer {
    var peers = std.ArrayList(WgPeer).empty;
    defer peers.deinit(allocator);

    // Iterate over all sections looking for wg.peer.* patterns
    var it = raw.iterator();
    while (it.next()) |entry| {
        const section_name = entry.key_ptr.*;
        if (std.mem.startsWith(u8, section_name, "wg.peer.")) {
            const peer_name = section_name[8..]; // Skip "wg.peer." prefix
            if (peer_name.len == 0) continue; // Skip empty names

            const section = entry.value_ptr.*;
            const p = parsePeerConfig(peer_name, &section) catch |e| {
                // For enabled peers, propagate the error loudly
                if (config.getString(section, "enabled")) |enabled_val| {
                    const enabled = config.parseBool(enabled_val) catch return config.ConfigError.InvalidValue;
                    if (enabled) return e;
                }
                // Disabled malformed peers are skipped silently
                continue;
            };
            try peers.append(allocator, p);
        }
    }

    return try peers.toOwnedSlice(allocator);
}
