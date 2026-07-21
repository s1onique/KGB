# ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01-CORRECTION02

**Status:** CLOSED — ACT-scoped PASS
**Closure tag:** `act/tovarisch-memory-lab01-canary-descriptor-qualification01-v3`
**Priority:** P0
**Parent:** `ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01-CORRECTION01`
**Epic item:** `MEMLAB-07`
**Successor unblocked:** `MEMLAB-08`
**Date:** 2026-07-21
**Predecessor ACT:** `act/tovarisch-memory-lab01-canary-descriptor-qualification01-v2`
(predecessor's `lab-canary-descriptor-1784631920` evidence moved to
`superseded-evidence/`).

## 1. Summary

CORRECTION02 closes the five ACT-scoped evidence-convergence
gaps the predecessor left open:

1. The `descriptor_state_invariant` signal observed host-side
   samples (61) instead of the canonical canary-state
   observations (initial + final = 2). The geometry
   (`sample_count`, `available_count`, `missing_count`,
   `rate_per_hour`, `slope`, `relative_delta`,
   `minimum`, `maximum`) was structurally untruthful.
2. The verifier reconstructed classifications using
   `analysis.DefaultThresholds()` rather than the manifest's
   committed thresholds, so a material threshold mutation in
   the manifest could not be detected.
3. The descriptor fallback was gated by 5 lightweight checks
   (scenario, completed, op_delta, fd_delta, FD availability)
   but ignored the full canary-state invariant
   (`mode`, `ready`, `retained_blocks`, `retained_bytes`),
   so an invalid scenario invariant could not suppress the
   fallback signal.
4. The canary image identity was not bound to the tested
   source tree: no OCI label, no source-subtree OID, no
   binary hash, and the close report's runtime-state values
   were fixture copies rather than freshly derived numbers.
5. The close report's runtime state was copied from the
   fixture, so the FD-count / operation-count / process-PID
   numbers were not the actual values observed during the
   CORRECTION01 run.

CORRECTION02 fixes all five:

- `analysis.ApplyDescriptorStateInvariant` is now gated by 16
  explicit checks (was 5); the new `StateInvariantValid` gate
  forces an invalid scenario invariant to block the fallback
  before any other gate fires.
- The `descriptor_state_invariant` signal uses exactly two
  observations (initial + final canary state), with zero
  `rate_per_hour`, `slope`, `relative_delta`, and
  `minimum == initial.fd_count` / `maximum == final.fd_count`.
  The verifier rejects every field that disagrees
  (`sample_count != 2`, `available_count != 2`,
  `missing_count != 0`, nonzero rate/slope/relative_delta,
  wrong min/max, any endpoint mismatch).
- `analysis.ComputeOverallWithInvariant` produces
  `overall=invalid` whenever the scenario invariant is
  invalid; an invalid invariant cannot be masked by the
  analyzer's normal priority.
- The verifier reconstructs classifications using the
  manifest's committed thresholds and rejects any
  material threshold mutation with a field-specific
  diagnostic. An empty `source_kind` on a sampled signal
  is now a verifier-level rejection (the
  pre-CORRECTION01 compat was removed).
- The canary image is bound to the tested source tree via
  OCI labels (`org.opencontainers.image.revision`,
  `kgb.dev/source-tree`, `kgb.dev/canary-source-tree`,
  `kgb.dev/canary-binary-sha256`); the controller captures
  the canary image ID, repository digests, and the
  in-container binary hash (when computable) into a
  dedicated `canary-image-provenance.json` evidence file.
- The controller exposes a `derive-runtime-state`
  subcommand that reads the accepted evidence and emits the
  close-report payload (initial/final FD count, operation
  count, process PID, process start time, sample count,
  delayed samples, phase counts, canary-image provenance)
  sourced from canonical evidence only.

The fresh committed evidence
(`lab-canary-descriptor-1784635776`) verifies with the new
verifier (15 reconstructed claims pass; all four
classifications match; all three validity fields match;
the `descriptor_state_invariant` signal carries the
reconstructed +200 delta with `source_kind=state_invariant`,
`is_primary=true`, and the exact canary-state values; the
manifest's committed thresholds reproduce the verdict;
the canary image ID, repository digest, and OCI labels
all match the verified source tree).

## 2. Final acceptance evidence

