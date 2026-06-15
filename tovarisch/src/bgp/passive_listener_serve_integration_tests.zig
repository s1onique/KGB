// bgp/passive_listener_serve_integration_tests.zig — Passive listener integration regression tests
//
// REGRESSION: Configured bundle with local_address starts passive listener.
// This test verifies that passive_listener_integration.createPassiveListener()
// is called when local_address is present, causing bundle.passive_listener to be non-null.

const std = @import("std");
const config = @import("../config.zig");
const config_parse = @import("config_parse.zig");
const serve_integration = @import("serve_integration.zig");
const session = @import("session.zig");
const transport = @import("transport.zig");
const passive_listener = @import("passive_listener.zig");
const passive_listener_integration = @import("passive_listener_integration.zig");

const VoidWriter = struct {
    const Self = @This();
    pub fn writeAll(_: Self, _: []const u8) error{}!void {}
    pub fn write(_: Self, _: []const u8) error{}!void {}
    pub fn print(_: Self, _: []const u8, _: anytype) error{}!void {}
};

// Fake transport for passive listener test
const FakeTransport = struct {
    const Self = @This();
    closed: bool = false,
    pub fn init() Self {
        return Self{};
    }
    pub fn send(_: *Self, _: []const u8) transport.TransportError!void {}
    pub fn recv(_: *Self) []const u8 {
        return &[_]u8{};
    }
    pub fn close(_: *Self) void {}
    pub fn toTransport(self: *Self) transport.Transport {
        return transport.Transport{
            .sendFn = struct {
                fn send(ctx: *anyopaque, data: []const u8) transport.TransportError!void {
                    const fake: *Self = @ptrCast(@alignCast(ctx));
                    return fake.send(data);
                }
            }.send,
            .recvFn = struct {
                fn recv(ctx: *anyopaque) []const u8 {
                    const fake: *Self = @ptrCast(@alignCast(ctx));
                    return fake.recv();
                }
            }.recv,
            .closeFn = struct {
                fn close(ctx: *anyopaque) void {
                    const fake: *Self = @ptrCast(@alignCast(ctx));
                    fake.close();
                }
            }.close,
            .isClosedFn = struct {
                fn isClosed(ctx: *anyopaque) bool {
                    const fake: *Self = @ptrCast(@alignCast(ctx));
                    return fake.closed;
                }
            }.isClosed,
            .ctx = @ptrCast(self),
        };
    }
};

// REGRESSION: Configured bundle with local_address stores passive_listener.
test "REGRESSION: configured bundle with local_address stores passive_listener" {
    // Simulate bundle creation with passive listener via the integration path.
    // This test proves the regression: passive listener is created when local_address is configured.
    //
    // The test creates a bundle similar to loadConfigAndBgp() and verifies:
    // 1. When session_config.local_address is present, passive_listener is non-null
    // 2. The passive listener state reflects success or failure (bound vs bind_failed)

    var raw = config.RawConfig{};
    defer raw.deinit(std.heap.page_allocator);
    try raw.put(std.heap.page_allocator, "bgp", .{});
    const bgp_section = raw.getPtr("bgp").?;
    try bgp_section.put(std.heap.page_allocator, "enabled", "true");
    try bgp_section.put(std.heap.page_allocator, "local_address", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "router_id", "10.0.0.1");
    try bgp_section.put(std.heap.page_allocator, "local_as", "65001");
    try bgp_section.put(std.heap.page_allocator, "peer_address", "10.0.0.2");
    try bgp_section.put(std.heap.page_allocator, "peer_as", "65002");
    try bgp_section.put(std.heap.page_allocator, "advertised_prefixes", "");

    const cfg = try config_parse.parseBgpConfig(&raw);
    try std.testing.expect(cfg.enabled);
    try std.testing.expect(cfg.local_address.len > 0);

    const local_addr = try config_parse.parseIpv4Address(cfg.local_address);
    const peer_addr = try config_parse.parseIpv4Address(cfg.peer_address);
    const router_addr = try config_parse.parseIpv4Address(cfg.router_id);

    const session_config = session.SessionConfig{
        .peer_address = peer_addr,
        .peer_port = 179,
        .local_address = local_addr,
        .local_as = cfg.local_as,
        .peer_as = cfg.peer_as,
        .router_id = router_addr,
        .hold_time_seconds = 180,
        .keepalive_seconds = 60,
        .connect_timeout_ms = 5000,
        .prefixes = &.{},
        .same_as = cfg.same_as,
    };

    // Create a fake transport for testing (simulating successful TCP connect)
    var fake_transport = FakeTransport.init();
    const fake_tport = fake_transport.toTransport();

    // Create the bundle on the heap
    const bundle = std.heap.page_allocator.create(serve_integration.BgpServeBundle) catch {
        fake_transport.close();
        return error.SkipZigTest;
    };
    defer {
        // Clean up passive listener first (this is the key cleanup we're testing)
        if (bundle.passive_listener) |*listener| {
            passive_listener.close(listener);
        }
        std.heap.page_allocator.destroy(bundle);
    }

    bundle.* = serve_integration.BgpServeBundle{
        .raw = raw,
        .bgp_config = cfg,
        .session_config = session_config,
        .state = .not_configured,
        .last_error = null,
        .prefixes = &.{},
        .tcp = undefined,
        .trans = fake_tport,
        .sess = undefined,
    };

    // Verify bundle before passive listener creation
    try std.testing.expect(bundle.passive_listener == null);

    // Create passive listener via integration (this is what loadConfigAndBgp() now calls)
    const w = VoidWriter{};
    passive_listener_integration.createPassiveListener(bundle, w);

    // REGRESSION ASSERTION: passive_listener must be non-null after createPassiveListener()
    try std.testing.expect(bundle.passive_listener != null);

    // Verify the listener is in a valid state (bound or bind_failed)
    if (bundle.passive_listener) |listener| {
        try std.testing.expect(listener.state == .bound or listener.state == .bind_failed);
    }

    fake_transport.close();
}
