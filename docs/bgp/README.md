# BGP Protocol Module

## Overview

This document describes the BGP protocol support for `tovarisch`, implemented in multiple ACTs:

- **ACT 1**: Pure encoding/parsing components — no sockets or runtime.
- **ACT 2**: Minimal TCP session state machine using FakeTransport — no daemon integration yet.
- **ACT 3**: Real TCP transport adapter — still no daemon integration yet.
- **ACT 4**: Wire BGP session into tovarisch serve runtime — **disabled by default, config-only**.
- **ACT 5**: Add BGP reconnect/backoff loop — lifecycle resilience without daemon restart.

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
- Bounded nonblocking connect timeout

## ACT 4: Runtime Wiring (COMPLETED)

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

## ACT 5: Reconnect/Backoff Loop

**Status:** COMPLETED

Implemented:
- Exponential backoff for failed connections/sessions
- Initial retry: 1s, doubling up to 60s max
- Backoff reset after successful establishment
- reconnect_wait runtime state
- Cleanup during reconnect_wait stops reconnect loop
- Transport closed exactly once on cleanup
- Status reports reconnect_wait state with backoff delay
- Session failure schedules reconnect (not thread exit)
- Peer close schedules reconnect
- Thread-safe cleanup via atomic cleanup_requested
- Joined thread (not detached) for safe bundle lifetime

**Thread Ownership Model:**
- Runtime thread is stored in `bundle.runtime_thread`
- `cleanupBgpBundle()` signals via atomic store, then joins thread
- No detached thread can access destroyed bundle
- `cleanup_requested` is atomic `u8` flag with `@atomicStore/@atomicLoad` for cross-thread signaling

**Reconnect Behavior:**
```
Initial connect failed → backoff 1s → retry
Retry failed → backoff 2s → retry
Retry failed → backoff 4s → retry
...
60s max → retry every 60s (no hot loop)
Session established → reset backoff to 0
```

**Runtime States:**
- `not_configured` — No [bgp] section in config
- `disabled` — [bgp] exists but enabled=false
- `configured` — Config built and validated, session running
- `reconnect_wait` — Waiting for backoff deadline after failure
- `failed` — Terminal config validation failure

## ACT 6: Advertised Prefix Files (CURRENT ACT)

**Status:** COMPLETED

Implemented:
- `advertised_prefix_files` config key for one or more prefix-list file paths
- Runtime loading of BIRD-style prefix files using existing `prefix_file.zig` parser
- Merge behavior: inline `advertised_prefixes` + file-loaded prefixes are concatenated
- Fail-closed: unreadable or invalid prefix files cause BGP load to fail with diagnostic
- Disabled BGP does not attempt to read prefix files
- At least one advertised prefix is required when BGP is enabled

**Config Example:**
```ini
[bgp]
enabled = true
local_address = "10.0.0.1"
router_id = "10.0.0.1"
local_as = 65001
peer_address = "10.0.0.2"
peer_as = 65002
advertised_prefixes = "10.0.0.0/8"
advertised_prefix_files = "/etc/kgb/bgp-prefixes.conf"
```

**Prefix File Format (BIRD-style):**
```
# /etc/kgb/bgp-prefixes.conf
route 192.168.0.0/16 reject;
route 172.16.0.0/12 reject;
```

**Merge Behavior:**
- Inline prefixes (comma-separated in config) are loaded first
- Prefix files are loaded and parsed second
- All parsed prefixes are concatenated into the advertised prefix list
- Duplicates are preserved (not deduplicated)
- Empty prefix file is valid if inline prefixes exist
- Empty prefix file with no inline prefixes causes failure

**Error Handling:**
- Unreadable file: `error: failed to read prefix file '/path/to/file.conf': ...`
- Parse error: `error: failed to parse prefix file '/path/to/file.conf': SyntaxError`
- No prefixes: `error: no advertised prefixes configured (need inline or prefix files)`

**Ownership Model:**
- Config parsing is allocation-free (stores raw strings only)
- Runtime parsing owns temporary path list and prefix array allocations
- All temporary allocations are freed on success
- Bundle-owned prefixes are freed exactly once by `cleanupBgpBundle()`

**Not implemented** (deferred to future ACTs):
- BFD-gated advertisement
- 32-bit ASN support
- Multiple peers
- Graceful withdrawal on shutdown
- Kernel route installation
- Learned/imported RIB

## Config Shape

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
connect_timeout_ms = 5000  # Bounded nonblocking connect
advertised_prefixes = "10.0.0.0/8,192.168.0.0/16"  # Comma-separated CIDR
advertised_prefix_files = "/etc/kgb/bgp-prefixes.conf"  # Optional BIRD-style prefix files
same_as = false
```

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
├── serve_integration.zig # Runtime wiring + reconnect (ACT 4/5)
├── serve_integration_tests.zig # Integration tests (ACT 4)
├── reconnect_lifecycle.zig # Reconnect/backoff lifecycle (ACT 5)
├── prefix_file_integration_tests.zig # Prefix file integration tests (ACT 6)
├── backoff_tests.zig     # Backoff computation tests (ACT 5)
├── lifecycle_tests.zig   # Reconnect lifecycle tests (ACT 5)
├── clock.zig             # Testable clock for timers
└── status.zig            # Status reporting (ACT 5)
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
    connect_timeout_ms: u32,     // Bounded nonblocking connect
    prefixes: []const Ipv4Prefix, // Must be non-empty
    same_as: bool,               // true = empty AS_PATH
};
```

## Future ACTs

| ACT | Description |
|-----|-------------|
| ACT 6 | Add advertised_prefix_files support |
| ACT 7 | Add BFD-gated BGP advertisement |
| ACT 8 | Add direct BgpServeBundle error-propagation regression test |
| ACT 9 | Add BGP route import/export status semantics |
| ACT 10 | Add graceful withdrawal on shutdown |
| ACT 11 | Add 32-bit ASN capability support |
| ACT 12 | Add multiple BGP peers |

## References

- RFC 4271 — Border Gateway Protocol 4 (BGP-4)
  - Sections 4.1 (message format)
  - Sections 4.2 (OPEN message)
  - Sections 4.3 (UPDATE message)
  - Sections 5.1 (path attributes: ORIGIN, AS_PATH, NEXT_HOP)
  - Section 8 (BGP State Machine)
  - Sections 8.2 (State Machine Description)
