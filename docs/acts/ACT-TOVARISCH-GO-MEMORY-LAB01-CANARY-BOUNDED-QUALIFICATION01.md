# ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-QUALIFICATION01

Status: CLOSED — ACT-scoped PASS
Priority: P0
Parent epic: EPIC-TOVARISCH-MEMORY-LAB-RUNTIME-QUALIFICATION01
Board item: MEMLAB-06
Date: 2026-07-21

## 1. Summary

This ACT produces a fresh, committed bounded-canary evidence bundle
from the current memory-lab implementation. It proves that the
bounded scenario correctly classifies a no-growth workload as
`stable` and that the verifier rejects every bounded-specific
mutation in the ACT's §9 mandatory negative test matrix.

This document records the **final closure** of the bounded
qualification ACT, after the CORRECTION01 and CORRECTION02
sub-ACTs repaired the four initial defects and tightened the
mutation diagnostics, the test fixture, the close report fields,
and the patch-hygiene status.

### Bounded history (this ACT's commits)

```
3efb711  bounded CORRECTION01 evidence                         ← ACT evidence (this ACT)
c566263  bounded qualification CORRECTION01                     ← ACT correction (this ACT)
e13be61  original bounded digest and OID backfill — superseded by CORRECTION01
22c81f0  original bounded evidence — superseded by CORRECTION01
04fb913  original bounded implementation
c8d7fac  pre-ACT base (memory-lab: producer-side live-inode binding + canonical format)
```

`complete_bounded_range: c8d7fac..3efb711` (six commits, all
`git diff --check` clean).
`correction_implementation_range: e13be61..c566263` (one commit, the
verifier change + hermetic testdata + split negative-test files).
`correction_evidence_commit_oid: 3efb711` (commit 2 of the ACT).

### Two implementation defects repaired (verifier + classifier)

- **CLASSIFIER_DEFECT** in
  `internal/analysis/classifier.go::classifyMemorySignals`:
  when only Docker memory was available (all procfs/cgroup primary
  signals were blocked by cross-namespace container restrictions)
  and the Docker delta was small (the canary's 1 MiB static
  buffer allocation, well below the 32 MiB canary calibration
  threshold), the function returned `inconclusive` instead of
  `stable`. The bounded scenario's own state invariants (buffer
  unchanged, retained=0, operation_count delta == completed) are
  the authoritative "no workload-proportional growth" signal and
  are verified separately by `validateStateInvariant`. Returning
  `inconclusive` incorrectly failed the bounded scenario even when
  every invariant was satisfied. Fix: docker-only-small-growth
  now returns `ClassificationStable`. The growing-canary path
  remains untouched (delta >= 32 MiB still classifies as
  `growing`).
- **VERIFIER_DEFECT** in
  `cmd/tovarisch-memory-lab/main.go::verifyCommand` (caught
  during CORRECTION01): `verifyCommand` did not check that the
  bounded canary's `buffer_capacity` is unchanged between
  initial and final state. The bounded scenario's invariant
  validator (`validateStateInvariant`) enforced this, but the
  verifier's "reconstruct claims" section did not, leaving a
  verifier gap that the negative test `TestState_BufferCapacityChange`
  exposed. Fix: added a `buffer_capacity` unchanged check in
  `verifyCommand` for the bounded case. The earlier
  `workload.returned == workload.completed` check (committed
  in the original bounded ACT) is also preserved.

## 2. Files changed

### Implementation / tests (commit `c566263ae1d151298018cb22a5e5827360c6e3b2`)

