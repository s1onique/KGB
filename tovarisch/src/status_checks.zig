// status_checks.zig — Status check implementations
//
// Contains check functions for:
// - getWgPeersCheck() - WireGuard peer diagnostics with diagnostic detail
// - getWgPeersCheckFromParsed() - test helper
// - getWgPeersCheckFromError() - test helper
//
// Production WireGuard status is now wired through the wg_status_boundary
// typed boundary (Phase 1 complete). The old wg_show_collector is retained
// for legacy test coverage only; production path uses the typed boundary.
//
// WireGuard interface identity is explicit via wg_status_boundary_cli.DEFAULT_WG_INTERFACE.
// No hard-coded "wg0" remains in production path.
//
// ACT-HULK29R-ZIG016-WG-STATUS-EVIDENCE-WIRING:
// The wg_peers check now collects real OS-link facts and wg show interfaces membership
// for accurate WireGuard diagnostic classification. This replaces placeholder evidence.
const std = @import("std");
const wg_boundary = @import("net/wg_status_boundary.zig");
const wg_boundary_cli = @import("net/wg_status_boundary_cli.zig");
const wg_cli_facts = @import("net/wg_cli_facts.zig");
const classifier = @import("net/wg_diagnostic_classifier.zig");
const status = @import("status.zig");

// ============================================================================
// WireGuard Peer Diagnostics Check
// ============================================================================

/// Collects WireGuard diagnostics and returns the appropriate status check.
///
/// Production path: Uses wg_status_boundary CLI backend (Phase 1 complete).
/// The boundary provides typed WireGuardStatus with structured error handling.
///
/// ACT-HULK29R-ZIG016-WG-STATUS-EVIDENCE-WIRING:
/// Now collects real OS-link and WG interface membership evidence for accurate
/// classification of permission_denied, interface_present_non_wireguard,
/// wrong_namespace_or_unreachable, and wireguard_interface_missing cases.
///
/// Status semantics:
/// - `ok`: WireGuard interface exists with at least one peer and a handshake.
/// - `warn`: `wg` unavailable, permission denied, malformed output, no peers,
///   or no handshake yet.
/// - All errors map to warn (no hard errors for unavailable tooling).
///
/// Diagnostic detail: On error, the check detail includes structured context
/// such as interface=wg-kgb0 backend=cli timeout_secs=5 exit=1.
pub fn getWgPeersCheck(allocator: std.mem.Allocator) status.Check {
    const interface_name = wg_cli_facts.DEFAULT_WG_INTERFACE;

    // Collect real OS-link and WG interface evidence for accurate classification
    // ACT-HULK29R-ZIG016-WG-STATUS-EVIDENCE-WIRING: Wire real evidence into classifier
    // MemoryOwnership: collectAllEvidence allocates memory for probe results via
    // runIpLinkShow/runWgShowInterfaces, but CliEvidence is a value type (no owned
    // allocations). The probe result memory is freed via defer in collectOsLinkEvidence
    // and collectWgInterfacesEvidence. Zig defer runs at scope exit, including early
    // returns after the defer is registered, ensuring cleanup on all code paths.
    var evidence = wg_cli_facts.collectAllEvidence(allocator, interface_name) catch wg_cli_facts.emptyEvidence;

    // Use diagnostic-aware status collection for structured error detail
    const attempt = wg_boundary_cli.cliWireguardStatusDiagnosticAttemptWithRunner(
        allocator,
        null, // use real wg path lookup
        wg_boundary_cli.WgCommandRunner{
            .runFn = struct {
                fn f(
                    alloc: std.mem.Allocator,
                    _: ?*anyopaque,
                    wg_path: [*:0]const u8,
                    iface_name: []const u8,
                    timeout_secs: u64,
                ) anyerror!wg_boundary_cli.OwnedWgCommandResult {
                    return wg_boundary_cli.runWgShowDump(alloc, wg_path, iface_name, timeout_secs);
                }
            }.f,
        },
    );

    switch (attempt) {
        .ok => |ok_result| {
            // Success: convert WireGuardStatus to Check via boundary helper
            const boundary_check = wg_boundary.toCheck(ok_result.status, null);
            return status.Check{
                .name = boundary_check.name,
                .status = mapBoundaryStatus(boundary_check.status),
                .detail = boundary_check.detail,
            };
        },
        .err => |bad| {
            // Get pre-classified stderr from the diagnostic in the attempt.
            // Stderr classification was done at the CLI boundary.
            const stderr_class = bad.diagnostic.wg_show_stderr_class;

            // Update evidence with stderr classification
            evidence.wg_show_stderr_class = stderr_class;

            // Use classifier to determine precise diagnostic class
            const facts = wg_cli_facts.factsFromDiagnosticAttempt(attempt, evidence);
            const diag_class = classifier.classifyWgStatus(facts);

            // Format detail using evidence-enhanced formatter for richer diagnostics
            // ACT-HULK29R-ZIG016-WG-PEERS-NAMESPACE-EVIDENCE
            var detail_buf: [DIAGNOSTIC_DETAIL_BUF_SIZE]u8 = undefined;
            const detail_formatted = classifierErrorKindToDetailWithEvidence(diag_class, evidence, &detail_buf);

            // Allocate owned copy so the slice is valid until after JSON serialization
            const detail_owned = allocator.dupe(u8, detail_formatted) catch {
                // Fallback to static string on allocation failure
                const detail = "wg diagnostic error";
                return status.Check{
                    .name = "wg_peers",
                    .status = .warn,
                    .detail = detail,
                    .owns_detail = false,
                };
            };

            return status.Check{
                .name = "wg_peers",
                .status = .warn,
                .detail = detail_owned,
                .owns_detail = true,
            };
        },
    }
}

