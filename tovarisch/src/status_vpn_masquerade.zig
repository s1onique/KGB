// status_vpn_masquerade.zig — VPN masquerade status check for tovarisch
//
// ACT: Add config-controlled VPN masquerade rule with rule watcher.
//
// This module provides the vpn_masquerade status check for the status JSON payload.
// It bridges the iptables rule management with the status check system.
//
// IMPORTANT: Status rendering observes only, never mutates firewall state.
// The ensureRule() function is the repair/watcher path, not the status rendering path.
//
// Status values:
// - ok: MASQUERADE rule is active
// - warn: enabled but rule missing or repair failed
// - ok (disabled): masquerade is not enabled (not degraded)

const std = @import("std");
const status = @import("status.zig");
const iptables = @import("net/iptables.zig");
const config = @import("config.zig");

// ============================================================================
// Status Check Builder (Observation Only)
// ============================================================================

/// Builds a vpn_masquerade status check from configuration.
/// 
/// This is the status rendering path - it only OBSERVES, never mutates.
/// Uses checkRuleExists() which is read-only.
/// For repair/watcher behavior, use ensureRule() directly.
pub fn buildMasqueradeCheck(
    cfg: config.VpnMasqueradeConfig,
    runner: iptables.CommandRunner,
) status.Check {
    // If disabled, return ok with disabled detail (not degraded)
    if (!cfg.enabled) {
        return status.Check{
            .name = "vpn_masquerade",
            .status = .ok, // Disabled is not degraded
            .detail = "disabled",
        };
    }

    // Enabled: observe if rule exists (no mutation)
    const result = iptables.checkRuleExists(
        runner,
        cfg.vpn_cidr,
        cfg.public_interface,
    );

    const check_result = iptables.buildMasqueradeCheckFromResult(result);

    // Map MasqueradeStatus to CheckStatus
    const check_status: status.CheckStatus = switch (check_result.status) {
        .ok => .ok,
        .warn => .warn,
        .disabled => .ok, // Should not happen when enabled, but not degraded if it does
    };

    return status.Check{
        .name = "vpn_masquerade",
        .status = check_status,
        .detail = check_result.detail,
    };
}

/// Builds a vpn_masquerade status check using the real command runner.
/// This is the production entry point.
pub fn buildMasqueradeCheckReal(cfg: config.VpnMasqueradeConfig) status.Check {
    return buildMasqueradeCheck(cfg, iptables.realRunner);
}

// ============================================================================
// Watcher/Repair Path (Mutation Allowed)
// ============================================================================

/// Watcher tick function: checks rule and repairs if missing.
/// This is the repair/watcher path - it MUTATES firewall state.
/// Call this periodically from a watcher goroutine/loop.
pub fn watchMasqueradeRuleTick(
    runner: iptables.CommandRunner,
    vpn_cidr: []const u8,
    public_interface: []const u8,
) iptables.IptablesError!void {
    _ = try iptables.ensureRule(runner, vpn_cidr, public_interface);
}

/// Initial setup: ensure rule exists on startup.
/// Call this once on daemon startup if masquerade is enabled.
pub fn ensureMasqueradeRuleOnce(
    runner: iptables.CommandRunner,
    vpn_cidr: []const u8,
    public_interface: []const u8,
) iptables.IptablesError!bool {
    return iptables.ensureRule(runner, vpn_cidr, public_interface);
}

// ============================================================================
// Test Helpers
// ============================================================================

/// Creates a simple fake runner that always succeeds with exit code 0 (rule exists).
pub fn createFakeRunnerExists() iptables.CommandRunner {
    return iptables.CommandRunner{
        .run = struct {
            fn run(argv: []const []const u8) iptables.IptablesError!c_int {
                _ = argv;
                return 0;
            }
        }.run,
    };
}

/// Creates a fake runner that returns exit code 1 (rule missing) for check,
/// but returns 0 (success) for add commands.
/// This simulates the case where check shows rule missing, add succeeds.
pub fn createFakeRunnerMissing() iptables.CommandRunner {
    return iptables.CommandRunner{
        .run = struct {
            fn run(argv: []const []const u8) iptables.IptablesError!c_int {
                // If argv contains "-A" (add), return success
                // If argv contains "-C" (check), return 1 (missing)
                for (argv) |arg| {
                    if (std.mem.eql(u8, arg, "-A")) return 0;
                }
                return 1; // Check shows missing
            }
        }.run,
    };
}

/// Creates a fake runner that returns an error.
pub fn createFakeRunnerError(comptime err: iptables.IptablesError) iptables.CommandRunner {
    return iptables.CommandRunner{
        .run = struct {
            fn run(argv: []const []const u8) iptables.IptablesError!c_int {
                _ = argv;
                return err;
            }
        }.run,
    };
}

// ============================================================================
// Tests
// ============================================================================

