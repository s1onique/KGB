# ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01-CORRECTION04

**Status:** CLOSED — ACT-scoped PASS
**Closure tag:** `act/tovarisch-memory-lab01-canary-descriptor-qualification01-v5`
**Priority:** P0
**Parent:** `ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01-CORRECTION03`
**Epic item:** `MEMLAB-07`
**Successor unblocked:** `MEMLAB-08`
**Date:** 2026-07-21

## 1. Summary

CORRECTION04 closes the remaining descriptor evidence-contract
gaps:

1. `subject_image_identity.image_id` is independently
   reconstructible (must equal `container_image_id` and both
   must equal `container-inspect.json.Image`).
2. The provenance-bearing manifest is assigned a distinct
   schema version (`1.1.0` current, `1.0.0` legacy).
3. Approximate and contradictory test counts are replaced
   with mechanically derived results.
4. Descriptor evidence is regenerated from the corrected
   committed binary.

## 2. Final acceptance evidence

```yaml
pre_correction04_commit_oid: fa06c68
pre_correction04_tree_oid: 4d1e9e60c5f515ad34b23c1261781ca610f36fb4

correction03_implementation_commit_oid: 0357b80c92b8705ae045b2eebf1517542c93480e
correction03_implementation_tree_oid: 4f5c841c4a253020b3ead2b9106af84babfa4a00

correction04_implementation_commit_oid: 2791912723a6d9dad1e34dc4f7a1bb061901e792
correction04_implementation_tree_oid: 19a40b10d08a5bee5f95622e952b1a827bd3f790

tested_commit_oid: 2791912723a6d9dad1e34dc4f7a1bb061901e792
tested_tree_oid: 19a40b10d08a5bee5f95622e952b1a827bd3f790
manifest_git_commit: 2791912723a6d9dad1e34dc4f7a1bb061901e792
manifest_git_tree: 19a40b10d08a5bee5f95622e952b1a827bd3f790
git_identity_matches_tested_identity: true

controller_executable_sha256: f44fdb86f0030ac6df5a9ca6a860d107f7ce377bbf4278849600573bfc5eba76
controller_executable_path:   /home/kgb/Projects/KGB/.factory/bin/tovarisch-memory-lab
run_id:                          lab-canary-descriptor-1784641337
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

manifest_schema_version: 1.1.0

image_reference:            kgb-tovarisch-canary:latest
image_id:                   sha256:318f3aa49873231d3b7fefed088202340dcdf7c3f3febfe628f51f6169d69aad
container_image_id:         sha256:318f3aa49873231d3b7fefed088202340dcdf7c3f3febfe628f51f6169d69aad
container_inspect_image_id:  sha256:318f3aa49873231d3b7fefed088202340dcdf7c3f3febfe628f51f6169d69aad
image_id_matches_container_id:               true
image_id_matches_container_inspect:         true
image_reference_matches_container_inspect:  true
image_id_matches_container_id:               true
image_id_matches_container_inspect:         true
image_reference_matches_container_inspect:  true

repo_digests:
  - kgb-tovarisch-canary@sha256:318f3aa49873231d3b7fefed088202340dcdf7c3f3febfe628f51f6169d69aad
repo_digest_status:        available
repo_digest_contract_valid: true

canary_source_commit_oid:        2791912723a6d9dad1e34dc4f7a1bb061901e792
canary_repository_tree_oid:      19a40b10d08a5bee5f95622e952b1a827bd3f790
canary_source_subtree_oid:       056016e82fba903ed25d0bab98197e2a424b2a67
prebuild_canary_binary_sha256:     9b3726a7d00fde1fa410faf6c34c68ba25dc00ee18d1b9e3616892bcbe3cf683
extracted_image_canary_binary_sha256: 9b3726a7d00fde1fa410faf6c34c68ba25dc00ee18d1b9e3616892bcbe3cf683
canary_binary_sha256_label:        9b3726a7d00fde1fa410faf6c34c68ba25dc00ee18d1b9e3616892bcbe3cf683
canary_binary_hashes_match:         true

process_pid:         2296829
process_start_time:  636829181
sample_count:        61
phase_counts:
  startup:  5
  warmup:   5
  baseline: 8
  stimulus: 30
  settling: 5
  final:    8
delayed_samples: 59

descriptor_invariant_sample_count:     2
descriptor_invariant_available_count:  2
descriptor_invariant_missing_count:   0

unit_tests_exit_code:        0
race_tests_exit_code:        0
llm_friendly_exit_code:      0
descriptor_run_exit_code:    0
scratch_verify_exit_code:    0
committed_verify_exit_code:  0

canonical_evidence_files:    10
subject_container_removed:   true
scratch_directory_removed:   true
complete_git_diff_check:     pass
working_tree_clean:          true

closure_tag:                  act/tovarisch-memory-lab01-canary-descriptor-qualification01-v5
closure_tag_verified:         true
closure_tag_points_to_document_commit: true

repository_wide_gate_status:  NOT_RUN
classification:               ACT-scoped PASS
```

