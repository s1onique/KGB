// wg_status_boundary_cli_tests.zig — CLI diagnostic integration tests
//
// Part of wg_status_boundary_test.zig (split for LLM-friendliness).
// Contains FakeWgCommandRunner and diagnostic attempt tests.
//
// ACT-HULK29R-ZIG016-WG-PEERS-DIAGNOSTIC-INTEGRATION

const std = @import("std");
const wg = @import("wg_status_boundary.zig");
const wg_cli = @import("wg_status_boundary_cli.zig");

// ============================================================================
// FakeWgCommandRunner — Test double for WgCommandRunner
// ============================================================================

/// Configuration for FakeWgCommandRunner.
const FakeWgCommandRunnerConfig = struct {
    /// Fixture stdout content (will be allocated fresh per call).
    stdout: []const u8,
    /// Fixture stderr content (will be allocated fresh per call).
    stderr: []const u8,
    /// Exit code for the fake command.
    exit_code: c_int,
    /// Whether command timed out.
    timed_out: bool = false,
    /// Whether stdout was truncated.
    stdout_truncated: bool = false,
    /// Whether stderr was truncated.
    stderr_truncated: bool = false,
};

/// Fake command runner that returns allocated stdout/stderr for unit testing.
/// Each call allocates fresh buffers using the provided allocator.
///
/// ACT-HULK29R-ZIG016-MEMOWN02-COMMAND-RUNNER-SEAM
pub const FakeWgCommandRunner = struct {
    /// Configuration for this fake runner.
    config: FakeWgCommandRunnerConfig,

    /// Initialize fake runner with configuration.
    pub fn init(config: FakeWgCommandRunnerConfig) FakeWgCommandRunner {
        return FakeWgCommandRunner{ .config = config };
    }

    /// Create a WgCommandRunner that uses this fake.
    pub fn asRunner(self: *FakeWgCommandRunner) wg_cli.WgCommandRunner {
        return wg_cli.WgCommandRunner{
            .runFn = FakeWgCommandRunner.run,
            .ctx = self,
        };
    }

    /// Run function: allocates stdout/stderr using the provided allocator
    /// and returns an OwnedWgCommandResult.
    fn run(
        allocator: std.mem.Allocator,
        ctx: ?*anyopaque,
        _: [*:0]const u8,
        _: []const u8,
        _: u64,
    ) anyerror!wg_cli.OwnedWgCommandResult {
        const self: *FakeWgCommandRunner = @ptrCast(@alignCast(ctx));

        // Allocate fresh stdout buffer per call
        const stdout_storage = try allocator.dupe(u8, self.config.stdout);
        errdefer allocator.free(stdout_storage);

        // Allocate fresh stderr buffer per call
        const stderr_storage = try allocator.dupe(u8, self.config.stderr);
        errdefer allocator.free(stderr_storage);

        return wg_cli.OwnedWgCommandResult{
            .stdout_storage = stdout_storage,
            .stderr_storage = stderr_storage,
            .stdout = stdout_storage,
            .stderr = stderr_storage,
            .exit_code = self.config.exit_code,
            .stdout_truncated = self.config.stdout_truncated,
            .stderr_truncated = self.config.stderr_truncated,
            .timed_out = self.config.timed_out,
        };
    }
};

// ============================================================================
// Valid WireGuard dump fixture for tests
// ============================================================================

/// Valid wg show dump output with one peer (for success path tests).
pub const valid_wg_dump_output = "private_key_base64\tpublic_key_base64\t51820\t0\n" ++
    "peer_pubkey_base64\tpsk_base64\t1.2.3.4:51820\t10.0.0.2/32\t1700000000\t1000\t2000\t25\n";

// ============================================================================
// Diagnostic Attempt Tests (ACT-HULK29R-ZIG016-WG-PEERS-DIAGNOSTIC-INTEGRATION)
// ============================================================================

