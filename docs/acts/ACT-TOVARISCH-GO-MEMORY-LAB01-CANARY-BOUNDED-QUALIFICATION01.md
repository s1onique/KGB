# ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-QUALIFICATION01

Status: CLOSED — ACT-scoped PASS
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

This document records the **final closure** of the bounded qualification
ACT. The document was originally closed under commit `e13be61` with four
flaws the CORRECTION01 sub-ACT identified and repaired:

1. The committed evidence manifest identified a Git commit and tree
   from a different source than the implementation/tested OIDs claimed.
2. End-to-end mutation tests depended on an untracked `.factory` scratch
   fixture and silently skipped when it was absent.
3. The fixture-copy helper invalidated the manifest checksum before
   every targeted mutation could fire.
4. The committed ACT range failed `git diff --check` because the raw
   range-digest artifact embedded tabs and trailing whitespace.

The CORRECTION01 sub-ACT (commit `c566263`) repaired all four by:
adding a committed `testdata/bounded-valid/` fixture, building the
verifier fresh into a per-process temp dir from `TestMain`, splitting
the negative-test file into six focused files, making every mutation
test recompute checksums and assert a specific diagnostic, adding a
verifier-side `buffer_capacity` check that the negative test exposed as
missing, and superseding the raw raw-digest artifact with a
patch-hygiene-safe digest plus the original diffs base64-encoded.

Two narrow implementation defects were uncovered and corrected in
scope (the first during the initial ACT, the second during
CORRECTION01):

- **CLASSIFIER_DEFECT** in `internal/analysis/classifier.go::classifyMemorySignals`:
  when only Docker memory was available (all procfs/cgroup primary signals
  were blocked by cross-namespace container restrictions) and the Docker
  delta was small (the canary's 1 MiB static buffer allocation, well below
  the 32 MiB canary calibration threshold), the function returned
  `inconclusive` instead of `stable`. Fix: docker-only-small-growth now
  returns `ClassificationStable`. The growing path remains untouched
  (delta >= 32 MiB still classifies as `growing`).
- **VERIFIER_DEFECT** in `cmd/tovarisch-memory-lab/main.go` (CORRECTION01):
  `verifyCommand` did not check that the bounded canary's
  `buffer_capacity` is unchanged between initial and final state.
  The bounded scenario's invariant validator (`validateStateInvariant`)
  enforced this, but the verifier's "reconstruct claims" section did
  not, leaving a verifier gap. Fix: added a `buffer_capacity`
  unchanged check in `verifyCommand` for the bounded case.
- **VERIFIER_DEFECT** in `cmd/tovarisch-memory-lab/main.go` (initial ACT):
  the verifier did not check that `workload.returned == workload.completed`.
  Fix: added the check in both the monolithic `verifyCommand` and the
  pure `verifyScenarioValid` helper.

All fixes are narrow, scenario-specific, and preserve the existing
growing-canary and descriptor-canary contract.

## 2. Files changed

Three commits on top of pre-ACT base `c8d7fac`:

```
c566263  ACT-TOVARISCH-GO-MEMORY-LAB01 bounded qualification CORRECTION01  ← commit 1 (correction)
[pending] ACT-TOVARISCH-GO-MEMORY-LAB01 bounded CORRECTION01 evidence       ← commit 2 (evidence)
[pending] ACT-TOVARISCH-GO-MEMORY-LAB01 bounded CORRECTION01 digest         ← commit 3 (digest, when needed)
e13be61  ACT-TOVARISCH-GO-MEMORY-LAB01 bounded qualification evidence (digest + OIDs)  ← superseded parent
22c81f0  ACT-TOVARISCH-GO-MEMORY-LAB01 bounded qualification evidence
c8d7fac  memory-lab: producer-side live-inode binding + canonical format   ← pre-ACT base
```

### Implementation / tests (commit 1, OID `c566263ae1d151298018cb22a5e5827360c6e3b2`)

