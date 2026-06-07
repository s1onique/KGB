// config.zig — BFD session configuration
//
// Defines the configuration model for multihop BFD sessions,
// compatible with BIRD-style config:
//   protocol bfd {
//       multihop {
//           interval 800 ms;
//           multiplier 3;
//       };
//   }

const std = @import("std");

/// BFD session mode
pub const BfdMode = enum {
    /// Multihop BFD (UDP port 4784) for multi-hop paths
    multihop,
    // NOTE: Single-hop mode not implemented in this ACT
};

/// BFD session configuration
pub const BfdConfig = struct {
    /// Session mode (multihop for this ACT)
    mode: BfdMode = .multihop,
    /// Local address as string (e.g., "192.168.1.1")
    local_addr: []const u8,
    /// Peer address as string (e.g., "192.168.1.2")
    peer_addr: []const u8,
    /// Desired transmit interval in milliseconds
    interval_ms: u32 = 800,
    /// Detection multiplier (number of missed packets before declaring down)
    multiplier: u8 = 3,
    /// Local discriminator (assigned by us)
    local_discr: u32 = 0,
    /// Role (initiator or responder)
    role: Role = .initiator,
};

/// Session role
pub const Role = enum {
    /// We initiate the BFD session
    initiator,
    /// We respond to the peer's BFD session
    responder,
};

/// Calculates the detection timeout for a session.
/// Detection time = negotiated TX interval (remote's requirement) × detect multiplier
///
/// For interval 800ms, multiplier 3:
///   Detection timeout = 800ms × 3 = 2400ms
pub fn calculateDetectionTimeout(
    remote_required_min_rx_interval_us: u32,
    remote_detect_mult: u8,
) u32 {
    const interval_ms = (remote_required_min_rx_interval_us + 999) / 1000;
    return interval_ms * @as(u32, remote_detect_mult);
}

/// Returns the default detection timeout for the given config.
/// Uses the local interval_ms and multiplier to calculate the expected
/// detection time when the peer responds with matching requirements.
pub fn defaultDetectionTimeout(config: BfdConfig) u32 {
    return config.interval_ms * @as(u32, config.multiplier);
}

/// Convert milliseconds to microseconds.
pub fn msToUs(ms: u32) u32 {
    return ms * 1000;
}

test "calculateDetectionTimeout for BIRD-style config" {
    // BIRD config: interval 800 ms; multiplier 3
    // Remote's required_min_rx_interval = 800000 us
    // Remote's detect_mult = 3
    // Expected: 800ms × 3 = 2400ms
    const timeout = calculateDetectionTimeout(800_000, 3);
    try std.testing.expectEqual(@as(u32, 2400), timeout);
}

test "calculateDetectionTimeout with different values" {
    // 100ms interval, multiplier 5
    try std.testing.expectEqual(@as(u32, 500), calculateDetectionTimeout(100_000, 5));

    // 1000ms interval, multiplier 2
    try std.testing.expectEqual(@as(u32, 2000), calculateDetectionTimeout(1_000_000, 2));

    // 500ms interval, multiplier 7
    try std.testing.expectEqual(@as(u32, 3500), calculateDetectionTimeout(500_000, 7));
}

test "defaultDetectionTimeout" {
    const config = BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 800,
        .multiplier = 3,
    };
    try std.testing.expectEqual(@as(u32, 2400), defaultDetectionTimeout(config));
}

test "config defaults" {
    const config = BfdConfig{
        .local_addr = "192.168.1.1",
        .peer_addr = "192.168.1.2",
    };
    try std.testing.expectEqual(BfdMode.multihop, config.mode);
    try std.testing.expectEqual(@as(u32, 800), config.interval_ms);
    try std.testing.expectEqual(@as(u8, 3), config.multiplier);
    try std.testing.expectEqual(Role.initiator, config.role);
}
