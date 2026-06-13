// bgp/same_as_regression_tests.zig — Same-AS/iBGP AS_PATH regression tests
//
// REGRESSION: loadConfigAndBgp() auto-derives same_as=true when local_as == peer_as.
// This ensures empty AS_PATH in UPDATE messages per RFC 4271.
//
// Tests exercise the buildSessionConfig() path which is the production
// auto-derivation logic used by loadConfigAndBgp().

const std = @import("std");
const config = @import("../config.zig");
const config_parse = @import("config_parse.zig");
const session_config_builder = @import("session_config_builder.zig");
const session = @import("session.zig");
const types = @import("types.zig");
const frame_decode = @import("frame_decode.zig");

// REGRESSION: buildSessionConfig auto-derives same_as=true when local_as == peer_as.
// This test calls buildSessionConfig() which is the production path used by loadConfigAndBgp().
test "REGRESSION: buildSessionConfig auto-derives same_as=true when local_as == peer_as" {
    // Create BgpConfig with same AS on both ends (no same_as specified)
    const bgp_cfg = config_parse.BgpConfig{
        .present = true,
        .enabled = true,
        .local_address = "10.0.0.1",
        .router_id = "10.0.0.1",
        .local_as = 65001,
        .peer_address = "10.0.0.2",
        .peer_as = 65001,
        .advertised_prefixes_raw = "192.168.0.0/16",
        .same_as = false, // NOT specified - auto-derivation is required
    };

    // Parse prefix for the call
    const prefix = try types.Ipv4Prefix.parse("192.168.0.0/16");
    const prefixes_slice: []const types.Ipv4Prefix = &.{prefix};

    // Call buildSessionConfig - this is the production path used by loadConfigAndBgp()
    const sess_cfg = try session_config_builder.buildSessionConfig(bgp_cfg, prefixes_slice);

    // CRITICAL assertion: same_as must be auto-derived to true
    try std.testing.expect(sess_cfg.same_as);
    try std.testing.expectEqual(@as(u16, 65001), sess_cfg.local_as);
    try std.testing.expectEqual(@as(u16, 65001), sess_cfg.peer_as);
}

// REGRESSION: buildSessionConfig keeps same_as=false when local_as != peer_as.
test "REGRESSION: buildSessionConfig keeps same_as=false when local_as != peer_as" {
    const bgp_cfg = config_parse.BgpConfig{
        .present = true,
        .enabled = true,
        .local_address = "10.0.0.1",
        .router_id = "10.0.0.1",
        .local_as = 65001,
        .peer_address = "10.0.0.2",
        .peer_as = 65002, // Different AS
        .advertised_prefixes_raw = "192.168.0.0/16",
        .same_as = false,
    };

    const prefix = try types.Ipv4Prefix.parse("192.168.0.0/16");
    const prefixes_slice: []const types.Ipv4Prefix = &.{prefix};

    const sess_cfg = try session_config_builder.buildSessionConfig(bgp_cfg, prefixes_slice);

    // same_as must remain false (different AS)
    try std.testing.expect(!sess_cfg.same_as);
    try std.testing.expectEqual(@as(u16, 65001), sess_cfg.local_as);
    try std.testing.expectEqual(@as(u16, 65002), sess_cfg.peer_as);
}

// REGRESSION: parseBgpConfig same_as defaults false when not specified.
test "REGRESSION: parseBgpConfig same_as defaults false when not specified" {
    const raw = config_parse.BgpConfig{};
    // same_as should default to false
    try std.testing.expect(!raw.same_as);
}

