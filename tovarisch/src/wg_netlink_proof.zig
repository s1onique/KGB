// wg_netlink_proof.zig — Direct GenericNetlinkBackend proof binary
//
// This binary provides a fail-closed runtime proof of GenericNetlinkBackend
// against a real WireGuard kernel interface.
//
// Usage:
//   wg_netlink_proof --interface wg-kgb0
//
// Output JSON:
//   {
//     "success": true/false,
//     "backend_kind": "generic_netlink",
//     "interface": "wg-kgb0",
//     "peer_count": 0,
//     "latest_handshake_epoch_sec": 0,
//     "rx_bytes": 0,
//     "tx_bytes": 0,
//     "listen_port": null,
//     "error": null,
//     "no_sensitive_data": true
//   }
//
// Exit codes:
//   0 - proof succeeded
//   1 - proof failed (interface missing, permission denied, etc.)
//   2 - unsupported platform (not Linux)
//
// NOT part of make gate - privileged Linux lab target.

const std = @import("std");
const Io = std.Io;
const wg = @import("net/wg_status_boundary.zig");
const netlink = @import("net/wg_status_boundary_netlink.zig");

/// Proof result emitted as JSON.
/// Note: "error" is a reserved keyword in Zig, escaped with @"
const ProofResult = struct {
    success: bool,
    backend_kind: []const u8,
    interface: ?[]const u8,
    peer_count: u32,
    latest_handshake_epoch_sec: u64,
    rx_bytes: u64,
    tx_bytes: u64,
    listen_port: ?u16,
    @"error": ?[]const u8,
    no_sensitive_data: bool,
};

/// Emit JSON proof result to stdout.
/// Uses manual JSON construction for simplicity and reliability.
fn emitProof(writer: anytype, result: ProofResult) !void {
    try writer.writeAll("{\n");
    try writer.print("  \"success\": {s},\n", .{if (result.success) "true" else "false"});
    try writer.print("  \"backend_kind\": \"{s}\",\n", .{result.backend_kind});

    if (result.interface) |iface| {
        try writer.print("  \"interface\": \"{s}\",\n", .{iface});
    } else {
        try writer.writeAll("  \"interface\": null,\n");
    }

    try writer.print("  \"peer_count\": {},\n", .{result.peer_count});
    try writer.print("  \"latest_handshake_epoch_sec\": {},\n", .{result.latest_handshake_epoch_sec});
    try writer.print("  \"rx_bytes\": {},\n", .{result.rx_bytes});
    try writer.print("  \"tx_bytes\": {},\n", .{result.tx_bytes});

    if (result.listen_port) |port| {
        try writer.print("  \"listen_port\": {},\n", .{port});
    } else {
        try writer.writeAll("  \"listen_port\": null,\n");
    }

    if (result.@"error") |err| {
        try writer.print("  \"error\": \"{s}\",\n", .{err});
    } else {
        try writer.writeAll("  \"error\": null,\n");
    }

    try writer.print("  \"no_sensitive_data\": {s}\n", .{if (result.no_sensitive_data) "true" else "false"});
    try writer.writeAll("}\n");
}

/// Map StatusError to error string.
fn mapErrorToString(err: anyerror) []const u8 {
    return switch (err) {
        error.unsupported_platform => "unsupported_platform",
        error.interface_missing => "interface_missing",
        error.backend_missing => "backend_missing",
        error.netlink_failed => "netlink_failed",
        error.timeout => "timeout",
        error.permission_denied => "permission_denied",
        error.out_of_memory => "out_of_memory",
        else => "unknown_error",
    };
}

/// Default interface name.
const DEFAULT_INTERFACE = "wg-kgb0";

