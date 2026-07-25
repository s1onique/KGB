# CORRECTION38 Close Report

## Title
Dockerlab Shared-Authority Migration and Engine-Seam Qualification
(bounded non-live ACT)

## Outcome
**PARTIAL** — E37 reconciled; dockerlab migration, recording Engine
seam, retry integration, and live execution deferred to CORRECTION39.

## Completed (in-scope)
- **Phase 0 (E37 reconciliation)**: Identities verified for S34..E37;
  E37 close-report noted as partial (missing executable-SHA-256,
  race-transcript, count100-transcript sections); E37 artifact-hashes
  noted as incomplete (placeholder); E37 race/count100 transcripts
  noted as incomplete (only canarycontrol recorded).
- **P0-1 (dockerlab inventory)**: full migration inventory produced
  with file, role, replacement, and action columns for every duplicate
  symbol in `internal/dockerlab/client.go` and related files.
- **P0-9 (protocol ownership scan)**: cmd/canary is consumer/alias-only;
  internal/canarycontrol is sole definition authority;
  internal/dockerlab still owns duplicate vocabulary.
- **P0-12 (focused verification)**: vet, unit, race, count=100, and
  short test commands all pass (real transcripts captured).
- **P0-13 (make gate)**: recorded as DEFERRED with reasons.

## Deferred (out-of-scope; belongs to CORRECTION39)
- **P0-2 (typed operations)**: Dockerlab must construct operations
  via `canarycontrol.NewControlOperation` + `BuildArgv`.
- **P0-3 (parallel decoder)**: Replace strictParseEnvelope,
  validateControlEnvelope, and private ControlEnvelope type with the
  shared canarycontrol authorities.
- **P0-4 (recording Engine seam)**: Inject a ControlExecRuntime
  interface that captures exec_create, exec_attach, exec_inspect.
- **P0-5 (exact Engine argv)**: Recording seam proves argv at the
  Engine boundary.
- **P0-6 (bounded output)**: stdout/stderr limits with overflow
  fail-closed.
- **P0-7 (exit/envelope consistency)**: Reject exit-0 + failure and
  nonzero-exit + success; preserve typed failure envelopes.
- **P0-8 (shared retry integration)**: CanaryHealthCheckViaExec uses
  canarycontrol.IsRetryable.
- **P0-10 (controller tests)**: Complete dockerlab test groups
  required by P0-10 list.
- **P0-11 (commit S38)**: No source changes made in CORRECTION38.
- **P0-13 (make gate)**: deferred to CORRECTION39.

## Status
```
CORRECTION37: SUPERSEDED_BY_CORRECTION38
CORRECTION38: PARTIAL
parent_correction03: PARTIAL
MEMLAB_08B: IN_PROGRESS
MEMLAB_08C: BLOCKED
```

## Identities
```
S34: 4ab1c7b925ba0e875c49caeedf7ead5422f2ff60
S35: 841dafc412a53709890ef37a0fd6e14644c219aa
S36: 54bc08b2f3e94179d1c0b398a8ad9eb65a125473
S37: f86253f9c285f937a2f7a44136a95363ea92d34c
E37: cd6c96211d27bac1006252b0a8177b1395363062
E38: (filled in at evidence commit)
```

## Why dockerlab migration is deferred

The dockerlab migration is a coordinated multi-file refactor that:
- Replaces dockerlab-owned types with canarycontrol types
  (ProtocolError, ErrorClass, ControlEnvelope, etc.).
- Replaces dockerlab's strictParseEnvelope with
  canarycontrol.DecodeEnvelopeExactlyOne.
- Replaces dockerlab's validateControlEnvelope with
  canarycontrol.ValidateControlEnvelope.
- Replaces dockerlab's IsProtocolNonRetryable with
  canarycontrol.IsRetryable (with inverted logic).
- Threads canarycontrol.NewControlOperation + BuildArgv through the
  three Canary*ViaExec entry points.
- Updates the recording Engine seam (new ControlExecRuntime interface).
- Updates the bounded-output handling.
- Updates exit/envelope consistency enforcement.
- Updates retry loop integration.
- Rewrites hundreds of dependent tests across dockerlab and the
  tovarisch-memory-lab CLI.

A single ACT cannot safely complete all of this in one session without
risking cascade test breakage. CORRECTION39 (or a successor ACT) is
explicitly scoped to the dockerlab migration.

## Recommendation for CORRECTION39 scope

```yaml
- dockerlab source migration (delete parallel vocabulary, use shared authority)
- recording Engine seam with full test coverage
- bounded stdout/stderr with overflow handling
- exit/envelope consistency enforcement
- shared retry integration into CanaryHealthCheckViaExec
- complete controller tests (P0-10 list)
- record exact Engine argv
- commit S39, run focused verification
- observe make gate
- if gate fails on pre-existing UVB-76 scope, repair or stop as PARTIAL
```

CORRECTION38 made no claims of parent closure and did not weaken the
verifier.