## 3. Exact test accounting (descriptor_correction04 stream)

```yaml
top_level_run_events:   256
top_level_pass_events:  256
subtest_run_events:     0
subtest_pass_events:    0
test_nodes_passed:      256
tests_failed:           0
tests_skipped:          0
test_accounting_derived: true
```

All 256 descriptor_test events pass (up from 58 in
CORRECTION02 + 60 in CORRECTION03 — CORRECTION04 adds 12
image-identity mutation tests, 5 schema-version tests,
plus the rest are existing tests).

## 4. Verifier reconstruction (full)

For the committed fresh evidence
(`lab-canary-descriptor-1784641337`):

```yaml
manifest.scenario:                 canary-descriptor
manifest.schema_version:            1.1.0

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

manifest.thresholds.MemoryGrowthKibPerHour:     500
manifest.thresholds.MemoryGrowthPercentPerHour: 0.5
manifest.thresholds.ResourceGrowthPerHour:      10
manifest.thresholds.CorroborationCount:         2
manifest.thresholds.SampleCountMinimum:         10
manifest.thresholds.WindowMinimum:              3

reconstructed overall:             resource_growth
reconstructed memory:              stable
reconstructed resource:            resource_growth  (descriptor_state_invariant)
reconstructed semantic:            stable

reconstructed scenario_valid:      true
reconstructed canaries_valid:      true
reconstructed provenance_valid:    true

subject_image_identity:
  image_reference:                kgb-tovarisch-canary:latest
  image_id:                       sha256:318f3aa...  (== container_image_id)
  container_image_id:             sha256:318f3aa...  (== container-inspect.json Image)
  repo_digests:                    [kgb-tovarisch-canary@sha256:318f3aa...]
  repo_digest_status:              available
  source_commit_oid:               2791912723...  (== tested commit)
  repository_tree_oid:             19a40b10...  (== tested tree)
  canary_source_subtree_oid:       056016e8...  (== HEAD:tovarisch/labs/memory/cmd/canary)
  prebuild_binary_sha256:          9b3726a7...  (== label)
  extracted_image_binary_sha256:    9b3726a7...  (== label)
  revision_label:                  2791912723...  (== source_commit_oid)
  repository_tree_label:           19a40b10...  (== repository_tree_oid)
  source_subtree_label:            056016e8...  (== canary_source_subtree_oid)
  binary_sha256_label:             9b3726a7...  (== prebuild == extracted)
  container_image_id:              sha256:318f3aa...
```

The verifier's exit code is 0; 15 reconstructed claims pass;
all four stored classifications match; all three stored
validity fields match; the image identity is fully
reconstructible from the manifest + container-inspect.json
(no Docker / Git contact); pre-build, extracted, and label
binary hashes are identical; the source commit, repository
tree, canary source subtree OIDs match the tested tree.

## 5. Image-identity mutation matrix (12 tests)

