# BGP Protocol Module

## Overview

This document describes the BGP protocol support for `tovarisch`, implemented in three ACTs:

- **ACT 1**: Pure encoding/parsing components — no sockets or runtime.
- **ACT 2**: Minimal TCP session state machine using FakeTransport — no daemon integration yet.
- **ACT 3**: Real TCP transport adapter — still no daemon integration yet.

## ACT 1: Pure Encoding/Parsing

Implemented:
- BGP message frame encoding (KEEPALIVE, OPEN, UPDATE)
- IPv4 NLRI prefix encoding
- BIRD-style `route <IPv4 CIDR> reject;` prefix-list parser
- Config validation helpers

**Not implemented** (deferred to future ACTs):
- 32-bit ASN capability support
- BFD-gated BGP session behavior
- Graceful withdrawal on shutdown

## ACT 2: TCP Session State Machine

Implemented:
- BGP frame decoding (OPEN, KEEPALIVE, UPDATE, NOTIFICATION)
- Session state machine (RFC 4271 Section 8.2)
- TCP connection and message exchange
- Mock peer for integration testing
- In-memory session status (not exposed via HTTP/status JSON yet)

**Still not implemented** (deferred to future ACTs):
- Production daemon integration
- Config-file wiring
- `/status --json` changes
- BFD gating
- Multiple peers
- IPv6
- MP-BGP
- 32-bit ASN capability
- Route refresh
- Graceful restart
- Communities
- TCP MD5 / TCP-AO
- Kernel route installation
- Learned/imported RIB
- BGP decision process
- Reconnect/backoff loop

## ACT 3: Real TCP Transport

Implemented:
- `TcpTransport` conforming to existing `Transport` interface
- TCP socket connect to configured peer address/port
- Optional local address binding
- Send/receive with partial handling
- Clean socket close on all error paths
- Non-blocking receive (returns empty slice when no data)

**Architecture:**
```
tovarisch/src/bgp/
├── tcp_transport.zig       # TcpTransport implementation (ACT 3)
├── tcp_transport_tests.zig # Local loopback tests (ACT 3)
├── transport.zig          # Transport interface (ACT 2)
├── session.zig            # Session state machine (ACT 2)
└── ...
```

**Test Coverage (ACT 3):**
- [x] TcpTransport IPv4 byte order is correct (memory layout)
- [x] TcpTransport port byte order is correct (memory layout)
- [x] TcpTransport connects to local listener
- [x] TcpTransport sends bytes to listener
- [x] TcpTransport receives bytes from listener
- [x] TcpTransport closes cleanly
- [x] TcpTransport wraps as Transport interface
- [x] TcpTransport handles peer close
- [x] TcpTransport returns empty when no data available

**Deferred:**
- Live invalid-port connect tests deferred until bounded nonblocking connect exists
- connect_timeout_ms is decorative until bounded nonblocking connect is implemented

**Still not implemented** (deferred to future ACTs):
- Production daemon integration
- Config-file wiring
- `/status --json` changes
- Reconnect/backoff loop
- BFD gating
- Multiple peers

## Architecture

```
tovarisch/src/bgp/
├── types.zig              # BGP types, constants, Ipv4Prefix
├── message.zig           # Frame encoding (KEEPALIVE, OPEN, UPDATE)
├── message_tests.zig     # UPDATE and NLRI encoding tests
├── validation.zig       # Config validation helpers
├── prefix_file.zig       # BIRD-style prefix-list parser
├── frame_decode.zig      # Frame decoding (ACT 2)
├── session_status.zig    # Session state/status types (ACT 2)
├── transport.zig         # Transport interface (ACT 2)
├── session.zig           # TCP session state machine (ACT 2)
├── session_tests.zig     # Integration tests (ACT 2)
├── session_handshake_tests.zig # Handshake flow tests (ACT 2)
├── tcp_transport.zig     # Real TCP transport (ACT 3)
└── tcp_transport_tests.zig # Local loopback tests (ACT 3)
```


## BGP Message Encoding

### KEEPALIVE (19 bytes)
```
Marker (16 bytes of 0xFF) + Length (2 bytes = 19) + Type (1 byte = 4)
```

### OPEN
```
Marker (16) + Length (2) + Type (1) + Version (1) + My AS (2) + Hold Time (2) + Router ID (4) + Opt Params Len (1)
```
- Version: 4 (BGP-4)
- My AS: 16-bit ASN (1..65535); 32-bit ASN is rejected in this ACT
- Hold Time: 0 or >= 3 seconds
- Optional parameters: none (length = 0)

### UPDATE
```
Marker (16) + Length (2) + Type (1) +
  Withdrawn Routes Length (2) +
  Path Attributes Length (2) +
  Path Attributes:
    - ORIGIN (IGP)
    - AS_PATH (empty for same-AS, local AS for different-AS)
    - NEXT_HOP (configured local/source IPv4)
  + NLRI (prefixes)
```

