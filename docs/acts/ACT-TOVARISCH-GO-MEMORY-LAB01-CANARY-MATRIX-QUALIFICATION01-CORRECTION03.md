# ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION03

## Summary

Fixed network identity verification by adding `network-identity.json` to required child artifacts. The test suite now correctly verifies complete canonical fixtures including network identity extraction.

## Status

**IN PROGRESS** - Review identified additional P0 issues that require follow-up work.

## Accepted Improvements (Review Verdict: PARTIAL)

| Item | Status |
|------|--------|
| network_identity_required_by_child_geometry | ✅ ACCEPTED |
| network_identity_in_child_checksums | ✅ ACCEPTED |
| strict_decoder_struct_includes_schema_version | ✅ ACCEPTED |
| positive_VerifyMatrixBundle_fixture | ✅ PRESENT |
| ordinary_tests | ✅ PASS_REPORTED |
| race_tests | ✅ PASS_REPORTED |

## Changes Made

### Modified: `matrix_verify.go`

- Added `network-identity.json` to `validateChildGeometry` required files list
- Fixed `verifyChildRunBundle` to decode `network-identity.json` with correct struct including `schema_version` field

### Modified: `matrix_fixture.go`

- Added `network-identity.json` artifact writing with correct JSON structure
- Added `network-identity.json` to child checksums computation
- Added deterministic `FixtureNetworkIDs` for fixture generation

### New File: `matrix_verify_terminal_test.go`

Terminal qualification tests:
- `TestVerifyMatrixBundle_AcceptsCompleteCanonicalFixture` - Valid fixture passes
- `TestVerifyMatrixBundle_SemanticMutations` - 22 semantic mutation tests
- `TestVerifyMatrixBundle_RejectsEqualInvalid` - Equal-invalid verdict rejection
- `TestVerifyMatrixBundle_RejectsChecksumMismatch` - Checksum failure detection
- `TestVerifyMatrixCommand_ValidFixtureEmitsPASS` - PASS line contract
- `TestVerifyMatrixCommand_FailureEmitsNoPASS` - No PASS on failure

## Open P0 Issues (Follow-up Work Required)

### P0-1: matrix-cleanup.json Not in Checksum Contract
**Issue**: The cleanup evidence is not integrity-bound by checksums.
**Required**: Add `matrix-cleanup.json` to canonical matrix inventory.

### P0-2: Production Observer Bypasses RunBoundedCommand
**Issue**: Two command-execution authorities exist.
**Required**: Refactor `DefaultDockerRunner` to use `RunDockerCommand`.

### P0-3: WaitDelayExpired Discarded at Docker Boundary
**Issue**: `BoundedCommandResult.WaitDelayExpired` not projected to `DockerCommandResult`.
**Required**: Add field to Docker result and reject in observers.

### P0-4: No Real WaitDelay Test
**Issue**: `retain-descriptor` mode defined but not invoked.
**Required**: Real helper-process test for WaitDelayExpired.

### P0-5: Command Tests Don't Invoke Actual Command
**Issue**: Tests are simulated strings, not real CLI invocation.
**Required**: Call `verifyMatrixCommand` or built binary.

### P0-6: Equal-Invalid Test Is Not Equal-Invalid
**Issue**: Test creates stored≠reconstructed, not equal-invalid.
**Required**: Mutate authoritative evidence so reconstruction becomes invalid.

### P0-7: Child Semantic Mutations Masked by Stale Checksums
**Issue**: Child checksums not regenerated after mutation.
**Required**: Regenerate child checksums before matrix checksums.

### P0-8: Fixture Checksum Silently Skips Missing Files
**Issue**: `continue` on read error hides missing artifacts.
**Required**: Fail immediately on missing required artifacts.

### P0-9: Network Schema Not Verified
**Issue**: Schema value not validated, decode errors silently ignored.
**Required**: Use `ReadNetworkIdentity` from cleanup_observation.go.

### P0-10: Docker Smoke Violates No-Pull Contract
**Issue**: `docker pull` and name-based removal.
**Required**: Use prebuilt local image, exact ID removal.

## Files Changed

```
tovarisch/labs/memory/cmd/tovarisch-memory-lab/
├── matrix_verify.go              # Modified (network identity extraction)
├── matrix_fixture.go             # NEW (canonical fixture helper)
└── matrix_verify_terminal_test.go # NEW (terminal qualification tests)
```

## Verification

```bash
cd /home/kgb/Projects/KGB/tovarisch/labs/memory/cmd/tovarisch-memory-lab
go test ./...          # PASS (8.972s)
go test -race ./...    # PASS (11.763s)
```

## Key Fix

The issue was that `network-identity.json` had a `schema_version` field, but the decode struct in `verifyChildRunBundle` only had `id` and `name` fields. With `DisallowUnknownFields` enabled in strict JSON decoding, this caused the decode to fail silently, leaving `NetworkID` empty.

**Root cause**: The fixture was writing correct JSON with `schema_version`, but the verifier's decode struct didn't include that field.

**Fix**: Added `SchemaVersion string `json:"schema_version"`` to the decode struct.

## Zig 0.16 Observations

Not applicable - this is a Go implementation.
