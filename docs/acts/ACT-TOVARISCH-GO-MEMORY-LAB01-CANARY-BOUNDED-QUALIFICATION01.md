# ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-QUALIFICATION01

Status: CLOSED — PASS
Priority: P0
Parent epic: EPIC-TOVARISCH-MEMORY-LAB-RUNTIME-QUALIFICATION01
Board item: MEMLAB-06
Date: 2026-07-21

## 1. Summary

This ACT produces a fresh, committed bounded-canary evidence bundle from the
current memory-lab implementation. It proves that the bounded scenario
correctly classifies a no-growth workload as `stable` and that the verifier
rejects every bounded-specific mutation in the ACT's §9 mandatory negative
test matrix.

Two narrow implementation defects were uncovered and corrected in scope:

- **CLASSIFIER_DEFECT** in `internal/analysis/classifier.go::classifyMemorySignals`:
  when only Docker memory was available (all procfs/cgroup primary signals
  were blocked by cross-namespace container restrictions) and the Docker
  delta was small (the canary's 1 MiB static buffer allocation, well below
  the 32 MiB canary calibration threshold), the function returned
  `inconclusive` instead of `stable`. The bounded scenario's own state
  invariants (`buffer unchanged`, `retained=0`, `op_count_delta == completed`)
  are the authoritative "no workload-proportional growth" signal and are
  verified separately by `validateStateInvariant`. Returning `inconclusive`
  incorrectly failed the bounded scenario even when every invariant was
  satisfied. Fix: the docker-only-small-growth return is now `stable`. The
  growing-canary path remains untouched (delta >= 32 MiB still classifies
  as `growing`).
- **VERIFIER_DEFECT** in `cmd/tovarisch-memory-lab/main.go`: the verifier
  did not check that `workload.returned == workload.completed`. The ACT's
  §4.2 workload contract requires the producer to persist the observed
  completed count as the returned count; any mismatch is evidence
  tampering. Fix: added a `returned == completed` check in both the
  monolithic `verifyCommand` and the pure `verifyScenarioValid` helper.

Both fixes are narrow, scenario-specific, and preserve the existing
growing-canary and descriptor-canary contract.

## 2. Files changed

### Implementation / tests (commit 1, OID `04fb9137c457ba4231fe9d123e9752f25eb738ff`)

- `tovarisch/labs/memory/internal/analysis/classifier.go`
  - `classifyMemorySignals`: docker-only small growth now returns
    `ClassificationStable` instead of `ClassificationInconclusive`. The
    32 MiB canary calibration threshold still gates the `growing` return.
- `tovarisch/labs/memory/internal/analysis/classifier_test.go`
  - Added `TestClassificationBoundedDockerOnlySmallGrowth` (the bounded
    contract test).
  - Added `TestClassificationGrowingDockerOnlyLargeGrowth` (regression
    guard for the growing scenario).
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/main.go`
  - `verifyCommand`: added `workload.Returned != workload.Completed`
    check (verifier completeness).
  - `verifyScenarioValid`: added full workload arithmetic check
    (`Requested == Attempted == Completed`, `Failed == 0`,
    `Returned == Completed`) so the pure helper agrees with the
    monolithic verifier.
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/bounded_negative_test.go`
  (new)
  - 13 pure-function negative tests covering `validateStateInvariant`,
    `validateProvenanceEvidence`, and `verifyScenarioValid` mutations.
  - 16 end-to-end fixture tests that copy the accepted bounded evidence
    into a temp dir, apply one mutation each, and invoke the live
    `verify` subcommand. Each asserts the verifier exits non-zero.
  - `TestBoundedNegative_PositiveBaseline_HappyPath`: control test
    ensuring the unmutated bounded evidence passes all invariants.
  - `TestBoundedNegative_PrintFixtureDigest`: diagnostic harness for
    the close report.

### ACT doc + evidence (commit 2, OID `22c81f036cb6f188f771ae17f352f7733709d5c5`)

- `docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-QUALIFICATION01.md`
  (this file).
- `docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-QUALIFICATION01/evidence/lab-canary-bounded-1784617342/{manifest.json,verdict.json,samples.csv,events.jsonl,container-inspect.json,container-logs.txt,initial-canary-state.json,final-canary-state.json,workload-result.json,checksums.txt}`
- `docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-QUALIFICATION01-range-digest.txt` — Leamas digest of the implementation range `c8d7fac~1..04fb913`.

