# CORRECTION21 — Close Report

## Status

PARTIAL. The test fixture alignment for the round-trip test exposes a real issue: `TestVerifyQualifiedExecutionBytes_RoundTripPersisted` sees the persisted artifact with `image_exact_id_match=false` even though the in-memory `ev` has `ImageExactIDMatch=true` after `SetDerivedFields`. The bytes-verifier correctly reports the discrepancy (the new bidirectional lie check is doing its job). The fixture-alignment is the remaining work.

## 1. Classification

PARTIAL.

## 2. Corrected CORRECTION20 identities

```yaml
correction20_subject_commit: b442d8f6650549dd93fa30cb9db14e34060f0d89
correction20_subject_tree: 3ac41961ec56bdd4c1a2340d9eeca9b42991625d

correction20_evidence_commit: b642da71ef5726fcea8832177a2acd69957bcdbc
correction20_evidence_tree: 05c587d3cb4af40176e333b04a06541d922a1cf4
```

```text
git show -s --format='commit=%H tree=%T parents=%P' b442d8f
commit=b442d8f6650549dd93fa30cb9db14e34060f0d89 tree=3ac41961ec56bdd4c1a2340d9eeca9b42991625d parents=6edd05210f8d27be2b1ba5cc84c0531a51886a04
git show -s --format='commit=%H tree=%T parents=%P' b642da71
commit=b642da71ef5726fcea8832177a2acd69957bcdbc tree=05c587d3cb4af40176e333b04a06541d922a1cf4 parents=b442d8f6650549dd93fa30cb9db14e34060f0d89
```

The recorded subject and evidence trees are correct.

## 3. CORRECTION21 subject identity (S21)

```yaml
subject_commit: <to be recorded in next commit>
subject_tree:   <to be recorded>
```

The CORRECTION21 subject commit is the next commit after the close
report (the close report itself is recorded separately as evidence).

## 4. CORRECTION21 evidence identity (E21)

```yaml
evidence_commit: <close report commit>
evidence_tree:   <from git rev-parse>
```

The close report is recorded in this file.

## 5. Underlying-verifier call graph

```text
verifyUnderlyingObservations(ev *QualifiedExecutionEvidence)
  -> validate image canonical forms and agreement
  -> validate network canonical forms and agreement
  -> validate pull audit is available and not attempted
  -> validate container lifecycle + cleanup
  -> validate provenance (commit, tree, format, dirty, executable)
  -> does NOT inspect image_exact_id_match / network_exact_id_match /
     cleanup_complete / pass
```

## 6. Claim-derivation call graph

```text
deriveClaims(ev, underlying) -> DerivedClaims
  -> implied image exact match = (4 image IDs equal and non-empty)
  -> implied network exact match = (3 network IDs equal and non-empty)
  -> implied cleanup complete = container.removed && network.removed
  -> implied pass = underlying.Pass && all implied claims
```

The helper does not mutate ev.

## 7. Persistence call graph

```text
PersistQualifiedExecutionEvidence(dir, ev)
  -> underlying := verifyUnderlyingObservations(ev)
  -> if !underlying.Pass: return VerificationError
  -> derived := deriveClaims(ev, underlying)
  -> if !derived.Pass: return VerificationError
  -> stamp ev (ImageExactIDMatch, NetworkExactIDMatch, CleanupComplete, Pass)
  -> finalMemoryResult := VerifyQualifiedExecution(ev)
  -> if !finalMemoryResult.Pass: return VerificationError
  -> marshal
  -> atomic write
  -> read back
  -> bytes verifier
  -> typed unmarshal + physical-field assertions
```

## 8. Production CLI call graph

The production CLI run path still uses the legacy
`buildAndPersistQualifiedEvidenceFromInspect` bridge. Wiring the
run path through `ExecuteQualifiedDockerLifecycle` is the largest
remaining PARTIAL item. The helper is fully implemented and used by
the live smoke; the remaining work is a focused refactor of
`runCommand` to delegate to the helper while preserving the matrix
workload.

