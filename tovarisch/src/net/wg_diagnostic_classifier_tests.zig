// wg_diagnostic_classifier_tests.zig — WireGuard diagnostic classifier tests
//
// ACT-HULK29R-ZIG016-WG-STATUS-CLASSIFICATION-FIX
//
// Tests for the precise WireGuard diagnostic classification system.
// Covers the decision table for distinguishing:
// - OS link presence from WireGuard interface visibility
// - Permission/capability failures from namespace/unreachable cases
// - Malformed output from healthy states

const std = @import("std");
const classifier = @import("wg_diagnostic_classifier.zig");
const testing = std.testing;

// ============================================================================
// Decision Table Tests
// ============================================================================

test "classifier: OS link missing + wg absent => wireguard_interface_missing" {
    const facts = classifier.WgInterfaceFacts{
        .configured_name = "wg-kgb0",
        .os_link_seen = false,
        .os_link_kind = .missing,
        .wg_tool_seen = true,
        .wg_interfaces_seen = true,
        .wg_interface_list_contains_name = false,
        .wg_show_exit_code = 1,
        .wg_show_stderr_class = .no_such_device,
        .wg_dump_succeeded = false,
        .wg_dump_peer_count = 0,
        .wg_dump_has_handshake = false,
        .wg_dump_malformed = false,
    };
    try testing.expectEqual(classifier.WgDiagnosticClass.wireguard_interface_missing, classifier.classifyWgStatus(facts));
}

test "classifier: OS link present non-WG + wg absent => interface_present_non_wireguard" {
    const facts = classifier.WgInterfaceFacts{
        .configured_name = "wg-kgb0",
        .os_link_seen = true,
        .os_link_kind = .non_wireguard,
        .wg_tool_seen = true,
        .wg_interfaces_seen = true,
        .wg_interface_list_contains_name = false,
        .wg_show_exit_code = 1,
        .wg_show_stderr_class = .no_such_device,
        .wg_dump_succeeded = false,
        .wg_dump_peer_count = 0,
        .wg_dump_has_handshake = false,
        .wg_dump_malformed = false,
    };
    try testing.expectEqual(classifier.WgDiagnosticClass.interface_present_non_wireguard, classifier.classifyWgStatus(facts));
}

test "classifier: wg command missing => wg_tool_missing" {
    const facts = classifier.WgInterfaceFacts{
        .configured_name = "wg-kgb0",
        .os_link_seen = true,
        .os_link_kind = .wireguard,
        .wg_tool_seen = false,
        .wg_interfaces_seen = false,
        .wg_interface_list_contains_name = false,
        .wg_show_exit_code = null,
        .wg_show_stderr_class = .none,
        .wg_dump_succeeded = false,
        .wg_dump_peer_count = 0,
        .wg_dump_has_handshake = false,
        .wg_dump_malformed = false,
    };
    try testing.expectEqual(classifier.WgDiagnosticClass.wg_tool_missing, classifier.classifyWgStatus(facts));
}

test "classifier: stderr Operation not permitted => permission_denied" {
    const facts = classifier.WgInterfaceFacts{
        .configured_name = "wg-kgb0",
        .os_link_seen = true,
        .os_link_kind = .wireguard,
        .wg_tool_seen = true,
        .wg_interfaces_seen = true,
        .wg_interface_list_contains_name = true,
        .wg_show_exit_code = 1,
        .wg_show_stderr_class = .operation_not_permitted,
        .wg_dump_succeeded = false,
        .wg_dump_peer_count = 0,
        .wg_dump_has_handshake = false,
        .wg_dump_malformed = false,
    };
    try testing.expectEqual(classifier.WgDiagnosticClass.permission_denied, classifier.classifyWgStatus(facts));
}

test "classifier: stderr Permission denied => permission_denied" {
    const facts = classifier.WgInterfaceFacts{
        .configured_name = "wg-kgb0",
        .os_link_seen = true,
        .os_link_kind = .wireguard,
        .wg_tool_seen = true,
        .wg_interfaces_seen = true,
        .wg_interface_list_contains_name = true,
        .wg_show_exit_code = 1,
        .wg_show_stderr_class = .permission_denied,
        .wg_dump_succeeded = false,
        .wg_dump_peer_count = 0,
        .wg_dump_has_handshake = false,
        .wg_dump_malformed = false,
    };
    try testing.expectEqual(classifier.WgDiagnosticClass.permission_denied, classifier.classifyWgStatus(facts));
}

test "classifier: wg interfaces missing configured name + OS link exists => wrong_namespace_or_unreachable" {
    const facts = classifier.WgInterfaceFacts{
        .configured_name = "wg-kgb0",
        .os_link_seen = true,
        .os_link_kind = .wireguard,
        .wg_tool_seen = true,
        .wg_interfaces_seen = true,
        .wg_interface_list_contains_name = false,
        .wg_show_exit_code = 1,
        .wg_show_stderr_class = .no_such_device,
        .wg_dump_succeeded = false,
        .wg_dump_peer_count = 0,
        .wg_dump_has_handshake = false,
        .wg_dump_malformed = false,
    };
    try testing.expectEqual(classifier.WgDiagnosticClass.wrong_namespace_or_unreachable, classifier.classifyWgStatus(facts));
}