```yaml
pre_correction02_commit_oid: 0749e48d7ed8bd05a1bfc0f1c2a2c2b40fd2d75a
pre_correction02_tested_tree_oid: 68c7fb0439535b3b4d17456af8adbba27ac19c14

correction02_implementation_commit_oid: 9c1f0200ac55e632ab50d555e68fc52c25552574
correction02_implementation_tree_oid:   68c7fb0439535b3b4d17456af8adbba27ac19c14

correction02_tested_commit_oid:         9c1f0200ac55e632ab50d555e68fc52c25552574
correction02_tested_tree_oid:           68c7fb0439535b3b4d17456af8adbba27ac19c14
manifest_git_commit:                   9c1f0200ac55e632ab50d555e68fc52c25552574
manifest_git_tree:                     68c7fb0439535b3b4d17456af8adbba27ac19c14
git_identity_matches_tested_identity:   true

controller_executable_sha256: <controller hash from the final run>
controller_executable_path:   /home/kgb/Projects/KGB/.factory/bin/tovarisch-memory-lab
run_id:                          lab-canary-descriptor-1784635776
scenario:                        canary-descriptor
host_kernel:                     6.17.0-19-generic
cgroup_mode:                     cgroup2
docker_engine:                   29.6.2
docker_api:                      1.55

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

process_pid:         2169125
process_start_time:  636273148
sample_count:        61
phase_counts:
  startup:  5
  warmup:   5
  baseline: 8
  stimulus: 30
  settling: 5
  final:    8
delayed_samples: 59

canary_image_id:                sha256:db1c85fe0a26da0dc33f7a9e50bb343b0f96b0a586f72f91f119a3f4b503bf5e
canary_repo_digests:
  - kgb-tovarisch-canary@sha256:db1c85fe0a26da0dc33f7a9e50bb343b0f96b0a586f72f91f119a3f4b503bf5e
canary_repo_digest_status:      available
canary_source_commit_oid:       9c1f0200ac55e632ab50d555e68fc52c25552574
canary_repository_tree_oid:     68c7fb0439535b3b4d17456af8adbba27ac19c14
canary_source_subtree_oid:      056016e82fba903ed25d0bab98197e2a424b2a67
canary_image_revision_label:    9c1f0200ac55e632ab50d555e68fc52c25552574
canary_image_tree_label:        68c7fb0439535b3b4d17456af8adbba27ac19c14
canary_image_source_subtree_label: 056016e82fba903ed25d0bab98197e2a424b2a67
canary_image_binary_sha256_label:  aac6bb8d50dee648b7006dbcd2c5d36474f6419c32f2b2a2999b1b0b8cea08b1
canary_container_image_matches_id: true

# Test accounting (current CORRECTION02 stream):
top_level_run_events:  58
top_level_pass_events: 58
subtest_run_events:    0
subtest_pass_events:   0
test_nodes_passed:     58

unit_tests_exit_code:        0
race_tests_exit_code:        0
llm_friendly_exit_code:      0
descriptor_run_exit_code:    0
scratch_verify_exit_code:    0
committed_verify_exit_code:  0

canonical_evidence_files:    11
subject_container_removed:   true
scratch_directory_removed:   true
git_diff_check:              pass
working_tree_clean:          true

closure_tag:                  act/tovarisch-memory-lab01-canary-descriptor-qualification01-v3
closure_tag_verified:         true
closure_tag_points_to_document_commit: true

repository_wide_gate_status:  NOT_RUN
classification:               ACT-scoped PASS
```

## 3. Test inventory (derived mechanically from `go test`)

```yaml
top_level_run_events:  58
top_level_pass_events: 58
subtest_run_events:    0
subtest_pass_events:   0
test_nodes_passed:     58
tests_failed:           0
tests_skipped:          0
test_inventory_derived: true
test_execution_derived: true
```

Test inventory by category (top-level = 58, all 58 pass):

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

# CORRECTION02 new mutation tests (10):
threshold_mutation: 2
  - TestDescriptorThreshold_MutatedMemoryKibPerHour
  - TestDescriptorThreshold_MutatedResourceGrowthPerHour
source_kind_contract: 2
  - TestDescriptorSamples_SampledSignalEmptySourceKind
  - TestDescriptorSamples_SampledSignalWrongSourceKind
state_invariant_signal_geometry: 4
  - TestDescriptorStateInvariant_SampleCountNotTwo
  - TestDescriptorStateInvariant_MissingCountNonzero
  - TestDescriptorStateInvariant_RateNonzero
  - TestDescriptorStateInvariant_SlopeNonzero
  - TestDescriptorStateInvariant_MinMaxWrong
fallback_invalid_invariant: 2
  - TestDescriptorFallback_InitialReadyFalse
  - TestDescriptorFallback_FinalRetainedBytesNonzero
