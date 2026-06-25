// iptables/types.zig — Typed definitions for iptables boundary
//
// Extracted types for iptables.zig to keep the main module under LLM-friendly size limits.

const std = @import("std");

// ============================================================================
// Constants
// ============================================================================

/// Default path to the iptables command.
pub const DEFAULT_IPTABLES_PATH = "/sbin/iptables";

// ============================================================================
// Typed Rule Specification
// ============================================================================

/// Protocol family for iptables rules.
pub const IpFamily = enum {
    ipv4,
    ipv6,
};

/// Standard iptables table names.
pub const TableName = enum {
    nat,
    filter,
    mangle,
    raw,
};

/// Standard iptables chain names.
pub const ChainName = enum {
    PREROUTING,
    POSTROUTING,
    INPUT,
    FORWARD,
    OUTPUT,
};

/// Standard iptables targets/jumps.
pub const JumpTarget = enum {
    MASQUERADE,
    ACCEPT,
    DROP,
    REJECT,
    DNAT,
    SNAT,
};

/// A typed MASQUERADE rule specification for VPN NAT.
pub const MasqueradeRuleSpec = struct {
    /// Protocol family (IPv4/IPv6).
    family: IpFamily = .ipv4,
    /// iptables table (nat, filter, mangle, raw).
    table: TableName = .nat,
    /// iptables chain (PREROUTING, POSTROUTING, etc.).
    chain: ChainName = .POSTROUTING,
    /// Jump target (e.g., MASQUERADE).
    jump: JumpTarget = .MASQUERADE,
    /// Source CIDR for the rule (e.g., "10.0.0.0/8").
    source_cidr: []const u8,
    /// Output interface name for the rule (e.g., "eth0").
    out_interface: []const u8,

    pub fn defaultMasquerade(source_cidr: []const u8, out_interface: []const u8) MasqueradeRuleSpec {
        return MasqueradeRuleSpec{
            .source_cidr = source_cidr,
            .out_interface = out_interface,
        };
    }
};

// ============================================================================
// Backend Configuration
// ============================================================================

/// Configuration for the iptables backend execution.
pub const IptablesBackendConfig = struct {
    /// Path to the iptables binary.
    executable_path: []const u8 = DEFAULT_IPTABLES_PATH,
    /// Wait seconds for the xtables lock.
    wait_seconds: u8 = 0,
    /// Timeout for command execution in seconds.
    timeout_seconds: u32 = 30,
};

/// Returns the default backend configuration.
pub fn defaultBackendConfig() IptablesBackendConfig {
    return .{};
}

// ============================================================================
// Errors
// ============================================================================

/// Errors that can occur when managing iptables rules.
pub const IptablesError = error{
    CommandNotFound,
    CommandFailed,
    PipeFailed,
    ForkFailed,
    ExecFailed,
    OutOfMemory,
    ValidationFailed,
};

// ============================================================================
// Command Runner Interface
// ============================================================================

/// Interface for running iptables commands.
pub const CommandRunner = struct {
    run: *const fn (argv: []const []const u8) IptablesError!c_int,
};

// ============================================================================
// Structured Result Types
// ============================================================================

/// Legacy result enum for backward compatibility.
pub const RuleExistsResult = enum {
    exists,
    missing,
    unknown,
};

/// Result of a rule check operation.
pub const CheckResult = enum {
    present,
    missing,
    backend_missing,
    permission_denied,
    backend_rejected,
    timed_out,
    unknown_failure,
};

/// Result of a rule apply/ensure operation.
pub const ApplyResult = enum {
    applied,
    already_present,
    backend_missing,
    permission_denied,
    backend_rejected,
    timed_out,
    invalid_rule,
    unknown_failure,
};

/// Status for the VPN masquerade check.
pub const MasqueradeStatus = enum {
    ok,
    warn,
    disabled,
};

/// Result of a masquerade status check.
pub const MasqueradeCheckResult = struct {
    status: MasqueradeStatus,
    detail: []const u8,
};
