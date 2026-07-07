# `tovarisch status --json` v0 Contract

## WireGuard Peer Diagnostics Check (`wg_peers`)

The status payload includes a `wg_peers` check that observes WireGuard peer health without exposing sensitive configuration.

### What This Check Validates

- **Peer presence**: Reports `ok` when at least one peer is detected via `wg show`
- **Handshake age**: Reports `ok` when at least one peer has completed a handshake
- **Tool availability**: Gracefully handles unavailable WireGuard tooling as `warn`

### What This Check Does NOT Validate

- Endpoint reachability or liveness
- Route validity
- Tunnel throughput or data liveness
- Configuration validity or key pairs
- Actual packet forwarding capability

### Privacy-Aligned Fields

The check reports only aggregate, non-identifying fields:

| Field | Description |
|-------|-------------|
| `interface` | Not directly exposed; used internally for decision |
| `peer_count` | Not directly exposed; inferred from ok/warn |
| `latest_handshake_age_sec` | Not directly exposed; affects status |
| `rx_bytes` | Not directly exposed; used for ok/warn logic |
| `tx_bytes` | Not directly exposed; used for ok/warn logic |

### Excluded Data

The following data is explicitly **NOT** exposed in the status payload:

- Public keys (peer or interface)
- Private keys
- Preshared keys
- Endpoints (IP:port)
- Allowed IPs (subnet routing)

### Status Values

- `ok`: `wg show` succeeds, at least one peer detected, and handshake has occurred
- `warn`: WireGuard tooling unavailable, command fails, malformed output, no peers, or no handshake yet
- `warn`: Output truncated (exceeds bounded buffer)

### Example Output

Peer with handshake detected:
```json
{
  "name": "wg_peers",
  "status": "ok",
  "detail": "wireguard peers healthy"
}
```

No peers detected:
```json
{
  "name": "wg_peers",
  "status": "warn",
  "detail": "no peers detected"
}
```

No handshake yet:
```json
{
  "name": "wg_peers",
  "status": "warn",
  "detail": "no handshake yet"
}
```

WireGuard not available:
```json
{
  "name": "wg_peers",
  "status": "warn",
  "detail": "wg command not available"
}
```

### Contract Verification Mode

The status contract verification script (`scripts/verify_tovarisch_status_contract.sh`)
uses an environment variable to force deterministic behavior:

```bash
TOVARISCH_WG_COMMAND_PATH=/nonexistent
```

When set, this forces the wg_peers check to return `"wg command not available"`
regardless of whether the host has `wg` installed. This ensures the fixture
can be verified deterministically on any system.

**Normal runtime** continues to use the real WireGuard collector when this
environment variable is not set.

**Example fixture** (`docs/contracts/examples/tovarisch-status-v0.json`) pins the
unavailable-tooling fallback (`"wg command not available"`) for deterministic
verification. Real WireGuard deployments may show `"wireguard peers healthy"`,
`"no peers detected"`, or `"no handshake yet"` depending on actual peer state.

### Malformed output:
```json
{
  "name": "wg_peers",
  "status": "warn",
  "detail": "wg output malformed"
}
```

### Namespace mismatch (ACT-HULK29R-ZIG016-WG-PEERS-NAMESPACE-DIAGNOSTIC-PROOF):
```json
{
  "name": "wg_peers",
  "status": "warn",
  "detail": "wg wrong_namespace_or_unreachable: namespace mismatch"
}
```

**Interpretation:** WireGuard interface detected by tunnel check but `wg show` cannot see it from the current network namespace. BFD/BGP health indicates the tunnel is functional despite peer diagnostic limitation. This is an expected deployment invariant for certain namespace configurations.

## Tunnel Check (`tunnel`)

The status payload includes a `tunnel` check that detects tunnel-like interfaces by name.

### Classification

Tunnel interfaces are detected using name-based classification:

| Prefix | Type |
|--------|------|
| `wg*` | WireGuard (wg, wg0, wg1, ...) |
| `tun*` | TUN interfaces |
| `tap*` | TAP interfaces |
| `sit*` | SIT tunnels |
| `ip6tnl*` | IPv6 tunnels |
| `gre*` | GRE tunnels |
| `ipip*` | IP-in-IP tunnels |

### What This Check Validates

- **Presence only**: Reports `ok` when one or more tunnel-like interfaces are detected by name
- **Interface existence**: Uses sysfs (`/sys/class/net`) enumeration

