// bgp/prefix_watch.zig — Prefix file watcher abstraction
//
// ACT: Add inotify watcher for BGP advertised prefix files (Phase 1: watcher + reload detection)
//
// This module provides a watcher abstraction for monitoring prefix file changes.
// Phase 1 focuses on detecting changes and reloading prefix data.
// Phase 2 (BGP UPDATE diff/apply) is deferred to a follow-up ACT.
//
// Design:
// - Watcher trait defines the interface (platform-specific implementations)
// - FakeWatcher provides deterministic testing without real inotify
// - Debouncing coalesces rapid successive events
// - reload-as-transaction: read all files, parse candidate, commit on success
//
// Key constraint: Do NOT mutate live export set until candidate parse succeeds.

const std = @import("std");
const types = @import("types.zig");

// ============================================================================
// Watcher Interface (Trait)
// ============================================================================

/// Watcher event types that trigger a reload.
pub const WatcherEvent = enum {
    /// File was modified and closed (common save pattern)
    close_write,
    /// File was modified (may be multiple events per save)
    modify,
    /// File was moved into the watched location (atomic rename pattern)
    moved_to,
    /// Watched file itself was deleted
    delete_self,
    /// Watched file itself was moved
    move_self,
};

/// Watcher error types.
pub const WatcherError = error{
    /// Failed to initialize the watcher subsystem
    InitFailed,
    /// Failed to add a watch for a path
    AddWatchFailed,
    /// Failed to remove a watch
    RemoveWatchFailed,
    /// Failed to read events
    ReadFailed,
    /// File path is too long for system limits
    PathTooLong,
    /// Watched file was deleted and recreation not detected
    WatchTargetDeleted,
    /// Reload failed but last-good state is preserved
    ReloadFailed,
};

/// Watcher trait - defines the interface for file system watchers.
/// Implementations: LinuxInotifyWatcher (real), FakeWatcher (testing).
pub const Watcher = struct {
    const Self = @This();

    /// Opaque watcher state - implementation-specific.
    state: *anyopaque,
    /// Virtual table for watcher operations.
    vtable: *const WatcherVTable,

    /// Watcher virtual table.
    pub const WatcherVTable = struct {
        /// Destroy the watcher and free resources.
        destroy: *const fn (state: *anyopaque) void,
        /// Poll for events (non-blocking or with timeout).
        /// Returns events that occurred, or null if no events.
        poll: *const fn (state: *anyopaque) anyerror!?[]const WatcherEvent,
        /// Check if a specific event type occurred.
        hasEvent: *const fn (state: *anyopaque, event: WatcherEvent) bool,
        /// Get the path that triggered the most recent event.
        getEventPath: *const fn (state: *anyopaque) ?[]const u8,
        /// Close and reopen watch for a path (handles delete_self/move_self).
        refreshWatch: *const fn (state: *anyopaque, path: []const u8) anyerror!void,
    };

    /// Destroy the watcher.
    pub fn destroy(self: Self) void {
        self.vtable.destroy(self.state);
    }

    /// Poll for events (non-blocking).
    pub fn poll(self: Self) !?[]const WatcherEvent {
        return self.vtable.poll(self.state);
    }

    /// Check if a specific event type occurred.
    pub fn hasEvent(self: Self, event: WatcherEvent) bool {
        return self.vtable.hasEvent(self.state, event);
    }

    /// Get the path that triggered the most recent event.
    pub fn getEventPath(self: Self) ?[]const u8 {
        return self.vtable.getEventPath(self.state);
    }

    /// Refresh watch for a path (handles delete_self/move_self).
    pub fn refreshWatch(self: Self, path: []const u8) !void {
        return self.vtable.refreshWatch(self.state, path);
    }
};

// ============================================================================
// Debouncer
// ============================================================================

/// Debouncer for coalescing rapid successive events.
/// Prevents multiple reloads from a single file save that generates many events.
pub const Debouncer = struct {
    const Self = @This();

    /// Default debounce window in milliseconds.
    pub const DEFAULT_DEBOUNCE_MS: u64 = 100;

    /// Pending reload state.
    pending: bool = false,
    /// Timestamp when pending reload was scheduled.
    scheduled_at_ms: u64 = 0,
    /// Debounce window in milliseconds.
    debounce_ms: u64 = DEFAULT_DEBOUNCE_MS,

    /// Create a new debouncer with default settings.
    pub fn init() Self {
        return Self{};
    }

    /// Create a new debouncer with custom debounce window.
    pub fn initWithDebounce(debounce_ms: u64) Self {
        return Self{ .debounce_ms = debounce_ms };
    }

    /// Schedule a reload (called when an event occurs).
    /// Returns true if this is the first event in the debounce window.
    pub fn schedule(self: *Self, now_ms: u64) bool {
        if (!self.pending) {
            self.pending = true;
            self.scheduled_at_ms = now_ms;
            return true;
        }
        // Already pending - just update timestamp (extend window)
        self.scheduled_at_ms = now_ms;
        return false;
    }

    /// Check if a pending reload should fire.
    /// Returns true if debounce window has elapsed.
    pub fn shouldFire(self: *Self, now_ms: u64) bool {
        if (!self.pending) return false;
        return (now_ms - self.scheduled_at_ms) >= self.debounce_ms;
    }

    /// Cancel a pending reload.
    pub fn cancel(self: *Self) void {
        self.pending = false;
    }

    /// Check if a reload is pending.
    pub fn isPending(self: *Self) bool {
        return self.pending;
    }
};

// ============================================================================
// Reload Result
// ============================================================================

/// Result of a prefix reload operation.
pub const ReloadResult = struct {
    /// Whether the reload succeeded.
    success: bool,
    /// Number of prefixes in the new set (valid only if success=true).
    prefix_count: usize,
    /// Error message if reload failed (valid only if success=false).
    error_message: ?[]const u8,
    /// Paths that had errors during reload.
    error_paths: []const []const u8,
};

/// Last-good state preserved across reload failures.
pub const LastGoodState = struct {
    /// The preserved prefixes from the last successful reload.
    prefixes: []types.Ipv4Prefix,
    /// Error message from the last failed reload attempt.
    last_error: ?[]const u8,
    /// Whether we have ever successfully loaded prefixes.
    has_value: bool,
    /// Error buffer owned by this state (avoids stack-back slices).
    error_buf: [256]u8 = undefined,
};
