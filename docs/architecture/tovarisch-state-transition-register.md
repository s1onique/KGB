# Tovarisch State-Transition Register

**ACT**: ACT-TOVARISCH-ZIG-HULK24  
**Purpose**: Document and verify BGP/BFD FSM state transitions are closed, total, and explicit

## Overview

Protocol state transitions in tovarisch follow the pattern: `state + event/input -> next state + action/result`.

**Target**: `DEFERRED transitions: 0`

All external inputs and protocol events map to explicit state transitions. No path should cause:
- `@panic` on malformed input
- `unreachable` that could be reached by external input
- Partial mutation (state changes without side effects)
- Unclassified transitions

---

## BGP State Machine

### State Enum (RFC 4271 Section 8.2)

```zig
pub const SessionState = enum {
    idle,         // Initial state, no connection
    connect,      // TCP connection in progress
    open_sent,    // OPEN sent, waiting for peer's OPEN
    open_confirm, // OPEN received, waiting for KEEPALIVE
    established,  // Connection active, can send UPDATE
    failed,       // Connection failed, should reconnect
    stopped,      // Session stopped cleanly
};
```

### Event Enum (Implicit)

```zig
// Events are implicit in runOnce() switch:
.runOnce()       // Main FSM step
.receiveOpen()   // Received OPEN message
.receiveKeepalive() // Received KEEPALIVE message
.receiveNotification() // Received NOTIFICATION message
.receiveUpdate() // Received UPDATE message
.sendFailure()   // Transport send failed
.recvFailure()   // Transport receive failed (connection closed)
.holdTimerExpired() // Hold timer expired
```

### Transition Table

| Current State | Event | Next State | Side Effects | Code Path |
|--------------|-------|------------|-------------|-----------|
| `idle` | `runOnce()` | `open_sent` | Send OPEN, increment messages_sent | `session.zig:331-349` |
| `idle` | send failure | `failed` | Set error message | `session.zig:338-346` |
| `open_sent` | recv OPEN (valid) | `open_confirm` | Send KEEPALIVE, increment messages_sent | `session.zig:272-297` |
| `open_sent` | recv OPEN (AS mismatch) | `failed` | Set error message | `session.zig:283-290` |
| `open_sent` | recv OPEN (malformed) | `failed` | Set error message | `session.zig:274-281` |
| `open_sent` | recv NOTIFICATION | `failed` | Set error details | `session.zig:298-300` |
| `open_sent` | recv other | `open_sent` | No change | `session.zig:302` |
| `open_sent` | hold timer expired | `failed` | Set error message | `session.zig:357-364` |
| `open_sent` | send failure | `failed` | Set error message | `session.zig:398-405` |
| `open_sent` | connection closed | `failed` | Set error message | `session.zig:417-424` |
| `open_confirm` | recv KEEPALIVE | `established` | Reset hold timer, start keepalive scheduler | `session.zig:305-309` |
| `open_confirm` | recv NOTIFICATION | `failed` | Set error details | `session.zig:310-312` |
| `open_confirm` | recv other | `open_confirm` | No change | `session.zig:314` |
| `open_confirm` | hold timer expired | `failed` | Set error message | `session.zig:357-364` |
| `open_confirm` | send failure | `failed` | Set error message | `session.zig:398-405` |
| `open_confirm` | connection closed | `failed` | Set error message | `session.zig:417-424` |
| `established` | recv KEEPALIVE | `established` | Reset hold timer, increment counter | `session.zig:316-319` |
| `established` | recv NOTIFICATION | `failed` | Set error details | `session.zig:320-322` |
| `established` | recv UPDATE | `established` | Reset hold timer | `session.zig:316` |
| `established` | hold timer expired | `failed` | Set error message, send NOTIFICATION | `session.zig:357-364` |
| `established` | keepalive due | `established` | Send KEEPALIVE | `session.zig:366-376` |
| `established` | send failure | `failed` | Set error message | `session.zig:398-405` |
| `established` | connection closed | `failed` | Set error message | `session.zig:417-424` |
| `failed` | any | `failed` | No change | `session.zig:438-440` |
| `stopped` | any | `stopped` | No change | `session.zig:438-440` |
| `connect` | `runOnce()` | `open_sent` | (legacy, rarely used) | `session.zig:441-444` |

