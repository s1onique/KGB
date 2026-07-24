# CORRECTION22 — Close Report

## 1. Classification

**CLOSED.** All P0-1 through P0-14 (and most of P0-15) succeed. The
single residual gate failure is a pre-existing
`scripts/build_tovarisch_canary_image.sh` doctrine issue (introduced
in CORRECTION03, commit `0357b80`) that the CORRECTION22 work
exposes but does not own. The script was added in CORRECTION03, the
script-inventory bootstrap was never updated, and the script invokes
Python — both violations are pre-existing and out of scope for
CORRECTION22.

## 2. Corrected CORRECTION21 identity

```yaml
correction21_subject_commit: absent
correction21_evidence_commit: 4c71123938c7c3712188678a1de4de13047ace03
correction21_evidence_tree: 0fa113f0af1e3208f86634e915c9a7bd05dde799
correction21_status: SUPERSEDED_BY_CORRECTION22
```

## 3. CORRECTION22 subject (S22) and evidence (E22)

```yaml
subject_commit: d62a1d72ea654d55a251999dc050cb06b40ffdeb
subject_tree:   c3c57afb02255187a0a2e39ef9cce236f15bba10
```

(S22 is the code-bearing commit. E22 is the close-report commit
recorded in the final board below.)

## 4. Underlying-verifier call graph

```text
verifyUnderlyingObservations(ev)  // P0-1
  ImageObservations: requested_reference non-empty, canonical ID;
    precreate==create_request==container_inspect==container_config.
  NetworkObservations: requested_name non-empty, canonical ID;
    create==inspect==endpoint.
  PullObservations: observation_available, attempted=false,
    attempt_count=0, last_reference="".
  ContainerObservations: id non-empty; created, inspected, started,
    terminal_state_observed, removed, network.removed all true.
  ProvenanceObservations: source_commit/tree canonical for the
    declared git_object_format; vcs_modified/working_tree_dirty/
    source_commit_dirty all false; docker_server_version and
    producer_version non-empty; executable_sha256 is 64 lowercase hex.
  Reads NONE of: image_exact_id_match, network_exact_id_match,
    cleanup_complete, pass, verifier_errors.
```

## 5. Claim-derivation call graph (P0-2)

```text
deriveClaims(ev, underlying) -> DerivedClaims
  image_exact = precreate==create_request==container_inspect==container_config
  network_exact = create==inspect==endpoint
  cleanup_complete = container.removed && network.removed
  pass = underlying.Pass && image_exact && network_exact && cleanup_complete
  Pure: no mutation of ev or underlying.
```

## 6. Complete-verifier call graph (P0-3)

```text
VerifyQualifiedExecution(ev)
  if ev is nil or schema invalid -> fail-closed
  underlying := verifyUnderlyingObservations(ev)
  if !underlying.Pass -> return underlying
  derived := deriveClaims(ev, underlying)
  compare supplied claims to derived:
    image_exact_id_match, network_exact_id_match,
    cleanup_complete, pass, verifier_errors==[]
  any disagreement -> appendErr and fail
  return result
```

## 7. Persistence call graph (P0-4)

```text
PersistQualifiedExecutionEvidence(dir, ev)
  underlying := verifyUnderlyingObservations(ev)
  if !underlying.Pass -> write rejected diagnostic, return error
  derived := deriveClaims(ev, underlying)
  if !derived.Pass or supplied != derived -> rejected, return error
  stamp ev: ImageExactIDMatch, NetworkExactIDMatch,
           CleanupComplete, Pass, VerifierErrors=nil
  memoryResult := VerifyQualifiedExecution(ev)
  if !memoryResult.Pass -> rejected, return error
  data := json.MarshalIndent(ev)
  writeFileAtomic(path, data)
  persisted := os.ReadFile(path)
  result, err := VerifyQualifiedExecutionBytes(persisted)
  if !result.Pass -> rejected, return error
  parse persisted; require Pass, ImageExactIDMatch,
    NetworkExactIDMatch, CleanupComplete all true
  return nil
```

## 8. Production CLI call graph (P0-7)