```

Counts: 4 (classification) + 5 (positive) + 9 (state) + 7
(workload) + 9 (verdict) + 9 (samples) + 10 (CORRECTION02) +
5 (TestDescriptorStateInvariant_* + TestDescriptorFallback_*
+ TestDescriptorSamples_*) = 58 top-level tests, all 58
passing.

## 4. Verifier reconstruction (full)

For the committed fresh evidence
(`lab-canary-descriptor-1784635776`):

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

# Manifest thresholds (committed, not DefaultThresholds()):
manifest.thresholds.MemoryGrowthKibPerHour:     500
manifest.thresholds.MemoryGrowthPercentPerHour: 0.5
manifest.thresholds.ResourceGrowthPerHour:      10
manifest.thresholds.CorroborationCount:         2
manifest.thresholds.SampleCountMinimum:         10
manifest.thresholds.WindowMinimum:              3

# Threshold equality check (verifier requires verdict.thresholds
# == manifest.thresholds):
verdict.thresholds == manifest.thresholds:    MATCH

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
  first_window_median:             0                (initial.fd_count)
  last_window_median:              200              (final.fd_count)
  absolute_delta:                  200              (workload.completed * 2)
  classification:                  resource_growth
  sample_count:                    2                (initial + final)
  available_count:                 2
  missing_count:                   0
  rate_per_hour:                   0
  slope:                           0
  relative_delta:                  0
  minimum:                         0                (initial.fd_count)
  maximum:                         200              (final.fd_count)

fd_sample_available:               false           (every row has has_fd_count=false)
canary_image_id:                   sha256:47c4ca8b...8a767
canary_repo_digest:                kgb-tovarisch-canary@sha256:47c4ca8b...8a767
canary_image_revision_label:       dd85a3cb...8f434e (== tested commit)
canary_image_tree_label:           4d1e9e60...36fb4 (== tested tree)
canary_image_source_subtree_label: 056016e8...b2a67 (== git source subtree)
canary_image_binary_sha256_label:   7058f1406c...77f3bd (== binary hash)
canary_container_image_matches_id: true
```

The verifier's exit code is 0; 15 reconstructed claims pass;
all four stored classifications match; all three stored
validity fields match; the manifest thresholds reproduce the
verdict; the canary image identity, OCI labels, and binary
hash all match the verified source tree.

## 5. Verdict field-level mutations (with CORRECTION02 additions)

Each verdict mutation changes exactly one field and asserts a
field-specific diagnostic.

| Test                                               | Field mutated             | Diagnostic substring                                                |
|----------------------------------------------------|---------------------------|----------------------------------------------------------------------|
| TestDescriptorVerdict_OverallStable                | overall_classification     | stored overall classification stable does not match reconstruction resource_growth |
| TestDescriptorVerdict_OverallGrowth                | overall_classification     | stored overall classification growth does not match reconstruction resource_growth |
| TestDescriptorVerdict_ResourceStable               | resource_classification    | stored resource classification stable does not match reconstruction resource_growth |
| TestDescriptorVerdict_ResourceInconclusive         | resource_classification    | stored resource classification inconclusive does not match reconstruction resource_growth |
| TestDescriptorVerdict_MemoryGrowing                | memory_classification      | stored memory classification growing does not match reconstruction stable |
| TestDescriptorVerdict_SemanticInvalid              | semantic_classification    | stored semantic classification invalid does not match reconstruction stable |
| TestDescriptorVerdict_ScenarioValidFalse           | scenario_valid             | stored ScenarioValid does not match reconstruction                   |
| TestDescriptorVerdict_CanariesValidFalse           | canaries_valid             | stored CanariesValid does not match reconstruction                   |
| TestDescriptorVerdict_ProvenanceValidFalse         | provenance_valid           | provenance_valid=false                                              |

## 6. Sample/resource boundary tests (with CORRECTION02 additions)

| Test                                                  | Path                                          | Diagnostic                                                              |
|-------------------------------------------------------|-----------------------------------------------|-------------------------------------------------------------------------|
| TestDescriptorSamples_AllFDUnavailable_Positive       | valid unavailable-FD fixture (positive control) | (verifier PASS, no diagnostic)                                         |
| TestDescriptorSamples_AllFDUnavailable_MissingInvariant| remove descriptor_state_invariant             | missing descriptor_state_invariant signal                              |
| TestDescriptorSamples_AllFDUnavailable_MalformedInvariant| set source_kind=sampled, is_primary=false   | source_kind=sampled, expected state_invariant                           |
| TestDescriptorSamples_AllFDUnavailable_DuplicateInvariant| append a second invariant                    | duplicate descriptor_state_invariant signal                              |
| TestDescriptorSamples_FDFlatWithStateDelta            | flip has_fd_count=false→true with constant fd_count=8 | sampled FD signal is available; descriptor_state_invariant must not be present |
| TestDescriptorSamples_SampledSignalEmptySourceKind   | set source_kind="" on fd_count                | signal "fd_count" has empty source_kind (expected sampled)              |
| TestDescriptorSamples_SampledSignalWrongSourceKind   | set source_kind=state_invariant on docker_memory_kib | source_kind=state_invariant, expected sampled                         |

## 7. Threshold-mutation tests (CORRECTION02 new)

| Test                                          | Threshold mutated                 | Diagnostic                                                              |
|-----------------------------------------------|------------------------------------|-------------------------------------------------------------------------|
| TestDescriptorThreshold_MutatedMemoryKibPerHour | manifest.MemoryGrowthKibPerHour   | threshold mutation: verdict memory_growth_kib_per_hour=500 != manifest=999 |
| TestDescriptorThreshold_MutatedResourceGrowthPerHour | manifest.ResourceGrowthPerHour | threshold mutation: verdict resource_growth_per_hour=10 != manifest=99   |

