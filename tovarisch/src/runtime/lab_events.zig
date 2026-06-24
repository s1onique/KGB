// runtime/lab_events.zig — Tovarisch-native lab event ring buffer
//
// Low-overhead bounded event stream for idle staircase memory lab attribution.
// Events are emitted by real tovarisch runtime paths (heartbeat, WG checks,
// BGP/BFD maintenance) and can be used to correlate memory steps with actual
// subsystem behavior.
//
// Design constraints:
// - Bounded ring buffer (no unbounded allocation)
// - Event emission is a no-op when disabled (zero overhead)
// - Details are bounded strings (no unbounded command output)
// - Process PID included for multi-process lab scenarios
// - Monotonic elapsed time (not wall clock) for determinism

const std = @import("std");
const c = std.c;

/// Maximum number of events in the ring buffer.
pub const RING_CAPACITY: usize = 256;

/// Maximum detail string length (prevents unbounded output).
pub const MAX_DETAIL_LEN: usize = 128;

/// Lab event names for attribution.
pub const EventName = enum(u8) {
    heartbeat_tick_start = 0,
    heartbeat_tick_end = 1,
    heartbeat_tick_failed = 2,
    wg_check_start = 10,
    wg_check_end = 11,
    wg_check_failed = 12,
    health_collect_start = 20,
    health_collect_end = 21,
    health_collect_failed = 22,
    bgp_maintenance_start = 30,
    bgp_maintenance_end = 31,
    bgp_maintenance_failed = 32,
    bgp_reconnect_start = 33,
    bgp_reconnect_end = 34,
    bfd_tick_start = 40,
    bfd_tick_end = 41,
    bfd_tick_failed = 42,
    status_request_start = 50,
    status_request_end = 51,
    log_emit = 60,
};

/// Subsystem classification for attribution.
pub const Subsystem = enum(u8) {
    heartbeat = 0,
    wireguard = 1,
    health = 2,
    bgp = 3,
    bfd = 4,
    status = 5,
    logging = 6,
};

/// WireGuard error classification for wg_check_failed detail.
pub const WgErrorClass = enum(u8) {
    command_not_found = 0,
    command_failed = 1,
    parse_failed = 2,
    timeout = 3,
};

/// Single lab event record.
const EventRecord = struct {
    const Self = @This();
    event: EventName,
    subsystem: Subsystem,
    elapsed_millis: u32,
    detail_len: u16,
    detail: [MAX_DETAIL_LEN]u8,
    pid: u32,

    fn empty() Self {
        return .{
            .event = .heartbeat_tick_start,
            .subsystem = .heartbeat,
            .elapsed_millis = 0,
            .detail_len = 0,
            .detail = [_]u8{0} ** MAX_DETAIL_LEN,
            .pid = 0,
        };
    }
};

/// Lab event ring buffer configuration.
pub const LabEventsConfig = struct {
    enabled: bool = false,
    pid: u32 = 0,
    output_path: []const u8 = "",
};

