# [Open] Epic: BGP/BFD Netns Lab

## Goal

Add a manual GitHub Actions production-path lab for tovarisch BFD/BGP behavior using Linux network namespaces. This must run in CI via `workflow_dispatch`, not depend on local snowflake Linux VMs, and not run automatically yet.

## Intent

Create a repeatable CI-hosted lab that can later be promoted to scheduled automation after several clean manual runs.

## Context

KGB/tovarisch doctrine prefers production-path parity where feasible. We want real Linux sockets, namespaces, BIRD, and tovarisch runtime behavior without depending on the operator's local machine or real production peers.

## Non-goals

- No `push`, `pull_request`, or `schedule` triggers yet.
- No promotion to scheduled CI in this ACT.
- No local snowflake VM requirement as primary path.
- No external peers or secrets.

## ACT 1 Scope

Add manual CI lab infrastructure.

### ACT 1 Board

| ID | Work Item | Status |
|----|-----------|--------|
| netns-001 | Create `.github/workflows/bgp-bfd-netns-lab.yml` | **done** |
| netns-002 | Create `scripts/lab_bgp_bfd_netns.sh` harness | **done** |
| netns-003 | Add `lab-bgp-bfd` to Makefile | **done** |
| netns-004 | Document lab in README.md | **done** |
| netns-005 | Create WAL/close-report doc | **done** |
| netns-006 | Run `make gate` | **done** |
| netns-007 | Run `bash -n` syntax check | **done** |

### ACT 1 Acceptance

- [x] `.github/workflows/bgp-bfd-netns-lab.yml` exists and is `workflow_dispatch` only.
- [x] `make lab-bgp-bfd` runs the lab harness.
- [x] The harness uses Linux network namespaces and generated configs.
- [x] The harness has bounded waits and cleanup traps.
- [x] Logs/configs/status snapshots are saved for CI artifacts.
- [x] No external peers, secrets, or local snowflake VM required.
- [x] No push/PR/schedule trigger added.
- [x] README documents manual CI lab as primary path.
- [x] `make gate` passes.
- [x] All scripts pass syntax check.

## ACT 1.5 Scope

Upgrade lab to prove running `tovarisch serve --config` loaded BFD/BGP config.

### Key Insight: CLI vs Runtime Status

The existing lab collected `tovarisch status --json` (standalone CLI) which calls `getLocalChecks()` with `null` BFD runtime. This always shows "bfd not configured" regardless of whether the daemon has loaded config.

The running `serve` process wires BFD/BGP runtime from config into `ServeContext`, which the HTTP `/status.json` endpoint uses. This endpoint CAN prove config was loaded.

### Product Capability: HTTP Status Endpoint

`tovarisch serve` exposes `GET /status.json` on `127.0.0.1:8317` (loopback) which uses `ServeContext` with the actual BFD/BGP runtime state. When config is loaded:
- BFD check shows runtime state (peer count, session states) not "not configured"
- BGP check shows loaded configuration state

### Implementation

1. **New artifact**: `status-http.json` from runtime HTTP endpoint
2. **New assertion**: Runtime HTTP status endpoint responds with valid JSON
3. **New assertion**: Runtime BFD/BGP check shows config was loaded (not "not configured")
4. **Curl dependency**: Added to GitHub Actions apt install list
5. **Artifact distinction**: Renamed `status.json` → `status-cli.json`, new `status-http.json`

### ACT 1.5 Board

| ID | Work Item | Status |
|----|-----------|--------|
| netns-008 | Add HTTP status collection function | **done** |
| netns-009 | Rename CLI status artifact | **done** |
| netns-010 | Add curl dependency to workflow | **done** |
| netns-011 | Add v1.5 runtime assertions | **done** |
| netns-012 | Update WAL with findings | **done** |
| netns-013 | Run `make gate` | **done** |

### ACT 1.5 Acceptance

- [x] `status-cli.json` artifact for standalone `tovarisch status --json`
- [x] `status-http.json` artifact for runtime `/status.json` from serve process
- [x] `curl` installed in GitHub Actions
- [x] Runtime HTTP status endpoint validation (valid JSON)
- [x] Runtime BFD check does not show "bfd not configured"
- [x] Runtime BGP check does not show "BGP not configured"
- [x] BFD/BGP convergence still deferred
- [x] Workflow remains `workflow_dispatch` only

## ACT 2 Scope

Assert BFD session reaches Up between `tovarisch` and BIRD.

### Root Cause of Previous BFD Non-Convergence

**Single-hop vs multihop BFD port mismatch.**

tovarisch implements BFD multihop on UDP port 4784 (per RFC 5883). The original BIRD config used:

```
protocol bfd {
    interface "$VETH_BIRD" { ... };
}
```

This is **single-hop BFD** which uses UDP port 3784. Both sides looked "configured" but could never form a session because they were listening on different UDP ports.

### Fix

Changed BIRD config to use multihop mode:

```
protocol bfd {
    multihop {
        interval $BFD_INTERVAL_MS ms;
        multiplier $BFD_MULTIPLIER;
    };
}
```