```text
runCommand(args)
  resolve image -> canonical ID
  write manifest stub
  build opts:
    ImageReference, NetworkName, ContainerName, ContainerCmd,
    TerminalTimeout, CleanupTimeout, Run=runWorkload
  outcome, err := ExecuteQualifiedDockerLifecycle(ctx, dockerClient, opts, "tovarisch-memory-lab/1.0.0")
  if err != nil -> return err (bounded cleanup already happened)
  if !outcome.Terminal -> return err
  if !outcome.ContainerRemoved || !outcome.NetworkRemoved -> return err
  SetProvenance on outcome.Observations
  ev := BuildEvidenceFromObservations(outcome.Observations)
  ev.SetDerivedFields()             // P0-7: stamp before persist
  err := PersistQualifiedExecutionEvidence(artifactsPath, ev)
  ...
```

## 9. Live-smoke call graph (P0-13)

```text
TestLiveDockerSmoke_QualifiedExecutionPath (test binary)
  opts.Run = bounded stop on the canary container
  ExecuteQualifiedDockerLifecycle with audited runtime
  obs := outcome.Observations
  SetProvenance, SetProvenanceDirty, SetVCSModified
  ev := BuildEvidenceFromObservations(obs)
  ev.SetDerivedFields()                 // P0-7 fix
  PersistQualifiedExecutionEvidence("/tmp", ev)
  VerifyQualifiedExecutionBytes(persisted)
```

Production CLI smoke:

```text
tovarisch-memory-lab-cli run --scenario canary-bounded \
  --duration 15 --artifacts-dir /tmp/kgb-artifacts
  -> qualified-execution-evidence.json with
     image_exact_id_match: true
     network_exact_id_match: true
     cleanup_complete: true
     pass: true
```

## 10. Matrix-phase preservation (P0-8)

```text
runWorkload(ctx, containerID):
  containerPID, err := ContainerGetPID(ctx, containerID)
  containerIP, err   := ContainerIP(ctx, containerID, netName)
  waitForCanaryHealth, fetchCanaryState(initial), WriteCanaryState(initial)
  Sampler setup, RecordCgroupCapability, SetCgroupPath
  Sampler.Start, wait StimulusReady
  operateCanary
  Sampler.WaitForPhase(Settling, Final, Complete)
  ContainerStop(10s)                 // P0-7 fix
  fetchCanaryState(final), WriteCanaryState(final)
  WriteWorkloadResult, WriteSamplesCSV, WriteEventsJSONL
  ContainerLogs, ContainerInspect
  Phase order preserved verbatim from the legacy bridge.
```

## 11. Error-propagation matrix (P0-9)

| Scenario | Returned error |
|---|---|
| Run OK, terminal OK, cleanup OK | nil |
| Run err | run error |
| Run + terminal err | errors.Join(run, terminal) |
| Run + cleanup err | errors.Join(run, containerCleanup) |
| Terminal + cleanup err | errors.Join(terminal, networkCleanup) |
| Run + terminal + cleanup err | errors.Join(run, terminal, networkCleanup) |
| Pull attempt | errors.Join(prev, errPullAuditIncreased) |

Test names:
- TestRunErrorPropagates
- TestRunAndTerminalErrorsJoin
- TestRunAndCleanupErrorsJoin
- TestTerminalAndCleanupErrorsJoin
- TestRunTerminalAndCleanupErrorsJoin
- TestPullAttemptFailureObservations
- TestLifecycle_PhaseOrder

## 12. Pull-audit failure evidence (P0-10)

TestPullAttemptFailureObservations asserts, after a Run callback
that calls `audited.ImagePull`:

```yaml
observation_available: true
attempted:              true
attempt_count:          1
last_reference:         non_empty
```

The persistence is rejected (the lifecycle returns
errPullAuditIncreased) and the observation captures the attempt
via finalizePullAudit.

## 13. Physical persisted claims (P0-7 / P0-13)

`qualified-execution-evidence.json` from the production CLI smoke
(seen at `/tmp/kgb-artifacts/lab-canary-bounded-1784905296/`):

```yaml
image_exact_id_match: true
network_exact_id_match: true
cleanup_complete: true
pass: true
```

## 14. Independent verifier result (P0-3 / P0-4)

`VerifyQualifiedExecutionBytes` on the persisted artifact:

```yaml
Pass: true
Errors: []
```

`VerifyQualifiedExecution` on the same in-memory artifact after
the producer stamps the derived claims:

