// status_network_diag.zig — Network diagnostics status integration (facade)
const std = @import("std");
const network_diag_config = @import("net/network_diag_config.zig");
const extended_interface_stats = @import("net/extended_interface_stats.zig");
const safe_command = @import("net/safe_command.zig");

pub const NetworkDiagStatus = @import("status_network_diag_types.zig").NetworkDiagStatus;
pub const InterfaceOutput = @import("status_network_diag_types.zig").InterfaceOutput;
pub const TcpSocketOutput = @import("status_network_diag_types.zig").TcpSocketOutput;
pub const EventOutput = @import("status_network_diag_types.zig").EventOutput;
pub const RouteOutput = @import("status_network_diag_types.zig").RouteOutput;
pub const WireguardPeerOutput = @import("status_network_diag_types.zig").WireguardPeerOutput;
pub const WireguardInterfaceOutput = @import("status_network_diag_types.zig").WireguardInterfaceOutput;
pub const NetworkDiag = @import("status_network_diag_types.zig").NetworkDiag;

pub const TcpAbsenceReason = @import("status_network_diag_events.zig").TcpAbsenceReason;
pub const appendTcpAbsenceEvent = @import("status_network_diag_events.zig").appendTcpAbsenceEvent;

const parseSockets = @import("status_network_diag_tcp.zig").parseSockets;

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

pub fn formatTimestamp(allocator: std.mem.Allocator, ts: i64) ![]const u8 {
    return std.fmt.allocPrint(allocator, "{d}", .{ts});
}