This aligns BIRD with tovarisch's UDP 4784 multihop behavior.

### Implementation

1. **Fix BIRD config generation**: Use `multihop {}` block instead of `interface {}`
2. **Add `assert_bfd_up()` function**: Bounded wait (30s) polling BIRD for "Up" state
3. **Add packet-level diagnostics**: tcpdump captures for UDP 4784 and UDP 3784
4. **Add BFD session artifact**: `bird-bfd-sessions.txt` for evidence
5. **Add HTTP status collection**: `status-http-bfd.json` for tovarisch BFD state
6. **Add failure diagnostics**: BIRD sessions, protocols, tovarisch logs, tcpdump

### ACT 2 Board

| ID | Work Item | Status |
|----|-----------|--------|
| netns-014 | Fix BIRD config to use multihop mode | **done** |
| netns-015 | Add assert_bfd_up() function | **done** |
| netns-016 | Add tcpdump packet diagnostics | **done** |
| netns-017 | Add BFD session artifact collection | **done** |
| netns-018 | Add HTTP status BFD evidence | **done** |
| netns-019 | Update diagnostics on failure | **done** |
| netns-020 | Update WAL with ACT 2 scope | **done** |
| netns-021 | Run `make gate` | **done** |
| netns-022 | Run manual GitHub Actions for BFD Up evidence | **pending** |
| netns-023 | Add CI verification script for BFD Up artifact | **done** |

### ACT 2 Acceptance Criteria

#### Implemented (local verification)

- [x] BIRD config fixed to use multihop mode
- [x] `assert_bfd_up()` function added
- [x] BFD Up detection regex: `(^|[[:space:]])Up([[:space:]]|$)`
- [x] tcpdump packet diagnostics added
- [x] BFD session artifact collection (`bird-bfd-sessions.txt`)
- [x] HTTP status BFD evidence (`status-http-bfd.json`)
- [x] Failure diagnostics with actionable output
- [x] `make gate` passes
- [x] Bash syntax checks pass
- [x] Workflow remains `workflow_dispatch` only
- [x] BGP convergence remains deferred (non-fatal)
- [x] CI verifies `bird-bfd-sessions.txt` contains `Up` before workflow can pass

#### Manual CI Close Condition

Manual workflow PASS means:
- lab exited 0
- `scripts/verify_bfd_up_artifact.sh` found latest lab artifact
- `bird-bfd-sessions.txt` exists
- `bird-bfd-sessions.txt` contains token `Up`
- artifacts uploaded

#### Requires Manual CI Verification

- [ ] **BFD session reaches Up in GitHub Actions**
- [ ] **BFD Up evidence saved in CI artifacts**
- [ ] **Artifacts upload succeeds on both success and failure**
- [ ] **Workflow PASS with BFD Up evidence**

### BFD Mode: Multihop

| Property | Value |
|----------|-------|
| Mode | multihop |
| UDP Port | 4784 (RFC 5883) |
| tovarisch | Always multihop |
| BIRD | Configured as multihop |

### Packet-Level Diagnostics Added

| Artifact | Purpose |
|----------|---------|
| `bird-bfd-sessions.txt` | BIRD BFD session state evidence |
| `status-http-bfd.json` | tovarisch runtime BFD state |
| `tcpdump-bfd-bird.txt` | BFD packets captured in BIRD namespace (UDP 4784/3784) |

### Remaining Deferred

- BGP session establishes
- Prefix file watch add/remove/invalid-reload
- Route announcement/withdrawal visibility
- Promotion to scheduled workflow

### Next Step (ACT 3)

Assert BGP session establishes after BFD is confirmed Up.

## Workflow Shape

```yaml
name: BGP/BFD Netns Lab

on:
  workflow_dispatch:

jobs:
  bgp-bfd-netns-lab:
    runs-on: ubuntu-latest
    timeout-minutes: 20

    steps:
      - uses: actions/checkout@v4

      - name: Install lab dependencies
        run: |
          sudo apt-get update
          sudo apt-get install -y iproute2 bird2 tcpdump jq curl

      - name: Run lab (under sudo for netns)
        run: sudo --preserve-env=PATH,TOVARISCH_BINARY make lab-bgp-bfd

      - name: Upload lab logs
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: bgp-bfd-netns-lab-logs
          path: /tmp/kgb-bgp-bfd-lab-*/**
```

## Namespace Topology (Fixed)

```
kgb-lab-tovarisch namespace          kgb-lab-bird namespace
┌────────────────────────┐          ┌────────────────────────┐
│  tovarisch daemon      │          │  BIRD router            │
│                        │          │                        │
│  10.77.0.2/30          │◄─────────►│  10.77.0.1/30           │
│  (veth-tovarisch)      │  veth     │  (veth-bird)            │
│                        │  pair     │                        │
└────────────────────────┘          └────────────────────────┘
```

Both veth endpoints are moved into their respective namespaces (fixed from v1).

## Lab Assertions (v1)

### Implemented

