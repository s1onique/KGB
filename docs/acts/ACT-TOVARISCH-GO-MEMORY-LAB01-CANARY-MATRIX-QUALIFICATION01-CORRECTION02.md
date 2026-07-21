# ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION02

## Summary

Converged the memory-lab matrix producer and verifier onto a single authoritative reconstruction path (`ReconstructMatrixVerdict`). Previously, both paths independently computed matrix validity, creating dual authority. Now both `matrix` and `verify-matrix` commands use the same shared function, eliminating the possibility of divergent verdicts.

## Changes

### New File: `reconstruct.go`

Single authoritative source for all matrix verdict reconstruction logic:

- **`CanonicalCrossRunCheckNames`** - Ordered list of 16 cross-run check names as single source of truth
- **`CanonicalMatrixChecks()`** - Projects `CrossRunChecks` onto canonical named check list
- **`CountCanonicalChecks()`** - Computes total/pass/failed from canonical projection
- **`ReconstructSameSchema()`** - Real SameSchema reconstruction from verified child manifests
- **`ReconstructCrossRunChecks()`** - Computes all 16 cross-run checks from verified runs
- **`ReconstructScenarioResults()`** - Builds scenario results with correct classification mapping
- **`ReconstructMatrixVerdict()`** - **SINGLE AUTHORITATIVE FUNCTION** for matrix verdict reconstruction
- **`CompareVerdicts()`** - Complete field-by-field verdict comparison with diagnostics
- **`BuildMatrixCleanupEvidence()`** - Constructs canonical cleanup evidence with exact identities
- **`ValidateCleanupEvidence()`** - Validates cleanup evidence bound to matrix manifest
- **`ComputeMatrixChecksums()`** - Matrix-level checksums including cleanup evidence
- **`BuildVerifiedRunsFromMatrix()`** - Hermetic verified run builder for both paths

### Modified: `matrix.go`

- Added `UniqueContainerIDs` field to `CrossRunChecks` struct
- Added `ChecksPassed` field to track canonical check count

### Modified: `matrix_cmd.go`

- `matrixCommand` now uses `ReconstructMatrixVerdict` and `WriteMatrixCleanupEvidence`
- `verifyMatrixCommand` updated to use shared authority
- `AllTrue()` method now correctly skips non-boolean fields
- Cleanup evidence persistence integrated into producer path

### Modified: `matrix_test.go`

- Updated `TestCrossRunChecksAllTrue` to include all 16 checks including `UniqueContainerIDs`

## Files Changed

```
tovarisch/labs/memory/cmd/tovarisch-memory-lab/
├── main.go                    # Modified (updated to use CORRECTION02 verify command)
├── matrix.go                  # Modified (added UniqueContainerIDs, ChecksPassed)
├── matrix_cmd.go              # Modified (uses shared authority, fixed AllTrue)
├── matrix_test.go             # Modified (updated test to include all 16 checks)
└── reconstruct.go             # NEW (single authoritative reconstruction)
```

## Verification

```bash
cd /home/kgb/Projects/KGB/tovarisch/labs/memory
go build ./cmd/tovarisch-memory-lab  # PASS
go test ./cmd/tovarisch-memory-lab/... -race -count=1  # PASS
```

**Note**: `make gate` fails due to pre-existing script doctrine violations unrelated to this ACT:
```
[missing-inventory] scripts/build_tovarisch_canary_image.sh: shell script not in inventory
[python-invocation] scripts/build_tovarisch_canary_image.sh: Script invokes Python
```

This is a pre-existing issue that should be tracked separately.

## Assumptions

1. The 16 cross-run checks are complete and correct for the matrix contract
2. Cleanup evidence must include exact container/network/process identities
3. The verifier must be able to independently reconstruct and compare verdicts
4. Fail-closed: any reconstruction error or mismatch causes non-zero exit

## Blockers

None.

## Zig 0.16 Observations

Not applicable - this is a Go implementation.
