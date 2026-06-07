// packet_tests.zig — BFD packet encode/decode tests
//
// Tests for RFC 5880 BFD control packet format (24 bytes).

const std = @import("std");
const packet = @import("packet.zig");

test "encode roundtrip" {
    const original = packet.ControlPacket{
        .diag = .neighbor_signaled_session_down,
        .state = .up,
        .detect_mult = 3,
        .my_discr = 0x12345678,
        .your_discr = 0x87654321,
        .desired_min_tx_interval = 800000,
        .required_min_rx_interval = 800000,
        .required_min_echo_rx_interval = 0,
    };

    var buf: [32]u8 = undefined;
    const written = packet.encode(original, &buf);
    try std.testing.expectEqual(@as(usize, 24), written);

    const decoded = try packet.decode(&buf);
    try std.testing.expectEqual(@intFromEnum(packet.State.up), @intFromEnum(decoded.state));
    try std.testing.expectEqual(@as(u8, 3), decoded.detect_mult);
    try std.testing.expectEqual(@as(u32, 0x12345678), decoded.my_discr);
    try std.testing.expectEqual(@as(u32, 0x87654321), decoded.your_discr);
    try std.testing.expectEqual(@as(u32, 800000), decoded.desired_min_tx_interval);
    try std.testing.expectEqual(@as(u32, 800000), decoded.required_min_rx_interval);
    try std.testing.expectEqual(@as(u32, 0), decoded.required_min_echo_rx_interval);
}

test "decode rejects short buffer" {
    const short_buf: [4]u8 = .{ 0x20, 0xC0, 0x03, 24 };
    _ = packet.decode(&short_buf) catch |err| {
        try std.testing.expect(err == error.InvalidPacket);
        return;
    };
    try std.testing.expect(false);
}

test "decode rejects wrong version" {
    var buf: [32]u8 = .{ 0x60, 0xC0, 0x03, 24, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0 };
    _ = packet.decode(&buf) catch |err| {
        try std.testing.expect(err == error.InvalidPacket);
        return;
    };
    try std.testing.expect(false);
}

test "usToMs rounds up" {
    try std.testing.expectEqual(@as(u32, 1), packet.usToMs(1));
    try std.testing.expectEqual(@as(u32, 1), packet.usToMs(500));
    try std.testing.expectEqual(@as(u32, 1), packet.usToMs(999));
    try std.testing.expectEqual(@as(u32, 1), packet.usToMs(1000));
    try std.testing.expectEqual(@as(u32, 2), packet.usToMs(1001));
    try std.testing.expectEqual(@as(u32, 3), packet.usToMs(2400));
}

test "msToUs" {
    try std.testing.expectEqual(@as(u32, 0), packet.msToUs(0));
    try std.testing.expectEqual(@as(u32, 800000), packet.msToUs(800));
    try std.testing.expectEqual(@as(u32, 2400000), packet.msToUs(2400));
}

test "encode minimal packet" {
    const minimal = packet.ControlPacket{};
    var buf: [32]u8 = undefined;
    const written = packet.encode(minimal, &buf);
    try std.testing.expectEqual(@as(usize, 24), written);

    // Verify first byte: Version 1 (0x20)
    try std.testing.expectEqual(@as(u8, 0x20), buf[0]);
    try std.testing.expectEqual(@as(u8, 0x40), buf[1]); // State Down (1), flags 0
    try std.testing.expectEqual(@as(u8, 0x03), buf[2]);
    try std.testing.expectEqual(@as(u8, 24), buf[3]);
}

test "encode all states" {
    var buf: [32]u8 = undefined;

    inline for (.{ packet.State.admin_down, packet.State.down, packet.State.init, packet.State.up }) |state| {
        const pkt = packet.ControlPacket{ .state = state };
        const written = packet.encode(pkt, &buf);
        try std.testing.expectEqual(@as(usize, 24), written);

        const decoded = try packet.decode(&buf);
        try std.testing.expectEqual(@intFromEnum(state), @intFromEnum(decoded.state));
    }
}

test "encode all diagnostics" {
    var buf: [32]u8 = undefined;

    inline for (.{
        packet.Diagnostic.no_diagnostic,
        packet.Diagnostic.control_detection_time_expired,
        packet.Diagnostic.echo_function_failed,
        packet.Diagnostic.neighbor_signaled_session_down,
        packet.Diagnostic.forwarding_plane_reset,
        packet.Diagnostic.path_down,
        packet.Diagnostic.concatenated_path_down,
        packet.Diagnostic.admin_down,
        packet.Diagnostic.reverse_concatenated_path_down,
    }) |diag| {
        const pkt = packet.ControlPacket{ .diag = diag };
        const written = packet.encode(pkt, &buf);
        try std.testing.expectEqual(@as(usize, 24), written);

        const decoded = try packet.decode(&buf);
        try std.testing.expectEqual(@intFromEnum(diag), @intFromEnum(decoded.diag));
    }
}

