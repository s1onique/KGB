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

const std = @import("std");
const telemetry = @import("runtime/telemetry.zig");
const tunnel_check = @import("tunnel_check.zig");
const status_checks = @import("status_checks.zig");
const build_info = @import("build_info.zig");
const bfd_status = @import("bfd/status.zig");

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

var local_checks_buf: [8]Check = undefined;

pub fn getBfdCheck(rt: ?*const bfd_status.BfdRuntime) Check {
    const snapshot = bfd_status.snapshotFromRuntime(rt);
    const raw_check = bfd_status.buildStatusCheck(snapshot);
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

pub fn getLocalChecks() []const Check {
    return getLocalChecksWithBfd(null);
}

pub fn getLocalChecksWithBfd(bfd_runtime: ?*const bfd_status.BfdRuntime) []const Check {
    local_checks_buf[0] = process_check;
    local_checks_buf[1] = binary_check;
    local_checks_buf[2] = config_check;
    local_checks_buf[3] = getStateDirCheck();
    local_checks_buf[4] = http_check;
    local_checks_buf[5] = tunnel_check.getTunnelCheckDefault();
    local_checks_buf[6] = status_checks.getWgPeersCheck(std.heap.page_allocator);
    local_checks_buf[7] = getBfdCheck(bfd_runtime);
    return &local_checks_buf;
}

pub fn getStatus() Status {
    return getStatusWithBfd(null);
}

pub fn getStatusWithBfd(bfd_runtime: ?*const bfd_status.BfdRuntime) Status {
    const checks = getLocalChecksWithBfd(bfd_runtime);
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
