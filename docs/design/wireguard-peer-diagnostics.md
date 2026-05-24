# WireGuard Peer Diagnostics: Design Note

## Purpose

This document defines the trust model and implementation approach for WireGuard-specific diagnostics in KGB's `tovarisch` leaf daemon. It prepares for a future ACT that adds peer-level visibility to the existing tunnel presence check.

## Current State

`tovarisch status --json` includes a `tunnel` check that:
- Detects tunnel-like interfaces by name (`wg*`, `tun*`, etc.)
- Reports `ok` if any exist, `warn` if none
- Uses sysfs enumeration (`/sys/class/net`)

**Limitation**: Presence detection does not imply health. A `wg0` interface existing locally does not indicate peer connectivity.

## Goal: Peer Diagnostics

Future implementation should expose limited WireGuard peer telemetry without:
- Mutating routing or tunnel state
- Probing remote endpoints
- Exposing secrets or private keys
- Monitoring human behavior

## Data Collection Approaches

### Option A: Passive Interface Detection (sysfs)

**Mechanism**: Read from `/sys/class/net/<iface>/wireguard/` or parse `/sys/kernel/debug/wireguard/<iface>/peers/*/handshake_last_time_ms` if accessible.

**Pros**:
- No external process invocation
- Kernel-provided data
- Consistent with current sysfs patterns

**Cons**:
- Not universally available (debugfs may be unmounted)
- WireGuard-specific kernel internals vary by version
- Limited peer-level detail without debugfs

### Option B: `wg show` Command Invocation

**Mechanism**: Shell out to `wg show <iface>` or `wg show all` and parse stdout.

**Pros**:
- Canonical WireGuard tool
- Rich peer data (endpoint, allowed IPs, handshake age, transfer totals)
- Stable interface across WireGuard versions

**Cons**:
- Requires `wg` binary on target system
- External process spawn adds latency and complexity
- May require elevated privileges in some configurations
- Adds a runtime dependency

### Option C: Netlink-based Discovery

**Mechanism**: Query WireGuard via generic netlink or `rtnl` with WireGuard-specific extensions.

**Pros**:
- No external binaries
- Programmatic, parseable output

**Cons**:
- Complex implementation
- WireGuard netlink interface is not universally stable
- May require kernel module headers

## Recommendation for v0

**Approach**: Shell out to `wg show` is acceptable for v0.

**Rationale**:
1. WireGuard is typically installed alongside the `wg` CLI on production nodes
2. Parsing human-readable output is simple and debuggable
3. Alternative approaches (debugfs, netlink) are more fragile and kernel-version dependent
4. The external process invocation is bounded (single shot, no loop)

**Fallback**: If `wg` is unavailable, diagnostics return `unavailable` (not error).

Although `wg show` may expose endpoint and allowed IP data, v0 diagnostics intentionally discard those fields and only report aggregate peer count, latest handshake age, and aggregate transfer totals.

## Proposed WireGuard Diagnostic Fields

For a future `wg_peers` check in the status contract:

| Field | Type | Description |
|-------|------|-------------|
| `interface` | string | WireGuard interface name (e.g., `wg0`) |
| `peer_count` | integer | Number of configured peers |
| `latest_handshake_age_sec` | integer \| null | Seconds since most recent handshake across all peers, or `null` if no handshakes |
| `rx_bytes` | integer \| null | Total bytes received via WireGuard, or `null` if unavailable |
| `tx_bytes` | integer \| null | Total bytes transmitted via WireGuard, or `null` if unavailable |

### Example Output

```json
{
  "name": "wg_peers",
  "status": "ok",
  "detail": "wg0: 2 peers, latest handshake 120s ago",
  "interface": "wg0",
  "peer_count": 2,
  "latest_handshake_age_sec": 120,
  "rx_bytes": 1048576,
  "tx_bytes": 524288
}
```

### Unavailable Fallback

```json
{
  "name": "wg_peers",
  "status": "warn",
  "detail": "wg command unavailable",
  "interface": null,
  "peer_count": null,
  "latest_handshake_age_sec": null,
  "rx_bytes": null,
  "tx_bytes": null
}
```

## Explicit Non-Goals

The WireGuard diagnostics feature explicitly excludes:

1. **No route mutation**: Will not add/remove routes or modify routing tables
2. **No endpoint probing**: Will not ping or reachability-test peer endpoints
3. **No secret exposure**: Will never output private keys, preshared keys, or session keys
4. **No private key inspection**: Will not read or report WireGuard private key material

## Privacy Alignment

WireGuard peer diagnostics align with KGB's privacy doctrine:

| Allowed | Forbidden |
|---------|-----------|
| Node identity (interface name) | Browsing history |
| Peer count (aggregate fact) | Visited domains |
| Last handshake age (transport state) | Destination IP flow logs |
| Transfer totals (aggregate, not per-destination) | Message contents |
| Reachability indicators | Per-user behavioral timelines |

**Rationale**: Handshake age and transfer totals describe tunnel transport health, not human activity.

## Implementation Constraints

1. **Bounded output**: WireGuard diagnostics must not grow unbounded (e.g., listing all peer endpoints)
2. **No dynamic memory growth**: Peer count is capped at compile-time or config-time limits
3. **Graceful degradation**: Unavailable data returns `null`, not errors
4. **No blocking I/O**: WireGuard data collection must not block the heartbeat loop

## Future Considerations

- **WireGuard interface selection**: Should we target a specific interface (config-driven) or enumerate all `wg*` interfaces?
- **Handshake threshold alerts**: Could flag `warn` when latest handshake exceeds configurable threshold
- **Peer-level detail**: Future versions may expose per-peer data if bounded (limited to N peers)

## Contract Version Impact

This design note targets a future version of `tovarisch-status-v0.md`. Implementation will require:

1. New contract version (e.g., `0.2.0`)
2. New `wg_peers` check definition
3. Test fixtures for success and fallback cases

## References

- [tovarisch-status-v0.md](../contracts/tovarisch-status-v0.md) — Current status contract
- [privacy.md](../doctrine/privacy.md) — KGB privacy doctrine
- [tiny-leafs.md](../doctrine/tiny-leafs.md) — Leaf constraints
