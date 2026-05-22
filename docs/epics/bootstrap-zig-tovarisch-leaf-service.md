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

## ACT 4 Scope (Day-0 Coverage Doctrine)

Add repository doctrine and quality-gate structure for Day-0 code coverage.

### ACT 4 Board

| ID | Work Item | Status |
|---|---|---|
| tovarisch-023 | Add `docs/doctrine/day-0-code-coverage.md` | **done** |
| tovarisch-024 | Add `docs/doctrine/README.md` doctrine index | **done** |
| tovarisch-025 | Wire coverage section into `scripts/quality_gate.sh` | **done** |
| tovarisch-026 | Add `coverage` target to `Makefile` | **done** |
| tovarisch-027 | Update epic with ACT 4 entry | **done** |
| tovarisch-028 | Run `make gate` | **done** |

### ACT 4 Acceptance

- [x] Coverage is documented as Day-0 practice in `docs/doctrine/day-0-code-coverage.md`.
- [x] `make coverage` exists and gives honest result.
- [x] `scripts/quality_gate.sh` exposes coverage status.
- [x] Missing coverage backend is explicit, not hidden.
- [x] Existing Zig tests still run and fail gate on failure.
- [x] No fake coverage percentage is invented.

## ACT 5 Scope (Status JSON Contract)

Make `tovarisch status --json` a stable, gate-verified JSON contract.

### ACT 5 Board

| ID | Work Item | Status |
|---|---|---|
| tovarisch-029 | Define canonical status JSON schema in `status.zig` | **done** |
| tovarisch-030 | Implement `renderPayload()` / `parseStatus()` for structural validation | **done** |
| tovarisch-031 | Add JSON verification script `verify_status_json.sh` | **done** |
| tovarisch-032 | Wire verification into `scripts/quality_gate.sh` (replace grep check) | **done** |
| tovarisch-033 | Add structural validation tests in `status.zig` | **done** |
| tovarisch-034 | Update `docs/doctrine/day-0-code-coverage.md` to document contract coverage | **done** |
| tovarisch-035 | Update epic with ACT 5 board and acceptance | **done** |
| tovarisch-036 | Run `make gate` | **done** |

### ACT 5 Acceptance

- [x] Minimal canonical status JSON shape defined as `Status` struct with required fields.
- [x] `make tovarisch-status` emits valid JSON (verified by structural parsing).
- [x] `scripts/verify_status_json.sh` parses JSON structurally, validates required fields and types.
- [x] Gate uses structural validation instead of grep for `status --json`.
- [x] Gate fails if `status --json` emits invalid JSON.
- [x] Gate fails if required fields disappear.
- [x] `docs/doctrine/day-0-code-coverage.md` updated to mark `status --json` as contract-validated.
- [x] `make gate` passes.

## ACT 6 Scope (Zig 0.16 Lessons + Text Hygiene)

Capture Zig 0.16 JSON serialization lessons into the field manual and harden repository text hygiene with final-newline checks.

### ACT 6 Board

| ID | Work Item | Status |
|---|---|---|
| tovarisch-037 | Add JSON serialization section to Zig 0.16 field manual | **done** |
| tovarisch-038 | Document `std.json.Stringify` streaming pattern | **done** |
| tovarisch-039 | Add example based on `status --json` | **done** |
| tovarisch-040 | Add final-newline checking to `scripts/quality_gate.sh` | **done** |
| tovarisch-041 | Fix existing files missing final newlines | **done** |
| tovarisch-042 | Update epic with ACT 6 board and acceptance | **done** |
| tovarisch-043 | Run `make gate` | **done** |

### ACT 6 Acceptance

- [x] `docs/tooling/zig-0.16-field-manual.md` contains JSON serialization section.
- [x] Section documents `beginObject()`, `objectField()`, `write()`, etc.
- [x] Example shows `renderPayload()` pattern from `status.zig`.
- [x] `scripts/quality_gate.sh` checks all tracked files for final newlines.
- [x] Gate fails if any tracked file lacks final newline.
- [x] All existing tracked files pass the final-newline check.
- [x] `make gate` passes.

## ACT 7 Scope (Local Health Checks)

Add first real local health checks to `tovarisch status --json`.

### ACT 7 Board

| ID | Work Item | Status |
|---|---|---|
| tovarisch-044 | Add `CheckStatus` enum with ok/warn/error values | **done** |
| tovarisch-045 | Add `deriveStatus()` function for top-level status derivation | **done** |
| tovarisch-046 | Implement static checks: process, binary, config | **done** |
| tovarisch-047 | Update JSON serialization to use enum values | **done** |
| tovarisch-048 | Add tests for status derivation logic | **done** |
| tovarisch-049 | Update epic docs and run `make gate` | **done** |

### ACT 7 Acceptance

- [x] `tovarisch status --json` contains multiple checks (process, binary, config).
- [x] Top-level `status` is derived from child checks, not hardcoded.
- [x] Config check shows "not configured yet" as warn.
- [x] Status derivation: any error => error, else any warn => warn, else ok.
- [x] JSON contract remains stable (verified by `verify_status_json.sh`).
- [x] All Zig tests pass.
- [x] `make gate` passes.

## Future Work

- Define signed report schema
- Define desired-state schema
- Define transport backend interface
- Add tunnel supervision (future)
- Add desired-state pull (future)
- Add signed status reports (future)
- Add tiny local diagnostics (future)
