# [Open] Epic: Bootstrap Zig tovarisch leaf service

## Goal

Bootstrap `tovarisch`, the constrained Zig leaf daemon for KGB.

`tovarisch` is responsible for local leaf-node supervision primitives:

- CLI identity
- local self-check
- machine-readable status
- future tunnel supervision
- future desired-state pull
- future signed status reports
- future tiny local diagnostics

## Non-goals

- No Kubernetes dependency.
- No container-first runtime.
- No generic observability agent.
- No destination/user activity logging.
- No modern web UI.
- No transport cryptography implementation in this spike.

## ACT 1 Scope

Create the first Zig package and wire it into the repo quality gate.

## Board

| ID | Work Item | Status |
|---|---|---|
| tovarisch-001 | Add this bootstrap epic doc | open |
| tovarisch-002 | Add `tovarisch/` Zig package skeleton | open |
| tovarisch-003 | Implement `tovarisch --version` | open |
| tovarisch-004 | Implement `tovarisch check` | open |
| tovarisch-005 | Implement `tovarisch status --json` static v0 payload | open |
| tovarisch-006 | Add Zig build/test/fmt checks to quality gate when Zig is available | open |
| tovarisch-007 | Add Makefile targets for tovarisch build/test/run | open |
| tovarisch-008 | Update TODO to reflect Zig spike start | open |
| tovarisch-009 | Run `make gate` | open |

## Acceptance

- `zig build` succeeds under `tovarisch/`.
- `zig build test` succeeds under `tovarisch/`.
- `zig build run -- --version` prints a version.
- `zig build run -- check` exits successfully.
- `zig build run -- status --json` emits a bounded static JSON payload.
- Repo-level `make gate` runs existing docs checks and Zig checks when Zig is installed.