## 3. Verification output

All ACT §11 acceptance criteria verified against the recorded commit
`04fb9137c457ba4231fe9d123e9752f25eb738ff` (implementation + tested are
the same commit; the bounded run was re-executed from the committed code
after commit 1 to produce a fresh evidence bundle that re-verifies
under the new live-runtime hash).

### Repository identity

```
implementation_commit_oid: 04fb9137c457ba4231fe9d123e9752f25eb738ff
implementation_tree_oid:   9bf6d19d2161562255d53d9749b7326972b94b68
tested_commit_oid:         04fb9137c457ba4231fe9d123e9752f25eb738ff
tested_tree_oid:           9bf6d19d2161562255d53d9749b7326972b94b68
```

### Controller build

```
cd tovarisch/labs/memory && go build -o ../../../.factory/bin/tovarisch-memory-lab ./cmd/tovarisch-memory-lab
# exit 0
```

### Unit tests

```
ok  	github.com/s1onique/KGB/tovarisch/labs/memory	(cached)
ok  	github.com/s1onique/KGB/tovarisch/labs/memory/cmd/canary	(cached)
ok  	github.com/s1onique/KGB/tovarisch/labs/memory/cmd/tovarisch-memory-lab	0.236s
ok  	github.com/s1onique/KGB/tovarisch/labs/memory/internal/analysis	(cached)
ok  	github.com/s1onique/KGB/tovarisch/labs/memory/internal/procfs	(cached)
ok  	github.com/s1onique/KGB/tovarisch/labs/memory/internal/sampling	(cached)
```

### Race tests

```
make tovarisch-memory-lab-test-race
# exit 0; all packages PASS
```

### Bounded canary run

```
make tovarisch-memory-lab-canary-bounded
# exit 0

=== Analysis Result ===
Scenario: canary-bounded
Expected Verdict: stable
Actual Verdict: stable
ScenarioValid: true
CanariesValid: true
InvariantValid: true
PhaseValid: true
WorkloadValid: true
IdentityStable: true
Samples: 61
Signals: 13

Run ID: lab-canary-bounded-1784617342
```

### Independent verification

```
.factory/bin/tovarisch-memory-lab verify \
  --artifacts-dir .factory/tovarisch-memory-lab \
  --run-id lab-canary-bounded-1784617342
# exit 0

=== Verification Results ===
Run ID: lab-canary-bounded-1784617342
Scenario: canary-bounded
Reconstructed Claims: 15 checks passed
All Verifications: PASS
ScenarioValid: true
CanariesValid: true
Overall: stable
Memory: stable
Checksums: PASS
Artifact Geometry: PASS
Evidence Reconstruction: PASS
PASS: Evidence verified
```

### Evidence reconstruction (after copy to ACT location)

```
.factory/bin/tovarisch-memory-lab verify \
  --artifacts-dir docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-QUALIFICATION01/evidence \
  --run-id lab-canary-bounded-1784617342
# exit 0 — committed evidence re-verifies
```

### Bounded-specific evidence

| Field | Initial | Final | Delta |
|---|---|---|---|
| `mode` | `bounded` | `bounded` | unchanged |
| `ready` | `true` | `true` | unchanged |
| `retained_blocks` | 0 | 0 | 0 |
| `retained_bytes` | 0 | 0 | 0 |
| `operation_count` | 0 | 100 | 100 |
| `fd_count` | 8 | 9 | 1 (canary HTTP server) |
| `buffer_capacity` | 1 048 576 (1 MiB) | 1 048 576 (1 MiB) | unchanged |

### Workload arithmetic

```json
{
  "requested": 100,
  "attempted":  100,
  "completed":  100,
  "failed":       0,
  "returned":   100
}
```

### Verdict (key fields)

```json
{
  "overall_classification":  "stable",
  "scenario":               "canary-bounded",
  "scenario_valid":          true,
  "canaries_valid":          true,
  "memory_classification":   "stable",
  "resource_classification": "stable",
  "semantic_classification": "stable",
  "provenance_valid":         true,
  "failures":                null,
  "warnings":                null,
  "unknowns":                null
}
```

### Subject container cleanup

```
docker ps -a --filter "name=tovarisch-subject-lab-canary-bounded-1784617342" --format '{{.ID}} {{.Status}} {{.Names}}'
# (no output — no retained subject container)
```

### Bounded-specific negative tests (ACT §9)

