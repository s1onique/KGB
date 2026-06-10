# BGP Protocol Module

## Overview

This document describes the BGP protocol support for `tovarisch`, implemented in multiple ACTs:

- **ACT 1**: Pure encoding/parsing components — no sockets or runtime.
- **ACT 2**: Minimal TCP session state machine using FakeTransport — no daemon integration yet.
- **ACT 3**: Real TCP transport adapter — still no daemon integration yet.
- **ACT 4**: Wire BGP session into tovarisch serve runtime — **disabled by default, config-only**.

## ACT 1: Pure Encoding/Parsing

Implemented:
- BGP message frame encoding (KEEPALIVE, OPEN, UPDATE)
- IPv4 NLRI prefix encoding
- BIRD-style `route <IPv4 CIDR> reject;` prefix-list parser
- Config validation helpers

## ACT 2: TCP Session State Machine

Implemented:
- BGP frame decoding (OPEN, KEEPALIVE, UPDATE, NOTIFICATION)
- Session state machine (RFC 4271 Section 8.2)
- TCP connection and message exchange
- Mock peer for integration testing
- In-memory session status (not exposed via HTTP/status JSON yet)

## ACT 3: Real TCP Transport

Implemented:
- `TcpTransport` conforming to existing `Transport` interface
- TCP socket connect to configured peer address/port
- Optional local address binding
- Send/receive with partial handling
- Clean socket close on all error paths
- Non-blocking receive (returns empty slice when no data)

**Deferred:**
- Live invalid-port connect tests deferred until bounded nonblocking connect exists
- connect_timeout_ms is decorative until bounded nonblocking connect is implemented

## ACT 4: Runtime Wiring (Current ACT)

**Status:** COMPLETED

Implemented:
- BGP config parsing from `[bgp]` section in tovarisch.conf
- Plain IPv4 address parser for local_address, router_id, peer_address
- Comma-separated advertised_prefixes parsing
- BGP disabled by default — ZERO sockets created when disabled
- BGP enabled via explicit `enabled = true` in config
- Session config building and validation from parsed config
- Runtime bundle management (similar to BFD pattern)
- CLI integration alongside BFD

**Key Constraints:**
1. **Disabled means zero sockets** — If BGP is disabled, no TcpTransport.connect() is ever called
2. **Config-only in ACT 4** — This ACT builds and validates config but does NOT call TcpTransport.connect()
3. **No hot-loop on failure** — Config validation errors are caught before connection
4. **No /status exposure** — BGP state is internal only in this ACT

**Not implemented** (deferred to future ACTs):
- `/status --json` BGP exposure
- TcpTransport.connect() call (deferred until bounded nonblocking connect exists)
- Reconnect/backoff loop
- BFD gating
- advertised_prefix_files (prefix list files)
- 32-bit ASN support
- Multiple peers
- Graceful withdrawal on shutdown
- Kernel route installation
- Learned/imported RIB
- Bounded nonblocking connect timeout

**Files Added:**
```
tovarisch/src/bgp/
├── config_parse.zig           # [bgp] section parser + IPv4 address parser
├── serve_integration.zig      # Runtime wiring (config validation only)
└── serve_integration_tests.zig # Integration tests
```

**Config Shape:**
```ini
[bgp]
enabled = false  # Default: disabled
local_address = "10.0.0.1"      # Plain IPv4 (no CIDR)
router_id = "10.0.0.1"           # Plain IPv4 (no CIDR)
local_as = 65001
peer_address = "10.0.0.2"        # Plain IPv4 (no CIDR)
peer_port = 179
peer_as = 65002
hold_time_seconds = 180
keepalive_seconds = 60
connect_timeout_ms = 1000  # Decorative until bounded connect exists
advertised_prefixes = "10.0.0.0/8,192.168.0.0/16"  # Comma-separated CIDR
same_as = false
```

**Runtime State:**
- `not_configured` — No [bgp] section in config
- `disabled` — [bgp] exists but enabled=false
- `configured` — Config built and validated, ready for connection
- `failed` — Config build or validation failed

**Test Coverage (ACT 4):**
- [x] Default config has BGP disabled
- [x] Disabled BGP config does not call connect
- [x] Serve startup path with BGP disabled works
- [x] Enabled config validates required fields
- [x] Enabled config rejects missing peer_address
- [x] Enabled config rejects missing local_as
- [x] Enabled config rejects missing advertised_prefixes
- [x] Plain IPv4 addresses parse correctly (not CIDR)
- [x] Plain IPv4 rejects CIDR suffix
- [x] Plain IPv4 rejects IPv6
- [x] Multiple advertised_prefixes parse correctly
- [x] No /status --json contract changes
- [x] No blocking connect call in serve startup

## Architecture

```
tovarisch/src/bgp/
├── types.zig              # BGP types, constants, Ipv4Prefix
├── message.zig           # Frame encoding (KEEPALIVE, OPEN, UPDATE)
├── validation.zig       # Config validation helpers
├── prefix_file.zig       # BIRD-style prefix-list parser
├── frame_decode.zig      # Frame decoding (ACT 2)
├── session_status.zig    # Session state/status types (ACT 2)
├── transport.zig         # Transport interface (ACT 2)
├── session.zig           # TCP session state machine (ACT 2)
├── session_tests.zig     # Integration tests (ACT 2)
├── tcp_transport.zig     # Real TCP transport (ACT 3)
├── tcp_transport_tests.zig # Local loopback tests (ACT 3)
├── config_parse.zig      # Config parsing + IPv4 address parser (ACT 4)
├── serve_integration.zig # Runtime wiring (ACT 4)
└── serve_integration_tests.zig # Integration tests (ACT 4)
```

```
tovarisch/src/cli/
├── bfd_serve.zig          # BFD runtime (existing)
└── bgp_serve.zig          # BGP runtime (ACT 4)
```

## Session Config

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
    connect_timeout_ms: u32,     // Decorative until bounded connect
    prefixes: []const Ipv4Prefix, // Must be non-empty
    same_as: bool,               // true = empty AS_PATH
};
```

## Future ACTs

| ACT | Description |
|-----|-------------|
| ACT 5 | Expose BGP state in /status --json |
| ACT 6 | Add bounded nonblocking connect timeout + actual connection |
| ACT 7 | Add BFD-gated BGP advertisement |
| ACT 8 | Add reconnect/backoff loop |
| ACT 9 | Add 32-bit ASN capability support |
| ACT 10 | Add multiple BGP peers |

## References

- RFC 4271 — Border Gateway Protocol 4 (BGP-4)
  - Sections 4.1 (message format)
  - Sections 4.2 (OPEN message)
  - Sections 4.3 (UPDATE message)
  - Sections 5.1 (path attributes: ORIGIN, AS_PATH, NEXT_HOP)
  - Section 8 (BGP State Machine)
  - Sections 8.2 (State Machine Description)
