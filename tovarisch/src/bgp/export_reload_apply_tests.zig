// bgp/export_reload_apply_tests.zig — Core tests for prefix reload with delta application

const std = @import("std");
const types = @import("types.zig");
const export_reload_apply = @import("export_reload_apply.zig");

test "ExportState init sets allocator" {
    var export_state = export_reload_apply.ExportState{};
    try std.testing.expect(export_state.allocator == null);
    export_state.init(std.testing.allocator);
    try std.testing.expect(export_state.allocator != null);
    try std.testing.expect(export_state.current_exported_prefixes.len == 0);
    try std.testing.expect(!export_state.last_reload_success);
}

test "ExportState exportedCount returns correct count" {
    var export_state = export_reload_apply.ExportState{};
    export_state.init(std.testing.allocator);
    try std.testing.expectEqual(@as(usize, 0), export_state.exportedCount());
    export_state.current_exported_prefixes = try std.testing.allocator.alloc(types.Ipv4Prefix, 3);
    export_state.current_exported_prefixes[0] = types.Ipv4Prefix.init("10.0.0.0/8");
    export_state.current_exported_prefixes[1] = types.Ipv4Prefix.init("172.16.0.0/12");
    export_state.current_exported_prefixes[2] = types.Ipv4Prefix.init("192.168.0.0/16");
    try std.testing.expectEqual(@as(usize, 3), export_state.exportedCount());
    std.testing.allocator.free(export_state.current_exported_prefixes);
}

test "ExportState deinit frees owned prefixes" {
    var export_state = export_reload_apply.ExportState{};
    export_state.init(std.testing.allocator);
    export_state.current_exported_prefixes = try std.testing.allocator.alloc(types.Ipv4Prefix, 2);
    export_state.current_exported_prefixes[0] = types.Ipv4Prefix.init("10.0.0.0/8");
    export_state.current_exported_prefixes[1] = types.Ipv4Prefix.init("172.16.0.0/12");
    export_state.deinit();
    try std.testing.expect(export_state.current_exported_prefixes.len == 0);
}

test "reloadAndApply: initExportedPrefixes sets initial state correctly" {
    var export_state = export_reload_apply.ExportState{};
    export_state.init(std.testing.allocator);
    defer export_state.deinit();
    try std.testing.expectEqual(@as(usize, 0), export_state.exportedCount());
    try std.testing.expect(!export_state.last_reload_success);

    // Create slice using .empty pattern and append
    var prefixes = std.ArrayList(types.Ipv4Prefix).empty;
    try prefixes.append(std.testing.allocator, types.Ipv4Prefix.init("10.0.0.0/8"));
    try prefixes.append(std.testing.allocator, types.Ipv4Prefix.init("172.16.0.0/12"));
    defer prefixes.deinit(std.testing.allocator);

    export_reload_apply.initExportedPrefixes(&export_state, prefixes.items);
    try std.testing.expectEqual(@as(usize, 2), export_state.exportedCount());
    try std.testing.expect(export_state.last_reload_success);
}

test "ReloadApplyResult tracks all fields" {
    const result = export_reload_apply.ReloadApplyResult{
        .reload_success = true,
        .reload_error = null,
        .current_prefix_count = 3,
        .delta_added_count = 1,
        .delta_removed_count = 1,
        .delta_unchanged_count = 1,
        .withdrawals_sent = 1,
        .announcements_sent = 1,
        .apply_error = null,
    };
    try std.testing.expect(result.reload_success);
    try std.testing.expectEqual(@as(usize, 3), result.current_prefix_count);
    try std.testing.expectEqual(@as(usize, 1), result.delta_added_count);
    try std.testing.expectEqual(@as(usize, 1), result.withdrawals_sent);
}

test "ReloadApplyResult with errors" {
    const result = export_reload_apply.ReloadApplyResult{
        .reload_success = false,
        .reload_error = "FileNotFound",
        .current_prefix_count = 2,
        .delta_added_count = 0,
        .delta_removed_count = 0,
        .delta_unchanged_count = 0,
        .withdrawals_sent = 0,
        .announcements_sent = 0,
        .apply_error = null,
    };
    try std.testing.expect(!result.reload_success);
    try std.testing.expect(result.reload_error != null);
    try std.testing.expectEqualStrings("FileNotFound", result.reload_error.?);
}