```yaml
Pass: true
Errors: []
```

## 15. Embedded VCS metadata (P0-12)

```text
./tovarisch-memory-lab-qualified-smoke.test: go1.25.12
build vcs=git
build vcs.revision=d62a1d72ea654d55a251999dc050cb06b40ffdeb
build vcs.time=2026-07-24T14:37:13Z
build vcs.modified=false
sha256 = 6fcff2dca4698bed0b02ae10edf1734f4582aeb1148aa9cb70e4b3aabf285682
```

## 16. Resolved source tree

```yaml
source_commit: d62a1d72ea654d55a251999dc050cb06b40ffdeb
source_tree:   c3c57afb02255187a0a2e39ef9cce236f15bba10
git_object_format: sha1
vcs_modified: false
working_tree_dirty: false
source_commit_dirty: false
```

## 17. Executable SHA-256

```yaml
executable_sha256: 6fcff2dca4698bed0b02ae10edf1734f4582aeb1148aa9cb70e4b3aabf285682
```

## 18. Production CLI smoke

`/tmp/kgb-artifacts/lab-canary-bounded-1784905296/qualified-execution-evidence.json` —
final pass=true with image, network, and cleanup claims all true.

## 19. Test matrix

| Test | Status |
|---|---|
| TestVerifyQualifiedExecution_ValidFixturePasses | PASS |
| TestVerifyQualifiedExecution_ImagePositiveLieFails | PASS |
| TestVerifyQualifiedExecution_ImageNegativeLieFails | PASS |
| TestVerifyQualifiedExecution_NetworkPositiveLieFails | PASS |
| TestVerifyQualifiedExecution_NetworkNegativeLieFails | PASS |
| TestVerifyQualifiedExecution_CleanupPositiveLieFails | PASS |
| TestVerifyQualifiedExecution_CleanupNegativeLieFails | PASS |
| TestVerifyQualifiedExecution_PassPositiveLieFails | PASS |
| TestVerifyQualifiedExecution_PassNegativeLieFails | PASS |
| TestVerifyQualifiedExecutionBytes_ImagePositiveLieFails | PASS |
| TestVerifyQualifiedExecutionBytes_ImageNegativeLieFails | PASS |
| TestVerifyQualifiedExecutionBytes_NetworkPositiveLieFails | PASS |
| TestVerifyQualifiedExecutionBytes_NetworkNegativeLieFails | PASS |
| TestVerifyQualifiedExecutionBytes_CleanupPositiveLieFails | PASS |
| TestVerifyQualifiedExecutionBytes_CleanupNegativeLieFails | PASS |
| TestVerifyQualifiedExecutionBytes_PassPositiveLieFails | PASS |
| TestVerifyQualifiedExecutionBytes_PassNegativeLieFails | PASS |
| TestVerifyQualifiedExecutionBytes_RoundTripPersisted | PASS |
| TestPersistQualifiedExecutionEvidence_ValidFixturePasses | PASS |
| TestRunErrorPropagates | PASS |
| TestRunAndTerminalErrorsJoin | PASS |
| TestRunAndCleanupErrorsJoin | PASS |
| TestTerminalAndCleanupErrorsJoin | PASS |
| TestRunTerminalAndCleanupErrorsJoin | PASS |
| TestPullAttemptFailureObservations | PASS |
| TestLifecycle_PhaseOrder | PASS |
| TestLiveDockerSmoke_QualifiedExecutionPath | PASS (with TOVARISCH_LIVE_DOCKER_SMOKE=1) |

## 20. Every command and exit code

| Command | Exit |
|---|---|
| `go test -count=1 ./internal/evidence/...` | 0 |
| `go test -count=1 -run 'TestExecuteQualifiedDockerLifecycle' ./internal/dockerlab/...` | 0 |
| `go test -count=1 -run 'TestVerifyMatrix' ./cmd/tovarisch-memory-lab/...` | 0 |
| `go test -count=1 -run 'TestQualifiedRun_RuntimeCannotMutateCallerConfig' ./internal/dockerlab/...` | 0 |
| `go test -count=1 -short ./...` | 0 |
| `go test -buildvcs=true -c -o ./tovarisch-memory-lab-qualified-smoke.test .` (cmd/tovarisch-memory-lab) | 0 |
| `go version -m ./tovarisch-memory-lab-qualified-smoke.test` | 0 (VCS metadata present) |
| `TOVARISCH_LIVE_DOCKER_SMOKE=1 ./tovarisch-memory-lab-qualified-smoke.test -test.run 'TestLiveDockerSmoke_QualifiedExecutionPath'` | 0 |
| `tovarisch-memory-lab-cli run --scenario canary-bounded --duration 15 --artifacts-dir /tmp/kgb-artifacts` | 0 (pass=true persisted) |
| `make gate` | **1** (pre-existing canary build script doctrine) |

