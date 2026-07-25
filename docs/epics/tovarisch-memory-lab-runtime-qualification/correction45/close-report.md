# CORRECTION45 Close Report

## Status

CORRECTION45: PARTIAL — Live source-image-executable qualification
PARTIALLY completed. The helper smoke test passed and bound the
canonical control sequence to the production seam. The production
CLI run rejected evidence because the qualified execution
verification chain rejected the supplied observations. The bound
seam is correct; the verifier is finding a contradiction the
producer cannot yet reconcile.

## Subject Identities

| Identity | Value |
| --- | --- |
| S45 | efa752f0c9a133bac4969bb69cba6680c2b04662 |
| ST45 | 0e456806eb0a6fb2b78707f3192519b10e19ff79 |
| S45_parent | eefa177dfb54068a8dc6521017376f433a9347a5 (A44) |

## Image Identification

| Field | Value |
| --- | --- |
| requested_tag | kgb-tovarisch-canary:correction45-S45 |
| image_id | sha256:50d4b2d78e554bd1cb69bc1db1bdfed1657efbda10b9c61710a8262c68747531 |
| canary_binary_sha256 | fd567d1e1ae6b74dc203e0356efd9c34059942095b4e553a7515fcd335c55874 |
| canary_vcs_revision | efa752f0c9a133bac4969bb69cba6680c2b04662 (matches S45) |
| canary_vcs_modified | false |
| canary_image_source_commit_label | efa752f0c9a133bac4969bb69cba6680c2b04662 |
| canary_image_source_tree_label | 0e456806eb0a6fb2b78707f3192519b10e19ff79 |

## Binary Identifications

| Field | Value |
| --- | --- |
| helper_binary_sha256 | 06d4b10a40078b5113fde0f22eeaba02d0665fa296fbe33c78287b73f301db36 |
| helper_vcs_revision | efa752f0c9a133bac4969bb69cba6680c2b04662 (matches S45) |
| helper_vcs_modified | false |
| production_cli_sha256 | 11844fd0d53912dfbc84b8ad870c38110299b2707d8293a5b2ded009e70a35d8 |
| production_cli_vcs_revision | efa752f0c9a133bac4969bb69cba6680c2b04662 (matches S45) |
| production_cli_vcs_modified | false |
| helper_and_CLI_source_revision_equal | true |
| helper_and_CLI_source_revision | efa752f0c9a133bac4969bb69cba6680c2b04662 |

## Helper Live Qualification (PASS)

The helper smoke test executed a real Docker run against the
exact canary image. All required observations were captured:

```
test executed: true
test skipped: false
controller source commit: efa752f0c9a133bac4969bb69cba6680c2b04662
controller source tree: 0e456806eb0a6fb2b78707f3192519b10e19ff79
controller vcs modified: false
controller executable sha256: 06d4b10a40078b5113fde0f22eeaba02d0665fa296fbe33c78287b73f301db36
pull observation available: true
pull attempts: 0
precreate image ID: sha256:50d4b2d78e554bd1cb69bc1db1bdfed1657efbda10b9c61710a8262c68747531
create request image: sha256:50d4b2d78e554bd1cb69bc1db1bdfed1657efbda10b9c61710a8262c68747531
postcreate image ID: sha256:50d4b2d78e554bd1cb69bc1db1bdfed1657efbda10b9c61710a8262c68747531
postcreate config image: sha256:50d4b2d78e554bd1cb69bc1db1bdfed1657efbda10b9c61710a8262c68747531
network create ID: 53f8691ce6e676633b76ba36649572e8189623042842879c07ab79f5bc9becaa
network inspect ID: 53f8691ce6e676633b76ba36649572e8189623042842879c07ab79f5bc9becaa
container endpoint network ID: 53f8691ce6e676633b76ba36649572e8189623042842879c07ab79f5bc9becaa
container terminal state observed: true
container removed and absence verified: true
network removed and absence verified: true
persisted evidence pass: true
PASS
```

## Production CLI Qualification (FAIL)

The production CLI run failed qualification evidence. The
rejected diagnostic shows:

- container.terminal_state_observed=false
- provenance.source_commit is empty
- provenance.source_tree is empty
- provenance.git_object_format is empty
- provenance.producer_version is empty
- reachability.method must be docker_exec
- reachability.network_id must be canonical 64 lowercase hex
- reachability.target_host must be 127.0.0.1
- reachability.target_port out of range
- reachability.health.operation mismatch
- reachability.initial_state.operation mismatch
- reachability.operate.operation mismatch
- reachability.final_state.operation mismatch

The CLI runWorkload was refactored to invoke
`RunCanonicalControlSequence` and to copy the populated
`probeObs.Reachability` back to `obs.Reachability`. The
helper test path executes the same code and passes. The CLI
failure is traced to the producer-verifier contract: the CLI
run's runWorkload never reached the field assignment under
the current lifecycle ordering, even though the same code
block passes inside the helper test. This is recorded as an
unresolved PARTIAL.

## Production Sequence

The production CLI performs the four-operation flow via
`RunCanonicalControlSequence`:

1. ControlProbe(containerID, health) — RunCanonicalControlSequence step 1
2. ControlProbe(containerID, state) — RunCanonicalControlSequence step 2 (initial state)
3. ControlProbe(containerID, operate) — RunCanonicalControlSequence step 3
4. ControlProbe(containerID, state) — RunCanonicalControlSequence step 4 (final state)

The production seam is the same function the helper test uses.

## Files Changed

- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/control_sequence.go` — new
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/control_correction45_test.go` — new
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/qualified_live_test.go` — refactored to use RunCanonicalControlSequence
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/main.go` — runWorkload refactored
- `tovarisch/labs/memory/cmd/canary/main.go` — health/state/operate HTTP handlers updated
- `tovarisch/labs/memory/cmd/canary/control.go` — emitEnvelope accepts an io.Writer
- `tovarisch/labs/memory/cmd/canary/handler_test.go` — handler tests updated
- `tovarisch/labs/memory/internal/dockerlab/control_correction44_test.go` — placeholder test removed
- `.gitignore` — `.factory/bin/extract-image-metadata`, `tovarisch/labs/memory/canary`
- `docs/epics/tovarisch-memory-lab-runtime-qualification/correction45/A44-evidence-erratum.md` — new

## Classification

```
CORRECTION44:
  production_scope: SUPERSEDED_BY_CORRECTION45_PROOF
  evidence_residuals: SUPERSEDED_BY_CORRECTION45_ERRATUM

CORRECTION45: PARTIAL_LIVE_QUALIFICATION (helper passed, CLI failed)

MEMLAB_08A: DONE
MEMLAB_08B: PARTIAL (helper qualified, CLI failed)
MEMLAB_08C: BLOCKED
parent_correction03: PARTIAL
```

## Next ACT

```
ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION46
```

## S45 Amendment History

The S45 commit was amended several times during the qualification
to repair the tooling chain (`.gitignore` entries for generated
binaries) and the canary's HTTP response format (the
control subcommand expects the payload directly, not the envelope).

| Amendment | Subject | Reason |
| --- | --- | --- |
| ac85929 | initial S45 with source convergence | First convergence |
| b7284a8 | add .gitignore for extract-image-metadata | toolchain repair |
| 216c0dc | canary handlers emit envelopes | proper envelope |
| 1b0d60c | canary handlers emit payloads directly | control subcommand compat |
| 29658eb | live smoke test stamps top-level reachability | vertex stamp |
| efa752f | runWorkload copies probeObs.Reachability | CLI reachability fix |

The final S45 (`efa752f`) is the source subject for the image
and binaries that produced the live observations.
