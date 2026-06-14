// bgp/prefix_watch_fake.zig — Fake watcher for deterministic testing
//
// Fake watcher implementation for testing.
// Does not use real inotify - allows controlled event injection.

const std = @import("std");
const prefix_watch = @import("prefix_watch.zig");

/// Fake watcher state.
pub const State = struct {
    /// Allocator used at creation (for consistent destroy/deinit).
    allocator: std.mem.Allocator,
    /// Injected events (FIFO queue).
    events: std.ArrayListUnmanaged(prefix_watch.WatcherEvent) = .empty,
    /// Path associated with last event.
    last_event_path: []const u8 = "",
    /// Whether the watcher is active.
    active: bool = true,
    /// Paths being watched (for validation).
    watched_paths: std.ArrayListUnmanaged([]const u8) = .empty,
    /// Owned event buffer for poll() return (avoids heap allocation).
    poll_events: [8]prefix_watch.WatcherEvent = undefined,
    /// Number of valid events in poll_events.
    poll_count: usize = 0,
};

/// Create a new fake watcher.
pub fn create(allocator: std.mem.Allocator) !prefix_watch.Watcher {
    var impl_state = try allocator.create(State);
    impl_state.* = State{
        .allocator = allocator,
        .events = .empty,
        .last_event_path = "",
    };
    try impl_state.events.ensureTotalCapacity(allocator, 16);
    try impl_state.watched_paths.ensureTotalCapacity(allocator, 4);

    return prefix_watch.Watcher{
        .state = @ptrCast(impl_state),
        .vtable = &vtable,
    };
}

/// Destroy the fake watcher.
fn destroy(state: *anyopaque) void {
    const self = @as(*State, @ptrCast(@alignCast(state)));
    const alloc = self.allocator;
    self.events.deinit(alloc);
    self.watched_paths.deinit(alloc);
    alloc.destroy(self);
}

/// Poll for events (returns state-owned buffer valid until next poll).
fn poll(state: *anyopaque) !?[]const prefix_watch.WatcherEvent {
    const self = @as(*State, @ptrCast(@alignCast(state)));
    if (!self.active or self.events.items.len == 0) return null;

    // Copy events into owned buffer
    self.poll_count = @min(self.events.items.len, self.poll_events.len);
    @memcpy(self.poll_events[0..self.poll_count], self.events.items[0..self.poll_count]);
    self.events.clearRetainingCapacity();
    return self.poll_events[0..self.poll_count];
}

/// Check if a specific event type occurred.
fn hasEvent(state: *anyopaque, event: prefix_watch.WatcherEvent) bool {
    const self = @as(*State, @ptrCast(@alignCast(state)));
    for (self.events.items) |e| {
        if (e == event) return true;
    }
    return false;
}

/// Get the path that triggered the most recent event.
fn getEventPath(state: *anyopaque) ?[]const u8 {
    const self = @as(*State, @ptrCast(@alignCast(state)));
    if (self.last_event_path.len == 0) return null;
    return self.last_event_path;
}

/// Refresh watch for a path (no-op for fake watcher).
fn refreshWatch(state: *anyopaque, path: []const u8) !void {
    _ = state;
    _ = path;
    // No-op for fake watcher
}

/// Virtual table for fake watcher.
const vtable = prefix_watch.Watcher.WatcherVTable{
    .destroy = destroy,
    .poll = poll,
    .hasEvent = hasEvent,
    .getEventPath = getEventPath,
    .refreshWatch = refreshWatch,
};

// ============================================================================
// Helper functions for tests
// ============================================================================

/// Inject an event into the fake watcher (for testing).
pub fn inject(watcher: prefix_watch.Watcher, event: prefix_watch.WatcherEvent) void {
    const self = @as(*State, @ptrCast(@alignCast(watcher.state)));
    self.events.append(self.allocator, event) catch {};
}

/// Set the path for the next event.
pub fn setEventPath(watcher: prefix_watch.Watcher, path: []const u8) void {
    const self = @as(*State, @ptrCast(@alignCast(watcher.state)));
    self.last_event_path = path;
}

/// Deactivate the fake watcher.
pub fn deactivate(watcher: prefix_watch.Watcher) void {
    const self = @as(*State, @ptrCast(@alignCast(watcher.state)));
    self.active = false;
}
