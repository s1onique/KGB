// status_wg_evidence_tests.zig — WireGuard evidence-enhanced diagnostic tests
//
// ACT-HULK29R-ZIG016-WG-PEERS-NAMESPACE-EVIDENCE
// Tests for the evidence-enhanced diagnostic detail formatter.
//
// Split from status_checks.zig to satisfy LLM-friendliness limits (450-line hard limit).

const std = @import("std");
const wg_cli_facts = @import("net/wg_cli_facts.zig");
const classifier = @import("net/wg_diagnostic_classifier.zig");
const status_checks = @import("status_checks.zig");
const testing = std.testing;

// ============================================================================
// Evidence-Enhanced Diagnostic Tests
// ACT-HULK29R-ZIG016-WG-PEERS-NAMESPACE-EVIDENCE
// ============================================================================

test "classifierErrorKindToDetailWithEvidence: namespace mismatch includes evidence" {
    // Test that namespace mismatch includes os_link_seen and wg_cli_seen evidence
    const evidence = wg_cli_facts.CliEvidence{
        .interface_name = "wg-kgb0",
        .os_link_seen = true,
        .os_link_kind = .unknown,
        .wg_interfaces_seen = true,
        .wg_interface_list_contains_name = false,
        .wg_show_stderr_class = .no_such_device,
    };

    var buf: [status_checks.DIAGNOSTIC_DETAIL_BUF_SIZE]u8 = undefined;
    const result = status_checks.classifierErrorKindToDetailWithEvidence(
        .wrong_namespace_or_unreachable,
        evidence,
        &buf,
    );

    // Verify the output contains evidence indicators
    try testing.expect(std.mem.startsWith(u8, result, "wg wrong_namespace_or_unreachable:"));
    try testing.expect(std.mem.containsAtLeast(u8, result, 1, "os_link_seen=true"));
    try testing.expect(std.mem.containsAtLeast(u8, result, 1, "wg_cli_seen=false"));
}

test "classifierErrorKindToDetailWithEvidence: namespace mismatch with both visible" {
    // Test namespace mismatch when both OS link and wg CLI can see the interface
    const evidence = wg_cli_facts.CliEvidence{
        .interface_name = "wg-kgb0",
        .os_link_seen = true,
        .os_link_kind = .wireguard,
        .wg_interfaces_seen = true,
        .wg_interface_list_contains_name = true,
        .wg_show_stderr_class = .none,
    };

    var buf: [status_checks.DIAGNOSTIC_DETAIL_BUF_SIZE]u8 = undefined;
    const result = status_checks.classifierErrorKindToDetailWithEvidence(
        .wrong_namespace_or_unreachable,
        evidence,
        &buf,
    );

    // Even when both visible, the format includes the evidence fields
    try testing.expect(std.mem.startsWith(u8, result, "wg wrong_namespace_or_unreachable:"));
    try testing.expect(std.mem.containsAtLeast(u8, result, 1, "os_link_seen=true"));
    try testing.expect(std.mem.containsAtLeast(u8, result, 1, "wg_cli_seen=true"));
}

test "classifierErrorKindToDetailWithEvidence: non-namespace classes fall back" {
    // Test that non-namespace classes use the standard formatter
    const evidence = wg_cli_facts.CliEvidence{
        .interface_name = "wg-kgb0",
        .os_link_seen = false,
        .os_link_kind = .missing,
        .wg_interfaces_seen = false,
        .wg_interface_list_contains_name = false,
        .wg_show_stderr_class = .none,
    };

    var buf: [status_checks.DIAGNOSTIC_DETAIL_BUF_SIZE]u8 = undefined;

    // Test wireguard_interface_missing
    const missing_result = status_checks.classifierErrorKindToDetailWithEvidence(
        .wireguard_interface_missing,
        evidence,
        &buf,
    );
    try testing.expectEqualStrings(
        "wg wireguard_interface_missing: interface not found",
        missing_result,
    );

    // Test permission_denied
    const perm_result = status_checks.classifierErrorKindToDetailWithEvidence(
        .permission_denied,
        evidence,
        &buf,
    );
    try testing.expectEqualStrings(
        "wg permission_denied: capability denied",
        perm_result,
    );

    // Test peers_healthy
    const healthy_result = status_checks.classifierErrorKindToDetailWithEvidence(
        .peers_healthy,
        evidence,
        &buf,
    );
    try testing.expectEqualStrings(
        "wg peers_healthy: all peers connected",
        healthy_result,
    );
}

test "classifierErrorKindToDetailWithEvidence: buffer size constant is correct" {
    // Note: In Zig 0.16, the function signature `*[DIAGNOSTIC_DETAIL_BUF_SIZE]u8`
    // prevents buffer overflow at compile time. The type system ensures the caller
    // provides a buffer of at least DIAGNOSTIC_DETAIL_BUF_SIZE (256 bytes).
    // This test verifies the buffer size constant is correctly defined.
    try testing.expectEqual(@as(usize, 256), status_checks.DIAGNOSTIC_DETAIL_BUF_SIZE);
}