test "classifier: dump success with zero peers => no_peers" {
    const facts = classifier.WgInterfaceFacts{
        .configured_name = "wg-kgb0",
        .os_link_seen = true,
        .os_link_kind = .wireguard,
        .wg_tool_seen = true,
        .wg_interfaces_seen = true,
        .wg_interface_list_contains_name = true,
        .wg_show_exit_code = 0,
        .wg_show_stderr_class = .none,
        .wg_dump_succeeded = true,
        .wg_dump_peer_count = 0,
        .wg_dump_has_handshake = false,
        .wg_dump_malformed = false,
    };
    try testing.expectEqual(classifier.WgDiagnosticClass.no_peers, classifier.classifyWgStatus(facts));
}

test "classifier: dump success with peers and zero handshake => no_handshake" {
    const facts = classifier.WgInterfaceFacts{
        .configured_name = "wg-kgb0",
        .os_link_seen = true,
        .os_link_kind = .wireguard,
        .wg_tool_seen = true,
        .wg_interfaces_seen = true,
        .wg_interface_list_contains_name = true,
        .wg_show_exit_code = 0,
        .wg_show_stderr_class = .none,
        .wg_dump_succeeded = true,
        .wg_dump_peer_count = 2,
        .wg_dump_has_handshake = false,
        .wg_dump_malformed = false,
    };
    try testing.expectEqual(classifier.WgDiagnosticClass.no_handshake, classifier.classifyWgStatus(facts));
}

test "classifier: dump success with peers and handshake => peers_healthy" {
    const facts = classifier.WgInterfaceFacts{
        .configured_name = "wg-kgb0",
        .os_link_seen = true,
        .os_link_kind = .wireguard,
        .wg_tool_seen = true,
        .wg_interfaces_seen = true,
        .wg_interface_list_contains_name = true,
        .wg_show_exit_code = 0,
        .wg_show_stderr_class = .none,
        .wg_dump_succeeded = true,
        .wg_dump_peer_count = 2,
        .wg_dump_has_handshake = true,
        .wg_dump_malformed = false,
    };
    try testing.expectEqual(classifier.WgDiagnosticClass.peers_healthy, classifier.classifyWgStatus(facts));
}

test "classifier: malformed dump => malformed_output" {
    const facts = classifier.WgInterfaceFacts{
        .configured_name = "wg-kgb0",
        .os_link_seen = true,
        .os_link_kind = .wireguard,
        .wg_tool_seen = true,
        .wg_interfaces_seen = true,
        .wg_interface_list_contains_name = true,
        .wg_show_exit_code = 0,
        .wg_show_stderr_class = .none,
        .wg_dump_succeeded = false,
        .wg_dump_peer_count = 0,
        .wg_dump_has_handshake = false,
        .wg_dump_malformed = true,
    };
    try testing.expectEqual(classifier.WgDiagnosticClass.malformed_output, classifier.classifyWgStatus(facts));
}

test "classifier: command_failed with non-zero exit" {
    const facts = classifier.WgInterfaceFacts{
        .configured_name = "wg-kgb0",
        .os_link_seen = true,
        .os_link_kind = .wireguard,
        .wg_tool_seen = true,
        .wg_interfaces_seen = true,
        .wg_interface_list_contains_name = true,
        .wg_show_exit_code = 2,
        .wg_show_stderr_class = .other,
        .wg_dump_succeeded = false,
        .wg_dump_peer_count = 0,
        .wg_dump_has_handshake = false,
        .wg_dump_malformed = false,
    };
    try testing.expectEqual(classifier.WgDiagnosticClass.command_failed, classifier.classifyWgStatus(facts));
}

// ============================================================================
// Formatter Contract Tests
// ============================================================================

test "formatter: interface_present_non_wireguard with link_kind" {
    const diag = struct {
        const std = @import("std");
        const wg_peer_diag = @import("wg_peer_diagnostic.zig");
    }.wg_peer_diag.WireGuardPeerDiagnostic{
        .backend = "cli",
        .selected_interface = "wg-kgb0",
        .command = "wg show wg-kgb0 dump",
        .timeout_secs = null,
        .exit_code = 1,
        .error_kind = "interface_present_non_wireguard",
        .stderr_len = 0,
        .stdout_len = 0,
        .os_link_kind = .non_wireguard,
        .peer_count = 0,
    };
    var buf: [256]u8 = undefined;
    const wg_peer_diag = @import("wg_peer_diagnostic.zig");
    const result = wg_peer_diag.formatPeerDiagnosticDetail(diag, &buf);
    try testing.expect(std.mem.startsWith(u8, result, "wg interface_present_non_wireguard:"));
    try testing.expect(std.mem.containsAtLeast(u8, result, 1, "interface=wg-kgb0"));
    try testing.expect(std.mem.containsAtLeast(u8, result, 1, "backend=cli"));
    try testing.expect(std.mem.containsAtLeast(u8, result, 1, "link_kind="));
}

