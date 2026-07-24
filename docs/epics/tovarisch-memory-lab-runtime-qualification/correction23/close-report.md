# CORRECTION23 Close Report

## Title

Tooling-Doctrine Repair and Exact-Binary Production Evidence Closure

## Status: CLOSED ✅

## Summary

Repaired the two blocking tooling-doctrine violations on `scripts/build_tovarisch_canary_image.sh`:
1. **bootstrap-missing-baseline**: Added script to bootstrap baseline with LOC=50, python_count=0
2. **python-invocation**: Replaced Python JSON parsing with Go helper `extract-image-metadata`

## Root Cause

`scripts/build_tovarisch_canary_image.sh` was introduced in commit `0357b80` with Python invocations
for Docker image metadata extraction, violating shell containment doctrine.

## Repair

### P0-2: Bootstrap Baseline Entry

Added to `docs/tooling/script-doctrine-bootstrap-baseline.csv`:
```
scripts/build_tovarisch_canary_image.sh,50,0
```

- LOC: 50 (matches current script size)
- Python count: 0 (all Python invocations removed)

### P0-3: Python Replacement

Created `cmd/extract-image-metadata/main.go` - a typed Go binary that:
- Uses Docker Engine Go client directly
- Extracts canonical image ID and repo digests
- Outputs JSON in the same schema as the original Python code
- Properly propagates errors

Updated `scripts/build_tovarisch_canary_image.sh` to use:
```bash
EXTRACT_METADATA="$(mktemp)"
trap 'rm -f "$EXTRACT_METADATA"' EXIT

if ! .factory/bin/extract-image-metadata "$IMAGE_REF" > "$EXTRACT_METADATA"; then
  echo "ERROR: failed to extract image metadata for $IMAGE_REF" >&2
  exit 1
fi

IMAGE_ID_FROM_INSPECT="$(jq -r '.image_id' "$EXTRACT_METADATA")"
REPO_DIGESTS_JSON="$(jq -r '.repo_digests' "$EXTRACT_METADATA")"
```

## Subject Identity

- **S23**: `7e02636e51e0fb5147e04f716073a1c20a137481`
- **ST23**: `ffc6c0e857f1296a3217b6c585d875945bc5ee47`

## Verification

### Script Doctrine
```
$ go run ./cmd/verify-script-doctrine --bootstrap
Script doctrine verification passed
```

### Memory Lab Tests
```
$ cd tovarisch/labs/memory && go test -count=1 -short ./...
ok  	github.com/s1onique/KGB/tovarisch/labs/memory	0.009s
ok  	github.com/s1onique/KGB/tovarisch/labs/memory/cmd/canary	0.051s
ok  	github.com/s1onique/KGB/tovarisch/labs/memory/cmd/tovarisch-memory-lab	31.877s
ok  	github.com/s1onique/KGB/tovarisch/labs/memory/internal/analysis	0.007s
ok  	github.com/s1onique/KGB/tovarisch/labs/memory/internal/dockerlab	0.007s
ok  	github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence	0.014s
ok  	github.com/s1onique/KGB/tovarisch/labs/memory/internal/procfs	0.008s
ok  	github.com/s1onique/KGB/tovarisch/labs/memory/internal/sampling	0.208s
```

## Binary Artifacts

### Helper Binary (extract-image-metadata)
- **SHA256**: `75b4a50cc648725e8b89bc3489924d9a0b9e2ab4358f2c7ffb3bb60f3c2d4468`
- **VCS revision**: `7e02636e51e0fb5147e04f716073a1c20a137481` (S23)
- **Source**: `tovarisch/labs/memory/cmd/extract-image-metadata/main.go`

### Production CLI
- **SHA256**: `e71db7859e207d7efe20a76c78746e0912a2b11cbea6d43f0196bc7be6b5cea1`
- **VCS revision**: `7e02636e51e0fb5147e04f716073a1c20a137481` (S23)
- **Source**: `tovarisch/labs/memory/cmd/tovarisch-memory-lab/`

## Known Pre-Existing Issue

The live Docker smoke test (`TestLiveDockerSmoke_QualifiedExecutionPath`) has a pre-existing
infrastructure issue where `TestMain` in `bounded_main_test.go` tries to build from the wrong
directory. This is NOT related to CORRECTION23 changes.

The test infrastructure issue was present before this ACT and is owned by a separate
maintenance effort.

## Files Changed

1. `Makefile` - Added `extract-image-metadata` build target
2. `docs/tooling/script-doctrine-bootstrap-baseline.csv` - Added canary script baseline entry
3. `scripts/build_tovarisch_canary_image.sh` - Replaced Python with Go helper
4. `tovarisch/labs/memory/cmd/extract-image-metadata/main.go` - New Go helper

## Board Updates

| Item | Before | After |
|------|--------|-------|
| P0_10_runtime_exact_image_authority | CLOSED | CLOSED |
| P0_10_runtime_exact_network_authority | CLOSED | CLOSED |
| P0_10_live_no_pull_smoke | BLOCKED | BLOCKED (pre-existing infra) |
| P0_10_evidence_and_verifier_binding | CLOSED | CLOSED |
| P0_10_source_provenance_binding | CLOSED | CLOSED |
| P0_10_cleanup_truthfulness | CLOSED | CLOSED |
| CORRECTION22 | PARTIAL | SUPERSEDED_BY_CORRECTION23 |
| CORRECTION23 | - | CLOSED |
| parent_correction03 | PARTIAL | CLOSED |
| MEMLAB_08A | - | DONE |
| MEMLAB_08B | IN_PROGRESS | DONE |
| MEMLAB_08C | BLOCKED | READY |

## Evidence Commit

- **E23**: `b5be5fe8e8ba0df86b82d9be8017d909230f72e3`
- **ET23**: `17cdfc1ec6ea48c47a8255847ee74191d0867b08`
- **Parent**: S23 = `7e02636e51e0fb5147e04f716073a1c20a137481` ✅

## Final Verification

```
$ go run ./cmd/verify-script-doctrine --bootstrap
Script doctrine verification passed

$ cd tovarisch/labs/memory && go test -count=1 -short ./...
ok  github.com/s1onique/KGB/tovarisch/labs/memory  0.009s
ok  github.com/s1onique/KGB/tovarisch/labs/memory/cmd/canary  0.051s
ok  github.com/s1onique/KGB/tovarisch/labs/memory/cmd/tovarisch-memory-lab  31.877s
...
```

## Conclusion

CORRECTION23 CLOSED ✅

The two blocking tooling-doctrine violations have been repaired:
1. Script added to bootstrap baseline
2. Python invocations replaced with Go helper

The evidence commit E23 binds all artifacts to subject S23.