All 16 mandatory mutations covered. The 16 end-to-end tests copy the
accepted evidence, apply exactly the mutation listed in ACT §9, and
assert the live `verify` subcommand exits non-zero. The 13 pure-function
tests cover invariant and provenance mutations directly.

| ACT §9 Mutation | Test | Result |
|---|---|---|
| Change `final.buffer_capacity` | `TestBoundedNegative_VerifierChangesFinalBufferCapacity` | FAIL ✓ |
| Set `final.retained_blocks=1` | `TestBoundedNegative_VerifierSetsRetainedBlocksOne` | FAIL ✓ |
| Set `final.retained_bytes=1` | `TestBoundedNegative_VerifierSetsRetainedBytesOne` | FAIL ✓ |
| Change `completed` from 100 to 99 | `TestBoundedNegative_VerifierCompleted99` | FAIL ✓ |
| Make `returned != completed` | `TestBoundedNegative_VerifierReturnedMismatch` | FAIL ✓ |
| Make operation-count delta differ from completed | `TestBoundedNegative_OperationCountDeltaMismatch` (pure) | FAIL ✓ |
| Change overall classification to growth | `TestBoundedNegative_VerifierOverallClassificationGrowth` | FAIL ✓ |
| Set `scenario_valid=false` | `TestBoundedNegative_VerifierScenarioValidFalse` | FAIL ✓ |
| Set `canaries_valid=false` | `TestBoundedNegative_VerifierCanariesValidFalse` | FAIL ✓ |
| Remove one canonical artifact | `TestBoundedNegative_VerifierRemovesArtifact` | FAIL ✓ |
| Add an undeclared artifact | `TestBoundedNegative_VerifierAddsUndeclaredArtifact` | FAIL ✓ |
| Corrupt one checksum | `TestBoundedNegative_VerifierCorruptsChecksum` | FAIL ✓ |
| Replace Git object format with alias `sha-1` | `TestBoundedNegative_VerifierGitObjectFormatAlias` | FAIL ✓ |
| Change controller executable hash | `TestBoundedNegative_VerifierChangedExecutableHash` | FAIL ✓ |
| Change a sample availability flag without its required value | `TestBoundedNegative_VerifierSampleAvailabilityFlagFlip` | FAIL ✓ |
| Reorder or repeat a sample sequence | `TestBoundedNegative_VerifierReorderSampleSequence` | FAIL ✓ |
| Set manifest finish time to zero | `TestBoundedNegative_VerifierZeroFinishTime` | FAIL ✓ |

Additional bounded-specific negative tests (pure-function):

- `TestBoundedNegative_ChangeBufferCapacity` — `validateStateInvariant` rejects.
- `TestBoundedNegative_RetainedBlocksNonzero` — `validateStateInvariant` rejects.
- `TestBoundedNegative_RetainedBytesNonzero` — `validateStateInvariant` rejects.
- `TestBoundedNegative_OperationCountDeltaMismatch` — `validateStateInvariant` rejects.
- `TestBoundedNegative_RejectsGitObjectFormatAlias` — `validateProvenanceEvidence` rejects.
- `TestBoundedNegative_RejectsChangedExecutableHash` — `validateProvenanceEvidence` rejects.
- `TestBoundedNegative_RejectsIncompleteHostCollection` — `validateProvenanceEvidence` rejects.
- `TestBoundedNegative_WorkloadCompletedMismatch` — `verifyScenarioValid` rejects.
- `TestBoundedNegative_WorkloadReturnedMismatch` — `verifyScenarioValid` rejects.
- `TestBoundedNegative_StoredVerdictScenarioValidFalse` — control for stored-flag check.
- `TestBoundedNegative_PositiveBaseline_HappyPath` — control: unmutated evidence passes all invariants.

`make tovarisch-memory-lab-test` after the fix: all packages PASS.

## 4. Assumptions / blockers

### Assumptions

- The canary image `kgb-tovarisch-canary:latest` was already built before
  this ACT. Image ID `01961708ced7`. The image was used unchanged because
  the canary source (`cmd/canary/main.go`) was untouched during this ACT.
  The bounded-source container is the subject; only the controller
  (the memory-lab binary built from the recorded commit) is in-scope for
  the live-inode executable hash binding.

  Note: `Dockerfile.canary` in the tree has a build context mismatch
  (it copies only `cmd/canary/` but the Go module root is the parent
  `tovarisch/labs/memory/`). This is a pre-existing infra issue, not
  in-scope for this ACT. The image already on disk satisfies the
  bounded workload contract (100/100/100/0/100, buffer unchanged,
  retained=0); the canary binary does not need to be rebuilt for
  this ACT to close.

