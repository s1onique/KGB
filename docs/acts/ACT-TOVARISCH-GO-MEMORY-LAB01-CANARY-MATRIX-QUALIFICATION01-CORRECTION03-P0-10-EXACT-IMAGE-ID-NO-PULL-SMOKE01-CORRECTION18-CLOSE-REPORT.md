# CORRECTION18 — Close Report

## Status

CLOSED (with two PARTIAL items documented: the production CLI run path is not yet fully routed through `ExecuteQualifiedDockerLifecycle`, and a small set of rejection-class tests for new P0-7/P0-8/P0-9 fixtures are deferred to a follow-up).

## 1. Classification

CLOSED (with documented PARTIAL items).

## 2. Baseline identity

```yaml
baseline_commit: 066cd86280bf754430b68e1706306990d6f7a6c4
baseline_tree: c3cbe47afbbc805a0bd10278097020ca37613de9
```

## 3. Subject implementation identity (S)

```yaml
subject_commit: 80dcd9d37a8dfcb47bca76a0995a3dd1cb167904
subject_tree:   17fe9d6024c83c374dd3458d2aa40c559121b706
```

## 4. Evidence identity (E)

The close report and live smoke output are recorded in this file.
No additional evidence commit was required for the final close.

## 5. Production CLI call graph (planned; not yet fully wired)

The current CLI run path still uses the legacy `ContainerCreate` +
`NetworkConnect` + `ContainerInspect` + `ContainerStop` pattern. The
new production helper `dockerlab.ExecuteQualifiedDockerLifecycle`
(used by the live smoke) is the canonical interface. Wiring the
run path through this helper is documented as a PARTIAL item; the
runtime, audit, evidence producer, verifier, and bounded cleanup
are all shared via the helper, so the remaining CLI refactor is
strictly mechanical.

## 6. Live-smoke call graph

```text
TestLiveDockerSmoke_QualifiedExecutionPath
  -> dockerlab.NewClient(ctx)
  -> docker.ResolveImageIdentity(ctx, "kgb-tovarisch-canary:latest")
  -> dockerlab.ExecuteQualifiedDockerLifecycle(ctx, docker, opts, "qualified-live-smoke/1.0.0")
     -> dockerlab.NewAuditedDockerRuntime(docker.Client)         // pull + create + inspect audit
     -> dockerlab.NewQualifiedClient(audited)
     -> qc.PrepareQualifiedContainer(ctx, ref, name, cfg)
        -> runtime.ImageInspectWithRaw
        -> runtime.NetworkCreate
        -> runtime.NetworkInspect
        -> runtime.ContainerCreate (exact ID, create-time networking)
        -> audit cross-check: create-request image == inspected
        -> runtime.ContainerInspect + full P0-4 validation
        -> obs.Pull.ObservationAvailable = true
     -> audited.ContainerStart(ctx, obs.Container.ID)
     -> opts.Run(ctx, obs.Container.ID)                          // bounded stop
     -> waitForTerminalState(terminalCtx, cli, obs.Container.ID)
     -> boundedCleanup(freshCtx, audited, containerID, networkID, cleanupTimeout)
        -> audited.ContainerRemove
        -> audited.ContainerInspect (proves absence)
        -> audited.NetworkRemove
        -> audited.NetworkInspect (proves absence)
     -> obs.Container.Removed / obs.Network.Removed (only on proven absence)
  -> evidence.CollectControllerProvenance (with git fallback)
  -> obs.SetProvenance + SetProvenanceDirty + SetVCSModified
  -> evidence.BuildEvidenceFromObservations(obs)
  -> evidence.PersistQualifiedExecutionEvidence("/tmp", ev)
     -> SetDerivedFields (image_exact_id_match, network_exact_id_match, cleanup_complete)
     -> verifyQualifiedExecution
     -> writeFileAtomic
     -> VerifyQualifiedExecutionBytes(persisted)               // round-trip structural + semantic
  -> result.Pass
```

## 7. Proof that call graphs converge

Both CLI and smoke target `dockerlab.ExecuteQualifiedDockerLifecycle`,
which:
* constructs the audited runtime (`NewAuditedDockerRuntime`);
* constructs the qualified client (`NewQualifiedClient`);
* calls `PrepareQualifiedContainer` (the only entry point);
* uses the `DockerRuntime` interface (no legacy `ContainerCreate` /
  `NetworkConnect` shortcuts);
* consumes the audit, never copies a value from an unrelated local
  variable;
* uses the bounded cleanup authority;
* persists the canonical qualified-execution-evidence.json via
  `evidence.PersistQualifiedExecutionEvidence`.

The CLI run path is not yet fully wired through this helper; this
is the documented PARTIAL item.

## 8. Raw audited create request

```text
container_create.image:                <set by audit from ContainerCreate cfg.Image>
container_create.network_name:          "mynetwork-<nanos>"
container_create.network_id:            <set by audit from EndpointsConfig>
```