/// Collects network diagnostics data based on the provided configuration.
///
/// Key behaviors:
/// - If network diagnostics are fully disabled (cfg.enabled=false), returns early
///   with status=disabled and no events.
/// - If underlay_tcp is enabled but commands are not, emits not_configured event.
/// - If ss command fails (non-zero exit), emits command_failed and skips parsing.
/// - Parser failures propagate as parse_failed events (not silently swallowed).
/// - If no sockets match the filter, emits no_matching_socket event.
pub fn collectNetworkDiag(
    allocator: std.mem.Allocator,
    cfg: network_diag_config.NetworkDiagConfig,
) !NetworkDiag {
    // Full disabled path: no events expected, no TCP reason needed.
    if (!cfg.enabled) {
        return NetworkDiag{
            .started_at = try formatTimestamp(allocator, wallClockMs()),
            .status = .disabled,
            .wireguard = null,
            .interfaces = &.{},
            .routes = &.{},
            .underlay_tcp = &.{},
            .events = &.{},
        };
    }

    var interfaces: std.ArrayList(InterfaceOutput) = .empty;
    defer interfaces.deinit(allocator);

    var routes: std.ArrayList(RouteOutput) = .empty;
    defer routes.deinit(allocator);

    var underlay_tcp: std.ArrayList(TcpSocketOutput) = .empty;
    defer underlay_tcp.deinit(allocator);

    var events: std.ArrayList(EventOutput) = .empty;
    defer events.deinit(allocator);

    var overall_status: NetworkDiagStatus = .ok;

    // Collect WireGuard interface stats if enabled.
    if (cfg.wireguard.enabled) {
        for (cfg.wireguard.interfaces) |iface_name| {
            const stats = extended_interface_stats.readExtendedInterfaceStats(
                allocator,
                "/sys/class/net",
                iface_name,
            ) catch continue;
            defer allocator.free(stats.operstate);

            const name_str = try allocator.dupe(u8, stats.name);
            errdefer allocator.free(name_str);

            const operstate_str = try allocator.dupe(u8, stats.operstate);
            errdefer allocator.free(operstate_str);

            try interfaces.append(allocator, .{
                .name = name_str,
                .operstate = operstate_str,
                .carrier = stats.carrier,
                .rx_bytes = stats.basic.rx_bytes,
                .tx_bytes = stats.basic.tx_bytes,
                .rx_packets = stats.basic.rx_packets,
                .tx_packets = stats.basic.tx_packets,
                .rx_errors = stats.errors.rx_errors,
                .tx_errors = stats.errors.tx_errors,
                .rx_dropped = stats.errors.rx_dropped,
                .tx_dropped = stats.errors.tx_dropped,
                .rx_errors_delta = if (stats.deltas) |d| d.rx_errors_delta else 0,
                .tx_errors_delta = if (stats.deltas) |d| d.tx_errors_delta else 0,
                .rx_dropped_delta = if (stats.deltas) |d| d.rx_dropped_delta else 0,
                .tx_dropped_delta = if (stats.deltas) |d| d.tx_dropped_delta else 0,
            });
        }
    }

    // Handle underlay TCP diagnostics.
    if (cfg.underlay_tcp.enabled) {
        if (cfg.underlay_tcp.commands_enabled) {
            // Run ss command.
            const result = safe_command.runSsTin(allocator, .{}) catch |err| {
                overall_status = .unavailable;
                try appendTcpAbsenceEvent(
                    allocator,
                    &events,
                    .command_failed,
                    @errorName(err),
                );
                return NetworkDiag{
                    .started_at = try formatTimestamp(allocator, wallClockMs()),
                    .status = overall_status,
                    .wireguard = null,
                    .interfaces = try interfaces.toOwnedSlice(allocator),
                    .routes = try routes.toOwnedSlice(allocator),
                    .underlay_tcp = &.{},
                    .events = try events.toOwnedSlice(allocator),
                };
            };
            defer allocator.free(result.stdout);
            defer allocator.free(result.stderr);

            // Command failed: emit event and stop (don't parse potentially bad output).
            if (result.exit_code != 0) {
                overall_status = .unavailable;
                const detail = try std.fmt.allocPrint(
                    allocator,
                    "exit={d}",
                    .{result.exit_code},
                );
                defer allocator.free(detail);
                try appendTcpAbsenceEvent(
                    allocator,
                    &events,
                    .command_failed,
                    detail,
                );
                return NetworkDiag{
                    .started_at = try formatTimestamp(allocator, wallClockMs()),
                    .status = overall_status,
                    .wireguard = null,
                    .interfaces = try interfaces.toOwnedSlice(allocator),
                    .routes = try routes.toOwnedSlice(allocator),
                    .underlay_tcp = &.{},
                    .events = try events.toOwnedSlice(allocator),
                };
            }

            // Command succeeded: parse output.
            const port = if (cfg.underlay_tcp.remote_ports.len > 0)
                cfg.underlay_tcp.remote_ports[0]
            else
                0;

            _ = parseSockets(
                allocator,
                &underlay_tcp,
                result.stdout,
                cfg.underlay_tcp.redact_addresses,
                port,
            ) catch |err| {
                // Parser failure is a truthfulness bug, not "no matching socket".
                try appendTcpAbsenceEvent(
                    allocator,
                    &events,
                    .parse_failed,
                    @errorName(err),
                );
                return NetworkDiag{
                    .started_at = try formatTimestamp(allocator, wallClockMs()),
                    .status = .unavailable,
                    .wireguard = null,
                    .interfaces = try interfaces.toOwnedSlice(allocator),
                    .routes = try routes.toOwnedSlice(allocator),
                    .underlay_tcp = &.{},
                    .events = try events.toOwnedSlice(allocator),
                };
            };

            // Check if we got any sockets.
            if (underlay_tcp.items.len == 0) {
                try appendTcpAbsenceEvent(
                    allocator,
                    &events,
                    .no_matching_socket,
                    null,
                );
            }
        } else {
            // Commands disabled but underlay_tcp enabled.
            try appendTcpAbsenceEvent(
                allocator,
                &events,
                .not_configured,
                null,
            );
            overall_status = .unavailable;
        }
    } else {
        // Underlay TCP not enabled.
        try appendTcpAbsenceEvent(
            allocator,
            &events,
            .not_configured,
            null,
        );
        overall_status = .disabled;
    }

    // Set warning if we expected WireGuard stats but got none.
    if (interfaces.items.len == 0 and cfg.wireguard.enabled) {
        overall_status = .warning;
    }

    return NetworkDiag{
        .started_at = try formatTimestamp(allocator, wallClockMs()),
        .status = overall_status,
        .wireguard = null,
        .interfaces = try interfaces.toOwnedSlice(allocator),
        .routes = try routes.toOwnedSlice(allocator),
        .underlay_tcp = try underlay_tcp.toOwnedSlice(allocator),
        .events = try events.toOwnedSlice(allocator),
    };
}

pub const renderNetworkDiag = @import("net/network_diag_json.zig").renderNetworkDiag;
