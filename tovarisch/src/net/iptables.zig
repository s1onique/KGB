// iptables.zig — Native-owned iptables boundary for VPN MASQUERADE
//
// ACT: Native-owned iptables rule application boundary for tovarisch VPN masquerade
//
// This module provides a TYPED, DETERMINISTIC boundary around the iptables backend.
// It is NOT a full native netfilter/nftables rewrite — the backend remains iptables
// executable, but all rule intent, argv rendering, validation, and result classification
// are now owned by this module.
//
// Design principles:
// - Rule intent is typed data, not raw string composition
// - Command argv is deterministically rendered and unit-tested
// - Backend outcomes are structured (not raw exit codes)
// - Invalid config values are rejected before backend execution
// - No shell invocation — argv passed directly to execve
// - Missing executable, permission errors, and backend rejections are classified
// - Unknown failures fail closed
//
// Deferred: Full native netfilter/nftables backend
//
// Safety guarantees:
// - Never flush chains
// - Never delete unrelated rules
// - Never alter default policies
// - Idempotent check-then-add pattern

const std = @import("std");
const build_options = @import("build_options");
const types = @import("iptables/types.zig");
const validate = @import("iptables/validate.zig");
const argv_mod = @import("iptables/argv.zig");

// Re-export types for external consumers
pub const IpFamily = types.IpFamily;
pub const TableName = types.TableName;
pub const ChainName = types.ChainName;
pub const JumpTarget = types.JumpTarget;
pub const MasqueradeRuleSpec = types.MasqueradeRuleSpec;
pub const IptablesBackendConfig = types.IptablesBackendConfig;
pub const defaultBackendConfig = types.defaultBackendConfig;
pub const IptablesError = types.IptablesError;
pub const CommandRunner = types.CommandRunner;
pub const RuleExistsResult = types.RuleExistsResult;
pub const CheckResult = types.CheckResult;
pub const ApplyResult = types.ApplyResult;
pub const MasqueradeStatus = types.MasqueradeStatus;
pub const MasqueradeCheckResult = types.MasqueradeCheckResult;

// Re-export validation functions
pub const isValidInterfaceName = validate.isValidInterfaceName;
pub const isValidCidrFormat = validate.isValidCidrFormat;
pub const validateRuleSpec = validate.validateRuleSpec;

// Re-export argv rendering
pub const renderCheckArgv = argv_mod.renderCheckArgv;
pub const renderAppendArgv = argv_mod.renderAppendArgv;

pub const DEFAULT_IPTABLES_PATH = types.DEFAULT_IPTABLES_PATH;

// ============================================================================
// Production Command Runner
// ============================================================================

/// Production command runner using raw fork/execve.
pub fn runIptablesReal(argv: []const []const u8) types.IptablesError!c_int {
    return runIptablesRealWithAllocator(std.heap.page_allocator, argv);
}

/// Production command runner with injectable allocator for testing.
///
/// The executable path is derived from argv[0] to honor the typed backend config.
/// Falls back to environment/default only if argv is empty.
pub fn runIptablesRealWithAllocator(
    allocator: std.mem.Allocator,
    argv: []const []const u8,
) types.IptablesError!c_int {
    // Derive executable path from argv[0] (honors backend.executable_path)
    // Fall back to env/default only if argv is empty
    const use_argv0 = argv.len > 0 and argv[0].len > 0;

    // Pre-allocate fallback executable path (for when argv[0] is empty)
    const fallback_executable = if (!use_argv0)
        try allocator.dupeZ(u8, std.mem.sliceTo(argv_mod.getIptablesPath(), 0))
    else
        null;
    defer {
        if (fallback_executable) |f| allocator.free(f);
    }

    var owned = try allocator.alloc(?[:0]u8, argv.len);
    @memset(owned, null);
    defer {
        for (owned) |z| {
            if (z) |slice| allocator.free(slice);
        }
        allocator.free(owned);
    }

    const c_args = try allocator.alloc(?[*:0]const u8, argv.len + 1);
    defer allocator.free(c_args);

    for (argv, 0..) |arg, i| {
        owned[i] = try allocator.dupeZ(u8, arg);
        c_args[i] = owned[i].?.ptr;
    }
    c_args[argv.len] = null;

    const pid = std.c.fork();
    if (pid < 0) {
        return error.ForkFailed;
    }

    if (pid == 0) {
        _ = std.c.close(2);
        // Use argv[0] pointer directly (already NUL-terminated in owned[0])
        // or use pre-allocated fallback if argv[0] was empty
        const executable_ptr = if (use_argv0) owned[0].?.ptr else fallback_executable.?.ptr;
        const argv_ptr: [*:null]const ?[*:0]const u8 = @ptrCast(c_args.ptr);
        _ = std.c.execve(executable_ptr, argv_ptr, &.{});
        std.c._exit(127);
    }

    var status: c_int = undefined;
    _ = std.c.waitpid(pid, &status, 0);

    if ((status & 0x7f) != 0) {
        return error.CommandFailed;
    }

    return (status >> 8) & 0xff;
}