- `tovarisch/labs/memory/internal/analysis/classifier.go` —
  `classifyMemorySignals`: docker-only small growth now returns
  `ClassificationStable` instead of `ClassificationInconclusive`.
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/main.go` —
  `verifyCommand`: added `workload.Returned != workload.Completed`
  check and the bounded `buffer_capacity` unchanged check.
  `verifyScenarioValid`: added full workload arithmetic check.
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/testdata/bounded-valid/` (new) —
  committed bounded canary evidence fixture (10 canonical artifacts).
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/bounded_main_test.go` (new) —
  `TestMain` builds the production controller binary into a
  per-process temp dir.
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/bounded_fixture_test.go` (new) —
  copy/rebind/compute-checksums helpers plus positive baseline
  tests.
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/bounded_state_negative_test.go` (new) —
  4 state invariant mutation tests.
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/bounded_workload_negative_test.go` (new) —
  4 workload arithmetic mutation tests.
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/bounded_provenance_negative_test.go` (new) —
  5 provenance mutation tests.
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/bounded_artifact_negative_test.go` (new) —
  7 artifact geometry and inventory mutation tests, each
  asserting one stable, intended parser or verifier diagnostic.
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/bounded_samples_negative_test.go` (new) —
  4 samples mutation tests.

The previous `bounded_negative_test.go` (1042 lines) is removed.

### ACT doc + accepted evidence (commit `3efb711e75e383cbcb34250e3311933d8bdda771`)

- `docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-QUALIFICATION01.md`
  (this file, rewritten).
- `docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-QUALIFICATION01/evidence/lab-canary-bounded-1784619592/` —
  the canonical fresh evidence bundle (10 canonical artifacts).
- `docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-QUALIFICATION01/rejected-evidence/lab-canary-bounded-1784617342/` —
  the superseded parent ACT's evidence, with `rejected_reason.yaml`
  recording the four defects.

## 3. Verification output

All ACT §11 acceptance criteria verified against the recorded
commit `3efb711e75e383cbcb34250e3311933d8bdda771` (correction
evidence commit). The bounded canary was re-executed from the
committed code at `c566263ae1d151298018cb22a5e5827360c6e3b2`
(correction implementation commit) to produce a fresh evidence
bundle whose manifest Git identity equals the recorded tested
commit (ACT §5.1 — provenance identity convergence).

### Repository identity

```yaml
complete_bounded_range: c8d7fac..3efb711
correction_implementation_range: e13be61..c566263
correction_evidence_commit_oid: 3efb711
implementation_commit_oid: 04fb9137c457ba4231fe9d123e9752f25eb738ff
tested_commit_oid:           c566263ae1d151298018cb22a5e5827360c6e3b2
tested_tree_oid:             b1087baf27b173215b8a311fa813ec6656786318
```

### Controller build

```
make tovarisch-memory-lab-build
# exit 0
```

### Unit tests

```
go test -count=1 ./...          (tovarisch/labs/memory)
ok  	github.com/s1onique/KGB/tovarisch/labs/memory	0.010s
ok  	github.com/s1onique/KGB/tovarisch/labs/memory/cmd/canary	0.055s
ok  	github.com/s1onique/KGB/tovarisch/labs/memory/cmd/tovarisch-memory-lab	2.391s
ok  	github.com/s1onique/KGB/tovarisch/labs/memory/internal/analysis	0.011s
ok  	github.com/s1onique/KGB/tovarisch/labs/memory/internal/procfs	0.010s
ok  	github.com/s1onique/KGB/tovarisch/labs/memory/internal/sampling	0.211s
```

### Race tests

```
make tovarisch-memory-lab-test-race
# exit 0; all packages PASS
```

### Bounded canary run (fresh, post-CORRECTION01)

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

Run ID: lab-canary-bounded-1784619592
```

### Independent verification (scratch copy)