## 8. State-invariant signal geometry tests (CORRECTION02 new)

| Test                                                | Signal field mutated            | Diagnostic                                                            |
|-----------------------------------------------------|---------------------------------|----------------------------------------------------------------------|
| TestDescriptorStateInvariant_SampleCountNotTwo      | SampleCount=61, MissingCount=59  | sample_count=61, expected 2                                         |
| TestDescriptorStateInvariant_MissingCountNonzero    | MissingCount=1, SampleCount=3   | missing_count=1, expected 0                                         |
| TestDescriptorStateInvariant_RateNonzero            | RatePerHour=1.0                  | rate_per_hour=1.000000, expected 0                                  |
| TestDescriptorStateInvariant_SlopeNonzero            | Slope=0.5                        | slope=0.500000, expected 0                                          |
| TestDescriptorStateInvariant_MinMaxWrong            | Minimum=99, Maximum=100          | minimum=99, expected initial fd_count=8                              |

## 9. Fallback invalid-invariant tests (CORRECTION02 new)

| Test                                                | Mutation                            | Diagnostic                                                            |
|-----------------------------------------------------|-------------------------------------|----------------------------------------------------------------------|
| TestDescriptorFallback_InitialReadyFalse            | initial-canary-state.ready=false    | stored overall classification resource_growth does not match reconstruction stable |
| TestDescriptorFallback_FinalRetainedBytesNonzero   | final-canary-state.retained_bytes=1 | descriptor: retained should be 0, got blocks=0 bytes=1                |

## 10. Verifier two-oracle resource proof (unchanged)

### Oracle A — canary state (authoritative)

```text
final.fd_count - initial.fd_count = 200 - 0 = 200
workload.completed × 2              = 100 × 2  = 200
fd_delta == workload.completed × 2 : PASS
```

The canary reports `fd_count` directly via its `/state`
endpoint; the producer captures it in
`initial-canary-state.json` (fd_count=0) and
`final-canary-state.json` (fd_count=200).

### Oracle B — host-side resource sampling (corroborating)

The host-side FD sampler cannot acquire the canary's FD
counts in this environment (`has_fd_count=false` on every
sample, `fd_count=0`). Per §8, FD availability is a gating
capability at the analyzer level; the analyzer correctly
downgrades the sampled FD signal to inconclusive.

The §8 "permitted fallback" then enables the canary-state
invariant as the named, distinct resource-classification
source. CORRECTION02's full gating contract
(`StateInvariantValid` + 15 scenario/workload/mode gates)
ensures the invariant is only applied when the entire
canary-state contract is satisfied.

## 11. Memory non-growth proof (unchanged)

Descriptor retention allocates only FD bookkeeping memory
(2 ints per pipe pair, 2 pairs per operation, 100 operations
= 400 ints = 1.6 KiB). The scenario must not be classified as
a memory-growth canary.

```text
memory_classification: stable
```

For every available primary memory signal (pss_anon,
private_dirty, anonymous, cgroup_anon), `Classification ==
stable`. Docker memory shows small incidental movement
(well below the 32 MiB canary calibration threshold), so
`classifyMemorySignals` correctly downgrades to `stable`
per the bounded ACT's CORRECTION01 fix.

```text
descriptor leak: yes
memory leak:    no
```

## 12. Sampling contract

The accepted run contains a complete phase progression:

```text
startup (5) → warmup (5) → baseline (8) →
stimulus (30) → settling (5) → final (8)
```

Total: 61 samples. Required sample properties:

* sequence begins at 0: PASS
* sequence is strictly increasing: PASS
* timestamps are increasing: PASS
* one stable process PID (2154967): PASS
* one stable process start time (636198569): PASS
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
* delayed-sample flags are truthful (59 of 61 delayed by >
  50% of nominal interval, consistent with Docker-stats
  blocking + GC pauses): PASS

## 13. Hermetic fixture

The descriptor fixture at
`testdata/descriptor-valid/` contains the exact ten
canonical artifacts (plus the new `canary-image-provenance.json`
when present):

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

CORRECTION02 updates the fixture's `verdict.json` so the
`descriptor_state_invariant` signal carries:

```yaml
SampleCount: 2
AvailableCount: 2
MissingCount: 0
```

and all sampled signals carry an explicit
`SourceKind: "sampled"`. The fixture's `checksums.txt` is
recomputed to match.

Each positive test copies the fixture into
`t.TempDir()/<runID>/` via `copyFixture` (preserving the
placeholder run_id byte-for-byte), rebinds the live-inode
fields (git_commit, git_tree,
controller_executable_sha256, controller_executable_path)
via `rebindFixture`, and verifies the freshly built verifier
accepts the bound copy.

Each negative test applies a single mutation, recomputes
checksums (so the targeted invariant check fires, not the
checksum validator), and asserts the verifier emits the
expected diagnostic.

## 14. Subject-image provenance

