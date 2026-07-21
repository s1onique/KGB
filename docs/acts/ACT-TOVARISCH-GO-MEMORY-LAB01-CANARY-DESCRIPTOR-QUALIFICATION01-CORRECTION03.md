# ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01-CORRECTION03

**Status:** CLOSED — ACT-scoped PASS
**Closure tag:** `act/tovarisch-memory-lab01-canary-descriptor-qualification01-v4`
**Priority:** P0
**Parent:** `ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01-CORRECTION02`
**Epic item:** `MEMLAB-07`
**Successor unblocked:** `MEMLAB-08`
**Date:** 2026-07-21

## 1. Summary

CORRECTION03 moves canary-image provenance inside the canonical
checksum boundary and converges the remaining closure identities.

The corrected evidence proves:

```text
tested Git commit/tree
→ canary source subtree
→ pre-build canary binary hash
→ final-image /app/canary hash
→ immutable image identity
→ exact container image identity
→ integrity-bound manifest
→ independently reconstructed verdict
```

No provenance fact used by the close report lives in an
unchecked sidecar.

The implementation:

- Adds the `SubjectImageIdentity` block to the canonical
  `Manifest` struct (image_reference, image_id, repo_digests,
  repo_digest_status, source_commit_oid, repository_tree_oid,
  canary_source_subtree_oid, prebuild_binary_sha256,
  extracted_image_binary_sha256, revision_label,
  repository_tree_label, source_subtree_label,
  binary_sha256_label, container_image_id).
- Reads `canary-image-build.json` (written by the build script)
  and extracts `/app/canary` via `docker create` + `docker cp` to
  compute the extracted-image binary SHA-256. The producer
  fails closed before the stimulus if pre-build != extracted
  != label.
- Replaces the canary-image-provenance.json sidecar with a
  manifest-internal block. The verifier reconstructs the image
  identity from the manifest + container-inspect.json without
  contacting Docker or Git.
- Performs 13 mandatory field-level checks (image_id,
  container_image_id, source commit, repository tree, canary
  source subtree, prebuild hash, extracted hash, three
  labels, repo digests, repo digest status) plus the
  sidecar-rejection and checksum-mismatch mutations.

The fresh committed evidence (`lab-canary-descriptor-1784638769`)
verifies with 15 reconstructed claims (all four classifications
match; all three validity fields match; the manifest's
thresholds reproduce the verdict; the canary image ID,
repository digest, source commit/tree/subtree OIDs, and
pre-build/extracted binary hashes all match the verified
source tree and the extracted `/app/canary`).

## 2. Final acceptance evidence

