// wg_cli_facts.zig — WireGuard CLI fact collection for classification
//
// Part of wg_status_boundary_cli.zig (split to satisfy LLM-friendliness limits).
// Contains fact collection and conversion functions that bridge CLI diagnostics
// to the pure classifier.
//
// ACT-HULK29R-ZIG016-WG-STATUS-EVIDENCE-WIRING:
// Extended to collect real OS-link facts and wg show interfaces membership
// for accurate WireGuard diagnostic classification.

const std = @import("std");
const wg = @import("wg_status_boundary.zig");
const classifier = @import("wg_diagnostic_classifier.zig");

// POSIX constants (not exposed in Zig 0.16 std.c)
const F_GETFL: c_int = 3;
const F_SETFL: c_int = 4;
const O_NONBLOCK: c_int = if (@import("builtin").os.tag == .linux) 2048 else 4;
const POLLIN: c_short = 0x0001;
const POLLHUP: c_short = 0x0010;
const POLLERR: c_short = 0x0008;

pub const OwnedWgCommandResult = struct {
    stdout_storage: []u8,
    stderr_storage: []u8,
    stdout: []const u8,
    stderr: []const u8,
    exit_code: c_int,
    stdout_truncated: bool,
    stderr_truncated: bool,
    timed_out: bool,

    pub fn deinit(self: *OwnedWgCommandResult, allocator: std.mem.Allocator) void {
        allocator.free(self.stdout_storage);
        allocator.free(self.stderr_storage);
        self.* = undefined;
    }
};

/// Default WireGuard interface name.
/// Re-exported from wg_status_boundary_cli for fact collection use.
pub const DEFAULT_WG_INTERFACE: [:0]const u8 = "wg-kgb0";

/// Structured evidence for WireGuard classification.
/// Collected from read-only OS and WG CLI probes.
pub const CliEvidence = struct {
    /// Interface name being checked.
    interface_name: []const u8,
    /// Whether OS-level link was detected for the interface.
    os_link_seen: bool,
    /// Kind of OS link detected.
    os_link_kind: classifier.OsLinkKind,
    /// Whether wg show interfaces succeeded.
    wg_interfaces_seen: bool,
    /// Whether wg show interfaces output contains our interface name.
    wg_interface_list_contains_name: bool,
    /// Classified stderr from wg show dump command.
    wg_show_stderr_class: classifier.WgStderrClass,
};

/// Default empty evidence with placeholder values.
pub const emptyEvidence = CliEvidence{
    .interface_name = DEFAULT_WG_INTERFACE,
    .os_link_seen = false,
    .os_link_kind = .unknown,
    .wg_interfaces_seen = false,
    .wg_interface_list_contains_name = false,
    .wg_show_stderr_class = .none,
};

/// Default timeout for CLI probes (3 seconds - shorter than wg dump timeout).
const PROBE_TIMEOUT_SECS: u64 = 3;

/// Maximum stdout/stderr buffer sizes for probes.
const PROBE_MAX_STDOUT: usize = 4096;
const PROBE_MAX_STDERR: usize = 256;

