// wg_cli_facts.zig — WireGuard CLI fact collection for classification
//
// Part of wg_status_boundary_cli.zig (split to satisfy LLM-friendliness limits).
// Contains fact collection and conversion functions that bridge CLI diagnostics
// to the pure classifier.
//
// ACT-HULK29R-ZIG016-WG-STATUS-EVIDENCE-WIRING:
// Extended to collect real OS-link facts and wg show interfaces membership
// for accurate WireGuard diagnostic classification.

const std = @import("std");
const wg = @import("wg_status_boundary.zig");
const classifier = @import("wg_diagnostic_classifier.zig");
const probes = @import("wg_cli_probes.zig");

// Re-export from probes for backward compatibility.
pub const OwnedWgCommandResult = probes.OwnedWgCommandResult;
pub const DEFAULT_WG_INTERFACE = probes.DEFAULT_WG_INTERFACE;
pub const PROBE_TIMEOUT_SECS = probes.PROBE_TIMEOUT_SECS;
pub const runIpLinkShow = probes.runIpLinkShow;
pub const runWgShowInterfaces = probes.runWgShowInterfaces;
pub const findWgCommand = probes.findWgCommand;

/// Structured evidence for WireGuard classification.
/// Collected from read-only OS and WG CLI probes.
pub const CliEvidence = struct {
    /// Interface name being checked.
    interface_name: []const u8,
    /// Whether OS-level link was detected for the interface.
    os_link_seen: bool,
    /// Kind of OS link detected.
    os_link_kind: classifier.OsLinkKind,
    /// Whether wg show interfaces succeeded.
    wg_interfaces_seen: bool,
    /// Whether wg show interfaces output contains our interface name.
    wg_interface_list_contains_name: bool,
    /// Classified stderr from wg show dump command.
    wg_show_stderr_class: classifier.WgStderrClass,
};

/// Default empty evidence with placeholder values.
pub const emptyEvidence = CliEvidence{
    .interface_name = DEFAULT_WG_INTERFACE,
    .os_link_seen = false,
    .os_link_kind = .unknown,
    .wg_interfaces_seen = false,
    .wg_interface_list_contains_name = false,
    .wg_show_stderr_class = .none,
};

/// Collect OS-link evidence for the configured interface.
/// Uses `ip -d link show <interface>` probe.
pub fn collectOsLinkEvidence(
    allocator: std.mem.Allocator,
    interface_name: []const u8,
) !struct {
    os_link_seen: bool,
    os_link_kind: classifier.OsLinkKind,
} {
    var result = runIpLinkShow(allocator, interface_name, PROBE_TIMEOUT_SECS) catch {
        return .{
            .os_link_seen = false,
            .os_link_kind = .probe_failed,
        };
    };
    defer result.deinit(allocator);

    // ip link returns exit 1 if interface doesn't exist
    if (result.exit_code == 1) {
        return .{
            .os_link_seen = false,
            .os_link_kind = .missing,
        };
    }

    if (result.exit_code != 0) {
        return .{
            .os_link_seen = false,
            .os_link_kind = .probe_failed,
        };
    }

    // Parse stdout to determine link kind
    const kind = classifier.classifyIpLinkDetail(result.stdout);
    return .{
        .os_link_seen = true,
        .os_link_kind = kind,
    };
}

/// Collect WG interfaces evidence.
/// Uses `wg show interfaces` probe.
pub fn collectWgInterfacesEvidence(
    allocator: std.mem.Allocator,
    interface_name: []const u8,
) !struct {
    wg_interfaces_seen: bool,
    wg_interface_list_contains_name: bool,
} {
    const wg_path = findWgCommand() orelse {
        return .{
            .wg_interfaces_seen = false,
            .wg_interface_list_contains_name = false,
        };
    };

    var result = runWgShowInterfaces(allocator, wg_path, PROBE_TIMEOUT_SECS) catch {
        return .{
            .wg_interfaces_seen = false,
            .wg_interface_list_contains_name = false,
        };
    };
    defer result.deinit(allocator);

    // wg show interfaces returns exit 1 if wg tool has issues
    // exit 0 means success (even with empty output)
    if (result.exit_code == 127) {
        return .{
            .wg_interfaces_seen = false,
            .wg_interface_list_contains_name = false,
        };
    }

    const contains_name = classifier.classifyWgInterfacesOutput(result.stdout, interface_name);
    return .{
        .wg_interfaces_seen = true,
        .wg_interface_list_contains_name = contains_name,
    };
}

/// Collect all CLI evidence for WireGuard classification.
pub fn collectAllEvidence(
    allocator: std.mem.Allocator,
    interface_name: []const u8,
) !CliEvidence {
    const os_link = try collectOsLinkEvidence(allocator, interface_name);
    const wg_interfaces = try collectWgInterfacesEvidence(allocator, interface_name);

    return CliEvidence{
        .interface_name = interface_name,
        .os_link_seen = os_link.os_link_seen,
        .os_link_kind = os_link.os_link_kind,
        .wg_interfaces_seen = wg_interfaces.wg_interfaces_seen,
        .wg_interface_list_contains_name = wg_interfaces.wg_interface_list_contains_name,
        .wg_show_stderr_class = .none, // Set separately from command result
    };
}

/// Converts a WireGuardStatusDiagnosticAttempt into WgInterfaceFacts for classification.
/// This allows the status_checks module to use the classifier for precise diagnostics.
///
/// ACT-HULK29R-ZIG016-WG-STATUS-EVIDENCE-WIRING:
/// Now accepts structured CliEvidence for accurate classification.
pub fn factsFromDiagnosticAttempt(
    attempt: anytype,
    evidence: CliEvidence,
) classifier.WgInterfaceFacts {
    return switch (attempt) {
        .ok => |ok| classifier.WgInterfaceFacts{
            .configured_name = evidence.interface_name,
            .os_link_seen = evidence.os_link_seen,
            .os_link_kind = evidence.os_link_kind,
            .wg_tool_seen = true,
            .wg_interfaces_seen = evidence.wg_interfaces_seen,
            .wg_interface_list_contains_name = evidence.wg_interface_list_contains_name,
            .wg_show_exit_code = 0,
            .wg_show_stderr_class = .none,
            .wg_dump_succeeded = true,
            .wg_dump_peer_count = ok.status.peer_count,
            .wg_dump_has_handshake = ok.status.hasHandshake(),
            .wg_dump_malformed = false,
        },
        .err => |bad| blk: {
            const is_backend_missing = switch (bad.err) {
                error.backend_missing => true,
                else => false,
            };
            const is_malformed = switch (bad.err) {
                error.malformed_output => true,
                else => false,
            };
            break :blk classifier.WgInterfaceFacts{
                .configured_name = evidence.interface_name,
                .os_link_seen = evidence.os_link_seen,
                .os_link_kind = evidence.os_link_kind,
                .wg_tool_seen = !is_backend_missing,
                .wg_interfaces_seen = !is_backend_missing and evidence.wg_interfaces_seen,
                .wg_interface_list_contains_name = !is_backend_missing and evidence.wg_interface_list_contains_name,
                .wg_show_exit_code = bad.diagnostic.exit_code,
                .wg_show_stderr_class = evidence.wg_show_stderr_class,
                .wg_dump_succeeded = false,
                .wg_dump_peer_count = 0,
                .wg_dump_has_handshake = false,
                .wg_dump_malformed = is_malformed,
            };
        },
    };
}
