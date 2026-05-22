const std = @import("std");
const Io = std.Io;
const cli = @import("cli.zig");

pub fn main(init: std.process.Init) !void {
    const arena = init.arena.allocator();

    var stdout_buf: [1024]u8 = undefined;
    var stdout_file = Io.File.Writer.init(.stdout(), init.io, stdout_buf[0..]);
    const stdout = &stdout_file.interface;

    var stderr_buf: [1024]u8 = undefined;
    var stderr_file = Io.File.Writer.init(.stderr(), init.io, stderr_buf[0..]);
    const stderr = &stderr_file.interface;

    // Use arena for argument parsing
    const args = try init.minimal.args.toSlice(arena);

    // Use arena for potential future allocations (e.g., error messages)
    // Currently we pre-allocate buffers, but this keeps arena ownership explicit.
    const exit_code = cli.run(args, stdout, stderr);

    try stdout.flush();
    std.process.exit(@intFromEnum(exit_code));
}
