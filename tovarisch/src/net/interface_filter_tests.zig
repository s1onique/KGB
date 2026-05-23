// interface_filter_tests.zig — Tests for private interface filtering
//
// Tests cover:
// 1. Interface with RFC1918 IPv4 address is included
// 2. Interface with public IPv4 only is excluded
// 3. Interface with multiple addresses is included if any address is private
// 4. Interface with no addresses is excluded
// 5. Multiple snapshots filter independently
// 6. Malformed address is ignored
// 7. IPv6 ULA handling (per private_ip.zig semantics)
// 8. Loopback and link-local behavior
// 9. Filter output owns its names
// 10. Input snapshots are not modified
// 11. Empty snapshots returns empty output
// 12. Empty addresses returns empty output

const std = @import("std");
const testing = std.testing;
const interface_filter = @import("interface_filter.zig");
const linux_interface_stats = @import("linux_interface_stats.zig");
const linux_stats = @import("linux_stats.zig");
const private_ip = @import("private_ip.zig");

// ============================================================================
// Test Fixtures
// ============================================================================

/// Helper to create a simple stats struct.
fn makeStats() linux_stats.InterfaceStats {
    return .{
        .rx_bytes = 100,
        .tx_bytes = 200,
        .rx_packets = 10,
        .tx_packets = 20,
    };
}

// ============================================================================
// interfaceHasPrivateAddress Tests
// ============================================================================

test "interfaceHasPrivateAddress: RFC1918 10.x.x.x included" {
    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "10.0.0.5" },
    };

    try testing.expect(interface_filter.interfaceHasPrivateAddress("eth0", &addresses));
}

test "interfaceHasPrivateAddress: RFC1918 172.16.x.x included" {
    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "172.16.0.1" },
    };

    try testing.expect(interface_filter.interfaceHasPrivateAddress("eth0", &addresses));
}

test "interfaceHasPrivateAddress: RFC1918 192.168.x.x included" {
    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "192.168.1.10" },
    };

    try testing.expect(interface_filter.interfaceHasPrivateAddress("eth0", &addresses));
}

test "interfaceHasPrivateAddress: public IPv4 excluded" {
    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "8.8.8.8" },
    };

    try testing.expect(!interface_filter.interfaceHasPrivateAddress("eth0", &addresses));
}

test "interfaceHasPrivateAddress: multiple addresses, public only = excluded" {
    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "1.1.1.1" },
        .{ .iface = "eth0", .address = "8.8.8.8" },
    };

    try testing.expect(!interface_filter.interfaceHasPrivateAddress("eth0", &addresses));
}

test "interfaceHasPrivateAddress: multiple addresses, one private = included" {
    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "8.8.8.8" },
        .{ .iface = "eth0", .address = "10.0.0.5" },
    };

    try testing.expect(interface_filter.interfaceHasPrivateAddress("eth0", &addresses));
}

test "interfaceHasPrivateAddress: interface with no addresses = excluded" {
    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "192.168.1.10" },
    };

    try testing.expect(!interface_filter.interfaceHasPrivateAddress("wan0", &addresses));
}

test "interfaceHasPrivateAddress: empty addresses = excluded" {
    const addresses: [0]interface_filter.InterfaceAddress = .{};

    try testing.expect(!interface_filter.interfaceHasPrivateAddress("eth0", &addresses));
}

test "interfaceHasPrivateAddress: malformed address ignored" {
    // Malformed addresses are classified as .invalid by private_ip.zig,
    // so they should not cause inclusion.
    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "not.an.address" },
    };

    try testing.expect(!interface_filter.interfaceHasPrivateAddress("eth0", &addresses));
}

test "interfaceHasPrivateAddress: loopback excluded (not private)" {
    // Loopback addresses are classified as .loopback, not .private
    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "lo", .address = "127.0.0.1" },
    };

    try testing.expect(!interface_filter.interfaceHasPrivateAddress("lo", &addresses));
}

test "interfaceHasPrivateAddress: link-local excluded (not private)" {
    // Link-local addresses are classified as .link_local, not .private
    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "169.254.1.2" },
    };

    try testing.expect(!interface_filter.interfaceHasPrivateAddress("eth0", &addresses));
}

test "interfaceHasPrivateAddress: carrier NAT excluded (not private)" {
    // Carrier NAT addresses are classified as .carrier_nat, not .private
    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "100.64.0.1" },
    };

    try testing.expect(!interface_filter.interfaceHasPrivateAddress("eth0", &addresses));
}

test "interfaceHasPrivateAddress: documentation excluded (not private)" {
    // Documentation addresses are classified as .documentation, not .private
    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "192.0.2.1" },
    };

    try testing.expect(!interface_filter.interfaceHasPrivateAddress("eth0", &addresses));
}

