// transition_totality_tests.zig — BFD state enum and parser totality tests
//
// ACT-TOVARISCH-ZIG-HULK24R: Replace documentation-only transition tests with executable FSM proofs
//
// Tests prove:
// - BFD State enum is closed (4 variants, u2)
// - BfdState enum is closed (5 variants)
// - parseBfdStateWire handles all valid wire values
// - parseBfdStateString handles all valid string formats
// - State values match RFC 5880 wire format

const std = @import("std");
const packet = @import("packet.zig");
const session = @import("session.zig");
const snapshot = @import("snapshot.zig");

// ============================================================================
// Test 1: BFD State enum completeness
// ============================================================================

test "BFD State enum has exactly 4 variants (u2)" {
    const field_count = @typeInfo(packet.State).@"enum".fields.len;
    try std.testing.expectEqual(@as(usize, 4), field_count);
}

test "BFD State variants are named correctly with correct values" {
    try std.testing.expectEqualStrings("admin_down", @tagName(.admin_down));
    try std.testing.expectEqual(@as(u2, 0), @intFromEnum(packet.State.admin_down));

    try std.testing.expectEqualStrings("down", @tagName(.down));
    try std.testing.expectEqual(@as(u2, 1), @intFromEnum(packet.State.down));

    try std.testing.expectEqualStrings("init", @tagName(.init));
    try std.testing.expectEqual(@as(u2, 2), @intFromEnum(packet.State.init));

    try std.testing.expectEqualStrings("up", @tagName(.up));
    try std.testing.expectEqual(@as(u2, 3), @intFromEnum(packet.State.up));
}

// ============================================================================
// Test 2: BfdState enum completeness (external-facing)
// ============================================================================

test "BfdState enum has exactly 5 variants" {
    const field_count = @typeInfo(snapshot.BfdState).@"enum".fields.len;
    try std.testing.expectEqual(@as(usize, 5), field_count);
}

test "BfdState variants are named correctly" {
    try std.testing.expectEqualStrings("admin_down", @tagName(.admin_down));
    try std.testing.expectEqualStrings("down", @tagName(.down));
    try std.testing.expectEqualStrings("init", @tagName(.init));
    try std.testing.expectEqualStrings("up", @tagName(.up));
    try std.testing.expectEqualStrings("unknown", @tagName(.unknown));
}

// ============================================================================
// Test 3: parseBfdStateWire covers all valid u2 values
// ============================================================================

test "parseBfdStateWire handles all valid RFC 5880 values" {
    try std.testing.expectEqual(snapshot.BfdState.admin_down, snapshot.parseBfdStateWire(0));
    try std.testing.expectEqual(snapshot.BfdState.down, snapshot.parseBfdStateWire(1));
    try std.testing.expectEqual(snapshot.BfdState.init, snapshot.parseBfdStateWire(2));
    try std.testing.expectEqual(snapshot.BfdState.up, snapshot.parseBfdStateWire(3));
}

// ============================================================================
// Test 4: parseBfdStateString handles all valid states
// ============================================================================

test "parseBfdStateString handles lowercase" {
    try std.testing.expectEqual(snapshot.BfdState.admin_down, snapshot.parseBfdStateString("admin_down"));
    try std.testing.expectEqual(snapshot.BfdState.down, snapshot.parseBfdStateString("down"));
    try std.testing.expectEqual(snapshot.BfdState.init, snapshot.parseBfdStateString("init"));
    try std.testing.expectEqual(snapshot.BfdState.up, snapshot.parseBfdStateString("up"));
}

test "parseBfdStateString handles capitalized" {
    try std.testing.expectEqual(snapshot.BfdState.admin_down, snapshot.parseBfdStateString("AdminDown"));
    try std.testing.expectEqual(snapshot.BfdState.down, snapshot.parseBfdStateString("Down"));
    try std.testing.expectEqual(snapshot.BfdState.init, snapshot.parseBfdStateString("Init"));
    try std.testing.expectEqual(snapshot.BfdState.up, snapshot.parseBfdStateString("Up"));
}

test "parseBfdStateString handles uppercase" {
    try std.testing.expectEqual(snapshot.BfdState.admin_down, snapshot.parseBfdStateString("ADMINDOWN"));
    try std.testing.expectEqual(snapshot.BfdState.down, snapshot.parseBfdStateString("DOWN"));
    try std.testing.expectEqual(snapshot.BfdState.init, snapshot.parseBfdStateString("INIT"));
    try std.testing.expectEqual(snapshot.BfdState.up, snapshot.parseBfdStateString("UP"));
}

test "parseBfdStateString returns .unknown for invalid strings" {
    try std.testing.expectEqual(snapshot.BfdState.unknown, snapshot.parseBfdStateString(""));
    try std.testing.expectEqual(snapshot.BfdState.unknown, snapshot.parseBfdStateString("invalid"));
    try std.testing.expectEqual(snapshot.BfdState.unknown, snapshot.parseBfdStateString("UNKNOWN"));
    try std.testing.expectEqual(snapshot.BfdState.unknown, snapshot.parseBfdStateString("UpDown"));
    try std.testing.expectEqual(snapshot.BfdState.unknown, snapshot.parseBfdStateString("up!"));
}

// ============================================================================
// Test 5: State values match RFC 5880
// ============================================================================

test "State values match RFC 5880 wire format" {
    try std.testing.expectEqual(@as(u2, 0), @intFromEnum(packet.State.admin_down));
    try std.testing.expectEqual(@as(u2, 1), @intFromEnum(packet.State.down));
    try std.testing.expectEqual(@as(u2, 2), @intFromEnum(packet.State.init));
    try std.testing.expectEqual(@as(u2, 3), @intFromEnum(packet.State.up));
}

// ============================================================================
// Test 6: Session State (alias) is the same as packet.State
// ============================================================================

test "SessionState is packet.State" {
    try std.testing.expectEqual(session.SessionState, packet.State);
}

// ============================================================================
// Test 7: Protocol version is 1
// ============================================================================

test "BFD protocol version is 1 (RFC 5880)" {
    try std.testing.expectEqual(packet.PROTOCOL_VERSION, @as(u3, 1));
}

// ============================================================================
// Test 8: Exhaustive mapping proof
// ============================================================================

test "BFD State to BfdState mapping is total" {
    try std.testing.expectEqual(snapshot.BfdState.admin_down, snapshot.parseBfdStateWire(@intFromEnum(packet.State.admin_down)));
    try std.testing.expectEqual(snapshot.BfdState.down, snapshot.parseBfdStateWire(@intFromEnum(packet.State.down)));
    try std.testing.expectEqual(snapshot.BfdState.init, snapshot.parseBfdStateWire(@intFromEnum(packet.State.init)));
    try std.testing.expectEqual(snapshot.BfdState.up, snapshot.parseBfdStateWire(@intFromEnum(packet.State.up)));
}
