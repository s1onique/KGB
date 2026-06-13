// session_update_capture_tests.zig — BGP UPDATE byte capture and structural decode tests
//
// ACT: Capture and decode runtime BGP UPDATE bytes sent by tovarisch session.
//
// Evidence: Unit tests prove message.encodeUpdate() places prefixes in NLRI with
// withdrawn_routes_length=0. But live BIRD reports "Import updates: 0" while
// "Import withdraws: 15810 ignored".
//
// This test captures actual sent bytes from the live session path (FakeTransport)
// and structurally decodes them to prove the wire format is correct.
//
// Acceptance criteria verified:
// - message type == UPDATE
// - withdrawn_routes_length == 0
// - total_path_attribute_length > 0
// - NLRI byte count > 0
// - Covers multi-batch prefix sets

const std = @import("std");
const session = @import("session.zig");
const types = @import("types.zig");
const frame_decode = @import("frame_decode.zig");

// === UPDATE Byte Capture Tests ===

test "session UPDATE capture: first batch has correct wire format" {
    // Complete BGP handshake with single prefix to capture first UPDATE batch
    var peer_open: [29]u8 = undefined;
    @memset(peer_open[0..16], 0xFF);
    peer_open[16] = 0;
    peer_open[17] = 29;
    peer_open[18] = 1; // OPEN
    peer_open[19] = 4; // version
    peer_open[20] = 0xFD; // peer AS = 65002
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
    peer_keepalive[18] = 4; // KEEPALIVE

    var fake = try session.FakeTransport.init(std.testing.allocator, &.{
        session.PeerResponse{ .recv_bytes = &peer_open },
        session.PeerResponse{ .recv_bytes = &peer_keepalive },
    });
    defer fake.deinit();
    const trans = fake.toTransport();

    const config = session.SessionConfig{
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
        .same_as = true,
    };

    var sess = try session.initWithClock(config, &trans, session.MockClock.interface());

    // Complete handshake
    _ = try session.runOnce(&sess); // Send OPEN
    _ = try session.runOnce(&sess); // OPEN -> KEEPALIVE
    _ = try session.runOnce(&sess); // KEEPALIVE -> established

    // Capture the UPDATE batch
    _ = try session.runOnce(&sess);

    // Get all captured bytes
    const all_sent = fake.getAllSent();

    // Parse all frames in the captured data
    var offset: usize = 0;
    var update_count: usize = 0;
    var update_batch: ?frame_decode.UpdateBody = null;

    while (offset < all_sent.len) {
        // Need at least header (19 bytes)
        if (offset + 19 > all_sent.len) break;

        const declared_len = @as(u16, all_sent[offset + 16]) * 256 + @as(u16, all_sent[offset + 17]);
        if (declared_len < 19 or declared_len > 4096) break;
        if (offset + declared_len > all_sent.len) break;

        const frame_buf = all_sent[offset..][0..declared_len];
        const frame = frame_decode.decodeFrame(frame_buf) catch break;

        // Focus on UPDATE frames
        if (frame_decode.isUpdate(frame)) {
            update_count += 1;
            const update_body = frame_decode.parseUpdateBody(frame);
            if (update_body) |ub| {
                // CRITICAL: withdrawn_routes_length MUST be 0
                try std.testing.expectEqual(@as(u16, 0), ub.withdrawn_routes_length);
                // path_attributes_length MUST be > 0
                try std.testing.expect(ub.path_attributes_length > 0);
                // NLRI MUST have prefixes
                try std.testing.expect(ub.nlri_prefix_count > 0);
                // NLRI byte count MUST be > 0
                try std.testing.expect(ub.nlri_byte_count > 0);
                update_batch = ub;
            }
        }

        offset += declared_len;
    }

    // Verify we captured exactly 1 UPDATE frame
    try std.testing.expectEqual(@as(usize, 1), update_count);
    try std.testing.expect(update_batch != null);

    // Verify the UPDATE batch matches expected values for 10.0.0.0/8
    try std.testing.expectEqual(@as(u16, 0), update_batch.?.withdrawn_routes_length);
    try std.testing.expectEqual(@as(u16, 14), update_batch.?.path_attributes_length);
    try std.testing.expectEqual(@as(usize, 1), update_batch.?.nlri_prefix_count);
}

