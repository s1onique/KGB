// iptables.zig — iptables MASQUERADE rule management for VPN NAT
//
// ACT: Add config-controlled VPN masquerade rule with rule watcher.
//
// Scope:
// - Manage iptables MASQUERADE rule for VPN traffic egress.
// - Use argv-style process execution only; no shell interpolation.
// - Provide injectable command runner for testability.
// - Add periodic watcher that repairs missing rules.
// - Expose status via vpn_masquerade check.
//
// Safety guarantees:
// - Never flush chains.
// - Never delete unrelated rules.
// - Never alter default policies.
// - Idempotent check-then-add pattern.

const std = @import("std");
const build_options = @import("build_options");

// ============================================================================
// Constants
// ============================================================================

/// Allowed path to the iptables command.
/// Can be overridden via environment variable for testing.
fn getIptablesPath() [*:0]const u8 {
    if (std.c.getenv("TOVARISCH_IPTABLES_COMMAND_PATH")) |env_path| {
        return env_path;
    }
    return "/sbin/iptables";
}

// ============================================================================
// Error Types
// ============================================================================

/// Errors that can occur when managing iptables rules.
pub const IptablesError = error{
    /// The iptables command is not available on this system.
    CommandNotFound,
    /// The iptables command exited with a non-zero status.
    CommandFailed,
    /// Failed to create pipe for stdout capture.
    PipeFailed,
    /// Failed to fork process.
    ForkFailed,
    /// Failed to execute iptables binary.
    ExecFailed,
    /// Memory allocation failed.
    OutOfMemory,
};

/// Result of rule existence check.
pub const RuleExistsResult = enum {
    /// Rule exists in the iptables ruleset.
    exists,
    /// Rule does not exist.
    missing,
    /// Check command failed unexpectedly.
    unknown,
};

// ============================================================================
// Command Runner Interface (Injectable for Testing)
// ============================================================================

/// Interface for running iptables commands with logical argv (clean []const []const u8).
/// Implement this trait for production (real Child process) or testing (fake runner).
pub const CommandRunner = struct {
    /// Runs an iptables command with the given argv.
    /// argv is clean logical argv - the runner converts to C argv internally.
    /// Returns exit code on success (0 = success, non-zero = failure).
    /// Returns error on system call failure.
    run: *const fn (argv: []const []const u8) IptablesError!c_int,
};

/// Production command runner using raw fork/execve.
///
/// Converts logical argv ([]const []const u8) to C argv (null-terminated array
/// of C-string pointers) before forking. Each argument is duplicated with
/// NUL terminator using the provided allocator.
pub fn runIptablesReal(
    argv: []const []const u8,
) IptablesError!c_int {
    return runIptablesRealWithAllocator(std.heap.page_allocator, argv);
}

/// Production command runner with injectable allocator for testing.
pub fn runIptablesRealWithAllocator(
    allocator: std.mem.Allocator,
    argv: []const []const u8,
) IptablesError!c_int {
    // Find iptables binary
    const iptables_path = getIptablesPath();

    // Allocate parallel arrays: owned (for freeing) and c_args (pointers for execve)
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

    // Convert each argument to NUL-terminated C string
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
        // Child process - must not allocate or use try
        // Close stderr to suppress iptables noise
        _ = std.c.close(2);

        // Cast to the type execve expects
        const argv_ptr: [*:null]const ?[*:0]const u8 = @ptrCast(c_args.ptr);

        // execve expects: argv[0] = program path, argv[1..] = args, argv[last] = null
        _ = std.c.execve(iptables_path, argv_ptr, &.{});

        // execve failed - exit with 127 for command not found semantics
        std.c._exit(127);
    }

    // Parent process - wait for child
    var status: c_int = undefined;
    _ = std.c.waitpid(pid, &status, 0);

    // Check if child exited normally
    if ((status & 0x7f) != 0) {
        // Child was killed by signal
        return error.CommandFailed;
    }

    // Return the exit code
    return (status >> 8) & 0xff;
}

/// The production command runner instance.
pub const realRunner: CommandRunner = .{
    .run = struct {
        fn run(argv: []const []const u8) IptablesError!c_int {
            return runIptablesReal(argv);
        }
    }.run,
};