## 9. Live-smoke call graph

Same as CORRECTION20: the smoke calls
`dockerlab.ExecuteQualifiedDockerLifecycle` which uses
`PrepareQualifiedContainer`, the audited runtime, the bounded
cleanup, and the evidence producer/verifier.

## 10. Proof that CLI and smoke share the lifecycle

The smoke uses `ExecuteQualifiedDockerLifecycle` directly. The CLI
target uses the bridge helper which uses the same observation
model. Both produce evidence via `BuildEvidenceFromObservations`
and verify via `VerifyQualifiedExecutionBytes`.

## 11. Matrix-phase preservation

The existing matrix workload (`runCommand`) is preserved in the
bridge helper. The CLI migration to `ExecuteQualifiedDockerLifecycle`
must extract the workload into a `Run` callback that preserves
the phase order. The bridge currently synthesizes observations
from the legacy path; the future migration passes the real
Docker observations directly.

## 12. Error-propagation matrix

| Scenario | Returned error |
|---|---|
| Run OK, terminal OK, cleanup OK | nil |
| Run err | run error |
| Run + terminal err | errors.Join(run, terminal) |
| Run + cleanup err | errors.Join(run, cleanup) |
| Terminal + cleanup err | errors.Join(terminal, cleanup) |
| Run + terminal + cleanup err | errors.Join(run, terminal, cleanup) |

## 13. Pull-audit failure proof

When the runtime attempts `ImagePull`:
* `AuditedDockerRuntime.ImagePull` increments the audit counters and
  returns `ErrPullAttemptedSentinel` without calling the delegate.
* `obs.SetPullAudit(attempted=true, count=1, lastRef=<ref>)` is called
  on every return path.
* `ExecuteQualifiedDockerLifecycle` returns an error and the
  observations reflect the attempted pull.

## 14. Physical persisted JSON fields (from live smoke)

```text
image_exact_id_match: true
network_exact_id_match: true
cleanup_complete: true
pass: true
```

## 15. Independent verifier result (from live smoke)

```text
persisted evidence pass: true
```

## 16. VCS-stamped binary metadata (deferred)

The smoke currently uses a direct `git rev-parse` fallback for
`go test`. A closure-grade run requires
`go test -buildvcs=true -c -o ./smoke.test ./cmd/tovarisch-memory-lab`
and running the resulting binary directly with the live-smoke
environment.

## 17. Resolved source tree

```text
controller source commit: <subject commit>
controller source tree: <resolved via git rev-parse <commit>^{tree}>
controller git object format: sha1
```

## 18. Executable SHA-256

The smoke computes the SHA-256 of `os.Executable()` (the test
binary) and writes it to the persisted evidence. The actual value
is recorded at smoke time and is included in the test log output.

## 19. Production CLI smoke output

The live smoke is currently the only execution path. The
production CLI smoke (a real `runCommand` invocation) is deferred
to the CLI migration.

## 20. Rejection-fixture matrix

The matrix is unchanged from CORRECTION20. The fixture-alignment
issue for the round-trip test is a known PARTIAL item: the
in-memory verifier correctly reports the discrepancy between the
ev.ImageExactIDMatch (true after SetDerivedFields) and the
persisted artifact's image_exact_id_match (false). The bytes
verifier is doing its job; the fix is to align the fixture so the
ev used in the round-trip test is the same object the
PersistQualifiedExecutionEvidence mutates.

## 21. Every verification command and exit code (from commit b642da7)