test "session UPDATE capture: multiple prefixes in single batch" {
    // Test with 3 prefixes to verify NLRI encoding
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

    const prefixes = &.{
        types.Ipv4Prefix.init("10.0.0.0/8"),
        types.Ipv4Prefix.init("192.168.0.0/16"),
        types.Ipv4Prefix.init("172.16.0.0/12"),
    };

    const config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = .{ 127, 0, 0, 1 },
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = prefixes,
        .same_as = true,
    };

    var sess = try session.initWithClock(config, &trans, session.MockClock.interface());

    // Complete handshake
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);

    // Capture UPDATE
    _ = try session.runOnce(&sess);

    // Parse captured UPDATE frames
    const all_sent = fake.getAllSent();
    var offset: usize = 0;
    var total_prefixes: usize = 0;

    while (offset < all_sent.len) {
        if (offset + 19 > all_sent.len) break;
        const declared_len = @as(u16, all_sent[offset + 16]) * 256 + @as(u16, all_sent[offset + 17]);
        if (declared_len < 19 or declared_len > 4096 or offset + declared_len > all_sent.len) break;

        const frame_buf = all_sent[offset..][0..declared_len];
        const frame = frame_decode.decodeFrame(frame_buf) catch break;

        if (frame_decode.isUpdate(frame)) {
            const update_body = frame_decode.parseUpdateBody(frame);
            if (update_body) |ub| {
                total_prefixes += ub.nlri_prefix_count;
                // Every UPDATE must have withdrawn_routes_length == 0
                try std.testing.expectEqual(@as(u16, 0), ub.withdrawn_routes_length);
                // Every UPDATE must have non-zero path attributes
                try std.testing.expect(ub.path_attributes_length > 0);
                // Every UPDATE must have non-empty NLRI
                try std.testing.expect(ub.nlri_prefix_count > 0);
            }
        }

        offset += declared_len;
    }

    // All 3 prefixes should be in the single UPDATE batch
    try std.testing.expectEqual(@as(usize, 3), total_prefixes);
    try std.testing.expectEqual(@as(u64, 3), sess.status.nlri_sent_count);
}

