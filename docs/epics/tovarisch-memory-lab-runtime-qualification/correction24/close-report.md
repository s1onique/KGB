# CORRECTION24 Close Report

## Title

Clean-Tree, TestMain Directory Repair, and Source-Bound Final Qualification

## Status: CLOSED ✅

## Summary

Successfully repaired the `TestMain` build directory issue and achieved source-bound qualification closure.

## CORRECTION23 Assessment

CORRECTION23 introduced tooling-doctrine repairs:
1. **bootstrap-missing-baseline**: Script added to bootstrap baseline
2. **python-invocation**: Python replaced with Go helper `extract-image-metadata`

CORRECTION23 was **VALID** but required TestMain repair for clean-tree closure.

## P0-1: Generated-Binary Hygiene

Added to `.gitignore`:
```
tovarisch/.factory/bin/extract-image-metadata
*.test
```

## P0-2: Doctrine Repair Verification

```
$ go run ./cmd/verify-script-doctrine --bootstrap
Script doctrine verification passed
```

## P0-3: TestMain Directory Repair

**Root Cause**: `buildOnce()` used `os.Getwd()` which returns the test binary's CWD, not the module root.

**Fix**: Use `go list -m -f '{{.Dir}}'` for module root discovery, with `TOVARISCH_REPO_ROOT` env override for closure mode.

**Files Modified**:
- `bounded_main_test.go`: Fixed `buildOnce()` to use canonical module root discovery
- `qualified_live_test.go`: Added `TOVARISCH_REPO_ROOT` support for repo discovery

## Subject Identity

- **S24**: `f3f8e7f55bec7c4efe21fbb23758905604804f67`
- **ST24**: `07f153615f5988de88492235f7e353bbc07dab56`
- **S23**: `7e02636e51e0fb5147e04f716073a1c20a137481` (parent)

## P0-5: Helper Binary

```
role: helper_smoke
vcs_revision: f3f8e7f55bec7c4efe21fbb23758905604804f67
vcs_modified: false
source_tree: 07f153615f5988de88492235f7e353bbc07dab56
sha256: cf238f2bce6d8e0e9d3bbad08ffcdb59ccd3cf79e642b8743481b71edc2d0da0
```

## P0-7: Live Smoke Results

```
test_executed: true
test_skipped: false
source_commit: f3f8e7f55bec7c4efe21fbb23758905604804f67
source_tree: 07f153615f5988de88492235f7e353bbc07dab56
executable_sha256: cf238f2bce6d8e0e9d3bbad08ffcdb59ccd3cf79e642b8743481b71edc2d0da0
pull_observation_available: true
pull_attempts: 0
image_exact_id_match: true
network_exact_id_match: true
cleanup_complete: true
pass: true
```

## P0-8: Production CLI Build

```
role: production_cli
vcs_revision: f3f8e7f55bec7c4efe21fbb23758905604804f67
vcs_modified: false
source_tree: 07f153615f5988de88492235f7e353bbc07dab56
sha256: 021897b1fe89c65e61cc959814d718edc9bbf6fa4948bb4bba3a9c0c77905979
```

## P0-11: Test Suite Results

```
$ cd tovarisch/labs/memory && go test -count=1 -short ./...
ok  github.com/s1onique/KGB/tovarisch/labs/memory  0.008s
ok  github.com/s1onique/KGB/tovarisch/labs/memory/cmd/canary  0.047s
ok  github.com/s1onique/KGB/tovarisch/labs/memory/cmd/tovarisch-memory-lab  30.035s
ok  github.com/s1onique/KGB/tovarisch/labs/memory/internal/analysis  0.007s
ok  github.com/s1onique/KGB/tovarisch/labs/memory/internal/dockerlab  0.014s
ok  github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence  0.014s
ok  github.com/s1onique/KGB/tovarisch/labs/memory/internal/procfs  0.013s
ok  github.com/s1onique/KGB/tovarisch/labs/memory/internal/sampling  0.213s
```

## P0-12: Canonical Gate

```
$ go run ./cmd/verify-script-doctrine --bootstrap
Script doctrine verification passed
```

## P0-13: Cleanup

Containers and networks cleaned up:
- Container `bf2251b6e948` removed
- Network `1515969e8e72` removed
- Working tree clean

## Files Changed

1. `.gitignore` - Added generated binary hygiene
2. `tovarisch/labs/memory/cmd/tovarisch-memory-lab/bounded_main_test.go` - Fixed build directory
3. `tovarisch/labs/memory/cmd/tovarisch-memory-lab/qualified_live_test.go` - Added TOVARISCH_REPO_ROOT support

## Evidence Commit

To be created as final step.

## Board Updates

| Item | Before | After |
|------|--------|-------|
| CORRECTION23 | PARTIAL | SUPERSEDED_BY_CORRECTION24 |
| CORRECTION24 | - | CLOSED |
| parent_correction03 | PARTIAL | CLOSED |
| P0_10_live_no_pull_smoke | BLOCKED | CLOSED |
| MEMLAB_08A | - | DONE |
| MEMLAB_08B | IN_PROGRESS | DONE |
| MEMLAB_08C | BLOCKED | READY |

## Conclusion

CORRECTION24 CLOSED ✅

- TestMain directory repair successful
- Live smoke passes with source-bound provenance
- All tests pass
- Working tree clean
- CORRECTION23 valid but extended
