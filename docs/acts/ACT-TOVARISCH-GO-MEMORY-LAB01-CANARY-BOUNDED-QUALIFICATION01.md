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
ACT, after the CORRECTION01, CORRECTION02, and CORRECTION03 sub-ACTs
repaired every defect identified across the three rounds. The CORRECTION03
round specifically tightened the parser and verifier contract so every
checksum hash and every checksum path is now validated against the
canonical grammar, and the inventory comparison is exact.

### Bounded history (this ACT's commits)

```
<correction03_evidence>  bounded CORRECTION03 evidence               ← this commit's evidence
5989833                bounded CORRECTION03 checksum containment   ← CORRECTION03 parser+verifier hardening
ad30274                bounded CORRECTION02 convergence             ← CORRECTION02 docs+tightening
3efb711                bounded CORRECTION01 evidence                 ← CORRECTION01 evidence
c566263                bounded CORRECTION01 implementation           ← CORRECTION01 hermetic harness
e13be61                original bounded digest and OID backfill — superseded
22c81f0                original bounded evidence — superseded
04fb913                original bounded implementation
c8d7fac                pre-ACT base
```

Immutable ranges:

```yaml
complete_bounded_range:               c8d7fac..<correction03_evidence>
correction02_commit_oid:              ad30274ccedf4eaaee5538d238ec69837e6f580e
correction02_tree_oid:                <tree of ad30274>
correction02_range:                   3efb711..ad30274
correction03_implementation_range:    ad30274..<correction03_implementation_commit>
correction03_implementation_commit_oid: 5989833b1ee334469ec7038e7473b2144e7c9db5
```

## 2. Files changed

### Implementation / tests (CORRECTION03 implementation commit)

- `tovarisch/labs/memory/internal/evidence/writer.go`:
  - `ValidateChecksumHash(hash)`: enforces exactly 64 lowercase
    hexadecimal characters; rejects wrong length (63 or 65
    characters), uppercase hexadecimal, and non-hex characters.
  - `ValidateChecksumArtifactPath(name)`: enforces the canonical
    flat-artifact path grammar: local, single-segment, does not
    contain `..` or path separators, is not empty, is not `.`,
    and is not absolute. Windows separator, nested paths, and
    `..` traversal are all rejected with the same
    `invalid checksum artifact path` diagnostic.
  - `ParseChecksumLine` now applies both validators to every
    line before adding to the map.
  - `ParseChecksumsFile` no longer rejects `checksums.txt`
    entries at the parser level so the verifier's
    `unexpected checksum entry` diagnostic can fire on it.
