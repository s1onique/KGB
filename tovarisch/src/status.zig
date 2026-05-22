const std = @import("std");

pub const version = "0.1.0";

pub const payload = "{\"service\":\"tovarisch\",\"version\":\"0.1.0\",\"node_id\":\"local-dev\",\"status\":\"ok\",\"checks\":[{\"name\":\"process\",\"status\":\"ok\",\"detail\":\"static bootstrap status\"}]}";

test "version constant is 0.1.0" {
    try std.testing.expectEqualStrings("0.1.0", version);
}

test "payload contains expected fields" {
    try std.testing.expect(std.mem.containsAtLeast(u8, payload, 1, "\"service\":\"tovarisch\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, payload, 1, "\"version\":\"0.1.0\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, payload, 1, "\"node_id\":\"local-dev\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, payload, 1, "\"status\":\"ok\""));
}
