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
// Ownership Model:
//
// This module provides owned result types for safe memory management:
//
// **WgDiagnosticsOwned** (recommended for production):
// - Returns an owned struct that bundles parsed diagnostics with the allocated stdout buffer
// - The `.deinit(allocator)` method releases all memory
// - Safe for repeated `/status` calls without lifetime issues or memory leaks
//
// **mapCommandOutputForTest** (for deterministic unit tests):
// - Test helper that directly maps command output to parsed result
// - No process spawning required
//
// Error handling ensures no memory leaks:
// - `collectWgDiagnosticsOwned()` uses `errdefer` to free stdout buffer on all error paths
// - On success, ownership transfers to the returned WgDiagnosticsOwned
// - Call `deinit()` when done to release memory
//
// Usage pattern for owned result:
//   var result = try collectWgDiagnosticsOwned(allocator);
//   defer result.deinit(allocator);
//   // Use result.diagnostics - safe until deinit

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

/// Environment variable name for forcing a specific wg command path.
/// Used for contract verification to force deterministic unavailable-tooling path.
pub const WG_COMMAND_PATH_ENV = "TOVARISCH_WG_COMMAND_PATH";

/// Find the first available wg command path.
///
/// Checks TOVARISCH_WG_COMMAND_PATH env var first if set.
/// Falls back to WG_PATHS list if env var is not set or empty.
///
/// Note: Env var takes precedence over auto-detection for:
/// - Forcing command-not-found path during contract verification
/// - Testing without wg installed
/// - Explicit path override for unusual system layouts
fn findWgCommand() ?[*:0]const u8 {
    // Check environment variable first (takes precedence)
    if (getenv(WG_COMMAND_PATH_ENV)) |env_path| {
        // Get length of C string before the null terminator.
        const len = std.mem.len(env_path);
        if (len > 0 and isExecutable(env_path)) {
            return env_path;
        }
        // Env var set but path not executable or empty - return null
        // This allows forcing CommandNotFound via /nonexistent
        return null;
    }

    // Fall back to auto-detection
    for (WG_PATHS) |path| {
        if (isExecutable(path)) {
            return path;
        }
    }
    return null;
}

/// Get environment variable value as a null-terminated C string.
/// Returns null if the environment variable is not set.
fn getenv(name: [*:0]const u8) ?[*:0]const u8 {
    return std.c.getenv(name);
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
// Collector — use WgDiagnosticsOwned for production
// ============================================================================
//
// Prefer collectWgDiagnosticsOwned() for all production use. The non-owned
// collectWgDiagnostics() API is removed because it returns WgInterface slices
// pointing into a hidden allocation that callers cannot free.
//
// collectWgDiagnosticsBounded() was removed for the same reason — use
// collectWgDiagnosticsOwned() instead.
//
// Use mapCommandOutputForTest() for deterministic unit tests.

// ============================================================================
// Owned Collector Result (safe for repeated /status calls)
// ============================================================================

/// Owned WireGuard diagnostics result for safe repeated status calls.
///
/// This struct bundles:
/// - `diagnostics`: parsed WireGuard interface data
/// - `stdout_buf`: the allocated stdout buffer that `diagnostics.interface` references
///
/// The `.interface` field is a slice into `stdout_buf`, which is owned by this struct.
/// Call `.deinit(allocator)` to release all memory when done.
///
/// **Usage pattern:**
/// ```zig
/// var result = try collectWgDiagnosticsOwned(allocator);
/// defer result.deinit(allocator);
/// // Use result.diagnostics safely until deinit
/// ```
///
/// **Ownership guarantees:**
/// - All memory is owned by this struct and released via deinit()
/// - No slice aliases exist after deinit
/// - Safe for repeated /status calls without lifetime issues
pub const WgDiagnosticsOwned = struct {
    /// The parsed interface diagnostics. The `.interface` field is a slice
    /// into `stdout_buf`.
    diagnostics: wg_show_parser.WgInterface,
    /// The allocated stdout buffer. The `diagnostics.interface` field is a
    /// slice into this buffer. Freed by `deinit()`.
    stdout_buf: []u8,

    /// Releases all memory owned by this result.
    ///
    /// After calling this method, do not use the diagnostics or any of its
    /// string slices (including `diagnostics.interface`).
    ///
    /// Safe to call multiple times (idempotent after first call).
    pub fn deinit(self: *WgDiagnosticsOwned, allocator: std.mem.Allocator) void {
        allocator.free(self.stdout_buf);
        self.stdout_buf = &[_]u8{};
        self.diagnostics.interface = &[_]u8{};
    }
};

/// Collects WireGuard diagnostics and returns an owned result.
///
/// This function is safe for repeated `/status` calls because:
/// - The result struct owns the stdout buffer that `diagnostics.interface` references
/// - The `.deinit(allocator)` method releases all memory
/// - No dangling pointers from freed output buffers
///
/// **Usage:**
/// ```zig
/// var result = try collectWgDiagnosticsOwned(allocator);
/// defer result.deinit(allocator);
/// // result.diagnostics.interface is valid until deinit
/// ```
///
/// Errors:
/// - `CommandNotFound` if `wg` is not available
/// - `CommandFailed` if `wg` exits non-zero
/// - `OutputTruncated` if stdout exceeds MAX_OUTPUT_SIZE
/// - `MalformedOutput` if parser fails
/// - `OutOfMemory` if memory allocation fails
///
/// **Ownership model:**
/// - `errdefer` handles cleanup on all error paths
/// - On success, ownership transfers to the returned WgDiagnosticsOwned
/// - Call `deinit()` when done to release memory
pub fn collectWgDiagnosticsOwned(allocator: std.mem.Allocator) CollectError!WgDiagnosticsOwned {
    const cmd_result = try runWgShowCapture(allocator);
    errdefer allocator.free(cmd_result.output);

    // Check for command not found (ENOENT - exit 127 from child)
    if (cmd_result.exit_code == 127) return error.CommandNotFound;

    // Check for non-zero exit (but not 127 which means command not found)
    if (cmd_result.exit_code != 0) return error.CommandFailed;

    // Check for truncated output - reject oversized output for v0
    if (cmd_result.output_truncated) return error.OutputTruncated;

    // Parse the output, mapping parser errors to MalformedOutput
    const parsed = wg_show_parser.parseWgShowOutput(cmd_result.output) catch |e| {
        switch (e) {
            error.NoInterface, error.InvalidNumber, error.MalformedOutput => return error.MalformedOutput,
        }
    };

    // Success path: transfer ownership of stdout_buf to WgDiagnosticsOwned
    // errdefer will NOT run on this return path
    return WgDiagnosticsOwned{
        .diagnostics = parsed,
        .stdout_buf = cmd_result.output,
    };
}

// ============================================================================
// Test-only helper
// ============================================================================

/// Test helper: directly maps command output to parsed result.
/// This allows deterministic unit tests without spawning processes.
///
/// For production use, prefer `collectWgDiagnosticsOwned()`.
///
/// Note: This tests the error mapping logic only. Real fork/exec behavior
/// requires integration testing with a real `wg` binary or a test fixture.
pub fn mapCommandOutputForTest(output: []const u8, exit_code: c_int) CollectError!wg_show_parser.WgInterface {
    // Simulate truncation check for test output > MAX_OUTPUT_SIZE
    const was_truncated = output.len > MAX_OUTPUT_SIZE;
    return mapAndParse(output, exit_code, was_truncated);
}