test "Diagnostic attempt carries timeout details" {
    const allocator = std.testing.allocator;
    var fake_runner = FakeWgCommandRunner.init(.{
        .stdout = "",
        .stderr = "",
        .exit_code = -1,
        .timed_out = true,
    });
    const attempt = wg_cli.cliWireguardStatusDiagnosticAttemptWithRunner(
        allocator, "/fake/wg", fake_runner.asRunner(),
    );
    try std.testing.expect(switch (attempt) { .err => true, .ok => false });
    const bad = attempt.err;
    try std.testing.expectEqual(wg.StatusError.timeout, bad.err);
    try std.testing.expectEqualStrings("timeout", bad.diagnostic.error_kind);
    try std.testing.expectEqual(wg_cli.CliBackend.DEFAULT_TIMEOUT_SECS, bad.diagnostic.timeout_secs.?);
}

test "Diagnostic attempt carries interface-missing details" {
    const allocator = std.testing.allocator;
    var fake_runner = FakeWgCommandRunner.init(.{
        .stdout = "",
        .stderr = "Unable to access interface: No such device\n",
        .exit_code = 1,
    });
    const attempt = wg_cli.cliWireguardStatusDiagnosticAttemptWithRunner(
        allocator, "/fake/wg", fake_runner.asRunner(),
    );
    try std.testing.expect(switch (attempt) { .err => true, .ok => false });
    const bad = attempt.err;
    try std.testing.expectEqual(wg.StatusError.interface_missing, bad.err);
    try std.testing.expectEqualStrings("interface_missing", bad.diagnostic.error_kind);
    try std.testing.expectEqual(@as(u8, 1), bad.diagnostic.exit_code.?);
    try std.testing.expect(bad.diagnostic.stderr_len > 0);
}

test "Diagnostic attempt carries permission_denied details" {
    const allocator = std.testing.allocator;
    var fake_runner = FakeWgCommandRunner.init(.{
        .stdout = "",
        .stderr = "Permission denied\n",
        .exit_code = 126,
    });
    const attempt = wg_cli.cliWireguardStatusDiagnosticAttemptWithRunner(
        allocator, "/fake/wg", fake_runner.asRunner(),
    );
    try std.testing.expect(switch (attempt) { .err => true, .ok => false });
    const bad = attempt.err;
    try std.testing.expectEqual(wg.StatusError.permission_denied, bad.err);
    try std.testing.expectEqualStrings("permission_denied", bad.diagnostic.error_kind);
    try std.testing.expectEqual(@as(u8, 126), bad.diagnostic.exit_code.?);
}

test "Diagnostic attempt carries backend_missing details" {
    const allocator = std.testing.allocator;
    var fake_runner = FakeWgCommandRunner.init(.{
        .stdout = "",
        .stderr = "wg: not found\n",
        .exit_code = 127,
    });
    const attempt = wg_cli.cliWireguardStatusDiagnosticAttemptWithRunner(
        allocator, "/fake/wg", fake_runner.asRunner(),
    );
    try std.testing.expect(switch (attempt) { .err => true, .ok => false });
    const bad = attempt.err;
    try std.testing.expectEqual(wg.StatusError.backend_missing, bad.err);
    try std.testing.expectEqualStrings("backend_missing", bad.diagnostic.error_kind);
    try std.testing.expectEqual(@as(u8, 127), bad.diagnostic.exit_code.?);
}

test "Diagnostic attempt carries command_failed details" {
    const allocator = std.testing.allocator;
    var fake_runner = FakeWgCommandRunner.init(.{
        .stdout = "",
        .stderr = "some wg failure\n",
        .exit_code = 2,
    });
    const attempt = wg_cli.cliWireguardStatusDiagnosticAttemptWithRunner(
        allocator, "/fake/wg", fake_runner.asRunner(),
    );
    try std.testing.expect(switch (attempt) { .err => true, .ok => false });
    const bad = attempt.err;
    try std.testing.expectEqual(wg.StatusError.command_failed, bad.err);
    try std.testing.expectEqualStrings("command_failed", bad.diagnostic.error_kind);
    try std.testing.expectEqual(@as(u8, 2), bad.diagnostic.exit_code.?);
}