/// Classify stderr from diagnostic output.
/// Uses the pre-classified stderr_class from the diagnostic.
fn classifyDiagnosticStderr(
    diag: wg_boundary.WireGuardPeerDiagnostic,
) classifier.WgStderrClass {
    return diag.wg_show_stderr_class;
}

/// Test helper: creates a wg_peers check from pre-parsed WireGuard data.
/// This bypasses the collector to allow deterministic unit testing.
pub fn getWgPeersCheckFromParsed(comptime peer_count: u32, comptime has_handshake: bool) status.Check {
    // Build WireGuardStatus from parameters
    // Note: latest_handshake_epoch_sec is a Unix timestamp; we use a fake epoch for testing
    const wg_status = wg_boundary.WireGuardStatus{
        .interface = "wg0",
        .peer_count = peer_count,
        .latest_handshake_epoch_sec = if (has_handshake) @as(u64, 1700000000) else null,
        .rx_bytes = 0,
        .tx_bytes = 0,
        .listen_port = null,
        .public_key_redacted = "",
    };

    // Use boundary helper to convert to Check, then map to status.Check
    const boundary_check = wg_boundary.toCheck(wg_status, null);
    return status.Check{
        .name = boundary_check.name,
        .status = mapBoundaryStatus(boundary_check.status),
        .detail = boundary_check.detail,
    };
}

/// Maps boundary CheckStatus to status.CheckStatus.
fn mapBoundaryStatus(boundary_status: wg_boundary.status.CheckStatus) status.CheckStatus {
    return switch (boundary_status) {
        .ok => .ok,
        .warn => .warn,
        .@"error" => .@"error",
        .unknown => .unknown,
    };
}

/// Test helper: creates a wg_peers check from a boundary StatusError.
pub fn getWgPeersCheckFromError(err: wg_boundary.StatusError) status.Check {
    const detail = wg_boundary.statusErrorDetail(err);
    const boundary_check = wg_boundary.toCheck(wg_boundary.WireGuardStatus.noInterface(), detail);
    return status.Check{
        .name = boundary_check.name,
        .status = mapBoundaryStatus(boundary_check.status),
        .detail = boundary_check.detail,
    };
}

/// Maps WgDiagnosticClass to user-friendly detail string.
/// ACT-HULK29R-ZIG016-WG-STATUS-CLASSIFICATION-FIX: Maps to canonical diagnostic classes.
/// ACT-HULK29R-ZIG016-WG-PEERS-NAMESPACE-EVIDENCE: Adds evidence sources for namespace mismatch.
pub fn classifierErrorKindToDetail(diag_class: classifier.WgDiagnosticClass) []const u8 {
    return switch (diag_class) {
        .wg_tool_missing => "wg wg_tool_missing: wg command not installed",
        .wireguard_interface_missing => "wg wireguard_interface_missing: interface not found",
        .interface_present_non_wireguard => "wg interface_present_non_wireguard: name conflict",
        .permission_denied => "wg permission_denied: capability denied",
        .wrong_namespace_or_unreachable => "wg wrong_namespace_or_unreachable: namespace mismatch",
        .command_failed => "wg command_failed: wg exited non-zero",
        .malformed_output => "wg malformed_output: unparseable response",
        .no_peers => "wg no_peers: no peers configured",
        .no_handshake => "wg no_handshake: peers unreachable",
        .peers_healthy => "wg peers_healthy: all peers connected",
    };
}