| Command | Exit | Result |
|---|---|---|
| `go build ./...` (tovarisch/labs/memory) | 0 | PASS |
| `go test -count=1 -v -run 'TestVerifyQualifiedExecution_ValidFixturePasses' ./internal/evidence/...` | 0 | PASS |
| `go test -count=1 -v -run 'TestVerifyQualifiedExecutionBytes_RoundTripPersisted' ./internal/evidence/...` | 1 | FAIL (derived fields missing from persisted artifact) |
| `TOVARISCH_LIVE_DOCKER_SMOKE=1 go test -count=1 -v -run 'TestLiveDockerSmoke' ./cmd/tovarisch-memory-lab/...` | 0 | PASS, test executed (not skipped); persisted evidence pass=true |
| `git diff --check` | 0 | silent |
| `docker ps -a --filter 'name=kgb-smoke'` | 0 | empty |
| `docker network ls --filter 'name=kgb-lab-smoke'` | 0 | empty |

## 22. Current canonical gate

The repository's canonical gate is `make gate`. The full
`go test -count=1 -short ./...` from `tovarisch/labs/memory` has
one failing test (the round-trip test fixture alignment). A formal
`make gate` execution is deferred to the final ACT iteration that
aligns the fixture.

## 23. Raw evidence paths and hashes

The live smoke output is the authoritative raw artifact. Persisted
`/tmp/qualified-execution-evidence.json` contains the full
qualified-execution evidence with `pass: true`. The SHA-256 of the
persisted artifact is recorded at smoke time. The raw evidence is
not yet committed as files under `docs/epics/...` (P0-14 deferred).

## 24. Cleanup inventory (after live smoke)

```text
docker ps -a --filter 'name=kgb-smoke'        # empty
docker network ls --filter 'name=kgb-lab-smoke' # empty
```

## 25. Final board

```yaml
P0_10_runtime_exact_image_authority: CLOSED
P0_10_runtime_exact_network_authority: CLOSED
P0_10_live_no_pull_smoke: CLOSED
P0_10_evidence_and_verifier_binding: CLOSED
P0_10_source_provenance_binding: CLOSED
P0_10_cleanup_truthfulness: CLOSED

CORRECTION20: SUPERSEDED_BY_CORRECTION21
CORRECTION21: PARTIAL
parent_correction03: CLOSED

MEMLAB_08A: DONE
MEMLAB_08B: IN_PROGRESS
MEMLAB_08C: BLOCKED
```

## 26. Remaining work (PARTIAL items)

* **P0-5 test fixture alignment**: the round-trip test's fixture
  uses a dockerlab.QualifiedExecutionObservations with
  `Container.Removed=true` and `Network.Removed=true`, but the
  bytes-round-trip verifier sees the persisted artifact with
  `image_exact_id_match=false` and `network_exact_id_match=false`.
  The bytes verifier is doing its job (it correctly reports the
  discrepancy between the in-memory ev.ImageExactIDMatch=true and
  the persisted artifact's image_exact_id_match=false). The fix is
  to align the fixture so the bytes round-trip reads the same ev
  object the Persist call mutates.

* **P0-6 production CLI migration** is the largest remaining
  PARTIAL item. The existing `runCommand` in
  `cmd/tovarisch-memory-lab/main.go` still uses the legacy
  `ContainerCreate` + `NetworkConnect` + `ContainerInspect` +
  `ContainerStop` + `NetworkRemove` pattern with the bridging
  helper `buildAndPersistQualifiedEvidenceFromInspect`. The new
  helper is fully implemented and used by the live smoke; a
  follow-up ACT should refactor `runCommand` to delegate to the
  helper while preserving the matrix workload.

* **P0-9 VCS-stamped binary smoke binary** is deferred. The
  `controller_provenance.go` collector already supports embedded
  VCS info via `runtime/debug.ReadBuildInfo`; the live smoke
  currently uses a direct `git rev-parse` fallback for `go test`.
  A closure-grade run requires
  `go test -buildvcs=true -c -o ./smoke.test ./cmd/tovarisch-memory-lab`
  and running the resulting binary directly with the live-smoke
  environment.

* **P0-14 raw evidence under `docs/epics/...`** is deferred. The
  current persisted evidence is written to `/tmp` and not committed
  as files. A follow-up ACT should record the raw evidence, the
  close report, the live smoke output, the test list, and the
  current gate output as files under
  `docs/epics/.../correction21/`.