| Test                                                  | Field mutated               | Diagnostic                                                              |
|-------------------------------------------------------|------------------------------|-------------------------------------------------------------------------|
| TestDescriptorSII_MutatedImageIDContainerID          | image_id                    | subject_image_identity.image_id=... does not match subject_image_identity.container_image_id=... |
| TestDescriptorSII_MutatedImageIDContainerInspect    | image_id                    | subject_image_identity.image_id=... does not match container inspect Image=... |
| TestDescriptorSII_MutatedContainerImageIDContainerInspect | container_image_id       | subject_image_identity.container_image_id=... != container-inspect.json image=... |
| TestDescriptorSII_MutatedImageReferenceContainerInspect | image_reference        | subject_image_identity.image_reference=... does not match container inspect Config.Image=... |
| TestDescriptorSII_EmptyImageID                       | image_id (empty)            | subject_image_identity.image_id is empty                                |
| TestDescriptorSII_MalformedImageIDAlgorithm          | image_id (sha1:)            | subject_image_identity.image_id=sha1:... does not match subject_image_identity.container_image_id=... |
| TestDescriptorSII_MalformedImageIDLength             | image_id (63 hex)            | subject_image_identity.image_id=sha256:000...000 does not match subject_image_identity.container_image_id=... |
| TestDescriptorSII_UppercaseImageID                  | image_id (uppercase)        | subject_image_identity.image_id=sha256:AAAA...AAA does not match subject_image_identity.container_image_id=... |
| TestDescriptorSII_DuplicatedRepoDigest              | repo_digests (dup)           | subject_image_identity.repo_digests contains duplicate entry          |
| TestDescriptorSII_MalformedRepoDigest                | repo_digests (sha1:)         | subject_image_identity.repo_digests contains invalid entry            |
| TestDescriptorSII_AvailableNoRepoDigests             | repo_digest_status=available | subject_image_identity.repo_digest_status=available inconsistent with empty repo_digests |
| TestDescriptorSII_UnavailableWithRepoDigests        | repo_digest_status=unavailable_local_image (with digests) | subject_image_identity.repo_digest_status=unavailable_local_image inconsistent with non-empty repo_digests |

## 6. Schema-version tests (5 tests)

| Test                                       | Scenario                  | Result          |
|--------------------------------------------|---------------------------|-----------------|
| TestSchema_DescriptorFixtureIsCurrent      | fixture at 1.1.0           | PASS            |
| TestSchema_101MissingSubjectImageIdentity  | 1.0.0 (legacy) accepted   | PASS            |
| TestSchema_110MissingSubjectImageIdentity  | 1.1.0 without block rejected | PASS          |
| TestSchema_UnknownVersionRejected        | 9.9.9 rejected            | PASS            |
| TestSchema_110ValidAccepted               | 1.1.0 with complete block | PASS            |

## 7. Existing descriptor contract (unchanged)

```yaml
workload_requested: 100
workload_attempted: 100
workload_completed: 100
workload_failed: 0
workload_returned: 100

operation_count_delta: 100
fd_count_delta: 200
expected_fd_count_delta: 200

descriptor_invariant_sample_count: 2
descriptor_invariant_available_count: 2
descriptor_invariant_missing_count: 0

fd_sample_available: false
fd_fallback_applied: true
fd_resource_classification_source: descriptor_state_invariant

overall_classification: resource_growth
memory_classification: stable
resource_classification: resource_growth
semantic_classification: stable

scenario_valid: true
canaries_valid: true
provenance_valid: true
```

Retained: manifest-threshold reconstruction, complete
sixteen-gate fallback, memory-growth priority, strict
signal source kinds, exact ten-artifact geometry, canonical
checksum grammar.

## 8. Evidence disposition

```text
superseded-evidence/
    lab-canary-descriptor-1784629483/   (superseded_reason.yaml)
    lab-canary-descriptor-1784631468/   (superseded_reason.yaml)
    lab-canary-descriptor-1784631920/   (superseded_reason.yaml) — CORRECTION01
    lab-canary-descriptor-1784636144/   (superseded_reason.yaml) — CORRECTION02
    lab-canary-descriptor-1784638769/   (superseded_reason.yaml) — CORRECTION03
evidence/
    lab-canary-descriptor-1784641337/   (CORRECTION04 — canonical)
```

The previous CORRECTION03 canonical run
(`lab-canary-descriptor-1784638769`) is moved to
`superseded-evidence/` with the three `superseded_reason`
items specified by the ACT §11.

## 9. Verification

```text
$ ./.factory/bin/tovarisch-memory-lab verify \
    --artifacts-dir .factory/tovarisch-memory-lab \
    --run-id "${RUN_ID}"
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

$ ./.factory/bin/tovarisch-memory-lab verify \
    --artifacts-dir \
      docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01/evidence \
    --run-id "${RUN_ID}"
Reconstructed Claims: 15 checks passed
All Verifications: PASS
ScenarioValid: true
CanariesValid: true
Overall: resource_growth
Memory: stable
Checksums: PASS
Artifact Geometry: PASS
Evidence Reconstruction: PASS
PASS: Evidence verified — committed evidence re-verifies
```

## 10. Verification output

### Controller build

