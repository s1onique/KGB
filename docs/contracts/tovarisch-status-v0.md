# `tovarisch status --json` v0 Contract

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
