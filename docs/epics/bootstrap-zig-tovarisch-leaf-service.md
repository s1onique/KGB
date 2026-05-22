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

### ACT 1 Board

| ID | Work Item | Status |
|---|---|---|
| tovarisch-001 | Add this bootstrap epic doc | **done** |
| tovarisch-002 | Add `tovarisch/` Zig package skeleton | **done** |
| tovarisch-003 | Implement `tovarisch --version` | **done** |
| tovarisch-004 | Implement `tovarisch check` | **done** |
| tovarisch-005 | Implement `tovarisch status --json` static v0 payload | **done** |
| tovarisch-006 | Add Zig build/test/fmt checks to quality gate when Zig is available | **done** |
| tovarisch-007 | Add Makefile targets for tovarisch build/test/run | **done** |
| tovarisch-008 | Update TODO to reflect Zig spike start | **done** |
| tovarisch-009 | Run `make gate` | **done** |

### ACT 1 Acceptance

- [x] `zig build` succeeds under `tovarisch/`.
- [x] `zig build test` succeeds under `tovarisch/`.
- [x] `zig build run -- --version` prints a version.
- [x] `zig build run -- check` exits successfully.
- [x] `zig build run -- status --json` emits a bounded static JSON payload.
- [x] Repo-level `make gate` runs existing docs checks and Zig checks when Zig is installed.

## ACT 2 Scope

Add repo-local recent-Zig knowledge pack and restore meaningful tests by separating pure CLI/status logic from process-entrypoint logic.

### ACT 2 Board

| ID | Work Item | Status |
|---|---|---|
| tovarisch-010 | Add Zig 0.16 field manual | **done** |
| tovarisch-011 | Add Cline/MiniMax context doc | **done** |
| tovarisch-012 | Split `main.zig` into entrypoint/CLI/status modules | **done** |
| tovarisch-013 | Restore meaningful Zig tests | **done** |
| tovarisch-014 | Update gate to require tooling docs | **done** |
| tovarisch-015 | Run `make gate` | **done** |

### ACT 2 Acceptance

- [x] `docs/tooling/zig-0.16-field-manual.md` exists and documents Zig 0.16 patterns.
- [x] `docs/tooling/cline-context.md` exists for Cline/MiniMax.
- [x] `tovarisch/src/main.zig` is process entrypoint only.
- [x] `tovarisch/src/cli.zig` exports `run()` and `ExitCode` for unit testing.
- [x] `tovarisch/src/status.zig` owns status payload construction.
- [x] Tests cover: `--version`, `check`, `status --json`, unknown command, missing args.
- [x] Gate requires the new tooling docs and validates field manual content.

## ACT 3a Scope

Add canonical coding-agent guidance via `AGENTS.md` and `.clinerules`.

### ACT 3a Board

| ID | Work Item | Status |
|---|---|---|
| tovarisch-016 | Add root `AGENTS.md` | **done** |
| tovarisch-017 | Add Cline bootstrap rule | **done** |
| tovarisch-018 | Add KGB doctrine rule | **done** |
| tovarisch-019 | Add Zig 0.16 rule | **done** |
| tovarisch-020 | Add verification rule | **done** |
| tovarisch-021 | Update quality gate | **done** |
| tovarisch-022 | Run `make gate` | **done** |

### ACT 3a Acceptance

- [x] `AGENTS.md` exists and contains: Zig learning protocol, "Do not downgrade Zig", "KGB observes infrastructure health, not people".
- [x] `.clinerules/00-bootstrap.md` exists and references `AGENTS.md`.
- [x] `.clinerules/10-kgb-doctrine.md` exists with KGB doctrine summary.
- [x] `.clinerules/20-zig-016.md` exists with Zig 0.16 rules.
- [x] `.clinerules/30-verification.md` exists with verification rules.
- [x] `scripts/quality_gate.sh` checks for new files and content.
- [x] `make gate` passes.

## Future Work

- Define signed report schema
- Define desired-state schema
- Define transport backend interface
- Add tunnel supervision (future)
- Add desired-state pull (future)
- Add signed status reports (future)
- Add tiny local diagnostics (future)
