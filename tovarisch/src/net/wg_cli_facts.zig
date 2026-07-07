// wg_cli_facts.zig — WireGuard CLI fact collection for classification
//
// Part of wg_status_boundary_cli.zig (split to satisfy LLM-friendliness limits).
// Contains fact collection and conversion functions that bridge CLI diagnostics
// to the pure classifier.
//
// ACT-HULK29R-ZIG016-WG-STATUS-CLASSIFICATION-FIX:
// Provides factsFromDiagnosticAttempt() that converts CLI diagnostic attempts
// into structured WgInterfaceFacts for precise classification.

const std = @import("std");
const wg = @import("wg_status_boundary.zig");
const classifier = @import("wg_diagnostic_classifier.zig");

/// Default WireGuard interface name.
/// Re-exported from wg_status_boundary_cli for fact collection use.
pub const DEFAULT_WG_INTERFACE: [:0]const u8 = "wg-kgb0";

/// Converts a WireGuardStatusDiagnosticAttempt into WgInterfaceFacts for classification.
/// This allows the status_checks module to use the classifier for precise diagnostics.
pub fn factsFromDiagnosticAttempt(
    attempt: anytype,
    os_link_seen: bool,
    os_link_kind: classifier.OsLinkKind,
    stderr_content: []const u8,
) classifier.WgInterfaceFacts {
    return switch (attempt) {
        .ok => |ok| classifier.WgInterfaceFacts{
            .configured_name = DEFAULT_WG_INTERFACE,
            .os_link_seen = os_link_seen,
            .os_link_kind = os_link_kind,
            .wg_tool_seen = true,
            .wg_interfaces_seen = true,
            .wg_interface_list_contains_name = true,
            .wg_show_exit_code = 0,
            .wg_show_stderr_class = .none,
            .wg_dump_succeeded = true,
            .wg_dump_peer_count = ok.status.peer_count,
            .wg_dump_has_handshake = ok.status.hasHandshake(),
            .wg_dump_malformed = false,
        },
        .err => |bad| blk: {
            // Check if the error is backend_missing
            const is_backend_missing = switch (bad.err) {
                error.backend_missing => true,
                else => false,
            };
            const is_malformed = switch (bad.err) {
                error.malformed_output => true,
                else => false,
            };
            break :blk classifier.WgInterfaceFacts{
                .configured_name = DEFAULT_WG_INTERFACE,
                .os_link_seen = os_link_seen,
                .os_link_kind = os_link_kind,
                .wg_tool_seen = !is_backend_missing,
                .wg_interfaces_seen = !is_backend_missing,
                .wg_interface_list_contains_name = false,
                .wg_show_exit_code = bad.diagnostic.exit_code,
                .wg_show_stderr_class = classifier.classifyWgStderr(stderr_content),
                .wg_dump_succeeded = false,
                .wg_dump_peer_count = 0,
                .wg_dump_has_handshake = false,
                .wg_dump_malformed = is_malformed,
            };
        },
    };
}
