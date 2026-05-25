// wg_show_collector.zig — Bounded WireGuard `wg show` command runner
//
// ACT: Implement bounded WireGuard command runner without status integration.
//
// Scope:
// - Add a collector module separate from the parser.
// - Invoke `wg show` through fixed argv only; no shell.
// - Add graceful fallback when `wg` is missing or exits non-zero.
// - Add bounded stdout capture.
// - Reject oversized stdout safely.
// - Do not expose endpoint, allowed IP, public key, private key, or preshared key material.
// - Do not integrate with `/status` yet.
//
// This module is intentionally separate from the parser to isolate process execution
// from data parsing. The parser remains testable without spawning processes.
//
// Ownership notes for future /status integration:
// - runWgShowCapture() returns allocated output via allocator.dupe()
// - The WgInterface.interface field is a slice pointing into this allocated buffer
// - For one-shot calls this is acceptable (small, bounded, short-lived)
// - For repeated status calls, consider returning an owned result struct that
//   includes both parsed data and owned output storage, or copy the interface
//   name into caller-owned static storage to avoid lifetime issues.

const std = @import("std");
const wg_show_parser = @import("wg_show_parser.zig");

// ============================================================================
// Constants
// ============================================================================

/// Maximum stdout buffer size for `wg show` output.
/// Typical WireGuard output is well under 1KB even with many peers.
/// 8KB provides generous headroom while preventing unbounded memory growth.
pub const MAX_OUTPUT_SIZE: usize = 8192;

/// Allowed paths to the WireGuard `wg` command.
/// Tried in order; first existing executable is used.
///
/// Note: For systems that require minimal env (PATH, LANG, etc.), this
/// may need hardening. For v0, empty env is acceptable.
const WG_PATHS = [_][*:0]const u8{
    "/usr/bin/wg",
    "/usr/sbin/wg",
    "/sbin/wg",
};

/// Empty environment for execve.
/// Note: Some systems may expect minimal env vars. This is fine for v0
/// where wg typically reads kernel interfaces via netlink, not env.
const EMPTY_ENV: [*:null]const ?[*:0]const u8 = &.{};

// ============================================================================
// Error Types
// ============================================================================

/// Errors that can occur when collecting WireGuard diagnostics.
pub const CollectError = error{
    /// The `wg` command is not available on this system.
    CommandNotFound,
    /// The `wg` command exited with a non-zero status.
    CommandFailed,
    /// Failed to create pipe for stdout capture.
    PipeFailed,
    /// Failed to fork process.
    ForkFailed,
    /// Failed to execute `wg` binary.
    ExecFailed,
    /// Stdout output exceeded MAX_OUTPUT_SIZE.
    OutputTruncated,
    /// Parser returned an error (malformed output).
    MalformedOutput,
    /// Memory allocation failed.
    OutOfMemory,
};

/// Result of collecting WireGuard diagnostics.
/// Contains the parsed interface data on success, or null on failure.
pub const WgDiagnosticsResult = ?wg_show_parser.WgInterface;

// ============================================================================
// Internal helpers
// ============================================================================

/// Result of running a command and capturing output.
const CommandResult = struct {
    exit_code: c_int,
    output: []u8,
    output_truncated: bool,
};

/// Check if a path exists and is executable using access().
fn isExecutable(path: [*:0]const u8) bool {
    return std.c.access(path, std.c.X_OK) == 0;
}

/// Find the first available wg command path.
fn findWgCommand() ?[*:0]const u8 {
    for (WG_PATHS) |path| {
        if (isExecutable(path)) {
            return path;
        }
    }
    return null;
}

/// Internal function to run wg show and capture output into a bounded buffer.
/// This function is testable without spawning actual processes.
fn runWgShowCapture(allocator: std.mem.Allocator) CollectError!CommandResult {
    // Find an available wg command
    const wg_path = findWgCommand() orelse return error.CommandNotFound;

    // Create pipe for stdout capture
    var pipe_fds: [2]c_int = undefined;
    if (std.c.pipe(&pipe_fds) != 0) {
        return error.PipeFailed;
    }

    const pid = std.c.fork();
    if (pid < 0) {
        _ = std.c.close(pipe_fds[0]);
        _ = std.c.close(pipe_fds[1]);
        return error.ForkFailed;
    }

    if (pid == 0) {
        // Child process
        _ = std.c.close(pipe_fds[0]); // Close read end

        // Redirect stdout to pipe write end
        _ = std.c.dup2(pipe_fds[1], 1);
        _ = std.c.close(pipe_fds[1]); // Close original write end after dup2

        // Close stderr to prevent noise from child process
        // Child exits immediately via execve or _exit, so this is safe
        // Note: Some programs behave oddly with closed stderr. For v0 this is
        // acceptable; future hardening could redirect to /dev/null instead.
        _ = std.c.close(2);

        // Execute wg show with fixed argv (no shell)
        const argv = [_][*:0]const u8{ wg_path, "show" };
        const argv_null: [*:null]const ?[*:0]const u8 = @ptrCast(&argv);
        _ = std.c.execve(wg_path, argv_null, EMPTY_ENV);

        // execve failed - exit with 127 for command not found semantics
        std.c._exit(127);
    }

    // Parent process - close write end immediately after fork
    _ = std.c.close(pipe_fds[1]);

    // Read from stdout pipe with bounded buffer
    var buffer: [MAX_OUTPUT_SIZE]u8 = undefined;
    var bytes_read: usize = 0;
    var output_truncated = false;

    while (true) {
        const remaining = MAX_OUTPUT_SIZE - bytes_read;
        const read_ptr: [*]u8 = @ptrCast(&buffer);
        const n = std.c.read(pipe_fds[0], read_ptr + bytes_read, remaining);
        if (n < 0) {
            _ = std.c.close(pipe_fds[0]);
            // Note: child may be briefly unreaped here on read error.
            // For v0 this is acceptable; future process helper could
            // use a separate signal handler or pipe close notification.
            return error.PipeFailed;
        }
        if (n == 0) break; // EOF

        bytes_read += @as(usize, @intCast(n));
        if (bytes_read >= MAX_OUTPUT_SIZE) {
            output_truncated = true;
            // Drain remaining data to prevent child from getting SIGPIPE
            var drain_buf: [1024]u8 = undefined;
            const drain_ptr: [*]u8 = @ptrCast(&drain_buf);
            while (true) {
                const drained = std.c.read(pipe_fds[0], drain_ptr, drain_buf.len);
                if (drained <= 0) break;
            }
            break;
        }
    }
    _ = std.c.close(pipe_fds[0]);

    // Wait for child to complete
    var status: c_int = undefined;
    _ = std.c.waitpid(pid, &status, 0);

    // Make a copy of the output for the caller
    const output = try allocator.dupe(u8, buffer[0..bytes_read]);

    return CommandResult{
        .exit_code = if ((status & 0x7f) == 0) (status >> 8) & 0xff else -1,
        .output = output,
        .output_truncated = output_truncated,
    };
}