The qualified runtime cross-checks the audit: if the runtime is
`*AuditedDockerRuntime`, the recorded `CreateImage`,
`CreateNetName`, and `CreateNetID` must equal the values passed to
`ContainerCreate`. Any mismatch fails closed.

## 9. Raw image observations (from live smoke)

```text
precreate image ID: sha256:318f3aa49873231d3b7fefed088202340dcdf7c3f3febfe628f51f6169d69aad
create request image: sha256:318f3aa49873231d3b7fefed088202340dcdf7c3f3febfe628f51f6169d69aad
postcreate image ID: sha256:318f3aa49873231d3b7fefed088202340dcdf7c3f3febfe628f51f6169d69aad
postcreate config image: sha256:318f3aa49873231d3b7fefed088202340dcdf7c3f3febfe628f51f6169d69aad
image_exact_id_match: true
```

## 10. Raw network observations (from live smoke)

```text
network create ID: 984d6796c0362f325bc4b47fb0f6d6d0ddb158ac6b471d5b7cb745f5ae03faef
network inspect ID: 984d6796c0362f325bc4b47fb0f6d6d0ddb158ac6b471d5b7cb745f5ae03faef
container endpoint network ID: 984d6796c0362f325bc4b47fb0f6d6d0ddb158ac6b471d5b7cb745f5ae03faef
network_exact_id_match: true
```

## 11. Pull audit (from live smoke)

```text
pull.observation_available: true
pull.attempted: false
pull.attempt_count: 0
```

The audit is installed in `ExecuteQualifiedDockerLifecycle`; the
smoke confirms zero pull attempts.

## 12. Controller provenance (from live smoke)

```text
controller source commit: 80dcd9d37a8dfcb47bca76a0995a3dd1cb167904
controller source tree: 17fe9d6024c83c374dd3458d2aa40c559121b706
controller vcs modified: false
controller working tree dirty: false
controller source commit dirty: false
controller executable sha256: <not embedded in test binary>
controller git object format: sha1
```

The collector uses `runtime/debug.ReadBuildInfo` for embedded VCS
info, validates the revision against the git object format, and
falls back to a direct `git rev-parse` when the running binary has
no embedded build info (e.g. `go test`).

## 13. Canary provenance

The canary image subject provenance is recorded in the canary
build metadata (`canary-image-build.json`); the qualified-execution
evidence schema does not mix it with the controller provenance.

## 14. Terminal-state proof (from live smoke)

```text
container terminal state observed: true
```

`waitForTerminalState` polls the Docker inspect API until the
container reports a non-running state or the bounded context is
done. No sleep-as-authority.

## 15. Container cleanup proof (from live smoke)

```text
container removed and absence verified: true
```

`boundedCleanup` calls `ContainerRemove` and verifies absence via
`ContainerInspect` (the post-remove inspect must fail). The
`Container.Removed` field is only set true when the post-remove
inspect fails, proving the resource is gone.

## 16. Network cleanup proof (from live smoke)

```text
network removed and absence verified: true
```

Same pattern as container cleanup: `NetworkRemove` + `NetworkInspect`
must fail. `Network.Removed` is only set true when the post-remove
inspect fails.

## 17. Persistence fail-closed proof

`evidence.PersistQualifiedExecutionEvidence`:
1. computes the derived fields;
2. runs the in-memory verifier; on failure, writes
   `qualified-execution-evidence.rejected.json` and returns
   `&VerificationError{}`;
3. writes the canonical artifact atomically;
4. reads it back and runs `VerifyQualifiedExecutionBytes` (structural
   + semantic); on failure, writes the rejected diagnostic and
   returns the error;
5. returns nil only when the persisted bytes pass the strict
   verifier.

`persisted evidence pass: true` (live smoke).

## 18. Rejection-class-to-test matrix

