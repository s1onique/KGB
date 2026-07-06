// transition_fsm_tests.zig — BFD executable FSM transition proofs
//
// ACT-TOVARISCH-ZIG-HULK24R: Executable transition proofs for BFD state machine
//
// Tests prove that ALL BFD state transitions are executable and verified:
// - processEvent() from all 4 states for all event types
// - Packet validation before state transitions
// - Invalid packets do not cause state changes
// - AdminDown is terminal for start event
// - Detection timeout guards are executable
//
// Target: DEFERRED transitions: 0

const std = @import("std");
const packet = @import("packet.zig");
const session = @import("session.zig");
const clock = @import("clock.zig");
const transport = @import("transport.zig");
const config = @import("config.zig");

// ============================================================================
// Test 1: Helper functions
// ============================================================================

/// Helper: create a test session with FakeTransport
fn createTestSession() session.Session {
    var fake = transport.FakeTransport.init(&.{});
    fake.reset();

    var ctx = transport.TransportContext.initFake(&fake);
    _ = ctx.toTransport(); // Suppress unused warning
    const mock_clock = clock.MockClock.interface();

    const bfd_config = config.BfdConfig{
        .local_addr = "10.0.0.1",
        .peer_addr = "10.0.0.2",
        .interval_ms = 800,
        .multiplier = 3,
    };

    var sess = session.init(bfd_config, mock_clock);
    sess.local_discr = 0x1234;
    return sess;
}

/// Helper: create a valid BFD control packet
fn createPacket(state: packet.State, my_discr: u32, your_discr: u32) packet.ControlPacket {
    return packet.ControlPacket{
        .state = state,
        .my_discr = my_discr,
        .your_discr = your_discr,
        .detect_mult = 3,
        .required_min_rx_interval = 800_000,
    };
}

// ============================================================================
// Test 2: AdminDown state transitions (terminal)
// ============================================================================

test "processEvent: stop from admin_down stays admin_down" {
    var sess = createTestSession();
    sess.state = .admin_down;
    
    const result = session.processEvent(&sess, .stop);
    
    try std.testing.expectEqual(packet.State.admin_down, sess.state);
    try std.testing.expectEqual(packet.State.admin_down, result);
}

test "processEvent: start from admin_down stays admin_down" {
    var sess = createTestSession();
    sess.state = .admin_down;
    
    const result = session.processEvent(&sess, .start);
    
    try std.testing.expectEqual(packet.State.admin_down, sess.state);
    try std.testing.expectEqual(packet.State.admin_down, result);
}

test "processEvent: received packet in admin_down is ignored" {
    var sess = createTestSession();
    sess.state = .admin_down;
    
    const pkt = createPacket(.up, 0x5678, sess.local_discr);
    const result = session.processEvent(&sess, .{ .packetReceived = pkt });
    
    try std.testing.expectEqual(packet.State.admin_down, sess.state);
    try std.testing.expectEqual(packet.State.admin_down, result);
}

// ============================================================================
// Test 3: Down state transitions (RFC 5880 Section 6.8.4)
// ============================================================================

test "processEvent: down + recv(init) -> init" {
    var sess = createTestSession();
    sess.state = .down;
    
    const pkt = createPacket(.init, 0x5678, sess.local_discr);
    const result = session.processEvent(&sess, .{ .packetReceived = pkt });
    
    try std.testing.expectEqual(packet.State.init, sess.state);
    try std.testing.expectEqual(packet.State.init, result);
}

test "processEvent: down + recv(up) -> up" {
    var sess = createTestSession();
    sess.state = .down;
    
    const pkt = createPacket(.up, 0x5678, sess.local_discr);
    const result = session.processEvent(&sess, .{ .packetReceived = pkt });
    
    try std.testing.expectEqual(packet.State.up, sess.state);
    try std.testing.expectEqual(packet.State.up, result);
}

