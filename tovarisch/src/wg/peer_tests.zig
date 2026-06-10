// wg/peer_tests.zig — Tests for WireGuard peer configuration
//
// Tests for peer config parsing and validation.

const std = @import("std");
const config = @import("../config.zig");
const peer = @import("peer.zig");
const wg_args = @import("../cli/wg_args.zig");
const wg_config = @import("config.zig");

test "parseAllPeerConfigs finds no peers in empty config" {
    const content = "[wg]\nenabled = true\ninterface = wg0";
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);

    try wg_args.parseIniContent(content, &raw, std.heap.page_allocator);

    const peers = try wg_config.parseAllPeerConfigs(&raw, std.heap.page_allocator);
    defer std.heap.page_allocator.free(peers);

    try std.testing.expectEqual(@as(usize, 0), peers.len);
}

test "parseAllPeerConfigs finds one peer" {
    const content =
        \\[wg]
        \\enabled = true
        \\interface = wg0
        \\
        \\[wg.peer.phone]
        \\enabled = true
        \\address = 10.149.149.10/32
        \\private_key_file = /etc/kgb/peers/phone.key
        \\public_key_file = /etc/kgb/peers/phone.pub
        \\allowed_ips = 10.149.149.10/32
        \\client_output_file = /var/lib/kgb/clients/phone.conf
    ;
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);

    try wg_args.parseIniContent(content, &raw, std.heap.page_allocator);

    const peers = try wg_config.parseAllPeerConfigs(&raw, std.heap.page_allocator);
    defer std.heap.page_allocator.free(peers);

    try std.testing.expectEqual(@as(usize, 1), peers.len);
    try std.testing.expectEqualStrings("phone", peers[0].name);
    try std.testing.expect(peers[0].enabled);
}

test "parseAllPeerConfigs finds multiple peers" {
    const content =
        \\[wg]
        \\enabled = true
        \\
        \\[wg.peer.phone]
        \\enabled = true
        \\address = 10.149.149.10/32
        \\private_key_file = /etc/kgb/peers/phone.key
        \\public_key_file = /etc/kgb/peers/phone.pub
        \\allowed_ips = 10.149.149.10/32
        \\client_output_file = /var/lib/kgb/clients/phone.conf
        \\
        \\[wg.peer.laptop]
        \\enabled = true
        \\address = 10.149.149.11/32
        \\private_key_file = /etc/kgb/peers/laptop.key
        \\public_key_file = /etc/kgb/peers/laptop.pub
        \\allowed_ips = 10.149.149.11/32
        \\client_output_file = /var/lib/kgb/clients/laptop.conf
    ;
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);

    try wg_args.parseIniContent(content, &raw, std.heap.page_allocator);

    const peers = try wg_config.parseAllPeerConfigs(&raw, std.heap.page_allocator);
    defer std.heap.page_allocator.free(peers);

    try std.testing.expectEqual(@as(usize, 2), peers.len);
}

test "parseAllPeerConfigs skips disabled peers" {
    const content =
        \\[wg]
        \\enabled = true
        \\
        \\[wg.peer.phone]
        \\enabled = false
        \\address = 10.149.149.10/32
        \\private_key_file = /etc/kgb/peers/phone.key
        \\public_key_file = /etc/kgb/peers/phone.pub
        \\allowed_ips = 10.149.149.10/32
        \\client_output_file = /var/lib/kgb/clients/phone.conf
    ;
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);

    try wg_args.parseIniContent(content, &raw, std.heap.page_allocator);

    const peers = try wg_config.parseAllPeerConfigs(&raw, std.heap.page_allocator);
    defer std.heap.page_allocator.free(peers);

    // Disabled peers should still be in the list but not enabled
    try std.testing.expectEqual(@as(usize, 1), peers.len);
    try std.testing.expect(!peers[0].enabled);
}

