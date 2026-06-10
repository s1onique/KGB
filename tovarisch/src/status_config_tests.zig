// status_config_tests.zig — Unit tests for ConfigCheckState and runtime status inputs.
//
// Tests the config check state derivation, buildConfigCheck(), RuntimeStatusInputs,
// buildStatusWithInputs(), and renderPayloadWithContext().

const std = @import("std");
const status = @import("status.zig");
const bfd_status = @import("bfd/status.zig");

// --- ConfigCheckState Tests ---

test "buildConfigCheck returns warn for no_config state" {
    const check = status.buildConfigCheck(.no_config);
    try std.testing.expectEqualStrings("config", check.name);
    try std.testing.expectEqual(status.CheckStatus.warn, check.status);
    try std.testing.expectEqualStrings("no config provided, using defaults", check.detail);
}

test "buildConfigCheck returns ok for loaded state" {
    const check = status.buildConfigCheck(.{ .loaded = .{ .path = "/etc/kgb/tovarisch.conf" } });
    try std.testing.expectEqualStrings("config", check.name);
    try std.testing.expectEqual(status.CheckStatus.ok, check.status);
    try std.testing.expectEqualStrings("/etc/kgb/tovarisch.conf", check.detail);
}

test "buildConfigCheck uses path as detail for loaded state" {
    const path = "/custom/path/to/config.toml";
    const check = status.buildConfigCheck(.{ .loaded = .{ .path = path } });
    try std.testing.expectEqual(status.CheckStatus.ok, check.status);
    try std.testing.expectEqualStrings(path, check.detail);
}

// --- ConfigCheckState union equality tests ---

test "ConfigCheckState.no_config is the no_config variant" {
    const state: status.ConfigCheckState = .no_config;
    try std.testing.expect(state == .no_config);
}

test "ConfigCheckState.loaded contains path" {
    const path = "/etc/tovarisch.conf";
    const state: status.ConfigCheckState = .{ .loaded = .{ .path = path } };
    try std.testing.expect(state == .loaded);
    try std.testing.expectEqualStrings(path, state.loaded.path);
}

// --- RuntimeStatusInputs Tests ---

test "RuntimeStatusInputs defaults to null bfd and no_config" {
    const inputs: status.RuntimeStatusInputs = .{};
    try std.testing.expect(inputs.bfd_runtime == null);
    try std.testing.expect(inputs.config_check == .no_config);
}

test "RuntimeStatusInputs can be constructed with loaded config" {
    const inputs: status.RuntimeStatusInputs = .{
        .bfd_runtime = null,
        .config_check = .{ .loaded = .{ .path = "/etc/kgb/tovarisch.conf" } },
    };
    try std.testing.expect(inputs.config_check == .loaded);
    try std.testing.expectEqualStrings("/etc/kgb/tovarisch.conf", inputs.config_check.loaded.path);
}

// --- buildStatusWithInputs Tests ---

test "buildStatusWithInputs uses no_config check by default" {
    const s = status.buildStatusWithInputs(.{});
    var found_config = false;
    for (s.checks) |check| {
        if (std.mem.eql(u8, check.name, "config")) {
            found_config = true;
            try std.testing.expectEqual(status.CheckStatus.warn, check.status);
            try std.testing.expectEqualStrings("no config provided, using defaults", check.detail);
        }
    }
    try std.testing.expect(found_config);
}

test "buildStatusWithInputs uses loaded config check when provided" {
    const config_path = "/etc/kgb/tovarisch.conf";
    const s = status.buildStatusWithInputs(.{
        .config_check = .{ .loaded = .{ .path = config_path } },
    });
    var found_config = false;
    for (s.checks) |check| {
        if (std.mem.eql(u8, check.name, "config")) {
            found_config = true;
            try std.testing.expectEqual(status.CheckStatus.ok, check.status);
            try std.testing.expectEqualStrings(config_path, check.detail);
        }
    }
    try std.testing.expect(found_config);
}

// --- renderPayloadWithContext Tests ---

test "renderPayloadWithContext with no_config shows warn status" {
    var buf: [4096]u8 = undefined;
    var len: usize = 0;

    const writer = struct {
        buf: *[4096]u8,
        len: *usize,

        pub fn print(self: @This(), comptime fmt: []const u8, args: anytype) !void {
            if (self.len.* >= 4096) return error.BufferOverflow;
            const written = std.fmt.bufPrint(self.buf[self.len.*..], fmt, args) catch return error.BufferOverflow;
            self.len.* += written.len;
        }

        pub fn writeAll(self: @This(), bytes: []const u8) !void {
            if (self.len.* + bytes.len > 4096) return error.BufferOverflow;
            @memcpy(self.buf[self.len.*..][0..bytes.len], bytes);
            self.len.* += bytes.len;
        }
    }{ .buf = &buf, .len = &len };

    try status.renderPayloadWithContext(writer, .{});
    const json = buf[0..len];

    // Config check should show warn with no config message
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"name\":\"config\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"status\":\"warn\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "no config provided"));
}

test "renderPayloadWithContext with loaded config shows ok status" {
    var buf: [4096]u8 = undefined;
    var len: usize = 0;

    const writer = struct {
        buf: *[4096]u8,
        len: *usize,

        pub fn print(self: @This(), comptime fmt: []const u8, args: anytype) !void {
            if (self.len.* >= 4096) return error.BufferOverflow;
            const written = std.fmt.bufPrint(self.buf[self.len.*..], fmt, args) catch return error.BufferOverflow;
            self.len.* += written.len;
        }

        pub fn writeAll(self: @This(), bytes: []const u8) !void {
            if (self.len.* + bytes.len > 4096) return error.BufferOverflow;
            @memcpy(self.buf[self.len.*..][0..bytes.len], bytes);
            self.len.* += bytes.len;
        }
    }{ .buf = &buf, .len = &len };

    const config_path = "/etc/kgb/tovarisch.conf";
    try status.renderPayloadWithContext(writer, .{
        .config_check = .{ .loaded = .{ .path = config_path } },
    });
    const json = buf[0..len];

    // Config check should show ok with path as detail
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"name\":\"config\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"status\":\"ok\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, config_path));
}

test "renderPayloadWithContext with BFD and config produces combined output" {
    var buf: [4096]u8 = undefined;
    var len: usize = 0;

    const writer = struct {
        buf: *[4096]u8,
        len: *usize,

        pub fn print(self: @This(), comptime fmt: []const u8, args: anytype) !void {
            if (self.len.* >= 4096) return error.BufferOverflow;
            const written = std.fmt.bufPrint(self.buf[self.len.*..], fmt, args) catch return error.BufferOverflow;
            self.len.* += written.len;
        }

        pub fn writeAll(self: @This(), bytes: []const u8) !void {
            if (self.len.* + bytes.len > 4096) return error.BufferOverflow;
            @memcpy(self.buf[self.len.*..][0..bytes.len], bytes);
            self.len.* += bytes.len;
        }
    }{ .buf = &buf, .len = &len };

    // Create a test BFD runtime
    var runtime = bfd_status.createTestRuntime();
    try bfd_status.addTestPeer(&runtime, "10.0.0.1", "10.0.0.2");
    runtime.startAll();

    const config_path = "/etc/kgb/tovarisch.conf";
    try status.renderPayloadWithContext(writer, .{
        .bfd_runtime = &runtime,
        .config_check = .{ .loaded = .{ .path = config_path } },
    });
    const json = buf[0..len];

    // Both BFD and config should be reflected in output
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"name\":\"bfd\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"name\":\"config\""));
    try std.testing.expect(std.mem.containsAtLeast(u8, json, 1, config_path));
}
