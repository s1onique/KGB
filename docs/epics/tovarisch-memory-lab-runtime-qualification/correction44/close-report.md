# CORRECTION44 Close Report

## Summary

Production control authority converged onto the canonical
`dockerlab.ControlRunner`. The legacy duplicate protocol
implementation and the permanent `v2` naming are deleted from
production. The qualified lifecycle now records four independent
canonical reachability operations (health, initial state, operate,
final state) populated from validated envelopes, and the qualified
evidence verifier independently recomputes reachability from the
raw fields instead of trusting the supplied claim.

## Status

```yaml
CORRECTION43: SUPERSEDED_BY_CORRECTION44
CORRECTION44: CLOSED_PRODUCTION_CONTROL_CONVERGENCE_HERMETIC
parent_correction03: PARTIAL
MEMLAB_08A: DONE
MEMLAB_08B: IN_PROGRESS
MEMLAB_08C: BLOCKED
next: CORRECTION45
```

## Production path exercised

- The production CLI run path binds the real Docker exec client
  via the typed `dockerlab.NewDockerControl` constructor and runs
  the four canonical operations through the canonical runner.
- The qualified lifecycle now carries an explicit
  `QualifiedLifecycleDependencies` record; the lifecycle fails
  before any Docker mutation when the control dependency is nil.
- `internal/evidence/qualified_execution.go` now exposes
  `deriveReachability` as a pure function. The complete verifier
  compares `ev.Reachability.Success` against the derived value
  and rejects every disagreement.

## Files changed

See `changed-files.txt` and `subject-identities.txt`. The canonical
production file rename is `control_protocol_v2.go → control_protocol.go`.

## Verification

`focused-tests.txt`, `race-tests.txt`, `count100-tests.txt`,
`reachability-mutation-tests.txt`, `legacy-regression-tests.txt`,
`focused-vet.txt`, `build.txt`, and `make-gate.txt` are committed
in this evidence directory. All targeted packages return `ok`; the
`make gate` failure is exclusively the pre-existing
`hulk-uvb76-artifact-producer-gate` and is unchanged by S44.

## Doctrine / ADR impact

None. The four-operation reachability observations are
encapsulated inside the existing `dockerlab.QualifiedExecutionObservations`
and `evidence.QualifiedExecutionEvidence` schemas. The legacy
duplicate protocol authority is removed; the shared
`canarycontrol` package remains the sole schema, decoder,
semantic-validator, and retry-policy authority.

## Cold resume / next exact step

CORRECTION45 rebuilds the canary image from S45, builds the
VCS-stamped helper and production binaries, executes the live
health/state/operate/state path on the exact canary, binds source
+ image + executable identities, persists independently verified
reachability evidence, proves pull=0, proves container + network
cleanup, and runs the canonical gate to close MEMLAB_08B.