test "formatter: peers_healthy with peer count" {
    const wg_peer_diag = @import("wg_peer_diagnostic.zig");
    const diag = wg_peer_diag.WireGuardPeerDiagnostic{
        .backend = "cli",
        .selected_interface = "wg-kgb0",
        .command = "wg show wg-kgb0 dump",
        .timeout_secs = null,
        .exit_code = 0,
        .error_kind = "peers_healthy",
        .stderr_len = 0,
        .stdout_len = 0,
        .os_link_kind = .wireguard,
        .peer_count = 2,
    };
    var buf: [256]u8 = undefined;
    const result = wg_peer_diag.formatPeerDiagnosticDetail(diag, &buf);
    try testing.expect(std.mem.startsWith(u8, result, "wg peers_healthy:"));
    try testing.expect(std.mem.containsAtLeast(u8, result, 1, "interface=wg-kgb0"));
    try testing.expect(std.mem.containsAtLeast(u8, result, 1, "peers=2"));
}

test "formatter: wrong_namespace_or_unreachable" {
    const wg_peer_diag = @import("wg_peer_diagnostic.zig");
    const diag = wg_peer_diag.WireGuardPeerDiagnostic{
        .backend = "cli",
        .selected_interface = "wg-kgb0",
        .command = "wg show wg-kgb0 dump",
        .timeout_secs = null,
        .exit_code = 1,
        .error_kind = "wrong_namespace_or_unreachable",
        .stderr_len = 15,
        .stdout_len = 0,
        .os_link_kind = .wireguard,
        .peer_count = 0,
    };
    var buf: [256]u8 = undefined;
    const result = wg_peer_diag.formatPeerDiagnosticDetail(diag, &buf);
    try testing.expect(std.mem.startsWith(u8, result, "wg wrong_namespace_or_unreachable:"));
    try testing.expect(std.mem.containsAtLeast(u8, result, 1, "interface=wg-kgb0"));
    try testing.expect(std.mem.containsAtLeast(u8, result, 1, "backend=cli"));
}

test "formatter: no_handshake with peer count" {
    const wg_peer_diag = @import("wg_peer_diagnostic.zig");
    const diag = wg_peer_diag.WireGuardPeerDiagnostic{
        .backend = "cli",
        .selected_interface = "wg-kgb0",
        .command = "wg show wg-kgb0 dump",
        .timeout_secs = null,
        .exit_code = 0,
        .error_kind = "no_handshake",
        .stderr_len = 0,
        .stdout_len = 0,
        .os_link_kind = .wireguard,
        .peer_count = 1,
    };
    var buf: [256]u8 = undefined;
    const result = wg_peer_diag.formatPeerDiagnosticDetail(diag, &buf);
    try testing.expect(std.mem.startsWith(u8, result, "wg no_handshake:"));
    try testing.expect(std.mem.containsAtLeast(u8, result, 1, "interface=wg-kgb0"));
    try testing.expect(std.mem.containsAtLeast(u8, result, 1, "peers=1"));
}

// ============================================================================
// Status Mapping Tests
// ============================================================================

test "toCheckStatus: peers_healthy maps to ok" {
    try testing.expectEqual(classifier.status.CheckStatus.ok, classifier.toCheckStatus(.peers_healthy));
}

test "toCheckStatus: all warn classes map to warn" {
    try testing.expectEqual(classifier.status.CheckStatus.warn, classifier.toCheckStatus(.wg_tool_missing));
    try testing.expectEqual(classifier.status.CheckStatus.warn, classifier.toCheckStatus(.wireguard_interface_missing));
    try testing.expectEqual(classifier.status.CheckStatus.warn, classifier.toCheckStatus(.interface_present_non_wireguard));
    try testing.expectEqual(classifier.status.CheckStatus.warn, classifier.toCheckStatus(.permission_denied));
    try testing.expectEqual(classifier.status.CheckStatus.warn, classifier.toCheckStatus(.wrong_namespace_or_unreachable));
    try testing.expectEqual(classifier.status.CheckStatus.warn, classifier.toCheckStatus(.command_failed));
    try testing.expectEqual(classifier.status.CheckStatus.warn, classifier.toCheckStatus(.malformed_output));
    try testing.expectEqual(classifier.status.CheckStatus.warn, classifier.toCheckStatus(.no_peers));
    try testing.expectEqual(classifier.status.CheckStatus.warn, classifier.toCheckStatus(.no_handshake));
}
