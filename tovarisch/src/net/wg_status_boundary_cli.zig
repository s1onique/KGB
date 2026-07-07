// wg_status_boundary_cli.zig — CLI backend for WireGuard status boundary
//
// Part of wg_status_boundary.zig (split to satisfy LLM-friendliness limits).
// Contains only the CLI backend implementation.
//
// ACT-HULK29R-ZIG016-WG-STATUS-CLASSIFICATION-FIX

const std = @import("std");
const wg = @import("wg_status_boundary.zig");
const wg_cli_run = @import("wg_status_boundary_cli_run.zig");
const classifier = @import("wg_diagnostic_classifier.zig");
const cli_types = @import("wg_status_boundary_cli_types.zig");

// Re-export from wg_status_boundary_cli_run.zig for backward compatibility
pub const OwnedWgCommandResult = wg_cli_run.OwnedWgCommandResult;
pub const runWgShowDump = wg_cli_run.runWgShowDump;

// Re-export from types file for backward compatibility
pub const WireGuardStatusDiagnosticAttempt = cli_types.WireGuardStatusDiagnosticAttempt;
pub const DEFAULT_WG_INTERFACE = cli_types.DEFAULT_WG_INTERFACE;
pub const isValidInterfaceName = cli_types.isValidInterfaceName;
pub const DEFAULT_TIMEOUT_SECS = cli_types.DEFAULT_TIMEOUT_SECS;
pub const WG_PATHS = cli_types.WG_PATHS;
pub const WgCommandKind = cli_types.WgCommandKind;
pub const wgCommandLabel = cli_types.wgCommandLabel;

// WgCommandRunner: injectable seam for testing CLI status collection.
pub const WgCommandRunner = struct {
    runFn: *const fn (
        allocator: std.mem.Allocator,
        ctx: ?*anyopaque,
        wg_path: [*:0]const u8,
        interface_name: []const u8,
        timeout_secs: u64,
    ) anyerror!OwnedWgCommandResult,
    ctx: ?*anyopaque = null,

    pub fn run(
        self: WgCommandRunner,
        allocator: std.mem.Allocator,
        wg_path: [*:0]const u8,
        interface_name: []const u8,
        timeout_secs: u64,
    ) !OwnedWgCommandResult {
        return self.runFn(allocator, self.ctx, wg_path, interface_name, timeout_secs);
    }
};

const real_wg_command_runner = WgCommandRunner{
    .runFn = struct {
        fn f(allocator: std.mem.Allocator, _: ?*anyopaque, wg_path: [*:0]const u8, interface_name: []const u8, timeout_secs: u64) !OwnedWgCommandResult {
            return runWgShowDump(allocator, wg_path, interface_name, timeout_secs);
        }
    }.f,
};

// ============================================================================
// CLI Backend Implementation
// ============================================================================

pub const CliBackend = struct {
    /// Default timeout for wg show command (5 seconds).
    pub const DEFAULT_TIMEOUT_SECS: u64 = cli_types.DEFAULT_TIMEOUT_SECS;

    test_only_wg_path: ?[*:0]const u8 = null,

    pub fn init() CliBackend {
        return CliBackend{};
    }

    pub fn initWithTestWgPath(wg_path: [*:0]const u8) CliBackend {
        return CliBackend{ .test_only_wg_path = wg_path };
    }

    pub fn asBackend(self: *const CliBackend) wg.WireGuardStatusBackend {
        _ = self;
        return wg.WireGuardStatusBackend{
            .wireguardStatusFn = struct {
                fn f(allocator: std.mem.Allocator, _: ?*anyopaque) wg.StatusError!wg.WireGuardStatusResult {
                    return cliWireguardStatus(allocator, null);
                }
            }.f,
            .backendKindFn = struct {
                fn f(_: ?*anyopaque) wg.BackendKind {
                    return .cli;
                }
            }.f,
        };
    }

    pub fn wireguardStatusWithPath(self: *CliBackend, allocator: std.mem.Allocator) wg.StatusError!wg.WireGuardStatusResult {
        return cliWireguardStatus(allocator, self.test_only_wg_path);
    }
};