test "interfaceHasPrivateAddress: multicast excluded (not private)" {
    // Multicast addresses are classified as .multicast, not .private
    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "224.0.0.1" },
    };

    try testing.expect(!interface_filter.interfaceHasPrivateAddress("eth0", &addresses));
}

// ============================================================================
// filterPrivateInterfaceStats Tests
// ============================================================================

test "filterPrivateInterfaceStats: IPv6 addresses handled correctly" {
    // IPv6 addresses are not handled by current private_ip.zig (IPv4 only)
    // This test documents expected behavior: IPv6 strings are classified
    // as .invalid by classifyIpv4Text(), so IPv6-only interfaces would be excluded.
    // This is acceptable since the current scope is IPv4 private detection.
    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "fd00::1" },
    };

    // fd00::1 is not a valid IPv4, so it won't be classified as private
    try testing.expect(!interface_filter.interfaceHasPrivateAddress("eth0", &addresses));
}

// ============================================================================
// Integration-style Tests with Owned Snapshots
// ============================================================================

test "filterPrivateInterfaceStats: end-to-end with RFC1918" {
    const allocator = testing.allocator;

    // Create test snapshots
    const eth0_name = try allocator.dupe(u8, "eth0");
    defer allocator.free(eth0_name);

    const snapshots = [_]linux_interface_stats.InterfaceStatsSnapshot{
        .{
            .name = eth0_name,
            .stats = .{
                .rx_bytes = 1000,
                .tx_bytes = 2000,
                .rx_packets = 10,
                .tx_packets = 20,
            },
        },
    };

    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "192.168.1.10" },
    };

    const filtered = try interface_filter.filterPrivateInterfaceStats(
        allocator,
        &snapshots,
        &addresses,
    );
    defer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, filtered);

    try testing.expectEqual(@as(usize, 1), filtered.len);
    try testing.expectEqualSlices(u8, "eth0", filtered[0].name);
    try testing.expectEqual(@as(u64, 1000), filtered[0].stats.rx_bytes);
}

test "filterPrivateInterfaceStats: end-to-end public excluded" {
    const allocator = testing.allocator;

    const eth0_name = try allocator.dupe(u8, "eth0");
    defer allocator.free(eth0_name);

    const snapshots = [_]linux_interface_stats.InterfaceStatsSnapshot{
        .{
            .name = eth0_name,
            .stats = .{
                .rx_bytes = 1000,
                .tx_bytes = 2000,
                .rx_packets = 10,
                .tx_packets = 20,
            },
        },
    };

    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "8.8.8.8" },
    };

    const filtered = try interface_filter.filterPrivateInterfaceStats(
        allocator,
        &snapshots,
        &addresses,
    );
    defer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, filtered);

    try testing.expectEqual(@as(usize, 0), filtered.len);
}

test "filterPrivateInterfaceStats: end-to-end multiple interfaces" {
    const allocator = testing.allocator;

    const eth0_name = try allocator.dupe(u8, "eth0");
    defer allocator.free(eth0_name);
    const wan0_name = try allocator.dupe(u8, "wan0");
    defer allocator.free(wan0_name);
    const wg0_name = try allocator.dupe(u8, "wg0");
    defer allocator.free(wg0_name);

    const snapshots = [_]linux_interface_stats.InterfaceStatsSnapshot{
        .{
            .name = eth0_name,
            .stats = .{ .rx_bytes = 1000, .tx_bytes = 2000, .rx_packets = 10, .tx_packets = 20 },
        },
        .{
            .name = wan0_name,
            .stats = .{ .rx_bytes = 3000, .tx_bytes = 4000, .rx_packets = 30, .tx_packets = 40 },
        },
        .{
            .name = wg0_name,
            .stats = .{ .rx_bytes = 5000, .tx_bytes = 6000, .rx_packets = 50, .tx_packets = 60 },
        },
    };

    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "192.168.1.10" },
        .{ .iface = "wan0", .address = "8.8.8.8" },
        .{ .iface = "wg0", .address = "10.0.0.1" },
    };

    const filtered = try interface_filter.filterPrivateInterfaceStats(
        allocator,
        &snapshots,
        &addresses,
    );
    defer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, filtered);

    // eth0 (192.168.x.x private) included, wan0 (public) excluded, wg0 (10.x.x.x private) included
    try testing.expectEqual(@as(usize, 2), filtered.len);

    // Names should be owned copies
    try testing.expectEqualSlices(u8, "eth0", filtered[0].name);
    try testing.expectEqualSlices(u8, "wg0", filtered[1].name);

    // Stats should be copied correctly
    try testing.expectEqual(@as(u64, 1000), filtered[0].stats.rx_bytes);
    try testing.expectEqual(@as(u64, 5000), filtered[1].stats.rx_bytes);
}