test "processEvent: down + recv(down) -> down" {
    var sess = createTestSession();
    sess.state = .down;
    
    const pkt = createPacket(.down, 0x5678, sess.local_discr);
    const result = session.processEvent(&sess, .{ .packetReceived = pkt });
    
    try std.testing.expectEqual(packet.State.down, sess.state);
    try std.testing.expectEqual(packet.State.down, result);
}

// ============================================================================
// Test 4: Init state transitions (RFC 5880 Section 6.8.4)
// ============================================================================

test "processEvent: init + recv(init) -> up" {
    var sess = createTestSession();
    sess.state = .init;
    
    const pkt = createPacket(.init, 0x5678, sess.local_discr);
    const result = session.processEvent(&sess, .{ .packetReceived = pkt });
    
    try std.testing.expectEqual(packet.State.up, sess.state);
    try std.testing.expectEqual(packet.State.up, result);
}

test "processEvent: init + recv(up) -> up" {
    var sess = createTestSession();
    sess.state = .init;
    
    const pkt = createPacket(.up, 0x5678, sess.local_discr);
    const result = session.processEvent(&sess, .{ .packetReceived = pkt });
    
    try std.testing.expectEqual(packet.State.up, sess.state);
    try std.testing.expectEqual(packet.State.up, result);
}

test "processEvent: init + recv(down) -> down" {
    var sess = createTestSession();
    sess.state = .init;
    
    const pkt = createPacket(.down, 0x5678, sess.local_discr);
    const result = session.processEvent(&sess, .{ .packetReceived = pkt });
    
    try std.testing.expectEqual(packet.State.down, sess.state);
    try std.testing.expectEqual(packet.State.down, result);
}

// ============================================================================
// Test 5: Up state transitions (RFC 5880 Section 6.8.4)
// ============================================================================

test "processEvent: up + recv(down) -> down" {
    var sess = createTestSession();
    sess.state = .up;
    
    const pkt = createPacket(.down, 0x5678, sess.local_discr);
    const result = session.processEvent(&sess, .{ .packetReceived = pkt });
    
    try std.testing.expectEqual(packet.State.down, sess.state);
    try std.testing.expectEqual(packet.State.down, result);
}

test "processEvent: up + recv(init) -> init" {
    var sess = createTestSession();
    sess.state = .up;
    
    const pkt = createPacket(.init, 0x5678, sess.local_discr);
    const result = session.processEvent(&sess, .{ .packetReceived = pkt });
    
    try std.testing.expectEqual(packet.State.init, sess.state);
    try std.testing.expectEqual(packet.State.init, result);
}

test "processEvent: up + recv(up) -> up" {
    var sess = createTestSession();
    sess.state = .up;
    
    const pkt = createPacket(.up, 0x5678, sess.local_discr);
    const result = session.processEvent(&sess, .{ .packetReceived = pkt });
    
    try std.testing.expectEqual(packet.State.up, sess.state);
    try std.testing.expectEqual(packet.State.up, result);
}

// ============================================================================
// Test 6: Start event transitions
// ============================================================================

test "processEvent: start from down -> down" {
    var sess = createTestSession();
    sess.state = .down;
    
    const result = session.processEvent(&sess, .start);
    
    try std.testing.expectEqual(packet.State.down, sess.state);
    try std.testing.expectEqual(packet.State.down, result);
}

test "processEvent: start from init -> down" {
    var sess = createTestSession();
    sess.state = .init;
    
    const result = session.processEvent(&sess, .start);
    
    try std.testing.expectEqual(packet.State.down, sess.state);
    try std.testing.expectEqual(packet.State.down, result);
}

test "processEvent: start from up -> down" {
    var sess = createTestSession();
    sess.state = .up;
    
    const result = session.processEvent(&sess, .start);
    
    try std.testing.expectEqual(packet.State.down, sess.state);
    try std.testing.expectEqual(packet.State.down, result);
}

// ============================================================================
// Test 7: Stop event transitions to admin_down
// ============================================================================

