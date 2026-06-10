# Day-0 Code Coverage Doctrine

## Principle

Coverage is not "80% or shame." Coverage is an **accountability surface**:

> Every important behavior must either be covered by an automated check or be consciously accepted as uncovered risk.

Coverage is a signal, not a vanity metric. We count from the first heartbeat.

## Philosophy

- **Coverage is tracked from the first useful commit.** We do not defer coverage to "after architecture stabilizes."
- **Uncovered critical paths are more important than raw percentage.** A 60% suite covering the right things beats 90% covering noise.
- **Temporary coverage gaps must be explicit TODOs, not invisible debt.** If a path cannot be covered yet, document why.
- **Local gates must expose coverage status whenever tooling supports it.** Absence of tooling is not absence of intent.

## Dual Coverage Model

`tovarisch` uses two complementary coverage mechanisms:

### 1. Real Line Coverage (kcov)

`kcov` instruments the test binary and measures actual line coverage:

- **Backend**: `kcov` — industry-standard coverage tool for compiled binaries
- **Threshold**: 60% line coverage (configurable via `COVERAGE_THRESHOLD`)
- **Gate**: Fails if coverage is below threshold, unless `ALLOW_MISSING_KCOV=1`
- **Files covered**: `tovarisch/src/` only (no cache, vendor, or generated files)
- **Build requirement**: Test binary must be built with debug info (`-Doptimize=Debug`)

#### Platform Policy

- **Linux CI is authoritative** for real kcov coverage. CI must NOT set `ALLOW_MISSING_KCOV=1`.
- **macOS ARM64**: kcov may install via Homebrew but may report 0% coverage due to kernel-level tracing limitations. This is not considered valid coverage data.
- **Local macOS developers**: Use `ALLOW_MISSING_KCOV=1 make coverage` as a local bypass only. The gate is bypassed, not the threshold.
- **Zero-line coverage is invalid**: If kcov reports 0 executable lines, the parser fails. This is intentional — no fake 0% coverage.
- **DWARF completeness check**: When kcov is available but DWARF is incomplete (missing source paths), the coverage gate uses test-as-signal as honest fallback. This is not a bypass — it is an honest signal when instrumentation is broken.

#### Honest Signal Policy

When kcov reports coverage but DWARF diagnostics show incomplete debug info:

1. **kcov numbers are marked untrustworthy** — coverage cannot be mapped to source lines
2. **Test-as-signal becomes the honest fallback** — `make tovarisch-test` passing proves code exercises covered behaviors
3. **The gate does NOT silently use the kcov number** — it uses test-as-signal when DWARF is broken

This policy applies to macOS Zig 0.16 where DWARF line tables may be incomplete. See [macOS Zig 0.16/kcov DWARF Limitation](../coverage/macos-zig-kcov-dwarf.md).

```bash
# Run coverage gate (Linux CI)
make coverage

# Local macOS bypass (dev only, not for CI)
ALLOW_MISSING_KCOV=1 make coverage

# Custom threshold
COVERAGE_THRESHOLD=70 make coverage
```

### 2. Behavior Coverage Ledger

The ledger tracks which behaviors are covered by automated checks:

- **Location**: `docs/coverage/tovarisch-coverage.md`
- **Purpose**: Documents test coverage per command/behavior
- **Gate**: Must exist and mention all public commands

See `docs/coverage/tovarisch-coverage.md` for the full ledger.

## Day-0 Targets for `tovarisch`

The early meaningful coverage targets are not broad percentages yet. They are specific behaviors:

| Area                     | Day-0 expectation                                       |
| ------------------------ | ------------------------------------------------------- |
| CLI command parsing      | Covered by tests or command verification                |
| `--version`              | Verified                                                |
| `check`                  | Verified                                                |
| `status --json`          | Verified, JSON-parseable, and contract-validated        |
| health/status model      | Unit-testable as soon as logic appears                  |
| config loading           | Must become covered before real config complexity lands |
| tunnel/probe supervision | Must become coverage-critical later                     |

### `status --json` Contract Coverage

`tovarisch status --json` is covered by **structural JSON validation**, not just grep.

The verification script (`scripts/verify_status_json.sh`) validates:

1. **Valid JSON** — parses without error
2. **Required fields** — `service`, `version`, `node_id`, `status`, `checks`
3. **Field types** — each field has the correct type (string or array)
4. **Semantic constraints** — `service` must be `"tovarisch"`, `status` must be one of `ok|warn|error`
5. **Check objects** — each entry has `name`, `status`, `detail` fields with correct types

This replaces brittle grep-only checks with structural validation. The gate fails if:
- JSON is malformed
- Required fields disappear
- Types mismatch

## Implementation Rules

1. **No fake coverage.** Do not invent percentages or fabricate reports.
2. **kcov is required for real coverage.** Gate fails if kcov is missing unless `ALLOW_MISSING_KCOV=1`.
3. **Threshold is enforced.** Coverage below `COVERAGE_THRESHOLD` fails the gate.
4. **TODOs must be explicit.** If a behavior is uncovered, add a TODO comment with a reason.
5. **JSON contracts use structural validation.** Prefer `jq`-based validation over grep where available.
6. **No cache/generated files in coverage.** Only `src/` paths count.

## Gate Integration

- `make coverage` runs real kcov line coverage with threshold enforcement
- `make coverage-report` prints human-readable coverage summary
- `make gate` includes real coverage gate (not just test passing)
- `scripts/coverage_gate.sh` is the coverage gate implementation
- `scripts/extract_kcov_line_coverage.py` parses kcov's output
- `scripts/verify_status_json.sh` validates `status --json` structurally
- Missing coverage backend fails the gate (not warning-only)

## References

- See `scripts/coverage_gate.sh` for real coverage gate implementation
- See `scripts/extract_kcov_line_coverage.py` for kcov parser
- See `scripts/coverage_report.sh` for human-readable report
- See `scripts/quality_gate.sh` for coverage integration
- See `scripts/verify_status_json.sh` for JSON contract validation
- See `Makefile` for `coverage` and `coverage-report` targets
- See `docs/coverage/tovarisch-coverage.md` for behavior coverage ledger
- See `docs/epics/bootstrap-zig-tovarisch-leaf-service.md` for ACT tracking