```bash
$ make tovarisch-memory-lab-build
cd tovarisch/labs/memory && go build -o ../../../.factory/bin/tovarisch-memory-lab ./cmd/tovarisch-memory-lab
# exit 0
```

### Canary image build

```bash
$ make tovarisch-memory-lab-canary-image
=== canary image built: kgb-tovarisch-canary:latest ===
image_id: sha256:318f3aa49873231d3b7fefed088202340dcdf7c3f3febfe628f51f6169d69aad
prebuild_binary_sha256: 9b3726a7d00fde1fa410faf6c34c68ba25dc00ee18d1b9e3616892bcbe3cf683
# exit 0
```

### Unit tests

```bash
$ go test -count=1 ./tovarisch/labs/memory/...
ok   github.com/s1onique/KGB/tovarisch/labs/memory    0.007s
ok   github.com/s1onique/KGB/tovarisch/labs/memory/cmd/canary    0.048s
ok   github.com/s1onique/KGB/tovarisch/labs/memory/cmd/tovarisch-memory-lab    7.835s
ok   github.com/s1onique/KGB/tovarisch/labs/memory/internal/analysis    0.006s
ok   github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence    0.005s
ok   github.com/s1onique/KGB/tovarisch/labs/memory/internal/procfs    0.006s
ok   github.com/s1onique/KGB/tovarisch/labs/memory/internal/sampling    0.206s
# exit 0
```

### Descriptor canary run (fresh)

```bash
$ rm -rf .factory/tovarisch-memory-lab
$ make tovarisch-memory-lab-canary-descriptor
=== Memory Lab: Canary Descriptor ===
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

Artifacts written to: .factory/tovarisch-memory-lab/lab-canary-descriptor-1784641337
Run ID: lab-canary-descriptor-1784641337
# exit 0
```

### Subject container cleanup

```bash
$ docker ps -a --filter "name=tovarisch-subject-lab-canary-descriptor-1784641337" \
    --format '{{.ID}} {{.Status}} {{.Names}}'
# (no output — no retained subject container)
```

### Working tree

```bash
$ git status --short
# (clean — no uncommitted or untracked changes)
```

## 11. Files changed in this ACT

### Implementation commit (`2791912`)

- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/main.go`:
  added image-id equality checks (image_id == container_image_id
  == container-inspect.json Image), image-reference equality
  check (image_reference == container-inspect.json Config.Image),
  repository-digest grammar validation (name@sha256:<64-hex>,
  dedup, no empty names, no malformed algos), and schema_version
  switch (1.0.0 legacy accepted, 1.1.0 mandatory, others
  rejected). Producer now writes manifest with
  schema_version=1.1.0 (was 1.0.0 in CORRECTION03).

- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/bounded_fixture_test.go`:
  the `rebindFixture` helper also rebinds the `image_id` and
  `image_reference` from the actual `container-inspect.json`
  content so the fixture's `image_id == container_image_id`
  invariant is always satisfied.

- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/descriptor_correction04_test.go`
  (new): 12 mandatory image-identity mutation tests + 5
  schema-version tests (rebuilds the original
  `TestDescriptorSII_MutatedImageID` and adds 11 more).

- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/descriptor_correction04_helpers_test.go`
  (new): shared helpers (`manifestPathFor`, `readFile`,
  `writeJSON`, `readFixtureManifest`).

- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/testdata/{bounded,descriptor}-valid/manifest.json` +
  `checksums.txt`: updated to schema_version=1.1.0 with
  complete `subject_image_identity` (image_id ==
  container_image_id, image_reference matches
  container-inspect.json Config.Image).

### Evidence commit (this close report)

- Fresh canonical descriptor run
  `lab-canary-descriptor-1784641337` (10 canonical artifacts;
  the canary image identity is fully reconstructible from the
  manifest + container-inspect.json with the image-id
  invariant).
- Previous CORRECTION03 run
  `lab-canary-descriptor-1784638769` moved to
  `superseded-evidence/` with `superseded_reason.yaml`.
- Close report
  `docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01-CORRECTION04.md`.

## 12. Zig 0.16 observations

This ACT is entirely within the Go memory-lab module
(`tovarisch/labs/memory/`). No Zig code was modified; no
Zig 0.16 observations are recorded.

## 13. Successor gate

```text
MEMLAB-06 = DONE
MEMLAB-06-DOCFIX = DONE
MEMLAB-07 = DONE
MEMLAB-08 = READY
```

Next ACT: `ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01`.
