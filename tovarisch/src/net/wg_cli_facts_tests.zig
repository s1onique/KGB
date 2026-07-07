// wg_cli_facts_tests.zig — WireGuard CLI evidence collection tests
//
// ACT-HULK29R-ZIG016-WG-STATUS-EVIDENCE-WIRING
//
// Tests for evidence collection and classification bridging.

const std = @import("std");
const wg_cli_facts = @import("wg_cli_facts.zig");
const classifier = @import("wg_diagnostic_classifier.zig");
const wg_boundary = @import("wg_status_boundary.zig");
const wg_boundary_cli = @import("wg_status_boundary_cli.zig");
const status = @import("../status.zig");
const testing = std.testing;

// ============================================================================
// CliEvidence Tests
// ============================================================================

test "CliEvidence has correct default values" {
    const evidence = wg_cli_facts.emptyEvidence;
    try testing.expectEqualStrings("wg-kgb0", evidence.interface_name);
    try testing.expectEqual(false, evidence.os_link_seen);
    try testing.expectEqual(classifier.OsLinkKind.unknown, evidence.os_link_kind);
    try testing.expectEqual(false, evidence.wg_interfaces_seen);
    try testing.expectEqual(false, evidence.wg_interface_list_contains_name);
    try testing.expectEqual(classifier.WgStderrClass.none, evidence.wg_show_stderr_class);
}

test "CliEvidence can be constructed with custom values" {
    const evidence = wg_cli_facts.CliEvidence{
        .interface_name = "wg-test0",
        .os_link_seen = true,
        .os_link_kind = .wireguard,
        .wg_interfaces_seen = true,
        .wg_interface_list_contains_name = true,
        .wg_show_stderr_class = .none,
    };
    try testing.expectEqualStrings("wg-test0", evidence.interface_name);
    try testing.expectEqual(true, evidence.os_link_seen);
    try testing.expectEqual(classifier.OsLinkKind.wireguard, evidence.os_link_kind);
    try testing.expectEqual(true, evidence.wg_interfaces_seen);
    try testing.expectEqual(true, evidence.wg_interface_list_contains_name);
}

// ============================================================================
// factsFromDiagnosticAttempt Tests
// ============================================================================

test "classifier integration: wg_tool_missing" {
    const attempt = wg_boundary_cli.WireGuardStatusDiagnosticAttempt{
        .err = .{
            .err = error.backend_missing,
            .diagnostic = wg_boundary.WireGuardPeerDiagnostic{
                .backend = "cli",
                .selected_interface = "wg-kgb0",
                .command = "wg show wg-kgb0 dump",
                .timeout_secs = null,
                .exit_code = 127,
                .error_kind = "backend_missing",
                .stderr_len = 0,
                .stdout_len = 0,
            },
        },
    };

    const evidence = wg_cli_facts.emptyEvidence;
    const facts = wg_cli_facts.factsFromDiagnosticAttempt(attempt, evidence);
    const diag_class = classifier.classifyWgStatus(facts);

    try testing.expectEqual(classifier.WgDiagnosticClass.wg_tool_missing, diag_class);
}

test "classifier integration: permission_denied" {
    const attempt = wg_boundary_cli.WireGuardStatusDiagnosticAttempt{
        .err = .{
            .err = error.permission_denied,
            .diagnostic = wg_boundary.WireGuardPeerDiagnostic{
                .backend = "cli",
                .selected_interface = "wg-kgb0",
                .command = "wg show wg-kgb0 dump",
                .timeout_secs = null,
                .exit_code = 126,
                .error_kind = "permission_denied",
                .stderr_len = 25,
                .stdout_len = 0,
            },
        },
    };

    const evidence = wg_cli_facts.CliEvidence{
        .interface_name = "wg-kgb0",
        .os_link_seen = true,
        .os_link_kind = .wireguard,
        .wg_interfaces_seen = true,
        .wg_interface_list_contains_name = true,
        .wg_show_stderr_class = .operation_not_permitted,
    };

    const facts = wg_cli_facts.factsFromDiagnosticAttempt(attempt, evidence);
    const diag_class = classifier.classifyWgStatus(facts);

    try testing.expectEqual(classifier.WgDiagnosticClass.permission_denied, diag_class);
}

test "classifier integration: wireguard_interface_missing" {
    const attempt = wg_boundary_cli.WireGuardStatusDiagnosticAttempt{
        .err = .{
            .err = error.interface_missing,
            .diagnostic = wg_boundary.WireGuardPeerDiagnostic{
                .backend = "cli",
                .selected_interface = "wg-kgb0",
                .command = "wg show wg-kgb0 dump",
                .timeout_secs = null,
                .exit_code = 1,
                .error_kind = "interface_missing",
                .stderr_len = 15,
                .stdout_len = 0,
            },
        },
    };

    const evidence = wg_cli_facts.CliEvidence{
        .interface_name = "wg-kgb0",
        .os_link_seen = false,
        .os_link_kind = .missing,
        .wg_interfaces_seen = true,
        .wg_interface_list_contains_name = false,
        .wg_show_stderr_class = .no_such_device,
    };

    const facts = wg_cli_facts.factsFromDiagnosticAttempt(attempt, evidence);
    const diag_class = classifier.classifyWgStatus(facts);

    try testing.expectEqual(classifier.WgDiagnosticClass.wireguard_interface_missing, diag_class);
}