test "session UPDATE capture: multi-batch prefix set" {
    // Test with more than MAX_PREFIXES_PER_UPDATE prefixes to force multiple batches.
    // MAX_PREFIXES_PER_UPDATE is ~811, so we use 900 to ensure at least 2 batches.
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

    // Use more than MAX_PREFIXES_PER_UPDATE (~811) to force multiple batches
    const prefix_count = session.MAX_PREFIXES_PER_UPDATE + 100;
    var prefixes: [prefix_count]types.Ipv4Prefix = undefined;
    for (0..prefix_count) |i| {
        prefixes[i] = types.Ipv4Prefix{
            .addr = .{ 10, @as(u8, @intCast(i / 256)), @as(u8, @intCast(i % 256)), 0 },
            .len = 24,
        };
    }

    var fake = try session.FakeTransport.init(std.testing.allocator, &.{
        session.PeerResponse{ .recv_bytes = &peer_open },
        session.PeerResponse{ .recv_bytes = &peer_keepalive },
    });
    defer fake.deinit();
    const trans = fake.toTransport();

    const config = session.SessionConfig{
        .peer_address = .{ 127, 0, 0, 1 },
        .peer_port = 179,
        .local_address = .{ 127, 0, 0, 1 },
        .local_as = 65001,
        .peer_as = 65002,
        .router_id = .{ 10, 0, 0, 1 },
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &prefixes,
        .same_as = true,
    };

    var sess = try session.initWithClock(config, &trans, session.MockClock.interface());

    // Complete handshake
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);

    // Send all UPDATE batches
    while (!sess.export_complete) {
        _ = try session.runOnce(&sess);
    }

    // Parse all captured UPDATE frames
    const all_sent = fake.getAllSent();
    var offset: usize = 0;
    var total_update_frames: usize = 0;
    var total_prefixes: usize = 0;
    var all_withdrawn_zero: bool = true;
    var all_attrs_nonzero: bool = true;
    var all_nlri_nonzero: bool = true;

    while (offset < all_sent.len) {
        if (offset + 19 > all_sent.len) break;
        const declared_len = @as(u16, all_sent[offset + 16]) * 256 + @as(u16, all_sent[offset + 17]);
        if (declared_len < 19 or declared_len > 4096 or offset + declared_len > all_sent.len) break;

        const frame_buf = all_sent[offset..][0..declared_len];
        const frame = frame_decode.decodeFrame(frame_buf) catch break;

        if (frame_decode.isUpdate(frame)) {
            total_update_frames += 1;
            const update_body = frame_decode.parseUpdateBody(frame);
            if (update_body) |ub| {
                total_prefixes += ub.nlri_prefix_count;
                if (ub.withdrawn_routes_length != 0) all_withdrawn_zero = false;
                if (ub.path_attributes_length == 0) all_attrs_nonzero = false;
                if (ub.nlri_prefix_count == 0) all_nlri_nonzero = false;
            }
        }

        offset += declared_len;
    }

    // Assertions for all exported UPDATE batches:
    // - Every batch must be message type UPDATE (checked via isUpdate)
    // - Every batch must have withdrawn_routes_length == 0
    try std.testing.expect(all_withdrawn_zero);
    // - Every batch must have total_path_attribute_length > 0
    try std.testing.expect(all_attrs_nonzero);
    // - Every batch must have NLRI byte count > 0
    try std.testing.expect(all_nlri_nonzero);

    // Verify totals
    try std.testing.expect(total_update_frames >= 2); // Must have multiple batches
    try std.testing.expectEqual(@as(usize, prefix_count), total_prefixes);
    try std.testing.expectEqual(@as(usize, prefix_count), sess.status.nlri_sent_count);
    try std.testing.expectEqual(@as(usize, prefix_count), sess.status.configured_prefix_count);
}

test "session UPDATE capture: different AS includes AS_PATH segment" {
    // Verify different_as=true includes AS in AS_PATH (non-empty)
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

    // different_as=true means AS_PATH has segment
    const config = session.SessionConfig{
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
        .same_as = false, // Different AS - AS_PATH will have segment
    };

    var sess = try session.initWithClock(config, &trans, session.MockClock.interface());

    // Complete handshake
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);
    _ = try session.runOnce(&sess);

    // Capture UPDATE
    _ = try session.runOnce(&sess);

    // Parse captured frames
    const all_sent = fake.getAllSent();
    var offset: usize = 0;

    while (offset < all_sent.len) {
        if (offset + 19 > all_sent.len) break;
        const declared_len = @as(u16, all_sent[offset + 16]) * 256 + @as(u16, all_sent[offset + 17]);
        if (declared_len < 19 or declared_len > 4096 or offset + declared_len > all_sent.len) break;

        const frame_buf = all_sent[offset..][0..declared_len];
        const frame = frame_decode.decodeFrame(frame_buf) catch break;

        if (frame_decode.isUpdate(frame)) {
            const update_body = frame_decode.parseUpdateBody(frame);
            try std.testing.expect(update_body != null);
            // For different_as, path_attributes_length should be 18 (ORIGIN(4) + AS_PATH(7) + NEXT_HOP(7))
            // vs same_as where it's 14 (ORIGIN(4) + AS_PATH(3) + NEXT_HOP(7))
            try std.testing.expectEqual(@as(u16, 18), update_body.?.path_attributes_length);
            try std.testing.expectEqual(@as(u16, 0), update_body.?.withdrawn_routes_length);
            try std.testing.expectEqual(@as(usize, 1), update_body.?.nlri_prefix_count);
        }

        offset += declared_len;
    }
}
