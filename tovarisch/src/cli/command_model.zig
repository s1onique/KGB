// cli/command_model.zig — Shared CLI contracts and types
//
// Extracted from commands.zig to satisfy LLM-friendliness line limits.

const std = @import("std");

/// CLI exit codes.
pub const ExitCode = enum(u8) {
    ok = 0,
    usage = 2,
    serve_error = 3,
};

/// Parse a listen address string like "10.149.149.1:8317" into host and port.
/// Returns the parsed host string and port number.
/// Does NOT validate whether the address is safe to bind.
pub fn parseListenAddr(listen: []const u8) ?struct { host: []const u8, port: u16 } {
    const colon_idx = std.mem.indexOfScalar(u8, listen, ':') orelse return null;
    const host = listen[0..colon_idx];
    const port_str = listen[colon_idx + 1 ..];
    const port = std.fmt.parseInt(u16, port_str, 10) catch return null;
    return .{ .host = host, .port = port };
}