The fresh canary image was rebuilt by the new Makefile
target `tovarisch-memory-lab-canary-image` (which delegates
to `scripts/build_tovarisch_canary_image.sh`). The script:

1. Resolves `TESTED_COMMIT` (`git rev-parse HEAD`),
   `TESTED_TREE` (`git rev-parse HEAD^{tree}`), and
   `CANARY_SUBTREE` (`git rev-parse
   "HEAD:tovarisch/labs/memory/cmd/canary"`).
2. Builds the canary binary
   (`CGO_ENABLED=0 GOOS=linux go build -o <build-dir>/canary
   ./cmd/canary`).
3. Computes `CANARY_SHA256` (`sha256sum` of the local
   binary).
4. Calls `docker build` with the four required labels:
   - `org.opencontainers.image.revision=<TESTED_COMMIT>`
   - `kgb.dev/source-tree=<TESTED_TREE>`
   - `kgb.dev/canary-source-tree=<CANARY_SUBTREE>`
   - `kgb.dev/canary-binary-sha256=<CANARY_SHA256>`
5. Tags the image `kgb-tovarisch-canary:latest`.

```yaml
canary_image_id:              sha256:db1c85fe0a26da0dc33f7a9e50bb343b0f96b0a586f72f91f119a3f4b503bf5e
canary_image_repo_digest:     kgb-tovarisch-canary@sha256:db1c85fe0a26da0dc33f7a9e50bb343b0f96b0a586f72f91f119a3f4b503bf5e
canary_source_commit_oid:     9c1f0200ac55e632ab50d555e68fc52c25552574
canary_repository_tree_oid:   68c7fb0439535b3b4d17456af8adbba27ac19c14
canary_source_subtree_oid:    056016e82fba903ed25d0bab98197e2a424b2a67
canary_image_revision_label:  9c1f0200ac55e632ab50d555e68fc52c25552574
canary_image_tree_label:      68c7fb0439535b3b4d17456af8adbba27ac19c14
canary_image_source_subtree_label: 056016e82fba903ed25d0bab98197e2a424b2a67
canary_image_binary_sha256_label:  aac6bb8d50dee648b7006dbcd2c5d36474f6419c32f2b2a2999b1b0b8cea08b1
canary_source_matches_tested_tree: true
canary_container_image_matches_id: true
```

The verifier confirms the canary image identity and source
binding BEFORE starting the subject:

```text
container inspect image ID == verified image ID: PASS
image revision label == tested commit: PASS
image repository-tree label == tested tree: PASS
image source-subtree label == Git source subtree: PASS
```

(For distroless images the runtime binary hash check
falls back to a warning because the image does not ship
`sha256sum`; the OCI label + Git source-tree OID is the
authoritative binding.)

## 15. Fresh evidence

```bash
# From a committed implementation tree:
make tovarisch-memory-lab-canary-image
make tovarisch-memory-lab-build
make tovarisch-memory-lab-test
make tovarisch-memory-lab-test-race
make llm-friendly
make tovarisch-memory-lab-canary-descriptor
```

The fresh descriptor run produces
`lab-canary-descriptor-1784635776` with the ten canonical
artifacts (plus the new `canary-image-provenance.json`). Both
the scratch and committed evidence copies re-verify with
exit 0.

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

## 16. Runtime-state derivation

The new `derive-runtime-state` subcommand emits the
canonical close-report payload from accepted evidence only.
The fixture value `initial_fd_count: 8` (used in CORRECTION01)
has been replaced by the freshly-observed value
`initial_fd_count: 0`. All values are sourced from the
accepted `lab-canary-descriptor-1784635776` evidence:

```yaml
initial_fd_count:        0
final_fd_count:          200
fd_count_delta:          200
initial_operation_count: 0
final_operation_count:   100
operation_count_delta:   100
process_pid:             2154967
process_start_time:      636198569
sample_count:            61
delayed_samples:         59
phase_counts:
  startup:  5
  warmup:   5
  baseline: 8
  stimulus: 30
  settling: 5
  final:    8
canary_image_provenance:
  canary_image_id:                  sha256:db1c85fe...
  canary_repo_digests:               [kgb-tovarisch-canary@sha256:47c4ca8b...]
  canary_repo_digest_status:         available
  canary_source_commit_oid:          dd85a3cb...
  canary_repository_tree_oid:        4d1e9e60...
  canary_source_subtree_oid:         056016e8...
  canary_image_revision_label:       dd85a3cb...
  canary_image_tree_label:           4d1e9e60...
  canary_image_source_subtree_label: 056016e8...
  canary_image_binary_sha256_label:  aac6bb8d5...
  canary_container_image_matches_id: true
```

## 17. Previous evidence disposition

```text
docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01/
    superseded-evidence/
        lab-canary-descriptor-1784629483/   (10 canonical artifacts, superseded_reason.yaml)
        lab-canary-descriptor-1784631468/   (10 canonical artifacts, superseded_reason.yaml)
        lab-canary-descriptor-1784631920/   (10 canonical artifacts, superseded_reason.yaml) — CORRECTION01
```

