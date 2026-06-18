// route_diag.zig — Route diagnostics parser for `ip route get`
//
// ACT: Add tovarisch WireGuard and XRay TCP underlay diagnostics
// Parses route information from `ip route get` output.
//
// The `ip route get` output format for diagnostic purposes:
//
//   10.77.0.2 via 192.0.2.1 dev eth0 src 10.0.0.1 uid 0
//   10.77.0.2 dev wg-kgb0 src 10.77.0.1 uid 0
//
// Privacy constraints:
// - Target address is from configuration (known, not exposed)
// - Gateway/interface may be redacted if configured

const std = @import("std");

// ============================================================================
// Types
// ============================================================================

/// Route diagnostics result.
pub const RouteDiag = struct {
    /// Route target (from config, not parsed).
    target: []const u8,
    /// Selected interface/device.
    interface: []const u8,
    /// Source address.
    source: []const u8,
    /// Gateway (null if direct).
    gateway: ?[]const u8 = null,
    /// Route status.
    status: RouteStatus = .ok,
};

/// Route status.
pub const RouteStatus = enum {
    ok,
    warning,
    @"error",
    unavailable,
};

/// Parser errors.
pub const ParseError = error{
    NoData,
    MalformedOutput,
};

/// Configuration for parsing.
pub const ParseConfig = struct {
    /// Whether to redact interface name.
    redact_interface: bool = false,
    /// Whether to redact gateway.
    redact_gateway: bool = false,
};

// ============================================================================
// Parser
// ============================================================================

/// Parse `ip route get` output and extract route diagnostics.
///
/// Input format examples:
///   10.77.0.2 via 192.0.2.1 dev eth0 src 10.0.0.1 uid 0
///   10.77.0.2 dev wg-kgb0 src 10.77.0.1 uid 0
pub fn parseRouteGetOutput(
    allocator: std.mem.Allocator,
    target: []const u8,
    input: []const u8,
    config: ParseConfig,
) ParseError!RouteDiag {
    const trimmed = std.mem.trim(u8, input, " \t\r\n");
    if (trimmed.len == 0) return error.NoData;

    // Parse the output line
    // Format: <target> [via <gateway>] dev <interface> [src <source>] ...
    var interface: []const u8 = "";
    var source: []const u8 = "";
    var gateway: ?[]const u8 = null;

    var fields = std.mem.splitScalar(u8, trimmed, ' ');
    while (fields.next()) |field| {
        const f = std.mem.trim(u8, field, " \t");

        if (std.mem.eql(u8, f, "via")) {
            // Gateway follows
            if (fields.next()) |gw| {
                gateway = if (config.redact_gateway) (redactGateway(allocator, gw) catch null) else std.mem.trim(u8, gw, " \t");
            }
        } else if (std.mem.eql(u8, f, "dev")) {
            // Interface follows
            if (fields.next()) |dev| {
                interface = if (config.redact_interface) redactInterface(dev) else std.mem.trim(u8, dev, " \t");
            }
        } else if (std.mem.eql(u8, f, "src")) {
            // Source follows
            if (fields.next()) |src| {
                source = std.mem.trim(u8, src, " \t");
            }
        }
    }

    if (interface.len == 0) {
        return error.MalformedOutput;
    }

    return RouteDiag{
        .target = target,
        .interface = interface,
        .source = source,
        .gateway = gateway,
        .status = .ok,
    };
}

/// Redact interface name.
fn redactInterface(_: []const u8) []const u8 {
    // Return "redacted" for interface name
    return "redacted";
}

/// Redact gateway address - returns allocator-owned string.
fn redactGateway(allocator: std.mem.Allocator, gw: []const u8) ![]const u8 {
    // Find the gateway address and redact it
    // Format could be: 192.0.2.1
    const colon_idx = std.mem.indexOfScalar(u8, gw, ':');
    if (colon_idx != null) {
        // IPv6 or host:port
        const port_idx = std.mem.lastIndexOfScalar(u8, gw, ':');
        if (port_idx != null and port_idx.? > colon_idx.?) {
            // IPv6 with port: [::1]:443
            return std.fmt.allocPrint(allocator, "[redacted]:{s}", .{gw[port_idx.? + 1 ..]});
        }
        return std.fmt.allocPrint(allocator, "[redacted]{s}", .{gw[colon_idx.?..]});
    }
    return try allocator.dupe(u8, "redacted");
}

