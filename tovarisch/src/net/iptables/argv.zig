// iptables/argv.zig — Argv rendering for iptables boundary
//
// Extracted argv rendering logic for iptables.zig.

const std = @import("std");
const types = @import("types.zig");
const validate = @import("validate.zig");

/// Default path to the iptables command.
const DEFAULT_IPTABLES_PATH = "/sbin/iptables";

/// Allowed path to the iptables command.
/// Returns the path from environment variable or default.
pub fn getIptablesPath() [*:0]const u8 {
    if (std.c.getenv("TOVARISCH_IPTABLES_COMMAND_PATH")) |env_path| {
        return env_path;
    }
    return DEFAULT_IPTABLES_PATH;
}

/// Renders a MasqueradeRuleSpec into argv for iptables -C (check) command.
pub fn renderCheckArgv(rule: types.MasqueradeRuleSpec, backend: types.IptablesBackendConfig) [11][]const u8 {
    return renderArgvInternal(rule, backend, "-C");
}

/// Renders a MasqueradeRuleSpec into argv for iptables -A (append) command.
pub fn renderAppendArgv(rule: types.MasqueradeRuleSpec, backend: types.IptablesBackendConfig) [11][]const u8 {
    return renderArgvInternal(rule, backend, "-A");
}

/// Internal argv renderer - constructs the full argv array.
fn renderArgvInternal(rule: types.MasqueradeRuleSpec, backend: types.IptablesBackendConfig, comptime action: []const u8) [11][]const u8 {
    const exe_path = if (backend.executable_path.len > 0)
        backend.executable_path
    else
        std.mem.sliceTo(getIptablesPath(), 0);

    return [11][]const u8{
        exe_path,
        "-t",
        @tagName(rule.table),
        action,
        @tagName(rule.chain),
        "-s",
        rule.source_cidr,
        "-o",
        rule.out_interface,
        "-j",
        @tagName(rule.jump),
    };
}

// ============================================================================
// Tests
// ============================================================================

test "renderCheckArgv produces deterministic argv" {
    const rule = types.MasqueradeRuleSpec.defaultMasquerade("10.0.0.0/8", "eth0");
    const backend = types.defaultBackendConfig();
    const argv = renderCheckArgv(rule, backend);

    try std.testing.expectEqual(@as(usize, 11), argv.len);
    try std.testing.expectEqualStrings("nat", argv[2]);
    try std.testing.expectEqualStrings("-C", argv[3]);
    try std.testing.expectEqualStrings("POSTROUTING", argv[4]);
    try std.testing.expectEqualStrings("-j", argv[9]);
    try std.testing.expectEqualStrings("MASQUERADE", argv[10]);
}

test "renderAppendArgv produces deterministic argv" {
    const rule = types.MasqueradeRuleSpec.defaultMasquerade("10.0.0.0/8", "eth0");
    const backend = types.defaultBackendConfig();
    const argv = renderAppendArgv(rule, backend);

    try std.testing.expectEqual(@as(usize, 11), argv.len);
    try std.testing.expectEqualStrings("-A", argv[3]);
}

test "renderCheckArgv uses custom executable_path as argv[0]" {
    var custom_backend = types.defaultBackendConfig();
    custom_backend.executable_path = "/usr/bin/iptables-custom";

    const rule = types.MasqueradeRuleSpec.defaultMasquerade("10.0.0.0/8", "eth0");
    const argv = renderCheckArgv(rule, custom_backend);

    try std.testing.expectEqualStrings("/usr/bin/iptables-custom", argv[0]);
}

test "renderAppendArgv uses custom executable_path as argv[0]" {
    var custom_backend = types.defaultBackendConfig();
    custom_backend.executable_path = "/opt/firewall/iptables";

    const rule = types.MasqueradeRuleSpec.defaultMasquerade("10.0.0.0/8", "eth0");
    const argv = renderAppendArgv(rule, custom_backend);

    try std.testing.expectEqualStrings("/opt/firewall/iptables", argv[0]);
}