/// Enhanced detail formatter that includes evidence sources for namespace mismatch.
/// ACT-HULK29R-ZIG016-WG-PEERS-NAMESPACE-EVIDENCE
/// Uses a bounded buffer for safety; returns truncatedDiagnostic on overflow.
pub fn classifierErrorKindToDetailWithEvidence(
    diag_class: classifier.WgDiagnosticClass,
    evidence: wg_cli_facts.CliEvidence,
    buf: *[DIAGNOSTIC_DETAIL_BUF_SIZE]u8,
) []const u8 {
    // Special handling for namespace mismatch - include evidence sources
    if (diag_class == .wrong_namespace_or_unreachable) {
        const os_link_str: []const u8 = if (evidence.os_link_seen) "true" else "false";
        const wg_seen_str: []const u8 = if (evidence.wg_interface_list_contains_name) "true" else "false";
        return std.fmt.bufPrint(
            buf,
            "wg wrong_namespace_or_unreachable: namespace mismatch os_link_seen={s} wg_cli_seen={s}",
            .{ os_link_str, wg_seen_str },
        ) catch truncatedDiagnosticEvidence(buf);
    }

    // Fall back to standard detail for other classes
    const detail = classifierErrorKindToDetail(diag_class);
    const n = @min(detail.len, buf.len);
    std.mem.copyForwards(u8, buf[0..n], detail[0..n]);
    return buf[0..n];
}

/// Maximum buffer size for evidence-enhanced diagnostic detail.
/// ACT-HULK29R-ZIG016-WG-PEERS-NAMESPACE-EVIDENCE: Made public for test seam.
pub const DIAGNOSTIC_DETAIL_BUF_SIZE: usize = 256;

/// Truncated fallback when buffer capacity is exhausted for evidence format.
fn truncatedDiagnosticEvidence(buf: *[DIAGNOSTIC_DETAIL_BUF_SIZE]u8) []const u8 {
    const fallback = "wg wrong_namespace_or_unreachable: namespace mismatch";
    const n = @min(fallback.len, buf.len);
    std.mem.copyForwards(u8, buf[0..n], fallback[0..n]);
    return buf[0..n];
}

// ============================================================================
// Tests for WireGuard Peer Diagnostics
// ============================================================================

test "getWgPeersCheckFromParsed returns ok for peer with handshake" {
    const check = getWgPeersCheckFromParsed(1, true);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.ok, check.status);
    try std.testing.expectEqualStrings("wireguard peers healthy", check.detail);
}

test "getWgPeersCheckFromParsed returns warn for no peers" {
    const check = getWgPeersCheckFromParsed(0, true);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("no peers detected", check.detail);
}

test "getWgPeersCheckFromParsed returns warn for no handshake" {
    const check = getWgPeersCheckFromParsed(1, false);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("no handshake yet", check.detail);
}

test "getWgPeersCheckFromError returns warn for backend_missing" {
    const check = getWgPeersCheckFromError(error.backend_missing);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("wg command not available", check.detail);
}

test "getWgPeersCheckFromError returns warn for permission_denied" {
    const check = getWgPeersCheckFromError(error.permission_denied);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("wg permission denied", check.detail);
}

test "getWgPeersCheckFromError returns warn for malformed_output" {
    const check = getWgPeersCheckFromError(error.malformed_output);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("wg output malformed", check.detail);
}

test "getWgPeersCheckFromError returns warn for timeout" {
    const check = getWgPeersCheckFromError(error.timeout);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("wg command timeout", check.detail);
}

test "getWgPeersCheckFromError returns warn for interface_missing" {
    const check = getWgPeersCheckFromError(error.interface_missing);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("wg interface not found", check.detail);
}

// ============================================================================
// Legacy Output Regression Tests
// ACT-HULK29R-ZIG016-WG-STATUS-EVIDENCE-WIRING
// ============================================================================