### SessionState to BgpPeerState Mapping

```zig
pub fn mapSessionStateToBgpPeerState(sess_state: session_status.SessionState) snapshot.BgpPeerState {
    return switch (sess_state) {
        .idle => .idle,
        .connect => .connect,
        .open_sent => .open_sent,
        .open_confirm => .open_confirm,
        .established => .established,
        .failed, .stopped => .unknown,  // Internal states map to .unknown
    };
}
```

### Structured Failure Modes

| Failure Mode | Structured Result | Transition |
|-------------|------------------|------------|
| Malformed OPEN | `SessionErrorKind.DecodeError` | `failed` |
| AS mismatch | `SessionErrorKind.InvalidFrame` | `failed` |
| Invalid frame length | `SessionErrorKind.DecodeError` | `failed` |
| Malformed frame | `SessionErrorKind.DecodeError` | `failed` |
| Peer NOTIFICATION | `RunResult.failed` | `failed` |
| Hold timer expired | `RunResult.failed` | `failed` |
| TCP connection closed | `SessionErrorKind.ConnectionClosed` | `failed` |
| Send failure | `SessionErrorKind.IoError` | `failed` |

### Test Coverage

- `bgp/session_tests.zig`: Session lifecycle tests
- `bgp/session_handshake_tests.zig`: OPEN/KEEPALIVE handshake
- `bgp/session_keepalive_basic_tests.zig`: Keepalive timer
- `bgp/session_keepalive_notification_tests.zig`: NOTIFICATION handling
- `bgp/reconnect_hold_timer_tests.zig`: Hold timer expiry recovery
- `bgp/transition_totality_tests.zig`: State transition totality (this ACT)

---

## BFD State Machine

### State Enum (RFC 5880 Section 6.8.4)

```zig
pub const State = enum(u2) {
    admin_down = 0,
    down = 1,
    init = 2,
    up = 3,
};
```

### Event Enum

```zig
pub const Event = union(enum) {
    start,                    // Begin session
    stop,                     // Admin down
    packetReceived: ControlPacket,  // Received BFD packet
    detectionTimeout,          // No packet received in time
    transmitTimeout,           // Time to send packet
};
```

### Transition Table (RFC 5880 Section 6.8.4)

| Current State | Received State | Next State | Notes |
|--------------|---------------|------------|-------|
| `admin_down` | any | `admin_down` | Stays; start ignored in admin_down |
| `down` | `init` | `init` | |
| `down` | `up` | `up` | |
| `down` | `down` | `down` | No change |
| `down` | `admin_down` | `down` | No change |
| `init` | `init`, `up` | `up` | |
| `init` | `down` | `down` | |
| `init` | `admin_down` | `init` | No change |
| `up` | `down` | `down` | |
| `up` | `init` | `init` | |
| `up` | `up` | `up` | No change |
| `up` | `admin_down` | `up` | No change |

### Internal State Machine Logic

```zig
fn nextState(current: SessionState, received: SessionState) SessionState {
    switch (current) {
        .admin_down => return .admin_down,
        .down => switch (received) {
            .init => return .init,
            .up => return .up,
            else => return .down,
        },
        .init => switch (received) {
            .init, .up => return .up,
            .down => return .down,
            else => return .init,
        },
        .up => switch (received) {
            .down => return .down,
            .init => return .init,
            else => return .up,
        },
    }
}
```

### Event Handlers

| Handler | Logic | State Changes |
|---------|-------|--------------|
| `handleStart()` | If `.admin_down`, stay there. Else set to `.down`, clear remote_discr | `admin_down` → `admin_down` or any → `down` |
| `handleStop()` | Always set to `.admin_down` | any → `admin_down` |
| `handlePacketReceived()` | Validate version/auth/discriminators, call `nextState()` | Only on valid packet |
| `handleDetectionTimeout()` | Only transition if `.up` or `.init` | `.up` → `.down` or `.init` → `.down` |
| `handleTransmitTimeout()` | No state change, update next_transmit_time | No state change |

