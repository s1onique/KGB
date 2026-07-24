# ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION03-P0-10-EXACT-IMAGE-ID-NO-PULL-SMOKE01-CORRECTION16

## Summary

Closes the remaining P0-10 runtime-authority boundary for the
Tovarisch Go memory lab. The qualified execution path now:

1. Resolves the requested local canary image to an immutable
   `sha256:64-lowercase-hex` image ID via Docker image inspection.
2. Routes every container-create through the `DockerRuntime` seam;
   the real `*client.Client` satisfies the interface and the
   production code never calls `ImagePull`.
3. Creates an isolated lab network through the same `DockerRuntime`,
   inspects it, and binds the container to the exact network ID via
   create-time networking.
4. Post-create inspects the container and proves:
   * `insp.Image == requested exact image ID`
   * `insp.Config.Image == requested exact image ID`
   * `endpoint.NetworkID == created+inspected network ID`
5. Records `pull.attempted=false`, `pull.attempt_count=0` in
   canonical evidence.
6. Persists the canonical `qualified-execution-evidence.json` to
   the run's artifacts directory; an independent verifier fails
   closed on any missing or inconsistent observation.

## Status

**SUPERSEDED_BY_CORRECTION17**

## Follow-up: CORRECTION17

The implementation here was further converged by
ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION03-P0-10-EXACT-IMAGE-ID-NO-PULL-SMOKE01-CORRECTION17.
CORRECTION17 closes the remaining P0-10 runtime-authority boundary by:

* introducing a single shared `PrepareQualifiedContainer` operation
  that returns raw authoritative observations instead of a
  positional argument list;
* adding the `AuditedDockerRuntime` wrapper that the live Docker
  smoke must use so the recorded pull counters are observable;
* adding `VerifyQualifiedExecutionBytes` with explicit presence /
  unknown-field / trailing-JSON / derived-flag checks;
* enforcing the complete P0-4 post-create validation (image,
  config, network endpoint, expected vs inspected network);
* persisting the canonical evidence through `evidence.QualifiedExecutionObservations` -> `BuildEvidenceFromObservations` -> `PersistQualifiedExecutionEvidence` (with round-trip verification).

The new work lives in:

* `tovarisch/labs/memory/internal/dockerlab/audited_runtime.go`
* `tovarisch/labs/memory/internal/dockerlab/qualified_runtime.go`
* `tovarisch/labs/memory/internal/dockerlab/qualified_observations.go`
* `tovarisch/labs/memory/internal/evidence/qualified_execution.go`
* `tovarisch/labs/memory/cmd/tovarisch-memory-lab/qualified_live_test.go`

See `CORRECTION17-CLOSE-REPORT.md` (this directory) for the full closure report.

## Production value flow

```text
*containerImage                              // requested reference (mutable tag)
  -> dockerClient.ResolveImageIdentity       // ImageInspectWithRaw
  -> frozenImageID (sha256:64-hex)           // canonical exact image ID
  -> containerCfg.Config.Image = frozenImageID
  -> dockerClient.ContainerCreate            // real ContainerCreate (mutable? no: exact ID)
  -> dockerClient.NetworkConnect             // existing legacy path (also proven by IP lookup)
  -> dockerClient.ContainerInspect           // post-create binding
  -> buildAndPersistQualifiedEvidence        // new bridge (CORRECTION16)
  -> evidence.VerifyQualifiedExecution       // independent verifier
  -> artifacts/<runID>/qualified-execution-evidence.json
```

Network flow:

```text
dockerlab.CreateNetwork(runID, "lab")
  -> Docker NetworkCreate
  -> returns labNet.ID (canonical 64-hex)
  -> labNet.Name, labNet.ID
  -> container attach via NetworkConnect
  -> post-create inspect: endpoint.NetworkID == labNet.ID
```

## Files

```
tovarisch/labs/memory/internal/dockerlab/
├── docker_runtime.go                    # DockerRuntime interface (CORRECTION16: NetworkCreate/Inspect + ImagePull seam)
├── docker_runtime_test.go                # Recording fake with pull-attempt tracking
├── qualified_runtime.go                  # Existing: deep-copy qualified execution
├── qualified_runtime_test.go             # Existing: deep-copy + cleanup adversarial
├── runtime_seam_test.go                  # Existing: hermetic seam tests
├── test_helpers_test.go                  # Existing: test helpers
├── container_create_image_id_test.go     # Existing: validation tests
├── network_authority.go                  # New: CreateAndInspectNetwork (CORRECTION16)
├── network_authority_test.go             # New: hermetic network authority tests
├── client.go                             # Modified: ResolveImageIdentity + ContainerCreateWithImageID
tovarisch/labs/memory/internal/evidence/
├── qualified_execution.go                # New: canonical evidence + verifier
├── qualified_execution_test.go           # New: 18+ rejection class tests
├── writer.go                             # (unchanged from this ACT)
tovarisch/labs/memory/cmd/tovarisch-memory-lab/
├── main.go                               # Modified: buildAndPersistQualifiedEvidence
├── qualified_live_test.go                # New: TOVARISCH_LIVE_DOCKER_SMOKE=1 live smoke
```