## Session State Machine

BGP sessions follow RFC 4271 Section 8.2:

```
Idle -> Connect -> OpenSent -> OpenConfirm -> Established
                                              |
                                              v
                                           Failed
```

### States

| State | Description |
|-------|-------------|
| idle | Initial state, no connection |
| connect | TCP connection in progress |
| open_sent | OPEN sent, waiting for peer's OPEN |
| open_confirm | OPEN received, waiting for KEEPALIVE |
| established | Connection active, can exchange UPDATE |
| failed | Connection failed |
| stopped | Session stopped cleanly |

### Session Config

```zig
pub const SessionConfig = struct {
    peer_address: [4]u8,        // Peer's IPv4
    peer_port: u16,              // Must be nonzero
    local_address: ?[4]u8,       // Our IPv4 (null = OS picks)
    local_as: u16,               // 1..65535
    peer_as: u16,                // 1..65535
    router_id: [4]u8,            // Our router ID
    hold_time_seconds: u16,      // 0 or >= 3
    keepalive_seconds: u16,      // < hold_time when hold_time != 0
    connect_timeout_ms: u32,
    prefixes: []const Ipv4Prefix, // Must be non-empty
    same_as: bool,               // true = empty AS_PATH
};
```

### Session Status (In-Memory)

```zig
pub const SessionStatus = struct {
    state: SessionState,
    peer_address: [4]u8,
    peer_as: u16,
    local_as: u16,
    router_id: [4]u8,
    advertised_prefix_count: usize,
    messages_sent: u64,
    messages_received: u64,
    updates_sent: u64,
    keepalives_sent: u64,
    keepalives_received: u64,
    last_error: ?SessionError,
    last_notification_code: ?u8,
    last_notification_subcode: ?u8,
};
```

**Note:** Session status is internal only. It is NOT exposed via HTTP `/status --json` in this ACT.

## Mock Peer Testing

ACT 2 includes a `MockPeer` for integration testing:

```zig
// Create mock peer
var peer = try session.MockPeer.init(65002, .{ 10, 0, 0, 2 });
defer peer.close();

// Accept connection
try peer.accept();

// Read and validate OPEN from tovarisch
try peer.readAndValidateOpen();

// Send peer OPEN
try peer.sendOpen();

// Continue handshake...
```

## Import-Nothing Invariant

**Important:** Incoming UPDATEs from peers are **accepted but never imported**.

The session:
- Receives and counts UPDATE messages
- Validates frame structure
- Does NOT create any route state
- Does NOT add prefixes to any RIB

This invariant is enforced to maintain the leaf-service doctrine: `tovarisch` observes infrastructure health, not people.

## Testing

All BGP tests are wired into `test_all.zig`:
- `make tovarisch-test` runs all tests
- `make tovarisch-build` verifies compilation

### Test Coverage (ACT 2)

**Frame Decode Tests:**
- [x] Frame rejects bad marker
- [x] Frame rejects length below 19
- [x] Frame rejects length above max
- [x] Frame recognizes KEEPALIVE
- [x] Frame parses OPEN minimally
- [x] Frame parses NOTIFICATION code/subcode

**Session Config Validation Tests:**
- [x] Rejects zero peer_port
- [x] Rejects empty prefixes
- [x] Rejects invalid hold_time
- [x] Rejects keepalive >= hold_time
- [x] Accepts valid config

**Session State Machine Tests:**
- [x] Session init creates idle session
- [x] Session sends OPEN first
- [x] Session sends KEEPALIVE after peer OPEN
- [x] Session reaches Established after peer KEEPALIVE
- [x] Session sends UPDATE after establishment
- [x] Session stop exits cleanly

**Mock Peer Tests:**
- [x] Mock peer validates advertised NLRI
- [x] Mock peer validates NEXT_HOP
- [x] Mock peer validates ORIGIN and AS_PATH

**Import-Nothing Tests:**
- [x] Incoming peer UPDATE is ignored/imports nothing
- [x] Peer NOTIFICATION is recorded safely

## Future ACTs

| ACT | Description |
|-----|-------------|
| ACT 4 | Wire BGP session into tovarisch serve runtime (disabled by default) |
| ACT 5 | Expose BGP state in /status --json |
| ACT 6 | Add reconnect/backoff loop |
| ACT 7 | Add BFD-gated BGP advertisement |
| ACT 8 | Add 32-bit ASN capability support |
| ACT 9 | Add graceful withdrawal on shutdown |
| ACT 10 | Add multiple BGP peers |


## References

- RFC 4271 — Border Gateway Protocol 4 (BGP-4)
  - Sections 4.1 (message format)
  - Sections 4.2 (OPEN message)
  - Sections 4.3 (UPDATE message)
  - Sections 5.1 (path attributes: ORIGIN, AS_PATH, NEXT_HOP)
  - Section 8 (BGP State Machine)
  - Sections 8.2 (State Machine Description)
