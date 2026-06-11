// bgp/passive_listener_config_tests.zig — Config and shape tests for passive listener
//
// Tests for:
// - PassiveListenerConfig defaults and options
// - AcceptResult structure
// - PassiveListener struct defaults
// - ListenerState enum variants
// - ListenerError enum variants
// - Atomic flag behavior

const std = @import("std");
const testing = std.testing;
const passive_listener = @import("passive_listener.zig");

test "PassiveListenerConfig default port is 179" {
    const config = passive_listener.PassiveListenerConfig{
        .local_address = .{ 127, 0, 0, 1 },
    };
    try testing.expectEqual(@as(u16, 179), config.port);
}

test "PassiveListenerConfig default accept_timeout_ms is 1000" {
    const config = passive_listener.PassiveListenerConfig{
        .local_address = .{ 127, 0, 0, 1 },
    };
    try testing.expectEqual(@as(u32, 1000), config.accept_timeout_ms);
}

test "PassiveListenerConfig allowed_peer_address is null by default" {
    const config = passive_listener.PassiveListenerConfig{
        .local_address = .{ 127, 0, 0, 1 },
    };
    try testing.expectEqual(true, config.allowed_peer_address == null);
}

test "PassiveListenerConfig with allowed_peer_address" {
    const config = passive_listener.PassiveListenerConfig{
        .local_address = .{ 127, 0, 0, 1 },
        .allowed_peer_address = .{ 10, 0, 0, 2 },
    };
    try testing.expectEqual(false, config.allowed_peer_address == null);
    try testing.expectEqual(@as(u8, 10), config.allowed_peer_address.?[0]);
    try testing.expectEqual(@as(u8, 2), config.allowed_peer_address.?[3]);
}

test "PassiveListenerConfig with custom port" {
    const config = passive_listener.PassiveListenerConfig{
        .local_address = .{ 127, 0, 0, 1 },
        .port = 8179,
    };
    try testing.expectEqual(@as(u16, 8179), config.port);
}

test "PassiveListenerConfig with custom accept_timeout" {
    const config = passive_listener.PassiveListenerConfig{
        .local_address = .{ 127, 0, 0, 1 },
        .accept_timeout_ms = 500,
    };
    try testing.expectEqual(@as(u32, 500), config.accept_timeout_ms);
}

test "AcceptResult structure" {
    const result = passive_listener.AcceptResult{
        .socket_fd = 42,
        .peer_address = .{ 10, 0, 0, 2 },
        .peer_port = 179,
    };
    try testing.expectEqual(@as(std.c.fd_t, 42), result.socket_fd);
    try testing.expectEqual(@as(u8, 10), result.peer_address[0]);
    try testing.expectEqual(@as(u8, 0), result.peer_address[1]);
    try testing.expectEqual(@as(u8, 0), result.peer_address[2]);
    try testing.expectEqual(@as(u8, 2), result.peer_address[3]);
    try testing.expectEqual(@as(u16, 179), result.peer_port);
}

test "AcceptResult with various socket descriptors" {
    // Test with zero fd
    const result1 = passive_listener.AcceptResult{
        .socket_fd = 0,
        .peer_address = .{ 192, 168, 1, 100 },
        .peer_port = 179,
    };
    try testing.expectEqual(@as(std.c.fd_t, 0), result1.socket_fd);

    // Test with high fd
    const result2 = passive_listener.AcceptResult{
        .socket_fd = 65535,
        .peer_address = .{ 192, 168, 1, 100 },
        .peer_port = 179,
    };
    try testing.expectEqual(@as(std.c.fd_t, 65535), result2.socket_fd);

    // Test with -1 (invalid)
    const result3 = passive_listener.AcceptResult{
        .socket_fd = -1,
        .peer_address = .{ 192, 168, 1, 100 },
        .peer_port = 179,
    };
    try testing.expectEqual(@as(std.c.fd_t, -1), result3.socket_fd);
}

test "PassiveListener struct default values" {
    const listener = passive_listener.PassiveListener{
        .config = .{
            .local_address = .{ 127, 0, 0, 1 },
        },
    };
    try testing.expectEqual(@as(std.c.fd_t, -1), listener.listen_fd);
    try testing.expectEqual(@as(u8, 0), listener.cleanup_requested);
    try testing.expectEqual(@as(?std.Thread, null), listener.thread);
    try testing.expectEqual(@as(std.c.fd_t, -1), listener.accepted_fd);
    try testing.expectEqual(@as(u8, 0), listener.has_pending_accept);
    try testing.expectEqual(passive_listener.ListenerState.disabled, listener.state);
    try testing.expectEqual(true, listener.error_message == null);
}

