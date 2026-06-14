// bgp/serve_export_integration.zig — Prefix export integration for serve command
//
// Provides prefix file watcher initialization and watched reload integration.
// This module is split from serve_integration.zig to keep it LLM-friendly.
//
// The watcher is optional - only created when prefix files are configured.
// The debouncer coalesces rapid successive file change events.
//
// Platform behavior:
// - Linux: uses real inotify watcher
// - Other: no watcher (prefixes loaded at startup only)

const std = @import("std");
const config_parse = @import("config_parse.zig");
const export_reload_apply = @import("export_reload_apply.zig");
const prefix_watch = @import("prefix_watch.zig");
const serve_integration = @import("serve_integration.zig");

// Linux-specific implementation for adding watches
const linux_watch = if (@import("builtin").os.tag == .linux) struct {
    const prefix_watch_linux = @import("prefix_watch_linux.zig");
} else null;

/// Initialize the prefix file watcher for a bundle.
/// Creates a real inotify watcher on Linux, or returns false on other platforms.
/// Returns true if watcher was created, false if no prefix files configured or
/// platform doesn't support inotify.
pub fn initPrefixWatcher(
    bundle: *serve_integration.BgpServeBundle,
    stderr: anytype,
    allocator: std.mem.Allocator,
) bool {
    // Only create watcher if prefix files are configured
    if (bundle.bgp_config.advertised_prefix_files_raw.len == 0) {
        return false;
    }

    // Parse file paths to watch
    const file_paths = config_parse.parsePrefixList(
        bundle.bgp_config.advertised_prefix_files_raw,
        allocator,
    ) catch {
        return false;
    };
    defer allocator.free(file_paths);

    if (file_paths.len == 0) {
        return false;
    }

    // Create real inotify watcher on Linux
    const watcher = prefix_watch.createInotifyWatcher(allocator) catch |e| {
        stderr.print("warning: failed to create prefix file watcher: {s}\n", .{
            @errorName(e),
        }) catch {};
        return false;
    };

    // Add watches for each prefix file path (Linux-specific)
    var failed_watches: usize = 0;
    if (linux_watch) |impl| {
        // Access the underlying linux state to add watches
        const linux_state = @as(*impl.prefix_watch_linux.State, @ptrCast(@alignCast(watcher.state)));
        for (file_paths) |file_path| {
            linux_state.addWatch(file_path) catch |e| {
                stderr.print("warning: failed to watch prefix file '{s}': {s}\n", .{
                    file_path,
                    @errorName(e),
                }) catch {};
                // Continue with other files even if one fails
                failed_watches += 1;
            };
        }
    }

    // If all watches failed, destroy the watcher and return false
    if (failed_watches == file_paths.len) {
        watcher.destroy();
        return false;
    }

    bundle.watcher = watcher;
    return true;
}

/// Check if the bundle has a prefix watcher configured.
pub fn hasPrefixWatcher(bundle: *const serve_integration.BgpServeBundle) bool {
    return bundle.watcher != null;
}

/// Destroy the prefix file watcher if present.
pub fn destroyPrefixWatcher(bundle: *serve_integration.BgpServeBundle) void {
    if (bundle.watcher) |*watcher| {
        watcher.destroy();
        bundle.watcher = null;
    }
}

/// Apply prefix reload via watcher if configured.
/// Returns ReloadApplyResult (may indicate no reload was needed).
pub fn applyPrefixReloadIfWatched(
    bundle: *serve_integration.BgpServeBundle,
    now_ms: u64,
) export_reload_apply.ReloadApplyResult {
    if (bundle.watcher) |*watcher| {
        return export_reload_apply.watchAndApply(
            &bundle.export_state,
            bundle.bgp_config.advertised_prefix_files_raw,
            &bundle.sess,
            watcher,
            &bundle.debouncer,
            now_ms,
        );
    }

    // No watcher - return current state
    return export_reload_apply.ReloadApplyResult{
        .reload_success = bundle.export_state.last_reload_success,
        .reload_error = bundle.export_state.last_reload_error,
        .current_prefix_count = bundle.export_state.exportedCount(),
        .delta_added_count = bundle.export_state.last_delta_added_count,
        .delta_removed_count = bundle.export_state.last_delta_removed_count,
        .delta_unchanged_count = bundle.export_state.last_delta_unchanged_count,
        .withdrawals_sent = 0,
        .announcements_sent = 0,
        .apply_error = bundle.export_state.last_apply_error,
    };
}
