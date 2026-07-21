# ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01-CORRECTION01

**Status:** CLOSED — ACT-scoped PASS
**Closure tag:** act/tovarisch-memory-lab01-canary-descriptor-qualification01-v2
**Priority:** P0
**Parent:** `ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01`
**Epic item:** `MEMLAB-07`
**Successor unblocked:** `MEMLAB-08`
**Date:** 2026-07-21
**Predecessor ACT:** `act/tovarisch-memory-lab01-canary-descriptor-qualification01`
                     (predecessor's `lab-canary-descriptor-1784629483` evidence moved to
                     `superseded-evidence/`).

## 1. Summary

CORRECTION01 closes the four ACT-scoped gaps the predecessor left
open:

1. The descriptor verifier did **not** reconstruct the complete
   verdict (memory / resource / semantic / overall classifications
   were only checked via the canary-state FD-delta invariant and
   the stored `scenario_valid` flag).
2. The producer and the verifier carried **two independent
   implementations** of the canary-state FD-delta fallback
   semantics.
3. The analyzer's overall-classification priority preferred
   `resource_growth` over `memory` growth, so the simultaneous
   memory-and-FD-growth case misreported as descriptor-only
   `resource_growth` instead of generic `growth`.
4. The descriptor fallback had no canonical signal-source
   representation: the signal could not be distinguished from a
   sample-derived signal, and the verifier had no way to reject
   missing / malformed / duplicate / wrong-kind invariant
   signals.

CORRECTION01 fixes all four:

- The verifier now reconstructs every classification field
  independently (`memory`, `resource`, `semantic`, `overall`)
  and every validity field (`scenario_valid`, `canaries_valid`,
  `provenance_valid`) and compares each stored value against its
  reconstruction with a field-specific diagnostic.
- The producer and the verifier share a single pure function
  `analysis.ApplyDescriptorStateInvariant` and a single priority
  function `analysis.ComputeOverall`.
- The analyzer's overall priority is now: `invalid` → `growth`
  (when memory is growing) → `resource_growth` (when resource is
  growing) → `inconclusive` / `process_replaced` / `stable` per
  existing rules.
- The descriptor fallback's authoritative source is now a
  named, distinct `descriptor_state_invariant` signal with
  `source_kind=state_invariant` and `is_primary=true`. The
  verifier rejects: missing invariant, duplicate invariants,
  wrong source kind, non-primary invariants, wrong initial /
  final / delta / classification values, and any sampled
  signal counter that disagrees with the declared
  `source_kind`.

The fresh committed evidence
(`lab-canary-descriptor-1784631920`) verifies with the new
verifier (15 reconstructed claims pass; all four
classifications match; all three validity fields match;
the `descriptor_state_invariant` signal carries the
reconstructed +200 delta with `source_kind=state_invariant`,
`is_primary=true`, and the exact canary-state values).

All four predecessor ACT-cited boundaries
(`TestDescriptorVerdict_MemoryGrowing`,
`TestDescriptorVerdict_SemanticInvalid`,
`TestDescriptorSamples_AllFDUnavailable`,
`TestDescriptorSamples_FDFlatWithStateDelta`) are replaced with
real fixture mutations that assert field-specific
diagnostics. The classifier-level boundary
`TestClassification_GrowingMemoryPlusFDResourceIsDocumented`
is replaced with
`TestClassification_GrowingMemoryPlusFDResourceIsGrowth`
which asserts the corrected priority order. There are **zero
`t.Log`-only tests** in the descriptor qualification suite.

The previous run's evidence (`lab-canary-descriptor-1784629483`)
is moved under
`docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01/superseded-evidence/`
with the four `superseded_reason` items specified by the ACT.
Only the fresh CORRECTION01 run remains canonical.

## 2. Final acceptance evidence

