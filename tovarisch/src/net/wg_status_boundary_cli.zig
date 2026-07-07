// wg_status_boundary_cli.zig — CLI backend for WireGuard status boundary
//
// Part of wg_status_boundary.zig (split to satisfy LLM-friendliness limits).
// Contains only the CLI backend implementation.
//
// Phase 1 CLI backend uses configured interface identity via `wg show <iface> dump`.
// Phase 2 generic netlink remains future work.
//
// ACT-HULK29R-ZIG016-WG-STATUS-CLASSIFICATION-FIX:
// Extended CLI backend that collects structured facts to enable precise WireGuard
// diagnostic classification. Separates OS link presence from WireGuard interface
// visibility for accurate status reporting.
//
// ACT-HULK29R-ZIG016-WG-PEERS-DIAGNOSTIC-INTEGRATION:
// This module provides diagnostic-aware status collection that carries structured
// diagnostic context (interface, backend, timeout_secs, exit code) on both success
// and failure paths without changing the public API surface.

const std = @import("std");
const wg = @import("wg_status_boundary.zig");
const config_parse_helpers = @import("../config_parse_helpers.zig");
const wg_cli_run = @import("wg_status_boundary_cli_run.zig");
const classifier = @import("wg_diagnostic_classifier.zig");
const wg_cli_facts = @import("wg_cli_facts.zig");

// Re-export from wg_status_boundary_cli_run.zig for backward compatibility
pub const OwnedWgCommandResult = wg_cli_run.OwnedWgCommandResult;
pub const runWgShowDump = wg_cli_run.runWgShowDump;

// WgCommandRunner: injectable seam for testing CLI status collection.
// ACT-HULK29R-ZIG016-MEMOWN02-COMMAND-RUNNER-SEAM
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
// Diagnostic Attempt Union (ACT-HULK29R-ZIG016-WG-PEERS-DIAGNOSTIC-INTEGRATION)
// ============================================================================

/// Union that carries both status and diagnostic on all paths.
/// This allows callers to get structured diagnostic context even on error paths,
/// unlike an error union which would lose diagnostic context on `return error.timeout`.
///
/// Usage:
///   const attempt = cliWireguardStatusDiagnosticAttemptWithRunner(...);
///   switch (attempt) {
///       .ok => |ok| use ok.status and ok.diagnostic,
///       .err => |bad| use bad.err and bad.diagnostic,
///   }
pub const WireGuardStatusDiagnosticAttempt = union(enum) {
    ok: struct {
        status: wg.WireGuardStatus,
        diagnostic: wg.WireGuardPeerDiagnostic,
    },
    err: struct {
        err: wg.StatusError,
        diagnostic: wg.WireGuardPeerDiagnostic,
    },
};

// POSIX fcntl and open flags (not exposed in Zig 0.16 std.c)
const F_GETFL: c_int = 3;
const F_SETFL: c_int = 4;

// Platform-specific O_NONBLOCK:
// - Linux: octal 04000 = decimal 2048
// - macOS/BSD: 0x0004 = decimal 4
const O_NONBLOCK: c_int = if (@import("builtin").os.tag == .linux) 2048 else 4;

// POSIX poll event flags (not exposed in Zig 0.16 std.c)
const POLLIN: c_short = 0x0001;
const POLLHUP: c_short = 0x0010;
const POLLERR: c_short = 0x0008;


// ============================================================================
// Interface Name Configuration
// ============================================================================

/// Default WireGuard interface name when not explicitly configured.
/// Documented single source of truth for this default value.
/// This is a compile-time constant; Zig inner functions cannot capture
/// instance fields, so interface name must be fixed at compile time.
///
/// Phase 1: Only DEFAULT_WG_INTERFACE is supported.
/// Runtime configurable interface name is future work.
pub const DEFAULT_WG_INTERFACE: [:0]const u8 = "wg-kgb0";

/// Validates an interface name for safety before passing to wg command.
///
/// Rejects:
///   - empty strings
///   - whitespace characters
///   - forward slashes (path traversal attempt)
///   - shell metacharacters
///   - names exceeding Linux interface name limits (IFNAMSIZ-1 = 15 bytes)
pub fn isValidInterfaceName(name: []const u8) bool {
    return config_parse_helpers.isValidInterfaceName(name);
}


// ============================================================================
// CLI Backend Implementation
// ============================================================================