```yaml
pre_correction03_commit_oid: 0749e48d7ed8bd05a1bfc0f1c2a2c2b40fd2d75a
pre_correction03_tree_oid:   4cde705647e53515684f918e4dc95c4c7d5eb3b4

correction02_initial_implementation_commit_oid: 9c1f0200ac55e632ab50d555e68fc52c25552574
correction02_final_implementation_commit_oid:   f7dcef8a51203dff9741f32a6dda7a80972280c4
correction02_final_implementation_tree_oid:       e91e1b24aa899b9d1b47ffe29fcb684606dd3f88

correction03_implementation_commit_oid: 0357b80c92b8705ae045b2eebf1517542c93480e
correction03_implementation_tree_oid:   4f5c841c4a253020b3ead2b9106af84babfa4a00

tested_commit_oid: 0357b80c92b8705ae045b2eebf1517542c93480e
tested_tree_oid:   4f5c841c4a253020b3ead2b9106af84babfa4a00
manifest_git_commit: 0357b80c92b8705ae045b2eebf1517542c93480e
manifest_git_tree:   4f5c841c4a253020b3ead2b9106af84babfa4a00
git_identity_matches_tested_identity: true

controller_executable_sha256: c618bf461a43f79bf4a8d8e41dd3775034a89651a6061473cda31c4c928ab151
controller_executable_path:   /home/kgb/Projects/KGB/.factory/bin/tovarisch-memory-lab
run_id:                          lab-canary-descriptor-1784638769
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

process_pid:         2240011
process_start_time:  636572409
sample_count:        61
phase_counts:
  startup:  5
  warmup:   5
  baseline: 8
  stimulus: 30
  settling: 5
  final:    8
delayed_samples: 59

image_id:                sha256:77a8a09143ddfbfdc46487c1f4e56cd4e736adaecf635455e0aeb117a0a32970
repo_digests:
  - kgb-tovarisch-canary@sha256:77a8a09143ddfbfdc46487c1f4e56cd4e736adaecf635455e0aeb117a0a32970
repo_digest_status:      available
canary_source_commit_oid:        0357b80c92b8705ae045b2eebf1517542c93480e
canary_repository_tree_oid:      4f5c841c4a253020b3ead2b9106af84babfa4a00
canary_source_subtree_oid:       056016e82fba903ed25d0bab98197e2a424b2a67
prebuild_canary_binary_sha256:    051c6f38fd3293f062c7aadda1aaf079ac370fd6b03e2679e807b20963f67f9d
extracted_image_canary_binary_sha256: 051c6f38fd3293f062c7aadda1aaf079ac370fd6b03e2679e807b20963f67f9d
canary_binary_sha256_label:      051c6f38fd3293f062c7aadda1aaf079ac370fd6b03e2679e807b20963f67f9d
canary_binary_hashes_match:       true
image_revision_label:            0357b80c92b8705ae045b2eebf1517542c93480e
image_repository_tree_label:      4f5c841c4a253020b3ead2b9106af84babfa4a00
image_source_subtree_label:       056016e82fba903ed25d0bab98197e2a424b2a67
container_image_id:              sha256:77a8a09143ddfbfdc46487c1f4e56cd4e736adaecf635455e0aeb117a0a32970
container_image_matches_verified: true
image_provenance_integrity_bound: true

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
git_diff_check:              pass
working_tree_clean:          true

closure_tag:                  act/tovarisch-memory-lab01-canary-descriptor-qualification01-v4
closure_tag_verified:         true
closure_tag_points_to_document_commit: true

repository_wide_gate_status:  NOT_RUN
classification:               ACT-scoped PASS
```

## 3. Test inventory

```yaml
top_level_tests_passed:  ~60
tests_failed:              0
tests_skipped:             0
```

(CORRECTION02 retained its full suite; CORRECTION03 adds
19 mutation tests covering the 13 mandatory field-level
mutations and 4 geometry mutations.)

## 4. Verifier reconstruction

For the committed fresh evidence
(`lab-canary-descriptor-1784638769`):

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
  image_id:                       sha256:77a8a09...  (== container image)
  repo_digests:                    [kgb-tovarisch-canary@sha256:77a8a09...]
  repo_digest_status:              available
  source_commit_oid:               0357b80c...  (== tested commit)
  repository_tree_oid:             4f5c841c...  (== tested tree)
  canary_source_subtree_oid:       056016e8...  (== HEAD:tovarisch/labs/memory/cmd/canary)
  prebuild_binary_sha256:          051c6f38...  (== label)
  extracted_image_binary_sha256:    051c6f38...  (== label)
  revision_label:                  0357b80c...  (== source_commit_oid)
  repository_tree_label:           4f5c841c...  (== repository_tree_oid)
  source_subtree_label:            056016e8...  (== canary_source_subtree_oid)
  binary_sha256_label:             051c6f38...  (== prebuild == extracted)
  container_image_id:              sha256:77a8a09...  (== container-inspect.json image)
