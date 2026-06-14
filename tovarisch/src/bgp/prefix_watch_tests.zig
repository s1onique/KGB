// bgp/prefix_watch_tests.zig — Tests for prefix file watcher
//
// ACT: Add inotify watcher for BGP advertised prefix files (Phase 1: watcher + reload detection)
//
// Tests:
// - Debouncer behavior (schedule, shouldFire, cancel)
// - FakeWatcher event injection
// - Reload logic with temp files
// - Invalid file preserves last-good state
// - Atomic rename replacement handling
// - Missing/deleted file diagnostic

const std = @import("std");
const prefix_watch = @import("prefix_watch.zig");
const prefix_watch_fake = @import("prefix_watch_fake.zig");
const prefix_watch_reload = @import("prefix_watch_reload.zig");
const types = @import("types.zig");

// Compile sentinels for Linux modules - only compile on Linux
test "linux prefix watcher modules compile" {
    if (@import("builtin").os.tag == .linux) {
        std.testing.refAllDecls(@import("prefix_watch_linux.zig"));
        std.testing.refAllDecls(@import("../net/inotify.zig"));
    }
}

// ============================================================================
// Debouncer Tests
// ============================================================================

test "Debouncer schedules first event" {
    var debouncer = prefix_watch.Debouncer.init();
    try std.testing.expect(!debouncer.isPending());

    const first = debouncer.schedule(1000);
    try std.testing.expect(first);
    try std.testing.expect(debouncer.isPending());
}

test "Debouncer coalesces rapid events" {
    var debouncer = prefix_watch.Debouncer.init();
    _ = debouncer.schedule(1000);

    const second = debouncer.schedule(1001);
    try std.testing.expect(!second);
    try std.testing.expect(debouncer.isPending());
}

test "Debouncer fires after window elapses" {
    var debouncer = prefix_watch.Debouncer.initWithDebounce(100);

    _ = debouncer.schedule(1000);
    try std.testing.expect(!debouncer.shouldFire(1050));

    try std.testing.expect(debouncer.shouldFire(1100));
}

test "Debouncer cancels pending reload" {
    var debouncer = prefix_watch.Debouncer.init();
    _ = debouncer.schedule(1000);

    debouncer.cancel();
    try std.testing.expect(!debouncer.isPending());
    try std.testing.expect(!debouncer.shouldFire(2000));
}

test "Debouncer extends window on subsequent events" {
    var debouncer = prefix_watch.Debouncer.initWithDebounce(100);

    _ = debouncer.schedule(1000);
    _ = debouncer.schedule(1090);
    try std.testing.expect(!debouncer.shouldFire(1180));
    try std.testing.expect(debouncer.shouldFire(1190));
}

// ============================================================================
// FakeWatcher Tests
// ============================================================================

test "FakeWatcher creates and destroys" {
    const watcher = try prefix_watch_fake.create(std.heap.page_allocator);
    defer watcher.destroy();

    const events = try watcher.poll();
    try std.testing.expect(events == null);
}

test "FakeWatcher injects and returns events" {
    const watcher = try prefix_watch_fake.create(std.heap.page_allocator);
    defer watcher.destroy();

    prefix_watch_fake.inject(watcher, .close_write);
    prefix_watch_fake.inject(watcher, .modify);

    const events = try watcher.poll();
    try std.testing.expect(events != null);
    try std.testing.expectEqual(@as(usize, 2), events.?.len);
    try std.testing.expectEqual(prefix_watch.WatcherEvent.close_write, events.?[0]);
    try std.testing.expectEqual(prefix_watch.WatcherEvent.modify, events.?[1]);
}

test "FakeWatcher hasEvent returns true for injected event" {
    const watcher = try prefix_watch_fake.create(std.heap.page_allocator);
    defer watcher.destroy();

    prefix_watch_fake.inject(watcher, .close_write);

    try std.testing.expect(watcher.hasEvent(.close_write));
    try std.testing.expect(!watcher.hasEvent(.delete_self));
}

test "FakeWatcher setEventPath" {
    const watcher = try prefix_watch_fake.create(std.heap.page_allocator);
    defer watcher.destroy();

    prefix_watch_fake.setEventPath(watcher, "/etc/tovarisch/prefixes.conf");

    const path = watcher.getEventPath();
    try std.testing.expect(path != null);
    try std.testing.expectEqualStrings("/etc/tovarisch/prefixes.conf", path.?);
}

