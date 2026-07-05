// status_bgp_fsm_tests.zig — FSM state vs zero-prefix warning tests
//
// Verifies that established FSM wins over zero-prefix warning.

const std = @import("std");
const status = @import("status.zig");

test "FSM established + zero prefixes => ok" {
    var scratch: [64]u8 = undefined;
    const check = status.getBgpCheck(.{
        .configured = .{
            .configured_prefix_count = 0,
            .updates_sent = 0,
            .nlri_sent_count = 0,
            .fsm_state = .established,
            .peer_address = .{ 10, 0, 0, 2 },
            .peer_as = 65002,
            .local_as = 65001,
            .last_error = null,
            .messages_sent = 5,
            .messages_received = 4,
            .keepalives_sent = 2,
            .keepalives_received = 2,
            .passive_listener_state = .disabled,
            .passive_listener_error = null,
        },
    }, &scratch);
    try std.testing.expect(check.status == .ok);
    try std.testing.expectEqualStrings("BGP established", check.detail);
}

test "FSM established + prefixes => ok with count" {
    var scratch: [64]u8 = undefined;
    const check = status.getBgpCheck(.{
        .configured = .{
            .configured_prefix_count = 3,
            .updates_sent = 1,
            .nlri_sent_count = 3,
            .fsm_state = .established,
            .peer_address = .{ 10, 0, 0, 2 },
            .peer_as = 65002,
            .local_as = 65001,
            .last_error = null,
            .messages_sent = 10,
            .messages_received = 8,
            .keepalives_sent = 5,
            .keepalives_received = 5,
            .passive_listener_state = .disabled,
            .passive_listener_error = null,
        },
    }, &scratch);
    try std.testing.expect(check.status == .ok);
    try std.testing.expect(std.mem.containsAtLeast(u8, check.detail, 1, "3 configured prefixes"));
}

test "FSM not established + zero prefixes => warn" {
    var scratch: [64]u8 = undefined;
    const check = status.getBgpCheck(.{
        .configured = .{
            .configured_prefix_count = 0,
            .updates_sent = 0,
            .nlri_sent_count = 0,
            .fsm_state = .open_sent,
            .peer_address = .{ 10, 0, 0, 2 },
            .peer_as = 65002,
            .local_as = 65001,
            .last_error = null,
            .messages_sent = 1,
            .messages_received = 0,
            .keepalives_sent = 0,
            .keepalives_received = 0,
            .passive_listener_state = .disabled,
            .passive_listener_error = null,
        },
    }, &scratch);
    try std.testing.expect(check.status == .warn);
    try std.testing.expectEqualStrings("BGP configured with no configured prefixes", check.detail);
}
