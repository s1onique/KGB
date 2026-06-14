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

test "serve_export_integration: initPrefixWatcher creates real watcher on Linux" {
    if (@import("builtin").os.tag != .linux) return error.SkipZigTest;

    // Create a real temp prefix file
    const prefix_path = "/tmp/tovarisch_test_real_watcher.conf";
    try linux_stats.writeFile(prefix_path, "10.0.0.0/8\n");

    // Build a minimal BgpServeBundle with advertised_prefix_files_raw
    var raw = config.RawConfig{};
    raw.sections = &.{
        config.RawSection{
            .name = "bgp",
            .fields = &.{
                config.RawField{ .key = "enabled", .value = "true" },
                config.RawField{ .key = "peer_address", .value = "10.0.0.2" },
                config.RawField{ .key = "peer_as", .value = "65002" },
                config.RawField{ .key = "local_address", .value = "10.0.0.1" },
                config.RawField{ .key = "local_as", .value = "65001" },
                config.RawField{ .key = "advertised_prefix_files", .value = prefix_path },
            },
        },
    };

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

    // Init watcher with real inotify
    const init_ok = serve_export_integration.initPrefixWatcher(bundle, std.io.getStdErr().writer(), std.testing.allocator);
    try std.testing.expect(init_ok);
    try std.testing.expect(serve_export_integration.hasPrefixWatcher(bundle));

    // Destroy watcher
    serve_export_integration.destroyPrefixWatcher(bundle);
    try std.testing.expect(!serve_export_integration.hasPrefixWatcher(bundle));
}