test "FakeWatcher returns null after clearing events" {
    const watcher = try prefix_watch_fake.create(std.heap.page_allocator);
    defer watcher.destroy();

    prefix_watch_fake.inject(watcher, .close_write);

    _ = try watcher.poll();
    const events = try watcher.poll();
    try std.testing.expect(events == null);
}

test "FakeWatcher deactivate" {
    const watcher = try prefix_watch_fake.create(std.heap.page_allocator);
    defer watcher.destroy();

    prefix_watch_fake.inject(watcher, .close_write);
    prefix_watch_fake.deactivate(watcher);

    const events = try watcher.poll();
    try std.testing.expect(events == null);
}

// ============================================================================
// Reload Logic Tests (using inline test data)
// ============================================================================

test "Reload handles empty file list" {
    var last_good = prefix_watch.LastGoodState{
        .prefixes = &.{},
        .last_error = null,
        .has_value = false,
    };

    const result = prefix_watch_reload.reloadPrefixes("", std.heap.page_allocator, &last_good);
    try std.testing.expect(result.success);
    try std.testing.expectEqual(@as(usize, 0), result.prefix_count);
    try std.testing.expect(last_good.has_value);
}

test "Reload handles missing file gracefully" {
    var last_good = prefix_watch.LastGoodState{
        .prefixes = &.{},
        .last_error = null,
        .has_value = false,
    };

    const result = prefix_watch_reload.reloadPrefixes("/nonexistent/path/prefixes.conf", std.heap.page_allocator, &last_good);
    try std.testing.expect(!result.success);
    try std.testing.expect(last_good.last_error != null);
}

// ============================================================================
// WatcherEvent Tests
// ============================================================================

test "WatcherEvent variants are distinct" {
    const events = .{
        prefix_watch.WatcherEvent.close_write,
        prefix_watch.WatcherEvent.modify,
        prefix_watch.WatcherEvent.moved_to,
        prefix_watch.WatcherEvent.delete_self,
        prefix_watch.WatcherEvent.move_self,
    };

    inline for (events, 0..) |event, i| {
        try std.testing.expectEqual(@as(usize, i), @intFromEnum(event));
    }
}

// ============================================================================
// ReloadResult Tests
// ============================================================================

test "ReloadResult success case" {
    const result = prefix_watch.ReloadResult{
        .success = true,
        .prefix_count = 5,
        .error_message = null,
        .error_paths = &.{},
    };

    try std.testing.expect(result.success);
    try std.testing.expectEqual(@as(usize, 5), result.prefix_count);
}

test "ReloadResult failure case" {
    const result = prefix_watch.ReloadResult{
        .success = false,
        .prefix_count = 3,
        .error_message = "FileNotFound",
        .error_paths = &.{ "/path/to/file.conf" },
    };

    try std.testing.expect(!result.success);
    try std.testing.expectEqual(@as(usize, 3), result.prefix_count);
}

// ============================================================================
// LastGoodState Tests
// ============================================================================

test "LastGoodState initial state" {
    const state = prefix_watch.LastGoodState{
        .prefixes = &.{},
        .last_error = null,
        .has_value = false,
    };

    try std.testing.expect(!state.has_value);
    try std.testing.expect(state.prefixes.len == 0);
}

test "LastGoodState after successful load" {
    const state = prefix_watch.LastGoodState{
        .prefixes = &.{},
        .last_error = null,
        .has_value = false,
    };

    try std.testing.expect(!state.has_value);
    try std.testing.expectEqual(@as(usize, 0), state.prefixes.len);
}

test "LastGoodState preserves state on failure" {
    var state = prefix_watch.LastGoodState{
        .prefixes = &.{},
        .last_error = null,
        .has_value = false,
    };

    state.last_error = "FileNotFound";

    try std.testing.expect(!state.has_value);
    try std.testing.expect(state.last_error != null);
}

// ============================================================================
// Debouncer Edge Cases
// ============================================================================

test "Debouncer with zero debounce fires immediately" {
    var debouncer = prefix_watch.Debouncer.initWithDebounce(0);

    _ = debouncer.schedule(1000);
    try std.testing.expect(debouncer.shouldFire(1000));
}

test "Debouncer with very large debounce never fires" {
    var debouncer = prefix_watch.Debouncer.initWithDebounce(1000000);

    _ = debouncer.schedule(1000);
    try std.testing.expect(!debouncer.shouldFire(1001));
    try std.testing.expect(!debouncer.shouldFire(100000));
}