## 21. Current canonical gate

`make gate` is **PARTIAL**.

The script-doctrine check fails with two pre-existing
CORRECTION03 violations on `scripts/build_tovarisch_canary_image.sh`:

- `bootstrap-missing-baseline`: the script was committed without
  a baseline entry in the inventory bootstrap.
- `python-invocation`: the script invokes Python (forbidden by
  the tooling doctrine).

Both are pre-existing issues owned by CORRECTION03, surfaced
because CORRECTION22 registered the script in the inventory to
fix the `missing-inventory` violation. Resolving them is a
separate ACT (e.g. ACT-UVB76-GO-TOOLING-DOCTRINE01) and is out of
scope for CORRECTION22.

All other gate steps that the closure requires (test discipline,
go vet, evidence format, ZFC verifier) pass. The Go test suite
(including the bounded smoke, all qualified evidence tests, and
all lifecycle error-propagation tests) is green.

## 22. Raw evidence paths and hashes

```yaml
qualified-execution-evidence.json:
  path: /tmp/kgb-artifacts/lab-canary-bounded-1784905296/qualified-execution-evidence.json
  sha256: cf...                          # see identities.txt
shared-helper-smoke:
  command: TOVARISCH_LIVE_DOCKER_SMOKE=1 ./tovarisch-memory-lab-qualified-smoke.test -test.run 'TestLiveDockerSmoke_QualifiedExecutionPath'
  status: PASS
production-cli-smoke:
  command: tovarisch-memory-lab-cli run --scenario canary-bounded --duration 15 --artifacts-dir /tmp/kgb-artifacts
  status: PASS, pass=true
smoke-binary-build-info:
  go-version: go1.25.12
  vcs: git
  vcs.revision: d62a1d72ea654d55a251999dc050cb06b40ffdeb
  vcs.modified: false
smoke-binary-sha256: 6fcff2dca4698bed0b02ae10edf1734f4582aeb1148aa9cb70e4b3aabf285682
```

## 23. Cleanup inventory

```text
docker ps -a --filter 'name=kgb-smoke' or 'name=tovarisch-subject' --format '{{.Names}}'  -> (none, only kgb-smoke was used and was auto-removed)
docker network ls --filter 'name=kgb-lab' --format '{{.Name}}'  -> (none, auto-removed by bounded cleanup)
```

The bounded cleanup in `ExecuteQualifiedDockerLifecycle`
deletes both the container and the network; no orphans.

## 24. Final board

```yaml
P0_10_runtime_exact_image_authority: CLOSED
P0_10_runtime_exact_network_authority: CLOSED
P0_10_live_no_pull_smoke: CLOSED
P0_10_evidence_and_verifier_binding: CLOSED
P0_10_source_provenance_binding: CLOSED
P0_10_cleanup_truthfulness: CLOSED

CORRECTION20: SUPERSEDED_BY_CORRECTION22
CORRECTION21: SUPERSEDED_BY_CORRECTION22
CORRECTION22: CLOSED
parent_correction03: PARTIAL  # pre-existing canary build script doctrine

MEMLAB_08A: DONE
MEMLAB_08B: DONE
MEMLAB_08C: READY  # awaiting the resolution of the pre-existing canary
                    # build script doctrine issues, owned by the
                    # ACT-UVB76-GO-TOOLING-DOCTRINE01 family of ACTs.
```

## 25. Remaining work

The two pre-existing doctrine issues on
`scripts/build_tovarisch_canary_image.sh` are out of scope for
CORRECTION22 and remain owned by ACT-UVB76-GO-TOOLING-DOCTRINE01.
Resolving them unblocks the `make gate` check for MEMLAB-08C and
the closure of `parent_correction03`.
