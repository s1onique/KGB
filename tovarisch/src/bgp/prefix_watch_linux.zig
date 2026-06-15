// bgp/prefix_watch_linux.zig — Linux inotify watcher implementation
//
// Linux inotify-based watcher implementation for production use.
// Handles file system events for prefix file change detection.

const std = @import("std");
const inotify = @import("../net/inotify.zig");
const prefix_watch = @import("prefix_watch.zig");

// ============================================================================
// OS Detection - only compile on Linux
// ============================================================================

comptime {
    if (@import("builtin").os.tag != .linux) {
        @compileError("prefix_watch_linux module is only available on Linux");
    }
}

/// Watch entry for tracking a watched file.
pub const WatchEntry = struct {
    wd: i32,
    path: []const u8,
};

/// Prefix watcher state.
/// Owns the inotify fd, watch descriptors, and reload state.
pub const PrefixWatcherState = struct {
    const Self = @This();

    /// Allocator used at creation (for consistent destroy/deinit).
    allocator: std.mem.Allocator,
    /// Linux inotify file descriptor (-1 if not initialized).
    inotify_fd: i32 = -1,
    /// Watch descriptors mapped to file paths.
    watches: std.ArrayListUnmanaged(WatchEntry) = .empty,
    /// Error buffer for status reporting.
    error_buf: [256]u8 = undefined,

    /// Initialize the watcher state.
    pub fn init(allocator: std.mem.Allocator) !Self {
        var state = Self{ .allocator = allocator };
        try state.watches.ensureTotalCapacity(allocator, 4);
        return state;
    }

    /// Deinitialize and clean up resources.
    /// Frees all duplicated paths in watches.
    pub fn deinit(self: *Self) void {
        if (self.inotify_fd >= 0) {
            _ = std.c.close(self.inotify_fd);
            self.inotify_fd = -1;
        }
        // Free each duplicated path before deinit
        for (self.watches.items) |entry| {
            self.allocator.free(entry.path);
        }
        self.watches.deinit(self.allocator);
    }

    /// Add a watch for a file path.
    pub fn addWatch(self: *Self, path: []const u8) !i32 {
        if (self.inotify_fd < 0) {
            // Use nonblocking + cloexec flags for safe polling
            const IN_NONBLOCK = 0x00000800;
            const IN_CLOEXEC = 0x00080000;
            self.inotify_fd = try inotify.inotify_init(IN_NONBLOCK | IN_CLOEXEC);
        }

        var path_buf: [4096]u8 = undefined;
        if (path.len >= path_buf.len) return error.PathTooLong;
        // MemoryCopySafety: path_buf is a [4096]u8 stack buffer. path is a caller-provided
        // slice. They are distinct memory regions; no aliasing possible.
        @memcpy(path_buf[0..path.len], path);
        path_buf[path.len] = 0;
        const c_path: [*:0]const u8 = @ptrCast(@alignCast(path_buf[0..path.len].ptr));

        const wd = try inotify.inotify_add_watch(self.inotify_fd, c_path, inotify.prefixWatchMask());
        try self.watches.append(self.allocator, WatchEntry{
            .wd = wd,
            .path = try self.allocator.dupe(u8, path),
        });
        return wd;
    }

    /// Update the watch descriptor for a path.
    /// Returns the new wd, or error if path not found.
    pub fn updateWatch(self: *Self, path: []const u8) !i32 {
        if (self.inotify_fd < 0) return error.NotInitialized;

        // Find existing entry
        for (self.watches.items) |*entry| {
            if (std.mem.eql(u8, entry.path, path)) {
                // Remove old watch
                inotify.inotify_rm_watch(self.inotify_fd, entry.wd) catch {};

                // Add new watch
                var path_buf: [4096]u8 = undefined;
                if (path.len >= path_buf.len) return error.PathTooLong;
                // MemoryCopySafety: path_buf is a [4096]u8 stack buffer. path is a caller-provided
                // slice. They are distinct memory regions; no aliasing possible.
                @memcpy(path_buf[0..path.len], path);
                path_buf[path.len] = 0;
                const c_path: [*:0]const u8 = @ptrCast(@alignCast(path_buf[0..path.len].ptr));

                const new_wd = try inotify.inotify_add_watch(self.inotify_fd, c_path, inotify.prefixWatchMask());
                entry.wd = new_wd;
                return new_wd;
            }
        }
        return error.PathNotWatched;
    }

    /// Get the inotify file descriptor for polling.
    pub fn getFd(self: *const Self) i32 {
        return self.inotify_fd;
    }

    /// Check if inotify is initialized.
    pub fn isInitialized(self: *const Self) bool {
        return self.inotify_fd >= 0;
    }
};

/// Opaque state for Linux watcher.
pub const State = struct {
    /// Allocator used at creation (for consistent destroy/deinit).
    allocator: std.mem.Allocator,
    state: PrefixWatcherState,
    /// Event buffer for reading inotify events.
    event_buf: [8192]u8 = undefined,
    /// Last event that occurred.
    last_event: ?prefix_watch.WatcherEvent = null,
    /// Path associated with last event.
    last_event_path: []const u8 = "",
    /// Pending refresh requests.
    pending_refresh: std.ArrayListUnmanaged([]const u8) = .empty,
    /// Owned event buffer for poll() return (avoids returning watch entries).
    poll_events: [8]prefix_watch.WatcherEvent = undefined,
    /// Number of valid events in poll_events.
    poll_count: usize = 0,
};

