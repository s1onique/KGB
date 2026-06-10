// status.zig — Status payload rendering for tovarisch
//
// Renders the v0 status JSON payload for `tovarisch status --json`.
// Uses injectable checks for testability:
// - getLocalChecks() returns all local health checks
// - getStatus() builds the full status payload
//
// Check ordering (stable for operator readability):
//   1. process  - daemon is running
//   2. binary   - binary name is correct
//   3. config   - configuration state
//   4. state_dir - state directory exists
//   5. http     - HTTP service route available
//   6. tunnel   - tunnel interface presence
//   7. wg_peers - WireGuard peer diagnostics
//   8. bfd      - BFD multihop session status
//   9. bgp      - BGP config/runtime state

const std = @import("std");
const telemetry = @import("runtime/telemetry.zig");
const tunnel_check = @import("tunnel_check.zig");
const status_checks = @import("status_checks.zig");
const build_info = @import("build_info.zig");
const bfd_status = @import("bfd/status.zig");
const bgp_status = @import("bgp/status.zig");

pub const DEFAULT_STATE_DIR = ".tovarisch/state";

pub const CheckStatus = enum {
    ok,
    warn,
    @"error",
    unknown,
};

pub const Check = struct {
    name: []const u8,
    status: CheckStatus,
    detail: []const u8,
};

/// Configuration check state for status reporting.
/// This is the immutable state derived at serve startup from the loaded config.
/// Ownership: Caller owns the path memory for .loaded case.
pub const ConfigCheckState = union(enum) {
    /// No config path was provided to serve command.
    no_config: void,
    /// Config was loaded successfully - path is owned by caller for daemon lifetime.
    loaded: struct {
        path: []const u8,
    },
};

/// Build a config check from the config check state.
/// This function is pure and stateless - it only transforms input.
pub fn buildConfigCheck(state: ConfigCheckState) Check {
    return switch (state) {
        .no_config => Check{
            .name = "config",
            .status = .warn,
            .detail = "no config provided, using defaults",
        },
        .loaded => |loaded| Check{
            .name = "config",
            .status = .ok,
            .detail = loaded.path,
        },
    };
}

pub const Status = struct {
    service: []const u8,
    version: []const u8,
    node_id: []const u8,
    status: CheckStatus,
    checks: []const Check,
    runtime: telemetry.RuntimeTelemetry,
};

pub fn deriveStatus(checks: []const Check) CheckStatus {
    for (checks) |check| {
        if (check.status == .@"error") return .@"error";
    }
    for (checks) |check| {
        if (check.status == .warn) return .warn;
    }
    return .ok;
}

const process_check = Check{
    .name = "process",
    .status = .ok,
    .detail = "running",
};

const binary_check = Check{
    .name = "binary",
    .status = .ok,
    .detail = "tovarisch",
};

const config_check = Check{
    .name = "config",
    .status = .warn,
    .detail = "not configured yet",
};

const http_check = Check{
    .name = "http",
    .status = .ok,
    .detail = "http service route available",
};

pub fn getStateDirCheckForPath(path: []const u8) Check {
    var path_buf: [4096]u8 = undefined;
    const c_path_result = toCString(path, &path_buf);
    if (c_path_result) |c_path| {
        const dir = std.c.opendir(c_path);
        if (dir) |d| {
            _ = std.c.closedir(d);
            return Check{
                .name = "state_dir",
                .status = .ok,
                .detail = "state directory ready",
            };
        }
        const errno = std.c._errno().*;
        const e_noent = @intFromEnum(std.c.E.NOENT);
        const e_notdir = @intFromEnum(std.c.E.NOTDIR);
        if (errno == e_noent or errno == e_notdir) {
            return Check{
                .name = "state_dir",
                .status = .warn,
                .detail = "state directory not found",
            };
        }
        return Check{
            .name = "state_dir",
            .status = .unknown,
            .detail = "state directory inaccessible",
        };
    }
    return Check{
        .name = "state_dir",
        .status = .unknown,
        .detail = "state directory inaccessible",
    };
}

pub fn getStateDirCheck() Check {
    return getStateDirCheckForPath(DEFAULT_STATE_DIR);
}