test "encode flags" {
    var buf: [32]u8 = undefined;
    const all_flags = packet.Flags{
        .poll = 1,
        .final = 1,
        .control_plane_independent = 1,
        .auth_present = 1,
        .demand = 1,
        .multipoint = 1,
    };

    const pkt = packet.ControlPacket{ .state = .up, .flags = all_flags };
    const written = packet.encode(pkt, &buf);
    try std.testing.expectEqual(@as(usize, 24), written);

    // All flags set = 0x3F (binary 111111)
    // State Up (3) in upper bits = 0xC0
    // Result: 0xFF
    try std.testing.expectEqual(@as(u8, 0xFF), buf[1]);

    const decoded = try packet.decode(&buf);
    try std.testing.expectEqual(@as(u1, 1), decoded.flags.poll);
    try std.testing.expectEqual(@as(u1, 1), decoded.flags.final);
    try std.testing.expectEqual(@as(u1, 1), decoded.flags.control_plane_independent);
    try std.testing.expectEqual(@as(u1, 1), decoded.flags.auth_present);
    try std.testing.expectEqual(@as(u1, 1), decoded.flags.demand);
    try std.testing.expectEqual(@as(u1, 1), decoded.flags.multipoint);
}

test "encode individual flags" {
    var buf: [32]u8 = undefined;

    // Test Poll flag (should be 0x20 in lower 6 bits)
    const poll_pkt = packet.ControlPacket{ .flags = packet.Flags{ .poll = 1 } };
    _ = packet.encode(poll_pkt, &buf);
    try std.testing.expectEqual(@as(u8, 0x20), buf[1] & 0x3F);

    // Test Final flag (should be 0x10 in lower 6 bits)
    const final_pkt = packet.ControlPacket{ .flags = packet.Flags{ .final = 1 } };
    _ = packet.encode(final_pkt, &buf);
    try std.testing.expectEqual(@as(u8, 0x10), buf[1] & 0x3F);

    // Test Auth Present flag (should be 0x04 in lower 6 bits)
    const auth_pkt = packet.ControlPacket{ .flags = packet.Flags{ .auth_present = 1 } };
    _ = packet.encode(auth_pkt, &buf);
    try std.testing.expectEqual(@as(u8, 0x04), buf[1] & 0x3F);
}

test "encode discriminators max values" {
    var buf: [32]u8 = undefined;
    const pkt = packet.ControlPacket{
        .my_discr = 0xFFFFFFFF,
        .your_discr = 0xFFFFFFFF,
    };
    const written = packet.encode(pkt, &buf);
    try std.testing.expectEqual(@as(usize, 24), written);

    const decoded = try packet.decode(&buf);
    try std.testing.expectEqual(@as(u32, 0xFFFFFFFF), decoded.my_discr);
    try std.testing.expectEqual(@as(u32, 0xFFFFFFFF), decoded.your_discr);
}

test "encode intervals max values" {
    var buf: [32]u8 = undefined;
    const pkt = packet.ControlPacket{
        .desired_min_tx_interval = 0xFFFFFFFF,
        .required_min_rx_interval = 0xFFFFFFFF,
        .required_min_echo_rx_interval = 0xFFFFFFFF,
    };
    const written = packet.encode(pkt, &buf);
    try std.testing.expectEqual(@as(usize, 24), written);

    const decoded = try packet.decode(&buf);
    try std.testing.expectEqual(@as(u32, 0xFFFFFFFF), decoded.desired_min_tx_interval);
    try std.testing.expectEqual(@as(u32, 0xFFFFFFFF), decoded.required_min_rx_interval);
    try std.testing.expectEqual(@as(u32, 0xFFFFFFFF), decoded.required_min_echo_rx_interval);
}

test "decode from raw bytes" {
    // Construct a raw BFD packet matching RFC 5880 format (24 bytes)
    // 800000 µs = 0x000C3500 in big-endian
    var raw: [24]u8 = undefined;
    raw[0] = 0x20; // Version 1, Reserved 0
    raw[1] = 0xC0; // State Up (3), Flags 0
    raw[2] = 0x03; // Detect Mult 3
    raw[3] = 24; // Length 24
    raw[4] = 0x00; // My Discr = 1
    raw[5] = 0x00;
    raw[6] = 0x00;
    raw[7] = 0x01;
    raw[8] = 0x00; // Your Discr = 2
    raw[9] = 0x00;
    raw[10] = 0x00;
    raw[11] = 0x02;
    raw[12] = 0x00; // Desired Min TX = 800000
    raw[13] = 0x0C;
    raw[14] = 0x35;
    raw[15] = 0x00;
    raw[16] = 0x00; // Required Min RX = 800000
    raw[17] = 0x0C;
    raw[18] = 0x35;
    raw[19] = 0x00;
    raw[20] = 0x00; // Required Min Echo RX = 0
    raw[21] = 0x00;
    raw[22] = 0x00;
    raw[23] = 0x00;

    const pkt = try packet.decode(&raw);
    try std.testing.expectEqual(packet.State.up, pkt.state);
    try std.testing.expectEqual(@as(u8, 3), pkt.detect_mult);
    try std.testing.expectEqual(@as(u32, 1), pkt.my_discr);
    try std.testing.expectEqual(@as(u32, 2), pkt.your_discr);
    try std.testing.expectEqual(@as(u32, 800000), pkt.desired_min_tx_interval);
    try std.testing.expectEqual(@as(u32, 800000), pkt.required_min_rx_interval);
}