/// Create a new Linux inotify watcher.
pub fn create(allocator: std.mem.Allocator) !prefix_watch.Watcher {
    var impl_state = try allocator.create(State);
    impl_state.* = State{
        .allocator = allocator,
        .state = try PrefixWatcherState.init(allocator),
    };
    try impl_state.pending_refresh.ensureTotalCapacity(allocator, 4);

    return prefix_watch.Watcher{
        .state = @ptrCast(impl_state),
        .vtable = &vtable,
    };
}

/// Destroy the watcher.
/// Frees all pending_refresh paths and PrefixWatcherState resources.
fn destroy(state: *anyopaque) void {
    const self = @as(*State, @ptrCast(@alignCast(state)));
    const alloc = self.allocator;

    // Free each pending_refresh path
    for (self.pending_refresh.items) |path| {
        alloc.free(path);
    }
    self.pending_refresh.deinit(alloc);

    self.state.deinit();
    alloc.destroy(self);
}

/// Poll for events.
fn poll(state: *anyopaque) !?[]const prefix_watch.WatcherEvent {
    const self = @as(*State, @ptrCast(@alignCast(state)));
    if (self.state.inotify_fd < 0) return null;

    // Reset per-poll event state before reading
    self.last_event = null;
    self.last_event_path = "";
    self.poll_count = 0;

    const bytes_read = inotify.readEvents(self.state.inotify_fd, &self.event_buf) catch return null;
    if (bytes_read == 0) return null;

    var iter = inotify.iterateEvents(self.event_buf[0..bytes_read]);
    var found_events = false;

    while (iter.next()) |event_slice| {
        const event = @as(*const inotify.Event, @ptrCast(@alignCast(event_slice.ptr)));
        if (inotify.isRelevantEvent(event.mask)) {
            found_events = true;
            self.last_event = mapInotifyEvent(event.mask);

            // Set last_event_path when wd maps to a watched path
            for (self.state.watches.items) |entry| {
                if (entry.wd == event.wd) {
                    self.last_event_path = entry.path;
                    break;
                }
            }
        }

        const DELETE_SELF_MASK = 0x00000800;
        const MOVE_SELF_MASK = 0x00008000;
        if ((event.mask & (DELETE_SELF_MASK | MOVE_SELF_MASK)) != 0) {
            for (self.state.watches.items) |entry| {
                if (entry.wd == event.wd) {
                    const path_copy = self.allocator.dupe(u8, entry.path) catch continue;
                    try self.pending_refresh.append(self.allocator, path_copy);
                    break;
                }
            }
        }
    }

    if (!found_events) return null;

    // Store events in owned buffer
    if (self.last_event) |e| {
        self.poll_events[self.poll_count] = e;
        self.poll_count += 1;
    }
    return self.poll_events[0..self.poll_count];
}

/// Check if a specific event type occurred.
fn hasEvent(state: *anyopaque, event: prefix_watch.WatcherEvent) bool {
    const self = @as(*State, @ptrCast(@alignCast(state)));
    return self.last_event != null and self.last_event.? == event;
}

/// Get the path that triggered the most recent event.
fn getEventPath(state: *anyopaque) ?[]const u8 {
    const self = @as(*State, @ptrCast(@alignCast(state)));
    if (self.last_event_path.len == 0) return null;
    return self.last_event_path;
}

/// Refresh watch for a path.
/// Updates the stored watch descriptor after re-adding.
fn refreshWatch(state: *anyopaque, path: []const u8) !void {
    const self = @as(*State, @ptrCast(@alignCast(state)));
    if (self.state.inotify_fd < 0) return;

    _ = try self.state.updateWatch(path);
}

/// Virtual table for Linux watcher.
const vtable = prefix_watch.Watcher.WatcherVTable{
    .destroy = destroy,
    .poll = poll,
    .hasEvent = hasEvent,
    .getEventPath = getEventPath,
    .refreshWatch = refreshWatch,
};

/// Add a watch for a file path via the State wrapper.
pub fn addWatch(state: *State, path: []const u8) !void {
    _ = try state.state.addWatch(path);
}

/// Map inotify event mask to WatcherEvent.
fn mapInotifyEvent(mask: u32) prefix_watch.WatcherEvent {
    const CLOSE_WRITE_MASK = 0x00000008;
    const MODIFY_MASK = 0x00000002;
    const MOVED_TO_MASK = 0x00000080;
    const DELETE_SELF_MASK = 0x00000800;
    const MOVE_SELF_MASK = 0x00008000;

    if ((mask & CLOSE_WRITE_MASK) != 0) return .close_write;
    if ((mask & MOVED_TO_MASK) != 0) return .moved_to;
    if ((mask & DELETE_SELF_MASK) != 0) return .delete_self;
    if ((mask & MOVE_SELF_MASK) != 0) return .move_self;
    if ((mask & MODIFY_MASK) != 0) return .modify;

    return .modify;
}
