// status_network_diag_wiring_tests.zig — Regression tests for TCP diag config wiring
//
// ACT: Wire parsed tovarisch network diagnostics config into HTTP status path
//
// These tests prove that:
// 1. Disabled/default config does NOT run underlay TCP commands
// 2. Enabled config honors the parsed network_diag config
// 3. Commands-disabled config still does not execute TCP diagnostic commands
// 4. The HTTP/status renderer path uses parsed config, not hardcoded config
//
// Bug being tested: Previously status.zig hardcoded:
//   const diag_cfg = network_diag_config.NetworkDiagConfig{ .enabled = true };
// This bypassed the parsed daemon config, causing false "disabled by config" messages.

const std = @import("std");
const testing = std.testing;
const status = @import("status.zig");
const network_diag_config = @import("net/network_diag_config.zig");
const status_network_diag = @import("status_network_diag.zig");

// ============================================================================
// Regression Test 1: Disabled config should not run TCP commands
// ============================================================================

test "REGRESSION: disabled network_diag config does NOT force TCP collection" {
    // This test proves the fix works: when config is disabled,
    // the status path should use disabled config, not force-enabled config.
    const allocator = testing.allocator;

    // Simulate the scenario where config is disabled (default)
    const disabled_cfg = network_diag_config.NetworkDiagConfig{
        .enabled = false,
        .underlay_tcp = .{
            .enabled = false,
            .commands_enabled = false,
        },
    };

    // Collect with disabled config
    var diag = try status_network_diag.collectNetworkDiag(allocator, disabled_cfg);
    defer diag.deinit(allocator);

    // When diagnostics are fully disabled, no events should be generated
    // The bug was that status.zig would use { .enabled = true } instead of the
    // parsed config, causing false "disabled by config" events to be emitted.
    try testing.expectEqualSlices(status_network_diag.EventOutput, &.{}, diag.events);
}

// ============================================================================
// Regression Test 2: Enabled config honors underlay_tcp.enabled
// ============================================================================

test "REGRESSION: enabled config with TCP enabled uses parsed config" {
    const allocator = testing.allocator;

    // Simulate a config where network diagnostics and TCP are enabled
    const enabled_cfg = network_diag_config.NetworkDiagConfig{
        .enabled = true,
        .underlay_tcp = .{
            .enabled = true,
            .commands_enabled = false, // commands still disabled
        },
    };

    var diag = try status_network_diag.collectNetworkDiag(allocator, enabled_cfg);
    defer diag.deinit(allocator);

    // With underlay_tcp.enabled = true but commands_enabled = false,
    // we expect a not_configured event (because commands are not enabled)
    // NOT the previous bug behavior where everything was forced on.
    try testing.expect(diag.events.len == 1);
    try testing.expectEqualStrings("underlay_tcp", diag.events[0].source);
    try testing.expectEqualStrings("warning", diag.events[0].severity);
    try testing.expect(std.mem.containsAtLeast(u8, diag.events[0].fields.?, 1, "not_configured"));
}

// ============================================================================
// Regression Test 3: Commands-disabled config does not execute commands
// ============================================================================

test "REGRESSION: TCP enabled but commands disabled produces not_configured event" {
    const allocator = testing.allocator;

    // TCP is enabled but commands are disabled
    const tcp_no_commands_cfg = network_diag_config.NetworkDiagConfig{
        .enabled = true,
        .underlay_tcp = .{
            .enabled = true,
            .commands_enabled = false,
        },
    };

    var diag = try status_network_diag.collectNetworkDiag(allocator, tcp_no_commands_cfg);
    defer diag.deinit(allocator);

    // Should emit not_configured event, not run commands
    try testing.expect(diag.events.len == 1);
    try testing.expectEqualStrings("underlay_tcp", diag.events[0].source);
    try testing.expect(std.mem.containsAtLeast(u8, diag.events[0].fields.?, 1, "not_configured"));
}

// ============================================================================
// Regression Test 4: renderPayloadWithContextAndDiag uses parsed config
// ============================================================================

test "REGRESSION: renderPayloadWithContextAndDiag honors RuntimeStatusInputs.network_diag_config" {
    const allocator = testing.allocator;

    // Create inputs with disabled network_diag config
    const inputs = status.RuntimeStatusInputs{
        .network_diag_config = network_diag_config.NetworkDiagConfig{
            .enabled = false,
            .underlay_tcp = .{ .enabled = false },
        },
    };

    // Render to a buffer
    var buf: [4096]u8 = undefined;
    var len: usize = 0;

    const writer = struct {
        buf: *[4096]u8,
        len: *usize,

        pub fn writeAll(self: @This(), bytes: []const u8) !void {
            @memcpy(self.buf[self.len.*..][0..bytes.len], bytes);
            self.len.* += bytes.len;
        }

        pub fn print(self: @This(), comptime fmt: []const u8, args: anytype) !void {
            const written = std.fmt.bufPrint(self.buf[self.len.*..], fmt, args) catch return error.BufferOverflow;
            self.len.* += written.len;
        }

        pub fn writeByte(self: @This(), c: u8) !void {
            if (self.len.* >= 4096) return error.BufferOverflow;
            self.buf[self.len.*] = c;
            self.len.* += 1;
        }
    }{ .buf = &buf, .len = &len };

    // Render with include_network_diag = true
    // The key test: even with include_network_diag = true,
    // if the config has .enabled = false, no events should be collected
    try status.renderPayloadWithContextAndDiag(writer, inputs, allocator, true);

    const json = buf[0..len];

    // With disabled config, the network_diag section should show empty events
    // (not a bug where it forces enabled=true regardless of config)
    try testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"network_diag\":"));
    try testing.expect(std.mem.containsAtLeast(u8, json, 1, "\"events\":[]"));
}

// ============================================================================
// Test 5: Verify RuntimeStatusInputs.network_diag_config field exists and works
// ============================================================================

test "RuntimeStatusInputs has network_diag_config field with correct default" {
    const inputs = status.RuntimeStatusInputs{};

    // Default should be all disabled (matching struct defaults)
    try testing.expect(!inputs.network_diag_config.enabled);
    try testing.expect(!inputs.network_diag_config.underlay_tcp.enabled);
    try testing.expect(!inputs.network_diag_config.underlay_tcp.commands_enabled);
}

test "RuntimeStatusInputs.network_diag_config can be set to enabled state" {
    const inputs = status.RuntimeStatusInputs{
        .network_diag_config = network_diag_config.NetworkDiagConfig{
            .enabled = true,
            .underlay_tcp = .{
                .enabled = true,
                .commands_enabled = true,
            },
        },
    };

    try testing.expect(inputs.network_diag_config.enabled);
    try testing.expect(inputs.network_diag_config.underlay_tcp.enabled);
    try testing.expect(inputs.network_diag_config.underlay_tcp.commands_enabled);
}
