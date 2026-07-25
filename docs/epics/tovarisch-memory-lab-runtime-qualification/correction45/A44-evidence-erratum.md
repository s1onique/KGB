# CORRECTION45 — A44 Evidence Erratum

## Status

CORRECTION44 evidence was accepted as a bounded closure. The
authoritative record at S44/E44/A44 remains the canonical control
provenance. CORRECTION45 repairs five bounded residuals without
rewriting S44, E44, or A44, and then performs the live
source-image-executable qualification.

## Bounded residuals repaired

1. **CORRECTION44 placeholder test deleted**:
   `TestExecuteQualifiedLifecycle_ContainerIDReachesExec` was a
   placeholder that merely referenced the `canarycontrol.OpOperate`
   constant. The placeholder is removed.
2. **Real production call-graph seam exposed**:
   `RunCanonicalControlSequence(ctx, control, observations, options)`
   is the bounded production orchestrator that performs exactly
   `health → state → operate → state`. The real CLI
   (`runCommand`) and the live qualification tests both call this
   function. There is no parallel test-only implementation.
3. **A44 attestation source-hash manifest**:
   A44 names `s44-source-sha256s.txt` as the source-hash manifest.
   The file is content-bearing and verified to list every S44
   production/test source blob.
4. **A44 final-git-status.txt**:
   The file is a pre-attestation snapshot taken just before A44
   was committed. The embedded `git status` records `M
   docs/.../final-git-status.txt` plus two `??` (additions) for
   the attestation and source-hash files. The snapshot was
   generated as part of the A44 commit workflow.
5. **legacy-authority-after.txt**:
   The post-migration evidence was regenerated with command,
   working directory, patterns, files_scanned, and exit code.
   The static regression test
   `TestProductionControlSequence_LegacyAuthorityScan` enforces
   the production source has no remaining forbidden authority.
6. **Targeted digest rename manifest**:
   The targeted digest emitted by `scripts/make_targeted_digest.sh`
   is malformed; the legacy-authority proof and the static
   regression test supersede the digest rename authority.

## Production seam

```
RunCanonicalControlSequence(
    ctx context.Context,
    control *dockerlab.ControlRunner,
    observations *dockerlab.QualifiedExecutionObservations,
    options CanonicalControlSequenceOptions,
) (*CanaryState, *WorkloadResult, *CanaryState, error)
```

The sequence is:

```
ControlProbe(containerID, health)
ControlProbe(containerID, state)
ControlProbe(containerID, operate)
ControlProbe(containerID, state)
```

The function fails closed on any failure and never invokes a
parallel test-only implementation.

## Tests

The following tests exercise the production sequence:

* `TestProductionControlSequence_ExactContainerIDEveryOperation`
* `TestProductionControlSequence_ExactOperationOrder`
* `TestProductionControlSequence_HealthObservationFromEnvelope`
* `TestProductionControlSequence_InitialStateObservationFromEnvelope`
* `TestProductionControlSequence_OperateObservationFromEnvelope`
* `TestProductionControlSequence_FinalStateObservationFromEnvelope`
* `TestProductionControlSequence_UsesDistinctInitialAndFinalState`
* `TestProductionControlSequence_FailureAtHealthStopsSequence`
* `TestProductionControlSequence_FailureAtInitialStateStopsSequence`
* `TestProductionControlSequence_FailureAtOperateStopsSequence`
* `TestProductionControlSequence_FailureAtFinalStateFailsClosed`
* `TestProductionControlSequence_ContainerIDMismatchFails`
* `TestProductionControlSequence_OperationArgvIsCanaryExecutable`
* `TestProductionControlSequence_NoShellForbiddenArgs`
* `TestProductionControlSequence_RejectsBadInputs`
* `TestProductionControlSequence_NilInputsRejected`
* `TestProductionControlSequence_AttachesStdoutStderrStdoutOnly`
* `TestProductionControlSequence_EnvelopesAreValid`
* `TestProductionControlSequence_RealMultiplexDecodeEndToEnd`
* `TestProductionControlSequence_StdoutStdoutBoundary`
* `TestProductionControlSequence_LegacyAuthorityScan`

The fixture (`recordingDockerExecAPI`) emits real Docker
multiplex framing and supplies valid canonical envelopes. No
live Docker daemon is required.
