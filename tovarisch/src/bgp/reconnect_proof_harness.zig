// bgp/reconnect_proof_harness.zig — shared harness and connector used by
// the bounded-memory proof split.
//
// The harness owns ONE `ReconnectMemoryState` (the authoritative oracle)
// and ONE deterministic connector. The connector is test-only:
//   * `acquire` returns a `ReconnectHandle` whose release_fn decrements
//     a `physical_resource` counter held on the connector itself. This
//     counter is asserted separately and is NOT the generation-boundary
//     authority — the state's counters are.

const std = @import("std");
const allocation_tracker = @import("../runtime/allocation_tracker.zig");
const clock = @import("clock.zig");
const reconnect_stress = @import("reconnect_stress_support.zig");
const serve_integration = @import("serve_integration.zig");
const session = @import("session.zig");
const tcp_transport = @import("tcp_transport.zig");
const types = @import("types.zig");

pub const STRESS_TEST_GENERATIONS: u64 = reconnect_stress.STRESS_TEST_GENERATIONS;
pub const WARMUP_GENERATIONS: u64 = allocation_tracker.warmup_generation_count;
pub const reconnect_boundary_socket_baseline: u32 = 0;
pub const RECONNECT_PEAK_SOCKET_BOUND: u32 = 1;

const ReconnectError = error{
    ExpectedConnectionFailure,
    UnexpectedConnectionError,
};

fn determRelease(ptr: *anyopaque) void {
    const self: *DeterministicConnector = @ptrCast(@alignCast(ptr));
    if (self.physical_resource_live == 0) @panic("determRelease: physical resource underflow");
    self.physical_resource_live -= 1;
    self.physical_resource_released = std.math.add(u64, self.physical_resource_released, 1)
        catch @panic("determ released_total overflow");
}

/// Deterministic connector: `acquire` always succeeds and returns a
/// handle whose release decrements the test-only `physical_resource`
/// counter. `finish` returns a dummy transport when `succeed_next` is
/// set or fails with `ConnectionFailed` otherwise.
pub const DeterministicConnector = struct {
    connect_attempts: u64 = 0,
    succeed_next: bool = false,
    /// Test-only physical-resource counter, NOT the generation boundary
    /// authority. The harness asserts it matches the state-derived
    /// counters where appropriate.
    physical_resource_live: u32 = 0,
    physical_resource_acquired: u64 = 0,
    physical_resource_released: u64 = 0,

    fn acquire(ctx: ?*anyopaque, _: tcp_transport.TcpTransportConfig) anyerror!allocation_tracker.ReconnectHandle {
        const self: *DeterministicConnector = @ptrCast(@alignCast(ctx.?));
        self.connect_attempts = std.math.add(u64, self.connect_attempts, 1) catch @panic("connect attempts overflow");
        self.physical_resource_acquired = std.math.add(u64, self.physical_resource_acquired, 1) catch @panic("physical acquired overflow");
        self.physical_resource_live = std.math.add(u32, self.physical_resource_live, 1) catch @panic("physical live overflow");
        // The resource points at the connector itself; release_fn reads
        // back through the same struct.
        return allocation_tracker.ReconnectHandle{
            .ptr = @ptrCast(self),
            .release_fn = determRelease,
        };
    }

    fn finish(ctx: ?*anyopaque, _: allocation_tracker.ReconnectHandle, _: tcp_transport.TcpTransportConfig) anyerror!tcp_transport.TcpTransport {
        const self: *DeterministicConnector = @ptrCast(@alignCast(ctx.?));
        if (self.succeed_next) {
            self.succeed_next = false;
            return tcp_transport.TcpTransport{
                .socket_fd = -1,
                .recv_buf = undefined,
                .recv_len = 0,
                .closed = false,
                .peer_address = .{ 127, 0, 0, 1 },
                .peer_port = 179,
            };
        }
        return error.ConnectionFailed;
    }
};