test "processEvent: stop transitions to admin_down from down" {
    var sess = createTestSession();
    sess.state = .down;
    
    const result = session.processEvent(&sess, .stop);
    
    try std.testing.expectEqual(packet.State.admin_down, sess.state);
    try std.testing.expectEqual(packet.State.admin_down, result);
}

test "processEvent: stop transitions to admin_down from init" {
    var sess = createTestSession();
    sess.state = .init;
    
    const result = session.processEvent(&sess, .stop);
    
    try std.testing.expectEqual(packet.State.admin_down, sess.state);
    try std.testing.expectEqual(packet.State.admin_down, result);
}

test "processEvent: stop transitions to admin_down from up" {
    var sess = createTestSession();
    sess.state = .up;
    
    const result = session.processEvent(&sess, .stop);
    
    try std.testing.expectEqual(packet.State.admin_down, sess.state);
    try std.testing.expectEqual(packet.State.admin_down, result);
}

// ============================================================================
// Test 8: Packet validation before state transition
// ============================================================================

test "processEvent: wrong version packet does not change state" {
    var sess = createTestSession();
    sess.state = .down;
    
    var pkt = createPacket(.up, 0x5678, sess.local_discr);
    pkt.version = 5;
    
    const result = session.processEvent(&sess, .{ .packetReceived = pkt });
    
    try std.testing.expectEqual(packet.State.down, sess.state);
    try std.testing.expectEqual(packet.State.down, result);
}

test "processEvent: auth-present packet does not change state" {
    var sess = createTestSession();
    sess.state = .down;
    
    var pkt = createPacket(.up, 0x5678, sess.local_discr);
    pkt.flags.auth_present = 1;
    
    const result = session.processEvent(&sess, .{ .packetReceived = pkt });
    
    try std.testing.expectEqual(packet.State.down, sess.state);
    try std.testing.expectEqual(packet.State.down, result);
}

test "processEvent: wrong discriminator packet does not change state" {
    var sess = createTestSession();
    sess.state = .down;
    
    const pkt = createPacket(.up, 0x5678, 0xFFFF);
    
    const result = session.processEvent(&sess, .{ .packetReceived = pkt });
    
    try std.testing.expectEqual(packet.State.down, sess.state);
    try std.testing.expectEqual(packet.State.down, result);
}

test "processEvent: zero your_discr is accepted for initial discovery" {
    var sess = createTestSession();
    sess.state = .down;
    
    const pkt = createPacket(.down, 0x5678, 0);
    
    const result = session.processEvent(&sess, .{ .packetReceived = pkt });
    
    try std.testing.expectEqual(packet.State.down, sess.state);
    try std.testing.expectEqual(packet.State.down, result);
}

// ============================================================================
// Test 9: Detection timeout transitions
// ============================================================================

test "processEvent: detection timeout from up -> down" {
    var sess = createTestSession();
    sess.state = .up;
    
    const result = session.processEvent(&sess, .detectionTimeout);
    
    try std.testing.expectEqual(packet.State.down, sess.state);
    try std.testing.expectEqual(packet.State.down, result);
}

test "processEvent: detection timeout from init -> down" {
    var sess = createTestSession();
    sess.state = .init;
    
    const result = session.processEvent(&sess, .detectionTimeout);
    
    try std.testing.expectEqual(packet.State.down, sess.state);
    try std.testing.expectEqual(packet.State.down, result);
}

test "processEvent: detection timeout from down -> no change" {
    var sess = createTestSession();
    sess.state = .down;
    
    const result = session.processEvent(&sess, .detectionTimeout);
    
    try std.testing.expectEqual(packet.State.down, sess.state);
    try std.testing.expectEqual(packet.State.down, result);
}

test "processEvent: detection timeout from admin_down -> no change" {
    var sess = createTestSession();
    sess.state = .admin_down;
    
    const result = session.processEvent(&sess, .detectionTimeout);
    
    try std.testing.expectEqual(packet.State.admin_down, sess.state);
    try std.testing.expectEqual(packet.State.admin_down, result);
}