test "ListenerState enum has expected variants" {
    // Verify all expected states exist
    try testing.expectEqual(@as(u8, 0), @intFromEnum(passive_listener.ListenerState.disabled));
    try testing.expectEqual(@as(u8, 1), @intFromEnum(passive_listener.ListenerState.bound));
    try testing.expectEqual(@as(u8, 2), @intFromEnum(passive_listener.ListenerState.thread_failed));
    try testing.expectEqual(@as(u8, 3), @intFromEnum(passive_listener.ListenerState.bind_failed));
}

test "PassiveListener state transitions" {
    var listener = passive_listener.PassiveListener{
        .config = .{
            .local_address = .{ 127, 0, 0, 1 },
        },
    };

    // Initial state should be disabled
    try testing.expectEqual(passive_listener.ListenerState.disabled, listener.state);

    // After bind failure, state should be bind_failed
    listener.state = .bind_failed;
    listener.error_message = "bind failed";
    try testing.expectEqual(passive_listener.ListenerState.bind_failed, listener.state);
    try testing.expectEqual(false, listener.error_message == null);

    // After thread failure, state should be thread_failed
    listener.state = .thread_failed;
    listener.error_message = "thread spawn failed";
    try testing.expectEqual(passive_listener.ListenerState.thread_failed, listener.state);
    try testing.expectEqual(false, listener.error_message == null);

    // After successful bind, state should be bound
    listener.state = .bound;
    listener.error_message = null;
    try testing.expectEqual(passive_listener.ListenerState.bound, listener.state);
    try testing.expectEqual(true, listener.error_message == null);
}

test "ListenerError enum has all expected variants" {
    // Verify error variants exist by checking they compile as valid error values
    const errors = .{
        passive_listener.ListenerError.SocketCreationFailed,
        passive_listener.ListenerError.BindFailed,
        passive_listener.ListenerError.ListenFailed,
        passive_listener.ListenerError.NonBlockingFailed,
        passive_listener.ListenerError.NotListening,
        passive_listener.ListenerError.NoPendingConnection,
        passive_listener.ListenerError.ThreadSpawnFailed,
    };
    _ = errors;
}

test "PassiveListener cleanup_requested atomic behavior" {
    var listener = passive_listener.PassiveListener{
        .config = .{
            .local_address = .{ 127, 0, 0, 1 },
        },
    };

    // Initial value should be 0
    try testing.expectEqual(@as(u8, 0), @atomicLoad(u8, &listener.cleanup_requested, .acquire));

    // Set to 1 (stop requested)
    @atomicStore(u8, &listener.cleanup_requested, 1, .release);
    try testing.expectEqual(@as(u8, 1), @atomicLoad(u8, &listener.cleanup_requested, .acquire));

    // Set back to 0 (for another test)
    @atomicStore(u8, &listener.cleanup_requested, 0, .release);
    try testing.expectEqual(@as(u8, 0), @atomicLoad(u8, &listener.cleanup_requested, .acquire));
}

test "PassiveListener has_pending_accept atomic behavior" {
    var listener = passive_listener.PassiveListener{
        .config = .{
            .local_address = .{ 127, 0, 0, 1 },
        },
    };

    // Initial value should be 0 (no pending)
    try testing.expectEqual(@as(u8, 0), @atomicLoad(u8, &listener.has_pending_accept, .acquire));
    try testing.expectEqual(false, passive_listener.hasPendingConnection(&listener));

    // Set to 1 (pending connection)
    @atomicStore(u8, &listener.has_pending_accept, 1, .release);
    try testing.expectEqual(@as(u8, 1), @atomicLoad(u8, &listener.has_pending_accept, .acquire));
    try testing.expectEqual(true, passive_listener.hasPendingConnection(&listener));

    // Clear pending
    @atomicStore(u8, &listener.has_pending_accept, 0, .release);
    try testing.expectEqual(@as(u8, 0), @atomicLoad(u8, &listener.has_pending_accept, .acquire));
    try testing.expectEqual(false, passive_listener.hasPendingConnection(&listener));
}
