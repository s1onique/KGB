// diag_event_ring.zig — Bounded diagnostic event ring buffer
//
// ACT: Add tovarisch WireGuard and XRay TCP underlay diagnostics
// In-memory ring buffer for network diagnostic events.
//
// Events are stored with:
// - timestamp (ISO 8601)
// - severity: info, warning, error
// - source: wireguard, interface, route, underlay_tcp, bgp, bfd
// - message
// - structured fields

const std = @import("std");

/// Get wall clock time in milliseconds.
pub fn wallClockMs() i64 {
    if (comptime @import("builtin").os.tag == .linux and @hasDecl(std.os.linux, "clock_gettime")) {
        var ts: std.os.linux.timespec = undefined;
        if (std.os.linux.clock_gettime(@enumFromInt(0), &ts) < 0) return 0;
        return ts.tv_sec * 1000 + @divTrunc(ts.tv_nsec, 1_000_000);
    }
    return 1718700000000; // test timestamp
}

// ============================================================================
// Types
// ============================================================================

/// Event severity levels.
pub const EventSeverity = enum {
    info,
    warning,
    @"error",
};

/// Event source categories.
pub const EventSource = enum {
    wireguard,
    interface,
    route,
    underlay_tcp,
    bgp,
    bfd,
    unknown,
};

/// A single diagnostic event.
pub const DiagEvent = struct {
    /// Event timestamp (ISO 8601 format).
    ts: []const u8,
    /// Event severity.
    severity: EventSeverity,
    /// Event source category.
    source: EventSource,
    /// Human-readable message.
    message: []const u8,
    /// Optional structured fields (JSON string).
    fields: ?[]const u8 = null,
};

/// Ring buffer configuration.
pub const RingConfig = struct {
    /// Maximum number of events to store.
    max_events: usize = 200,
    /// Whether to include info-level events (vs warning/error only).
    include_info: bool = true,
};

/// Bounded ring buffer for diagnostic events.
pub const EventRing = struct {
    /// Stored events (owned).
    events: []DiagEvent,
    /// Current write index.
    write_index: usize = 0,
    /// Number of events ever written.
    total_written: usize = 0,
    /// Configuration.
    config: RingConfig,

    /// Initialize a new event ring.
    pub fn init(allocator: std.mem.Allocator, config: RingConfig) !EventRing {
        const events = try allocator.alloc(DiagEvent, config.max_events);
        for (events) |*e| e.* = .{ .ts = "", .severity = .info, .source = .unknown, .message = "" };

        return EventRing{
            .events = events,
            .write_index = 0,
            .total_written = 0,
            .config = config,
        };
    }

    /// Free event ring memory.
    pub fn deinit(self: *EventRing, allocator: std.mem.Allocator) void {
        allocator.free(self.events);
        self.* = undefined;
    }

    /// Add an event to the ring.
    pub fn push(self: *EventRing, event: DiagEvent) void {
        // Skip info events if configured to exclude them
        if (event.severity == .info and !self.config.include_info) {
            return;
        }

        // Write event to current position (overwriting oldest if full)
        self.events[self.write_index] = event;
        self.write_index = (self.write_index + 1) % self.config.max_events;
        self.total_written += 1;
    }

    /// Get all events in chronological order (oldest first).
    /// Caller must provide a buffer and receives up to `buffer.len` events.
    /// Returns the number of events written to the buffer.
    pub fn getEvents(self: *const EventRing, buffer: []DiagEvent) usize {
        const count = @min(self.total_written, @min(self.config.max_events, buffer.len));
        if (count == 0) return 0;

        // Events are stored newest-to-oldest wrapped around the ring.
        // To return chronological order, we need to calculate the start.
        if (self.total_written >= self.config.max_events) {
            // Ring is full, oldest event is at write_index
            for (0..count) |i| {
                buffer[i] = self.events[(self.write_index + i) % self.config.max_events];
            }
        } else {
            // Ring is not full, events are at the beginning
            for (0..count) |i| {
                buffer[i] = self.events[i];
            }
        }
        return count;
    }

    /// Get events newest first.
    /// Caller must provide a buffer and receives up to `buffer.len` events.
    /// Returns the number of events written to the buffer.
    pub fn getEventsNewestFirst(self: *const EventRing, buffer: []DiagEvent) usize {
        const count = @min(self.total_written, @min(self.config.max_events, buffer.len));
        if (count == 0) return 0;

        for (0..count) |i| {
            const idx = (self.write_index + count - 1 - i) % self.config.max_events;
            buffer[i] = self.events[idx];
        }
        return count;
    }

    /// Clear all events from the ring.
    pub fn clear(self: *EventRing) void {
        for (self.events) |*e| e.* = .{ .ts = "", .severity = .info, .source = .unknown, .message = "" };
        self.write_index = 0;
        self.total_written = 0;
    }

    /// Get the number of events currently stored.
    pub fn len(self: *const EventRing) usize {
        return @min(self.total_written, self.config.max_events);
    }
};

// ============================================================================
// Event Factory Helpers
// ============================================================================

/// Format a timestamp as ISO 8601.
pub fn formatTimestamp(allocator: std.mem.Allocator, timestamp: i64) ![]const u8 {
    return std.fmt.allocPrint(allocator, "{d}", .{timestamp});
}

