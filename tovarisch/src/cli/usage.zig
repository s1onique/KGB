const std = @import("std");

/// Usage text for the tovarisch CLI.
/// The deprecated --listen-all flag is intentionally NOT included.
pub const usage_text =
    \\usage:
    \\  tovarisch --version
    \\  tovarisch check
    \\  tovarisch serve [--listen ADDR:PORT] [--listen-private] [--listen-all-public-dangerous] [--statonly] [--stats-interval SECONDS]
    \\  tovarisch status --json
    \\  tovarisch thread-smoke
    \\
;

const Self = @This();

/// Prints the usage text to the given writer.
pub fn printUsage(writer: anytype) !void {
    try writer.writeAll(usage_text);
}

// --- Tests ---

test "usage text contains tovarisch --version" {
    try std.testing.expect(std.mem.indexOf(u8, usage_text, "tovarisch --version") != null);
}

test "usage text contains tovarisch check" {
    try std.testing.expect(std.mem.indexOf(u8, usage_text, "tovarisch check") != null);
}

test "usage text contains tovarisch serve" {
    try std.testing.expect(std.mem.indexOf(u8, usage_text, "tovarisch serve") != null);
}

test "usage text contains --listen-all-public-dangerous" {
    try std.testing.expect(std.mem.indexOf(u8, usage_text, "--listen-all-public-dangerous") != null);
}

test "usage text does NOT contain deprecated --listen-all" {
    try std.testing.expect(std.mem.indexOf(u8, usage_text, "--listen-all ") == null);
}

test "usage text contains tovarisch status --json" {
    try std.testing.expect(std.mem.indexOf(u8, usage_text, "tovarisch status --json") != null);
}

test "usage text contains tovarisch thread-smoke" {
    try std.testing.expect(std.mem.indexOf(u8, usage_text, "tovarisch thread-smoke") != null);
}