// REGRESSION: same_as=true produces attrs_len=14 (ORIGIN(4) + AS_PATH(3 empty) + NEXT_HOP(7)).
test "REGRESSION: same_as=true produces empty AS_PATH wire format" {
    var peer_open: [29]u8 = undefined;
    @memset(peer_open[0..16], 0xFF);
    peer_open[16] = 0;
    peer_open[17] = 29;
    peer_open[18] = 1;
    peer_open[19] = 4;
    peer_open[20] = 0xFD;
    peer_open[21] = 0xE9;
    peer_open[22] = 0;
    peer_open[23] = 180;
    peer_open[24] = 10;
    peer_open[25] = 0;
    peer_open[26] = 0;
    peer_open[27] = 2;
    peer_open[28] = 0;

    var peer_keepalive: [19]u8 = undefined;
    @memset(peer_keepalive[0..16], 0xFF);
    peer_keepalive[16] = 0;
    peer_keepalive[17] = 19;
    peer_keepalive[18] = 4;

    var fake = try session.FakeTransport.init(std.testing.allocator, &.{
        session.PeerResponse{ .recv_bytes = &peer_open },
        session.PeerResponse{ .recv_bytes = &peer_keepalive },
    });
    defer fake.deinit();
    const trans = fake.toTransport();

    const sess_cfg = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = .{ 127, 0, 0, 1 },
        .local_as = 65001,
        .peer_as = 65001,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{types.Ipv4Prefix.init("10.0.0.0/8")},
        .same_as = true,
    };

    var sess = try session.initWithClock(sess_cfg, &trans, session.MockClock.interface());
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);

    const all_sent = fake.getAllSent();
    var offset: usize = 0;
    var update_body: ?frame_decode.UpdateBody = null;

    while (offset < all_sent.len) {
        if (offset + 19 > all_sent.len) break;
        const declared_len = @as(u16, all_sent[offset + 16]) * 256 + @as(u16, all_sent[offset + 17]);
        if (declared_len < 19 or declared_len > 4096 or offset + declared_len > all_sent.len) break;
        const frame_buf = all_sent[offset..][0..declared_len];
        const frame = frame_decode.decodeFrame(frame_buf) catch break;
        if (frame_decode.isUpdate(frame)) {
            update_body = frame_decode.parseUpdateBody(frame);
            break;
        }
        offset += declared_len;
    }

    try std.testing.expect(update_body != null);
    // CRITICAL: attrs_len must be 14 (ORIGIN(4) + AS_PATH(3 empty) + NEXT_HOP(7))
    try std.testing.expectEqual(@as(u16, 14), update_body.?.path_attributes_length);
    try std.testing.expectEqual(@as(u16, 0), update_body.?.withdrawn_routes_length);
    try std.testing.expectEqual(@as(usize, 1), update_body.?.nlri_prefix_count);
}

// REGRESSION: different-AS produces attrs_len=18 (ORIGIN(4) + AS_PATH(7) + NEXT_HOP(7)).
test "REGRESSION: different-AS produces non-empty AS_PATH wire format" {
    var peer_open: [29]u8 = undefined;
    @memset(peer_open[0..16], 0xFF);
    peer_open[16] = 0;
    peer_open[17] = 29;
    peer_open[18] = 1;
    peer_open[19] = 4;
    peer_open[20] = 0xFD;
    peer_open[21] = 0xEA;
    peer_open[22] = 0;
    peer_open[23] = 180;
    peer_open[24] = 10;
    peer_open[25] = 0;
    peer_open[26] = 0;
    peer_open[27] = 2;
    peer_open[28] = 0;

    var peer_keepalive: [19]u8 = undefined;
    @memset(peer_keepalive[0..16], 0xFF);
    peer_keepalive[16] = 0;
    peer_keepalive[17] = 19;
    peer_keepalive[18] = 4;

    var fake = try session.FakeTransport.init(std.testing.allocator, &.{
        session.PeerResponse{ .recv_bytes = &peer_open },
        session.PeerResponse{ .recv_bytes = &peer_keepalive },
    });
    defer fake.deinit();
    const trans = fake.toTransport();

    const sess_cfg = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = .{ 127, 0, 0, 1 },
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{types.Ipv4Prefix.init("10.0.0.0/8")},
        .same_as = false,
    };

    var sess = try session.initWithClock(sess_cfg, &trans, session.MockClock.interface());
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);

    const all_sent = fake.getAllSent();
    var offset: usize = 0;

    while (offset < all_sent.len) {
        if (offset + 19 > all_sent.len) break;
        const declared_len = @as(u16, all_sent[offset + 16]) * 256 + @as(u16, all_sent[offset + 17]);
        if (declared_len < 19 or declared_len > 4096 or offset + declared_len > all_sent.len) break;
        const frame_buf = all_sent[offset..][0..declared_len];
        const frame = frame_decode.decodeFrame(frame_buf) catch break;
        if (frame_decode.isUpdate(frame)) {
            const ub = frame_decode.parseUpdateBody(frame);
            try std.testing.expect(ub != null);
            // CRITICAL: attrs_len must be 18 (ORIGIN(4) + AS_PATH(7) + NEXT_HOP(7))
            try std.testing.expectEqual(@as(u16, 18), ub.?.path_attributes_length);
            try std.testing.expectEqual(@as(u16, 0), ub.?.withdrawn_routes_length);
            try std.testing.expectEqual(@as(usize, 1), ub.?.nlri_prefix_count);
            break;
        }
        offset += declared_len;
    }
}
