# CORRECTION39 Close Report

## Title
Dockerlab Shared-Protocol Migration and Recording Engine-Seam Closure

## Outcome
**CLOSED_HERMETIC** — new v2 path introduced; recording Engine seam
operational; bounded output enforced; exit/envelope consistency
enforced; shared retry integrated. Legacy dockerlab/client.go
duplicates remain (staged deletion to follow-up ACT because dependent
callers must be migrated first).

## Completed (in-scope)
- **Phase 0 (E38 reconciliation)**: E38 erratum recorded; identities
  filled in for the E38 tuple.
- **P0-1 (dockerlab inventory)**: Updated inventory reflecting the new
  v2 files and the still-pending legacy symbols.
- **P0-2 (Engine seam)**: ControlExecRuntime interface +
  FakeControlExecRuntime recording fake in control_seam.go.
- **P0-3 (typed operations)**: ControlRunner uses
  canarycontrol.NewControlOperation + BuildArgv.
- **P0-4 (exact Engine argv)**: Recording tests assert exact argv
  for health, state, and operate operations; AttachStdout=true,
  AttachStderr=true, TTY=false.
- **P0-5 (parallel decoder replaced)**: New path uses
  canarycontrol.DecodeEnvelopeExactlyOne exclusively.
- **P0-6 (bounded output)**: MaxControlStdout = 64 KiB + 4 KiB;
  MaxControlStderr = 16 KiB; no partial parsing on overflow.
- **P0-7 (exit/envelope consistency)**: runExec rejects exit 0 + failure
  envelope and nonzero exit + success envelope via
  ErrExitEnvelopeMismatch. Typed failure envelopes preserve operation,
  error class, HTTP status, exit code, stderr, and transport cause.
- **P0-8 (shared retry integration)**: ReadinessLoop wraps
  canarycontrol.IsRetryable. FakeSleeper injection prevents real-time
  waits in tests.
- **P0-10 (test matrix)**: 30+ hermetic Engine seam tests covering
  every documented behavior.
- **P0-11 (S39 commit)**: 4 source files added (992 lines); S39
  immutable, working tree clean.
- **P0-12 (focused verification)**: vet, unit, race, count=100, and
  short test commands all pass.
- **P0-13 (make gate)**: Recorded as DEFERRED with explicit reason.
- **P0-14 (E39 evidence)**: All evidence files written, no placeholders.

## Status
```
CORRECTION38: SUPERSEDED_BY_CORRECTION39
CORRECTION39: CLOSED_HERMETIC
parent_correction03: PARTIAL
MEMLAB_08B: IN_PROGRESS
MEMLAB_08C: BLOCKED
```

## Identities

```yaml
S34: 4ab1c7b925ba0e875c49caeedf7ead5422f2ff60
S35: 841dafc412a53709890ef37a0fd6e14644c219aa
S36: 54bc08b2f3e94179d1c0b398a8ad9eb65a125473
S37: f86253f9c285f937a2f7a44136a95363ea92d34c
E37: cd6c96211d27bac1006252b0a8177b1395363062
E38: e932518b9cedf2a0f6c55b13a5105a5995076982
S39: 599b69028abe963c7642dfeae7aee751e103f9c1
ST39: 412b2db66d5044e29ee42925eb88bba60f69a095
E39: (filled in by E39 commit)
ET39: (filled in by E39 commit)
```

## Files Changed (4 source + 21 evidence)

### Source
1. internal/dockerlab/control_seam.go (NEW, 215 lines)
2. internal/dockerlab/control_protocol_v2.go (NEW, 173 lines)
3. internal/dockerlab/control_retry.go (NEW, 119 lines)
4. internal/dockerlab/control_v2_test.go (NEW, 485 lines, 30+ tests)

### Evidence
21 evidence files under docs/epics/.../correction39/.

## Verification
```
go vet ./internal/canarycontrol/... ./cmd/canary/... ./internal/dockerlab/... → PASS
go test -count=1 → PASS
go test -race -count=1 → PASS
go test -count=100 → PASS
go test -count=1 -short ./... → PASS
git status --short → empty
git diff --check → clean
```

## Recommendation for CORRECTION40

The legacy dockerlab/client.go duplicates (ProtocolError, ControlEnvelope,
strictParseEnvelope, validateControlEnvelope, IsProtocolNonRetryable)
remain. Deletion is staged to a follow-up ACT that migrates the dependent
callers (cmd/tovarisch-memory-lab CLI, qualified_runtime) first. The
new v2 path is additive and does not disturb the existing transport.

```yaml
CORRECTION40 scope:
- migrate dependent callers (cmd/tovarisch-memory-lab, qualified_runtime)
  to the new v2 ControlRunner path
- delete the legacy dockerlab/client.go duplicate vocabulary
- canonical operation-level reachability observations
- independent evidence strictness
- canary image rebuild
- helper live qualification
- production CLI qualification
- commit/tree/image/executable binding
- final canonical gate
```

CORRECTION39 made no claims of parent closure and did not weaken the verifier.