# CORRECTION48 — S47 Classification Erratum

## Provenance

* `S46`  = `b6415f94aa8b8cc8b0118e9590fcbc17fabb82af` (`ST46` = `f34bcab8fcbcd805f449f52ddd8e1f7447bf2097`)
* `S47`  = `7934a8a8bf71a10df845028fa447d965bc3e1490` (`ST47` = `ce91b691c42aed3b5647344fda1dc8c2e9f9970e`)
* `S47_parent` = `S46`

## Statement

1. **`S47` is documentation-only.** Its single tracked change adds
   `docs/epics/tovarisch-memory-lab-runtime-qualification/correction47/correction46-live-side-effect-inventory.txt`.
   The `S46` source tree is preserved bit-for-bit under `S47`.
2. **`S47` does not supersede `S46`’s source implementation.** No
   `*.go`, `Makefile`, `go.mod`, `go.sum` or `tovarisch/labs/memory/**`
   source file was modified by `S47`.
3. **The metadata-path resolver was not committed.** No production
   package under `tovarisch/labs/memory/internal/buildmetadata/`
   contains the canonical `ResolveCanaryMetadataPath` resolver that
   `CORRECTION47` was supposed to deliver. Only an untracked
   proposed-patch copy exists under
   `docs/epics/tovarisch-memory-lab-runtime-qualification/correction47/pending-internal-evidence/canary_metadata_path.go`.
4. **The provenance-cleanliness policy was not committed.** The
   binary `RequireClean bool` lives in
   `tovarisch/labs/memory/internal/evidence/controller_provenance.go`.
   No `ProvenanceCleanPolicy` enum, no
   `ProvenanceIgnoreWorktree` constant, and no rejection coverage
   for tracked/staged/untracked modifications, revision mismatch,
   tree mismatch or embedded `vcs.modified=true` has been merged.
5. **No live artifact was built from `S47`.** The previously-tracked
   `tovarisch/labs/memory/canary-image-build.json` was restored to
   its `S46` placeholder via `git restore` so the `S47` freeze is
   not contaminated by `S46` canary-image outputs. No canary image
   was built from `S47`. No `tovarisch-memory-lab` binary was built
   from `S47`. No helper test executable was compiled from `S47`.
   No Docker daemon was contacted by `S47`.
6. **The source no-amendment boundary was therefore never entered.**
   `S47` is a checkpoint, not a code-bearing source subject.
7. **`S48` is the first intended qualification-harness source
   subject.** It introduces the committed metadata-path resolver,
   the committed `ProvenanceCleanPolicy` enum, the committed
   external artifact-root contract, the committed
   hermetic qualification-build script, and the committed
   artifact-role verifier.

## Authority chain

```
CORRECTION46   S46  (b6415f94)  code-bearing
CORRECTION47   S47  (7934a8a8)  documentation-only
CORRECTION48   S48  (...)       code-bearing
```

Per `kgb://factory/workflow`, only the code-bearing subjects
(`S46`, `S48` and later) participate in the live qualification
harness. `S47` exists solely as a clean documentation checkpoint
that records the `S46` cleanup.

## Downstream consumers

* The live Docker canary image must be produced from `S48`.
* The compiled helper `go test -c` artifact must be produced from
  `S48`.
* The production `tovarisch-memory-lab` CLI build must come from
  `S48`.
* All claim fields stamped into those artifacts must reference
  `S48` (not `S46`, not `S47`).

This erratum freezes that authority so `CORRECTION49` cannot
accidentally regress to `S46`.
