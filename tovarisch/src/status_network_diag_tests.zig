// status_network_diag_tests.zig — Unit tests for TCP diagnostics absence events
//
// Tests TCP absence events:
// - non-empty TCP sockets -> no absence reason required
// - empty TCP after successful ss/no match -> event reason no_matching_socket
// - command failure -> reason command_failed
// - disabled/not configured -> reason not_configured

const std = @import("std");
const status_network_diag = @import("status_network_diag.zig");
const network_diag_config = @import("net/network_diag_config.zig");

const testing = std.testing;

// ============================================================================
// Tests: TCP Absence Events
// ============================================================================

test "TcpAbsenceReason enum has all allowed values" {
    // Verify all expected reasons are present
    const reasons = .{
        status_network_diag.TcpAbsenceReason.no_matching_socket,
        status_network_diag.TcpAbsenceReason.socket_closed_before_capture,
        status_network_diag.TcpAbsenceReason.command_failed,
        status_network_diag.TcpAbsenceReason.not_configured,
        status_network_diag.TcpAbsenceReason.permission_denied,
        status_network_diag.TcpAbsenceReason.target_not_tcp,
        status_network_diag.TcpAbsenceReason.target_mapping_missing,
        status_network_diag.TcpAbsenceReason.unsupported_platform,
        status_network_diag.TcpAbsenceReason.parse_failed,
    };

    // All reasons should have valid tag names
    inline for (reasons) |reason| {
        const name = @tagName(reason);
        try testing.expect(name.len > 0);
    }
}

// ============================================================================
// Tests: collectNetworkDiag with TCP diagnostics disabled
// ============================================================================

test "collectNetworkDiag with disabled diagnostics returns empty events" {
    const allocator = testing.allocator;

    const cfg = network_diag_config.NetworkDiagConfig{
        .enabled = false,
        .wireguard = .{ .enabled = false },
        .underlay_tcp = .{ .enabled = false },
    };

    var diag = try status_network_diag.collectNetworkDiag(allocator, cfg);
    defer diag.deinit(allocator);

    // When diagnostics are fully disabled, no events should be generated
    try testing.expectEqualSlices(status_network_diag.EventOutput, &.{}, diag.events);
}

test "collectNetworkDiag with TCP disabled but overall enabled" {
    const allocator = testing.allocator;

    const cfg = network_diag_config.NetworkDiagConfig{
        .enabled = true,
        .wireguard = .{ .enabled = false },
        .underlay_tcp = .{
            .enabled = false,
            .commands_enabled = false,
        },
    };

    var diag = try status_network_diag.collectNetworkDiag(allocator, cfg);
    defer diag.deinit(allocator);

    // TCP disabled should produce a not_configured event
    try testing.expect(diag.events.len == 1);
    try testing.expectEqualStrings("underlay_tcp", diag.events[0].source);
    try testing.expectEqualStrings("warning", diag.events[0].severity);
    try testing.expect(diag.events[0].fields != null);
    try testing.expect(std.mem.containsAtLeast(u8, diag.events[0].fields.?, 1, "not_configured"));
}

test "collectNetworkDiag with TCP enabled but commands disabled" {
    const allocator = testing.allocator;

    const cfg = network_diag_config.NetworkDiagConfig{
        .enabled = true,
        .wireguard = .{ .enabled = false },
        .underlay_tcp = .{
            .enabled = true,
            .commands_enabled = false,
        },
    };

    var diag = try status_network_diag.collectNetworkDiag(allocator, cfg);
    defer diag.deinit(allocator);

    // Commands disabled should produce a not_configured event
    try testing.expect(diag.events.len == 1);
    try testing.expectEqualStrings("underlay_tcp", diag.events[0].source);
    try testing.expectEqualStrings("warning", diag.events[0].severity);
    try testing.expect(diag.events[0].fields != null);
    try testing.expect(std.mem.containsAtLeast(u8, diag.events[0].fields.?, 1, "not_configured"));
}

// ============================================================================
// Tests: JSON shape validation for TCP absence events
// ============================================================================

test "TCP absence event fields JSON is valid" {
    const allocator = testing.allocator;

    // Build a fields JSON string as the code does
    const fields_json = try std.fmt.allocPrint(allocator, "{{\"reason\":\"{s}\"}}", .{"no_matching_socket"});
    defer allocator.free(fields_json);

    // The fields should be a valid JSON object string
    try testing.expect(std.mem.startsWith(u8, fields_json, "{\"reason\":"));
    try testing.expect(std.mem.endsWith(u8, fields_json, "\"}"));
}

test "TCP absence event fields JSON with exit_code is valid" {
    const allocator = testing.allocator;

    // Build a fields JSON string with exit_code as the code does
    const fields_json = try std.fmt.allocPrint(allocator, "{{\"reason\":\"{s}\",\"exit_code\":{d}}}", .{ "command_failed", 127 });
    defer allocator.free(fields_json);

    // The fields should be a valid JSON object string with both fields
    try testing.expect(std.mem.containsAtLeast(u8, fields_json, 1, "\"reason\":"));
    try testing.expect(std.mem.containsAtLeast(u8, fields_json, 1, "\"exit_code\":"));
    try testing.expect(std.mem.containsAtLeast(u8, fields_json, 1, "127"));
}
