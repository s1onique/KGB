# S50 Production Call Graph and Defect Classification

## Call Graph

```
runCommand (main.go)
  │
  ├── metadata resolution (line 211-218)
  │     └─ buildmetadata.ResolveCanaryMetadataPath
  │
  ├── Docker client factory (line 228-231)
  │     └─ dockerlab.NewClient
  │
  ├── Docker server version (line 233-236)
  │
  ├── Image identity resolution (line 245-253)
  │     └─ dockerClient.ResolveImageIdentity
  │
  ├── Create run directory (line 263-266)
  │     └─ artifactsPath = <artifacts-dir>/<run-id>
  │
  ├── Initialize evidence writer (line 268)
  │     └─ evidence.NewWriter(runID, scenario, artifactsPath)
  │
  ├── Write initial manifest (line 285-287)
  │     └─ evidenceWriter.WriteManifest(manifest)
  │
  ├── Execute lifecycle (line 428-430)
  │     └─ dockerlab.ExecuteQualifiedDockerLifecycle
  │           │
  │           ├── PrepareQualifiedContainer
  │           │     ├─ ImageInspectWithRaw (resolve exact ID)
  │           │     ├─ NetworkCreate
  │           │     ├─ NetworkInspect
  │           │     └─ ContainerCreate + ContainerInspect
  │           │
  │           ├── ContainerStart
  │           │
  │           ├── Run workload callback (line 307-416)
  │           │     ├─ ContainerGetPID
  │           │     ├─ RunCanonicalControlSequence
  │           │     ├─ Sampler operations
  │           │     ├─ ContainerStop
  │           │     └─ Write workload artifacts
  │           │
  │           ├── WaitForTerminalState
  │           │
  │           └── BoundedCleanup
  │                 ├─ ContainerRemove + absence verify
  │                 └─ NetworkRemove + absence verify
  │
  ├── Check lifecycle outcome (line 432-439)
  │     └─ Return error if outcome==nil or err!=nil
  │
  ├── Collect provenance (line 441-454)
  │     └─ evidence.CollectControllerProvenance
  │
  ├── Build qualified evidence ★ (line 455-457)
  │     └─ evidence.BuildAndPersistFinalQualifiedEvidence
  │           │
  │           ├── Validate outcome completeness
  │           │     ├─ Terminal state observed
  │           │     ├─ Container removed
  │           │     └─ Network removed
  │           │
  │           ├── Validate provenance
  │           │     ├─ Source commit/tree present
  │           │     ├─ Git object format valid
  │           │     ├─ VCS not modified
  │           │     ├─ Clean policy enforced
  │           │     └─ Executable SHA-256 valid
  │           │
  │           ├── Build evidence from observations
  │           │
  │           ├── Persist to: <artifactsPath>/qualified-execution-evidence.json
  │           │     └─ writeFileAtomic
  │           │
  │           ├── Verify persisted bytes
  │           │     └─ VerifyQualifiedExecutionBytes
  │           │
  │           └── Return: *QualifiedExecutionEvidence
  │
  ├── Write final manifest (line 596-628)
  │     └─ evidenceWriter.WriteManifest(finalizedManifest)
  │
  ├── Write checksums (line 642-644)
  │     └─ evidenceWriter.WriteChecksumsForInventory
  │
  └── Return success/failure
```

## Artifact Location

- **Expected evidence location**: `<artifacts-dir>/<run-id>/qualified-execution-evidence.json`
- **Evidence producer path**: `BuildAndPersistFinalQualifiedEvidence(..., artifactsPath)`
- **Producer writes to**: `<artifactsPath>/qualified-execution-evidence.json`

## Defect Classification

Based on analysis of S50 source code (93f2e1c):

### Classification: `producer_not_called`

**OR** possibly `producer_called_with_wrong_artifact_directory`

The production code at line 455 calls `BuildAndPersistFinalQualifiedEvidence` with `artifactsPath`, which is the correct `<artifacts-dir>/<run-id>` path.

However, the CORRECTION50 observation states production evidence was "ABSENT". This suggests one of:
1. The call was NOT present in S50 (producer_not_called)
2. The call was present but used wrong directory (producer_called_with_wrong_artifact_directory)
3. The call failed but error was swallowed (producer_error_discarded)

### Evidence Path Analysis

Looking at `final_qualified_evidence.go` line 66:
```go
data, err := os.ReadFile(artifactDir + "/qualified-execution-evidence.json")
```

The producer writes to `artifactDir + "/qualified-execution-evidence.json"`.

If `artifactsPath = "<artifacts-dir>/<run-id>"`, then evidence should be at:
`<artifacts-dir>/<run-id>/qualified-execution-evidence.json`

### Required Verification

A regression test must verify:
1. Production CLI produces `qualified-execution-evidence.json`
2. Evidence is in `<artifacts-dir>/<run-id>/qualified-execution-evidence.json`
3. Evidence has `pass: true`
4. Evidence binds production executable SHA-256
5. Evidence is verified before CLI returns success

## Test Strategy

1. **TestProductionRun_S50Regression_NoQualifiedEvidence**
   - Reproduces: CLI returns 0, workload artifacts present, qualified evidence absent
   
2. **TestProductionRun_QualifiedEvidenceMandated**
   - Verifies: CLI returns 0, qualified evidence present with pass=true

3. **TestProductionRun_QualifiedEvidenceCannotBeDisabled**
   - Verifies: No flag can disable evidence production

4. **TestProductionRun_ProductionUsesCanonicalProducer**
   - Verifies: Both helper and production call BuildAndPersistFinalQualifiedEvidence

5. **TestProductionRun_ProductionEvidenceUsesProductionExecutable**
   - Verifies: Production evidence binds production executable SHA-256