### Packet Validation Before Transition

```zig
// Version check
if (pkt.version != PROTOCOL_VERSION) return sess.state;

// Auth check
if (pkt.flags.auth_present == 1) { sess.stats.packets_dropped += 1; return sess.state; }

// Discriminator check (accept zero on initial discovery)
if (pkt.your_discr != 0 and pkt.your_discr != sess.local_discr) {
    sess.stats.packets_dropped += 1;
    return sess.state;
}
```

### Structured Failure Modes

| Failure Mode | Handling | State |
|-------------|----------|-------|
| Wrong version | Drop packet, increment `packets_dropped` | No change |
| Auth present | Drop packet, increment `packets_dropped` | No change |
| Wrong discriminator | Drop packet, increment `packets_dropped` | No change |
| Detection timeout | Set to `.down`, increment `detection_timeouts` | `.up`/`.init` → `.down` |
| Start from admin_down | Stay in admin_down | `admin_down` |

### Test Coverage

- `bfd/session_tests.zig`: Session state machine tests
- `bfd/session_bird_tests.zig`: BIRD interop tests
- `bfd/receive_startup_tests.zig`: Startup sequence tests
- `bfd/receive_tests.zig`: Receive path tests
- `bfd/transition_totality_tests.zig`: State transition totality (this ACT)

---

## BgpPeerState (External-Facing)

```zig
pub const BgpPeerState = enum {
    idle,
    connect,
    active,
    open_sent,
    open_confirm,
    established,
    unknown,  // For external sources
};
```

### Mapping from External Strings

```zig
pub fn parseBgpPeerState(str: []const u8) BgpPeerState {
    // Returns .unknown for unrecognized strings
    // Handles BIRD, Quagga, FRR, GoBGP variants
}
```

---

## BfdState (External-Facing)

```zig
pub const BfdState = enum {
    admin_down,
    down,
    init,
    up,
    unknown,  // For external/wire sources
};
```

### Mapping from Wire and Strings

```zig
pub fn parseBfdStateWire(state_val: u2) BfdState {
    // 0 -> admin_down, 1 -> down, 2 -> init, 3 -> up
}

pub fn parseBfdStateString(str: []const u8) BfdState {
    // Returns .unknown for unrecognized strings
}
```

---

## Totality Guarantees

### BGP Guarantees

1. **Closed state enum**: `SessionState` has exactly 7 variants - compiler catches missed cases
2. **Exhaustive switch**: All `runOnce()` paths have `else` branches that return errors
3. **No unreachable on input**: Malformed frames return `null` from `tryDecodeFrame()` and are skipped
4. **Structured failures**: All failure paths set `status.last_error` before transitioning to `failed`
5. **Stop is terminal**: `stop()` only sets `.stopped`, never transitions elsewhere

### BFD Guarantees

1. **Closed state enum**: `State` has exactly 4 variants (u2) - all values covered
2. **Packet validation before transition**: Invalid packets don't trigger state changes
3. **nextState() is total**: Every combination of (current, received) has explicit handling
4. **admin_down is terminal**: `handleStart()` explicitly checks and returns `.admin_down` if already there
5. **Detection timeout guards**: Only `.up` and `.init` transition on timeout

---

## DEFERRED Transitions

**DEFERRED transitions: 0**

All documented transitions are implemented and tested.

---

## Maintenance

When adding new protocol states or events:

1. Document the new transition in this register
2. Ensure the switch is exhaustive (compiler will warn)
3. Add structured failure handling
4. Add test coverage in `transition_totality_tests.zig`
5. Run `make tovarisch-test` to verify

---

## Revision History

- 2026-07-06: ACT-TOVARISCH-ZIG-HULK24 initial register
