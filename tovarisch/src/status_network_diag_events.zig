// status_network_diag_events.zig — TCP absence events for network diagnostics
const std = @import("std");
const EventOutput = @import("status_network_diag_types.zig").EventOutput;

pub const TcpAbsenceReason = enum {
    /// No matching socket found for the filter criteria.
    no_matching_socket,
    /// Socket was closed before capture completed.
    socket_closed_before_capture,
    /// The ss command itself failed (non-zero exit, signal, etc.).
    command_failed,
    /// Underlay TCP diagnostics are not configured/enabled.
    not_configured,
    /// Permission denied when running ss command.
    permission_denied,
    /// Target is not a TCP/HTTP target.
    target_not_tcp,
    /// No target-to-socket mapping exists.
    target_mapping_missing,
    /// Parser failed to parse ss output (malformed).
    parse_failed,
    /// TCP diagnostics not supported on this platform.
    unsupported_platform,
};

fn wallClockMs() i64 {
    if (comptime @import("builtin").os.tag == .linux and
        @hasDecl(std.os.linux, "clock_gettime"))
    {
        var ts: std.os.linux.timespec = undefined;
        if (std.os.linux.clock_gettime(@enumFromInt(0), &ts) < 0) {
            return 0;
        }
        return ts.sec * 1000 + @divTrunc(ts.nsec, 1_000_000);
    }
    return 1718700000000;
}

/// Appends a TCP absence event to the events list.
/// 
/// The `detail` parameter must be a safe JSON-escapable token (no quotes, backslashes, or control chars).
/// Valid examples: "exit=127", "UnexpectedToken", "permission_denied".
/// If detail contains special JSON characters, escape them before calling this function.
pub fn appendTcpAbsenceEvent(
    allocator: std.mem.Allocator,
    events: *std.ArrayList(EventOutput),
    reason: TcpAbsenceReason,
    detail: ?[]const u8,
) !void {
    const ts_str = try std.fmt.allocPrint(allocator, "{d}", .{wallClockMs()});
    errdefer allocator.free(ts_str);

    const sev = switch (reason) {
        .command_failed, .permission_denied, .parse_failed, .unsupported_platform => "error",
        else => "warning",
    };
    const sev_owned = try allocator.dupe(u8, sev);
    errdefer allocator.free(sev_owned);

    const src = try allocator.dupe(u8, "underlay_tcp");
    errdefer allocator.free(src);

    const msg = switch (reason) {
        .no_matching_socket => try allocator.dupe(u8, "no matching socket found for filter"),
        .socket_closed_before_capture => try allocator.dupe(u8, "socket closed before capture completed"),
        .command_failed => try std.fmt.allocPrint(
            allocator,
            "ss command failed",
            .{},
        ),
        .not_configured => try allocator.dupe(u8, "underlay TCP diagnostics disabled by config"),
        .permission_denied => try allocator.dupe(u8, "permission denied for ss command"),
        .target_not_tcp => try allocator.dupe(u8, "target is not a TCP/HTTP target"),
        .target_mapping_missing => try allocator.dupe(u8, "no target-to-socket mapping exists"),
        .parse_failed => try allocator.dupe(u8, "failed to parse ss output"),
        .unsupported_platform => try allocator.dupe(u8, "TCP diagnostics not supported on this platform"),
    };
    errdefer allocator.free(msg);

    // Build fields JSON with reason and optional detail.
    var fields_json: []const u8 = undefined;
    if (detail) |d| {
        fields_json = try std.fmt.allocPrint(
            allocator,
            "{{\"reason\":\"{s}\",\"detail\":\"{s}\"}}",
            .{ @tagName(reason), d },
        );
    } else {
        fields_json = try std.fmt.allocPrint(
            allocator,
            "{{\"reason\":\"{s}\"}}",
            .{@tagName(reason)},
        );
    }
    errdefer allocator.free(fields_json);

    try events.append(allocator, .{
        .ts = ts_str,
        .severity = sev_owned,
        .source = src,
        .message = msg,
        .fields = fields_json,
    });
}