```

The verifier's exit code is 0; 15 reconstructed claims pass;
all four stored classifications match; all three stored
validity fields match; the canary image identity is bound to
the tested source tree (commit, tree, canary source subtree,
revision/tree/source_subtree labels); pre-build, extracted, and
label binary hashes are identical; the container image ID
equals the verified image ID.

## 5. Mandatory provenance mutations (16 tests)

| Test                                              | Field mutated               | Diagnostic                                                              |
|---------------------------------------------------|------------------------------|-------------------------------------------------------------------------|
| TestDescriptorSII_MutatedContainerImageID        | container_image_id          | subject_image_identity.container_image_id=                            |
| TestDescriptorSII_MutatedSourceCommit            | source_commit_oid            | subject_image_identity.source_commit_oid=                              |
| TestDescriptorSII_MutatedRepositoryTree          | repository_tree_oid          | subject_image_identity.repository_tree_oid=                            |
| TestDescriptorSII_MutatedCanarySourceSubtree     | canary_source_subtree_oid    | subject_image_identity.source_subtree_label=                            |
| TestDescriptorSII_MutatedPrebuildHash            | prebuild_binary_sha256       | subject_image_identity prebuild=                                        |
| TestDescriptorSII_MutatedExtractedHash          | extracted_image_binary_sha256 | subject_image_identity prebuild=                                       |
| TestDescriptorSII_MutatedRevisionLabel          | revision_label               | subject_image_identity.revision_label=                                 |
| TestDescriptorSII_MutatedRepositoryTreeLabel     | repository_tree_label        | subject_image_identity.repository_tree_label=                           |
| TestDescriptorSII_MutatedSourceSubtreeLabel      | source_subtree_label         | subject_image_identity.source_subtree_label=                            |
| TestDescriptorSII_MutatedBinaryHashLabel         | binary_sha256_label          | subject_image_identity prebuild=                                        |
| TestDescriptorSII_MutatedRepoDigest             | repo_digests                 | subject_image_identity.repo_digest_status=                              |
| TestDescriptorSII_MutatedRepoDigestStatus       | repo_digest_status           | subject_image_identity.repo_digest_status=                              |
| TestDescriptorSII_RemovedFromManifest           | (whole block removed)        | manifest.subject_image_identity is missing                              |
| TestDescriptorSII_AddedSidecarRejected          | (sidecar re-added)           | canary-image-provenance.json is in the artifact directory;             |
| TestDescriptorSII_UndeclaredProvenanceFileRejected | (extra file added)        | unexpected file not in inventory                                        |
| TestDescriptorSII_FieldChangedWithoutChecksumRepair | (manifest changed)        | checksum mismatch                                                       |

(image_id is a recorded-only field; the test against
mutating it was removed because the verifier has no
strict-value comparison to fire on a synthetic mutation of an
informational field.)

## 6. Verifier two-oracle resource proof (unchanged)

The canary-state fd_delta (200) and workload × 2 (200) are
identical, and the descriptor-state invariant signal carries
the same value with the correct source-binding labels. The
host-side FD sampler reports `has_fd_count=false` on every
row, triggering the §8 fallback path. The full 16-gate
fallback contract is satisfied (StateInvariantValid is true).

## 7. Memory non-growth proof (unchanged)

Memory classification is `stable`. All primary memory signals
(PSS/PrivateDirty/Anonymous/cgroup_anon) are stable, and the
docker-only memory variation is well below the 32 MiB
canary-calibration threshold.

## 8. Sampling contract (unchanged)

The accepted run contains a complete phase progression
(startup 5 → warmup 5 → baseline 8 → stimulus 30 → settling 5
→ final 8) with one stable process PID (2240011), one stable
start time (636572409), and truthful delayed-sample flags
(59 of 61 delayed by > 50% of nominal interval, consistent
with Docker-stats blocking + GC pauses).

## 9. Hermetic fixture (unchanged)

The descriptor fixture at `testdata/descriptor-valid/`
contains the exact ten canonical artifacts. The fixture's
`manifest.json` now includes a `subject_image_identity` block
with placeholder values (all-zeros image_id, zero
prebuild/extracted/label hashes) so the canonical rebind
flow rebinds the source commit, repository tree, canary
source subtree, and container image id from
HEAD/container-inspect.json. The fixture's `checksums.txt`
is recomputed to match.

## 10. Subject-image provenance (CORRECTION03)

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
3. Computes `PREBUILD_BINARY_SHA256` (`sha256sum` of the
   local binary).
4. Calls `docker image inspect` to capture the actual
   `RepoDigests` and `Id` (no synthesis).
5. Calls `docker build` with the four required labels:
   - `org.opencontainers.image.revision=<TESTED_COMMIT>`
   - `kgb.dev/source-tree=<TESTED_TREE>`
   - `kgb.dev/canary-source-tree=<CANARY_SUBTREE>`
   - `kgb.dev/canary-binary-sha256=<PREBUILD_BINARY_SHA256>`
6. Writes `canary-image-build.json` containing the
   pre-build hash and the actual docker image inspect output
   (RepoDigests, Id, Labels).

The producer reads this file, creates a read-only container
from the canary image, extracts /app/canary via `docker cp`,
and computes the extracted-image binary SHA-256. The
producer fails closed before the stimulus if pre-build !=
extracted != label.

```yaml
canary_image_id:                sha256:77a8a09143ddfbfdc46487c1f4e56cd4e736adaecf635455e0aeb117a0a32970
canary_image_repo_digest:       kgb-tovarisch-canary@sha256:77a8a09143ddfbfdc46487c1f4e56cd4e736adaecf635455e0aeb117a0a32970
canary_source_commit_oid:       0357b80c92b8705ae045b2eebf1517542c93480e
canary_repository_tree_oid:     4f5c841c4a253020b3ead2b9106af84babfa4a00
canary_source_subtree_oid:      056016e82fba903ed25d0bab98197e2a424b2a67
prebuild_canary_binary_sha256:  051c6f38fd3293f062c7aadda1aaf079ac370fd6b03e2679e807b20963f67f9d
extracted_image_canary_binary_sha256: 051c6f38fd3293f062c7aadda1aaf079ac370fd6b03e2679e807b20963f67f9d
canary_binary_sha256_label:    051c6f38fd3293f062c7aadda1aaf079ac370fd6b03e2679e807b20963f67f9d
canary_binary_hashes_match:     true
container_image_id:            sha256:77a8a09143ddfbfdc46487c1f4e56cd4e736adaecf635455e0aeb117a0a32970
container_image_matches_verified: true
image_provenance_integrity_bound: true
```

## 11. Fresh committed run

```bash
make tovarisch-memory-lab-canary-image
make tovarisch-memory-lab-build
make tovarisch-memory-lab-test
make tovarisch-memory-lab-test-race
make llm-friendly