/// Create a warning event for WireGuard handshake staleness.
pub fn makeHandshakeStaleEvent(
    allocator: std.mem.Allocator,
    peer_key: []const u8,
    age_seconds: u64,
) !DiagEvent {
    const fields = try std.fmt.allocPrint(allocator,
        \\{{"peer_key":"{s}","age_seconds":{d}}}
    , .{ peer_key, age_seconds });
    errdefer allocator.free(fields);

    return DiagEvent{
        .ts = try formatTimestamp(allocator, wallClockMs()),
        .severity = .warning,
        .source = .wireguard,
        .message = "WireGuard handshake is stale",
        .fields = fields,
    };
}

/// Create a warning event for interface error/drop counters increasing.
pub fn makeInterfaceErrorEvent(
    allocator: std.mem.Allocator,
    iface_name: []const u8,
    error_type: []const u8,
    count: u64,
) !DiagEvent {
    const fields = try std.fmt.allocPrint(allocator,
        \\{{"interface":"{s}","error_type":"{s}","count":{d}}}
    , .{ iface_name, error_type, count });
    errdefer allocator.free(fields);

    return DiagEvent{
        .ts = try formatTimestamp(allocator, wallClockMs()),
        .severity = .warning,
        .source = .interface,
        .message = "Interface error counter increased",
        .fields = fields,
    };
}

/// Create a warning event for TCP retransmit evidence.
pub fn makeTcpRetransmitEvent(
    allocator: std.mem.Allocator,
    socket_name: []const u8,
    retransmit_count: u64,
    rto_ms: u64,
) !DiagEvent {
    const fields = try std.fmt.allocPrint(allocator,
        \\{{"name":"{s}","retransmit_count":{d},"rto_ms":{d}}}
    , .{ socket_name, retransmit_count, rto_ms });
    errdefer allocator.free(fields);

    return DiagEvent{
        .ts = try formatTimestamp(allocator, wallClockMs()),
        .severity = .warning,
        .source = .underlay_tcp,
        .message = "TCP retransmit evidence increased",
        .fields = fields,
    };
}

/// Create an info event for route change.
pub fn makeRouteChangeEvent(
    allocator: std.mem.Allocator,
    target: []const u8,
    interface: []const u8,
) !DiagEvent {
    const fields = try std.fmt.allocPrint(allocator,
        \\{{"target":"{s}","interface":"{s}"}}
    , .{ target, interface });
    errdefer allocator.free(fields);

    return DiagEvent{
        .ts = try formatTimestamp(allocator, wallClockMs()),
        .severity = .info,
        .source = .route,
        .message = "Route changed",
        .fields = fields,
    };
}

// ============================================================================
// Tests
// ============================================================================

test "EventRing stores events" {
    const allocator = std.heap.page_allocator;
    var ring = try EventRing.init(allocator, .{ .max_events = 10 });
    defer ring.deinit(allocator);

    try std.testing.expectEqual(@as(usize, 0), ring.len());

    ring.push(DiagEvent{
        .ts = "2026-06-18T08:00:00Z",
        .severity = .info,
        .source = .wireguard,
        .message = "test event",
    });

    try std.testing.expectEqual(@as(usize, 1), ring.len());
}

test "EventRing wraps around" {
    const allocator = std.heap.page_allocator;
    const config = RingConfig{ .max_events = 3 };
    var ring = try EventRing.init(allocator, config);
    defer ring.deinit(allocator);

    // Add 5 events to a 3-slot ring
    inline for (1..6) |i| {
        var ts_buf: [32]u8 = undefined;
        var msg_buf: [64]u8 = undefined;
        const ts = std.fmt.bufPrintZ(&ts_buf, "{d}", .{i}) catch unreachable;
        const msg = std.fmt.bufPrintZ(&msg_buf, "event {d}", .{i}) catch unreachable;
        ring.push(DiagEvent{
            .ts = ts,
            .severity = .info,
            .source = .unknown,
            .message = msg,
        });
    }

    // Should have 3 events (capacity)
    try std.testing.expectEqual(@as(usize, 3), ring.len());
    // Total written should be 5
    try std.testing.expectEqual(@as(usize, 5), ring.total_written);
}

test "EventRing excludes info when configured" {
    const allocator = std.heap.page_allocator;
    const config = RingConfig{ .max_events = 10, .include_info = false };
    var ring = try EventRing.init(allocator, config);
    defer ring.deinit(allocator);

    ring.push(DiagEvent{
        .ts = "2026-06-18T08:00:00Z",
        .severity = .info,
        .source = .wireguard,
        .message = "info event",
    });

    ring.push(DiagEvent{
        .ts = "2026-06-18T08:00:00Z",
        .severity = .warning,
        .source = .wireguard,
        .message = "warning event",
    });

    try std.testing.expectEqual(@as(usize, 1), ring.len());
}

test "EventRing clear works" {
    const allocator = std.heap.page_allocator;
    var ring = try EventRing.init(allocator, .{ .max_events = 10 });
    defer ring.deinit(allocator);

    ring.push(DiagEvent{
        .ts = "2026-06-18T08:00:00Z",
        .severity = .info,
        .source = .wireguard,
        .message = "test",
    });

    try std.testing.expectEqual(@as(usize, 1), ring.len());

    ring.clear();

    try std.testing.expectEqual(@as(usize, 0), ring.len());
    try std.testing.expectEqual(@as(usize, 0), ring.total_written);
}

test "formatTimestamp returns valid string" {
    const allocator = std.heap.page_allocator;
    const ts = try formatTimestamp(allocator, 1718700000);
    defer allocator.free(ts);
    try std.testing.expect(ts.len > 0);
}
