# BGP Protocol Module (ACT 1)

## Overview

This is **ACT 1** of in-process BGP support for `tovarisch`. This ACT adds only pure encoding/parsing components — no sockets, no session state machine, no runtime daemon integration.

## Scope of This ACT

Implemented:
- BGP message frame encoding (KEEPALIVE, OPEN, UPDATE)
- IPv4 NLRI prefix encoding
- BIRD-style `route <IPv4 CIDR> reject;` prefix-list parser
- Config validation helpers

**Not implemented** (deferred to future ACTs):
- TCP sockets or BGP session state machine
- 32-bit ASN capability support
- BFD-gated BGP session behavior
- Graceful withdrawal on shutdown
- Runtime daemon integration
- `/status` changes

## Architecture

```
tovarisch/src/bgp/
├── types.zig          # BGP types, constants, Ipv4Prefix
├── message.zig        # Frame encoding (KEEPALIVE, OPEN, UPDATE)
├── message_tests.zig  # UPDATE and NLRI encoding tests
├── validation.zig     # Config validation helpers
└── prefix_file.zig    # BIRD-style prefix-list parser
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

## Prefix-list Parser

The parser supports the exact BIRD static route-list format:

```
route <IPv4 CIDR> reject;
```

**Accepted:**
- `route 23.192.0.0/11 reject;`
- Blank lines
- `#` comments

**Rejected:**
- IPv6 prefixes (`:/` in CIDR)
- `via` routes (e.g., `route ... via ...`)
- Missing semicolons
- Quoted strings or injection-like input
- Unknown BIRD directives

**Mapping:** `route <cidr> reject;` → advertised BGP NLRI

## Validation Rules

- ASN: 1..65535 (16-bit only in this ACT)
- Hold time: 0 or >= 3 seconds
- Keepalive: < hold time (when hold time != 0)
- Prefix length: 0..32 for IPv4
- Prefix list: must not be empty for UPDATE

## Testing

All BGP tests are wired into `test_all.zig`:
- `make tovarisch-test` runs all tests
- `make tovarisch-build` verifies compilation

## Future ACTs

| ACT | Description |
|-----|-------------|
| ACT 2 | Add BGP TCP session state machine |
| ACT 3 | Add BGP runtime wiring and /status state |
| ACT 4 | Add 32-bit ASN capability support |
| ACT 5 | Add BFD-gated BGP session behavior |
| ACT 6 | Add graceful withdrawal on shutdown |

## References

- RFC 4271 — Border Gateway Protocol 4 (BGP-4)
  - Sections 4.1 (message format), 4.2 (OPEN), 4.3 (UPDATE)
  - Sections 5.1 (path attributes: ORIGIN, AS_PATH, NEXT_HOP)
