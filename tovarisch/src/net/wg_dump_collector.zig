// wg_dump_collector.zig — Collector for WireGuard `wg show dump` command
//
// ACT: Add tovarisch WireGuard and XRay TCP underlay diagnostics
// Bounded command runner for WireGuard dump diagnostics.
//
// Reuses the safe_command runner with wg_show_dump command.
// Provides higher-level interface with configuration and redaction.

const std = @import("std");
const safe_command = @import("safe_command.zig");
const wg_dump_parser = @import("wg_dump_parser.zig");
const network_diag_config = @import("network_diag_config.zig");

// ============================================================================
// Types
// ============================================================================

/// WireGuard dump collector errors.
pub const CollectError = error{
    /// The `wg` command is not available on this system.
    CommandNotFound,
    /// The `wg` command exited with a non-zero status.
    CommandFailed,
    /// Failed to create pipe.
    PipeFailed,
    /// Failed to fork process.
    ForkFailed,
    /// Failed to execute `wg` binary.
    ExecFailed,
    /// Command timed out.
    Timeout,
    /// Output exceeded buffer size.
    OutputTruncated,
    /// Parser returned an error (malformed output).
    MalformedOutput,
    /// Memory allocation failed.
    OutOfMemory,
};

/// Owned WireGuard dump diagnostics result.
pub const WgDumpOwned = struct {
    /// Parsed dump result.
    result: wg_dump_parser.WgDumpResult,
    /// Owned stdout buffer (backing result data).
    stdout_buf: []u8,

    /// Release memory.
    pub fn deinit(self: *WgDumpOwned, allocator: std.mem.Allocator) void {
        allocator.free(self.stdout_buf);
        self.* = undefined;
    }
};

// ============================================================================
// Collection
// ============================================================================

/// Collect WireGuard dump diagnostics for an interface.
pub fn collectWgDumpOwned(
    allocator: std.mem.Allocator,
    iface: []const u8,
    cfg: network_diag_config.WireguardDiagConfig,
) CollectError!WgDumpOwned {
    // Build parse config from diagnostics config
    const parse_cfg = wg_dump_parser.ParseConfig{
        .redact = wg_dump_parser.RedactMode{
            .redact_public_keys = cfg.redact_peer_keys,
            .redact_endpoints = cfg.redact_endpoints,
        },
        .stale_handshake_seconds = cfg.stale_handshake_seconds,
    };

    // Run the command
    const cmd_result = safe_command.runWgShowDump(allocator, iface, .{}) catch |err| {
        return mapCommandError(err);
    };
    defer allocator.free(cmd_result.stdout);
    defer allocator.free(cmd_result.stderr);

    // Check exit code
    if (cmd_result.exit_code == 127) {
        return error.CommandNotFound;
    }
    if (cmd_result.exit_code != 0) {
        return error.CommandFailed;
    }

    // Check truncation
    if (cmd_result.stdout_truncated) {
        return error.OutputTruncated;
    }

    // Parse the output
    const result = wg_dump_parser.parseWgDumpOutput(allocator, cmd_result.stdout, parse_cfg) catch |err| {
        switch (err) {
            error.NoData, error.MalformedOutput, error.InvalidNumber, error.MissingPrivateKey => return error.MalformedOutput,
        }
    };

    // Transfer ownership of stdout_buf
    return WgDumpOwned{
        .result = result,
        .stdout_buf = cmd_result.stdout,
    };
}

/// Map safe_command errors to CollectError.
fn mapCommandError(err: safe_command.CommandError) CollectError {
    switch (err) {
        error.CommandNotAllowed => return error.CommandNotFound,
        error.PipeFailed => return error.PipeFailed,
        error.ForkFailed => return error.ForkFailed,
        error.ExecFailed => return error.ExecFailed,
        error.Timeout => return error.Timeout,
        error.OutOfMemory => return error.OutOfMemory,
    }
}

/// Test helper: directly map command output to parsed result.
pub fn mapCommandOutputForTest(
    allocator: std.mem.Allocator,
    output: []const u8,
    exit_code: c_int,
    config: network_diag_config.WireguardDiagConfig,
) CollectError!wg_dump_parser.WgDumpResult {
    const parse_cfg = wg_dump_parser.ParseConfig{
        .redact = wg_dump_parser.RedactMode{
            .redact_public_keys = config.redact_peer_keys,
            .redact_endpoints = config.redact_endpoints,
        },
        .stale_handshake_seconds = config.stale_handshake_seconds,
    };

    if (exit_code == 127) return error.CommandNotFound;
    if (exit_code != 0) return error.CommandFailed;

    return wg_dump_parser.parseWgDumpOutput(allocator, output, parse_cfg) catch |err| {
        switch (err) {
            error.NoData, error.MalformedOutput, error.InvalidNumber, error.MissingPrivateKey => return error.MalformedOutput,
        }
    };
}