1. **Namespace topology** — proves both namespaces exist and veths are inside them
2. **IP verification** — confirms 10.77.0.2 in tovarisch ns, 10.77.0.1 in BIRD ns
3. **Connectivity test** — bidirectional ping between namespaces
4. **BIRD startup** — proves BIRD starts with generated config
5. **tovarisch startup** — proves tovarisch starts with generated config
6. **Status JSON collectable** — proves `tovarisch status --json` emits valid JSON
7. **BFD convergence attempt** — bounded wait with diagnostic output
8. **BGP convergence attempt** — bounded wait with diagnostic output

### Deferred

- BFD session reaches Up
- BGP session establishes
- Prefix file watch add/remove/invalid-reload
- Route announcement/withdrawal visibility

## Dependencies

- `iproute2` — for `ip netns`, `ip link` commands
- `bird2` — BIRD BGP daemon (Ubuntu ships as `bird2` package)
- `tcpdump` — optional packet capture
- `jq` — JSON verification
- `sudo` — required for network namespace operations on GitHub Actions

## BIRD Version Handling

GitHub Actions installs `bird2` (BIRD 2.x). The harness detects version:

```bash
birdc show version || bird --version
```

BIRD 2.x syntax is used by default (compatible with Ubuntu `bird2` package).

## Execution Policy

- **Primary:** GitHub Actions `workflow_dispatch`
- **Local:** Optional debugging only (requires Linux with network namespaces)
- **Local syntax check:** `bash -n scripts/lab_bgp_bfd_netns.sh scripts/lab_bgp_bfd_netns_lib.sh scripts/lab_bgp_bfd_netns_consts.sh`

## Artifacts

On lab completion (success or failure):

```
/tmp/kgb-bgp-bfd-lab-<timestamp>/
├── bird.conf          # Generated BIRD config
├── tovarisch.conf     # Generated tovarisch config
├── prefixes.txt       # Initial prefix file
├── bird.log           # BIRD daemon log
├── tovarisch.log      # tovarisch daemon log
├── status-cli.json    # tovarisch status --json output (CLI, standalone)
├── status-http.json   # Runtime /status.json from serve process (HTTP)
└── bird-routes.txt    # BIRD route table
```

### Artifact Distinction: CLI vs Runtime

| Artifact | Source | Proves |
|----------|--------|--------|
| `status-cli.json` | `tovarisch status --json` (standalone CLI) | JSON collectability works; BFD/BGP always show "not configured" |
| `status-http.json` | `curl http://127.0.0.1:8317/status.json` (runtime) | Config was loaded; BFD/BGP reflect runtime state |

**Key insight**: The CLI status command (`status --json`) always shows "bfd not configured" because it calls `getLocalChecks()` with `null` BFD runtime. The runtime HTTP endpoint uses `ServeContext` which contains the actual BFD/BGP runtime state loaded from config.

## GitHub Actions Status

### v1.5 Runtime Config Evidence (Completed)

The following assertions passed in v1.5 manual CI run:

| Assertion | Result | Evidence |
|-----------|--------|----------|
| Namespace creation | ✅ PASS | Both namespaces exist |
| veth placement/IPs | ✅ PASS | IPs verified |
| Bidirectional ping | ✅ PASS | Connectivity confirmed |
| BIRD startup | ✅ PASS | BIRD process running |
| tovarisch startup | ✅ PASS | tovarisch process running |
| CLI status JSON | ✅ PASS | Valid JSON emitted |
| Runtime HTTP status | ✅ PASS | HTTP endpoint responds with valid JSON |
| Runtime BFD config | ✅ PASS | "not configured" NOT shown when config loaded |
| Runtime BGP config | ✅ PASS | "BGP not configured" NOT shown when config loaded |
| BFD convergence | ⏳ DEFERRED | Expected for v1.5 |
| BGP convergence | ⏳ DEFERRED | Expected for v1.5 |

### v1 Caveat (Now Resolved)

v1 status JSON showed `"bfd": "bfd not configured"` and `"bgp": "BGP not configured"`. This only proved JSON collectability, not config loading.

**v1.5 resolution**: Runtime HTTP `/status.json` from the running `serve` process proves BFD/BGP config was loaded. When config is provided, the BFD check reflects peer count/session states and BGP check reflects loaded configuration — not "not configured".

### Harness Exit Semantics (Fixed)

The lab now exits 0 for v1 assertions:
- `collect_bgp_routes` is non-fatal: logs `[DEFERRED]` warning instead of failing
- BIRD command corrected to `show route` (not `show routes`)
- Artifact permissions fixed: harness-side `make_artifacts_readable()` + workflow belt-and-suspenders `sudo chmod`

## Promotion Policy

After 3-5 clean manual CI runs, open a new ACT:

> `ACT: Promote BGP/BFD netns lab from manual to scheduled advisory workflow`

This keeps the promotion explicit and reviewable.

## Future Work

- ACT 2: Assert BFD session reaches Up — **implementation complete, pending manual CI evidence**
- ACT 3: Assert BGP session establishes
- ACT 4: Assert prefix file watch add/remove/invalid-reload behavior
- ACT 5: Promote from manual to scheduled advisory workflow
