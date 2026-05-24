// tunnel_classification_tests.zig — Tests for tunnel interface classification
//
// Tests cover name-based tunnel interface classification:
// - WireGuard (wg*)
// - TUN (tun*)
// - TAP (tap*)
// - SIT (sit*)
// - IPv6 tunnel (ip6tnl*)
// - GRE (gre*)
// - IP-in-IP (ipip*)

const std = @import("std");
const testing = std.testing;
const interface_filter = @import("interface_filter.zig");

// ============================================================================
// Tunnel Interface Classification Tests
// ============================================================================

test "isTunnelInterface: WireGuard interfaces" {
    // Valid WireGuard: wg followed by digits
    try testing.expect(interface_filter.isTunnelInterface("wg0"));
    try testing.expect(interface_filter.isTunnelInterface("wg1"));
    try testing.expect(interface_filter.isTunnelInterface("wg42"));
    try testing.expect(interface_filter.isTunnelInterface("wg99"));

    // Edge case: "wg" alone is valid
    try testing.expect(interface_filter.isTunnelInterface("wg"));

    // WireGuard must be followed by digit - these are NOT tunnel interfaces
    try testing.expect(!interface_filter.isTunnelInterface("wga"));
    try testing.expect(!interface_filter.isTunnelInterface("wgh"));
    try testing.expect(!interface_filter.isTunnelInterface("wg-peer1"));
    try testing.expect(!interface_filter.isTunnelInterface("wgx"));
}

test "isTunnelInterface: TUN interfaces" {
    try testing.expect(interface_filter.isTunnelInterface("tun0"));
    try testing.expect(interface_filter.isTunnelInterface("tun1"));
    try testing.expect(interface_filter.isTunnelInterface("tunvpn"));
}

test "isTunnelInterface: TAP interfaces" {
    try testing.expect(interface_filter.isTunnelInterface("tap0"));
    try testing.expect(interface_filter.isTunnelInterface("tap1"));
    try testing.expect(interface_filter.isTunnelInterface("tapbridge"));
}

test "isTunnelInterface: SIT tunnel interfaces" {
    try testing.expect(interface_filter.isTunnelInterface("sit0"));
    try testing.expect(interface_filter.isTunnelInterface("sit1"));
}

test "isTunnelInterface: IPv6 tunnel interfaces" {
    try testing.expect(interface_filter.isTunnelInterface("ip6tnl0"));
    try testing.expect(interface_filter.isTunnelInterface("ip6tnl1"));
}

test "isTunnelInterface: GRE tunnel interfaces" {
    try testing.expect(interface_filter.isTunnelInterface("gre0"));
    try testing.expect(interface_filter.isTunnelInterface("gretap0"));
}

test "isTunnelInterface: IP-in-IP tunnel interfaces" {
    try testing.expect(interface_filter.isTunnelInterface("ipip0"));
    try testing.expect(interface_filter.isTunnelInterface("ipip1"));
}

test "isTunnelInterface: non-tunnel interfaces excluded" {
    // Regular ethernet interfaces
    try testing.expect(!interface_filter.isTunnelInterface("eth0"));
    try testing.expect(!interface_filter.isTunnelInterface("eth1"));
    try testing.expect(!interface_filter.isTunnelInterface("ens0"));
    try testing.expect(!interface_filter.isTunnelInterface("enp0s0"));

    // WiFi interfaces
    try testing.expect(!interface_filter.isTunnelInterface("wlan0"));
    try testing.expect(!interface_filter.isTunnelInterface("wlp3s0"));

    // Loopback
    try testing.expect(!interface_filter.isTunnelInterface("lo"));

    // Bridge
    try testing.expect(!interface_filter.isTunnelInterface("br0"));
    try testing.expect(!interface_filter.isTunnelInterface("bridge0"));

    // VLAN
    try testing.expect(!interface_filter.isTunnelInterface("vlan0"));
    try testing.expect(!interface_filter.isTunnelInterface("eth0.100"));
}

test "isTunnelInterface: edge cases" {
    // Empty string is not a tunnel
    try testing.expect(!interface_filter.isTunnelInterface(""));

    // Single character is not a tunnel
    try testing.expect(!interface_filter.isTunnelInterface("w"));
    try testing.expect(!interface_filter.isTunnelInterface("t"));

    // Partial prefix match - these start with 'wg' but aren't 'wg' prefix
    try testing.expect(!interface_filter.isTunnelInterface("wga"));
    try testing.expect(!interface_filter.isTunnelInterface("wgh"));
}

test "isTunnelInterface: tunnel prefixes array is non-empty" {
    try testing.expect(interface_filter.tunnel_prefixes.len > 0);
}

test "isTunnelInterface: all prefixes in array start expected patterns" {
    // Verify the prefixes array contains expected entries
    const prefixes = interface_filter.tunnel_prefixes;
    try testing.expect(std.mem.containsAtLeast([]const u8, prefixes, 1, "wg"));
    try testing.expect(std.mem.containsAtLeast([]const u8, prefixes, 1, "tun"));
    try testing.expect(std.mem.containsAtLeast([]const u8, prefixes, 1, "tap"));
}