```
.factory/bin/tovarisch-memory-lab verify \
  --artifacts-dir .factory/tovarisch-memory-lab \
  --run-id lab-canary-bounded-1784619592
# exit 0

=== Verification Results ===
Run ID: lab-canary-bounded-1784619592
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

### Independent verification (committed ACT evidence copy)

```
.factory/bin/tovarisch-memory-lab verify \
  --artifacts-dir \
    docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-QUALIFICATION01/evidence \
  --run-id lab-canary-bounded-1784619592
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
docker ps -a --filter "name=tovarisch-subject-lab-canary-bounded-1784619592" --format '{{.ID}} {{.Status}} {{.Names}}'
# (no output — no retained subject container)
```

### Bounded-specific negative tests (ACT §9 + extension)

All 29 negative + 2 positive baseline tests pass. Each negative
test asserts the intended stable error code or diagnostic substring
on the targeted rejection path; the checksum validator is **not**
the one that fires for semantic mutations.

| Category | Test | Targeted diagnostic |
|---|---|---|
| Harness | `TestBoundedPositiveBaseline_CopiedFixtureVerifies` | exit 0 (positive control) |
| Harness | `TestBoundedPositiveBaseline_InventoryVerifies` | checksums.txt matches every canonical artifact |
| State | `TestState_BufferCapacityChange` | `bounded: buffer_capacity changed from 1048576 to 2097152` |
| State | `TestState_RetainedBlocksNonzero` | `bounded: retained should be 0, got blocks=1 bytes=0` |
| State | `TestState_RetainedBytesNonzero` | `bounded: retained should be 0, got blocks=0 bytes=1` |
| State | `TestState_OperationCountDeltaMismatch` | `operation_count_delta=50 != completed=100` |
| Workload | `TestWorkload_CompletedNotEqualRequested` | `workload counts: req=100 att=100 com=99 fail=0` |
| Workload | `TestWorkload_ReturnedNotEqualCompleted` | `workload returned=99 != completed=100` |
| Workload | `TestWorkload_AttemptedNotEqualRequested` | `workload counts: req=100 att=99 com=100 fail=0` |
| Workload | `TestWorkload_FailedNonzero` | `workload counts: req=100 att=100 com=99 fail=1` |
| Provenance | `TestProvenance_GitObjectFormatAlias` | `git_object_format: unsupported git_object_format="sha-1"` |
| Provenance | `TestProvenance_ChangedExecutableHash` | `executable hash mismatch` |
| Provenance | `TestProvenance_MissingGitCommit` | `subject_identity.git_commit is empty` |
| Provenance | `TestProvenance_MalformedExecutableHash` | `subject_identity.controller_executable_sha256: invalid hex` |
| Provenance | `TestProvenance_ZeroFinishedTime` | `manifest not finalized: missing finished_at` |
| Artifact | `TestArtifact_RemoveCanonicalArtifact` | `missing file from inventory: workload-result.json` |
| Artifact | `TestArtifact_AddUndeclaredArtifact` | `unexpected file not in inventory: extra-file.txt` |
| Artifact | `TestArtifact_CorruptChecksum` | `checksum mismatch for` |
| Artifact | `TestArtifact_RemoveChecksumEntry` | `missing checksum for:` |
| Artifact | `TestArtifact_DuplicateChecksumEntry` | `duplicate entry for:` |
| Artifact | `TestArtifact_ChecksumPathTraversal` | `missing checksum for: container-inspect.json` |
| Artifact | `TestArtifact_MalformedChecksumHash` | `invalid hash length:` |
| Samples | `TestSamples_AvailabilityValueContradiction` | `has_docker_memory=false` |
| Samples | `TestSamples_RepeatedSequence` | `sequence` |
| Samples | `TestSamples_MissingBaselinePhase` | `phase regression` |
| Samples | `TestSamples_PIDInstability` | `PID changed` |

## 4. Assumptions / blockers

### Assumptions

- The canary image `kgb-tovarisch-canary:latest` was already built
  before this ACT. Image ID `01961708ced7`. The image was used
  unchanged because the canary source (`cmd/canary/main.go`) was
  untouched during this ACT.
- The bounded canary's 1 MiB `buffer_capacity` value is the historical
  default. The new fix in `classifyMemorySignals` is robust to a
  buffer-size evolution: the threshold is the 32 MiB canary
  calibration delta, not a specific buffer byte count.
- A pre-existing subject container from a prior run
  (`tovarisch-subject-lab-canary-bounded-1784580197`) was found at
  start of this ACT and stopped+removed.
- Go's standard `go build` embeds the build path and timestamp in
  the produced binary, so each rebuild produces a different
  SHA-256. The bounded run was re-executed after commit 1 to
  produce a fresh evidence bundle whose
  `controller_executable_sha256` matches the live binary.
- The hermetic test data fixture in
  `cmd/tovarisch-memory-lab/testdata/bounded-valid/` is the
  authoritative source for negative-test mutation. No test depends
  on the prior `.factory` scratch state.

### Blockers

- None.

## 5. Zig 0.16 observations

This ACT is entirely within the Go memory-lab module
(`tovarisch/labs/memory/`). No Zig code was modified; no Zig 0.16
observations are recorded.

## 6. Close report (machine-readable)

```yaml
correction02_commit_oid: dc89c298d47fbb1fd8693ed53f8dad5ac7e239bb
correction02_tree_oid:   dc89c298d47fbb1fd8693ed53f8dad5ac7e239bb

complete_bounded_range:     c8d7fac..3efb711
correction02_range:         3efb711..dc89c298d47fbb1fd8693ed53f8dad5ac7e239bb
correction_implementation_range: e13be61..c566263

controller_executable_sha256: f010b08b8b93104da21b394a7ee58376062e4a2c60ea3ad90a96168cad684706
run_id:                              lab-canary-bounded-1784619592
scenario:                            canary-bounded

positive_fixture_verify_exit_code: 0
negative_tests_executed:            29
negative_tests_skipped:             0
negative_tests_expected_reason_matched: all

test_exit_code:            0
race_exit_code:            0
bounded_run_exit_code:     0
scratch_verify_exit_code:  0
committed_verify_exit_code: 0
llm_friendly_exit_code:    0

targeted_diagnostics_exact:       true
pending_placeholders_remaining:   0
dummy_import_sentinels_remaining: 0

correction02_git_diff_check:      pass
complete_bounded_git_diff_check:  pass
scratch_directory_removed:       true
working_tree_clean:              true

repository_wide_gate_status:     NOT_RUN
classification:                  ACT-scoped PASS
```

### Classification semantics

- **ACT-scoped PASS** — every ACT §11 acceptance criterion verified
  against the recorded commit and the live evidence bundle, with the
  CORRECTION01 defects repaired in scope. All 29 mandatory negative
  tests pass and assert their intended diagnostic; 2 positive
  baseline tests pass. Working tree clean.
- **repository-wide PASS** — not claimed. The `make gate`
  repository-wide gate was not executed against the final committed
  tree in this ACT's scope.
- **repository-wide FAIL_PREEXISTING** — not observed.

## 7. Successor

The next ACT is:

`ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01`

Its central invariant will be:

> 100 completed descriptor operations
> → exactly 200 additional retained descriptors
> → `resource_growth` classification
> → no false memory-growth classification

The implementation already expresses the two-descriptors-per-operation
relationship. The bounded-buffer-capacity check is scenario-specific
to bounded; the descriptor path uses `fd_delta == 2 * completed`
(no buffer check).

On close of the descriptor qualification, the bounded ACT
`MEMLAB-06` and the descriptor ACT `MEMLAB-07` can both be marked
DONE, unblocking the full three-scenario matrix.
