// status.zig — Status payload rendering for tovarisch
// Renders v0 status JSON. Check ordering: process, binary, config, state_dir,
// http, tunnel, wg_peers, bfd, bgp. All dynamic detail uses caller-owned scratch.

const std = @import("std");
const telemetry = @import("runtime/telemetry.zig");
const tunnel_check = @import("tunnel_check.zig");
const status_checks = @import("status_checks.zig");
const build_info = @import("build_info.zig");
const bfd_status = @import("bfd/status.zig");
const bgp_status = @import("bgp/status.zig");
const bgp_serve = @import("cli/bgp_serve.zig");
const bgp_diag = @import("status_bgp_diagnostics.zig");
const status_network_diag = @import("status_network_diag.zig");
const network_diag_config = @import("net/network_diag_config.zig");
const lab_events = @import("runtime/lab_events.zig");
const tovarisch_config = @import("config.zig");

// Re-export BGP diagnostics from separate module for LLM-friendly file sizes.
pub const BgpDiagnostics = bgp_diag.BgpDiagnostics;
pub const deriveBgpDiagnostics = bgp_diag.deriveBgpDiagnostics;

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
    /// Whether this check owns the detail string allocation.
    /// When true, caller must free detail via deinit() or track ownership.
    owns_detail: bool = false,

    /// Free the detail string if this check owns it.
    /// Safe to call multiple times or on checks that don't own detail.
    pub fn deinit(self: *Check, allocator: std.mem.Allocator) void {
        if (self.owns_detail and self.detail.len > 0) {
            allocator.free(self.detail);
            self.detail = "";
            self.owns_detail = false;
        }
    }
};

pub const ConfigCheckState = union(enum) {
    no_config: void,
    loaded: struct {
        path: []const u8,
    },
};

pub fn buildConfigCheck(state: ConfigCheckState) Check {
    return switch (state) {
        .no_config => Check{ .name = "config", .status = .warn, .detail = "no config provided, using defaults" },
        .loaded => |loaded| Check{ .name = "config", .status = .ok, .detail = loaded.path },
    };
}