// ============================================================================
// Rule Management Logic
// ============================================================================

/// Checks if the MASQUERADE rule exists (observation only, no mutation).
/// Uses argv-style execution: iptables -t nat -C POSTROUTING -s <cidr> -o <interface> -j MASQUERADE
pub fn checkRuleExists(
    runner: CommandRunner,
    vpn_cidr: []const u8,
    public_interface: []const u8,
) IptablesError!RuleExistsResult {
    const iptables_path = std.mem.sliceTo(getIptablesPath(), 0);
    const argv = [_][]const u8{
        iptables_path,
        "-t", "nat",
        "-C", "POSTROUTING",
        "-s", vpn_cidr,
        "-o", public_interface,
        "-j", "MASQUERADE",
    };

    const exit_code = try runner.run(&argv);

    // iptables -C exits 0 if rule exists, 1 if it doesn't exist
    if (exit_code == 0) return .exists;
    if (exit_code == 1) return .missing;
    // Non-zero exit code (non-1) indicates an error
    return .unknown;
}

/// Ensures the MASQUERADE rule exists. Adds it if missing (mutation).
/// Uses argv-style execution: iptables -t nat -A POSTROUTING -s <cidr> -o <interface> -j MASQUERADE
/// This is the repair/watcher path, NOT the status rendering path.
pub fn ensureRule(
    runner: CommandRunner,
    vpn_cidr: []const u8,
    public_interface: []const u8,
) IptablesError!bool {
    const exists_result = try checkRuleExists(runner, vpn_cidr, public_interface);

    switch (exists_result) {
        .exists => return false, // Rule already exists, no action needed
        .missing => {
            // Rule is missing, add it
            const iptables_path = std.mem.sliceTo(getIptablesPath(), 0);
            const argv = [_][]const u8{
                iptables_path,
                "-t", "nat",
                "-A", "POSTROUTING",
                "-s", vpn_cidr,
                "-o", public_interface,
                "-j", "MASQUERADE",
            };

            const exit_code = try runner.run(&argv);
            if (exit_code != 0) return error.CommandFailed;
            return true; // Rule was added
        },
        .unknown => return error.CommandFailed, // Check failed, treat as error
    }
}

// ============================================================================
// Status Check Builder (Observation Only - No Mutation)
// ============================================================================

/// Status for the VPN masquerade check.
pub const MasqueradeStatus = enum {
    /// Masquerade is active and working.
    ok,
    /// Masquerade is enabled but rule is missing or repair failed.
    warn,
    /// Masquerade is disabled (not degraded).
    disabled,
};

/// Result of a masquerade status check.
pub const MasqueradeCheckResult = struct {
    status: MasqueradeStatus,
    detail: []const u8,
};

/// Builds a masquerade check result from observation result.
/// This function only observes, never mutates (no rule add).
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
// Interface Name Validation
// ============================================================================

/// Validates a network interface name conservatively.
/// Returns true if the name is valid for use as a public interface.
pub fn isValidInterfaceName(name: []const u8) bool {
    if (name.len == 0 or name.len > 15) return false;

    for (name) |c| {
        // Allow only conservative interface-name characters: [A-Za-z0-9_.-]
        if (c >= 'A' and c <= 'Z') continue;
        if (c >= 'a' and c <= 'z') continue;
        if (c >= '0' and c <= '9') continue;
        if (c == '_' or c == '.' or c == '-') continue;
        return false;
    }

    return true;
}

// ============================================================================
// Tests
// ============================================================================

test "checkRuleExists accepts valid parameters" {
    // This test verifies the function signature is correct
    // Real execution would require root/iptables
    const cfg = struct {
        fn run(argv: []const []const u8) IptablesError!c_int {
            _ = argv;
            return 0;
        }
    }.run;
    const runner = CommandRunner{ .run = cfg };
    const result = checkRuleExists(runner, "10.0.0.0/8", "eth0");
    try std.testing.expect(try result == .exists);
}