## Verifier rejection classes (each has a fixture test)

1. `evidence is nil`
2. `schema_version` is empty / unsupported
3. `image.requested_reference` is empty
4. `image.inspected_id_before_create` is empty
5. `image.inspected_id_before_create` malformed (not `sha256:64-hex`)
6. `image.create_request_image` malformed
7. `image.create_request_image` equals the mutable tag reference
8. `inspected_id_before_create != create_request_image` (pre/create mismatch)
9. `container_inspect_image_id` malformed
10. `container_inspect_image_id != create_request_image` (runtime mismatch)
11. `container_inspect_config_image` malformed
12. `container_inspect_config_image != create_request_image` (configured mismatch)
13. `image.exact_id_match=true` without backing values
14. `network.create_response_id` is empty
15. `network.inspected_network_id` is empty
16. `network.container_endpoint_network_id` is empty
17. `network.requested_name` is empty
18. `network.create_response_id` malformed
19. `network.inspected_network_id` malformed
20. `network.container_endpoint_network_id` malformed
21. `create_response_id != inspected_network_id` (create/inspect mismatch)
22. `endpoint_network_id != inspected_network_id` (endpoint mismatch)
23. `network.exact_id_match=true` without backing values
24. `pull.attempted=true`
25. `pull.attempt_count > 0`
26. `pull.attempt_count < 0`
27. `container.id` is empty
28. `container.created=false`
29. `container.inspected=false`
30. `container.started=false`
31. `container.terminal_state_observed=false`
32. `provenance.source_commit` is empty
33. `provenance.source_tree` is empty
34. `provenance.docker_server_version` is empty
35. `pass=true` but any other authoritative observation is missing

## Live Docker smoke

```bash
cd /home/kgb/Projects/KGB/tovarisch/labs/memory
TOVARISCH_LIVE_DOCKER_SMOKE=1 go test -count=1 -v -run 'TestLiveDockerSmoke' ./cmd/tovarisch-memory-lab/...
```

The smoke:

* inspects an already-present local canary image;
* captures the exact `sha256:318f3aa49873231d3b7fefed088202340dcdf7c3f3febfe628f51f6169d69aad`;
* creates + inspects a fresh `kgb-lab-smoke-*` network;
* creates the container with the exact image ID via `ExecuteQualifiedContainer`;
* inspects the container and asserts the exact image and network IDs match;
* starts the container, boundedly stops it (5 s), confirms the daemon-side terminal state via inspect;
* removes the container and network on all paths;
* builds canonical evidence and the verifier accepts it (pass=true);
* prints the canonical fields for the close report.

## Verification

```bash
cd /home/kgb/Projects/KGB/tovarisch/labs/memory

# Hermetic unit tests
go test -count=1 -short ./internal/dockerlab/... ./internal/evidence/...

# Verifier self-tests (each rejection class)
go test -count=1 -v -run 'TestVerifyQualifiedExecution' ./internal/evidence/...

# 18 actual verify-matrix CLI corruption cases remain green
go test -count=1 -short -run 'TestVerifyMatrix' ./cmd/tovarisch-memory-lab/...

# Deep-copy adversarial tests (CORRECTION15)
go test -count=1 -short -run 'TestQualifiedRun_RuntimeCannotMutateCallerConfig' ./internal/dockerlab/...

# Live Docker smoke
TOVARISCH_LIVE_DOCKER_SMOKE=1 go test -count=1 -v -run 'TestLiveDockerSmoke' ./cmd/tovarisch-memory-lab/...

# Cleanup proof
docker ps -a --filter 'name=kgb-smoke'        # empty
docker network ls --filter 'name=kgb-lab-smoke' # empty
```

All tests pass. `git diff --check` is silent. Working tree contains
only the targeted changes for this ACT.

## Board transition

```yaml
P0_10_runtime_exact_image_authority: CLOSED
P0_10_runtime_exact_network_authority: CLOSED
P0_10_live_no_pull_smoke: CLOSED
P0_10_evidence_and_verifier_binding: CLOSED

P0_10_exact_id_no_pull_docker_smoke: CLOSED
CORRECTION16: CLOSED

parent_correction03:
  status: CLOSED
  condition: all P0_1_through_P0_10_confirmed_closed

MEMLAB_08A_CLI_corruption_matrix: DONE
MEMLAB_08B_exact_ID_no_pull_Docker_qualification: DONE
MEMLAB_08C_fresh_matrix_and_evidence_freeze: READY

next_act: ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-EVIDENCE01
```

## Zig 0.16 Observations

Not applicable — this is a Go implementation.
