# ACT-TOVARISCH-ZIG-HULK26: State-Transition Register Verifier

## Status

Complete

## Intent

Make protocol transition coverage as gate-like as parser, allocation, and effect-boundary coverage.

The project already had allocation drift, parser-boundary mistakes, and effect-boundary mistakes covered by executable gates. Protocol transitions were covered by doctrine and executable tests, but they lacked a dedicated static register verifier that fails CI when the transition proof harness silently drifts out of wiring.

HULK26 adds that missing guardrail.

## Problem

FSM and protocol transition files can change while their transition proof harness remains stale, disconnected, documentation-only, or accidentally removed from split suite wiring.

The practical risk HULK26 prevents:
- A transition test file exists but is no longer wired
- A split suite drops BGP or BFD transition coverage
- A transition register accumulates `DEFERRED` entries
- A placeholder `expect(true)` creates false confidence
- `test_all.zig` imports coverage, but shard/split suites do not
- Future protocol work updates FSM behavior without keeping the proof harness alive

## Scope

HULK26 adds a static verifier that checks:
1. The state-transition register exists
2. No active DEFERRED transitions exist in the register
3. BGP totality test exists (`tovarisch/src/bgp/transition_totality_tests.zig`)
4. BFD totality test exists (`tovarisch/src/bfd/transition_totality_tests.zig`)
5. BFD FSM transition test exists (`tovarisch/src/bfd/transition_fsm_tests.zig`)
6. Transition tests are wired into canonical aggregate test entrypoint (`tovarisch/src/test_all.zig`)
7. Transition tests are wired into split suites (`tovarisch/src/test_suite_bgp.zig`, `tovarisch/src/test_suite_bfd.zig`)
8. No documentation-only `expect(true)` placeholder assertions in transition tests

## Files Changed

```
scripts/verify_state_transition_register.py   # New: state transition register verifier
tests/test_verify_state_transition_register.py # New: verifier unit tests
scripts/quality_gate.sh                        # Modified: wire HULK26 into gate
docs/acts/ACT-TOVARISCH-ZIG-HULK26-STATE-TRANSITION-REGISTER-VERIFIER.md  # New: this doc
```

## Verifier Contract

`python3 scripts/verify_state_transition_register.py` exits with non-zero code if:

| Check | Failure Message |
|-------|-----------------|
| Missing register | `[missing] docs/architecture/tovarisch-state-transition-register.md` |
| DEFERRED in register | `[deferred] docs/architecture/...:N contains DEFERRED` |
| Missing BGP totality test | `[missing] tovarisch/src/bgp/transition_totality_tests.zig` |
| Missing BFD totality test | `[missing] tovarisch/src/bfd/transition_totality_tests.zig` |
| Missing BFD FSM test | `[missing] tovarisch/src/bfd/transition_fsm_tests.zig` |
| Unwired import | `[unwired] test_all.zig does not import ...` |
| Placeholder expect(true) | `[placeholder] ...:N contains documentation-only expect(true)` |

Success output:
```
STATE TRANSITION REGISTER VERIFIER: PASS
checked_register=docs/architecture/tovarisch-state-transition-register.md
deferred_transitions=0
checked_transition_tests=3
checked_suite_files=9
```

## Local Verification Command

```bash
python3 scripts/verify_state_transition_register.py
python3 tests/test_verify_state_transition_register.py
```

## CI Proof

HULK26 is wired into `scripts/quality_gate.sh` which is run by:
- `make gate`
- GitHub Actions CI workflow

The gate runs:
1. The verifier itself
2. The verifier's self-tests

## Known Non-Goals

HULK26 does not statically prove every semantic FSM transition. It does not replace:
- BGP transition totality tests
- BFD transition totality tests
- BFD FSM behavioral tests
- Runtime protocol tests
- Future protocol-specific model checking

HULK26 only guarantees that transition doctrine and transition proof harness wiring cannot silently drift.

## Acceptance Criteria

All criteria met:

- [x] `scripts/verify_state_transition_register.py` exists
- [x] The verifier passes on the current repository state
- [x] The verifier fails when the transition register is missing
- [x] The verifier fails when the register contains active deferred transitions
- [x] The verifier fails when BGP transition totality tests are missing
- [x] The verifier fails when BFD transition totality tests are missing
- [x] The verifier fails when BFD FSM transition tests are missing
- [x] The verifier fails when transition tests are not imported by `test_all.zig`
- [x] The verifier fails when transition tests are not imported by split suites
- [x] The verifier fails on documentation-only `expect(true)` transition tests
- [x] The verifier is wired into the canonical local verification command (`make gate`)
- [x] CI runs the verifier (via `quality_gate.sh`)
- [x] ACT docs created
- [x] Verifier unit tests created and passing

## Revision History

- 2026-07-06: ACT-TOVARISCH-ZIG-HULK26 initial implementation