/// Run `ip -d link show <interface>` to probe OS-level link status.
/// Returns owned result - caller must call deinit().
pub fn runIpLinkShow(
    allocator: std.mem.Allocator,
    interface_name: []const u8,
    timeout_secs: u64,
) !OwnedWgCommandResult {
    var stdout_pipe: [2]c_int = undefined;
    var stderr_pipe: [2]c_int = undefined;

    if (std.c.pipe(&stdout_pipe) != 0) return error.PipeFailed;
    if (std.c.pipe(&stderr_pipe) != 0) {
        _ = std.c.close(stdout_pipe[0]);
        _ = std.c.close(stdout_pipe[1]);
        return error.PipeFailed;
    }
    defer {
        _ = std.c.close(stdout_pipe[0]);
        _ = std.c.close(stdout_pipe[1]);
        _ = std.c.close(stderr_pipe[0]);
        _ = std.c.close(stderr_pipe[1]);
    }

    // Set nonblocking on pipes
    _ = std.c.fcntl(stdout_pipe[0], F_SETFL, std.c.fcntl(stdout_pipe[0], F_GETFL, @as(c_int, 0)) | O_NONBLOCK);
    _ = std.c.fcntl(stderr_pipe[0], F_SETFL, std.c.fcntl(stderr_pipe[0], F_GETFL, @as(c_int, 0)) | O_NONBLOCK);

    const pid = std.c.fork();
    if (pid < 0) return error.ForkFailed;

    if (pid == 0) {
        // Child
        _ = std.c.close(stdout_pipe[0]);
        _ = std.c.close(stderr_pipe[0]);
        _ = std.c.dup2(stdout_pipe[1], 1);
        _ = std.c.dup2(stderr_pipe[1], 2);
        _ = std.c.close(stdout_pipe[1]);
        _ = std.c.close(stderr_pipe[1]);

        var iface_buf: [64]u8 = undefined;
        const iface_formatted = std.fmt.bufPrint(&iface_buf, "{s}\x00", .{interface_name}) catch {
            std.c._exit(127);
        };
        const iface_null: [*:0]const u8 = @ptrCast(iface_buf[0..iface_formatted.len - 1 :0].ptr);

        // Explicitly null-terminated argv for execve
        const argv: [6]?[*:0]const u8 = .{ "/sbin/ip", "-d", "link", "show", iface_null, null };
        const argv_null: [*:null]const ?[*:0]const u8 = @ptrCast(&argv);
        const empty_env: [*:null]const ?[*:0]const u8 = &.{};
        _ = std.c.execve("/sbin/ip", argv_null, empty_env);
        std.c._exit(127);
    }

    // Parent - read output
    _ = std.c.close(stdout_pipe[1]);
    _ = std.c.close(stderr_pipe[1]);

    var stdout_buf = try allocator.alloc(u8, PROBE_MAX_STDOUT);
    errdefer allocator.free(stdout_buf);
    var stderr_buf = try allocator.alloc(u8, PROBE_MAX_STDERR);
    errdefer allocator.free(stderr_buf);

    var stdout_len: usize = 0;
    var stderr_len: usize = 0;
    var stdout_truncated = false;
    var stderr_truncated = false;
    var timed_out = false;

    var poll_fds: [2]std.c.pollfd = .{
        .{ .fd = stdout_pipe[0], .events = POLLIN, .revents = 0 },
        .{ .fd = stderr_pipe[0], .events = POLLIN, .revents = 0 },
    };

    var remaining_ms: i32 = @intCast(timeout_secs * 1000);
    const poll_interval_ms: i32 = 100;

    while (true) {
        if (stdout_truncated and stderr_truncated) break;

        const poll_ms: i32 = @min(remaining_ms, poll_interval_ms);
        const poll_result = std.c.poll(&poll_fds, 2, poll_ms);
        remaining_ms -= poll_ms;

        if (poll_result < 0) continue;
        if (remaining_ms <= 0) {
            timed_out = true;
            break;
        }

        if (poll_fds[0].revents & POLLIN != 0 and !stdout_truncated) {
            const n = std.c.read(stdout_pipe[0], stdout_buf.ptr + stdout_len, PROBE_MAX_STDOUT - stdout_len);
            if (n > 0) {
                stdout_len += @intCast(n);
            } else if (n == 0 or (n < 0 and std.c.errno(n) != .AGAIN)) {
                poll_fds[0].fd = -1;
            }
        }
        if (stdout_len >= PROBE_MAX_STDOUT) stdout_truncated = true;

        if (poll_fds[1].revents & POLLIN != 0 and !stderr_truncated) {
            const n = std.c.read(stderr_pipe[0], stderr_buf.ptr + stderr_len, PROBE_MAX_STDERR - stderr_len);
            if (n > 0) {
                stderr_len += @intCast(n);
            } else if (n == 0 or (n < 0 and std.c.errno(n) != .AGAIN)) {
                poll_fds[1].fd = -1;
            }
        }
        if (stderr_len >= PROBE_MAX_STDERR) stderr_truncated = true;

        if ((poll_fds[0].revents & (POLLHUP | POLLERR) != 0) and (poll_fds[1].revents & (POLLHUP | POLLERR) != 0)) {
            break;
        }
        if (poll_fds[0].fd == -1 and poll_fds[1].fd == -1) break;
    }

    if (stdout_truncated) {
        var drain: [256]u8 = undefined;
        while (true) {
            const n = std.c.read(stdout_pipe[0], &drain, drain.len);
            if (n <= 0) break;
        }
    }

    if (timed_out) {
        _ = std.c.kill(pid, .KILL);
    }

    var status: c_int = undefined;
    _ = std.c.waitpid(pid, &status, 0);

    return OwnedWgCommandResult{
        .stdout_storage = stdout_buf,
        .stderr_storage = stderr_buf,
        .stdout = stdout_buf[0..stdout_len],
        .stderr = stderr_buf[0..stderr_len],
        .exit_code = if ((status & 0x7f) == 0) (status >> 8) & 0xff else -1,
        .stdout_truncated = stdout_truncated,
        .stderr_truncated = stderr_truncated,
        .timed_out = timed_out,
    };
}