test "classifier integration: interface_present_non_wireguard" {
    const attempt = wg_boundary_cli.WireGuardStatusDiagnosticAttempt{
        .err = .{
            .err = error.interface_missing,
            .diagnostic = wg_boundary.WireGuardPeerDiagnostic{
                .backend = "cli",
                .selected_interface = "wg-kgb0",
                .command = "wg show wg-kgb0 dump",
                .timeout_secs = null,
                .exit_code = 1,
                .error_kind = "interface_missing",
                .stderr_len = 15,
                .stdout_len = 0,
            },
        },
    };

    const evidence = wg_cli_facts.CliEvidence{
        .interface_name = "wg-kgb0",
        .os_link_seen = true,
        .os_link_kind = .non_wireguard,
        .wg_interfaces_seen = true,
        .wg_interface_list_contains_name = false,
        .wg_show_stderr_class = .no_such_device,
    };

    const facts = wg_cli_facts.factsFromDiagnosticAttempt(attempt, evidence);
    const diag_class = classifier.classifyWgStatus(facts);

    try testing.expectEqual(classifier.WgDiagnosticClass.interface_present_non_wireguard, diag_class);
}

test "classifier integration: wrong_namespace_or_unreachable" {
    const attempt = wg_boundary_cli.WireGuardStatusDiagnosticAttempt{
        .err = .{
            .err = error.interface_missing,
            .diagnostic = wg_boundary.WireGuardPeerDiagnostic{
                .backend = "cli",
                .selected_interface = "wg-kgb0",
                .command = "wg show wg-kgb0 dump",
                .timeout_secs = null,
                .exit_code = 1,
                .error_kind = "interface_missing",
                .stderr_len = 15,
                .stdout_len = 0,
            },
        },
    };

    const evidence = wg_cli_facts.CliEvidence{
        .interface_name = "wg-kgb0",
        .os_link_seen = true,
        .os_link_kind = .wireguard,
        .wg_interfaces_seen = true,
        .wg_interface_list_contains_name = false,
        .wg_show_stderr_class = .no_such_device,
    };

    const facts = wg_cli_facts.factsFromDiagnosticAttempt(attempt, evidence);
    const diag_class = classifier.classifyWgStatus(facts);

    try testing.expectEqual(classifier.WgDiagnosticClass.wrong_namespace_or_unreachable, diag_class);
}

// ============================================================================
// Stderr Classification Tests
// ============================================================================

test "classifyWgStderr: Operation not permitted" {
    const result = classifier.classifyWgStderr("wg: Operation not permitted");
    try testing.expectEqual(classifier.WgStderrClass.operation_not_permitted, result);
}

test "classifyWgStderr: Permission denied" {
    const result = classifier.classifyWgStderr("wg: Permission denied");
    try testing.expectEqual(classifier.WgStderrClass.permission_denied, result);
}

test "classifyWgStderr: No such device" {
    const result = classifier.classifyWgStderr("wg: No such device");
    try testing.expectEqual(classifier.WgStderrClass.no_such_device, result);
}

test "classifyWgStderr: empty string" {
    const result = classifier.classifyWgStderr("");
    try testing.expectEqual(classifier.WgStderrClass.none, result);
}

test "classifyWgStderr: unknown error" {
    const result = classifier.classifyWgStderr("something went wrong");
    try testing.expectEqual(classifier.WgStderrClass.other, result);
}

// ============================================================================
// OS Link Classification Tests
// ============================================================================

test "classifyIpLinkDetail: wireguard link" {
    const output = "3: wg-kgb0: <POINTOPOINT,NOARP,UP,LOWER_UP> mtu 1420 qdisc noqueue state UNKNOWN mode DEFAULT group default qlen 1000\n    link/wireguard";
    const result = classifier.classifyIpLinkDetail(output);
    try testing.expectEqual(classifier.OsLinkKind.wireguard, result);
}

test "classifyIpLinkDetail: non-wireguard link" {
    const output = "2: eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc pfifo_fast state UP mode DEFAULT group default qlen 1000\n    link/ether";
    const result = classifier.classifyIpLinkDetail(output);
    try testing.expectEqual(classifier.OsLinkKind.non_wireguard, result);
}