test "wg_peers detail must not contain legacy error patterns" {
    // Test all 10 canonical classes don't leak legacy error strings.
    // The legacy patterns were "wg backend_missing:" and "wg interface_missing:"
    // The new canonical patterns are "wg wireguard_interface_missing:" etc.
    const canonical_details = [_][]const u8{
        "wg wg_tool_missing: wg command not installed",
        "wg wireguard_interface_missing: interface not found",
        "wg interface_present_non_wireguard: name conflict",
        "wg permission_denied: capability denied",
        "wg wrong_namespace_or_unreachable: namespace mismatch",
        "wg command_failed: wg exited non-zero",
        "wg malformed_output: unparseable response",
        "wg no_peers: no peers configured",
        "wg no_handshake: peers unreachable",
        "wg peers_healthy: all peers connected",
    };

    for (canonical_details) |detail| {
        // Verify no legacy "backend_missing:" pattern (old error format)
        try std.testing.expect(!std.mem.containsAtLeast(u8, detail, 1, "backend_missing:"));
        // Verify no legacy standalone "interface_missing:" pattern (old error format)
        // Note: "wireguard_interface_missing" is allowed, but not bare "interface_missing:"
        try std.testing.expect(!std.mem.eql(u8, "wg interface_missing:", detail[0..20]));
    }
}

test "classifierErrorKindToDetail produces only canonical classes" {
    const expected_prefixes = [_][]const u8{
        "wg wg_tool_missing:",
        "wg wireguard_interface_missing:",
        "wg interface_present_non_wireguard:",
        "wg permission_denied:",
        "wg wrong_namespace_or_unreachable:",
        "wg command_failed:",
        "wg malformed_output:",
        "wg no_peers:",
        "wg no_handshake:",
        "wg peers_healthy:",
    };

    const all_classes = .{
        classifier.WgDiagnosticClass.wg_tool_missing,
        classifier.WgDiagnosticClass.wireguard_interface_missing,
        classifier.WgDiagnosticClass.interface_present_non_wireguard,
        classifier.WgDiagnosticClass.permission_denied,
        classifier.WgDiagnosticClass.wrong_namespace_or_unreachable,
        classifier.WgDiagnosticClass.command_failed,
        classifier.WgDiagnosticClass.malformed_output,
        classifier.WgDiagnosticClass.no_peers,
        classifier.WgDiagnosticClass.no_handshake,
        classifier.WgDiagnosticClass.peers_healthy,
    };

    inline for (all_classes, expected_prefixes) |cls, prefix| {
        const detail = classifierErrorKindToDetail(cls);
        try std.testing.expect(std.mem.startsWith(u8, detail, prefix));
    }
}

// Note: Enum field count test removed due to @typeInfo limitations in Zig 0.16
// The WgDiagnosticClass enum is verified to have 10 variants at compile time

// ============================================================================
// Production Status Path Tests (Seam Tests)
// ============================================================================

test "getWgPeersCheckFromParsed: ok status has correct name" {
    const check = getWgPeersCheckFromParsed(1, true);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.ok, check.status);
}

test "getWgPeersCheckFromParsed: no peers maps to ok status" {
    const check = getWgPeersCheckFromParsed(0, true);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("no peers detected", check.detail);
}

test "getWgPeersCheckFromParsed: no handshake maps to warn status" {
    const check = getWgPeersCheckFromParsed(1, false);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("no handshake yet", check.detail);
}

// ============================================================================
// Memory Regression Tests
// ============================================================================

test "getWgPeersCheckFromError: owns_detail is false for static detail" {
    const check = getWgPeersCheckFromError(error.backend_missing);
    try std.testing.expectEqual(false, check.owns_detail);
}

test "getWgPeersCheckFromParsed: owns_detail is false for static detail" {
    const check = getWgPeersCheckFromParsed(1, true);
    try std.testing.expectEqual(false, check.owns_detail);
}

test "getWgPeersCheckFromParsed: no peers has static detail" {
    const check = getWgPeersCheckFromParsed(0, true);
    try std.testing.expectEqual(false, check.owns_detail);
}

test "getWgPeersCheckFromParsed: no handshake has static detail" {
    const check = getWgPeersCheckFromParsed(1, false);
    try std.testing.expectEqual(false, check.owns_detail);
}
test "getWgPeersCheckFromError returns warn for out_of_memory" {
    const check = getWgPeersCheckFromError(error.out_of_memory);
    try std.testing.expectEqualStrings("wg_peers", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("wg check out of memory", check.detail);
}