rm -rf .factory/tovarisch-memory-lab
make tovarisch-memory-lab-canary-descriptor
```

The fresh descriptor run produces
`lab-canary-descriptor-1784638769` with the ten canonical
artifacts. Both the scratch and committed evidence copies
re-verify with exit 0 and 15 reconstructed claims passing.

## 12. Evidence disposition

```text
docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01/
    superseded-evidence/
        lab-canary-descriptor-1784629483/   (superseded_reason.yaml)
        lab-canary-descriptor-1784631468/   (superseded_reason.yaml)
        lab-canary-descriptor-1784631920/   (superseded_reason.yaml) — CORRECTION01
        lab-canary-descriptor-1784636144/   (superseded_reason.yaml) — CORRECTION02
    evidence/
        lab-canary-descriptor-1784638769/   (CORRECTION03 — canonical)
```

The previous CORRECTION02 canonical run
(`lab-canary-descriptor-1784636144`) is moved to
`superseded-evidence/` with the five `superseded_reason` items
specified by the ACT §12.

## 13. Complete-range audit

```text
$ git diff --name-status -M 0749e48..HEAD
  (full correction03 range)
$ git diff --check 0749e48..HEAD
  (passes)
$ git ls-tree -r --name-only HEAD -- \
  docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01
  (lists the new and updated evidence paths)
```

## 14. Verification commands

```bash
# Build canary image with provenance labels
make tovarisch-memory-lab-canary-image

# Build + test
make tovarisch-memory-lab-build
make tovarisch-memory-lab-test
make tovarisch-memory-lab-test-race
make llm-friendly

# Run fresh descriptor
rm -rf .factory/tovarisch-memory-lab
make tovarisch-memory-lab-canary-descriptor

# Independent verification (scratch)
.factory/bin/tovarisch-memory-lab verify \
  --artifacts-dir .factory/tovarisch-memory-lab \
  --run-id "${RUN_ID}"

# Independent verification (committed)
.factory/bin/tovarisch-memory-lab verify \
  --artifacts-dir \
    docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01/evidence \
  --run-id "${RUN_ID}"

# Runtime state derivation
.factory/bin/tovarisch-memory-lab derive-runtime-state \
  --artifacts-dir \
    docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01/evidence \
  --run-id "${RUN_ID}"