/// Validate that a target address is safe to use in command execution.
/// Returns true if the address appears to be a valid IP address.
pub fn validateTargetAddress(target: []const u8) bool {
    const trimmed = std.mem.trim(u8, target, " \t\r\n");

    // Check for CIDR notation first (e.g., 10.77.0.2/32)
    if (std.mem.containsAtLeast(u8, trimmed, 1, "/")) {
        const slash_idx = std.mem.indexOfScalar(u8, trimmed, '/').?;
        const addr = trimmed[0..slash_idx];
        const prefix_str = trimmed[slash_idx + 1..];
        const prefix = std.fmt.parseInt(u8, prefix_str, 10) catch return false;
        if (prefix > 32) return false;

        // Recursively validate the address portion
        return validateTargetAddress(addr);
    }

    // Count dots to detect IPv4
    var dot_count: usize = 0;
    for (trimmed) |c| {
        if (c == '.') dot_count += 1;
    }

    if (dot_count == 3) {
        // Likely IPv4, validate octets
        var octets: [4][]const u8 = undefined;
        var idx: usize = 0;
        var iter = std.mem.splitScalar(u8, trimmed, '.');
        while (iter.next()) |octet| {
            if (idx >= 4) break;
            octets[idx] = octet;
            idx += 1;
        }
        if (idx != 4) return false;
        for (octets) |octet| {
            _ = std.fmt.parseInt(u8, octet, 10) catch return false;
        }
        return true;
    }

    // If not obviously valid, reject
    return false;
}

// ============================================================================
// Tests
// ============================================================================

test "parseRouteGetOutput parses direct route" {
    const allocator = std.testing.allocator;
    const input = "10.77.0.2 dev wg-kgb0 src 10.77.0.1 uid 0";
    const result = try parseRouteGetOutput(allocator, "10.77.0.2", input, .{});
    try std.testing.expectEqualStrings("wg-kgb0", result.interface);
    try std.testing.expectEqualStrings("10.77.0.1", result.source);
    try std.testing.expect(result.gateway == null);
}

test "parseRouteGetOutput parses routed via gateway" {
    const allocator = std.testing.allocator;
    const input = "10.77.0.2 via 192.0.2.1 dev eth0 src 10.0.0.1 uid 0";
    const result = try parseRouteGetOutput(allocator, "10.77.0.2", input, .{});
    try std.testing.expectEqualStrings("eth0", result.interface);
    try std.testing.expect(result.gateway != null);
    try std.testing.expectEqualStrings("192.0.2.1", result.gateway.?);
}

test "parseRouteGetOutput returns error for empty input" {
    try std.testing.expectError(error.NoData, parseRouteGetOutput(std.testing.allocator, "10.0.0.1", "", .{}));
}

test "parseRouteGetOutput returns error when interface missing" {
    try std.testing.expectError(error.MalformedOutput, parseRouteGetOutput(std.testing.allocator, "10.0.0.1", "10.0.0.1 via 192.0.2.1", .{}));
}

test "redactInterface returns redacted" {
    const result = redactInterface("wg-kgb0");
    try std.testing.expectEqualStrings("redacted", result);
}

test "validateTargetAddress accepts valid IPv4" {
    try std.testing.expect(validateTargetAddress("10.77.0.2"));
    try std.testing.expect(validateTargetAddress("192.0.2.1"));
    try std.testing.expect(validateTargetAddress("10.77.0.2/32"));
}

test "validateTargetAddress rejects invalid addresses" {
    try std.testing.expect(!validateTargetAddress("; rm -rf /"));
    try std.testing.expect(!validateTargetAddress("$(whoami)"));
    try std.testing.expect(!validateTargetAddress(""));
}