/// The production command runner instance.
pub const realRunner: CommandRunner = .{
    .run = struct {
        fn run(argv: []const []const u8) types.IptablesError!c_int {
            return runIptablesReal(argv);
        }
    }.run,
};

// ============================================================================
// Exit Code Mapping
// ============================================================================

/// Maps exit code to CheckResult for iptables -C command.
fn mapCheckExitCode(exit_code: c_int) CheckResult {
    switch (exit_code) {
        0 => return .present,
        1 => return .missing,
        else => return .unknown_failure,
    }
}

/// Maps exit code to ApplyResult for iptables -A command.
fn mapApplyExitCode(exit_code: c_int) ApplyResult {
    switch (exit_code) {
        0 => return .applied,
        else => return .backend_rejected,
    }
}

// ============================================================================
// Rule Management Logic (Typed API)
// ============================================================================

/// Checks if the rule exists (observation only, no mutation).
pub fn checkRule(
    runner: CommandRunner,
    rule: MasqueradeRuleSpec,
    backend: IptablesBackendConfig,
) types.IptablesError!CheckResult {
    if (validateRuleSpec(rule)) |_| {
        return error.ValidationFailed;
    }

    const argv = renderCheckArgv(rule, backend);
    const exit_code = try runner.run(&argv);

    return mapCheckExitCode(exit_code);
}

/// Legacy wrapper for checkRuleExists.
pub fn checkRuleExists(
    runner: CommandRunner,
    vpn_cidr: []const u8,
    public_interface: []const u8,
) types.IptablesError!RuleExistsResult {
    const rule = MasqueradeRuleSpec.defaultMasquerade(vpn_cidr, public_interface);
    const backend = defaultBackendConfig();

    if (validateRuleSpec(rule)) |_| {
        return .unknown;
    }

    const argv = renderCheckArgv(rule, backend);
    const exit_code = try runner.run(&argv);

    if (exit_code == 0) return .exists;
    if (exit_code == 1) return .missing;
    return .unknown;
}

/// Ensures the rule exists. Adds it if missing (mutation).
pub fn ensureRuleTyped(
    runner: CommandRunner,
    rule: MasqueradeRuleSpec,
    backend: IptablesBackendConfig,
) types.IptablesError!ApplyResult {
    if (validateRuleSpec(rule)) |_| {
        return .invalid_rule;
    }

    const check_result = checkRule(runner, rule, backend) catch |err| {
        switch (err) {
            error.CommandNotFound => return .backend_missing,
            error.ForkFailed => return .unknown_failure,
            error.ExecFailed => return .backend_rejected,
            error.OutOfMemory => return .unknown_failure,
            else => return .unknown_failure,
        }
    };

    switch (check_result) {
        .present => return .already_present,
        .missing => {
            const argv = renderAppendArgv(rule, backend);
            const exit_code = runner.run(&argv) catch |err| {
                switch (err) {
                    error.CommandNotFound => return .backend_missing,
                    error.ForkFailed => return .unknown_failure,
                    error.ExecFailed => return .backend_rejected,
                    error.OutOfMemory => return .unknown_failure,
                    else => return .unknown_failure,
                }
            };
            return mapApplyExitCode(exit_code);
        },
        .backend_missing => return .backend_missing,
        .permission_denied => return .permission_denied,
        .backend_rejected => return .backend_rejected,
        .timed_out => return .timed_out,
        .unknown_failure => return .unknown_failure,
    }
}