test "decode exact packet bytes" {
    // Test: Up, detect_mult 3, length 24, my_discr=100, your_discr=200, interval=800000
    var raw: [24]u8 = undefined;
    raw[0] = 0x20; // Version 1
    raw[1] = 0xC0; // State Up (3), Flags 0
    raw[2] = 0x03; // Detect Mult 3
    raw[3] = 24; // Length 24
    raw[4] = 0x00; // My Discr = 100 = 0x64
    raw[5] = 0x00;
    raw[6] = 0x00;
    raw[7] = 0x64;
    raw[8] = 0x00; // Your Discr = 200 = 0xC8
    raw[9] = 0x00;
    raw[10] = 0x00;
    raw[11] = 0xC8;
    raw[12] = 0x00; // Desired Min TX = 800000
    raw[13] = 0x0C;
    raw[14] = 0x35;
    raw[15] = 0x00;
    raw[16] = 0x00; // Required Min RX = 800000
    raw[17] = 0x0C;
    raw[18] = 0x35;
    raw[19] = 0x00;
    raw[20] = 0x00; // Required Min Echo RX = 0
    raw[21] = 0x00;
    raw[22] = 0x00;
    raw[23] = 0x00;

    const pkt = try packet.decode(&raw);
    try std.testing.expectEqual(packet.State.up, pkt.state);
    try std.testing.expectEqual(@as(u8, 3), pkt.detect_mult);
    try std.testing.expectEqual(@as(u32, 100), pkt.my_discr);
    try std.testing.expectEqual(@as(u32, 200), pkt.your_discr);
    try std.testing.expectEqual(@as(u32, 800000), pkt.desired_min_tx_interval);
    try std.testing.expectEqual(@as(u32, 800000), pkt.required_min_rx_interval);
}

test "buffer too small" {
    const pkt = packet.ControlPacket{};
    var small_buf: [10]u8 = undefined;
    const written = packet.encode(pkt, &small_buf);
    try std.testing.expectEqual(@as(usize, 0), written);
}

test "protocol version constant" {
    try std.testing.expectEqual(@as(u3, 1), packet.PROTOCOL_VERSION);
}

test "UDP port constants" {
    try std.testing.expectEqual(@as(u16, 4784), packet.MULTIHOP_UDP_PORT);
    try std.testing.expectEqual(@as(u16, 3784), packet.SINGLEHOP_UDP_PORT);
    try std.testing.expectEqual(@as(u16, 3785), packet.ECHO_UDP_PORT);
}

test "control packet length constant" {
    try std.testing.expectEqual(@as(usize, 24), packet.CONTROL_PACKET_LEN);
}

test "encode flags function" {
    // Poll only: 0x20
    try std.testing.expectEqual(@as(u8, 0x20), packet.encodeFlags(packet.Flags{ .poll = 1 }));
    // Final only: 0x10
    try std.testing.expectEqual(@as(u8, 0x10), packet.encodeFlags(packet.Flags{ .final = 1 }));
    // Auth only: 0x04
    try std.testing.expectEqual(@as(u8, 0x04), packet.encodeFlags(packet.Flags{ .auth_present = 1 }));
    // All flags: 0x3F
    try std.testing.expectEqual(@as(u8, 0x3F), packet.encodeFlags(packet.Flags{
        .poll = 1, .final = 1, .control_plane_independent = 1,
        .auth_present = 1, .demand = 1, .multipoint = 1,
    }));
}

test "decode flags function" {
    // Poll bit (bit 5)
    const poll_flags = packet.decodeFlags(0x20);
    try std.testing.expectEqual(@as(u1, 1), poll_flags.poll);
    try std.testing.expectEqual(@as(u1, 0), poll_flags.final);

    // Final bit (bit 4)
    const final_flags = packet.decodeFlags(0x10);
    try std.testing.expectEqual(@as(u1, 0), final_flags.poll);
    try std.testing.expectEqual(@as(u1, 1), final_flags.final);

    // Auth bit (bit 2)
    const auth_flags = packet.decodeFlags(0x04);
    try std.testing.expectEqual(@as(u1, 1), auth_flags.auth_present);
}
