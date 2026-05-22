# Tovarisch Coverage Ledger

> **Purpose**: This ledger tracks which `tovarisch` behaviors are covered by automated checks and which remain accepted uncovered risk.

Coverage is an accountability surface. Every important behavior must either be covered or consciously accepted as uncovered.

## Dual Coverage System

### 1. Real Line Coverage (kcov)

`tovarisch` uses `kcov` to measure actual line coverage of the test binary:

- **Threshold**: 60% (configurable via `COVERAGE_THRESHOLD`)
- **Files covered**: `tovarisch/src/` only (no cache/vendor paths)
- **Gate**: Fails if below threshold, unless `ALLOW_MISSING_KCOV=1`
- **Enforcement**: Required in `make gate`

See `scripts/coverage_gate.sh` for implementation.

### 2. Behavior Coverage Ledger

This document tracks which specific behaviors are covered by automated checks.

## Behavior Coverage Matrix

### Covered Behaviors

| Behavior | Coverage Mechanism | Gate-Enforced? | Status | Gap / Follow-up |
|----------|-------------------|----------------|--------|-----------------|
| CLI usage / invalid args | Unit tests in `cli.zig` | Yes (kcov + gate) | ✅ Covered | None |
| `--help` / `-h` command | Unit tests: returns ok, prints usage | Yes (kcov + gate) | ✅ Covered | None |
| `--version` command | Unit test + gate verification | Yes (kcov + gate) | ✅ Covered | None |
| `check` command | Unit test + gate verification | Yes (kcov + gate) | ✅ Covered | None |
| `status --json` JSON validity | `verify_status_json.sh` structural validation | Yes (gate) | ✅ Covered | None |
| JSON structural contract | Unit tests + verification script | Yes (gate) | ✅ Covered | None |
| Required JSON fields/types | Verification script checks fields and types | Yes (gate) | ✅ Covered | None |
| `CheckStatus` enum rendering | Unit tests for `deriveStatus()` | Yes (kcov + gate) | ✅ Covered | None |
| Top-level status derivation | Unit tests verify error > warn > ok | Yes (kcov + gate) | ✅ Covered | None |
| Local checks: process | Static check in `status.zig` + output test | Yes (kcov + gate) | ✅ Covered | None |
| Local checks: binary | Static check in `status.zig` + output test | Yes (kcov + gate) | ✅ Covered | None |
| Local checks: config | Static check shows "not configured yet" as warn | Yes (kcov + gate) | ✅ Covered | None |
| Local checks: state_dir (placeholder) | Emits warn with "state directory not found" | Yes (kcov + gate) | ✅ Covered | Temporary until real Io.Dir API used |
| Multiple local checks in output | Unit test `status --json contains multiple checks` | Yes (kcov + gate) | ✅ Covered | None |

### Accepted Uncovered Future Behaviors

| Behavior | Reason Uncovered | Follow-up |
|----------|-----------------|-----------|
| Real config loading | Config system not implemented; static config check used as placeholder | Implement config loading and add coverage |
| Dynamic node identity | Hardcoded to "local-dev"; identity schema TBD | Define node identity scheme and add coverage |
| Probe execution | No probes implemented yet; static checks only | Implement probe execution and add coverage |
| Tunnel supervision | Tunnel backend not designed yet | Design tunnel interface and add coverage |
| Signed status reports | Report schema TBD; no signing implementation | Define report schema and add coverage |
| Desired-state pull | Desired-state model not designed | Design desired-state interface and add coverage |
| Transport to station | Station-side not implemented | Implement station transport and add coverage |
| state_dir (directory exists) | Io.Dir API not yet understood in Zig 0.16; placeholder returns warn | Investigate std.fs.Dir.stat() or simpler API |
| state_dir (path is file, not dir) | Io.Dir API not yet understood; placeholder only | Implement real filesystem check |
| state_dir (permission denied) | Io.Dir API not yet understood; placeholder only | Implement real filesystem check |

## Coverage Mechanisms

### Real Line Coverage (kcov)

`kcov` instruments the test binary and measures actual line coverage:

- **Command**: `make coverage`
- **Threshold**: 60% (configurable via `COVERAGE_THRESHOLD`)
- **Files covered**: `tovarisch/src/` only
- **Parser**: `scripts/extract_kcov_line_coverage.py`
- **Gate**: Fails if coverage below threshold, unless `ALLOW_MISSING_KCOV=1`

### Unit Tests (`zig build test`)

The Zig test suite in `tovarisch/src/` covers:

- **cli.zig**: `--help`, `-h`, `--version`, `check`, unknown command, missing args, `status` validation
- **status.zig**: `deriveStatus()` derivation logic, JSON parsing/serialization round-trip, required fields

Run: `cd tovarisch && zig build test`

### Structural JSON Validation (`verify_status_json.sh`)

Validates `status --json` output:
1. Valid JSON (parses without error)
2. Required fields: `service`, `version`, `node_id`, `status`, `checks`
3. Field types: strings and arrays
4. Semantic constraints: `service` must be `"tovarisch"`, `status` one of `ok|warn|error`
5. Check objects: each has `name`, `status`, `detail` fields

### Gate Integration (`make gate`)

- Runs `kcov` coverage gate when kcov is available
- Runs `verify_status_json.sh` on `status --json` output
- Fails if coverage below threshold or JSON contract is violated
- Behavior coverage ledger must exist and mention all public commands

## Commands Tracked

All public `tovarisch` commands must appear in this ledger:

| Command | Public? | Covered |
|---------|---------|---------|
| `--help`, `-h` | Yes | ✅ Unit test + gate |
| `--version` | Yes | ✅ Unit test + gate |
| `check` | Yes | ✅ Unit test + gate |
| `status --json` | Yes | ✅ Unit test + structural validation |

## Updating This Ledger

When adding new behavior:

1. Add row to the Behavior Coverage Matrix
2. Specify coverage mechanism (unit test, integration test, script, gate check)
3. Mark gate-enforced status
4. If uncovered, document reason and follow-up

When implementing previously-uncovered behavior:

1. Add explicit test coverage
2. Update this ledger to mark as "Covered"
3. Remove from "Accepted Uncovered" section

## Philosophy

- **No fake coverage**: We do not invent percentages. We track specific behaviors.
- **Uncovered ≠ ignored**: If a behavior is important but not yet covered, it appears here as accepted uncovered risk.
- **Coverage signals intent**: Test passing is the current coverage proxy until Zig coverage backend matures.
- **TODOs are visible**: Gaps are documented, not hidden.

## References

- [Day-0 Code Coverage Doctrine](../doctrine/day-0-code-coverage.md)
- [Quality Gate Script](../scripts/quality_gate.sh)
- [JSON Verification Script](../scripts/verify_status_json.sh)
