// wg_cli_facts_classifier_tests.zig — WireGuard CLI classifier integration tests (part 2)
//
// Part of wg_cli_facts_tests.zig (split to satisfy LLM-friendliness limits).
// Contains classifier integration tests for status classification.
//
// ACT-HULK29R-ZIG016-WG-STATUS-EVIDENCE-WIRING

const std = @import("std");
const wg_cli_facts = @import("wg_cli_facts.zig");
const classifier = @import("wg_diagnostic_classifier.zig");
const wg_boundary = @import("wg_status_boundary.zig");
const wg_boundary_cli = @import("wg_status_boundary_cli.zig");
const testing = std.testing;

test "classifier integration: no_peers" {
    const ok_status = wg_boundary.WireGuardStatus{
        .interface = "wg-kgb0",
        .peer_count = 0,
        .latest_handshake_epoch_sec = null,
        .rx_bytes = 0,
        .tx_bytes = 0,
        .listen_port = 51820,
        .public_key_redacted = "",
    };

    const attempt = wg_boundary_cli.WireGuardStatusDiagnosticAttempt{
        .ok = .{
            .status = ok_status,
            .diagnostic = wg_boundary.WireGuardPeerDiagnostic{
                .backend = "cli",
                .selected_interface = "wg-kgb0",
                .command = "wg show wg-kgb0 dump",
                .timeout_secs = null,
                .exit_code = 0,
                .error_kind = "ok",
                .stderr_len = 0,
                .stdout_len = 50,
            },
        },
    };

    const evidence = wg_cli_facts.CliEvidence{
        .interface_name = "wg-kgb0",
        .os_link_seen = true,
        .os_link_kind = .wireguard,
        .wg_interfaces_seen = true,
        .wg_interface_list_contains_name = true,
        .wg_show_stderr_class = .none,
    };

    const facts = wg_cli_facts.factsFromDiagnosticAttempt(attempt, evidence);
    const diag_class = classifier.classifyWgStatus(facts);

    try testing.expectEqual(classifier.WgDiagnosticClass.no_peers, diag_class);
}

test "classifier integration: no_handshake" {
    const ok_status = wg_boundary.WireGuardStatus{
        .interface = "wg-kgb0",
        .peer_count = 2,
        .latest_handshake_epoch_sec = null,
        .rx_bytes = 1000,
        .tx_bytes = 2000,
        .listen_port = 51820,
        .public_key_redacted = "",
    };

    const attempt = wg_boundary_cli.WireGuardStatusDiagnosticAttempt{
        .ok = .{
            .status = ok_status,
            .diagnostic = wg_boundary.WireGuardPeerDiagnostic{
                .backend = "cli",
                .selected_interface = "wg-kgb0",
                .command = "wg show wg-kgb0 dump",
                .timeout_secs = null,
                .exit_code = 0,
                .error_kind = "ok",
                .stderr_len = 0,
                .stdout_len = 200,
            },
        },
    };

    const evidence = wg_cli_facts.CliEvidence{
        .interface_name = "wg-kgb0",
        .os_link_seen = true,
        .os_link_kind = .wireguard,
        .wg_interfaces_seen = true,
        .wg_interface_list_contains_name = true,
        .wg_show_stderr_class = .none,
    };

    const facts = wg_cli_facts.factsFromDiagnosticAttempt(attempt, evidence);
    const diag_class = classifier.classifyWgStatus(facts);

    try testing.expectEqual(classifier.WgDiagnosticClass.no_handshake, diag_class);
}

test "classifier integration: peers_healthy" {
    const ok_status = wg_boundary.WireGuardStatus{
        .interface = "wg-kgb0",
        .peer_count = 2,
        .latest_handshake_epoch_sec = 1700000000,
        .rx_bytes = 1000,
        .tx_bytes = 2000,
        .listen_port = 51820,
        .public_key_redacted = "",
    };

    const attempt = wg_boundary_cli.WireGuardStatusDiagnosticAttempt{
        .ok = .{
            .status = ok_status,
            .diagnostic = wg_boundary.WireGuardPeerDiagnostic{
                .backend = "cli",
                .selected_interface = "wg-kgb0",
                .command = "wg show wg-kgb0 dump",
                .timeout_secs = null,
                .exit_code = 0,
                .error_kind = "ok",
                .stderr_len = 0,
                .stdout_len = 200,
            },
        },
    };

    const evidence = wg_cli_facts.CliEvidence{
        .interface_name = "wg-kgb0",
        .os_link_seen = true,
        .os_link_kind = .wireguard,
        .wg_interfaces_seen = true,
        .wg_interface_list_contains_name = true,
        .wg_show_stderr_class = .none,
    };

    const facts = wg_cli_facts.factsFromDiagnosticAttempt(attempt, evidence);
    const diag_class = classifier.classifyWgStatus(facts);

    try testing.expectEqual(classifier.WgDiagnosticClass.peers_healthy, diag_class);
}

test "classifier integration: command_failed" {
    const attempt = wg_boundary_cli.WireGuardStatusDiagnosticAttempt{
        .err = .{
            .err = error.command_failed,
            .diagnostic = wg_boundary.WireGuardPeerDiagnostic{
                .backend = "cli",
                .selected_interface = "wg-kgb0",
                .command = "wg show wg-kgb0 dump",
                .timeout_secs = null,
                .exit_code = 2,
                .error_kind = "command_failed",
                .stderr_len = 50,
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
        .wg_show_stderr_class = .other,
    };

    const facts = wg_cli_facts.factsFromDiagnosticAttempt(attempt, evidence);
    const diag_class = classifier.classifyWgStatus(facts);

    try testing.expectEqual(classifier.WgDiagnosticClass.command_failed, diag_class);
}