test "Diagnostic attempt success path returns normal status and ok diagnostic" {
    const allocator = std.testing.allocator;
    var fake_runner = FakeWgCommandRunner.init(.{
        .stdout = valid_wg_dump_output,
        .stderr = "",
        .exit_code = 0,
    });
    const attempt = wg_cli.cliWireguardStatusDiagnosticAttemptWithRunner(
        allocator, "/fake/wg", fake_runner.asRunner(),
    );
    try std.testing.expect(switch (attempt) { .err => false, .ok => true });
    const ok_result = attempt.ok;
    try std.testing.expectEqual(@as(u32, 1), ok_result.status.peer_count);
    try std.testing.expectEqualStrings("ok", ok_result.diagnostic.error_kind);
    try std.testing.expectEqual(@as(u8, 0), ok_result.diagnostic.exit_code.?);
}

test "Public compatibility wrapper still maps errors" {
    const allocator = std.testing.allocator;

    // exit 127 -> error.backend_missing
    var fake_runner127 = FakeWgCommandRunner.init(.{ .stdout = "", .stderr = "", .exit_code = 127 });
    try std.testing.expectEqual(
        error.backend_missing,
        wg_cli.cliWireguardStatusWithRunner(allocator, "/fake/wg", fake_runner127.asRunner()),
    );

    // exit 126 -> error.permission_denied
    var fake_runner126 = FakeWgCommandRunner.init(.{ .stdout = "", .stderr = "", .exit_code = 126 });
    try std.testing.expectEqual(
        error.permission_denied,
        wg_cli.cliWireguardStatusWithRunner(allocator, "/fake/wg", fake_runner126.asRunner()),
    );

    // exit 1 -> error.interface_missing
    var fake_runner1 = FakeWgCommandRunner.init(.{ .stdout = "", .stderr = "", .exit_code = 1 });
    try std.testing.expectEqual(
        error.interface_missing,
        wg_cli.cliWireguardStatusWithRunner(allocator, "/fake/wg", fake_runner1.asRunner()),
    );

    // success -> WireGuardStatusResult.ok
    var fake_runner_ok = FakeWgCommandRunner.init(.{ .stdout = valid_wg_dump_output, .stderr = "", .exit_code = 0 });
    const result = try wg_cli.cliWireguardStatusWithRunner(allocator, "/fake/wg", fake_runner_ok.asRunner());
    try std.testing.expectEqual(@as(u32, 1), result.status.peer_count);
}

test "Diagnostic detail formatting produces expected format" {
    const allocator = std.testing.allocator;
    var fake_runner = FakeWgCommandRunner.init(.{
        .stdout = "",
        .stderr = "",
        .exit_code = -1,
        .timed_out = true,
    });
    const attempt = wg_cli.cliWireguardStatusDiagnosticAttemptWithRunner(
        allocator, "/fake/wg", fake_runner.asRunner(),
    );

    var detail_buf: [wg.DIAGNOSTIC_DETAIL_BUF_SIZE]u8 = undefined;
    const diag = attempt.err.diagnostic;
    const detail = wg.formatPeerDiagnosticDetail(diag, &detail_buf);

    // Expected format: "wg timeout: interface=wg-kgb0 backend=cli timeout_secs=5"
    try std.testing.expect(std.mem.startsWith(u8, detail, "wg timeout:"));
    try std.testing.expect(std.mem.indexOf(u8, detail, "interface=wg-kgb0") != null);
    try std.testing.expect(std.mem.indexOf(u8, detail, "backend=cli") != null);
    try std.testing.expect(std.mem.indexOf(u8, detail, "timeout_secs=5") != null);
}