/// Run `wg show interfaces` to list WireGuard interfaces.
/// Returns owned result - caller must call deinit().
pub fn runWgShowInterfaces(
    allocator: std.mem.Allocator,
    wg_path: [*:0]const u8,
    timeout_secs: u64,
) !OwnedWgCommandResult {
    var stdout_pipe: [2]c_int = undefined;
    var stderr_pipe: [2]c_int = undefined;

    if (std.c.pipe(&stdout_pipe) != 0) return error.PipeFailed;
    if (std.c.pipe(&stderr_pipe) != 0) {
        _ = std.c.close(stdout_pipe[0]);
        _ = std.c.close(stdout_pipe[1]);
        return error.PipeFailed;
    }
    defer {
        _ = std.c.close(stdout_pipe[0]);
        _ = std.c.close(stdout_pipe[1]);
        _ = std.c.close(stderr_pipe[0]);
        _ = std.c.close(stderr_pipe[1]);
    }

    _ = std.c.fcntl(stdout_pipe[0], F_SETFL, std.c.fcntl(stdout_pipe[0], F_GETFL, @as(c_int, 0)) | O_NONBLOCK);
    _ = std.c.fcntl(stderr_pipe[0], F_SETFL, std.c.fcntl(stderr_pipe[0], F_GETFL, @as(c_int, 0)) | O_NONBLOCK);

    const pid = std.c.fork();
    if (pid < 0) return error.ForkFailed;

    if (pid == 0) {
        _ = std.c.close(stdout_pipe[0]);
        _ = std.c.close(stderr_pipe[0]);
        _ = std.c.dup2(stdout_pipe[1], 1);
        _ = std.c.dup2(stderr_pipe[1], 2);
        _ = std.c.close(stdout_pipe[1]);
        _ = std.c.close(stderr_pipe[1]);

        // Explicitly null-terminated argv for execve
        const argv: [4]?[*:0]const u8 = .{ wg_path, "show", "interfaces", null };
        const argv_null: [*:null]const ?[*:0]const u8 = @ptrCast(&argv);
        const empty_env: [*:null]const ?[*:0]const u8 = &.{};
        _ = std.c.execve(wg_path, argv_null, empty_env);
        std.c._exit(127);
    }

    _ = std.c.close(stdout_pipe[1]);
    _ = std.c.close(stderr_pipe[1]);

    var stdout_buf = try allocator.alloc(u8, PROBE_MAX_STDOUT);
    errdefer allocator.free(stdout_buf);
    var stderr_buf = try allocator.alloc(u8, PROBE_MAX_STDERR);
    errdefer allocator.free(stderr_buf);

    var stdout_len: usize = 0;
    var stderr_len: usize = 0;
    var stdout_truncated = false;
    var stderr_truncated = false;
    var timed_out = false;

    var poll_fds: [2]std.c.pollfd = .{
        .{ .fd = stdout_pipe[0], .events = POLLIN, .revents = 0 },
        .{ .fd = stderr_pipe[0], .events = POLLIN, .revents = 0 },
    };

    var remaining_ms: i32 = @intCast(timeout_secs * 1000);
    const poll_interval_ms: i32 = 100;

    while (true) {
        if (stdout_truncated and stderr_truncated) break;

        const poll_ms: i32 = @min(remaining_ms, poll_interval_ms);
        const poll_result = std.c.poll(&poll_fds, 2, poll_ms);
        remaining_ms -= poll_ms;

        if (poll_result < 0) continue;
        if (remaining_ms <= 0) {
            timed_out = true;
            break;
        }

        if (poll_fds[0].revents & POLLIN != 0 and !stdout_truncated) {
            const n = std.c.read(stdout_pipe[0], stdout_buf.ptr + stdout_len, PROBE_MAX_STDOUT - stdout_len);
            if (n > 0) {
                stdout_len += @intCast(n);
            } else if (n == 0 or (n < 0 and std.c.errno(n) != .AGAIN)) {
                poll_fds[0].fd = -1;
            }
        }
        if (stdout_len >= PROBE_MAX_STDOUT) stdout_truncated = true;

        if (poll_fds[1].revents & POLLIN != 0 and !stderr_truncated) {
            const n = std.c.read(stderr_pipe[0], stderr_buf.ptr + stderr_len, PROBE_MAX_STDERR - stderr_len);
            if (n > 0) {
                stderr_len += @intCast(n);
            } else if (n == 0 or (n < 0 and std.c.errno(n) != .AGAIN)) {
                poll_fds[1].fd = -1;
            }
        }
        if (stderr_len >= PROBE_MAX_STDERR) stderr_truncated = true;

        if ((poll_fds[0].revents & (POLLHUP | POLLERR) != 0) and (poll_fds[1].revents & (POLLHUP | POLLERR) != 0)) {
            break;
        }
        if (poll_fds[0].fd == -1 and poll_fds[1].fd == -1) break;
    }

    if (stdout_truncated) {
        var drain: [256]u8 = undefined;
        while (true) {
            const n = std.c.read(stdout_pipe[0], &drain, drain.len);
            if (n <= 0) break;
        }
    }

    if (timed_out) {
        _ = std.c.kill(pid, .KILL);
    }

    var status: c_int = undefined;
    _ = std.c.waitpid(pid, &status, 0);

    return OwnedWgCommandResult{
        .stdout_storage = stdout_buf,
        .stderr_storage = stderr_buf,
        .stdout = stdout_buf[0..stdout_len],
        .stderr = stderr_buf[0..stderr_len],
        .exit_code = if ((status & 0x7f) == 0) (status >> 8) & 0xff else -1,
        .stdout_truncated = stdout_truncated,
        .stderr_truncated = stderr_truncated,
        .timed_out = timed_out,
    };
}