pub fn main(init: std.process.Init) !void {
    const arena = init.arena.allocator();

    var stdout_buf: [1024]u8 = undefined;
    var stdout_file = Io.File.Writer.init(.stdout(), init.io, stdout_buf[0..]);
    const stdout = &stdout_file.interface;

    // Use arena for argument parsing
    const args = try init.minimal.args.toSlice(arena);

    // Parse interface name from args or use default
    var interface_name: []const u8 = DEFAULT_INTERFACE;

    var i: usize = 0;
    while (i < args.len) : (i += 1) {
        const arg = args[i];
        if (std.mem.eql(u8, arg, "--interface") or std.mem.eql(u8, arg, "-i")) {
            i += 1;
            if (i < args.len) {
                interface_name = args[i];
            }
        }
    }

    // Check platform
    if (@import("builtin").os.tag != .linux) {
        const result = ProofResult{
            .success = false,
            .backend_kind = "generic_netlink",
            .interface = null,
            .peer_count = 0,
            .latest_handshake_epoch_sec = 0,
            .rx_bytes = 0,
            .tx_bytes = 0,
            .listen_port = null,
            .@"error" = "unsupported_platform",
            .no_sensitive_data = true,
        };
        try emitProof(stdout, result);
        try stdout.flush();
        std.process.exit(2);
    }

    // Initialize the generic-netlink backend directly
    const backend = netlink.GenericNetlinkBackend.init();
    const trait = backend.asBackend();

    // Verify backend kind
    if (trait.backendKind() != wg.BackendKind.generic_netlink) {
        const result = ProofResult{
            .success = false,
            .backend_kind = @tagName(trait.backendKind()),
            .interface = null,
            .peer_count = 0,
            .latest_handshake_epoch_sec = 0,
            .rx_bytes = 0,
            .tx_bytes = 0,
            .listen_port = null,
            .@"error" = "wrong_backend_kind",
            .no_sensitive_data = true,
        };
        try emitProof(stdout, result);
        try stdout.flush();
        std.process.exit(1);
    }

    // Call wireguardStatus directly
    const status_result = trait.wireguardStatus(arena);

    const wg_result: wg.WireGuardStatusResult = status_result catch |err| {
        const proof_error = ProofResult{
            .success = false,
            .backend_kind = "generic_netlink",
            .interface = interface_name,
            .peer_count = 0,
            .latest_handshake_epoch_sec = 0,
            .rx_bytes = 0,
            .tx_bytes = 0,
            .listen_port = null,
            .@"error" = mapErrorToString(err),
            .no_sensitive_data = true,
        };
        try emitProof(stdout, proof_error);
        try stdout.flush();
        std.process.exit(1);
    };

    // Unwrap latest_handshake_epoch_sec: null means never handshaked, use 0
    const latest_handshake_epoch_sec = wg_result.status.latest_handshake_epoch_sec orelse 0;

    // Success - verify no sensitive data
    const no_sensitive = wg_result.status.public_key_redacted.len == 0;

    // Verify returned interface matches requested interface
    if (!std.mem.eql(u8, wg_result.status.interface, interface_name)) {
        const mismatch_result = ProofResult{
            .success = false,
            .backend_kind = "generic_netlink",
            .interface = wg_result.status.interface,
            .peer_count = wg_result.status.peer_count,
            .latest_handshake_epoch_sec = latest_handshake_epoch_sec,
            .rx_bytes = wg_result.status.rx_bytes,
            .tx_bytes = wg_result.status.tx_bytes,
            .listen_port = wg_result.status.listen_port,
            .@"error" = "interface_mismatch",
            .no_sensitive_data = true,
        };
        try emitProof(stdout, mismatch_result);
        try stdout.flush();
        std.process.exit(1);
    }

    const proof = ProofResult{
        .success = true,
        .backend_kind = "generic_netlink",
        .interface = wg_result.status.interface,
        .peer_count = wg_result.status.peer_count,
        .latest_handshake_epoch_sec = latest_handshake_epoch_sec,
        .rx_bytes = wg_result.status.rx_bytes,
        .tx_bytes = wg_result.status.tx_bytes,
        .listen_port = wg_result.status.listen_port,
        .@"error" = null,
        .no_sensitive_data = no_sensitive,
    };

    try emitProof(stdout, proof);
    try stdout.flush();
    std.process.exit(0);
}

test "ProofResult structure" {
    const result = ProofResult{
        .success = true,
        .backend_kind = "generic_netlink",
        .interface = "wg-kgb0",
        .peer_count = 0,
        .latest_handshake_epoch_sec = 0,
        .rx_bytes = 0,
        .tx_bytes = 0,
        .listen_port = null,
        .@"error" = null,
        .no_sensitive_data = true,
    };

    try std.testing.expect(result.success);
    try std.testing.expect(result.no_sensitive_data);
}