/// Lab event emitter with bounded ring buffer.
pub const LabEventEmitter = struct {
    const Self = @This();

    config: LabEventsConfig,
    ring: [RING_CAPACITY]EventRecord = undefined,
    head: usize = 0,
    count: usize = 0,
    output_file: ?c_int = null,

    pub fn init(config: LabEventsConfig) Self {
        var emitter = Self{
            .config = config,
            .output_file = null,
        };
        for (&emitter.ring) |*record| {
            record.* = EventRecord.empty();
        }
        if (config.enabled and config.output_path.len > 0) {
            emitter.openOutputFile();
        }
        return emitter;
    }

    fn openOutputFile(self: *Self) void {
        if (self.config.output_path.len == 0) return;
        var path_buf: [4096]u8 = undefined;
        if (self.config.output_path.len >= path_buf.len) return;
        // MemoryCopySafety: src=dynamic dst=stack buf=4096
        @memcpy(path_buf[0..self.config.output_path.len], self.config.output_path);
        path_buf[self.config.output_path.len] = 0;
        // O_LARGEFILE | O_WRONLY | O_CREAT | O_TRUNC
        const flags = @as(std.c.O, @bitCast(@as(u32, 0o100000 | 0o1 | 0o100 | 0o1000)));
        const fd = std.c.open(
            @as([*:0]const u8, @ptrCast(&path_buf)),
            flags,
            @as(c_uint, 0o644),
        );
        if (fd >= 0) {
            self.output_file = fd;
            const header = "timestamp\telapsed_millis\tevent\tsubsystem\tdetail\tpid\n";
            _ = std.c.write(fd, @ptrCast(header), header.len);
        }
        // Note: caller handles logging of open failures via lab_native_events_open_failed
    }

    pub fn deinit(self: *Self) void {
        if (self.output_file) |fd| {
            _ = std.c.close(fd);
            self.output_file = null;
        }
    }

    pub fn shouldEmit(self: *const Self) bool {
        return self.config.enabled;
    }

    fn getPid(self: *const Self) u32 {
        if (self.config.pid != 0) return self.config.pid;
        return @intCast(std.c.getpid());
    }

    /// Emits a lab event with caller-provided elapsed milliseconds.
    /// 
    /// DESIGN: Elapsed time is passed from the caller (e.g., heartbeat thread's uptime_seconds * 1000)
    /// rather than derived from event count. This ensures timestamps reflect real elapsed time,
    /// not event-count-based synthetic time that could create false correlations.
    pub fn emit(self: *Self, event: EventName, subsystem: Subsystem, elapsed_millis: u32, detail: []const u8) void {
        if (!self.config.enabled) return;
        const pid = self.getPid();
        const detail_len: u16 = @intCast(@min(detail.len, MAX_DETAIL_LEN));
        var record = EventRecord{
            .event = event,
            .subsystem = subsystem,
            .elapsed_millis = elapsed_millis,
            .detail_len = detail_len,
            .detail = [_]u8{0} ** MAX_DETAIL_LEN,
            .pid = pid,
        };
        // MemoryCopySafety: src=bounded dst=fixed buf[detail_len]
        @memcpy(record.detail[0..detail_len], detail[0..detail_len]);
        self.ring[self.head] = record;
        self.head = (self.head + 1) % RING_CAPACITY;
        if (self.count < RING_CAPACITY) self.count += 1;
        self.writeEventToFile(&record);
    }

    pub fn emitHeartbeatStart(self: *Self, elapsed_millis: u32) void { self.emit(.heartbeat_tick_start, .heartbeat, elapsed_millis, ""); }
    pub fn emitHeartbeatEnd(self: *Self, elapsed_millis: u32) void { self.emit(.heartbeat_tick_end, .heartbeat, elapsed_millis, ""); }
    pub fn emitHeartbeatFailed(self: *Self, elapsed_millis: u32, detail: []const u8) void { self.emit(.heartbeat_tick_failed, .heartbeat, elapsed_millis, detail); }
    pub fn emitWgCheckStart(self: *Self, elapsed_millis: u32) void { self.emit(.wg_check_start, .wireguard, elapsed_millis, ""); }
    pub fn emitWgCheckEnd(self: *Self, elapsed_millis: u32) void { self.emit(.wg_check_end, .wireguard, elapsed_millis, ""); }
    pub fn emitWgCheckFailed(self: *Self, elapsed_millis: u32, error_class: WgErrorClass, detail: []const u8) void {
        var buf: [MAX_DETAIL_LEN]u8 = undefined;
        const class_str = switch (error_class) {
            .command_not_found => "command_not_found",
            .command_failed => "command_failed",
            .parse_failed => "parse_failed",
            .timeout => "timeout",
        };
        const dlen = @min(class_str.len + 1 + detail.len, MAX_DETAIL_LEN);
        // MemoryCopySafety: src=const_str dst=stack buf[MAX_DETAIL_LEN]
        @memcpy(buf[0..class_str.len], class_str);
        if (detail.len > 0 and class_str.len + 1 < MAX_DETAIL_LEN) {
            buf[class_str.len] = ':';
            // MemoryCopySafety: src=bounded dst=stack buf[dlen]
            @memcpy(buf[class_str.len + 1 .. dlen], detail[0..dlen - class_str.len - 1]);
        }
        self.emit(.wg_check_failed, .wireguard, elapsed_millis, buf[0..dlen]);
    }
    pub fn emitHealthCollectStart(self: *Self, elapsed_millis: u32) void { self.emit(.health_collect_start, .health, elapsed_millis, ""); }
    pub fn emitHealthCollectEnd(self: *Self, elapsed_millis: u32) void { self.emit(.health_collect_end, .health, elapsed_millis, ""); }
    pub fn emitHealthCollectFailed(self: *Self, elapsed_millis: u32, detail: []const u8) void { self.emit(.health_collect_failed, .health, elapsed_millis, detail); }
    pub fn emitBgpMaintenanceStart(self: *Self, elapsed_millis: u32) void { self.emit(.bgp_maintenance_start, .bgp, elapsed_millis, ""); }
    pub fn emitBgpMaintenanceEnd(self: *Self, elapsed_millis: u32) void { self.emit(.bgp_maintenance_end, .bgp, elapsed_millis, ""); }
    pub fn emitBgpMaintenanceFailed(self: *Self, elapsed_millis: u32, detail: []const u8) void { self.emit(.bgp_maintenance_failed, .bgp, elapsed_millis, detail); }
    pub fn emitBgpReconnectStart(self: *Self, elapsed_millis: u32) void { self.emit(.bgp_reconnect_start, .bgp, elapsed_millis, ""); }
    pub fn emitBgpReconnectEnd(self: *Self, elapsed_millis: u32) void { self.emit(.bgp_reconnect_end, .bgp, elapsed_millis, ""); }
    pub fn emitBfdTickStart(self: *Self, elapsed_millis: u32) void { self.emit(.bfd_tick_start, .bfd, elapsed_millis, ""); }
    pub fn emitBfdTickEnd(self: *Self, elapsed_millis: u32) void { self.emit(.bfd_tick_end, .bfd, elapsed_millis, ""); }
    pub fn emitBfdTickFailed(self: *Self, elapsed_millis: u32, detail: []const u8) void { self.emit(.bfd_tick_failed, .bfd, elapsed_millis, detail); }
    pub fn emitStatusRequestStart(self: *Self, elapsed_millis: u32) void { self.emit(.status_request_start, .status, elapsed_millis, ""); }
    pub fn emitStatusRequestEnd(self: *Self, elapsed_millis: u32) void { self.emit(.status_request_end, .status, elapsed_millis, ""); }

    fn writeEventToFile(self: *Self, record: *const EventRecord) void {
        const fd = self.output_file orelse return;
        const ts = "2026-01-01T00:00:00.000Z";
        const detail_slice = record.detail[0..record.detail_len];
        var line_buf: [512]u8 = undefined;
        const line = std.fmt.bufPrint(
            &line_buf,
            "{s}\t{d}\t{s}\t{s}\t{s}\t{d}\n",
            .{ ts, record.elapsed_millis, @tagName(record.event), @tagName(record.subsystem), detail_slice, record.pid },
        ) catch return;
        _ = std.c.write(fd, @ptrCast(line.ptr), line.len);
    }

    pub fn len(self: *const Self) usize { return self.count; }

    pub fn get(self: *const Self, index: usize) ?*const EventRecord {
        if (index >= self.count) return null;
        const idx = if (self.count >= RING_CAPACITY) self.head + index else index;
        return &self.ring[idx % RING_CAPACITY];
    }

    pub fn renderJson(self: *const Self, writer: anytype) !void {
        if (!self.config.enabled) {
            try writer.writeAll("{\"native_events_enabled\":false,\"event_count\":0,\"events\":[]}");
            return;
        }
        var num_buf: [32]u8 = undefined;
        try writer.writeAll("{\"native_events_enabled\":true,\"event_count\":");
        const count_slice = std.fmt.bufPrint(&num_buf, "{}", .{self.count}) catch return;
        try writer.writeAll(count_slice);
        try writer.writeAll(",\"events\":[");
        for (0..self.count) |i| {
            if (i > 0) try writer.writeAll(",");
            if (self.get(i)) |record| {
                try writer.writeAll("{\"elapsed_millis\":");
                const e = std.fmt.bufPrint(&num_buf, "{}", .{record.elapsed_millis}) catch return;
                try writer.writeAll(e);
                try writer.writeAll(",\"event\":\"");
                try writer.writeAll(@tagName(record.event));
                try writer.writeAll("\",\"subsystem\":\"");
                try writer.writeAll(@tagName(record.subsystem));
                try writer.writeAll("\",\"detail\":\"");
                try writer.writeAll(record.detail[0..record.detail_len]);
                try writer.writeAll("\",\"pid\":");
                const p = std.fmt.bufPrint(&num_buf, "{}", .{record.pid}) catch return;
                try writer.writeAll(p);
                try writer.writeAll("}");
            }
        }
        try writer.writeAll("]}");
    }

    pub fn renderTsv(self: *const Self, writer: anytype) !void {
        try writer.writeAll("timestamp\telapsed_millis\tevent\tsubsystem\tdetail\tpid\n");
        const ts = "2026-01-01T00:00:00.000Z";
        var num_buf: [32]u8 = undefined;
        for (0..self.count) |i| {
            if (self.get(i)) |record| {
                try writer.writeAll(ts);
                try writer.writeAll("\t");
                const e = std.fmt.bufPrint(&num_buf, "{}", .{record.elapsed_millis}) catch return;
                try writer.writeAll(e);
                try writer.writeAll("\t");
                try writer.writeAll(@tagName(record.event));
                try writer.writeAll("\t");
                try writer.writeAll(@tagName(record.subsystem));
                try writer.writeAll("\t");
                try writer.writeAll(record.detail[0..record.detail_len]);
                try writer.writeAll("\t");
                const p = std.fmt.bufPrint(&num_buf, "{}", .{record.pid}) catch return;
                try writer.writeAll(p);
                try writer.writeAll("\n");
            }
        }
    }
};