/// CLI backend using `wg show <interface> dump` command.
/// Uses machine-readable tab-separated dump format to avoid human parsing issues.
///
/// Safety properties:
///   - Fixed argv only, no shell interpolation
///   - Bounded stdout/stderr capture (8KB stdout, 1KB stderr)
///   - Bounded timeout with SIGKILL enforcement
///   - Explicit command allowlist (only /usr/bin/wg, /usr/sbin/wg, /sbin/wg)
///   - Validated interface name before execve
///
/// Note: Interface name is a compile-time constant (DEFAULT_WG_INTERFACE).
/// Zig inner functions cannot capture instance fields, so we use a fixed
/// interface name rather than per-instance configuration.
pub const CliBackend = struct {
    /// Default timeout for wg show command (5 seconds).
    pub const DEFAULT_TIMEOUT_SECS: u64 = 5;

    /// Maximum stdout buffer size (8KB) - sufficient for dump output.
    pub const MAX_STDOUT_SIZE: usize = 8192;

    /// Maximum stderr buffer size (1KB).
    pub const MAX_STDERR_SIZE: usize = 1024;

    /// Allowed paths to the WireGuard `wg` command.
    const WG_PATHS = [_][*:0]const u8{
        "/usr/bin/wg",
        "/usr/sbin/wg",
        "/sbin/wg",
    };

    /// Override path for testing (null = use default WG_PATHS).
    /// This allows tests to inject a fake wg command without modifying PATH.
    test_only_wg_path: ?[*:0]const u8 = null,

    /// Initialize CLI backend with defaults (uses DEFAULT_WG_INTERFACE).
    pub fn init() CliBackend {
        return CliBackend{};
    }

    /// Initialize CLI backend with a test-only wg path override.
    /// Only for use in tests - allows injecting a fake wg command.
    pub fn initWithTestWgPath(wg_path: [*:0]const u8) CliBackend {
        return CliBackend{ .test_only_wg_path = wg_path };
    }

    /// Convert to generic backend trait.
    /// Uses DEFAULT_WG_INTERFACE for the wg show command.
    ///
    /// Phase 1: Uses compile-time constant interface name.
    /// Note: Zig inner functions cannot capture outer scope variables,
    /// so interface_name must be a compile-time constant.
    pub fn asBackend(self: *const CliBackend) wg.WireGuardStatusBackend {
        _ = self;
        return wg.WireGuardStatusBackend{
            .wireguardStatusFn = struct {
                fn f(allocator: std.mem.Allocator, _: ?*anyopaque) wg.StatusError!wg.WireGuardStatusResult {
                    // For production, use default path lookup
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

    /// Test-only: WireGuard status with explicit wg command path.
    /// This exercises the real CLI path (fork/execve) but with a fake command.
    /// Returns the result directly (not wrapped in backend trait) for testing.
    pub fn wireguardStatusWithPath(self: *CliBackend, allocator: std.mem.Allocator) wg.StatusError!wg.WireGuardStatusResult {
        return cliWireguardStatus(allocator, self.test_only_wg_path);
    }
};

// ============================================================================
// Production CLI Status Collection
// ============================================================================

/// Production wireguardStatus implementation using real fork/execve path.
fn cliWireguardStatus(allocator: std.mem.Allocator, test_path_override: ?[*:0]const u8) wg.StatusError!wg.WireGuardStatusResult {
    return cliWireguardStatusWithRunner(allocator, test_path_override, real_wg_command_runner);
}

// ============================================================================
// Diagnostic Builder (ACT-HULK29R-ZIG016-WG-PEERS-DIAGNOSTIC-INTEGRATION)
// ============================================================================

// WireGuard command kinds for diagnostic reporting.
const WgCommandKind = enum {
    /// wg show <interface> dump
    show_dump,
    /// wg show interfaces
    show_interfaces,
    /// ip -d link show <interface>
    ip_link_show,
};

/// Returns a stable command label without the interface name.
fn wgCommandLabel(kind: WgCommandKind) []const u8 {
    return switch (kind) {
        .show_dump => "wg show <interface> dump",
        .show_interfaces => "wg show interfaces",
        .ip_link_show => "ip -d link show <interface>",
    };
}

/// Builds a value-only WireGuardPeerDiagnostic from command result data.
/// All fields are value types - no borrowed slices escape the command result.
fn buildCliDiagnostic(
    error_kind: []const u8,
    exit_code: ?u8,
    timed_out: bool,
    stdout_len: usize,
    stderr_len: usize,
) wg.WireGuardPeerDiagnostic {
    return .{
        .backend = "cli",
        .selected_interface = DEFAULT_WG_INTERFACE,
        .command = wgCommandLabel(.show_dump),
        .timeout_secs = if (timed_out) CliBackend.DEFAULT_TIMEOUT_SECS else null,
        .exit_code = exit_code,
        .error_kind = error_kind,
        .stderr_len = stderr_len,
        .stdout_len = stdout_len,
    };
}

/// Builds a WireGuardPeerDiagnostic from classified facts.
/// Extended to support precise classification (ACT-HULK29R-ZIG016-WG-STATUS-CLASSIFICATION-FIX).
fn buildCliDiagnosticFromFacts(
    facts: classifier.WgInterfaceFacts,
    exit_code: ?u8,
    timed_out: bool,
    stdout_len: usize,
    stderr_len: usize,
) wg.WireGuardPeerDiagnostic {
    // Classify the status
    const diag_class = classifier.classifyWgStatus(facts);

    // Map classifier class to error_kind string
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
        .command = wgCommandLabel(.show_dump),
        .timeout_secs = if (timed_out) CliBackend.DEFAULT_TIMEOUT_SECS else null,
        .exit_code = exit_code,
        .error_kind = error_kind,
        .stderr_len = stderr_len,
        .stdout_len = stdout_len,
        .os_link_kind = facts.os_link_kind,
        .peer_count = facts.wg_dump_peer_count,
    };
}

// ============================================================================
// Diagnostic-Aware Status Collection (ACT-HULK29R-ZIG016-WG-PEERS-DIAGNOSTIC-INTEGRATION)
// ============================================================================

/// Diagnostic-aware WireGuard status collection with injectable command runner.
///
/// This internal function returns a union that carries both status and diagnostic
/// on all paths, allowing callers to get structured diagnostic context even on
/// error paths without changing the public API.
///
/// Declassifies the diagnostic attempt into the legacy error union for callers
/// that only need the status (preserving API compatibility).
pub fn cliWireguardStatusDiagnosticAttemptWithRunner(
    allocator: std.mem.Allocator,
    test_path_override: ?[*:0]const u8,
    runner: WgCommandRunner,
) WireGuardStatusDiagnosticAttempt {
    // Validate interface name before execve (defense in depth)
    if (!isValidInterfaceName(DEFAULT_WG_INTERFACE)) {
        return .{ .err = .{
            .err = error.interface_missing,
            .diagnostic = buildCliDiagnostic("interface_missing", null, false, 0, 0),
        } };
    }

    const wg_path = test_path_override orelse (findWgCommand() orelse return .{ .err = .{
        .err = error.backend_missing,
        .diagnostic = buildCliDiagnostic("backend_missing", null, false, 0, 0),
    } });

    var cmd_result = runner.run(allocator, wg_path, DEFAULT_WG_INTERFACE, CliBackend.DEFAULT_TIMEOUT_SECS) catch |run_err| {
        // runner.run() failure - classify based on error type
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
            null,
            false,
            0,
            0,
        );
        return .{ .err = .{ .err = classified, .diagnostic = diag } };
    };

    // MemoryOwnership: Use defer for owned result cleanup on all return paths.
    // cmd_result is an OwnedWgCommandResult with explicit deinit() contract.
    defer cmd_result.deinit(allocator);

    const stdout_len = cmd_result.stdout.len;
    const stderr_len = cmd_result.stderr.len;

    // Classify command result
    if (cmd_result.exit_code == 127) {
        return .{ .err = .{
            .err = error.backend_missing,
            .diagnostic = buildCliDiagnostic("backend_missing", 127, false, stdout_len, stderr_len),
        } };
    }

    if (cmd_result.exit_code == 126) {
        return .{ .err = .{
            .err = error.permission_denied,
            .diagnostic = buildCliDiagnostic("permission_denied", 126, false, stdout_len, stderr_len),
        } };
    }

    if (cmd_result.timed_out) {
        return .{ .err = .{
            .err = error.timeout,
            .diagnostic = buildCliDiagnostic("timeout", null, true, stdout_len, stderr_len),
        } };
    }

    if (cmd_result.exit_code != 0) {
        // wg show <iface> returns exit 1 if interface doesn't exist
        const kind: []const u8 = if (cmd_result.exit_code == 1) "interface_missing" else "command_failed";
        return .{ .err = .{
            .err = if (cmd_result.exit_code == 1) error.interface_missing else error.command_failed,
            .diagnostic = buildCliDiagnostic(kind, @intCast(cmd_result.exit_code), false, stdout_len, stderr_len),
        } };
    }

    if (cmd_result.stdout_truncated or cmd_result.stderr_truncated) {
        return .{ .err = .{
            .err = error.command_failed,
            .diagnostic = buildCliDiagnostic("command_failed", 0, false, stdout_len, stderr_len),
        } };
    }

    // Parse output with explicit interface name (not invented from output)
    const status = wg.parseWgDumpOutput(cmd_result.stdout, DEFAULT_WG_INTERFACE) catch |parse_err| {
        _ = parse_err;
        return .{ .err = .{
            .err = error.malformed_output,
            .diagnostic = buildCliDiagnostic("malformed_output", 0, false, stdout_len, stderr_len),
        } };
    };

    // Success path - return status with ok diagnostic
    return .{ .ok = .{
        .status = status,
        .diagnostic = buildCliDiagnostic("ok", 0, false, stdout_len, stderr_len),
    } };
}

// ============================================================================
// Testing Export (ACT-HULK29R-ZIG016-MEMOWN02-COMMAND-RUNNER-SEAM)
// ============================================================================

/// Test-only: runner-aware status collection with injectable command runner.
/// Allows tests to inject allocated fake stdout/stderr without fork/execve.
///
/// Preserves the legacy public API for backward compatibility while delegating
/// to the diagnostic-aware internal implementation.
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

/// Maps legacy collector errors to structured StatusError.
fn mapCollectorError(err: anyerror) wg.StatusError {
    return switch (err) {
        error.PipeFailed => error.command_failed,
        error.ForkFailed => error.command_failed,
        error.OutOfMemory => error.out_of_memory,
        else => error.command_failed,
    };
}

/// Internal: find first available wg command path.
fn findWgCommand() ?[*:0]const u8 {
    for (CliBackend.WG_PATHS) |path| {
        if (std.c.access(path, std.c.X_OK) == 0) {
            return path;
        }
    }
    return null;
}