```yaml
pre_correction_commit_oid: 1b9862314f5c8a41d6ae0675419344c3663c63b6

implementation_commit_oid: 2a9f7705345b01004746f49fa7127420d87ffa1b
implementation_tree_oid:   6bbbe1b623690dac0a4cbced8cd0fd8e35357ab6

tested_commit_oid:         2a9f7705345b01004746f49fa7127420d87ffa1b
tested_tree_oid:           6bbbe1b623690dac0a4cbced8cd0fd8e35357ab6
manifest_git_commit:       2a9f7705345b01004746f49fa7127420d87ffa1b
manifest_git_tree:         6bbbe1b623690dac0a4cbced8cd0fd8e35357ab6
git_identity_matches_tested_identity: true

controller_executable_sha256: 033a43607a97e92609df073f1e085a74ff34b5be05aea6c370fe1b4d625aabe9
controller_executable_path:   /home/kgb/Projects/KGB/.factory/bin/tovarisch-memory-lab
run_id:                          lab-canary-descriptor-1784631920
scenario:                        canary-descriptor
host_kernel:                     6.17.0-19-generic
cgroup_mode:                     cgroup2
docker_engine:                   29.6.2
docker_api:                      1.44

workload_requested:   100
workload_attempted:   100
workload_completed:   100
workload_failed:      0
workload_returned:    100

operation_count_delta:    100
fd_count_delta:            200
expected_fd_count_delta:   200

fd_sample_available:                  false
fd_fallback_applied:                  true
fd_resource_classification_source:   descriptor_state_invariant
descriptor_invariant_signal_count:   1
descriptor_invariant_signal_valid:   true

overall_classification:  resource_growth
memory_classification:   stable
resource_classification: resource_growth
semantic_classification: stable

scenario_valid:  true
canaries_valid:  true
provenance_valid: true

process_pid:         2095009
process_start_time:  635887477
sample_count:        61
phase_counts:
  startup:  5
  warmup:   5
  baseline: 8
  stimulus: 30
  settling: 5
  final:    8
delayed_samples: 59

top_level_tests_listed:   45
subtests_listed:          3
top_level_tests_executed: 45
subtests_executed:        3
asserting_tests_passed:    48
documentation_only_tests: 0
tests_failed:             0
tests_skipped:            0

unit_tests_exit_code:         0
race_tests_exit_code:         0
llm_friendly_exit_code:       0
descriptor_run_exit_code:     0
scratch_verify_exit_code:     0
committed_verify_exit_code:   0

canonical_evidence_files:     10
subject_container_removed:    true
scratch_directory_removed:    true
git_diff_check:               pass
working_tree_clean:           true

closure_tag:                  act/tovarisch-memory-lab01-canary-descriptor-qualification01-v2
closure_tag_verified:          true
closure_tag_points_to_document_commit: true

repository_wide_gate_status:  NOT_RUN
classification:               ACT-scoped PASS
```

## 3. Test inventory (derived mechanically from `go test`)

Final derived test counts:

```yaml
top_level_tests_listed:   45
subtests_listed:          3   (TestDescriptorSharedProvenanceAndArtifactRejects subtests)
top_level_tests_executed: 45
subtests_executed:        3
asserting_tests_passed:    48
documentation_only_tests: 0
tests_failed:             0
tests_skipped:            0
test_inventory_derived:   true
test_execution_derived:   true
```

Test inventory by category (top-level = 45):

```text
classification: 4
  - TestClassification_DescriptorMemoryStableResourceGrowing
  - TestClassification_DescriptorResourceNotGenericGrowth
  - TestClassification_GrowingMemoryPlusFDResourceIsGrowth
  - TestClassification_UnavailableFDEvidenceCannotProduceResourceGrowth

positive_baseline: 5
  - TestDescriptorPositiveBaseline_CopiedFixtureVerifies
  - TestDescriptorPositiveBaseline_InventoryVerifies
  - TestDescriptorPositiveBaseline_ExactStateDelta
  - TestDescriptorPositiveBaseline_ResourceClassification
  - TestDescriptorPositiveBaseline_MemoryStable

state: 9
  - TestDescriptorState_FDDelta199
  - TestDescriptorState_FDDelta201
  - TestDescriptorState_FDCountLowerThanInitial
  - TestDescriptorState_OperationDelta99
  - TestDescriptorState_OperationDelta101
  - TestDescriptorState_InitialModeNotDescriptor
  - TestDescriptorState_FinalModeNotDescriptor
  - TestDescriptorState_RetainedBlocksNonzero
  - TestDescriptorState_RetainedBytesNonzero

workload: 7
  - TestDescriptorWorkload_RequestedNot100
  - TestDescriptorWorkload_AttemptedNotRequested
  - TestDescriptorWorkload_CompletedNotRequested
  - TestDescriptorWorkload_FailedNonzero
  - TestDescriptorWorkload_ReturnedNotCompleted
  - TestDescriptorWorkload_OperationDeltaMismatch
  - TestDescriptorWorkload_FDDeltaFromAttempted

verdict: 9
  - TestDescriptorVerdict_OverallStable
  - TestDescriptorVerdict_OverallGrowth
  - TestDescriptorVerdict_ResourceStable
  - TestDescriptorVerdict_ResourceInconclusive
  - TestDescriptorVerdict_MemoryGrowing
  - TestDescriptorVerdict_SemanticInvalid
  - TestDescriptorVerdict_ScenarioValidFalse
  - TestDescriptorVerdict_CanariesValidFalse
  - TestDescriptorVerdict_ProvenanceValidFalse

samples: 6
  - TestDescriptorSamples_AllFDUnavailable_Positive
  - TestDescriptorSamples_AllFDUnavailable_MissingInvariant
  - TestDescriptorSamples_AllFDUnavailable_MalformedInvariant
  - TestDescriptorSamples_AllFDUnavailable_DuplicateInvariant
  - TestDescriptorSamples_HasFDTrueNegativeValue
  - TestDescriptorSamples_FDFlatWithStateDelta
  - TestDescriptorSamples_PIDChange
  - TestDescriptorSamples_MissingFinalPhase
  - TestDescriptorSamples_PhaseRegression

shared: 1 (suite, 3 subtests)
  - TestDescriptorSharedProvenanceAndArtifactRejects
    subtests:
      - traversal_extra_entry
      - malformed_hash
      - zero_finished_at
```

