// test_all.zig — Aggregate test root for tovarisch
//
// This file forces Zig's test discovery to find tests in all modules.
// Use this as the root_source_file for test steps to ensure every module's
// tests are included in the test binary.
//
// Pattern: import every module with tests, then refAllDecls to force linking.
const std = @import("std");

// Import all source modules to ensure they are compiled and their tests discovered
const _cli = @import("cli.zig");
const _status = @import("status.zig");

// Force test discovery for all imported modules
// This ensures the test binary actually runs the tests from these modules
test {
    std.testing.refAllDecls(@import("cli.zig"));
}

test {
    std.testing.refAllDecls(@import("status.zig"));
}