test "parsePeerConfig requires all mandatory fields for enabled peer" {
    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    defer section.deinit(std.heap.page_allocator);

    // Missing address
    try section.put(std.heap.page_allocator, "enabled", "true");
    try section.put(std.heap.page_allocator, "private_key_file", "/path/to/key");
    try section.put(std.heap.page_allocator, "public_key_file", "/path/to/pub");
    try section.put(std.heap.page_allocator, "allowed_ips", "10.0.0.1/32");
    try section.put(std.heap.page_allocator, "client_output_file", "/path/to/output");

    const result = peer.parsePeerConfig("test", &section);
    try std.testing.expectError(peer.PeerConfigError.MissingKey, result);
}

test "parsePeerConfig requires private_key_file" {
    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    defer section.deinit(std.heap.page_allocator);

    try section.put(std.heap.page_allocator, "enabled", "true");
    try section.put(std.heap.page_allocator, "address", "10.149.149.10/32");
    try section.put(std.heap.page_allocator, "public_key_file", "/path/to/pub");
    try section.put(std.heap.page_allocator, "allowed_ips", "10.149.149.10/32");
    try section.put(std.heap.page_allocator, "client_output_file", "/path/to/output");

    const result = peer.parsePeerConfig("test", &section);
    try std.testing.expectError(peer.PeerConfigError.MissingKey, result);
}

test "parsePeerConfig requires public_key_file" {
    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    defer section.deinit(std.heap.page_allocator);

    try section.put(std.heap.page_allocator, "enabled", "true");
    try section.put(std.heap.page_allocator, "address", "10.149.149.10/32");
    try section.put(std.heap.page_allocator, "private_key_file", "/path/to/key");
    try section.put(std.heap.page_allocator, "allowed_ips", "10.149.149.10/32");
    try section.put(std.heap.page_allocator, "client_output_file", "/path/to/output");

    const result = peer.parsePeerConfig("test", &section);
    try std.testing.expectError(peer.PeerConfigError.MissingKey, result);
}

test "parsePeerConfig requires allowed_ips" {
    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    defer section.deinit(std.heap.page_allocator);

    try section.put(std.heap.page_allocator, "enabled", "true");
    try section.put(std.heap.page_allocator, "address", "10.149.149.10/32");
    try section.put(std.heap.page_allocator, "private_key_file", "/path/to/key");
    try section.put(std.heap.page_allocator, "public_key_file", "/path/to/pub");
    try section.put(std.heap.page_allocator, "client_output_file", "/path/to/output");

    const result = peer.parsePeerConfig("test", &section);
    try std.testing.expectError(peer.PeerConfigError.MissingKey, result);
}

test "parsePeerConfig requires client_output_file" {
    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    defer section.deinit(std.heap.page_allocator);

    try section.put(std.heap.page_allocator, "enabled", "true");
    try section.put(std.heap.page_allocator, "address", "10.149.149.10/32");
    try section.put(std.heap.page_allocator, "private_key_file", "/path/to/key");
    try section.put(std.heap.page_allocator, "public_key_file", "/path/to/pub");
    try section.put(std.heap.page_allocator, "allowed_ips", "10.149.149.10/32");

    const result = peer.parsePeerConfig("test", &section);
    try std.testing.expectError(peer.PeerConfigError.MissingKey, result);
}

test "parsePeerConfig parses optional persistent_keepalive" {
    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    defer section.deinit(std.heap.page_allocator);

    try section.put(std.heap.page_allocator, "enabled", "true");
    try section.put(std.heap.page_allocator, "address", "10.149.149.10/32");
    try section.put(std.heap.page_allocator, "private_key_file", "/path/to/key");
    try section.put(std.heap.page_allocator, "public_key_file", "/path/to/pub");
    try section.put(std.heap.page_allocator, "allowed_ips", "10.149.149.10/32");
    try section.put(std.heap.page_allocator, "persistent_keepalive", "25");
    try section.put(std.heap.page_allocator, "client_output_file", "/path/to/output");

    const p = try peer.parsePeerConfig("test", &section);
    try std.testing.expect(p.persistent_keepalive != null);
    try std.testing.expectEqual(@as(u16, 25), p.persistent_keepalive.?);
}

