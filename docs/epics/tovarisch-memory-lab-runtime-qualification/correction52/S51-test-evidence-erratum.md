# S51 Test Evidence Erratum

## Summary

S51 (6deb9383d8e1a9c4f7a470012221ac6edc978fd4) added tests for production qualified evidence wiring but the tests are non-proving. This erratum documents the S51 test failures.

## S51 Classification

```yaml
S51_commit: 6deb9383d8e1a9c4f7a470012221ac6edc978fd4
S51_tree: 6fd87ff562e0c9205805aa1a255aa9d4c6b79cf7
S51_parent: 93f2e1c576a5d854f298c0aec2986a4030c71e23
production_delta: zero
patch_hygiene_findings:
  - docs/epics/tovarisch-memory-lab-runtime-qualification/correction51/call-graph-classification.md:138: trailing whitespace
```

## S51 Files

1. `docs/epics/tovarisch-memory-lab-runtime-qualification/correction51/CORRECTION50-production-evidence-erratum.md`
2. `docs/epics/tovarisch-memory-lab-runtime-qualification/correction51/call-graph-classification.md`
3. `tovarisch/labs/memory/cmd/tovarisch-memory-lab/production_qualified_evidence_test.go`

## S51 Test Analysis

### S51 Test File: production_qualified_evidence_test.go

The following tests were added but are non-proving:

#### 1. TestProductionRun_S50Regression_NoQualifiedEvidence
- **Status**: Environment-gated, skips in hermetic mode
- **Issue**: Requires `TOVARISCH_INTEGRATION_TEST=1` and Docker
- **P0-1 through P0-13**: NOT PROVEN (test skips in hermetic mode)

#### 2. TestProductionEvidence_ProductionCLIProducesEvidence
- **Status**: Environment-gated, skips when build fails
- **Issue**: Uses `buildCmd.Dir = "../../../.."` - fragile relative path
- **Issue**: Skips when production binary cannot be built
- **P0-1 through P0-13**: NOT PROVEN (test can skip due to build path)

#### 3. TestProductionRun_QualifiedEvidenceCannotBeDisabled
- **Status**: Log-only
- **Issue**: Inspects global `flag.CommandLine` but production uses `NewRunFlagSet`
- **Issue**: No executable proof, only comment-based verification
- **P0-1 through P0-13**: NOT PROVEN (wrong FlagSet inspected)

#### 4. TestProductionEvidence_BothConsumersUseSameProducer
- **Status**: Log-only
- **Issue**: Contains only `t.Log()` statements, no assertions
- **P0-1 through P0-13**: NOT PROVEN (log-only test)

#### 5. TestProductionEvidence_ProductionUsesProductionExecutable
- **Status**: Log-only
- **Issue**: Uses `os.Executable()` - hashes the test process, not production binary
- **Issue**: Contains only `t.Log()` statements, no assertions
- **P0-1 through P0-13**: NOT PROVEN (test executable hashed, not production)

#### 6. TestProductionEvidence_TimingSequence
- **Status**: Log-only
- **Issue**: Contains only `t.Log()` statements referencing other tests
- **P0-1 through P0-13**: NOT PROVEN (log-only, no own assertions)

#### 7. TestProductionEvidence_RejectionProducesNoPassingArtifact
- **Status**: Log-only
- **Issue**: Contains only `t.Log()` statements referencing other tests
- **P0-1 through P0-13**: NOT PROVEN (log-only, no own assertions)

#### 8. TestProductionRun_UsesCanonicalEvidenceProducer
- **Status**: Log-only
- **Issue**: Contains only `t.Log()` statements
- **P0-1 through P0-13**: NOT PROVEN (log-only, no assertions)

## P0-1 through P0-13 Status

S51 does NOT prove P0-1 through P0-13 because:
1. S51 contains no production wiring change (production_delta: zero)
2. Key integration tests are environment-gated
3. Second production test can skip when build path is wrong
4. Several tests contain only logging and comments
5. Bypass test inspects the wrong flag set
6. Executable-binding test hashes the test executable

## Patch Hygiene Status

S51 patch hygiene: **FAIL**

```
docs/epics/tovarisch-memory-lab-runtime-qualification/correction51/call-graph-classification.md:138: trailing whitespace
```

## Actual S50 Defect Classification

Based on historical artifact tree analysis of CORRECTION50 artifacts:

```yaml
historical_production_evidence:
  physically_present: false
  discovered_path: null
  included_in_checksum_inventory: false
  included_in_manifest: false

classification: artifact_written_but_not_inventoried
```

The production path in S50:
- ✅ Calls `evidence.BuildAndPersistFinalQualifiedEvidence`
- ✅ Writes `qualified-execution-evidence.json` to correct location
- ❌ Does NOT include `qualified-execution-evidence.json` in `ArtifactInventory`
- ❌ Does NOT include `qualified-execution-evidence.json` in checksums.txt

## Required Correction

CORRECTION51-CORRECTION01 must:
1. Add `qualified-execution-evidence.json` to `ArtifactInventory`
2. Replace all non-proving tests with hermetic executable contract tests
3. Fix patch hygiene (trailing whitespace removed)
4. Prove P0-1 through P0-13 without environment gates