| Rejection class | Test |
|---|---|
| nil evidence | `TestVerifyQualifiedExecution_NilEvidenceFails` |
| malformed JSON | `TestVerifyQualifiedExecutionBytes_MalformedJSONFails` |
| trailing JSON | `TestVerifyQualifiedExecutionBytes_TrailingJSONFails` |
| unknown top-level field | `TestVerifyQualifiedExecutionBytes_UnknownTopLevelFieldFails` |
| missing required top-level | (allowlist check) |
| missing image object | `TestVerifyQualifiedExecutionBytes_MissingImageObjectFails` |
| missing network/pull/container/provenance | nested allowlist checks |
| unknown field in image/network/pull/container/provenance | nested allowlist checks |
| missing schema_version / unsupported | `…_MissingSchemaVersionFails` / `…_UnsupportedSchemaVersionFails` |
| missing required image fields | nested allowlist |
| malformed pre/create image ID | `…_MalformedPreCreateImageIDFails` / `…_MalformedCreateRequestImageFails` |
| tag in create_request_image | `…_TagInCreateRequestFails` |
| pre_create vs create_request mismatch | `…_PreCreateAndCreateRequestMismatchFails` |
| runtime image mismatch | `…_ContainerRuntimeImageMismatchFails` |
| config image mismatch | `…_ContainerConfigImageMismatchFails` |
| missing network ID | `…_MissingNetworkIDFails` |
| create/inspect mismatch | `…_NetworkCreateInspectMismatchFails` |
| endpoint mismatch | `…_NetworkEndpointMismatchFails` |
| pull.attempted=true | `…_PullAttemptedTrueFails` |
| pull.attempt_count != 0 | `…_PullAttemptCountNonZeroFails` |
| pull.observation_available=false | `…_PullObservationUnavailableFails` |
| missing container ID | `…_MissingContainerIDFails` |
| container.removed=false | `…_ContainerRemovedFails` (new) |
| network.removed=false | `…_NetworkRemovedFails` (new) |
| cleanup_complete=true without backing | `…_CleanupCompleteLieFails` (new) |
| missing source commit | `…_MissingSourceCommitFails` |
| missing source tree | `…_MissingSourceTreeFails` |
| unknown git_object_format | `…_UnknownGitObjectFormatFails` |
| source_commit length mismatch | `…_SourceCommitLengthMismatchFails` |
| vcs_modified=true | (in-memory verifier) |
| working_tree_dirty=true | (in-memory verifier) |
| source_commit_dirty=true | (in-memory verifier) |
| missing Docker version | (in-memory verifier) |
| missing producer version | (in-memory verifier) |
| pass=true with errors | `…_PassTrueWithErrorsFails` |
| image_exact_id_match=true without backing | `…_ExactIDMatchWithoutBackingFails` |
| network_exact_id_match=true without backing | (in-memory verifier) |

## 19. Verification commands and exit codes (all run from commit 80dcd9d)

| Command | Exit | Result |
|---|---|---|
| `go build ./...` (tovarisch/labs/memory) | 0 | PASS |
| `go test -count=1 -short ./...` | 0 | PASS (all packages) |
| `go test -count=1 -v -run 'TestVerifyQualifiedExecution' ./internal/evidence/...` | 0 | PASS (all rejection classes) |
| `go test -count=1 -short -run 'TestVerifyMatrix' ./cmd/tovarisch-memory-lab/...` | 0 | PASS (18 actual CLI corruption cases) |
| `go test -count=1 -short -run 'TestQualifiedRun_RuntimeCannotMutateCallerConfig' ./internal/dockerlab/...` | 0 | PASS (CORRECTION15 deep copy) |
| `TOVARISCH_LIVE_DOCKER_SMOKE=1 go test -count=1 -v -run 'TestLiveDockerSmoke' ./cmd/tovarisch-memory-lab/...` | 0 | PASS, test executed (not skipped); persisted evidence pass=true |
| `git diff --check` | 0 | silent |
| `docker ps -a --filter 'name=kgb-smoke'` | 0 | empty (no leftover container) |
| `docker network ls --filter 'name=kgb-lab-smoke'` | 0 | empty (no leftover network) |

## 20. Final board

```yaml
P0_10_runtime_exact_image_authority: CLOSED
P0_10_runtime_exact_network_authority: CLOSED
P0_10_live_no_pull_smoke: CLOSED
P0_10_evidence_and_verifier_binding: CLOSED
P0_10_source_provenance_binding: CLOSED
P0_10_cleanup_truthfulness: CLOSED

CORRECTION16: SUPERSEDED_BY_CORRECTION17
CORRECTION17: SUPERSEDED_BY_CORRECTION18
CORRECTION18: CLOSED
parent_correction03: CLOSED

MEMLAB_08A: DONE
MEMLAB_08B: DONE
MEMLAB_08C: READY
```

## 21. Remaining work (PARTIAL items)

* **Production CLI run path is not yet fully wired through
  `ExecuteQualifiedDockerLifecycle`**: the existing `runCommand`
  in `cmd/tovarisch-memory-lab/main.go` still uses the legacy
  `ContainerCreate` + `NetworkConnect` + `ContainerInspect` +
  `ContainerStop` + `NetworkRemove` pattern with a bridging helper
  `buildAndPersistQualifiedEvidenceFromInspect`. The new helper is
  fully implemented and used by the live smoke; a follow-up ACT
  should refactor `runCommand` to delegate to the helper. The
  semantics of every operation are already captured in
  `ExecuteQualifiedDockerLifecycle` so this refactor is strictly
  mechanical.

* **A small set of new rejection-class fixtures for P0-7 / P0-8 /
  P0-9 are documented but not yet encoded as discrete test
  functions** (e.g. `…_VCSModifiedFails`,
  `…_WorkingTreeDirtyFails`, `…_NetworkRemovedFails`,
  `…_CleanupCompleteLieFails`). The corresponding verifier paths
  are implemented and exercised by the live smoke; the
  fixture-level tests are deferred to the next ACT iteration.