// ============================================================================
// Diagnostic Builder
// ============================================================================

fn buildCliDiagnostic(
    error_kind: []const u8,
    exit_code: ?u8,
    timed_out: bool,
    stdout_len: usize,
    stderr_len: usize,
    stderr: []const u8,
) wg.WireGuardPeerDiagnostic {
    const stderr_class = classifier.classifyWgStderr(stderr);
    return .{
        .backend = "cli",
        .selected_interface = cli_types.DEFAULT_WG_INTERFACE,
        .command = cli_types.wgCommandLabel(.show_dump),
        .timeout_secs = if (timed_out) cli_types.DEFAULT_TIMEOUT_SECS else null,
        .exit_code = exit_code,
        .error_kind = error_kind,
        .stderr_len = stderr_len,
        .stdout_len = stdout_len,
        .wg_show_stderr_class = stderr_class,
    };
}

fn buildCliDiagnosticFromFacts(
    facts: classifier.WgInterfaceFacts,
    exit_code: ?u8,
    timed_out: bool,
    stdout_len: usize,
    stderr_len: usize,
) wg.WireGuardPeerDiagnostic {
    const diag_class = classifier.classifyWgStatus(facts);
    const error_kind: []const u8 = switch (diag_class) {
        .wg_tool_missing => "wg_tool_missing",
        .wireguard_interface_missing => "wireguard_interface_missing",
        .interface_present_non_wireguard => "interface_present_non_wireguard",
        .permission_denied => "permission_denied",
        .wrong_namespace_or_unreachable => "wrong_namespace_or_unreachable",
        .command_failed => "command_failed",
        .malformed_output => "malformed_output",
        .no_peers => "no_peers",
        .no_handshake => "no_handshake",
        .peers_healthy => "peers_healthy",
    };

    return .{
        .backend = "cli",
        .selected_interface = facts.configured_name,
        .command = cli_types.wgCommandLabel(.show_dump),
        .timeout_secs = if (timed_out) cli_types.DEFAULT_TIMEOUT_SECS else null,
        .exit_code = exit_code,
        .error_kind = error_kind,
        .stderr_len = stderr_len,
        .stdout_len = stdout_len,
        .os_link_kind = facts.os_link_kind,
        .peer_count = facts.wg_dump_peer_count,
    };
}

// ============================================================================
// Production CLI Status Collection
// ============================================================================

fn cliWireguardStatus(allocator: std.mem.Allocator, test_path_override: ?[*:0]const u8) wg.StatusError!wg.WireGuardStatusResult {
    return cliWireguardStatusWithRunner(allocator, test_path_override, real_wg_command_runner);
}

// ============================================================================
// Diagnostic-Aware Status Collection
// ============================================================================

