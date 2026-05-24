# `tovarisch` Local Health Checks v0 Contract

## Purpose

This document defines the v0 contract for local health checks that `tovarisch status --json` emits. It extends the base status contract in `tovarisch-status-v0.md` with specific check definitions, semantics, and scope boundaries.

The health checks answer a single question:

> "Can this constrained leaf node still serve its anti-censorship / tunnel-supervision role, and what is degraded?"

## Check Taxonomy

All health checks are identified by a canonical name. Check names are stable; adding new checks is additive, renaming requires a contract version bump.

### Defined Checks

| Check Name | Purpose | Initial Behavior | Future Behavior |
|------------|---------|------------------|-----------------|
| `process` | Is `tovarisch` alive enough to report? | Always `ok` | Same |
| `binary` | Is the running binary identifiable? | Always `ok` | Same |
| `config` | Is local config present and parseable? | `warn` until config exists | Same |
| `state_dir` | Can local state directory be found/used? | `warn` if missing, `ok` if usable | `opendir()` v0; file-vs-dir distinction is accepted uncovered |
| `network` | Is basic local network usable? | `warn` until configured | `ok` with valid config |
| `routes` | Are required routes/interfaces visible? | N/A | Later ACT |
| `control_tower` | Can a configured tower be reached? | N/A | Later ACT |
| `tunnel` | Is the supervised tunnel healthy? | N/A | Later ACT |

### Check Object Shape

Each check emits:

```json
{
  "name": "config",
  "status": "warn",
  "detail": "not configured yet"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Canonical check identifier |
| `status` | string | yes | One of: `ok`, `warn`, `error`, `unknown` |
| `detail` | string | yes | Human-readable detail (empty string allowed) |

## Status Derivation Rules

The top-level `status` field is derived from child checks:

1. **Any `error` in checks** → top-level `error`
2. **Any `warn` in checks** (no errors) → top-level `warn`
3. **All `ok`** → top-level `ok`
4. **`unknown` only** → top-level `unknown`

## Allowed Status Values

| Value | Meaning |
|-------|---------|
| `ok` | Check passed; component is healthy |
| `warn` | Check passed but with degraded condition or missing optional config |
| `error` | Check failed; component is not functioning correctly |
| `unknown` | Check could not be performed (e.g., timeout, permission denied) |

## Timeout and Error Expectations

### Timing Constraints

- Each check must complete within **5 seconds** or emit `unknown`.
- Checks are **synchronous** for v0; no background loops.
- Total status generation must complete within **10 seconds**.

### Error Discipline

| Condition | Status | Detail Pattern |
|-----------|--------|----------------|
| Check succeeded | `ok` | Normal operation message |
| Check passed but degraded | `warn` | Explanation of degradation |
| Check failed permanently | `error` | Specific failure reason |
| Check could not run | `unknown` | Timeout or permission issue |

### Timeout Behavior

```text
if check_execution_time > 5s:
    emit { status: "unknown", detail: "timeout exceeded" }
```

## Config-Not-Present Semantics

Many checks depend on optional configuration. The semantics:

| Config Missing | Check Status | Detail Format |
|----------------|--------------|---------------|
| Required config | `error` | `"config required: <file>"` |
| Optional config | `warn` | `"not configured yet"` |
| State dir missing | `warn` | `"state directory not found"` |

The `config` check specifically reports on the primary config file presence.

## Explicit Non-Goals (Scope Boundary)

This is **NOT** a generic observability stack. Explicitly excluded:

| Forbidden | Reason |
|-----------|--------|
| Prometheus exporter | Not a metrics platform |
| Netdata-style dashboards | Not a general-purpose monitor |
| Kubernetes assumptions | Not designed for k8s environments |
| Container visibility | Not container-aware |
| Embedded TSDB | No time-series storage |
| Full observability stack | Constrained leaf, not full agent |
| Unbounded metrics | Bounded output only |
| Browsing history | Privacy violation |
| Visited domains | Privacy violation |
| Destination IP flow logs | Privacy violation |
| Per-user behavioral timelines | Privacy violation |

### What Is Allowed

The checks focus on **infrastructure health only**:

- Node identity and configuration presence
- Transport state and reachability
- Tunnel supervision health
- Config version and validity
- Basic local network state

## Implementation Constraints

For v0 ACTs, maintain these constraints:

```text
No daemon yet.
No background loops yet.
No async scheduler yet.
No “metrics platform”.
No Prometheus exporter.
No Kubernetes thinking.
```

Each new check must be:
- Deterministic
- Locally testable
- Gate-backed
- Bounded in execution time

## Contract Version

| Version | Date | Changes |
|---------|------|---------|
| 0.1.0 | 2026-05-22 | Initial local health check contract |

## Relationship to Base Status Contract

This contract extends `tovarisch-status-v0.md`:

- Base contract defines: top-level fields, JSON shape, validation rules
- This contract defines: specific check names, semantics, scope boundaries
- Together they form the complete v0 status+health specification

## Future Evolution

1. **New checks**: Additive; update this doc and tests
2. **Check removal**: Requires contract version bump
3. **New status values**: Requires contract version bump
4. **Timing changes**: Additive config options allowed