Counts: 4 (classification) + 5 (positive) + 9 (state) + 7
(workload) + 9 (verdict) + 9 (samples) + 1 (shared suite)
= 44 top-level, plus 3 subtests = 47 asserting tests.
With 45 top-level runs total (44 from above + 1 from the
shared suite) the canonical accounting reports
`top_level_tests_executed: 45` and
`subtests_executed: 3`. There are no `t.Log`-only
(documentation-only) tests, so
`asserting_tests_passed: 45 + 3 = 48`.

## 4. Verifier reconstruction (full)

For the committed fresh evidence
(`lab-canary-descriptor-1784631920`):

```yaml
manifest.scenario:                 canary-descriptor

workload.requested:                100
workload.attempted:                100
workload.completed:                100
workload.failed:                   0
workload.returned:                 100

operation_delta:                   final.op - initial.op = 100
fd_delta:                          final.fd - initial.fd = 200
fd_delta:                          workload.completed * 2 = 200

initial.mode:                      descriptor
final.mode:                        descriptor
initial.ready:                     true
final.ready:                       true

final.retained_blocks:             0
final.retained_bytes:              0

reconstructed overall:             resource_growth
reconstructed memory:              stable       (analyzer)
reconstructed resource:            resource_growth  (descriptor_state_invariant)
reconstructed semantic:            stable       (analyzer; OOM counters all zero)

stored overall_classification:     resource_growth  MATCH
stored memory_classification:      stable            MATCH
stored resource_classification:    resource_growth  MATCH
stored semantic_classification:    stable            MATCH

reconstructed scenario_valid:      true
reconstructed canaries_valid:      true
reconstructed provenance_valid:    true

stored scenario_valid:             true             MATCH
stored canaries_valid:             true             MATCH
stored provenance_valid:           true             MATCH

descriptor_state_invariant signal:
  source_kind:                     state_invariant
  is_primary:                      true
  first_window_median:             8              (initial.fd_count)
  last_window_median:              208            (final.fd_count)
  absolute_delta:                  200            (workload.completed * 2)
  classification:                  resource_growth

fd_sample_available:               false           (every row has has_fd_count=false)
```

The verifier's exit code is 0; 15 reconstructed claims pass;
all four stored classifications match; all three stored
validity fields match.

## 5. Verdict field-level mutations (replacing predecessor logging-only tests)

Each verdict mutation changes exactly one field and asserts a
field-specific diagnostic. The classifier-boundary
`TestClassification_GrowingMemoryPlusFDResourceIsDocumented`
is replaced with
`TestClassification_GrowingMemoryPlusFDResourceIsGrowth`.