/// Production-shaped bundle owner. The subsystem tracker is the only
/// classified allocator — the production reconnect path itself is
/// allocation-free under this proof's connector seam.
pub const ProductionReconnectHarness = struct {
    state: *allocation_tracker.ReconnectMemoryState = undefined,
    subsystem_allocator: std.mem.Allocator = undefined,
    connector: DeterministicConnector = .{},
    faults: serve_integration.ReconnectFaultPlan = .{},
    prefixes: [1]types.Ipv4Prefix = .{types.Ipv4Prefix.init("10.0.0.0/8")},
    bundle: ?*serve_integration.BgpServeBundle = null,

    /// Initialize the harness. The state is created via `init(allocator)`
    /// and is the SOLE authority for connector-handle accounting.
    pub fn init(self: *ProductionReconnectHarness, allocator: std.mem.Allocator) !void {
        self.state = try allocation_tracker.init(allocator);
        self.subsystem_allocator = try allocation_tracker.trackingAllocator(
            self.state,
            allocator,
            .bgp_subsystem,
            .permanent,
        );
    }

    pub fn setup(self: *ProductionReconnectHarness) !void {
        const bundle = try self.subsystem_allocator.create(serve_integration.BgpServeBundle);
        errdefer self.subsystem_allocator.destroy(bundle);

        const config = reconnect_stress.makeTestSessionConfig();
        bundle.* = .{
            .raw = undefined,
            .bgp_config = undefined,
            .session_config = config,
            .state = .configured,
            .prefixes = self.prefixes[0..],
            .tcp = undefined,
            .trans = undefined,
            .sess = undefined,
            .allocator = self.subsystem_allocator,
            .reconnect_memory_state = self.state,
            .reconnect_faults = &self.faults,
            .reconnect_connector = .{
                .ctx = @ptrCast(&self.connector),
                .acquireFn = DeterministicConnector.acquire,
                .finishFn = DeterministicConnector.finish,
            },
            .reconnect_clock = clock.MockClock.interface(),
        };
        bundle.tcp = .{
            .socket_fd = -1,
            .recv_buf = undefined,
            .recv_len = 0,
            .closed = true,
            .peer_address = .{ 127, 0, 0, 1 },
            .peer_port = 179,
        };
        bundle.trans = bundle.tcp.toTransport();
        bundle.sess = try session.initWithClock(config, &bundle.trans, clock.MockClock.interface());
        self.bundle = bundle;
    }

    pub fn deinit(self: *ProductionReconnectHarness, allocator: std.mem.Allocator) void {
        if (self.bundle) |bundle| {
            self.faults = .{};
            serve_integration.cancelReconnectTimer(bundle);
            serve_integration.closeForReconnect(bundle);
            self.subsystem_allocator.destroy(bundle);
            self.bundle = null;
        }
        allocation_tracker.deinit(self.state, allocator);
    }

    pub fn runOneFailedReconnectGeneration(self: *ProductionReconnectHarness) !void {
        const bundle = self.bundle.?;
        const test_clock = clock.MockClock.interface();
        serve_integration.scheduleReconnect(bundle, test_clock, serve_integration.DEFAULT_RECONNECT_MAX_MS);
        clock.MockClock.setTime(bundle.reconnect_deadline);
        if (!serve_integration.isReconnectReady(bundle, test_clock)) return error.ReconnectNotReady;
        if (serve_integration.runReconnectAttempt(bundle, test_clock)) |_| return ReconnectError.ExpectedConnectionFailure else |err| if (err != error.ConnectionFailed) return ReconnectError.UnexpectedConnectionError;
    }

    pub fn runOneSuccessfulReconnect(self: *ProductionReconnectHarness) !void {
        const bundle = self.bundle.?;
        self.connector.succeed_next = true;
        const test_clock = clock.MockClock.interface();
        serve_integration.scheduleReconnect(bundle, test_clock, serve_integration.DEFAULT_RECONNECT_MAX_MS);
        clock.MockClock.setTime(bundle.reconnect_deadline);
        if (!serve_integration.isReconnectReady(bundle, test_clock)) return error.ReconnectNotReady;
        try serve_integration.runReconnectAttempt(bundle, test_clock);
    }
};

pub fn expectSessionBaseline(bundle: *serve_integration.BgpServeBundle) !void {
    try std.testing.expectEqual(session.SessionState.idle, bundle.sess.status.state);
    try std.testing.expect(bundle.tcp.isClosed());
    try std.testing.expect(!bundle.socket_owned);
    try std.testing.expect(bundle.sess.trans == &bundle.trans);
    try std.testing.expect(bundle.sess.peer_open == null);
    try std.testing.expectEqual(@as(usize, 0), bundle.sess.recv_len);
    try std.testing.expectEqual(@as(usize, 0), bundle.sess.send_pos);
    try std.testing.expectEqual(@as(u16, 0), bundle.sess.negotiated_hold_time);
    try std.testing.expectEqual(@as(u32, 0), bundle.sess.keepalive_interval_ms);
    try std.testing.expectEqual(@as(u64, 0), bundle.sess.hold_timer_deadline);
    try std.testing.expect(!bundle.sess.pending_keepalive);
    try std.testing.expectEqual(@as(u64, 0), bundle.sess.pending_keepalive_ms);
    try std.testing.expectEqual(@as(usize, 0), bundle.sess.export_batch_index);
    try std.testing.expect(!bundle.sess.export_complete);
    try std.testing.expectEqual(@as(usize, 0), bundle.sess.nlri_sent_count);
    try std.testing.expectEqual(session.UpdateDiagnostic.none, bundle.sess.last_update_diagnostic);
    try std.testing.expect(bundle.sess.status.last_error == null);
    try std.testing.expect(bundle.sess.status.last_notification_code == null);
    try std.testing.expect(bundle.sess.status.last_notification_subcode == null);
    try std.testing.expect(!bundle.reconnect_timer_active);
}

pub fn expectBoundary(
    state: *const allocation_tracker.ReconnectMemoryState,
    bundle: *serve_integration.BgpServeBundle,
) !void {
    try expectSessionBaseline(bundle);
    const memory = allocation_tracker.memorySnapshot(state);
    const resources = allocation_tracker.resourceSnapshot(state);
    const handles = allocation_tracker.handleSnapshot(state);
    try std.testing.expectEqual(@as(i128, 0), memory.baseline_delta_bytes);
    try std.testing.expectEqual(@as(u64, 0), memory.reconnect_live_bytes);
    try std.testing.expectEqual(@as(u64, 0), allocation_tracker.liveBytesForLifetime(state, .operation));
    try std.testing.expectEqual(@as(u64, 0), allocation_tracker.currentAllocationsForLifetime(state, .reconnect_generation));
    try std.testing.expectEqual(@as(u64, 0), allocation_tracker.currentAllocationsForLifetime(state, .operation));
    try std.testing.expectEqual(reconnect_boundary_socket_baseline, resources.active_sockets);
    try std.testing.expectEqual(@as(u32, 0), resources.active_timers);
    try std.testing.expect(resources.error_history_count <= resources.error_history_capacity);
    try std.testing.expect(resources.retry_collection_count <= resources.retry_collection_capacity);
    try std.testing.expectEqual(@as(i128, 0), handles.delta);
    try std.testing.expect(handles.active_handle == null);
}