/// Find first available wg command path.
fn findWgCommand() ?[*:0]const u8 {
    const wg_paths = [_][*:0]const u8{
        "/usr/bin/wg",
        "/usr/sbin/wg",
        "/sbin/wg",
    };
    for (wg_paths) |path| {
        if (std.c.access(path, std.c.X_OK) == 0) {
            return path;
        }
    }
    return null;
}

/// Collect OS-link evidence for the configured interface.
/// Uses `ip -d link show <interface>` probe.
pub fn collectOsLinkEvidence(
    allocator: std.mem.Allocator,
    interface_name: []const u8,
) !struct {
    os_link_seen: bool,
    os_link_kind: classifier.OsLinkKind,
} {
    var result = runIpLinkShow(allocator, interface_name, PROBE_TIMEOUT_SECS) catch {
        return .{
            .os_link_seen = false,
            .os_link_kind = .probe_failed,
        };
    };
    defer result.deinit(allocator);

    // ip link returns exit 1 if interface doesn't exist
    if (result.exit_code == 1) {
        return .{
            .os_link_seen = false,
            .os_link_kind = .missing,
        };
    }

    if (result.exit_code != 0) {
        return .{
            .os_link_seen = false,
            .os_link_kind = .probe_failed,
        };
    }

    // Parse stdout to determine link kind
    const kind = classifier.classifyIpLinkDetail(result.stdout);
    return .{
        .os_link_seen = true,
        .os_link_kind = kind,
    };
}

