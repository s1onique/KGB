# Scripts Inventory

Canonical reference for gate scripts and tooling in KGB.

## Quality Gates

| Script | Purpose | Entry Point |
|--------|---------|-------------|
| `scripts/quality_gate.sh` | Master quality gate — runs all checks | `make gate` |
| `scripts/check_llm_friendliness.sh` | LLM-friendliness checks (file limits, no broken refs) | auto-invoked |
| `scripts/coverage_gate.sh` | Real line coverage via kcov | auto-invoked |

## Cross-Platform Compile Gate

| Target | Purpose | Entry Point |
|--------|---------|-------------|
| `tovarisch-compile-linux` | Cross-compile tovarisch for Linux target (x86_64-linux-gnu) | `make tovarisch-compile-linux` |
| `cross-platform-gate` | Compile-only gate to catch platform-specific API drift | `make cross-platform-gate` |

**Rationale:** Zig does not semantically analyze inactive platform branches on non-target hosts. Linux-only code in `@import("builtin").os.tag == .linux` branches can pass macOS local gate and still fail Linux CI. The cross-platform gate catches this drift before merge.

## Regression Tests

| Script | Purpose | Entry Point |
|--------|---------|-------------|
| `scripts/check_final_newlines_regression.sh` | Verifies gate catches untracked files without final newlines | `make test-final-newlines-regression` |

## Verification Scripts

| Script | Purpose | Entry Point |
|--------|---------|-------------|
| `scripts/verify_tovarisch_status_contract.sh` | Validates status JSON contract | `make verify-status-contract` |
| `scripts/verify_status_json.sh` | JSON schema validation for status output | auto-invoked |
| `scripts/verify_structured_logs.sh` | Verifies no prose runtime logs; uses structured logging | `make verify-structured-logs` |

## Coverage

| Script | Purpose | Entry Point |
|--------|---------|-------------|
| `scripts/coverage_report.sh` | Generate human-readable coverage report | `make coverage-report` |
| `scripts/extract_kcov_line_coverage.py` | Parse kcov output formats | auto-invoked |
| `scripts/kcov_parsers.py` | Format-specific kcov parsers | auto-invoked |

## Utilities

| Script | Purpose |
|--------|---------|
| `scripts/make_targeted_digest.sh` | Generate targeted digest for dirty repos |

## Adding New Scripts

When adding a new gate or verification script:

1. Add to this inventory with purpose and entry point
2. Wire into `scripts/quality_gate.sh` if it should run automatically
3. Add to `Makefile` as a `.PHONY` target if it needs a discoverable name
4. Document any new patterns in the relevant doctrine docs
