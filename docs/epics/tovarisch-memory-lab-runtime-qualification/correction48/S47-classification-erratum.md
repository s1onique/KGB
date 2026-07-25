CORRECTION48 — S47 Classification Erratum
====================================

Provenance
---------

* S46  = `b6415f94aa8b8cc8b0118e9590fcbc17fabb82af` (`ST46` = `f34bcab8fcbcd805f449f52ddd8e1f7447bf2097`)
* S47  = `7934a8a8bf71a10df845028fa447d965bc3e1490` (`ST47` = `ce91b691c42aed3b5647344fda1dc8c2e9f9970e`)
        S47_parent = S46
* S48  = `9d431ab83c168bb6f880a32fcf45c36d12fdc0d8` (`ST48` = `793f86e6d20c9dd147bdf73e2a763c3f1bfc67b2`)
        S48_parent = S47

Statement
---------

1. **S47 is documentation-only.** Its single tracked change adds
   `docs/epics/tovarisch-memory-lab-runtime-qualification/correction47/correction46-live-side-effect-inventory.txt`.
   The S46 source tree is preserved bit-for-bit under S47.

2. **S47 does not supersede S46's source implementation.** No
   `*.go`, `Makefile`, `go.mod`, `go.sum` or `tovarisch/labs/memory/**`
   source file was modified by S47.

3. **The metadata-path resolver was not committed at S47.** No
   production package under `tovarisch/labs/memory/internal/buildmetadata/`
   contains the canonical `ResolveCanaryMetadataPath` resolver that
   CORRECTION48 was supposed to deliver. Only an untracked proposed-
   patch copy existed under
   `docs/.../correction47/pending-internal-evidence/canary_metadata_path.go`
   (deleted at S48).

4. **The provenance-cleanliness policy was not committed at S47.**
   The binary `RequireClean bool` lived in
   `tovarisch/labs/memory/internal/evidence/controller_provenance.go`
   but the typed `ProvenanceCleanPolicy` enum, the unknown-policy
   rejection, and the dirty-state-rejection coverage were not yet
   merged.

5. **No live artifact was built from S47.** The previously-tracked
   `tovarisch/labs/memory/canary-image-build.json` was restored to
   its S46 placeholder via `git restore` so the S47 freeze is not
   contaminated by S46 canary-image outputs. No canary image was
   built from S47, no `tovarisch-memory-lab` binary was built from
   S47, no helper test executable was compiled from S47, and no
   Docker daemon was contacted by S47.

6. **The source no-amendment boundary was therefore never entered.**
   S47 is a checkpoint, not a code-bearing source subject.

7. **S48 is the first intended qualification-harness source subject.**
   S48 introduces the committed metadata-path resolver, the typed
   `ProvenanceCleanPolicy` enum, the committed external qualification-
   artifact root, the committed hermetic qualification-build program,
   and the committed artifact-role verifier.

Authority chain
---------------

```
CORRECTION46   S46  (b6415f94)  code-bearing
CORRECTION47   S47  (7934a8a8)  documentation-only
CORRECTION48   S48  (9d431ab8)  code-bearing
```

Per `kgb://factory/workflow`, only the code-bearing subjects
participate in the live qualification harness. S47 existed solely
as a clean documentation checkpoint that recorded the S46 cleanup.

Downstream consumers
-------------------

* The live Docker canary image must be produced from S48.
* The compiled helper `go test -c` artifact must be produced from S48.
* The production `tovarisch-memory-lab` CLI build must come from S48.
* All claim fields stamped into those artifacts must reference
  `S48` (commit `9d431ab83c168bb6f880a32fcf45c36d12fdc0d8`, tree
  `793f86e6d20c9dd147bdf73e2a763c3f1bfc67b2`) — not S46, not S47.

This erratum freezes that authority so CORRECTION49 cannot
accidentally regress to S46 or S47.