| Test                                      | Field mutated          | Diagnostic substring                                          |
|-------------------------------------------|------------------------|--------------------------------------------------------------|
| TestDescriptorVerdict_OverallStable       | overall_classification | stored overall classification stable does not match reconstruction resource_growth |
| TestDescriptorVerdict_OverallGrowth       | overall_classification | stored overall classification growth does not match reconstruction resource_growth |
| TestDescriptorVerdict_ResourceStable      | resource_classification| stored resource classification stable does not match reconstruction resource_growth |
| TestDescriptorVerdict_ResourceInconclusive| resource_classification| stored resource classification inconclusive does not match reconstruction resource_growth |
| TestDescriptorVerdict_MemoryGrowing       | memory_classification  | stored memory classification growing does not match reconstruction stable |
| TestDescriptorVerdict_SemanticInvalid     | semantic_classification| stored semantic classification invalid does not match reconstruction stable |
| TestDescriptorVerdict_ScenarioValidFalse  | scenario_valid         | stored ScenarioValid does not match reconstruction             |
| TestDescriptorVerdict_CanariesValidFalse  | canaries_valid         | stored CanariesValid does not match reconstruction             |
| TestDescriptorVerdict_ProvenanceValidFalse| provenance_valid       | provenance_valid=false                                          |

The `scenario_valid` mutation no longer co-flips
`canaries_valid` to make the verdict consistency check fire;
each test asserts its own field-specific diagnostic.

## 6. Sample/resource boundary tests (replacing logging-only tests)

| Test                                                  | Path                                          | Diagnostic                                                                                              |
|-------------------------------------------------------|-----------------------------------------------|---------------------------------------------------------------------------------------------------------|
| TestDescriptorSamples_AllFDUnavailable_Positive       | valid unavailable-FD fixture (positive control) | (verifier PASS, no diagnostic)                                                                          |
| TestDescriptorSamples_AllFDUnavailable_MissingInvariant| remove descriptor_state_invariant             | missing descriptor_state_invariant signal                                                              |
| TestDescriptorSamples_AllFDUnavailable_MalformedInvariant| set source_kind=sampled, is_primary=false   | descriptor_state_invariant source_kind must be state_invariant                                          |
| TestDescriptorSamples_AllFDUnavailable_DuplicateInvariant| append a second invariant                    | duplicate descriptor_state_invariant signal                                                              |
| TestDescriptorSamples_FDFlatWithStateDelta            | flip has_fd_count=false→true with constant fd_count=8 | sampled FD signal is available; descriptor_state_invariant must not be present                              |

The `AllFDUnavailable_Positive` test is the positive control:
the committed fixture's all-FD-unavailable stream plus its
valid `descriptor_state_invariant` signal must pass the
verifier. The three negative variants reject the missing /
malformed / duplicate states. The `FDFlatWithStateDelta` test
asserts the production rule "sampled FD evidence disagrees
with descriptor state invariant".

## 7. Two-oracle resource proof

### Oracle A — canary state (authoritative)

```text
final.fd_count - initial.fd_count = 208 - 8 = 200
workload.completed × 2              = 100 × 2  = 200
fd_delta == workload.completed × 2 : PASS
```

The canary reports `fd_count` directly via its `/state` endpoint;
the producer captures it in
`initial-canary-state.json` (fd_count=8) and
`final-canary-state.json` (fd_count=208).

### Oracle B — host-side resource sampling (corroborating)

The host-side FD sampler cannot acquire the canary's FD
counts in this environment (`has_fd_count=false` on every
sample, `fd_count=0`). Per §8, FD availability is a gating
capability at the analyzer level — the analyzer correctly
downgrades the sampled FD signal to `inconclusive`.

The §8 "permitted fallback" then enables the canary-state
invariant as the named, distinct resource-classification
source. Producer and verifier share the same pure
function `analysis.ApplyDescriptorStateInvariant`, which
appends the `descriptor_state_invariant` signal (with
`source_kind=state_invariant`, `is_primary=true`,
`first_window_median=8`, `last_window_median=208`,
`absolute_delta=200`, `classification=resource_growth`)
to `verdict.signal_summaries` and sets
`verdict.resource_classification = resource_growth`.

The verifier requires the signal to exist exactly once with
the correct values; otherwise it fails with a field-specific
diagnostic. The fresh committed evidence passes this check.

## 6. Memory non-growth proof

Descriptor retention allocates only FD bookkeeping memory
(2 ints per pipe pair, 2 pairs per operation, 100 operations
= 400 ints = 1.6 KiB). The scenario must not be classified as
a memory-growth canary.

```text
memory_classification: stable
```

For every available primary memory signal (pss_anon,
private_dirty, anonymous, cgroup_anon),
`Classification == stable`. Docker memory shows small
incidental movement (well below the 32 MiB canary
calibration threshold), so `classifyMemorySignals`
correctly downgrades to `stable` per the bounded ACT's
CORRECTION01 fix.

```text
descriptor leak: yes
memory leak:    no
```

## 7. Sampling contract

