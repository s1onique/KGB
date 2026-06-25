// iptables/validate.zig — Validation functions for iptables boundary
//
// Extracted validation logic for iptables.zig to keep the main module under LLM-friendly size limits.

const std = @import("std");
const types = @import("types.zig");

/// Validates a network interface name conservatively.
/// Returns true if the name is valid for use as a public interface.
pub fn isValidInterfaceName(name: []const u8) bool {
    if (name.len == 0 or name.len > 15) return false;

    for (name) |c| {
        if (c >= 'A' and c <= 'Z') continue;
        if (c >= 'a' and c <= 'z') continue;
        if (c >= '0' and c <= '9') continue;
        if (c == '_' or c == '.' or c == '-') continue;
        return false;
    }

    return true;
}

/// Validates a CIDR string conservatively.
pub fn isValidCidrFormat(cidr: []const u8) bool {
    if (cidr.len < 7 or cidr.len > 18) return false;

    var slash_found = false;
    var prefix_start: usize = 0;

    for (cidr, 0..) |c, i| {
        if (c == '/') {
            if (slash_found) return false;
            slash_found = true;
            prefix_start = i + 1;
            continue;
        }
        if (!slash_found) {
            if (c != '.' and (c < '0' or c > '9')) return false;
        } else {
            if (c < '0' or c > '9') return false;
        }
    }

    if (!slash_found or prefix_start == 0) return false;
    if (prefix_start >= cidr.len) return false;

    const prefix_str = cidr[prefix_start..];
    if (prefix_str.len == 0 or prefix_str.len > 2) return false;

    if (prefix_str.len == 2) {
        if (prefix_str[0] > '3') return false;
        if (prefix_str[0] == '3' and prefix_str[1] > '2') return false;
    }

    return true;
}

/// Validates a MasqueradeRuleSpec before rendering.
///
/// Enforces MASQUERADE invariants:
/// - family must be ipv4 (ip6tables support does not exist)
/// - table must be nat (MASQUERADE only works in nat table)
/// - chain must be POSTROUTING (MASQUERADE only works in POSTROUTING)
/// - jump must be MASQUERADE (only MASQUERADE target for this use case)
///
/// Returns null if valid, or an error message if invalid.
pub fn validateRuleSpec(rule: types.MasqueradeRuleSpec) ?[]const u8 {
    // Enforce MASQUERADE invariants
    if (rule.family != .ipv4) {
        return "ipv6 not supported: ip6tables boundary not implemented";
    }
    if (rule.table != .nat) {
        return "MASQUERADE requires table=nat";
    }
    if (rule.chain != .POSTROUTING) {
        return "MASQUERADE requires chain=POSTROUTING";
    }
    if (rule.jump != .MASQUERADE) {
        return "only MASQUERADE jump target supported";
    }

    // Validate CIDR and interface name
    if (!isValidCidrFormat(rule.source_cidr)) {
        return "invalid CIDR format";
    }
    if (!isValidInterfaceName(rule.out_interface)) {
        return "invalid interface name";
    }
    return null;
}