test "parsePeerConfig parses optional endpoint" {
    var section = std.StringArrayHashMapUnmanaged([]const u8){};
    defer section.deinit(std.heap.page_allocator);

    try section.put(std.heap.page_allocator, "enabled", "true");
    try section.put(std.heap.page_allocator, "address", "10.149.149.10/32");
    try section.put(std.heap.page_allocator, "private_key_file", "/path/to/key");
    try section.put(std.heap.page_allocator, "public_key_file", "/path/to/pub");
    try section.put(std.heap.page_allocator, "allowed_ips", "10.149.149.10/32");
    try section.put(std.heap.page_allocator, "endpoint", "vpn.example.com:51821");
    try section.put(std.heap.page_allocator, "client_output_file", "/path/to/output");

    const p = try peer.parsePeerConfig("test", &section);
    try std.testing.expect(p.endpoint != null);
    try std.testing.expectEqualStrings("vpn.example.com:51821", p.endpoint.?);
}

// 44-character base64 key constant for tests
const valid_key_44 = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA";

test "validateKey accepts 44-char base64 without padding" {
    try peer.validateKey(valid_key_44);
}

test "validateKey rejects keys with invalid length after trim" {
    // Key with trailing spaces (should be stripped, leaving 44 valid chars)
    const key = valid_key_44 ++ "   ";
    try peer.validateKey(key);
}

test "parseAllPeerConfigs fails on enabled malformed peer" {
    const content =
        \\[wg]
        \\enabled = true
        \\interface = wg0
        \\
        \\[wg.peer.phone]
        \\enabled = true
        \\private_key_file = /etc/kgb/peers/phone.key
        \\public_key_file = /etc/kgb/peers/phone.pub
        \\allowed_ips = 10.149.149.10/32
        \\client_output_file = /var/lib/kgb/clients/phone.conf
    ;
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);

    try wg_args.parseIniContent(content, &raw, std.heap.page_allocator);

    // This should fail because the enabled peer is missing the address field
    const result = wg_config.parseAllPeerConfigs(&raw, std.heap.page_allocator);
    try std.testing.expectError(peer.PeerConfigError.MissingKey, result);
}

test "parseAllPeerConfigs skips disabled malformed peers" {
    const content =
        \\[wg]
        \\enabled = true
        \\interface = wg0
        \\
        \\[wg.peer.phone]
        \\enabled = false
        \\address = 10.149.149.10/32
        \\private_key_file = /etc/kgb/peers/phone.key
        \\public_key_file = /etc/kgb/peers/phone.pub
        \\allowed_ips = 10.149.149.10/32
        \\client_output_file = /var/lib/kgb/clients/phone.conf
    ;
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);

    try wg_args.parseIniContent(content, &raw, std.heap.page_allocator);

    // Disabled malformed peers should be skipped silently
    const peers = try wg_config.parseAllPeerConfigs(&raw, std.heap.page_allocator);
    defer std.heap.page_allocator.free(peers);

    try std.testing.expectEqual(@as(usize, 0), peers.len);
}

test "parseAllPeerConfigs fails on invalid enabled value" {
    const content =
        \\[wg]
        \\enabled = true
        \\interface = wg0
        \\
        \\[wg.peer.phone]
        \\enabled = maybe
        \\address = 10.149.149.10/32
        \\private_key_file = /etc/kgb/peers/phone.key
        \\public_key_file = /etc/kgb/peers/phone.pub
        \\allowed_ips = 10.149.149.10/32
        \\client_output_file = /var/lib/kgb/clients/phone.conf
    ;
    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);

    try wg_args.parseIniContent(content, &raw, std.heap.page_allocator);

    // Invalid enabled value should fail loudly
    const result = wg_config.parseAllPeerConfigs(&raw, std.heap.page_allocator);
    try std.testing.expectError(config.ConfigError.InvalidValue, result);
}
