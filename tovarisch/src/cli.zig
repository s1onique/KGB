const std = @import("std");
const status = @import("status.zig");

pub const ExitCode = enum(u8) {
    ok = 0,
    usage = 2,
};

pub fn run(args: []const []const u8, stdout: anytype, stderr: anytype) ExitCode {
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
        stdout.print("tovarisch {s}\n", .{status.version}) catch return .usage;
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

    status.renderPayload(stdout) catch return .usage;
    stdout.writeByte('\n') catch return .usage;

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

// --- Tests ---

test "version command returns ok" {
    var buf: [256]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const allocator = fba.allocator();

    var output = std.ArrayList(u8).init(allocator);
    const writer = output.writer();

    const code = run(&.{ "tovarisch", "--version" }, writer, writer);
    try std.testing.expectEqual(ExitCode.ok, code);
}

test "version command prints version" {
    var buf: [256]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const allocator = fba.allocator();

    var output = std.ArrayList(u8).init(allocator);
    const writer = output.writer();

    _ = run(&.{ "tovarisch", "--version" }, writer, writer);
    try std.testing.expect(std.mem.containsAtLeast(u8, output.items, 1, "tovarisch"));
    try std.testing.expect(std.mem.containsAtLeast(u8, output.items, 1, "0.1.1"));
}

test "check command returns ok" {
    var buf: [256]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const allocator = fba.allocator();

    var output = std.ArrayList(u8).init(allocator);
    const writer = output.writer();

    const code = run(&.{ "tovarisch", "check" }, writer, writer);
    try std.testing.expectEqual(ExitCode.ok, code);
}

test "check command prints ok" {
    var buf: [256]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const allocator = fba.allocator();

    var output = std.ArrayList(u8).init(allocator);
    const writer = output.writer();

    _ = run(&.{ "tovarisch", "check" }, writer, writer);
    try std.testing.expect(std.mem.containsAtLeast(u8, output.items, 1, "tovarisch check: ok"));
}

test "unknown command returns usage" {
    var buf: [256]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const allocator = fba.allocator();

    var err_output = std.ArrayList(u8).init(allocator);
    const err_writer = err_output.writer();

    const code = run(&.{ "tovarisch", "badcmd" }, err_writer, err_writer);
    try std.testing.expectEqual(ExitCode.usage, code);
}

test "no args returns usage" {
    var buf: [256]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const allocator = fba.allocator();

    var err_output = std.ArrayList(u8).init(allocator);
    const err_writer = err_output.writer();

    const code = run(&.{"tovarisch"}, err_writer, err_writer);
    try std.testing.expectEqual(ExitCode.usage, code);
}

test "status without --json returns usage" {
    var buf: [512]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const allocator = fba.allocator();

    var err_output = std.ArrayList(u8).init(allocator);
    const err_writer = err_output.writer();

    const code = run(&.{ "tovarisch", "status" }, err_writer, err_writer);
    try std.testing.expectEqual(ExitCode.usage, code);
}

test "status --json returns ok" {
    var buf: [512]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const allocator = fba.allocator();

    var output = std.ArrayList(u8).init(allocator);
    const writer = output.writer();

    const code = run(&.{ "tovarisch", "status", "--json" }, writer, writer);
    try std.testing.expectEqual(ExitCode.ok, code);
}

test "status --json contains expected fields" {
    var buf: [512]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const allocator = fba.allocator();

    var output = std.ArrayList(u8).init(allocator);
    const writer = output.writer();

    _ = run(&.{ "tovarisch", "status", "--json" }, writer, writer);
    try std.testing.expect(std.mem.containsAtLeast(u8, output.items, 1, "\"service\":\"tovarisch\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, output.items, 1, "\"version\":\"0.1.1\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, output.items, 1, "\"node_id\":\"local-dev\""));
}

test "status --json contains multiple checks" {
    var buf: [512]u8 = undefined;
    var fba = std.heap.FixedBufferAllocator.init(&buf);
    const allocator = fba.allocator();

    var output = std.ArrayList(u8).init(allocator);
    const writer = output.writer();

    _ = run(&.{ "tovarisch", "status", "--json" }, writer, writer);
    try std.testing.expect(std.mem.containsAtLeast(u8, output.items, 1, "\"name\":\"process\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, output.items, 1, "\"name\":\"binary\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, output.items, 1, "\"name\":\"config\""));
}