pub fn toCString(path: []const u8, buf: *[4096]u8) ?[*:0]const u8 {
    if (path.len >= buf.len) return null;
    @memcpy(buf[0..path.len], path);
    buf[path.len] = 0;
    return @as([*:0]const u8, @ptrCast(buf));
}

var local_checks_buf: [9]Check = undefined;

/// Module-level buffer for BFD status detail when peers are partially up.
/// This avoids per-request heap allocation for the "X/Y bfd sessions up" case.
/// Max format: "9999/9999 bfd sessions up" = 24 chars, well within 64 byte buffer.
var bfd_detail_buf: [64]u8 = undefined;

/// BGP detail buffer for status rendering.
/// Uses caller-provided buffer to avoid module-level mutable state.
/// Max format: "BGP configured; 9999 advertised prefixes" = 42 chars max.
const BGP_DETAIL_BUF_SIZE: usize = 64;

pub fn getBfdCheck(rt: ?*const bfd_status.BfdRuntime) Check {
    const snapshot = bfd_status.snapshotFromRuntime(rt);
    // Use buffer-based variant to avoid any heap allocation
    const raw_check = bfd_status.buildStatusCheckInto(snapshot, &bfd_detail_buf);
    const mapped_status: CheckStatus = switch (raw_check.status) {
        .ok => .ok,
        .warn => .warn,
        .@"error" => .@"error",
        .unknown => .unknown,
    };
    return Check{
        .name = raw_check.name,
        .status = mapped_status,
        .detail = raw_check.detail,
    };
}

/// Get BGP check from BGP status state using caller-provided scratch buffer.
/// This function is allocation-free - uses caller's buffer for dynamic content.
pub fn getBgpCheck(state: bgp_status.BgpStatusState, scratch: *[64]u8) Check {
    const bgp_check = bgp_status.buildBgpCheckInto(state, scratch);
    // Map BGP check status to status.CheckStatus
    const mapped_status: CheckStatus = switch (bgp_check.status) {
        .ok => .ok,
        .warn => .warn,
        .@"error" => .@"error",
        .unknown => .unknown,
    };
    return Check{
        .name = bgp_check.name,
        .status = mapped_status,
        .detail = bgp_check.detail,
    };
}

/// Default config check for standalone CLI (status --json).
/// Uses the static warn "not configured yet" message since there's no serve context.
pub fn getDefaultConfigCheck() Check {
    return config_check;
}

/// Default BGP status state for standalone CLI (status --json).
/// Uses no_config since there's no serve context.
pub fn getDefaultBgpState() bgp_status.BgpStatusState {
    return .no_config;
}

pub fn getLocalChecks() []const Check {
    var scratch = StatusScratch{};
    return getLocalChecksWithBgp(null, getDefaultConfigCheck(), getDefaultBgpState(), &scratch);
}

/// Build local checks with explicit BFD runtime, config check state, and BGP state.
/// Uses caller-provided scratch buffer for BGP detail formatting to avoid dangling pointers.
/// The scratch buffer must outlive the returned checks slice.
pub fn getLocalChecksWithBgp(
    bfd_runtime: ?*const bfd_status.BfdRuntime,
    config_check_injected: Check,
    bgp_state: bgp_status.BgpStatusState,
    scratch: *StatusScratch,
) []const Check {
    local_checks_buf[0] = process_check;
    local_checks_buf[1] = binary_check;
    local_checks_buf[2] = config_check_injected;
    local_checks_buf[3] = getStateDirCheck();
    local_checks_buf[4] = http_check;
    local_checks_buf[5] = tunnel_check.getTunnelCheckDefault();
    local_checks_buf[6] = status_checks.getWgPeersCheck(std.heap.page_allocator);
    local_checks_buf[7] = getBfdCheck(bfd_runtime);
    local_checks_buf[8] = getBgpCheck(bgp_state, &scratch.bgp_detail);
    return local_checks_buf[0..9];
}

/// Build local checks with explicit BFD runtime and config check state.
/// Legacy function for backward compatibility - uses no_config for BGP.
/// DEPRECATED: Use getLocalChecksWithBgp() instead.
pub fn getLocalChecksWithBfd(
    bfd_runtime: ?*const bfd_status.BfdRuntime,
    config_check_injected: Check,
    scratch: *StatusScratch,
) []const Check {
    return getLocalChecksWithBgp(bfd_runtime, config_check_injected, .no_config, scratch);
}

