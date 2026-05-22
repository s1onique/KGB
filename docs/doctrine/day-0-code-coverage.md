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

## Day-0 Targets for `tovarisch`

The early meaningful coverage targets are not broad percentages yet. They are specific behaviors:

| Area                     | Day-0 expectation                                       |
| ------------------------ | ------------------------------------------------------- |
| CLI command parsing      | Covered by tests or command verification                |
| `--version`              | Verified                                                |
| `check`                  | Verified                                                |
| `status --json`          | Verified and JSON-parseable                             |
| health/status model      | Unit-testable as soon as logic appears                  |
| config loading           | Must become covered before real config complexity lands |
| tunnel/probe supervision | Must become coverage-critical later                     |

## Implementation Rules

1. **No fake coverage.** Do not invent percentages or fabricate reports.
2. **Coverage tooling is a future step.** When Zig coverage backend matures, we wire it in.
3. **Until then, test execution IS the signal.** `zig build test` passing is the current coverage proxy.
4. **TODOs must be explicit.** If a behavior is uncovered, add a TODO comment with a reason.

## Gate Integration

- `make coverage` exposes current coverage status.
- `make gate` runs coverage checks (currently via test execution).
- Missing coverage backend is explicitly announced, not hidden.
- Gate fails on test failure, not on coverage percentage.

## References

- See `scripts/quality_gate.sh` for current coverage integration.
- See `Makefile` for `coverage` target.
- See `docs/epics/bootstrap-zig-tovarisch-leaf-service.md` for ACT tracking.