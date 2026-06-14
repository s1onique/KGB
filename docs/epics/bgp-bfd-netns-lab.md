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
          sudo apt-get install -y iproute2 bird2 tcpdump jq

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
├── tovarisch.conf    # Generated tovarisch config
├── prefixes.txt      # Initial prefix file
├── bird.log          # BIRD daemon log
├── tovarisch.log     # tovarisch daemon log
├── status.json       # tovarisch status --json output
└── bird-routes.txt   # BIRD route table
```

## GitHub Actions Status

**NOT YET RUN** — Infrastructure added. First manual CI run pending.

The workflow must be triggered manually from the Actions tab to validate the netns topology and BFD/BGP convergence.

## Promotion Policy

After 3-5 clean manual CI runs, open a new ACT:

> `ACT: Promote BGP/BFD netns lab from manual to scheduled advisory workflow`

This keeps the promotion explicit and reviewable.

## Future Work

- ACT 2: Assert BFD session reaches Up
- ACT 3: Assert BGP session establishes
- ACT 4: Assert prefix file watch add/remove/invalid-reload behavior
- ACT 5: Promote from manual to scheduled advisory workflow