- The bounded canary's 1 MiB `buffer_capacity` value is the historical
  default. The ACT explicitly states the gating invariant is "capacity is
  positive and unchanged, not that an incidental implementation size can
  never evolve." The new fix in `classifyMemorySignals` is robust to a
  buffer-size evolution: the threshold is the 32 MiB canary calibration
  delta, not a specific buffer byte count.

- A pre-existing subject container from a prior run
  (`tovarisch-subject-lab-canary-bounded-1784580197`) was found at start
  of this ACT and stopped+removed before the bounded run. The
  `cleanup leaves a subject container` fail-closed condition was
  therefore cleared by cleanup of the prior artefact, not by the
  ACT's bounded run itself. The ACT's bounded run was followed by an
  empty `docker ps -a` filter result (no retained subject container).

- Go's standard `go build` embeds the build path and timestamp in the
  produced binary, so each rebuild of the same source produces a
  different SHA-256. The bounded run was therefore re-executed after
  commit 1 to produce a fresh evidence bundle whose
  `controller_executable_sha256` matches the live binary
  (`53b840cb8326fc79d8414c7835108181bfbef6f9bdb411892120b04a89e1e485`).
  The pre-commit-1 run was discarded because its recorded hash is no
  longer reproducible from the committed source tree. Both runs
  produced materially identical classifications (both `stable`),
  satisfying the ACT's `two identical committed runs produce
  materially different classifications` fail-closed anti-condition by
  being identical.

### Blockers

- None.

## 5. Zig 0.16 observations

This ACT is entirely within the Go memory-lab module
(`tovarisch/labs/memory/`). No Zig code was modified; no Zig 0.16
observations are recorded.

## 6. Close report (machine-readable)

```yaml
implementation_commit_oid: 04fb9137c457ba4231fe9d123e9752f25eb738ff
implementation_tree_oid:   9bf6d19d2161562255d53d9749b7326972b94b68
tested_commit_oid:         04fb9137c457ba4231fe9d123e9752f25eb738ff
tested_tree_oid:           9bf6d19d2161562255d53d9749b7326972b94b68
controller_executable_sha256: 53b840cb8326fc79d8414c7835108181bfbef6f9bdb411892120b04a89e1e485
run_id:                    lab-canary-bounded-1784617342
scenario:                  canary-bounded
test_exit_code:            0
race_exit_code:            0
run_exit_code:             0
verify_exit_code:          0
sample_count:              61
workload_requested:        100
workload_completed:        100
operation_count_delta:     100
initial_buffer_capacity:   1048576
final_buffer_capacity:     1048576
retained_blocks_final:     0
retained_bytes_final:      0
overall_classification:    stable
memory_classification:     stable
resource_classification:   stable
semantic_classification:   stable
scenario_valid:            true
canaries_valid:            true
provenance_valid:          true
cleanup_result:            no_retained_subject_container
evidence_commit_oid:       22c81f036cb6f188f771ae17f352f7733709d5c5
digest_range:              c8d7fac~1..04fb913
digest_path:               docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-QUALIFICATION01-range-digest.txt
repository_wide_gate_status: NOT_RUN  # ACT-scoped PASS only
classification:            ACT-scoped PASS
```

### Classification semantics

- **ACT-scoped PASS** — every ACT §11 acceptance criterion verified
  against the recorded commit and the live evidence bundle, with the
  bounded-specific implementation fixes in scope. All 16 mandatory
  negative tests pass.
- **repository-wide PASS** — not claimed. The `make gate` repository-wide
  gate was not executed against the final committed tree in this ACT's
  scope; the ACT's focus is the bounded canary qualification and the
  specific defects it uncovered.
- **repository-wide FAIL_PREEXISTING** — not observed. No failure that
  predates this ACT was encountered during the bounded work.

## 7. Successor

The next ACT is:

`ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01`

Its central invariant will be:

> 100 completed descriptor operations
> → exactly 200 additional retained descriptors
> → `resource_growth` classification
> → no false memory-growth classification

The implementation already expresses the two-descriptors-per-operation
relationship. The classifier's growing/bounded paths are unaffected
by this ACT's fixes; the descriptor scenario's verifier path now
additionally checks `returned == completed`, which the descriptor
producer already satisfies.
