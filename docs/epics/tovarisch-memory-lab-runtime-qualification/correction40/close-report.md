# CORRECTION40 Close Report

## Title
Production Docker-Control Migration, Legacy Authority Deletion and
Canonical Reachability

## Outcome
**PARTIAL** — CORRECTION40's bounded migration scope could not be
completed in this session because the full dockerlab source migration
(deletion of legacy vocabulary + caller migration + reachability
implementation + evidence hardening) exceeds a single bounded ACT.
S40 captures the structural v2 refactor (container ID threading,
streaming, attempt timeout, typed failure, ControlFailureError).
The remaining legacy deletion, caller migration, reachability
implementation, and evidence hardening are deferred to CORRECTION41.

## Completed (in-scope)

- **Phase 0 (E39/A39 reconciliation)**: E38/A39 erratum recorded with
  complete identity verification. E39/A39 NOT rewritten.
- **P0-1 (streaming redesign)**: ControlExecAttachment interface with
  bounded Reader. boundedReader retains at most limit bytes plus
  overflow sentinel. No unbounded backing buffer.
- **P0-2 (production adapter shell)**: ProductionControlExecRuntime
  defined with compile-time assertion `var _ ControlExecRuntime =
  (*ProductionControlExecRuntime)(nil)`. Full Docker SDK wiring
  deferred to CORRECTION41.
- **P0-3 (container ID threading)**: ControlProbe requires non-empty
  containerID. Every ExecCreate/ExecAttach/ExecInspect record carries
  the same container ID and exec ID. ErrContainerIDRequired returned
  on empty/whitespace input.
- **P0-4 (bounded streaming)**: boundedReader enforces
  MaxControlStdout and MaxControlStderr with overflow sentinel. Both
  readers reject overflow distinctly.
- **P0-5 (host-side attempt timeout)**: attemptCtx = context.WithTimeout
  (parent, timeout) wraps every Engine call. ErrControlTimeout
  distinguished from caller-cancellation and transport failures.
- **P0-6 (operation identity binding)**: env.Operation must match
  op.Kind. ErrWrongOperation returned otherwise.
- **P0-7 (typed failure errors)**: ControlFailureError preserves
  operation, exit code, HTTP status, bounded stderr, transport cause,
  and shared error class via *canarycontrol.ProtocolError. errors.As to
  *ProtocolError succeeds. err != nil for failure envelopes.
- **P0-8 (readiness integration)**: TestEngineSeam_TypedFailureEnvelope_
  Preserved verifies errors.As, Protocol.ErrClass, exit code, env.

## Deferred (out-of-scope; belongs to CORRECTION41)

- **P0-2 (full Docker SDK wiring)**: Real implementation of
  ProductionControlExecRuntime requires Docker SDK type imports.
- **P0-9 (caller migration)**: cmd/tovarisch-memory-lab, qualified_runtime,
  live-smoke helper, and any dockerlab internal wrappers still use the
  legacy path.
- **P0-10 (legacy deletion)**: dockerlab.ProtocolError, dockerlab.
  ErrorClass, dockerlab.ControlEnvelope, strictParseEnvelope,
  validateControlEnvelope, IsProtocolNonRetryable, and legacy argv
  construction remain in client.go.
- **P0-11 (canonical reachability)**: ReachabilityOperationObservation
  and ReachabilityObservations structs are not yet declared.
- **P0-12 (evidence strictness)**: The qualified-evidence verifier is
  not yet updated to require the new reachability shape.
- **P0-13 (evidence mutation tests)**: Reachability field deletion/null/
  wrong-type tests are not yet added.
- **P0-16 (full hermetic verification with race + count=100 across
  all affected packages)**: Deferred.

## Status
```
CORRECTION39: SUPERSEDED_BY_CORRECTION40
CORRECTION40: PARTIAL
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
E39: 4ae9e8f09cc26c15b1b946302aca93cdc3bfe0cb
A39: bed700fd955c5b952bbdfe9e989c51f8b1a12399
S40: e0d0aecb2c65227f5c4b63bbdda5f1954a8176be
ST40: d000905f4bcee19ca67207144386a2873eb71e16
E40: (filled in by E40 commit)
ET40: (filled in by E40 commit)
```

## Files Changed (S40)

3 source files modified:
- internal/dockerlab/control_seam.go
- internal/dockerlab/control_protocol_v2.go
- internal/dockerlab/control_v2_test.go

## Verification
```
go vet ./internal/canarycontrol/... ./cmd/canary/... ./internal/dockerlab/... → PASS
go test -count=1 ./internal/dockerlab/... → PASS
git status --short → empty
git diff --check → clean
```

## Recommendation for CORRECTION41

```yaml
CORRECTION41 scope:
- wire ProductionControlExecRuntime to the actual Docker SDK
- migrate cmd/tovarisch-memory-lab, qualified_runtime, and all
  dockerlab internal wrappers to the new v2 ControlRunner path
- delete the legacy dockerlab/client.go duplicate vocabulary
  (ProtocolError, ErrorClass, ControlEnvelope, strictParseEnvelope,
  validateControlEnvelope, IsProtocolNonRetryable)
- add ReachabilityOperationObservation and ReachabilityObservations
  to qualified_runtime
- update evidence verifier to require new reachability shape
- add evidence mutation tests
- rebuild canary image from S41
- VCS-stamped helper and CLI binaries
- live health/state/operate/state
- source/image/binary binding
- persisted qualified evidence
- cleanup proof
- final green canonical gate
- MEMLAB_08B closure
```

CORRECTION40 made no claims of parent closure and did not weaken the verifier.