/// Legacy wrapper for ensureRule.
pub fn ensureRule(
    runner: CommandRunner,
    vpn_cidr: []const u8,
    public_interface: []const u8,
) types.IptablesError!bool {
    const rule = MasqueradeRuleSpec.defaultMasquerade(vpn_cidr, public_interface);
    const backend = defaultBackendConfig();

    const result = try ensureRuleTyped(runner, rule, backend);
    switch (result) {
        .already_present => return false,
        .applied => return true,
        .invalid_rule => return error.ValidationFailed,
        .backend_missing, .permission_denied, .backend_rejected, .timed_out, .unknown_failure => return error.CommandFailed,
    }
}

// ============================================================================
// Status Check Builder (Observation Only)
// ============================================================================

/// Builds a masquerade check result from observation result.
pub fn buildMasqueradeCheckFromResult(
    result: anyerror!RuleExistsResult,
) MasqueradeCheckResult {
    const exists_result = result catch |err| {
        const detail: []const u8 = switch (err) {
            error.CommandNotFound => "iptables not available",
            error.CommandFailed => "iptables check failed",
            error.ForkFailed => "iptables fork failed",
            error.ExecFailed => "iptables exec failed",
            error.OutOfMemory => "iptables check out of memory",
            error.ValidationFailed => "iptables validation failed",
            else => "iptables unknown error",
        };
        return MasqueradeCheckResult{
            .status = .warn,
            .detail = detail,
        };
    };

    switch (exists_result) {
        .exists => return MasqueradeCheckResult{
            .status = .ok,
            .detail = "MASQUERADE active",
        },
        .missing => return MasqueradeCheckResult{
            .status = .warn,
            .detail = "iptables rule missing",
        },
        .unknown => return MasqueradeCheckResult{
            .status = .warn,
            .detail = "iptables check returned unexpected exit code",
        },
    }
}

/// Builds a disabled masquerade check result.
pub fn buildDisabledCheck() MasqueradeCheckResult {
    return MasqueradeCheckResult{
        .status = .disabled,
        .detail = "disabled",
    };
}

// ============================================================================
// Tests
// ============================================================================

test "isValidInterfaceName accepts valid names" {
    try std.testing.expect(isValidInterfaceName("eth0"));
    try std.testing.expect(isValidInterfaceName("wg0"));
}

test "isValidCidrFormat accepts valid CIDR" {
    try std.testing.expect(isValidCidrFormat("10.0.0.0/8"));
}

test "validateRuleSpec accepts valid spec" {
    const rule = MasqueradeRuleSpec.defaultMasquerade("10.0.0.0/8", "eth0");
    try std.testing.expect(validateRuleSpec(rule) == null);
}

test "validateRuleSpec rejects ipv6 family" {
    var rule = MasqueradeRuleSpec.defaultMasquerade("10.0.0.0/8", "eth0");
    rule.family = .ipv6;
    try std.testing.expect(validateRuleSpec(rule) != null);
}

test "validateRuleSpec rejects non-nat table" {
    var rule = MasqueradeRuleSpec.defaultMasquerade("10.0.0.0/8", "eth0");
    rule.table = .filter;
    try std.testing.expect(validateRuleSpec(rule) != null);
}

test "mapCheckExitCode maps exit 0 to present" {
    try std.testing.expect(mapCheckExitCode(0) == .present);
}

test "checkRuleExists accepts valid parameters" {
    const cfg = struct {
        fn run(argv: []const []const u8) types.IptablesError!c_int {
            _ = argv;
            return 0;
        }
    }.run;
    const runner = CommandRunner{ .run = cfg };
    const result = checkRuleExists(runner, "10.0.0.0/8", "eth0");
    try std.testing.expect(try result == .exists);
}

test "ensureRule does not add when exists" {
    const cfg = struct {
        fn run(argv: []const []const u8) types.IptablesError!c_int {
            _ = argv;
            return 0;
        }
    }.run;
    const runner = CommandRunner{ .run = cfg };
    const result = try ensureRule(runner, "10.0.0.0/8", "eth0");
    try std.testing.expect(!result);
}

test "buildMasqueradeCheckFromResult handles exists" {
    const result = buildMasqueradeCheckFromResult(.exists);
    try std.testing.expect(result.status == .ok);
}

test "defaultBackendConfig returns sensible defaults" {
    const backend = defaultBackendConfig();
    try std.testing.expectEqualStrings(DEFAULT_IPTABLES_PATH, backend.executable_path);
}