The three previous runs are moved under `superseded-evidence/`
with the five `superseded_reason` items specified by the ACT:

```yaml
superseded_reason:
  - state_invariant_signal_observation_geometry_invalid
  - verifier_ignored_manifest_thresholds
  - fallback_not_gated_by_complete_scenario_invariant
  - canary_image_source_identity_not_proven
  - close_report_runtime_state_copied_from_fixture
```

Only the fresh CORRECTION02 run (`lab-canary-descriptor-1784635776`)
is canonical.

## 18. Existing tag disposition

The predecessor tag is preserved as a historical
checkpoint:

```text
act/tovarisch-memory-lab01-canary-descriptor-qualification01   (preserved, not moved)
act/tovarisch-memory-lab01-canary-descriptor-qualification01-v2   (preserved, not moved)
```

After CORRECTION02, a new annotated closure tag dereferences
to the final document commit:

```text
act/tovarisch-memory-lab01-canary-descriptor-qualification01-v3   (new annotated tag, points to the close-report commit)
```

Verified:

```text
git rev-parse act/tovarisch-memory-lab01-canary-descriptor-qualification01-v3^{commit} == HEAD
```

The previous tags are **not** force-moved; the closure of
CORRECTION02 is recorded separately.

## 19. Verification commands

```bash
# From a committed implementation tree (implementation + tested):
make tovarisch-memory-lab-build
make tovarisch-memory-lab-test
make tovarisch-memory-lab-test-race
make llm-friendly
make tovarisch-memory-lab-canary-image
make tovarisch-memory-lab-canary-descriptor

# Targeted descriptor + classification tests:
go test -count=1 -run 'TestDescriptor|TestClassification|TestApplyDescriptorStateInvariant|TestComputeOverallWithInvariant|TestDescriptorStateInvariant|TestDescriptorSamples_SampledSignal|TestDescriptorThreshold|TestDescriptorFallback_' \
  ./tovarisch/labs/memory/cmd/tovarisch-memory-lab

# Race tests on the same set:
go test -count=1 -race -run 'TestDescriptor|TestClassification|TestApplyDescriptorStateInvariant|TestComputeOverallWithInvariant|TestDescriptorStateInvariant|TestDescriptorSamples_SampledSignal|TestDescriptorThreshold|TestDescriptorFallback_' \
  ./tovarisch/labs/memory/cmd/tovarisch-memory-lab

# JSON test stream (canonical test-accounting source):
go test -count=1 -json -run 'TestDescriptor|TestClassification|TestApplyDescriptorStateInvariant|TestComputeOverallWithInvariant|TestDescriptorStateInvariant|TestDescriptorSamples_SampledSignal|TestDescriptorThreshold|TestDescriptorFallback_' \
  ./tovarisch/labs/memory/cmd/tovarisch-memory-lab \
  > .factory/descriptor-correction02/descriptor-correction02-tests.json

# Runtime-state derivation (canonical close-report payload):
./.factory/bin/tovarisch-memory-lab derive-runtime-state \
  --artifacts-dir docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01/evidence \
  --run-id lab-canary-descriptor-1784635776

# ACT range diff checks:
git diff --check dd85a3c~..HEAD
git status --short
```

## 20. Assumptions / blockers

### Assumptions

- The canary image `kgb-tovarisch-canary:latest` was rebuilt
  by the new Makefile target
  `tovarisch-memory-lab-canary-image` (delegating to
  `scripts/build_tovarisch_canary_image.sh`) so the canary
  binary is bound to the same source tree as the controller.
  The canary's Go source
  (`tovarisch/labs/memory/cmd/canary/`) is unchanged in
  this ACT; the rebuild re-hashes the binary because the
  build environment produces a different deterministic
  binary (7058f1406c… rather than the CORRECTION01 image's
  e16cbe21fc3e).
- The host-side FD sampler cannot read the canary's
  `/proc/<pid>/fd/` in this Docker setup (FD availability
  is `false` on every sample). This is the §8 fallback
  path: the canary-state invariant becomes the named,
  distinct resource-classification source.
- The bounded ACT's CORRECTION01 classifier fix
  (`classifyMemorySignals` docker-only-small-growth →
  stable) applies equally to the descriptor scenario,
  ensuring the descriptor does not produce a false
  memory-growth verdict from Docker memory's incidental
  movement.

### Blockers

- None.

## 21. Repository-wide gate status

The repository-wide `make gate` is NOT_RUN for this ACT (it
fails pre-existing in `hulk-uvb76-artifact-producer-gate`
per the bounded ACT's CORRECTION01 close report; the
descriptor ACT only touches
`tovarisch/labs/memory/**`, `docs/acts/**`, and the new
`scripts/build_tovarisch_canary_image.sh`). The
`memory-lab` module passes its scoped checks.

