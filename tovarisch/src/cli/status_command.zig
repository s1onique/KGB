// cli/status_command.zig — Status command implementation
//
// Extracted from commands.zig to satisfy LLM-friendliness line limits.

const std = @import("std");
const status = @import("../status.zig");
const command_model = @import("command_model.zig");

/// Execute the `status` command.
/// Returns ExitCode.ok on success, .usage on invalid arguments.
pub fn statusCommand(status_args: []const []const u8, stdout: anytype, stderr: anytype) command_model.ExitCode {
    if (status_args.len != 1 or !std.mem.eql(u8, status_args[0], "--json")) {
        stderr.writeAll("usage: tovarisch status --json\n") catch {};
        return .usage;
    }

    status.renderPayload(stdout) catch return .usage;
    stdout.writeByte('\n') catch return .usage;

    return .ok;
}