/// Scratch buffer for status rendering.
/// Caller must keep this alive until JSON serialization completes.
/// This avoids dangling pointers when checks reference dynamically formatted details.
pub const StatusScratch = struct {
    /// Buffer for BGP detail formatting (e.g., "BGP configured; N advertised prefixes").
    bgp_detail: [BGP_DETAIL_BUF_SIZE]u8 = undefined,
};

/// Runtime status inputs for injectable status rendering.
/// This struct allows explicit runtime inputs without module-global state.
pub const RuntimeStatusInputs = struct {
    /// Optional BFD runtime owned by the daemon.
    bfd_runtime: ?*const bfd_status.BfdRuntime = null,
    /// Config check state - defaults to warn with static message.
    config_check: ConfigCheckState = .no_config,
    /// BGP status state - defaults to no_config.
    bgp_state: bgp_status.BgpStatusState = .no_config,
};

/// Build status with explicit runtime inputs and caller-provided scratch.
/// The scratch buffer must outlive the returned Status for JSON serialization.
pub fn buildStatusWithInputs(inputs: RuntimeStatusInputs, scratch: *StatusScratch) Status {
    const config_check_built = buildConfigCheck(inputs.config_check);
    const checks = getLocalChecksWithBgp(inputs.bfd_runtime, config_check_built, inputs.bgp_state, scratch);
    return Status{
        .service = "tovarisch",
        .version = build_info.version,
        .node_id = "local-dev",
        .status = deriveStatus(checks),
        .checks = checks,
        .runtime = telemetry.getRuntimeTelemetry(),
    };
}

pub fn getStatus() Status {
    return getStatusWithBfd(null);
}

pub fn getStatusWithBfd(bfd_runtime: ?*const bfd_status.BfdRuntime) Status {
    var scratch = StatusScratch{};
    const checks = getLocalChecksWithBfd(bfd_runtime, getDefaultConfigCheck(), &scratch);
    return Status{
        .service = "tovarisch",
        .version = build_info.version,
        .node_id = "local-dev",
        .status = deriveStatus(checks),
        .checks = checks,
        .runtime = telemetry.getRuntimeTelemetry(),
    };
}

pub fn renderPayload(writer: anytype) !void {
    try renderPayloadWithBfd(writer, null);
}

pub fn renderPayloadWithBfd(writer: anytype, bfd_runtime: ?*const bfd_status.BfdRuntime) !void {
    const s = getStatusWithBfd(bfd_runtime);
    try renderStatus(writer, s);
}

/// Render status payload with explicit runtime inputs (BFD + config check).
/// This is the primary rendering path for the HTTP /status endpoint.
/// Creates scratch buffer owned by this function's stack frame - safe for JSON serialization.
pub fn renderPayloadWithContext(writer: anytype, inputs: RuntimeStatusInputs) !void {
    var scratch = StatusScratch{};
    const s = buildStatusWithInputs(inputs, &scratch);
    try renderStatus(writer, s);
}

fn renderStatus(writer: anytype, s: Status) !void {
    try writer.writeAll("{\"service\":\"");
    try writer.writeAll(s.service);
    try writer.writeAll("\",\"version\":\"");
    try writer.writeAll(s.version);
    try writer.writeAll("\",\"node_id\":\"");
    try writer.writeAll(s.node_id);
    try writer.writeAll("\",\"status\":\"");
    try writer.writeAll(@tagName(s.status));
    try writer.writeAll("\",\"checks\":[");
    for (s.checks, 0..) |check, i| {
        if (i > 0) try writer.writeAll(",");
        try writer.writeAll("{\"name\":\"");
        try writer.writeAll(check.name);
        try writer.writeAll("\",\"status\":\"");
        try writer.writeAll(@tagName(check.status));
        try writer.writeAll("\",\"detail\":\"");
        try writer.writeAll(check.detail);
        try writer.writeAll("\"}");
    }
    try writer.writeAll("],\"runtime\":{\"pid\":");
    try writer.print("{d}", .{s.runtime.pid});
    if (s.runtime.rss_kib) |rss| {
        try writer.writeAll(",\"rss_kib\":");
        try writer.print("{d}", .{rss});
    } else {
        try writer.writeAll(",\"rss_kib\":null");
    }
    try writer.writeAll("}}\n");
}