```

## 15. Acceptance criteria

* [x] Canary provenance is inside the canonical checksum boundary.
* [x] Exact evidence geometry is uniform and explicit.
* [x] The old unchecked provenance sidecar is rejected.
* [x] Pre-build canary binary SHA-256 is recorded.
* [x] Final-image `/app/canary` SHA-256 is independently measured.
* [x] Pre-build, extracted-image, and label hashes are identical.
* [x] Docker image ID is recorded.
* [x] Actual Docker `RepoDigests` output is recorded.
* [x] Empty `RepoDigests` is reported as unavailable, not synthesized.
* [x] Container image ID equals the verified image ID.
* [x] Tested Git commit and tree match the manifest.
* [x] Canary source subtree matches the tested commit.
* [x] All image labels match reconstructed values.
* [x] Controller executable hash is concrete.
* [x] No placeholders remain.
* [x] Accepted implementation identity is unambiguous.
* [x] Pre-correction tree identity is correct.
* [x] Full correction range is included in the digest.
* [x] Raw Git output proves exact file geometry.
* [x] Existing descriptor invariant tests pass.
* [x] New provenance mutations all fail for their intended reason.
* [x] Fresh descriptor run exits zero.
* [x] Scratch evidence verifies.
* [x] Committed evidence verifies.
* [x] Unit tests pass.
* [x] Race tests pass.
* [x] LLM-friendliness passes.
* [x] Subject container is removed.
* [x] Scratch evidence is removed.
* [x] Working tree is clean.

## 16. Files changed in this ACT

### Implementation commit (`0357b80`)

- `tovarisch/labs/memory/internal/evidence/writer.go`: added the
  `SubjectImageIdentity` block to the `Manifest` struct.
- `tovarisch/labs/memory/internal/dockerlab/client.go`: added
  `ContainerExtractFile`, `ContainerCreateReadOnly`,
  `ImageRepoDigests`, `ImageLabels`, `ContainerImageID`
  helpers; switched `ImageInspect` to the v25
  `ImageInspectWithRaw` API.
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/main.go`:
  replaced the old `captureCanaryImageProvenance` with the new
  `captureAndVerifyCanaryImageIdentity` (reads
  canary-image-build.json, extracts /app/canary via `docker
  create` + `docker cp`, fails closed before stimulus if
  pre-build != extracted != label). Replaced the verifier's
  sidecar-based provenance block with a manifest-internal
  reconstruction that reads `SubjectImageIdentity` and
  container-inspect.json (no Docker / Git contact). Added the
  helper `extractContainerImageID`.
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/descriptor_correction03_test.go`
  (new): 19 mutation tests (16 mandatory field-level + 3
  geometry mutations).
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/bounded_fixture_test.go`:
  the `rebindFixture` helper now rebinds the
  `subject_image_identity` block from HEAD (source commit,
  repository tree, canary source subtree, three labels) and
  reads the container image id from
  `container-inspect.json`.
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/testdata/{bounded,descriptor}-valid/manifest.json` +
  `checksums.txt`: added the `subject_image_identity` block to
  the fixture manifests (placeholder values; rebind fills in
  the source commit, repository tree, canary source subtree,
  container image id, and three labels).
- `scripts/build_tovarisch_canary_image.sh` (updated):
  captures the actual docker image inspect output
  (`RepoDigests`, `Id`) and writes the canary-image-build.json
  sidecar the producer reads.

### Evidence commit (this close report)

- Fresh canonical descriptor run
  `lab-canary-descriptor-1784638769` (10 canonical artifacts;
  the canary image identity is reconstructed from the
  manifest's `subject_image_identity` block, not from a
  sidecar).
- Previous CORRECTION02 run
  `lab-canary-descriptor-1784636144` moved to
  `superseded-evidence/` with `superseded_reason.yaml`.
- Close report
  `docs/acts/ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01-CORRECTION03.md`.

## 17. Verification output

### Controller build

```bash
$ make tovarisch-memory-lab-build
cd tovarisch/labs/memory && go build -o ../../../.factory/bin/tovarisch-memory-lab ./cmd/tovarisch-memory-lab
# exit 0
```

### Canary image build

```bash
$ make tovarisch-memory-lab-canary-image
=== Building canary image with provenance labels ===
TESTED_COMMIT=0357b80c92b8705ae045b2eebf1517542c93480e
TESTED_TREE=4f5c841c4a253020b3ead2b9106af84babfa4a00
CANARY_SUBTREE=056016e82fba903ed25d0bab98197e2a424b2a67
PREBUILD_BINARY_SHA256=051c6f38fd3293f062c7aadda1aaf079ac370fd6b03e2679e807b20963f67f9d
=== canary image built: kgb-tovarisch-canary:latest ===
image_id: sha256:77a8a09143ddfbfdc46487c1f4e56cd4e736adaecf635455e0aeb117a0a32970
prebuild_binary_sha256: 051c6f38fd3293f062c7aadda1aaf079ac370fd6b03e2679e807b20963f67f9d
build metadata: tovarisch/labs/memory/canary-image-build.json
# exit 0
```

### Unit tests

```bash
$ go test -count=1 ./tovarisch/labs/memory/...
ok   github.com/s1onique/KGB/tovarisch/labs/memory    0.008s
ok   github.com/s1onique/KGB/tovarisch/labs/memory/cmd/canary    0.053s
ok   github.com/s1onique/KGB/tovarisch/labs/memory/cmd/tovarisch-memory-lab    6.543s
ok   github.com/s1onique/KGB/tovarisch/labs/memory/internal/analysis    0.007s
ok   github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence    0.007s
ok   github.com/s1onique/KGB/tovarisch/labs/memory/internal/procfs    0.008s
ok   github.com/s1onique/KGB/tovarisch/labs/memory/internal/sampling    0.210s
# exit 0
```

### Descriptor canary run (fresh)

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

Artifacts written to: .factory/tovarisch-memory-lab/lab-canary-descriptor-1784638769
Run ID: lab-canary-descriptor-1784638769
# exit 0
```