test "classifyIpLinkDetail: empty output" {
    const result = classifier.classifyIpLinkDetail("");
    try testing.expectEqual(classifier.OsLinkKind.non_wireguard, result);
}

// ============================================================================
// WG Interfaces Classification Tests
// ============================================================================

test "classifyWgInterfacesOutput: interface present" {
    const output = "wg-kgb0\nwg1";
    const result = classifier.classifyWgInterfacesOutput(output, "wg-kgb0");
    try testing.expectEqual(true, result);
}

test "classifyWgInterfacesOutput: interface absent" {
    const output = "wg1\nwg2";
    const result = classifier.classifyWgInterfacesOutput(output, "wg-kgb0");
    try testing.expectEqual(false, result);
}

test "classifyWgInterfacesOutput: empty output" {
    const result = classifier.classifyWgInterfacesOutput("", "wg-kgb0");
    try testing.expectEqual(false, result);
}

// ============================================================================
// Backend Coherence Tests
// ACT-HULK29R-ZIG016-WG-STATUS-BACKEND-COHERENCE
// ============================================================================

test "classifier: sysfs-visible CLI-invisible => wrong_namespace_or_unreachable" {
    // This is the key coherence test: when sysfs sees the interface but
    // wg CLI doesn't contain it, we should classify as namespace mismatch,
    // NOT as wireguard_interface_missing.
    //
    // This proves that:
    //   tunnel ok: detected tunnel interfaces: wg-kgb0
    //   wg_peers warn: wg wrong_namespace_or_unreachable: namespace mismatch
    //
    // is the correct coherent state.
    //
    // The production sysfs fallback returns .unknown for os_link_kind because
    // we cannot verify WireGuard link kind from sysfs alone. The classifier
    // must handle .unknown correctly to avoid false wireguard_interface_missing.
    const attempt = wg_boundary_cli.WireGuardStatusDiagnosticAttempt{
        .err = .{
            .err = error.interface_missing,
            .diagnostic = wg_boundary.WireGuardPeerDiagnostic{
                .backend = "cli",
                .selected_interface = "wg-kgb0",
                .command = "wg show wg-kgb0 dump",
                .timeout_secs = null,
                .exit_code = 1,
                .error_kind = "interface_missing",
                .stderr_len = 15,
                .stdout_len = 0,
            },
        },
    };

    // Evidence: sysfs sees the interface with unknown kind, but wg CLI doesn't
    const evidence = wg_cli_facts.CliEvidence{
        .interface_name = "wg-kgb0",
        .os_link_seen = true, // sysfs fallback detected it
        .os_link_kind = .unknown, // sysfs cannot verify WireGuard kind
        .wg_interfaces_seen = true, // wg command works
        .wg_interface_list_contains_name = false, // but doesn't see wg-kgb0
        .wg_show_stderr_class = .no_such_device,
    };

    const facts = wg_cli_facts.factsFromDiagnosticAttempt(attempt, evidence);
    const diag_class = classifier.classifyWgStatus(facts);

    // KEY ASSERTION: .unknown + os_link_seen + CLI invisible => wrong_namespace_or_unreachable
    try testing.expectEqual(classifier.WgDiagnosticClass.wrong_namespace_or_unreachable, diag_class);
}

test "classifier: true interface missing => wireguard_interface_missing" {
    // When BOTH sysfs and wg CLI don't see the interface, it's a true absence.
    const attempt = wg_boundary_cli.WireGuardStatusDiagnosticAttempt{
        .err = .{
            .err = error.interface_missing,
            .diagnostic = wg_boundary.WireGuardPeerDiagnostic{
                .backend = "cli",
                .selected_interface = "wg-kgb0",
                .command = "wg show wg-kgb0 dump",
                .timeout_secs = null,
                .exit_code = 1,
                .error_kind = "interface_missing",
                .stderr_len = 15,
                .stdout_len = 0,
            },
        },
    };

    // Evidence: neither sysfs nor wg CLI sees the interface
    const evidence = wg_cli_facts.CliEvidence{
        .interface_name = "wg-kgb0",
        .os_link_seen = false, // sysfs doesn't see it
        .os_link_kind = .missing,
        .wg_interfaces_seen = true, // wg command works
        .wg_interface_list_contains_name = false, // but doesn't see wg-kgb0
        .wg_show_stderr_class = .no_such_device,
    };

    const facts = wg_cli_facts.factsFromDiagnosticAttempt(attempt, evidence);
    const diag_class = classifier.classifyWgStatus(facts);

    // This should be wireguard_interface_missing - true absence
    try testing.expectEqual(classifier.WgDiagnosticClass.wireguard_interface_missing, diag_class);
}

test "classifyWgInterfacesOutput: with whitespace" {
    const output = "  wg-kgb0  \n  wg1  ";
    const result = classifier.classifyWgInterfacesOutput(output, "wg-kgb0");
    try testing.expectEqual(true, result);
}