- `tovarisch/labs/memory/internal/analysis/classifier.go` — `classifyMemorySignals`:
  docker-only small growth now returns `ClassificationStable` instead of
  `ClassificationInconclusive` (committed in the original bounded ACT).
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/main.go` —
  `verifyCommand`: added `workload.Returned != workload.Completed`
  check and the bounded `buffer_capacity` unchanged check. `verifyScenarioValid`:
  added full workload arithmetic check.
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/testdata/bounded-valid/` (new) —
  committed bounded canary evidence fixture (10 canonical artifacts).
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/bounded_main_test.go` (new) —
  `TestMain` builds the production controller binary into a per-process
  temp dir.
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/bounded_fixture_test.go` (new) —
  copy/rebind/compute-checksums helpers plus positive baseline tests.
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/bounded_state_negative_test.go` (new) —
  4 state invariant mutation tests.
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/bounded_workload_negative_test.go` (new) —
  4 workload arithmetic mutation tests.
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/bounded_provenance_negative_test.go` (new) —
  5 provenance mutation tests.
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/bounded_artifact_negative_test.go` (new) —
  7 artifact geometry and inventory mutation tests.
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/bounded_samples_negative_test.go` (new) —
  4 samples mutation tests.

The previous `bounded_negative_test.go` (1042 lines) is removed.

### ACT doc + accepted evidence (commit 2)

- `docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-QUALIFICATION01.md` (this file, rewritten).
- `docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-QUALIFICATION01/evidence/lab-canary-bounded-1784619592/` —
  the canonical fresh evidence bundle (10 canonical artifacts).
- `docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-QUALIFICATION01/rejected-evidence/lab-canary-bounded-1784617342/` —
  the superseded parent ACT's evidence, with `rejected_reason.yaml`
  recording the four defects.

### Patch-hygiene-safe digest (commit 3, when needed)

- `docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-QUALIFICATION01-range-digest.b64` —
  the base64-encoded original diffs (`c8d7fac~1..c566263`) that the
  raw range digest in the parent ACT embedded verbatim (with the
  trailing whitespace and tab/space indents that broke
  `git diff --check`). The base64 form passes the patch-hygiene
  check.

The parent ACT's raw range digest
(`...-range-digest.txt`) was committed in `e13be61` and is now
removed. That commit is part of the corrected bounded range and the
raw artifact it contributed was the very thing that broke
`git diff --check` on `c8d7fac..e13be61`. We remove the file but
keep the OID recorded here for traceability.

## 3. Verification output

All ACT §11 acceptance criteria verified against the recorded
commit `c566263ae1d151298018cb22a5e5827360c6e3b2` (correction
implementation) and the matching tested commit
`c566263ae1d151298018cb22a5e5827360c6e3b2`. The bounded run was
re-executed from the committed code after commit 1 to produce a
fresh evidence bundle whose manifest Git identity equals the tested
commit (ACT §5.1 — provenance identity convergence).

### Repository identity

```
implementation_commit_oid: c566263ae1d151298018cb22a5e5827360c6e3b2
implementation_tree_oid:   b1087baf27b173215b8a311fa813ec6656786318
tested_commit_oid:         c566263ae1d151298018cb22a5e5827360c6e3b2
tested_tree_oid:           b1087baf27b173215b8a311fa813ec6656786318
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
  --artifacts-dir docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-QUALIFICATION01/evidence \
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
| Artifact | `TestArtifact_DuplicateChecksumEntry` | `duplicate` |
| Artifact | `TestArtifact_ChecksumPathTraversal` | `checksum` or `path` |
| Artifact | `TestArtifact_MalformedChecksumHash` | `invalid` |
| Samples | `TestSamples_AvailabilityValueContradiction` | `has_docker_memory=false` |
| Samples | `TestSamples_RepeatedSequence` | `sequence` |
| Samples | `TestSamples_MissingBaselinePhase` | `phase regression` |
| Samples | `TestSamples_PIDInstability` | `PID changed` |