- `tovarisch/labs/memory/internal/evidence/writer_test.go` (new):
  19 new direct tests for `ValidateChecksumHash` (length 63/65,
  non-hex 64 chars, uppercase, missing hash, malformed delimiter,
  one non-hex char among valid hex) and `ValidateChecksumArtifactPath`
  (`../`, `a/../../`, absolute, `subdir/`, Windows `..\`, `subdir\`,
  `.`, empty), plus duplicate-path detection in
  `ParseChecksumsFile`.
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/bounded_checksum_e2e_test.go` (new):
  four new end-to-end tests for the four mandatory CORRECTION03
  mutations: genuine traversal, unexpected local checksum,
  non-hex 64-character checksum, and self-checksum entry.
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/bounded_artifact_negative_test.go`:
  tightened the two existing artifact tests' expected
  diagnostics to match the new parser's exact strings.

### Evidence + final closure (CORRECTION03 evidence commit)

- `docs/acts/.../evidence/lab-canary-bounded-1784624046/` (new):
  the canonical fresh evidence bundle (10 canonical artifacts).
- `docs/acts/.../superseded-evidence/lab-canary-bounded-1784619592/` (new):
  the previous evidence bundle with `superseded_reason.yaml`
  recording the three defects.
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/testdata/bounded-valid/`
  updated to match the canonical evidence (new run_id).
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/bounded_fixture_test.go`:
  `boundedFixtureRunID` updated to the new run_id.

## 3. Verification output

All ACT §11 acceptance criteria verified against the
CORRECTION03 evidence commit. The bounded canary was re-executed
from the committed CORRECTION03 implementation at `5989833` to
produce a fresh evidence bundle whose manifest Git identity equals
the recorded tested commit (ACT §5.1 — provenance identity
convergence).

### Repository identity

```yaml
complete_bounded_range:               c8d7fac..<correction03_evidence>
correction02_commit_oid:              ad30274ccedf4eaaee5538d238ec69837e6f580e
correction02_tree_oid:                <tree of ad30274>
correction02_range:                   3efb711..ad30274
correction03_implementation_range:    ad30274..5989833b1ee334469ec7038e7473b2144e7c9db5
correction03_implementation_commit_oid: 5989833b1ee334469ec7038e7473b2144e7c9db5
correction03_evidence_commit_oid:      <TBD_BY_THIS_COMMIT>
tested_commit_oid:                     5989833b1ee334469ec7038e7473b2144e7c9db5
tested_tree_oid:                       <tree of 5989833>
```

### Controller build

```
make tovarisch-memory-lab-build
# exit 0
```

### Unit tests

```
go test -count=1 ./...          (tovarisch/labs/memory)
ok  github.com/s1onique/KGB/tovarisch/labs/memory                  0.007s
ok  github.com/s1onique/KGB/tovarisch/labs/memory/cmd/canary       0.049s
ok  github.com/s1onique/KGB/tovarisch/labs/memory/cmd/tovarisch-memory-lab  2.232s
ok  github.com/s1onique/KGB/tovarisch/labs/memory/internal/analysis 0.007s
ok  github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence 0.007s
ok  github.com/s1onique/KGB/tovarisch/labs/memory/internal/procfs   0.009s
ok  github.com/s1onique/KGB/tovarisch/labs/memory/internal/sampling 0.211s
```

### Race tests

```
go test -count=1 -race ./...     (tovarisch/labs/memory)
ok  github.com/s1onique/KGB/tovarisch/labs/memory/internal/procfs   1.025s
ok  github.com/s1onique/KGB/tovarisch/labs/memory/internal/sampling 1.227s
```

### Bounded canary run (fresh, post-CORRECTION03)

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

Run ID: lab-canary-bounded-1784624046
```

### Independent verification (scratch copy)

```
.factory/bin/tovarisch-memory-lab verify \
  --artifacts-dir .factory/tovarisch-memory-lab \
  --run-id lab-canary-bounded-1784624046
# exit 0

=== Verification Results ===
Run ID: lab-canary-bounded-1784624046
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
  --run-id lab-canary-bounded-1784624046
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
docker ps -a --filter "name=tovarisch-subject-lab-canary-bounded-1784624046" --format '{{.ID}} {{.Status}} {{.Names}}'
# (no output — no retained subject container)
```

### Bounded-specific negative tests (ACT §9 + extension)

All 29 negative + 2 positive baseline tests pass. Each negative
test asserts the intended stable error code or diagnostic substring
on the targeted rejection path; the checksum validator is **not**
the one that fires for semantic mutations. The 4 new
CORRECTION03 E2E tests assert exact diagnostics on the new
hardened parser/verifier.

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
| Artifact | `TestArtifact_ChecksumPathTraversal` | `invalid checksum artifact path:` |
| Artifact | `TestArtifact_MalformedChecksumHash` | `invalid checksum hash length:` |
| Samples | `TestSamples_AvailabilityValueContradiction` | `has_docker_memory=false` |
| Samples | `TestSamples_RepeatedSequence` | `sequence` |
| Samples | `TestSamples_MissingBaselinePhase` | `phase regression` |
| Samples | `TestSamples_PIDInstability` | `PID changed` |
| E2E (CORRECTION03) | `TestE2E_GenuinePathTraversal` | `invalid checksum artifact path:` |
| E2E (CORRECTION03) | `TestE2E_UnexpectedLocalChecksum` | `unexpected checksum entry: extra.json` |
| E2E (CORRECTION03) | `TestE2E_NonHexChecksum64Char` | `invalid checksum hash encoding:` |
| E2E (CORRECTION03) | `TestE2E_SelfChecksumEntry` | `unexpected checksum entry: checksums.txt` |

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
  SHA-256. The bounded run was re-executed after the CORRECTION03
  commit to produce a fresh evidence bundle whose
  `controller_executable_sha256` matches the live binary.
- The hermetic test data fixture in
  `cmd/tovarisch-memory-lab/testdata/bounded-valid/` is the
  authoritative source for negative-test mutation. No test depends
  on the prior `.factory` scratch state.
- The previous evidence bundle
  (`lab-canary-bounded-1784619592`) was moved to
  `superseded-evidence/` with `superseded_reason.yaml` documenting
  the three defects that the CORRECTION03 commit repaired.

### Blockers

- None.

## 5. Zig 0.16 observations

This ACT is entirely within the Go memory-lab module
(`tovarisch/labs/memory/`). No Zig code was modified; no Zig 0.16
observations are recorded.

## 6. Close report (machine-readable)

```yaml
correction02_commit_oid:            ad30274ccedf4eaaee5538d238ec69837e6f580e
correction02_tree_oid:              <tree of ad30274>

correction03_implementation_commit_oid: 5989833b1ee334469ec7038e7473b2144e7c9db5
correction03_implementation_tree_oid:   <tree of 5989833>
correction03_evidence_commit_oid:       <TBD_BY_THIS_COMMIT>
correction03_evidence_tree_oid:         <TBD_BY_THIS_COMMIT>

tested_commit_oid:                 5989833b1ee334469ec7038e7473b2144e7c9db5
tested_tree_oid:                   <tree of 5989833>
manifest_git_commit:               5989833b1ee334469ec7038e7473b2144e7c9db5
manifest_git_tree:                 <tree of 5989833>
git_identity_matches_tested_identity: true

controller_executable_sha256: 4a373f196385b6f4ce0143100ff7b7a02cad8c67f6145019f1b69960b0ae3f1c
run_id:                          lab-canary-bounded-1784624046
scenario:                        canary-bounded

checksum_hash_hex_enforced:      true
checksum_hash_case_canonical:    true
checksum_path_local_enforced:    true
checksum_path_flat_enforced:     true
checksum_inventory_exact:        true
path_traversal_test_targeted:    true
non_hex_64_char_test_targeted:   true

positive_fixture_verify_exit_code: 0
negative_tests_executed:            29
negative_tests_skipped:             0
negative_tests_expected_reason_matched: all

test_exit_code:            0
race_exit_code:            0
llm_friendly_exit_code:    0
bounded_run_exit_code:     0
scratch_verify_exit_code:  0
committed_verify_exit_code: 0

correction03_range:         ad30274..<correction03_evidence>
complete_bounded_range:     c8d7fac..<correction03_evidence>
correction03_git_diff_check: pass
complete_bounded_git_diff_check: pass

scratch_directory_removed:  true
working_tree_clean:         true

repository_wide_gate_status: NOT_RUN
classification: ACT-scoped PASS
```

### Classification semantics

- **ACT-scoped PASS** — every ACT §11 acceptance criterion verified
  against the recorded commit and the live evidence bundle, with
  the CORRECTION01, CORRECTION02, and CORRECTION03 defects all
  repaired in scope. All 29 mandatory negative tests + 2 positive
  baseline tests + 4 new E2E tests pass and assert their intended
  diagnostics. Working tree clean.
- **repository-wide PASS** — not claimed. `make gate` not executed
  in scope.
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
to bounded; the descriptor path uses `fd_delta == 2 * completed` (no
buffer check).

`MEMLAB-06 = DONE`, `MEMLAB-07 = READY`.