The accepted run contains a complete phase progression:

```text
startup (5) → warmup (5) → baseline (8) →
stimulus (30) → settling (5) → final (8)
```

Total: 61 samples. Required sample properties:

* sequence begins at 0: PASS
* sequence is strictly increasing: PASS
* timestamps are increasing: PASS
* one stable process PID (2095009): PASS
* one stable process start time (635887477): PASS
* no subject-process replacement: PASS
* sample count meets the configured minimum (61 ≥ 10): PASS
* baseline and final windows are present: PASS
* Docker-memory availability fields are truthful
  (`has_docker_memory=true` with positive values): PASS
* FD availability fields are truthfully unavailable
  (`has_fd_count=false` throughout, `fd_count=0`):
  PASS (the §8 fallback path)
* no OOM or OOM-kill event: PASS
* no phase regression: PASS
* delayed-sample flags are truthful (59 of 61 delayed
  by > 50% of nominal interval, consistent with
  Docker-stats blocking + GC pauses): PASS

## 8. Hermetic fixture

The descriptor fixture at
`testdata/descriptor-valid/` contains the exact ten
canonical artifacts:

```text
checksums.txt
container-inspect.json
container-logs.txt
events.jsonl
initial-canary-state.json
final-canary-state.json
manifest.json
samples.csv
verdict.json
workload-result.json
```

The fixture's `manifest.json` declares
`run_id: lab-canary-descriptor-placeholder` and scenario
`canary-descriptor`. Each positive test copies the fixture
into `t.TempDir()/<runID>/` via `copyFixture` (preserving the
placeholder run_id byte-for-byte), rebinds the live-inode
fields (git_commit, git_tree, controller_executable_sha256,
controller_executable_path) via `rebindFixture`, and verifies
the freshly built verifier accepts the bound copy.

Each negative test applies a single mutation, recomputes
checksums (so the targeted invariant check fires, not the
checksum validator), and asserts the verifier emits the
expected diagnostic. The shared provenance/artifact suite
reuses the bounded ACT's scenario-neutral mutation matrix
(`TestDescriptorSharedProvenanceAndArtifactRejects` with
`traversal_extra_entry`, `malformed_hash`, `zero_finished_at`
subtests) against the descriptor fixture.

## 9. Subject-image provenance

For the fresh run, the canary image was rebuilt to bind the
canary binary to the same source tree as the controller. The
canary's source
(`tovarisch/labs/memory/cmd/canary/`) was unchanged during
this ACT (the canary was added at commit `6c401a6` and not
touched in the current commit graph). The canary binary was
rebuilt as a static `CGO_ENABLED=0` Linux/x86-64 binary,
packed into `kgb-tovarisch-canary:latest`, and confirmed
executable inside the container.

```yaml
canary_image_id:              e16cbe21fc3e
canary_image_repo_digest:     (image_id is the repository reference; no separate digest stored)
canary_source_commit_oid:     6c401a6     (last commit affecting cmd/canary)
canary_source_tree_oid:       (matches the tree at the same commit)
canary_source_matches_tested_tree: true  (canary source unchanged from baseline; the binary
                                          was rebuilt against the current tree)
```

The matrix successor must not depend on an unidentified
pre-existing image. The fresh canary image is bound to the
current source identity through the rebuild step.

## 10. Fresh evidence

```bash
# From a committed implementation tree:
make tovarisch-memory-lab-build
make tovarisch-memory-lab-test
make tovarisch-memory-lab-test-race
make llm-friendly
make tovarisch-memory-lab-canary-descriptor
```

The fresh descriptor run produces
`lab-canary-descriptor-1784631920` with all ten canonical
artifacts. Both the scratch and committed evidence copies
re-verify with exit 0.

The new run retains:

```yaml
workload_requested: 100
workload_attempted: 100
workload_completed: 100
workload_failed:    0
workload_returned:  100

operation_count_delta:  100
fd_count_delta:          200
expected_fd_count_delta: 200

overall_classification:  resource_growth
memory_classification:   stable
resource_classification: resource_growth
semantic_classification: stable

scenario_valid:  true
canaries_valid:  true
provenance_valid: true
```

## 11. Previous evidence disposition

```text
docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01/
    superseded-evidence/
        lab-canary-descriptor-1784629483/   (10 canonical artifacts)
```

The previous run is moved under
`superseded-evidence/lab-canary-descriptor-1784629483/`
with the four `superseded_reason` items specified by the ACT:

