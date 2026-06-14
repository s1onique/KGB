// net/inotify.zig — Linux inotify syscall wrapper
//
// Provides low-level inotify API for file system event monitoring.
// This module is Linux-only and provides the foundation for prefix file watching.

const std = @import("std");

// ============================================================================
// OS Detection - only compile on Linux
// ============================================================================

comptime {
    if (@import("builtin").os.tag != .linux) {
        @compileError("inotify module is only available on Linux");
    }
}

// ============================================================================
// Inotify Constants (from linux/inotify.h)
// ============================================================================

/// inotify_init() syscall
pub fn inotify_init(flags: u32) !i32 {
    const fd = std.c.inotify_init1(@bitCast(flags));
    if (fd < 0) {
        return error.InotifyInitFailed;
    }
    return fd;
}

/// inotify_add_watch() syscall
pub fn inotify_add_watch(fd: i32, path: [*:0]const u8, mask: u32) !i32 {
    const wd = std.c.inotify_add_watch(fd, path, mask);
    if (wd < 0) {
        return error.AddWatchFailed;
    }
    return wd;
}

/// inotify_rm_watch() syscall
pub fn inotify_rm_watch(fd: i32, wd: i32) !void {
    const result = std.c.inotify_rm_watch(fd, wd);
    if (result < 0) {
        return error.RemoveWatchFailed;
    }
}

/// Inotify event structure (matches kernel layout)
pub const Event = extern struct {
    /// Watch descriptor
    wd: i32,
    /// Event mask (what happened)
    mask: u32,
    /// Cookie (for renames - same cookie for from/to events)
    cookie: u32,
    /// Length of the name field (0 if name is empty)
    len: u32,
    /// Name of the file (null-terminated, variable length)
    name: [0]u8,
};

/// Size of the inotify event header (before name field)
pub const EVENT_HEADER_SIZE: usize = @sizeOf(Event);

/// Read inotify events from the given file descriptor.
pub fn readEvents(fd: i32, buf: []u8) !usize {
    const bytes_read = std.c.read(fd, @ptrCast(buf.ptr), buf.len);
    if (bytes_read < 0) {
        return error.ReadFailed;
    }
    return @as(usize, @intCast(bytes_read));
}

/// Iterate over inotify events in a buffer.
pub fn iterateEvents(buf: []const u8) EventIterator {
    return EventIterator{ .buffer = buf, .offset = 0 };
}

/// Event iterator for parsing inotify event buffer.
pub const EventIterator = struct {
    buffer: []const u8,
    offset: usize = 0,

    pub fn next(self: *EventIterator) ?[]const u8 {
        if (self.offset >= self.buffer.len) {
            return null;
        }

        if (self.offset + EVENT_HEADER_SIZE > self.buffer.len) {
            return null;
        }

        const event_ptr = @as(*const Event, @ptrCast(@alignCast(self.buffer.ptr + self.offset)));
        const event_size = EVENT_HEADER_SIZE + event_ptr.len;

        if (self.offset + event_size > self.buffer.len) {
            return null;
        }

        self.offset += event_size;
        self.offset = (self.offset + 7) & ~@as(usize, 7);

        return self.buffer[self.offset - event_size .. self.offset - event_size + event_size];
    }
};

// ============================================================================
// Event Classification
// ============================================================================

/// Relevant event types for prefix file watching.
pub const RelevantEvent = enum {
    close_write,
    modify,
    moved_to,
    delete_self,
    move_self,
};

/// Check if an inotify event mask indicates a relevant event.
pub fn isRelevantEvent(mask: u32) bool {
    const CLOSE_WRITE_MASK = 0x00000008;
    const MODIFY_MASK = 0x00000002;
    const MOVED_TO_MASK = 0x00000080;
    const DELETE_SELF_MASK = 0x00000800;
    const MOVE_SELF_MASK = 0x00008000;

    return (mask & (CLOSE_WRITE_MASK | MODIFY_MASK | MOVED_TO_MASK | DELETE_SELF_MASK | MOVE_SELF_MASK)) != 0;
}

/// Classify an inotify event mask into a RelevantEvent variant.
pub fn classifyEvent(mask: u32) ?RelevantEvent {
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

    return null;
}

/// Build a watch mask for prefix file monitoring.
pub fn prefixWatchMask() u32 {
    return 0x00000008 | // CLOSE_WRITE
           0x00000002 | // MODIFY
           0x00000080 | // MOVED_TO
           0x00000100 | // CREATE
           0x00000200 | // DELETE
           0x00000800 | // DELETE_SELF
           0x00008000; // MOVE_SELF
}
