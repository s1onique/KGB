# Cline/MiniMax Context Pack

## What is KGB?

KGB is a lightweight anti-censorship control plane for keeping escape routes alive across hostile, degraded, or unreliable networks.

KGB is **not** a VPN protocol. It is a control plane and supervision layer for transport backends.

## What is `tovarisch`?

`tovarisch` is the constrained Zig leaf daemon for KGB. It runs on tiny VPSes and constrained machines.

`tovarisch` responsibilities:
- tunnel supervision
- health probes
- signed status reports (future)
- desired-state pull (future)
- tiny local status/CLI

## Current Zig Version Doctrine

- KGB targets **Zig 0.16.x** style APIs.
- Do **NOT** downgrade Zig to match stale examples.
- Read `docs/tooling/zig-0.16-field-manual.md` before editing Zig.

## Required Files to Read Before Editing Zig

1. `docs/tooling/zig-0.16-field-manual.md`
2. `tovarisch/src/main.zig` (current working implementation)
3. `tovarisch/build.zig`
4. `Makefile`

## Commands to Run

```bash
make gate              # Full quality gate
make tovarisch-build   # Build the Zig package
make tovarisch-test    # Run Zig tests
make tovarisch-status  # Run: tovarisch status --json
```

## Rules

- **Do not** add generic observability-agent behavior.
- **Do not** log browsing history, visited domains, destination IP flow logs, or human behavior.
- **Do not** call `tovarisch` an agent except when explaining generic role.
- **Do not** introduce Kubernetes/container assumptions for leafs.
- **Do not** downgrade Zig.
