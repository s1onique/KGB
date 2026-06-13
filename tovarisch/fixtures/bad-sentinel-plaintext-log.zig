// bad-sentinel-plaintext-log.zig
//
// BAD SENTINEL: This file demonstrates the forbidden pattern.
// DO NOT COPY THIS CODE - std.log.* calls bypass structured logging.
//
// This file exists to verify the gate catches plain-text logging.
//
// FORBIDDEN: prose runtime logs using std.log
const std = @import("std");

// This is the forbidden pattern that the gate should catch:
pub fn forbiddenLogExample() void {
    // FORBIDDEN: plain-text logging
    std.log.info("plain text message here", .{});
    std.log.warn("another plain text message", .{});
    std.log.err("error occurred during processing", .{});
}