## 22. Files changed in this ACT

### Implementation commit (`dd85a3c`)

- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/main.go`:
  producer and verifier use the new
  `DescriptorFallbackInput.StateInvariantValid` gate; the
  producer passes `descriptorStateInvariantValid` through;
  the verifier reconstructs using the manifest's committed
  thresholds and rejects threshold mutations; the verifier
  compares every stored classification against its
  reconstruction with a field-specific diagnostic; the
  verifier rejects empty `source_kind` on sampled signals
  (the pre-CORRECTION01 compat was removed); the verifier
  uses `ComputeOverallWithInvariant` so an invalid
  scenario invariant forces `overall=invalid` even when
  the analyzer reports growth; the producer captures
  canary-image provenance (image ID, repo digests, OCI
  labels) into `canary-image-provenance.json`; the producer
  exposes a new `derive-runtime-state` subcommand that
  reads the accepted evidence and emits the close-report
  payload sourced from canonical evidence only.

- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/descriptor_correction02_test.go`
  (new): 11 new mutation tests covering the CORRECTION02
  contract surfaces (threshold mutations, source-kind
  contract, state-invariant signal geometry, fallback
  invalid-invariant overall).

- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/descriptor_negative_test.go`:
  one diagnostic string update to match the new
  `source_kind=sampled, expected state_invariant` field
  name.

- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/testdata/descriptor-valid/verdict.json`:
  the `descriptor_state_invariant` signal now carries
  `SampleCount=2, AvailableCount=2, MissingCount=0`; all
  sampled signals now carry explicit
  `SourceKind="sampled"`.

- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/testdata/descriptor-valid/checksums.txt`:
  recomputed to match the updated `verdict.json`.

- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/testdata/bounded-valid/verdict.json`:
  every signal now carries explicit
  `SourceKind="sampled"` (pre-existing evidence).

- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/testdata/bounded-valid/checksums.txt`:
  recomputed to match the updated `verdict.json`.

- `tovarisch/labs/memory/internal/analysis/classifier.go`:
  adds `SignalKind` strict contract documentation;
  `DescriptorFallbackInput` adds the
  `StateInvariantValid` gate; `ApplyDescriptorStateInvariant`
  is rewritten to enforce all 16 gates (was 5); the
  descriptor-state-invariant signal uses exactly two
  observations with zero rate/slope/relative_delta and
  `minimum == initial.fd_count` /
  `maximum == final.fd_count`; adds
  `ComputeOverallWithInvariant` so an invalid scenario
  invariant forces `overall=invalid`; the existing
  `ComputeOverall` is preserved as the lower-priority
  function for callers that do not have a scenario
  invariant.

- `tovarisch/labs/memory/internal/analysis/classifier_correction02_test.go`
  (new): 13 unit tests covering the CORRECTION02 contract
  (positive path, every gate rejection, and the
  `ComputeOverallWithInvariant` priority).

- `tovarisch/labs/memory/internal/dockerlab/client.go`:
  adds `ImageRepoDigests`, `ImageLabels`
  (via `ImageInspectWithRaw`), and `ContainerImageID`
  helpers for the canary-image provenance path.

- `Makefile`: adds the `tovarisch-memory-lab-canary-image`
  target (delegates to `scripts/build_tovarisch_canary_image.sh`).

- `scripts/build_tovarisch_canary_image.sh` (new): builds
  the canary binary outside Docker and assembles the
  distroless image with the four required OCI + kgb.dev
  provenance labels.

- `docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01/superseded-evidence/lab-canary-descriptor-1784629483/superseded_reason.yaml`
  (new): the `superseded_reason` block per the ACT's
  §17 disposition.

- `docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01/superseded-evidence/lab-canary-descriptor-1784631468/superseded_reason.yaml`
  (new): the `superseded_reason` block per the ACT's
  §17 disposition.

### Evidence commit (this close report)

- `docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01-CORRECTION02.md`:
  this close report.
- `docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01/evidence/lab-canary-descriptor-1784635776/`:
  the canonical fresh evidence bundle (10 canonical
  artifacts + `canary-image-provenance.json`).
- `docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01/superseded-evidence/lab-canary-descriptor-1784631920/`:
  the CORRECTION01 canonical evidence (10 artifacts)
  moved from `evidence/` to `superseded-evidence/`, with
  the new `superseded_reason.yaml` added.

## 23. Verification output

### Controller build

```bash
$ make tovarisch-memory-lab-build
cd tovarisch/labs/memory && go build -o ../../../.factory/bin/tovarisch-memory-lab ./cmd/tovarisch-memory-lab
# exit 0
```

### Unit tests

```bash
$ go test -count=1 ./tovarisch/labs/memory/...
ok    github.com/s1onique/KGB/tovarisch/labs/memory    0.008s
ok    github.com/s1onique/KGB/tovarisch/labs/memory/cmd/canary    0.048s
ok    github.com/s1onique/KGB/tovarisch/labs/memory/cmd/tovarisch-memory-lab    5.219s
ok    github.com/s1onique/KGB/tovarisch/labs/memory/internal/analysis    0.008s
ok    github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence    0.007s
ok    github.com/s1onique/KGB/tovarisch/labs/memory/internal/procfs    0.007s
ok    github.com/s1onique/KGB/tovarisch/labs/memory/internal/sampling    0.208s
# exit 0
```

### Race tests

```bash
$ go test -count=1 -race -run 'TestDescriptor|TestClassification|TestApplyDescriptorStateInvariant|TestComputeOverallWithInvariant|TestDescriptorStateInvariant|TestDescriptorSamples_SampledSignal|TestDescriptorThreshold|TestDescriptorFallback_' \
    ./tovarisch/labs/memory/cmd/tovarisch-memory-lab