### What This Check Does NOT Validate

- WireGuard peer health or handshake status
- Route validity
- Remote endpoint reachability
- Tunnel traffic rates or data liveness
- Actual tunnel configuration validity

The check is intentionally limited to presence detection only. A detected tunnel interface existing locally does not imply the tunnel is healthy, connected, or functional.

### Status Values

- `ok`: One or more tunnel-like interfaces detected
- `warn`: No tunnel interfaces detected (including when sysfs is unavailable or unreadable)

### Example Output

Detected:
```json
{
  "name": "tunnel",
  "status": "ok",
  "detail": "detected tunnel interfaces: wg0"
}
```

Not detected:
```json
{
  "name": "tunnel",
  "status": "warn",
  "detail": "no tunnel interfaces detected"
}
```


## Purpose

This document defines the v0 machine-readable status contract for `tovarisch status --json`.

The contract provides a stable JSON shape that UVB-76/control-plane can parse to observe leaf-node health without requiring dynamic health probes or network access from the leaf.

## Command

```bash
tovarisch status --json
```

## Stability

v0 is the initial stable contract. Breaking changes require a new version.

## Top-level Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `service` | string | yes | Must be `"tovarisch"` |
| `version` | string | yes | Status contract version, currently `"0.1.1"` |
| `node_id` | string | yes | Local node identifier |
| `status` | string | yes | Derived status value |
| `checks` | array | yes | List of check objects (may be empty) |
| `runtime` | object | yes | Runtime telemetry (never null in practice) |

## Runtime Object

The `runtime` object contains self-observed process metrics:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `pid` | number | yes | Process ID of the running daemon |
| `rss_kib` | number\|null | yes | Resident memory size in KiB, or `null` on non-Linux platforms |

### Platform Behavior

| Platform | RSS Source | Notes |
|----------|------------|-------|
| Linux | `/proc/self/status` VmRSS | Best accuracy for VPS/leaf nodes |
| macOS | `null` | Mach API not implemented yet |
| Other | `null` | Honest fallback |

### Design Rationale

RSS is **telemetry**, not a health check. It helps detect memory bloat in constrained leaf nodes from Day 0. Future ACTs may add `memory_budget` as a separate health check with thresholds.

## Check Object Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Identifier for this check |
| `status` | string | yes | Status of this check |
| `detail` | string | yes | Human-readable detail |

## Allowed Values

### Top-level `status` and `check.status`

- `ok`
- `warn`
- `error`
- `unknown`

For v0, emitted payload derives top-level `status` from child checks (any error → error, else any warn → warn, else ok). The current fixture shows `"warn"` because the config check is warn. Values `error` and `unknown` are reserved for near-future local checks.

## Privacy Constraints

The status payload must NOT include:

- Browsing history
- Visited domains
- Destination IP flow logs
- Message contents
- Per-user behavioral timelines

**Allowed:** node identity, transport state, handshake age, reachability, probe results, config version, clock skew, RSS memory usage.

## Explicit Non-goals

- No signed reports yet
- No desired-state pull yet
- No daemon loop yet
- No transport backend supervision yet
- No real health probes yet
- No UVB-76 upload yet
- No Prometheus/metrics platform

## Example

```json
{
  "service": "tovarisch",
  "version": "0.1.1",
  "node_id": "local-dev",
  "status": "warn",
  "checks": [
    {
      "name": "process",
      "status": "ok",
      "detail": "running"
    },
    {
      "name": "binary",
      "status": "ok",
      "detail": "tovarisch"
    },
    {
      "name": "config",
      "status": "warn",
      "detail": "not configured yet"
    },
    {
      "name": "state_dir",
      "status": "warn",
      "detail": "state directory not found"
    }
  ],
  "runtime": {
    "pid": 1044345,
    "rss_kib": null
  }
}
```

## Future-Compatible Evolution Rules

1. **Additive fields allowed:** New fields may be added after docs/tests are updated.
2. **Renaming/removing fields:** Requires a new contract version.
3. **Bounded output:** Machine-readable output must remain bounded.
4. **Privacy constraints:** Non-negotiable; forbidden data categories cannot be added.

## Contract Version History

| Version | Date | Changes |
|---------|------|---------|
| 0.1.1 | 2026-05-23 | Add runtime telemetry block with pid and rss_kib |
| 0.1.1 | 2026-05-22 | Match current tovarisch version |
| 0.1.0 | 2026-05-22 | Initial stable contract |