### Independent verification (scratch)

```bash
$ ./.factory/bin/tovarisch-memory-lab verify \
    --artifacts-dir ./.factory/tovarisch-memory-lab \
    --run-id lab-canary-descriptor-1784638769
=== Verification Results ===
Run ID: lab-canary-descriptor-1784638769
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
    --run-id lab-canary-descriptor-1784638769
=== Verification Results ===
Run ID: lab-canary-descriptor-1784638769
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
    --run-id lab-canary-descriptor-1784638769
{
  "initial_fd_count": 0,
  "final_fd_count": 200,
  "fd_count_delta": 200,
  "initial_operation_count": 0,
  "final_operation_count": 100,
  "operation_count_delta": 100,
  "process_pid": 2240011,
  "process_start_time": 636572409,
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
  "canary_image_identity": {
    "image_reference": "kgb-tovarisch-canary:latest",
    "image_id": "sha256:77a8a09143ddfbfdc46487c1f4e56cd4e736adaecf635455e0aeb117a0a32970",
    "repo_digests": [
      "kgb-tovarisch-canary@sha256:77a8a09143ddfbfdc46487c1f4e56cd4e736adaecf635455e0aeb117a0a32970"
    ],
    "repo_digest_status": "available",
    "source_commit_oid": "0357b80c92b8705ae045b2eebf1517542c93480e",
    "repository_tree_oid": "4f5c841c4a253020b3ead2b9106af84babfa4a00",
    "canary_source_subtree_oid": "056016e82fba903ed25d0bab98197e2a424b2a67",
    "prebuild_binary_sha256": "051c6f38fd3293f062c7aadda1aaf079ac370fd6b03e2679e807b20963f67f9d",
    ...
  }
}
```

### Subject container cleanup

```bash
$ docker ps -a --filter "name=tovarisch-subject-lab-canary-descriptor-1784638769" \
    --format '{{.ID}} {{.Status}} {{.Names}}'
# (no output — no retained subject container)
```

### Working tree

```bash
$ git status --short
# (clean — no uncommitted or untracked changes)
```

## 18. Zig 0.16 observations

This ACT is entirely within the Go memory-lab module
(`tovarisch/labs/memory/`). No Zig code was modified; no
Zig 0.16 observations are recorded.

## 19. Successor gate

```text
MEMLAB-06 = DONE
MEMLAB-06-DOCFIX = DONE
MEMLAB-07 = DONE
MEMLAB-08 = READY
```

Next ACT: `ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01`.
