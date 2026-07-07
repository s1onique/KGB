// wg_diagnostic_classifier.zig — Pure WireGuard diagnostic classification
//
// ACT-HULK29R-ZIG016-WG-STATUS-CLASSIFICATION-FIX
//
// Provides pure classification functions that separate:
// - OS link presence from WireGuard interface visibility
// - wg CLI availability from interface inspection
// - Permission failures from namespace/unreachable cases
// - Malformed output from healthy states
//
// This module contains NO command execution and NO allocations.

const std = @import("std");

// ============================================================================
// Diagnostic Classification Enums
// ============================================================================

/// OS link kind as detected by tunnel check.
pub const OsLinkKind = enum {
    /// Link state unknown or not checked.
    unknown,
    /// No OS-level link exists.
    missing,
    /// OS link exists and is a WireGuard interface.
    wireguard,
    /// OS link exists but is NOT a WireGuard interface.
    non_wireguard,
    /// OS link probe failed (e.g., sysfs inaccessible).
    probe_failed,
};

/// Classification of stderr output from wg commands.
pub const WgStderrClass = enum {
    /// No stderr output.
    none,
    /// "No such device" - interface does not exist as WireGuard.
    no_such_device,
    /// "Operation not permitted" - permission denied.
    operation_not_permitted,
    /// "Permission denied" - capability/capability issue.
    permission_denied,
    /// wg command not found.
    command_not_found,
    /// Command timed out.
    timeout,
    /// Other/unknown error.
    other,
};

/// Final WireGuard diagnostic classification.
pub const WgDiagnosticClass = enum {
    /// WireGuard tool (wg) is not available on this system.
    wg_tool_missing,
    /// WireGuard interface does not exist.
    wireguard_interface_missing,
    /// OS link exists but is not a WireGuard interface.
    interface_present_non_wireguard,
    /// Permission/capability denied when accessing interface.
    permission_denied,
    /// Interface exists but is in wrong namespace or unreachable.
    wrong_namespace_or_unreachable,
    /// Command failed with non-zero exit code.
    command_failed,
    /// Output was malformed/unparseable.
    malformed_output,
    /// WireGuard interface exists with no peers configured.
    no_peers,
    /// WireGuard interface exists with peers but no handshake.
    no_handshake,
    /// WireGuard interface exists with healthy peers.
    peers_healthy,
};

/// Structured facts about a WireGuard interface for classification.
pub const WgInterfaceFacts = struct {
    /// The interface name we're checking.
    configured_name: []const u8,

    /// Whether tunnel check detected an OS-level link.
    os_link_seen: bool,
    /// Kind of OS link detected.
    os_link_kind: OsLinkKind,

    /// Whether wg command was found/available.
    wg_tool_seen: bool,
    /// Whether wg show interfaces succeeded.
    wg_interfaces_seen: bool,
    /// Whether wg show interfaces output contains our interface name.
    wg_interface_list_contains_name: bool,

    /// Exit code from wg show <interface>.
    wg_show_exit_code: ?u8,
    /// Stderr classification from wg show <interface>.
    wg_show_stderr_class: WgStderrClass,

    /// Whether wg show dump succeeded.
    wg_dump_succeeded: bool,
    /// Number of peers from dump.
    wg_dump_peer_count: u32,
    /// Whether dump showed any handshake.
    wg_dump_has_handshake: bool,
    /// Whether dump output was malformed.
    wg_dump_malformed: bool,
};

// ============================================================================
// Pure Classification Functions
// ============================================================================

/// Classifies WireGuard interface status from collected facts.
/// This is a pure function - no side effects, no allocations.
pub fn classifyWgStatus(facts: WgInterfaceFacts) WgDiagnosticClass {
    // Check wg tool availability first
    if (!facts.wg_tool_seen) {
        return .wg_tool_missing;
    }

    // Check for permission/denial patterns
    if (facts.wg_show_stderr_class == .operation_not_permitted or
        facts.wg_show_stderr_class == .permission_denied)
    {
        return .permission_denied;
    }

    // Check for malformed output
    if (facts.wg_dump_malformed) {
        return .malformed_output;
    }

    // Check dump success for peer health
    if (facts.wg_dump_succeeded) {
        if (facts.wg_dump_peer_count == 0) {
            return .no_peers;
        }
        if (!facts.wg_dump_has_handshake) {
            return .no_handshake;
        }
        return .peers_healthy;
    }

    // Dump failed - check why
    if (facts.wg_show_stderr_class == .no_such_device) {
        // Interface doesn't exist as WireGuard - check if OS link exists
        if (!facts.os_link_seen) {
            return .wireguard_interface_missing;
        }
        // OS link exists but wg doesn't see it
        if (facts.os_link_kind == .non_wireguard) {
            return .interface_present_non_wireguard;
        }
        // OS link exists but wg can't see it - namespace or other issue
        return .wrong_namespace_or_unreachable;
    }

    // Check if interface is missing from wg show interfaces list
    if (!facts.wg_interface_list_contains_name) {
        if (!facts.os_link_seen) {
            return .wireguard_interface_missing;
        }
        if (facts.os_link_kind == .non_wireguard) {
            return .interface_present_non_wireguard;
        }
        return .wrong_namespace_or_unreachable;
    }

    // Command failed for other reason
    if (facts.wg_show_exit_code != null and facts.wg_show_exit_code != 0) {
        return .command_failed;
    }

    // Default: wireguard interface missing
    return .wireguard_interface_missing;
}

/// Parses stderr to classify the error type.
pub fn classifyWgStderr(stderr: []const u8) WgStderrClass {
    if (stderr.len == 0) {
        return .none;
    }

    // Check for common patterns (case-sensitive per Linux error messages)
    if (std.mem.containsAtLeast(u8, stderr, 1, "No such device")) {
        return .no_such_device;
    }
    if (std.mem.containsAtLeast(u8, stderr, 1, "Operation not permitted")) {
        return .operation_not_permitted;
    }
    if (std.mem.containsAtLeast(u8, stderr, 1, "Permission denied")) {
        return .permission_denied;
    }
    if (std.mem.containsAtLeast(u8, stderr, 1, "wg: command not found")) {
        return .command_not_found;
    }

    return .other;
}

/// Checks if wg show interfaces output contains a specific interface name.
pub fn classifyWgInterfacesOutput(stdout: []const u8, interface_name: []const u8) bool {
    // wg show interfaces outputs one interface name per line
    var it = std.mem.splitScalar(u8, stdout, '\n');
    while (it.next()) |line| {
        // Trim whitespace
        const trimmed = std.mem.trim(u8, line, " \t\r");
        if (std.mem.eql(u8, trimmed, interface_name)) {
            return true;
        }
    }
    return false;
}

/// Classifies an OS link based on ip link output.
/// Returns wireguard if the link type is wireguard, non_wireguard otherwise.
pub fn classifyIpLinkDetail(stdout: []const u8) OsLinkKind {
    // Look for "wireguard" in the output which indicates wireguard type
    if (std.mem.containsAtLeast(u8, stdout, 1, "wireguard")) {
        return .wireguard;
    }
    return .non_wireguard;
}

// ============================================================================
// Status Mapping
// ============================================================================

/// Maps WgDiagnosticClass to status CheckStatus.
pub fn toCheckStatus(diag_class: WgDiagnosticClass) status.CheckStatus {
    return switch (diag_class) {
        .peers_healthy => .ok,
        else => .warn,
    };
}

/// Re-exports from status.zig for this module's use.
pub const status = struct {
    pub const CheckStatus = enum { ok, warn, @"error", unknown };
};