pub const Status = struct {
    service: []const u8,
    version: []const u8,
    node_id: []const u8,
    status: CheckStatus,
    checks: []const Check,
    bgp: ?BgpDiagnostics,
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

// Static/immutable check definitions (safe - static strings only)
const process_check = Check{ .name = "process", .status = .ok, .detail = "running" };
const binary_check = Check{ .name = "binary", .status = .ok, .detail = "tovarisch" };
const config_check = Check{ .name = "config", .status = .warn, .detail = "not configured yet" };
const http_check = Check{ .name = "http", .status = .ok, .detail = "http service route available" };

// Buffer size constants
const BFD_DETAIL_BUF_SIZE: usize = 64;
const BGP_DETAIL_BUF_SIZE: usize = 64;
const LOCAL_CHECKS_COUNT: usize = 9;

// Scratch buffer for status rendering
pub const StatusScratch = struct {
    /// Allocator for status checks that need dynamic memory
    allocator: std.mem.Allocator,
    bgp_detail: [BGP_DETAIL_BUF_SIZE]u8 = undefined,
    bfd_detail: [BFD_DETAIL_BUF_SIZE]u8 = undefined,
    checks: [LOCAL_CHECKS_COUNT]Check = undefined,
};

pub fn getStateDirCheckForPath(path: []const u8) Check {
    var path_buf: [4096]u8 = undefined;
    const c_path_result = toCString(path, &path_buf);
    if (c_path_result) |c_path| {
        const dir = std.c.opendir(c_path);
        if (dir) |d| {
            _ = std.c.closedir(d);
            return Check{ .name = "state_dir", .status = .ok, .detail = "state directory ready" };
        }
        const errno = std.c._errno().*;
        const e_noent = @intFromEnum(std.c.E.NOENT);
        const e_notdir = @intFromEnum(std.c.E.NOTDIR);
        if (errno == e_noent or errno == e_notdir) {
            return Check{ .name = "state_dir", .status = .warn, .detail = "state directory not found" };
        }
        return Check{ .name = "state_dir", .status = .unknown, .detail = "state directory inaccessible" };
    }
    return Check{ .name = "state_dir", .status = .unknown, .detail = "state directory inaccessible" };
}

pub fn getStateDirCheck() Check {
    return getStateDirCheckForPath(DEFAULT_STATE_DIR);
}

pub fn toCString(path: []const u8, buf: *[4096]u8) ?[*:0]const u8 {
    if (path.len >= buf.len) return null;
    // MemoryCopySafety: buf is a fixed [4096]u8 buffer. path is a caller-provided
    // slice. They are distinct memory regions; no aliasing.
    @memcpy(buf[0..path.len], path);
    buf[path.len] = 0;
    return @as([*:0]const u8, @ptrCast(buf));
}

// BFD and BGP check builders
pub fn getBfdCheck(rt: ?*const bfd_status.BfdRuntime, bfd_detail_buf: *[BFD_DETAIL_BUF_SIZE]u8) Check {
    const snapshot = bfd_status.snapshotFromRuntime(rt);
    const raw_check = bfd_status.buildStatusCheckInto(snapshot, bfd_detail_buf);
    const mapped_status: CheckStatus = switch (raw_check.status) {
        .ok => .ok,
        .warn => .warn,
        .@"error" => .@"error",
        .unknown => .unknown,
    };
    return Check{ .name = raw_check.name, .status = mapped_status, .detail = raw_check.detail };
}

pub fn getBgpCheck(state: bgp_status.BgpStatusState, bgp_detail_buf: *[BGP_DETAIL_BUF_SIZE]u8) Check {
    const bgp_check = bgp_status.buildBgpCheckInto(state, bgp_detail_buf);
    const mapped_status: CheckStatus = switch (bgp_check.status) {
        .ok => .ok,
        .warn => .warn,
        .@"error" => .@"error",
        .unknown => .unknown,
    };
    return Check{ .name = bgp_check.name, .status = mapped_status, .detail = bgp_check.detail };
}

// Default check helpers for CLI (no serve context)
pub fn getDefaultConfigCheck() Check {
    return config_check;
}
pub fn getDefaultBgpState() bgp_status.BgpStatusState {
    return .no_config;
}

// Main local checks builders
pub fn getLocalChecksWithBgp(
    bfd_runtime: ?*const bfd_status.BfdRuntime,
    config_check_injected: Check,
    bgp_state: bgp_status.BgpStatusState,
    scratch: *StatusScratch,
) []const Check {
    return getLocalChecksWithBgpAndLab(bfd_runtime, config_check_injected, bgp_state, scratch, .{});
}

/// Deinitialize all checks in a StatusScratch that may have owned allocations.
/// This must be called after rendering is complete and before the scratch is discarded.
/// Safe to call even if some checks don't have owned detail strings.
pub fn deinitScratchChecks(scratch: *StatusScratch) void {
    for (&scratch.checks) |*check| {
        check.deinit(scratch.allocator);
    }
}

pub fn getLocalChecksWithBgpAndLab(
    bfd_runtime: ?*const bfd_status.BfdRuntime,
    config_check_injected: Check,
    bgp_state: bgp_status.BgpStatusState,
    scratch: *StatusScratch,
    lab_config: tovarisch_config.LabConfig,
) []const Check {
    scratch.checks[0] = process_check;
    scratch.checks[1] = binary_check;
    scratch.checks[2] = config_check_injected;
    scratch.checks[3] = getStateDirCheck();
    scratch.checks[4] = http_check;
    scratch.checks[5] = tunnel_check.getTunnelCheckWithAllocator(scratch.allocator);

    // WireGuard check: return "disabled" if lab toggle is set
    if (lab_config.disable_wg_checks) {
        scratch.checks[6] = Check{ .name = "wg_peers", .status = .warn, .detail = "disabled via lab_config" };
    } else {
        // MemoryOwnership: Use scratch.allocator for wg_peers detail allocation.
        // Caller must call deinitScratchChecks() after rendering to free owned detail.
        scratch.checks[6] = status_checks.getWgPeersCheck(scratch.allocator);
    }

    // BFD check: return "disabled" if lab toggle is set
    if (lab_config.disable_bfd) {
        scratch.checks[7] = Check{ .name = "bfd", .status = .warn, .detail = "disabled via lab_config" };
    } else {
        scratch.checks[7] = getBfdCheck(bfd_runtime, &scratch.bfd_detail);
    }

    // BGP check: return "disabled" if lab toggle is set
    if (lab_config.disable_bgp) {
        scratch.checks[8] = Check{ .name = "bgp", .status = .warn, .detail = "disabled via lab_config" };
    } else {
        scratch.checks[8] = getBgpCheck(bgp_state, &scratch.bgp_detail);
    }

    return scratch.checks[0..LOCAL_CHECKS_COUNT];
}

pub fn getLocalChecksWithBfd(
    bfd_runtime: ?*const bfd_status.BfdRuntime,
    config_check_injected: Check,
    scratch: *StatusScratch,
) []const Check {
    return getLocalChecksWithBgp(bfd_runtime, config_check_injected, .no_config, scratch);
}

// Runtime status inputs
pub const RuntimeStatusInputs = struct {
    bfd_runtime: ?*const bfd_status.BfdRuntime = null,
    config_check: ConfigCheckState = .no_config,
    bgp_result: bgp_serve.BgpLoadResult = .{ .no_config = {} },
    network_diag_config: network_diag_config.NetworkDiagConfig = .{},
    lab_config: tovarisch_config.LabConfig = .{},
    lab_event_emitter: ?*const lab_events.LabEventEmitter = null,
};

// Status building
fn deriveBgpFromResult(result: bgp_serve.BgpLoadResult) ?BgpDiagnostics {
    const state = bgp_status.statusStateFromLoadResult(result);
    switch (state) {
        .configured, .reconnect_wait => {},
        else => return null,
    }
    return deriveBgpDiagnostics(state);
}

pub fn buildStatusWithInputs(inputs: RuntimeStatusInputs, scratch: *StatusScratch) Status {
    const config_check_built = buildConfigCheck(inputs.config_check);
    const live_bgp_state = bgp_status.statusStateFromLoadResult(inputs.bgp_result);
    const checks = getLocalChecksWithBgpAndLab(inputs.bfd_runtime, config_check_built, live_bgp_state, scratch, inputs.lab_config);
    return Status{
        .service = "tovarisch",
        .version = build_info.version,
        .node_id = "local-dev",
        .status = deriveStatus(checks),
        .checks = checks,
        .bgp = deriveBgpFromResult(inputs.bgp_result),
        .runtime = telemetry.getRuntimeTelemetry(),
    };
}

pub fn buildStatus(scratch: *StatusScratch) Status {
    return buildStatusWithBfd(null, scratch);
}

pub fn buildStatusWithBfd(bfd_runtime: ?*const bfd_status.BfdRuntime, scratch: *StatusScratch) Status {
    const checks = getLocalChecksWithBfd(bfd_runtime, getDefaultConfigCheck(), scratch);
    return Status{
        .service = "tovarisch",
        .version = build_info.version,
        .node_id = "local-dev",
        .status = deriveStatus(checks),
        .checks = checks,
        .bgp = null,
        .runtime = telemetry.getRuntimeTelemetry(),
    };
}

// JSON rendering
pub fn renderPayload(writer: anytype) !void {
    try renderPayloadWithBfd(writer, null);
}

// MemoryOwnership: page_allocator is bounded per call and released immediately after render.
pub fn renderPayloadWithBfd(writer: anytype, bfd_runtime: ?*const bfd_status.BfdRuntime) !void {
    var scratch = StatusScratch{ .allocator = std.heap.page_allocator };
    const s = buildStatusWithBfd(bfd_runtime, &scratch);
    try renderStatus(writer, s);
}

// MemoryOwnership: page_allocator is bounded per call and released immediately after render.
pub fn renderPayloadWithContext(writer: anytype, inputs: RuntimeStatusInputs) !void {
    var scratch = StatusScratch{ .allocator = std.heap.page_allocator };
    const s = buildStatusWithInputs(inputs, &scratch);
    try renderStatus(writer, s);
}

fn renderBgpDiagnostics(writer: anytype, bgp: ?BgpDiagnostics) !void {
    if (bgp) |d| {
        try writer.writeAll(",\"bgp\":{");
        if (d.state) |state| {
            try writer.writeAll("\"state\":\"");
            try writer.writeAll(state);
            try writer.writeAll("\",");
        } else {
            try writer.writeAll("\"state\":null,");
        }
        try writer.writeAll("\"reconnect_count\":");
        try writer.print("{d}", .{d.reconnect_count});
        try writer.writeAll(",");
        if (d.last_socket_error) |err| {
            try writer.writeAll("\"last_socket_error\":\"");
            try writer.writeAll(err);
            try writer.writeAll("\"}");
        } else {
            try writer.writeAll("\"last_socket_error\":null}");
        }
    }
}

pub fn renderStatus(writer: anytype, s: Status) !void {
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
    try writer.writeAll("]"); // Close checks array
    try renderBgpDiagnostics(writer, s.bgp);
    try writer.writeAll(",\"runtime\":{\"pid\":");
    try writer.print("{d}", .{s.runtime.pid});
    if (s.runtime.rss_kib) |rss| {
        try writer.writeAll(",\"rss_kib\":");
        try writer.print("{d}", .{rss});
    } else {
        try writer.writeAll(",\"rss_kib\":null");
    }
    try writer.writeAll("}}\n");
}

/// Render status payload with optional network diagnostics.
/// When include_network_diag is true, includes the network_diag field.
/// MemoryOwnership: transient allocations freed via defer before return.
pub fn renderPayloadWithContextAndDiag(
    writer: anytype,
    inputs: RuntimeStatusInputs,
    allocator: std.mem.Allocator,
    include_network_diag: bool,
) !void {
    var scratch = StatusScratch{ .allocator = allocator };
    const s = buildStatusWithInputs(inputs, &scratch);

    // Collect network diagnostics if requested
    var network_diag_opt: ?status_network_diag.NetworkDiag = null;
    if (include_network_diag) {
        network_diag_opt = status_network_diag.collectNetworkDiag(allocator, inputs.network_diag_config) catch null;
    }
    defer {
        if (network_diag_opt) |*d| {
            d.deinit(allocator);
        }
        deinitScratchChecks(&scratch);
    }

    // Render the status JSON with optional network_diag
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
    try writer.writeAll("]"); // Close checks array
    try renderBgpDiagnostics(writer, s.bgp);

    // Include network_diag when requested - either collected data or structured error
    if (include_network_diag) {
        try writer.writeAll(",\"network_diag\":");
        if (network_diag_opt) |*diag| {
            try status_network_diag.renderNetworkDiag(writer, diag);
        } else {
            // On collection failure, return structured error response instead of omitting
            try writer.writeAll("{\"status\":\"unavailable\",\"wireguard\":null,\"interfaces\":[],\"routes\":[],\"underlay_tcp\":[],\"events\":[]}");
        }
    }

    try writer.writeAll(",\"runtime\":{\"pid\":");
    try writer.print("{d}", .{s.runtime.pid});
    if (s.runtime.rss_kib) |rss| {
        try writer.writeAll(",\"rss_kib\":");
        try writer.print("{d}", .{rss});
    } else {
        try writer.writeAll(",\"rss_kib\":null");
    }
    try writer.writeAll("}\n");
}