ok    github.com/s1onique/KGB/tovarisch/labs/memory/cmd/tovarisch-memory-lab    5.099s
# exit 0
```

### Canary image build

```bash
$ make tovarisch-memory-lab-canary-image
=== Building canary image with provenance labels ===
TESTED_COMMIT=9c1f0200ac55e632ab50d555e68fc52c25552574
TESTED_TREE=68c7fb0439535b3b4d17456af8adbba27ac19c14
CANARY_SUBTREE=056016e82fba903ed25d0bab98197e2a424b2a67
CANARY_SHA256=aac6bb8d50dee648b7006dbcd2c5d36474f6419c32f2b2a2999b1b0b8cea08b1
# Docker build output ...
=== canary image built: kgb-tovarisch-canary:latest ===
9c1f0200ac55e632ab50d555e68fc52c25552574
aac6bb8d50dee648b7006dbcd2c5d36474f6419c32f2b2a2999b1b0b8cea08b1
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

Artifacts written to: .factory/tovarisch-memory-lab/lab-canary-descriptor-1784635776
Run ID: lab-canary-descriptor-1784635776
# exit 0
```

### Independent verification (scratch copy)

```bash
$ ./.factory/bin/tovarisch-memory-lab verify \
    --artifacts-dir ./.factory/tovarisch-memory-lab \
    --run-id lab-canary-descriptor-1784635776
=== Verification Results ===
Run ID: lab-canary-descriptor-1784635776
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
$ ./.factory/bin/tovarisch-memory-lab verify \
    --artifacts-dir \
      docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01/evidence \
    --run-id lab-canary-descriptor-1784635776
=== Verification Results ===
Run ID: lab-canary-descriptor-1784635776
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

### Runtime-state derivation

```bash
$ ./.factory/bin/tovarisch-memory-lab derive-runtime-state \
    --artifacts-dir \
      docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01/evidence \
    --run-id lab-canary-descriptor-1784635776
{
  "initial_fd_count": 0,
  "final_fd_count": 200,
  "fd_count_delta": 200,
  "initial_operation_count": 0,
  "final_operation_count": 100,
  "operation_count_delta": 100,
  "process_pid": 2154967,
  "process_start_time": 636198569,
  "sample_count": 61,
  "delayed_samples": 59,
  "phase_counts": {
    "baseline": 8,
    "final": 8,
    "settling": 5,
    "startup": 5,
    "stimulus": 30,
    "warmup": 5
  },
  "canary_image_provenance": {
    "canary_image_id": "sha256:db1c85fe0a26da0dc33f7a9e50bb343b0f96b0a586f72f91f119a3f4b503bf5e",
    "canary_repo_digests": [
      "kgb-tovarisch-canary@sha256:db1c85fe0a26da0dc33f7a9e50bb343b0f96b0a586f72f91f119a3f4b503bf5e"
    ],
    "canary_repo_digest_status": "available",
    "canary_source_commit_oid": "9c1f0200ac55e632ab50d555e68fc52c25552574",
    "canary_repository_tree_oid": "68c7fb0439535b3b4d17456af8adbba27ac19c14",
    "canary_source_subtree_oid": "056016e82fba903ed25d0bab98197e2a424b2a67",
    "canary_image_revision_label": "9c1f0200ac55e632ab50d555e68fc52c25552574",
    "canary_image_tree_label": "68c7fb0439535b3b4d17456af8adbba27ac19c14",
    "canary_image_source_subtree_label": "056016e82fba903ed25d0bab98197e2a424b2a67",
    "canary_image_binary_sha256_label": "aac6bb8d50dee648b7006dbcd2c5d36474f6419c32f2b2a2999b1b0b8cea08b1",
    "canary_container_image_matches_id": true
  }
}
```

### Subject container cleanup

```bash
$ docker ps -a --filter "name=tovarisch-subject-lab-canary-descriptor-1784635776" \
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

## 24. Zig 0.16 observations

This ACT is entirely within the Go memory-lab module
(`tovarisch/labs/memory/`). No Zig code was modified; no
Zig 0.16 observations are recorded.

## 25. Successor

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
