// status_bgp_error_tests.zig — Concrete error preservation tests
//
// Tests verify that concrete session errors (e.g., "send: EBADF") are
// preserved in status derivation, not replaced with generic "IoError".

const std = @import("std");
const bgp_status = @import("bgp/status.zig");

test "deriveStatusStateFromBundle returns runtime_failed when session state is failed" {
    var scratch: [64]u8 = undefined;
    const state = bgp_status.BgpStatusState{
        .runtime_failed = .{ .message = "send: EBADF" },
    };
    const check = bgp_status.buildBgpCheckInto(state, &scratch);
    try std.testing.expect(check.status == .@"error");
    try std.testing.expectEqualStrings("send: EBADF", check.detail);
}

test "buildBgpCheckInto renders concrete send error, not IoError" {
    var scratch: [64]u8 = undefined;

    const check1 = bgp_status.buildBgpCheckInto(.{ .runtime_failed = .{ .message = "send: EBADF" } }, &scratch);
    try std.testing.expect(check1.status == .@"error");
    try std.testing.expectEqualStrings("send: EBADF", check1.detail);
    try std.testing.expect(!std.mem.eql(u8, check1.detail, "IoError"));

    const check2 = bgp_status.buildBgpCheckInto(.{ .runtime_failed = .{ .message = "send: ECONNRESET" } }, &scratch);
    try std.testing.expect(check2.status == .@"error");
    try std.testing.expectEqualStrings("send: ECONNRESET", check2.detail);
    try std.testing.expect(!std.mem.eql(u8, check2.detail, "IoError"));

    const check3 = bgp_status.buildBgpCheckInto(.{ .runtime_failed = .{ .message = "send: EAGAIN/EWOULDBLOCK" } }, &scratch);
    try std.testing.expect(check3.status == .@"error");
    try std.testing.expectEqualStrings("send: EAGAIN/EWOULDBLOCK", check3.detail);
    try std.testing.expect(!std.mem.eql(u8, check3.detail, "IoError"));
}

test "runtime_failed is distinguishable from failed in status output" {
    var scratch: [64]u8 = undefined;

    const check1 = bgp_status.buildBgpCheckInto(.{ .failed = .{ .message = "invalid AS number" } }, &scratch);
    try std.testing.expect(check1.status == .@"error");
    try std.testing.expectEqualStrings("invalid AS number", check1.detail);

    const check2 = bgp_status.buildBgpCheckInto(.{ .runtime_failed = .{ .message = "send: EBADF" } }, &scratch);
    try std.testing.expect(check2.status == .@"error");
    try std.testing.expectEqualStrings("send: EBADF", check2.detail);

    try std.testing.expect(check1.status == check2.status);
    try std.testing.expect(!std.mem.eql(u8, check1.detail, check2.detail));
}