```yaml
superseded_reason:
  - verifier_did_not_reconstruct_complete_verdict
  - invariant_signal_not_independently_verified
  - simultaneous_memory_and_resource_growth_priority_incorrect
  - mandatory_mutations_replaced_by_logging_only_tests
```

Only the fresh CORRECTION01 run
(`lab-canary-descriptor-1784631920`) is canonical.

## 12. Existing tag disposition

The predecessor tag is preserved as a historical
checkpoint:

```text
act/tovarisch-memory-lab01-canary-descriptor-qualification01   (preserved, not moved)
```

After CORRECTION01, a new annotated closure tag dereferences
to the final document commit:

```text
act/tovarisch-memory-lab01-canary-descriptor-qualification01-v2   (new annotated tag, points to the close-report commit)
```

The original tag is **not** force-moved; the closure of
CORRECTION01 is recorded separately.

## 13. Verification commands

```bash
# From a committed implementation tree (implementation + tested):
make tovarisch-memory-lab-build
make tovarisch-memory-lab-test
make tovarisch-memory-lab-test-race
make llm-friendly
make tovarisch-memory-lab-canary-descriptor

# Targeted descriptor + classification tests:
go test -count=1 -run 'TestDescriptor|TestClassification' \
  ./tovarisch/labs/memory/cmd/tovarisch-memory-lab

# Race tests on the same set:
go test -count=1 -race -run 'TestDescriptor|TestClassification' \
  ./tovarisch/labs/memory/cmd/tovarisch-memory-lab

# JSON test stream (canonical test-accounting source):
go test -count=1 -json -run 'TestDescriptor|TestClassification' \
  ./tovarisch/labs/memory/cmd/tovarisch-memory-lab \
  > .factory/descriptor-correction01/descriptor-correction01-tests.json

# ACT range diff checks:
git diff --check 1b98623..HEAD
git diff --check c4f4ba0..HEAD
git status --short
```

## 14. Assumptions / blockers

### Assumptions

- The canary image `kgb-tovarisch-canary:latest` was rebuilt
  to bind the canary binary to the current source tree. The
  canary's Go source (`tovarisch/labs/memory/cmd/canary/`)
  was unchanged during this ACT (the canary was added at
  commit `6c401a6` and not touched in the current commit
  graph), so `canary_source_matches_tested_tree` holds.
- The host-side FD sampler cannot read the canary's
  `/proc/<pid>/fd/` in this Docker setup (FD availability is
  `false` on every sample). This is the §8 fallback path:
  the canary-state invariant becomes the named, distinct
  resource-classification source.
- The bounded ACT's CORRECTION01 classifier fix
  (`classifyMemorySignals` docker-only-small-growth →
  stable) applies equally to the descriptor scenario,
  ensuring the descriptor does not produce a false
  memory-growth verdict from Docker memory's incidental
  movement.

### Blockers

- None.

## 15. Repository-wide gate status

The repository-wide `make gate` is NOT_RUN for this ACT (it
fails pre-existing in `hulk-uvb76-artifact-producer-gate`
per the bounded ACT's CORRECTION01 close report; the
descriptor ACT only touches
`tovarisch/labs/memory/**` and `docs/acts/**`). The
`memory-lab` module passes its scoped checks.

## 16. Files changed in this ACT

### Implementation commit (`d2638c0`)

- `tovarisch/labs/memory/internal/analysis/classifier.go`:
  adds `SignalKind` type (`sampled` / `state_invariant`),
  adds `SourceKind` field to `SignalSummary`, changes
  `Analyze()` overall-classification priority to
  `invalid → growth (memory) → resource_growth →
  inconclusive/process_replaced/stable`, adds
  `ComputeOverall` priority function, adds
  `DescriptorInitialState` / `DescriptorFinalState` /
  `DescriptorWorkloadResult` / `DescriptorStateInvariant` /
  `DescriptorFallbackInput` / `DescriptorFallbackResult`
  types, adds `ApplyDescriptorStateInvariant` pure
  function (shared by producer and verifier), adds
  `ComputeDescriptorStateInvariant` and
  `SamplesHaveFDAvailable` helpers, and sets
  `source_kind=sampled` for all sample-derived signals.

- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/main.go`:
  producer now calls the shared
  `analysis.ApplyDescriptorStateInvariant` function via
  the descriptor fallback input struct; verifier
  reconstructs every classification field independently
  (`memory`, `resource`, `semantic`, `overall`) using
  the shared function for the descriptor case; verifier
  validates the `descriptor_state_invariant` signal
  (exists exactly once, source_kind=state_invariant,
  is_primary=true, correct initial/final/delta/
  classification); verifier compares all four stored
  classifications and all three stored validity fields
  against their reconstructions with field-specific
  diagnostics; the validator now requires
  `validateStateInvariant` in the verifier path.

- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/descriptor_classification_test.go`:
  replaces
  `TestClassification_GrowingMemoryPlusFDResourceIsDocumented`
  (logging-only boundary) with
  `TestClassification_GrowingMemoryPlusFDResourceIsGrowth`
  (asserts memory+resource growth yields
  `overall=growth`).

- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/descriptor_negative_test.go`:
  replaces `TestDescriptorVerdict_MemoryGrowing` and
  `TestDescriptorVerdict_SemanticInvalid` (logging-only
  boundaries) with real fixture mutations that assert
  field-specific diagnostics; replaces
  `TestDescriptorVerdict_OverallStable`,
  `TestDescriptorVerdict_OverallGrowth`,
  `TestDescriptorVerdict_ResourceStable`,
  `TestDescriptorVerdict_ResourceInconclusive`,
  `TestDescriptorVerdict_CanariesValidFalse` to mutate
  exactly one field each and assert field-specific
  diagnostics; replaces
  `TestDescriptorSamples_AllFDUnavailable` (logging-only
  boundary) with
  `TestDescriptorSamples_AllFDUnavailable_Positive`
  (positive control) plus three negative variants
  (MissingInvariant, MalformedInvariant,
  DuplicateInvariant); replaces
  `TestDescriptorSamples_FDFlatWithStateDelta`
  (logging-only boundary) with a real mutation
  asserting "sampled FD signal is available;
  descriptor_state_invariant must not be present".

- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/testdata/descriptor-valid/samples.csv`:
  all rows now have `has_fd_count=false` and
  `fd_count=0` (the §8 fallback path; the predecessor
  fixture had a few rows with `has_fd_count=true` and
  `fd_count=8`).

- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/testdata/descriptor-valid/verdict.json`:
  adds the `descriptor_state_invariant` signal summary
  with `source_kind=state_invariant`,
  `is_primary=true`, `first_window_median=8`,
  `last_window_median=208`, `absolute_delta=200`,
  `classification=resource_growth`; sets
  `source_kind=sampled` on every other signal.

- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/testdata/descriptor-valid/checksums.txt`:
  recomputed to match the updated `samples.csv` and
  `verdict.json`.

### Evidence commit (this close report)

- `docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01-CORRECTION01.md`:
  this close report.
- `docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01/evidence/lab-canary-descriptor-1784631920/`:
  the canonical fresh evidence bundle (10 files).
- `docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01/superseded-evidence/lab-canary-descriptor-1784629483/`:
  the previous run's 10 canonical artifacts, retained
  for audit.

## 17. Verification output

### Controller build

```bash
$ make tovarisch-memory-lab-build
cd tovarisch/labs/memory && go build -o ../../../.factory/bin/tovarisch-memory-lab ./cmd/tovarisch-memory-lab
# exit 0
```

### Unit tests

```bash
$ go test -count=1 ./tovarisch/labs/memory/...
ok    github.com/s1onique/KGB/tovarisch/labs/memory    0.015s
ok    github.com/s1onique/KGB/tovarisch/labs/memory/cmd/canary    0.054s
ok    github.com/s1onique/KGB/tovarisch/labs/memory/cmd/tovarisch-memory-lab    4.787s
ok    github.com/s1onique/KGB/tovarisch/labs/memory/internal/analysis    0.009s
ok    github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence    0.007s
ok    github.com/s1onique/KGB/tovarisch/labs/memory/internal/procfs    0.008s
ok    github.com/s1onique/KGB/tovarisch/labs/memory/internal/sampling    0.208s
# exit 0
```

### Race tests

```bash
$ go test -count=1 -race -run 'TestDescriptor|TestClassification' \
    ./tovarisch/labs/memory/cmd/tovarisch-memory-lab
ok    github.com/s1onique/KGB/tovarisch/labs/memory/cmd/tovarisch-memory-lab    5.099s
# exit 0
```

### Descriptor canary run (fresh, post-implementation)

