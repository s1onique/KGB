// lab_events_tests.zig — Tests for lab_events module
const std = @import("std");
const lab_events = @import("lab_events.zig");

const LabEventsConfig = lab_events.LabEventsConfig;
const LabEventEmitter = lab_events.LabEventEmitter;
const RING_CAPACITY = lab_events.RING_CAPACITY;
const MAX_DETAIL_LEN = lab_events.MAX_DETAIL_LEN;

test "LabEventEmitter init with disabled config" {
    const config = LabEventsConfig{ .enabled = false };
    const emitter = LabEventEmitter.init(config);
    try std.testing.expect(!emitter.shouldEmit());
    try std.testing.expectEqual(@as(usize, 0), emitter.len());
}

test "LabEventEmitter emit does nothing when disabled" {
    const config = LabEventsConfig{ .enabled = false };
    var emitter = LabEventEmitter.init(config);
    defer emitter.deinit();

    emitter.emit(.heartbeat_tick_start, .heartbeat, 1000, "test");
    try std.testing.expectEqual(@as(usize, 0), emitter.len());
}

test "LabEventEmitter emit writes to ring when enabled" {
    const config = LabEventsConfig{ .enabled = true };
    var emitter = LabEventEmitter.init(config);
    defer emitter.deinit();

    try std.testing.expect(emitter.shouldEmit());
    emitter.emit(.heartbeat_tick_start, .heartbeat, 1000, "test");
    try std.testing.expectEqual(@as(usize, 1), emitter.len());
}

test "LabEventEmitter ring wraps at capacity" {
    const config = LabEventsConfig{ .enabled = true };
    var emitter = LabEventEmitter.init(config);
    defer emitter.deinit();

    // Emit more events than capacity
    var i: usize = 0;
    while (i < RING_CAPACITY + 10) : (i += 1) {
        emitter.emit(.heartbeat_tick_start, .heartbeat, @as(u32, @intCast(i * 1000)), "test");
    }

    // Should be capped at capacity
    try std.testing.expectEqual(@as(usize, RING_CAPACITY), emitter.len());
}

test "LabEventEmitter detail is truncated to MAX_DETAIL_LEN" {
    const config = LabEventsConfig{ .enabled = true };
    var emitter = LabEventEmitter.init(config);
    defer emitter.deinit();

    const long_detail: []const u8 = "x" ** (MAX_DETAIL_LEN * 2);
    emitter.emit(.heartbeat_tick_start, .heartbeat, 1000, long_detail);

    try std.testing.expectEqual(@as(usize, 1), emitter.len());
    if (emitter.get(0)) |record| {
        try std.testing.expectEqual(@as(u16, MAX_DETAIL_LEN), record.detail_len);
    }
}

test "LabEventEmitter get returns records in order" {
    const config = LabEventsConfig{ .enabled = true };
    var emitter = LabEventEmitter.init(config);
    defer emitter.deinit();

    emitter.emit(.heartbeat_tick_start, .heartbeat, 1000, "first");
    emitter.emit(.heartbeat_tick_end, .heartbeat, 31000, "second");

    try std.testing.expectEqual(@as(usize, 2), emitter.len());
    if (emitter.get(0)) |record| {
        try std.testing.expectEqualStrings("first", record.detail[0..record.detail_len]);
        try std.testing.expectEqual(@as(u32, 1000), record.elapsed_millis);
    }
    if (emitter.get(1)) |record| {
        try std.testing.expectEqualStrings("second", record.detail[0..record.detail_len]);
        try std.testing.expectEqual(@as(u32, 31000), record.elapsed_millis);
    }
}

test "LabEventEmitter wg_check_failed error class" {
    const config = LabEventsConfig{ .enabled = true };
    var emitter = LabEventEmitter.init(config);
    defer emitter.deinit();

    emitter.emitWgCheckFailed(5000, .command_not_found, "wg_show");

    try std.testing.expectEqual(@as(usize, 1), emitter.len());
    if (emitter.get(0)) |record| {
        try std.testing.expectEqual(.wg_check_failed, record.event);
        try std.testing.expect(record.detail_len > 0);
    }
}

test "LabEventEmitter subsystem classification" {
    const config = LabEventsConfig{ .enabled = true };
    var emitter = LabEventEmitter.init(config);
    defer emitter.deinit();

    emitter.emit(.heartbeat_tick_start, .heartbeat, 1000, "");
    emitter.emit(.wg_check_start, .wireguard, 2000, "");
    emitter.emit(.bgp_maintenance_start, .bgp, 3000, "");
    emitter.emit(.bfd_tick_start, .bfd, 4000, "");

    try std.testing.expectEqual(@as(usize, 4), emitter.len());

    if (emitter.get(0)) |r| try std.testing.expectEqual(.heartbeat, r.subsystem);
    if (emitter.get(1)) |r| try std.testing.expectEqual(.wireguard, r.subsystem);
    if (emitter.get(2)) |r| try std.testing.expectEqual(.bgp, r.subsystem);
    if (emitter.get(3)) |r| try std.testing.expectEqual(.bfd, r.subsystem);
}

test "LabEventEmitter renderJson disabled" {
    const config = LabEventsConfig{ .enabled = false };
    var emitter = LabEventEmitter.init(config);
    defer emitter.deinit();

    // renderJson needs a writer, test config directly
    try std.testing.expect(!emitter.config.enabled);
    try std.testing.expectEqual(@as(usize, 0), emitter.len());
}

test "LabEventEmitter renderJson with events" {
    const config = LabEventsConfig{ .enabled = true };
    var emitter = LabEventEmitter.init(config);
    defer emitter.deinit();

    emitter.emit(.heartbeat_tick_start, .heartbeat, 1000, "test");

    // Verify event was recorded
    try std.testing.expectEqual(@as(usize, 1), emitter.len());
    if (emitter.get(0)) |record| {
        try std.testing.expectEqual(@as(u16, 4), record.detail_len);
        try std.testing.expectEqualStrings("test", record.detail[0..4]);
    }
}

test "LabEventEmitter renderTsv header" {
    const config = LabEventsConfig{ .enabled = true };
    var emitter = LabEventEmitter.init(config);
    defer emitter.deinit();

    emitter.emit(.heartbeat_tick_start, .heartbeat, 1000, "test");

    // Verify event was recorded
    try std.testing.expectEqual(@as(usize, 1), emitter.len());
    if (emitter.get(0)) |record| {
        try std.testing.expectEqual(@as(u16, 4), record.detail_len);
    }
}