/// Maps command result to collector errors and invokes parser.
fn mapAndParse(output: []const u8, exit_code: c_int, output_truncated: bool) CollectError!wg_show_parser.WgInterface {
    // Check for command not found (ENOENT - exit 127 from child)
    if (exit_code == 127) {
        return error.CommandNotFound;
    }

    // Check for non-zero exit (but not 127 which means command not found)
    if (exit_code != 0) {
        return error.CommandFailed;
    }

    // Check for truncated output - reject oversized output for v0
    if (output_truncated) {
        return error.OutputTruncated;
    }

    // Parse the output, mapping parser errors to MalformedOutput
    return wg_show_parser.parseWgShowOutput(output) catch |e| {
        switch (e) {
            error.NoInterface, error.InvalidNumber, error.MalformedOutput => return error.MalformedOutput,
        }
    };
}

// ============================================================================
// Collector
// ============================================================================

/// Collects WireGuard diagnostics by invoking `wg show` and parsing stdout.
///
/// This function:
/// - Invokes `wg show` with fixed argv (no shell)
/// - Captures stdout into a bounded buffer
/// - Passes captured output to the parser
/// - Returns parsed diagnostics or null on any error
///
/// Errors:
/// - `CommandNotFound` if `wg` is not available
/// - `CommandFailed` if `wg` exits non-zero
/// - `OutputTruncated` if stdout exceeds MAX_OUTPUT_SIZE
/// - `MalformedOutput` if parser fails
///
/// Note: For one-shot calls, the returned WgInterface.interface slice
/// points into an allocated buffer. This is acceptable for v0.
/// Before repeated /status calls, consider returning an owned result
/// struct or copying interface name into static storage.
pub fn collectWgDiagnostics(allocator: std.mem.Allocator) CollectError!wg_show_parser.WgInterface {
    const result = try runWgShowCapture(allocator);
    return mapAndParse(result.output, result.exit_code, result.output_truncated);
}

// ============================================================================
// Bounded Collector with Explicit Truncation
// ============================================================================

/// Result struct that includes truncation information.
pub const BoundedCollectResult = struct {
    /// The parsed interface diagnostics.
    diagnostics: wg_show_parser.WgInterface,
    /// True if the output was truncated before parsing.
    was_truncated: bool,
};

/// Collects WireGuard diagnostics with explicit truncation handling.
///
/// For v0, this function behaves the same as `collectWgDiagnostics`:
/// it rejects oversized output and returns `OutputTruncated` if the
/// output exceeds MAX_OUTPUT_SIZE.
///
/// The `was_truncated` field in the result indicates whether the
/// truncation was detected, even though parsing fails in that case.
/// This variant exists for future use cases that may need to parse
/// partial data while tracking truncation state.
pub fn collectWgDiagnosticsBounded(allocator: std.mem.Allocator) error{
    CommandNotFound,
    CommandFailed,
    PipeFailed,
    ForkFailed,
    ExecFailed,
    OutputTruncated,
    MalformedOutput,
    OutOfMemory,
}!BoundedCollectResult {
    const result = try runWgShowCapture(allocator);

    // Detect truncation state
    const was_truncated = result.output_truncated;

    // For v0: reject oversized output consistently
    if (was_truncated) {
        return error.OutputTruncated;
    }

    const parsed = try mapAndParse(result.output, result.exit_code, false);
    return BoundedCollectResult{
        .diagnostics = parsed,
        .was_truncated = was_truncated,
    };
}

// ============================================================================
// Test-only helper
// ============================================================================

/// Test helper: directly maps command output to parsed result.
/// This allows deterministic unit tests without spawning processes.
///
/// For production use, prefer `collectWgDiagnostics`.
///
/// Note: This tests the error mapping logic only. Real fork/exec behavior
/// requires integration testing with a real `wg` binary or a test fixture.
pub fn mapCommandOutputForTest(output: []const u8, exit_code: c_int) CollectError!wg_show_parser.WgInterface {
    // Simulate truncation check for test output > MAX_OUTPUT_SIZE
    const was_truncated = output.len > MAX_OUTPUT_SIZE;
    return mapAndParse(output, exit_code, was_truncated);
}