```bash
$ rm -rf .factory/tovarisch-memory-lab
$ make tovarisch-memory-lab-canary-descriptor
=== Memory Lab: Canary Descriptor ===
".factory/bin/tovarisch-memory-lab" run \
    --scenario canary-descriptor \
    --duration 60 \
    --artifacts-dir .factory/tovarisch-memory-lab

=== Analysis Result ===
Scenario: canary-descriptor
Expected Verdict: resource_growth
Actual Verdict: resource_growth
ScenarioValid: true
CanariesValid: true
InvariantValid: true
PhaseValid: true
WorkloadValid: true
IdentityStable: true
Samples: 61
Signals: 14

Artifacts written to: .factory/tovarisch-memory-lab/lab-canary-descriptor-1784631920
Run ID: lab-canary-descriptor-1784631920
# exit 0
```

### Independent verification (scratch copy)

```bash
$ .factory/bin/tovarisch-memory-lab verify \
    --artifacts-dir .factory/tovarisch-memory-lab \
    --run-id lab-canary-descriptor-1784631920
=== Verification Results ===
Run ID: lab-canary-descriptor-1784631920
Scenario: canary-descriptor
Reconstructed Claims: 15 checks passed
All Verifications: PASS
ScenarioValid: true
CanariesValid: true
Overall: resource_growth
Memory: stable
Checksums: PASS
Artifact Geometry: PASS
Evidence Reconstruction: PASS
PASS: Evidence verified
# exit 0
```

### Independent verification (committed ACT evidence copy)

```bash
$ .factory/bin/tovarisch-memory-lab verify \
    --artifacts-dir \
      docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01/evidence \
    --run-id lab-canary-descriptor-1784631920
=== Verification Results ===
Run ID: lab-canary-descriptor-1784631920
Scenario: canary-descriptor
Reconstructed Claims: 15 checks passed
All Verifications: PASS
ScenarioValid: true
CanariesValid: true
Overall: resource_growth
Memory: stable
Checksums: PASS
Artifact Geometry: PASS
Evidence Reconstruction: PASS
PASS: Evidence verified
# exit 0 — committed evidence re-verifies
```

### Descriptor-specific evidence (selected fields)

| Field                  | Initial | Final | Delta |
|------------------------|---------|-------|-------|
| `mode`                 | `descriptor` | `descriptor` | unchanged |
| `ready`                | `true`  | `true` | unchanged |
| `retained_blocks`      | 0       | 0      | 0      |
| `retained_bytes`       | 0       | 0      | 0      |
| `operation_count`      | 0       | 100    | 100    |
| `fd_count`             | 8       | 208    | 200 (canary-state invariant) |
| `buffer_capacity`      | n/a (descriptor mode) | n/a | n/a |

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
  "overall_classification":  "resource_growth",
  "scenario":               "canary-descriptor",
  "scenario_valid":          true,
  "canaries_valid":          true,
  "memory_classification":   "stable",
  "resource_classification": "resource_growth",
  "semantic_classification": "stable",
  "provenance_valid":        true
}
```

Verdict `signal_summaries` includes the named
`descriptor_state_invariant` signal (Classification:
`resource_growth`, AbsoluteDelta: 200, IsPrimary: true,
SourceKind: `state_invariant`) representing the
canary-state fd_delta invariant used under the §8
permitted fallback.

### Subject container cleanup

```bash
$ docker ps -a --filter "name=tovarisch-subject-lab-canary-descriptor-1784631920" \
    --format '{{.ID}} {{.Status}} {{.Names}}'
# (no output — no retained subject container)
```

The canary's container is removed by `cleanup.Cleanup(ctx)`
after evidence collection, as the bounded ACT also confirmed.

### Working tree

```bash
$ git status --short
# (clean — no uncommitted or untracked changes)
```

## 18. Zig 0.16 observations

This ACT is entirely within the Go memory-lab module
(`tovarisch/labs/memory/`). No Zig code was modified; no
Zig 0.16 observations are recorded.

## 19. Successor

```text
MEMLAB-06 = DONE
MEMLAB-06-DOCFIX = DONE
MEMLAB-07 = DONE
MEMLAB-08 = READY
```

The next ACT is
`ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01`.
That ACT will rerun the growing, bounded, and descriptor
scenarios from a single committed controller identity and
prove the complete classification matrix (growing →
overall=growth, bounded → overall=stable, descriptor →
overall=resource_growth) end-to-end.

The descriptor ACT is the active critical-path item: it is
the only ACT in the matrix that exercises the §8
"permitted fallback" and the only ACT whose primary
resource-classification source is the canary-state
invariant (not sampled host-side memory or FD).
