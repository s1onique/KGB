// bgp/serve_export_integration_tests.zig — Linux-only tests for serve_export_integration
//
// Tests the real inotify watcher init/destroy path on Linux.
// This file is kept separate to avoid exceeding LLM-friendly line limits.

const std = @import("std");
const config = @import("../config.zig");
const config_parse = @import("config_parse.zig");
const serve_export_integration = @import("serve_export_integration.zig");
const serve_integration = @import("serve_integration.zig");
const linux_stats = @import("../net/linux_stats.zig");

/// VoidWriter discards all output - used for stderr when we don't care about warnings.
const VoidWriter = struct {
    const Self = @This();
    pub fn write(_: Self, _: []const u8) error{}!void {}
    pub fn print(_: Self, _: []const u8, _: anytype) error{}!void {}
    pub fn writeByte(_: Self, _: u8) error{}!void {}
};

test "serve_export_integration: initPrefixWatcher creates real watcher on Linux" {
    if (@import("builtin").os.tag != .linux) return error.SkipZigTest;

    // Create a real temp prefix file
    const prefix_path = "/tmp/tovarisch_test_real_watcher.conf";
    try linux_stats.writeFile(prefix_path, "10.0.0.0/8\n");

    // Build the nested raw configuration expected by parseBgpConfig.
    var raw: config.RawConfig = .empty;
    defer raw.deinit(std.testing.allocator);

    // Add the bgp section with required fields
    var bgp_section: std.array_hash_map.String([]const u8) = .empty;
    defer bgp_section.deinit(std.testing.allocator);

    try bgp_section.put(std.testing.allocator, "enabled", "true");
    try bgp_section.put(std.testing.allocator, "peer_address", "10.0.0.2");
    try bgp_section.put(std.testing.allocator, "peer_as", "65002");
    try bgp_section.put(std.testing.allocator, "local_address", "10.0.0.1");
    try bgp_section.put(std.testing.allocator, "local_as", "65001");
    try bgp_section.put(std.testing.allocator, "advertised_prefix_files", prefix_path);

    try raw.put(std.testing.allocator, "bgp", bgp_section);

    const bgp_cfg = config_parse.parseBgpConfig(&raw) catch return error.SkipZigTest;
    try std.testing.expect(bgp_cfg.present);
    try std.testing.expect(bgp_cfg.enabled);

    // Create bundle on heap (like loadConfigAndBgp does)
    var bundle = std.heap.page_allocator.create(serve_integration.BgpServeBundle) catch return error.SkipZigTest;
    defer std.heap.page_allocator.destroy(bundle);

    bundle.* = serve_integration.BgpServeBundle{
        .raw = raw,
        .bgp_config = bgp_cfg,
        .session_config = undefined,
        .state = .not_configured,
        .last_error = null,
        .prefixes = &.{},
        .tcp = undefined,
        .trans = undefined,
        .sess = undefined,
        .export_state = .{},
    };
    bundle.export_state.init(std.testing.allocator);
    defer bundle.export_state.deinit();

    // Verify no watcher initially
    try std.testing.expect(!serve_export_integration.hasPrefixWatcher(bundle));

    // Init watcher with real inotify (VoidWriter for stderr since we don't care about warnings)
    const stderr = VoidWriter{};
    const init_ok = serve_export_integration.initPrefixWatcher(bundle, stderr, std.testing.allocator);
    try std.testing.expect(init_ok);
    try std.testing.expect(serve_export_integration.hasPrefixWatcher(bundle));

    // Destroy watcher
    serve_export_integration.destroyPrefixWatcher(bundle);
    try std.testing.expect(!serve_export_integration.hasPrefixWatcher(bundle));
}