test "buildMasqueradeCheck returns ok for disabled config" {
    const cfg = config.VpnMasqueradeConfig{
        .enabled = false,
        .vpn_cidr = "",
        .public_interface = "",
    };
    const runner = createFakeRunnerExists();
    const check = buildMasqueradeCheck(cfg, runner);
    try std.testing.expectEqualStrings("vpn_masquerade", check.name);
    try std.testing.expect(check.status == .ok); // Disabled is not degraded
    try std.testing.expectEqualStrings("disabled", check.detail);
}

test "buildMasqueradeCheck returns ok when rule exists" {
    const cfg = config.VpnMasqueradeConfig{
        .enabled = true,
        .vpn_cidr = "10.0.0.0/8",
        .public_interface = "eth0",
    };
    const runner = createFakeRunnerExists();
    const check = buildMasqueradeCheck(cfg, runner);
    try std.testing.expectEqualStrings("vpn_masquerade", check.name);
    try std.testing.expect(check.status == .ok);
    try std.testing.expectEqualStrings("MASQUERADE active", check.detail);
}

test "buildMasqueradeCheck returns warn when rule is missing" {
    const cfg = config.VpnMasqueradeConfig{
        .enabled = true,
        .vpn_cidr = "10.0.0.0/8",
        .public_interface = "eth0",
    };
    const runner = createFakeRunnerMissing();
    const check = buildMasqueradeCheck(cfg, runner);
    try std.testing.expectEqualStrings("vpn_masquerade", check.name);
    try std.testing.expect(check.status == .warn);
    try std.testing.expectEqualStrings("iptables rule missing", check.detail);
}

test "buildMasqueradeCheck returns warn when iptables not available" {
    const cfg = config.VpnMasqueradeConfig{
        .enabled = true,
        .vpn_cidr = "10.0.0.0/8",
        .public_interface = "eth0",
    };
    const runner = createFakeRunnerError(error.CommandNotFound);
    const check = buildMasqueradeCheck(cfg, runner);
    try std.testing.expectEqualStrings("vpn_masquerade", check.name);
    try std.testing.expect(check.status == .warn);
    try std.testing.expectEqualStrings("iptables not available", check.detail);
}

test "createFakeRunnerExists produces valid runner" {
    const runner = createFakeRunnerExists();
    const result = try runner.run(&.{});
    try std.testing.expect(result == 0);
}

test "createFakeRunnerMissing produces valid runner" {
    const runner = createFakeRunnerMissing();
    const result = try runner.run(&.{});
    try std.testing.expect(result == 1);
}

test "createFakeRunnerError produces valid runner" {
    const runner = createFakeRunnerError(error.CommandNotFound);
    const result = runner.run(&.{});
    try std.testing.expect(result == error.CommandNotFound);
}

// ============================================================================
// Watcher/Repair Tests
// ============================================================================

test "ensureRule: existing rule returns false (no add needed)" {
    const runner = createFakeRunnerExists();
    const was_added = try iptables.ensureRule(runner, "10.0.0.0/8", "eth0");
    try std.testing.expect(!was_added);
}

test "ensureRule: missing rule returns true (add succeeded)" {
    const runner = createFakeRunnerMissing();
    const was_added = try iptables.ensureRule(runner, "10.0.0.0/8", "eth0");
    try std.testing.expect(was_added);
}

test "ensureRule: add failure returns CommandFailed" {
    const runner = createFakeRunnerError(error.CommandFailed);
    const result = iptables.ensureRule(runner, "10.0.0.0/8", "eth0");
    try std.testing.expectError(iptables.IptablesError.CommandFailed, result);
}

test "buildMasqueradeCheck: missing rule returns warn but does not add" {
    const cfg = config.VpnMasqueradeConfig{
        .enabled = true,
        .vpn_cidr = "10.0.0.0/8",
        .public_interface = "eth0",
    };
    const runner = createFakeRunnerMissing();
    const check = buildMasqueradeCheck(cfg, runner);
    try std.testing.expect(check.status == .warn);
    try std.testing.expectEqualStrings("iptables rule missing", check.detail);
}

test "watchMasqueradeRuleTick: missing rule repairs without error" {
    const runner = createFakeRunnerMissing();
    // Should not error - watcher handles repair
    try watchMasqueradeRuleTick(runner, "10.0.0.0/8", "eth0");
}

test "ensureMasqueradeRuleOnce: missing rule returns true" {
    const runner = createFakeRunnerMissing();
    const was_added = try ensureMasqueradeRuleOnce(runner, "10.0.0.0/8", "eth0");
    try std.testing.expect(was_added);
}

test "repeated existing rule does not add duplicates" {
    const runner = createFakeRunnerExists();
    
    // First call
    const was_added1 = try iptables.ensureRule(runner, "10.0.0.0/8", "eth0");
    try std.testing.expect(!was_added1);
    
    // Second call - should still not add
    const was_added2 = try iptables.ensureRule(runner, "10.0.0.0/8", "eth0");
    try std.testing.expect(!was_added2);
}