test "checkRuleExists maps exit 0 to exists" {
    const cfg = struct {
        fn run(argv: []const []const u8) IptablesError!c_int {
            _ = argv;
            return 0; // Rule exists
        }
    }.run;
    const runner = CommandRunner{ .run = cfg };
    const result = try checkRuleExists(runner, "10.0.0.0/8", "eth0");
    try std.testing.expect(result == .exists);
}

test "checkRuleExists maps exit 1 to missing" {
    const cfg = struct {
        fn run(argv: []const []const u8) IptablesError!c_int {
            _ = argv;
            return 1; // Rule missing
        }
    }.run;
    const runner = CommandRunner{ .run = cfg };
    const result = try checkRuleExists(runner, "10.0.0.0/8", "eth0");
    try std.testing.expect(result == .missing);
}

test "checkRuleExists maps other non-zero to unknown" {
    const cfg = struct {
        fn run(argv: []const []const u8) IptablesError!c_int {
            _ = argv;
            return 2; // Unexpected exit code
        }
    }.run;
    const runner = CommandRunner{ .run = cfg };
    const result = try checkRuleExists(runner, "10.0.0.0/8", "eth0");
    try std.testing.expect(result == .unknown);
}

test "ensureRule does not add when exists" {
    const cfg = struct {
        fn run(argv: []const []const u8) IptablesError!c_int {
            _ = argv;
            return 0; // Rule exists
        }
    }.run;
    const runner = CommandRunner{ .run = cfg };
    const result = try ensureRule(runner, "10.0.0.0/8", "eth0");
    try std.testing.expect(!result); // No addition
}

test "buildMasqueradeCheckFromResult handles disabled" {
    const result = buildDisabledCheck();
    try std.testing.expect(result.status == .disabled);
    try std.testing.expectEqualStrings("disabled", result.detail);
}

test "buildMasqueradeCheckFromResult handles exists" {
    const result = buildMasqueradeCheckFromResult(.exists);
    try std.testing.expect(result.status == .ok);
    try std.testing.expectEqualStrings("MASQUERADE active", result.detail);
}

test "buildMasqueradeCheckFromResult handles missing" {
    const result = buildMasqueradeCheckFromResult(.missing);
    try std.testing.expect(result.status == .warn);
    try std.testing.expectEqualStrings("iptables rule missing", result.detail);
}

test "buildMasqueradeCheckFromResult handles unknown" {
    const result = buildMasqueradeCheckFromResult(.unknown);
    try std.testing.expect(result.status == .warn);
    try std.testing.expectEqualStrings("iptables check returned unexpected exit code", result.detail);
}

test "buildMasqueradeCheckFromResult handles command not found error" {
    const result = buildMasqueradeCheckFromResult(error.CommandNotFound);
    try std.testing.expect(result.status == .warn);
    try std.testing.expectEqualStrings("iptables not available", result.detail);
}

test "isValidInterfaceName accepts valid names" {
    try std.testing.expect(isValidInterfaceName("eth0"));
    try std.testing.expect(isValidInterfaceName("ens33"));
    try std.testing.expect(isValidInterfaceName("wlp2s0"));
    try std.testing.expect(isValidInterfaceName("en0"));
    try std.testing.expect(isValidInterfaceName("br0"));
    try std.testing.expect(isValidInterfaceName("veth_test"));
    try std.testing.expect(isValidInterfaceName("wg0"));
    try std.testing.expect(isValidInterfaceName("tun0"));
}

test "isValidInterfaceName rejects empty" {
    try std.testing.expect(!isValidInterfaceName(""));
}

test "isValidInterfaceName rejects too long" {
    const long_name = "abcdefghijklmnopqrstuvwxyz";
    try std.testing.expect(!isValidInterfaceName(long_name));
}

test "isValidInterfaceName rejects whitespace" {
    try std.testing.expect(!isValidInterfaceName("eth 0"));
    try std.testing.expect(!isValidInterfaceName("eth\t0"));
    try std.testing.expect(!isValidInterfaceName("eth\n0"));
}

test "isValidInterfaceName rejects slash" {
    try std.testing.expect(!isValidInterfaceName("eth/0"));
}

test "isValidInterfaceName rejects NUL/control characters" {
    try std.testing.expect(!isValidInterfaceName("eth\x000"));
    try std.testing.expect(!isValidInterfaceName("eth\x01"));
}