/// Collect WG interfaces evidence.
/// Uses `wg show interfaces` probe.
pub fn collectWgInterfacesEvidence(
    allocator: std.mem.Allocator,
    interface_name: []const u8,
) !struct {
    wg_interfaces_seen: bool,
    wg_interface_list_contains_name: bool,
} {
    const wg_path = findWgCommand() orelse {
        return .{
            .wg_interfaces_seen = false,
            .wg_interface_list_contains_name = false,
        };
    };

    var result = runWgShowInterfaces(allocator, wg_path, PROBE_TIMEOUT_SECS) catch {
        return .{
            .wg_interfaces_seen = false,
            .wg_interface_list_contains_name = false,
        };
    };
    defer result.deinit(allocator);

    // wg show interfaces returns exit 1 if wg tool has issues
    // exit 0 means success (even with empty output)
    if (result.exit_code == 127) {
        return .{
            .wg_interfaces_seen = false,
            .wg_interface_list_contains_name = false,
        };
    }

    const contains_name = classifier.classifyWgInterfacesOutput(result.stdout, interface_name);
    return .{
        .wg_interfaces_seen = true,
        .wg_interface_list_contains_name = contains_name,
    };
}

/// Collect all CLI evidence for WireGuard classification.
pub fn collectAllEvidence(
    allocator: std.mem.Allocator,
    interface_name: []const u8,
) !CliEvidence {
    const os_link = try collectOsLinkEvidence(allocator, interface_name);
    const wg_interfaces = try collectWgInterfacesEvidence(allocator, interface_name);

    return CliEvidence{
        .interface_name = interface_name,
        .os_link_seen = os_link.os_link_seen,
        .os_link_kind = os_link.os_link_kind,
        .wg_interfaces_seen = wg_interfaces.wg_interfaces_seen,
        .wg_interface_list_contains_name = wg_interfaces.wg_interface_list_contains_name,
        .wg_show_stderr_class = .none, // Set separately from command result
    };
}

/// Converts a WireGuardStatusDiagnosticAttempt into WgInterfaceFacts for classification.
/// This allows the status_checks module to use the classifier for precise diagnostics.
///
/// ACT-HULK29R-ZIG016-WG-STATUS-EVIDENCE-WIRING:
/// Now accepts structured CliEvidence for accurate classification.
pub fn factsFromDiagnosticAttempt(
    attempt: anytype,
    evidence: CliEvidence,
) classifier.WgInterfaceFacts {
    return switch (attempt) {
        .ok => |ok| classifier.WgInterfaceFacts{
            .configured_name = evidence.interface_name,
            .os_link_seen = evidence.os_link_seen,
            .os_link_kind = evidence.os_link_kind,
            .wg_tool_seen = true,
            .wg_interfaces_seen = evidence.wg_interfaces_seen,
            .wg_interface_list_contains_name = evidence.wg_interface_list_contains_name,
            .wg_show_exit_code = 0,
            .wg_show_stderr_class = .none,
            .wg_dump_succeeded = true,
            .wg_dump_peer_count = ok.status.peer_count,
            .wg_dump_has_handshake = ok.status.hasHandshake(),
            .wg_dump_malformed = false,
        },
        .err => |bad| blk: {
            const is_backend_missing = switch (bad.err) {
                error.backend_missing => true,
                else => false,
            };
            const is_malformed = switch (bad.err) {
                error.malformed_output => true,
                else => false,
            };
            break :blk classifier.WgInterfaceFacts{
                .configured_name = evidence.interface_name,
                .os_link_seen = evidence.os_link_seen,
                .os_link_kind = evidence.os_link_kind,
                .wg_tool_seen = !is_backend_missing,
                .wg_interfaces_seen = !is_backend_missing and evidence.wg_interfaces_seen,
                .wg_interface_list_contains_name = !is_backend_missing and evidence.wg_interface_list_contains_name,
                .wg_show_exit_code = bad.diagnostic.exit_code,
                .wg_show_stderr_class = evidence.wg_show_stderr_class,
                .wg_dump_succeeded = false,
                .wg_dump_peer_count = 0,
                .wg_dump_has_handshake = false,
                .wg_dump_malformed = is_malformed,
            };
        },
    };
}
