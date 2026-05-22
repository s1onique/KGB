const std = @import("std");
const Io = std.Io;

const version = "0.1.0";

const ExitCode = enum(u8) {
    ok = 0,
    usage = 2,
};

pub fn main(init: std.process.Init) !void {
    const arena = init.arena.allocator();
    var stdout_buf: [1024]u8 = undefined;
    var stdout_file = Io.File.Writer.init(.stdout(), init.io, stdout_buf[0..]);
    const stdout = &stdout_file.interface;

    var stderr_buf: [1024]u8 = undefined;
    var stderr_file = Io.File.Writer.init(.stderr(), init.io, stderr_buf[0..]);
    const stderr = &stderr_file.interface;

    const args = try init.minimal.args.toSlice(arena);
    const exit_code = run(args, stdout, stderr);
    try stdout.flush();
    std.process.exit(@intFromEnum(exit_code));
}

fn run(args: []const []const u8, stdout: anytype, stderr: anytype) ExitCode {
    if (args.len == 1) {
        printUsage(stderr);
        return .usage;
    }

    const command = args[1];

    if (std.mem.eql(u8, command, "--help") or std.mem.eql(u8, command, "-h")) {
        printUsage(stdout);
        return .ok;
    }

    if (std.mem.eql(u8, command, "--version")) {
        stdout.print("tovarisch {s}\n", .{version}) catch return .usage;
        return .ok;
    }

    if (std.mem.eql(u8, command, "check")) {
        stdout.writeAll("tovarisch check: ok\n") catch return .usage;
        return .ok;
    }

    if (std.mem.eql(u8, command, "status")) {
        return statusCommand(args[2..], stdout, stderr);
    }

    stderr.print("unknown command: {s}\n\n", .{command}) catch {};
    printUsage(stderr);
    return .usage;
}

fn statusCommand(args: []const []const u8, stdout: anytype, stderr: anytype) ExitCode {
    if (args.len != 1 or !std.mem.eql(u8, args[0], "--json")) {
        stderr.writeAll("usage: tovarisch status --json\n") catch {};
        return .usage;
    }

    stdout.writeAll(
        \\{"service":"tovarisch","version":"0.1.0","node_id":"local-dev","status":"ok","checks":[{"name":"process","status":"ok","detail":"static bootstrap status"}]}
        \\
    ) catch return .usage;

    return .ok;
}

fn printUsage(writer: anytype) void {
    writer.writeAll(
        \\usage:
        \\  tovarisch --version
        \\  tovarisch check
        \\  tovarisch status --json
        \\
    ) catch {};
}