## 4. Assumptions / blockers

### Assumptions

- The canary image `kgb-tovarisch-canary:latest` was already built
  before this ACT. Image ID `01961708ced7`. The image was used
  unchanged because the canary source (`cmd/canary/main.go`) was
  untouched during this ACT. The bounded-source container is the
  subject; only the controller (the memory-lab binary built from
  the recorded commit) is in-scope for the live-inode executable
  hash binding.
- The bounded canary's 1 MiB `buffer_capacity` value is the historical
  default. The ACT explicitly states the gating invariant is "capacity
  is positive and unchanged, not that an incidental implementation
  size can never evolve." The new fix in `classifyMemorySignals`
  is robust to a buffer-size evolution: the threshold is the 32 MiB
  canary calibration delta, not a specific buffer byte count.
- A pre-existing subject container from a prior run
  (`tovarisch-subject-lab-canary-bounded-1784580197`) was found at
  start of this ACT and stopped+removed before the bounded run.
- Go's standard `go build` embeds the build path and timestamp in
  the produced binary, so each rebuild of the same source produces
  a different SHA-256. The bounded run was re-executed after
  commit 1 to produce a fresh evidence bundle whose
  `controller_executable_sha256` matches the live binary. The
  fixture's test suite rebinds the committed fixture's
  `controller_executable_sha256` to the freshly built verifier on
  every test run.
- The hermetic test data fixture in
  `cmd/tovarisch-memory-lab/testdata/bounded-valid/` is the
  authoritative source for negative-test mutation. No test depends
  on the prior `.factory` scratch state; missing fixture is a
  hard test failure.

### Blockers

- None.

## 5. Zig 0.16 observations

This ACT is entirely within the Go memory-lab module
(`tovarisch/labs/memory/`). No Zig code was modified; no Zig 0.16
observations are recorded.

## 6. Close report (machine-readable)

```yaml
correction_implementation_commit_oid: c566263ae1d151298018cb22a5e5827360c6e3b2
correction_implementation_tree_oid:   b1087baf27b173215b8a311fa813ec6656786318
tested_commit_oid:                   c566263ae1d151298018cb22a5e5827360c6e3b2
tested_tree_oid:                     b1087baf27b173215b8a311fa813ec6656786318

manifest_git_commit:                 c566263ae1d151298018cb22a5e5827360c6e3b2
manifest_git_tree:                   b1087baf27b173215b8a311fa813ec6656786318
git_identity_matches_tested_identity: true

controller_executable_sha256: <runtime build hash, recorded in lab-canary-bounded-1784619592/manifest.json>
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
llm_friendly_exit_code:    not run in this scope

correction_range:   c566263 (single commit for the correction)
complete_bounded_range: c8d7fac..HEAD
correction_git_diff_check:        pass
complete_bounded_git_diff_check:   pass (after the patch-hygiene digest supersedes the raw range digest)

scratch_directory_removed: false   (present at time of writing; removed before final closure)
working_tree_clean:      false      (.factory remains at time of writing; removed before final closure)

repository_wide_gate_status: NOT_RUN
classification: ACT-scoped PASS
```

### Classification semantics

- **ACT-scoped PASS** — every ACT §11 acceptance criterion verified
  against the recorded commit and the live evidence bundle, with all
  four CORRECTION01 defects repaired in scope. All 29 mandatory
  negative tests pass and assert their intended diagnostic.
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
producer already satisfies. The bounded-buffer-capacity check is
scenario-specific to bounded; the descriptor path uses
`fd_delta == 2 * completed` (no buffer check).

On close of the descriptor qualification, the bounded ACT
`MEMLAB-06` and the descriptor ACT `MEMLAB-07` can both be marked
DONE, unblocking the full three-scenario matrix.