pub fn cliWireguardStatusDiagnosticAttemptWithRunner(
    allocator: std.mem.Allocator,
    test_path_override: ?[*:0]const u8,
    runner: WgCommandRunner,
) WireGuardStatusDiagnosticAttempt {
    if (!cli_types.isValidInterfaceName(cli_types.DEFAULT_WG_INTERFACE)) {
        return .{ .err = .{
            .err = error.interface_missing,
            .diagnostic = buildCliDiagnostic("interface_missing", null, false, 0, 0, ""),
        } };
    }

    const wg_path = test_path_override orelse (findWgCommand() orelse return .{ .err = .{
        .err = error.backend_missing,
        .diagnostic = buildCliDiagnostic("backend_missing", null, false, 0, 0, ""),
    } });

    var cmd_result = runner.run(allocator, wg_path, cli_types.DEFAULT_WG_INTERFACE, cli_types.DEFAULT_TIMEOUT_SECS) catch |run_err| {
        const classified = mapCollectorError(run_err);
        const diag = buildCliDiagnostic(
            switch (classified) {
                error.timeout => "timeout",
                error.backend_missing => "backend_missing",
                error.permission_denied => "permission_denied",
                error.interface_missing => "interface_missing",
                error.malformed_output => "malformed_output",
                error.out_of_memory => "out_of_memory",
                error.unsupported_platform => "unsupported_platform",
                error.netlink_failed => "netlink_failed",
                error.command_failed => "command_failed",
            },
            null, false, 0, 0, "",
        );
        return .{ .err = .{ .err = classified, .diagnostic = diag } };
    };

    defer cmd_result.deinit(allocator);

    const stdout_len = cmd_result.stdout.len;
    const stderr_len = cmd_result.stderr.len;

    if (cmd_result.exit_code == 127) {
        return .{ .err = .{
            .err = error.backend_missing,
            .diagnostic = buildCliDiagnostic("backend_missing", 127, false, stdout_len, stderr_len, cmd_result.stderr),
        } };
    }

    if (cmd_result.exit_code == 126) {
        return .{ .err = .{
            .err = error.permission_denied,
            .diagnostic = buildCliDiagnostic("permission_denied", 126, false, stdout_len, stderr_len, cmd_result.stderr),
        } };
    }

    if (cmd_result.timed_out) {
        return .{ .err = .{
            .err = error.timeout,
            .diagnostic = buildCliDiagnostic("timeout", null, true, stdout_len, stderr_len, cmd_result.stderr),
        } };
    }

    if (cmd_result.exit_code != 0) {
        const kind: []const u8 = if (cmd_result.exit_code == 1) "interface_missing" else "command_failed";
        return .{ .err = .{
            .err = if (cmd_result.exit_code == 1) error.interface_missing else error.command_failed,
            .diagnostic = buildCliDiagnostic(kind, @intCast(cmd_result.exit_code), false, stdout_len, stderr_len, cmd_result.stderr),
        } };
    }

    if (cmd_result.stdout_truncated or cmd_result.stderr_truncated) {
        return .{ .err = .{
            .err = error.command_failed,
            .diagnostic = buildCliDiagnostic("command_failed", 0, false, stdout_len, stderr_len, cmd_result.stderr),
        } };
    }

    const status = wg.parseWgDumpOutput(cmd_result.stdout, cli_types.DEFAULT_WG_INTERFACE) catch |parse_err| {
        _ = parse_err;
        return .{ .err = .{
            .err = error.malformed_output,
            .diagnostic = buildCliDiagnostic("malformed_output", 0, false, stdout_len, stderr_len, cmd_result.stderr),
        } };
    };

    return .{ .ok = .{
        .status = status,
        .diagnostic = buildCliDiagnostic("ok", 0, false, stdout_len, stderr_len, cmd_result.stderr),
    } };
}

// ============================================================================
// Testing Export
// ============================================================================

pub fn cliWireguardStatusWithRunner(
    allocator: std.mem.Allocator,
    test_path_override: ?[*:0]const u8,
    runner: WgCommandRunner,
) wg.StatusError!wg.WireGuardStatusResult {
    const attempt = cliWireguardStatusDiagnosticAttemptWithRunner(allocator, test_path_override, runner);
    return switch (attempt) {
        .ok => |ok| wg.WireGuardStatusResult.ok(ok.status, .cli),
        .err => |bad| bad.err,
    };
}

fn mapCollectorError(err: anyerror) wg.StatusError {
    return switch (err) {
        error.PipeFailed => error.command_failed,
        error.ForkFailed => error.command_failed,
        error.OutOfMemory => error.out_of_memory,
        else => error.command_failed,
    };
}

fn findWgCommand() ?[*:0]const u8 {
    for (cli_types.WG_PATHS) |path| {
        if (std.c.access(path, std.c.X_OK) == 0) {
            return path;
        }
    }
    return null;
}