test "filterPrivateInterfaceStats: end-to-end no addresses for interface" {
    const allocator = testing.allocator;

    const eth0_name = try allocator.dupe(u8, "eth0");
    defer allocator.free(eth0_name);
    const wg0_name = try allocator.dupe(u8, "wg0");
    defer allocator.free(wg0_name);

    const snapshots = [_]linux_interface_stats.InterfaceStatsSnapshot{
        .{
            .name = eth0_name,
            .stats = .{ .rx_bytes = 1000, .tx_bytes = 2000, .rx_packets = 10, .tx_packets = 20 },
        },
        .{
            .name = wg0_name,
            .stats = .{ .rx_bytes = 5000, .tx_bytes = 6000, .rx_packets = 50, .tx_packets = 60 },
        },
    };

    // Only eth0 has an address (public), wg0 has no addresses
    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "8.8.8.8" },
    };

    const filtered = try interface_filter.filterPrivateInterfaceStats(
        allocator,
        &snapshots,
        &addresses,
    );
    defer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, filtered);

    // eth0 (public) excluded, wg0 (no addresses) excluded
    try testing.expectEqual(@as(usize, 0), filtered.len);
}

test "filterPrivateInterfaceStats: end-to-end empty snapshots" {
    const allocator = testing.allocator;

    const snapshots: [0]linux_interface_stats.InterfaceStatsSnapshot = .{};

    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "192.168.1.10" },
    };

    const filtered = try interface_filter.filterPrivateInterfaceStats(
        allocator,
        &snapshots,
        &addresses,
    );
    defer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, filtered);

    try testing.expectEqual(@as(usize, 0), filtered.len);
}

test "filterPrivateInterfaceStats: end-to-end empty addresses" {
    const allocator = testing.allocator;

    const eth0_name = try allocator.dupe(u8, "eth0");
    defer allocator.free(eth0_name);

    const snapshots = [_]linux_interface_stats.InterfaceStatsSnapshot{
        .{
            .name = eth0_name,
            .stats = .{ .rx_bytes = 1000, .tx_bytes = 2000, .rx_packets = 10, .tx_packets = 20 },
        },
    };

    const addresses: [0]interface_filter.InterfaceAddress = .{};

    const filtered = try interface_filter.filterPrivateInterfaceStats(
        allocator,
        &snapshots,
        &addresses,
    );
    defer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, filtered);

    // eth0 has no addresses in the addresses list, so it's excluded
    try testing.expectEqual(@as(usize, 0), filtered.len);
}

test "filterPrivateInterfaceStats: end-to-end mixed addresses for same interface" {
    const allocator = testing.allocator;

    const eth0_name = try allocator.dupe(u8, "eth0");
    defer allocator.free(eth0_name);

    const snapshots = [_]linux_interface_stats.InterfaceStatsSnapshot{
        .{
            .name = eth0_name,
            .stats = .{ .rx_bytes = 1000, .tx_bytes = 2000, .rx_packets = 10, .tx_packets = 20 },
        },
    };

    // eth0 has both public and private addresses
    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "8.8.8.8" },
        .{ .iface = "eth0", .address = "10.0.0.5" },
    };

    const filtered = try interface_filter.filterPrivateInterfaceStats(
        allocator,
        &snapshots,
        &addresses,
    );
    defer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, filtered);

    // eth0 has a private address (10.0.0.5), so it's included
    try testing.expectEqual(@as(usize, 1), filtered.len);
    try testing.expectEqualSlices(u8, "eth0", filtered[0].name);
}

test "filterPrivateInterfaceStats: input snapshots not freed or modified" {
    const allocator = testing.allocator;

    const eth0_name = try allocator.dupe(u8, "eth0");
    defer allocator.free(eth0_name);

    var snapshots = std.ArrayList(linux_interface_stats.InterfaceStatsSnapshot).empty;
    defer snapshots.deinit(allocator);

    try snapshots.append(allocator, .{
        .name = eth0_name,
        .stats = .{ .rx_bytes = 1000, .tx_bytes = 2000, .rx_packets = 10, .tx_packets = 20 },
    });

    const addresses = [_]interface_filter.InterfaceAddress{
        .{ .iface = "eth0", .address = "192.168.1.10" },
    };

    // Get a slice reference to pass to filter
    const snapshots_slice = try snapshots.toOwnedSlice(allocator);
    defer allocator.free(snapshots_slice);

    // Store original name pointer to verify it's not modified
    const original_name_ptr = snapshots_slice[0].name.ptr;

    const filtered = try interface_filter.filterPrivateInterfaceStats(
        allocator,
        snapshots_slice,
        &addresses,
    );
    defer linux_interface_stats.freeInterfaceStatsSnapshots(allocator, filtered);

    // Original snapshot name should still be valid
    try testing.expectEqualSlices(u8, "eth0", snapshots_slice[0].name);
    // Pointer should be unchanged (name not freed)
    try testing.expectEqual(original_name_ptr, snapshots_slice[0].name.ptr);
}
