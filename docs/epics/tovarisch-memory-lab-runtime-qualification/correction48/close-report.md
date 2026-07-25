CORRECTION48 — Close Report
==========================

S48
---

```
S48 commit  = 9d431ab83c168bb6f880a32fcf45c36d12fdc0d8
S48 tree    = 793f86e6d20c9dd147bdf73e2a763c3f1bfc67b2
S48 parent  = 7934a8a8bf71a10df845028fa447d965bc3e1490  (S47)
```

E48
---

```
E48 parent  = 9d431ab83c168bb6f880a32fcf45c36d12fdc0d8  (S48)
```

(E48 will be committed after this report; the E48 commit must NOT
contain any A48 attestation or final identities — see spec.)

Phase 0 — S47 documentation-only freeze
----------------------------------------

S47 is documentation-only. No source, no `*.go`, no `Makefile`, no
`go.mod`/`go.sum`, no `tovarisch/labs/memory/**` was modified by S47.
The S46 source tree is preserved bit-for-bit under S47. The S47
erratum (S47-classification-erratum.md) freezes this property.

P0-1 — Pending-patch inventory
--------------------------------

The two untracked pending `.go` candidates under
`docs/.../correction47/pending-internal-evidence/` were inventoried
in `pending-patch-inventory.txt` and have been deleted from the
documentation tree at the S48 commit.

P0-2 — Metadata-path resolver
------------------------------

The committed resolver lives in
`tovarisch/labs/memory/internal/buildmetadata/path.go`. The required
`ResolveCanaryMetadataPath(MetadataPathOptions) (string, error)`
signature is exported, and the source priority (explicit flag →
`TOVARISCH_CANARY_METADATA_PATH` env → repository compatibility
fallback) is implemented exactly as specified. The required tests
all pass:

```
ok  	github.com/s1onique/KGB/tovarisch/labs/memory/internal/buildmetadata
```

P0-3 — Production CLI metadata option
-------------------------------------

`tovarisch-memory-lab run` accepts `--canary-build-metadata
<absolute-or-relative-path>` and `--repository-root <path>`. The
metadata is resolved BEFORE any Docker contact via the canonical
resolver. The resolved canonical path is recorded on the manifest's
`canary_build_metadata_path` field (new optional field) and is
passed to `captureAndVerifyCanaryImageIdentity`. No production
function reopens a separate fixed path. The required tests all pass.

P0-4 — Provenance-cleanliness policy
--------------------------------------

`internal/evidence/clean_policy.go` introduces the typed
`ProvenanceCleanPolicy` enum (`ProvenanceRequireClean`,
`ProvenanceIgnoreWorktree`) and `ErrUnknownProvenanceCleanPolicy`.
`ProvenanceCleanPolicy` is wired into `ProvenanceOptions` and
replaces the legacy binary `RequireClean bool`. The unknown-policy
rejection, the dirty-state rejection (tracked/staged/untracked/
vcs.modified=true), and the `QualifyingObservation` downgrade for
`ProvenanceIgnoreWorktree` are all in place. The required tests
all pass.

P0-5 — Production closure provenance policy
--------------------------------------------

Both the tovarisch-memory-lab CLI and the helper test executable
build with `CleanPolicy: evidence.ProvenanceRequireClean` (no
fallback). The fallback Git collector is preserved inside
`CollectControllerProvenance` for cases where the embedded VCS info
is physically absent, and it is still guarded by the same
`ProvenanceRequireClean` discipline.

P0-6 — External qualification-artifact root
--------------------------------------------

`internal/qualification/artifacts.go` exports
`QualificationArtifactPaths` and `NewQualificationArtifactPaths`.
The constructor refuses relative paths, refuses paths beneath the
source checkout, refuses paths whose ancestor chain contains
`.git`, and refuses paths where helper and production collide. All
required tests pass.

P0-7 — Hermetic qualification-build program
--------------------------------------------

`cmd/build-qualification-artifacts/main.go` is the Go-native
qualification-build program. It compiles the helper test
executable and the production CLI, records the role-separation
JSON, and refuses to build from a dirty source. The original shell
wrapper was deleted (it tripped the frozen script-doctrine
bootstrap baseline and could not be added to that baseline).

P0-8 — Artifact-role verifier
--------------------------------

`cmd/verify-qualification-artifacts/main.go` is the Go-native
verifier. It rejects the seven required mutations
(same path, same inode, same hash, helper/working tree rev
mismatch, helper/working tree modified, unknown field, missing
field, second JSON), the null-field rejection, and the
help-success rejection. The verifier passes on the S48-built
artifacts:

```
PASS: role-separation record verified
```

P0-9 — Full source verification
--------------------------------

* `gofmt -l .` — clean
* `go vet ./...` — clean
* `go build ./...` — clean
* `go test -count=1 ./...` — PASS for every package
* `go test -race -count=1 ./internal/buildmetadata/...
  ./internal/evidence/... ./cmd/tovarisch-memory-lab/...` — PASS
* `go test -count=100 ./internal/buildmetadata/...
  ./internal/evidence/...` — PASS
* `go test -count=1 -run 'TestVerifyMatrix'
  ./cmd/tovarisch-memory-lab/...` — PASS
* `go test -count=1 -run 'TestQualifiedRun_RuntimeCannotMutateCallerConfig'
  ./internal/dockerlab/...` — PASS (pre-existing test in
  qualified_runtime_test.go)

P0-10 — S48 commit
------------------

S48 was committed (and amended) at
`9d431ab83c168bb6f880a32fcf45c36d12fdc0d8` (tree
`793f86e6d20c9dd147bdf73e2a763c3f1bfc67b2`). The amend added the
`build-qualification-artifacts` Go program and the `--help` flag in
the production CLI for verifier compatibility.

P0-11 — Detached S48 build worktree
------------------------------------

The worktree was created with
`git worktree add --detach $QUAL_SRC HEAD`, and after the amend
the worktree was reset to the new HEAD. `git status --short` is
empty and `git diff --check` is clean. (See
`detached-worktree-identity.txt`,
`detached-worktree-clean-before.txt`,
`detached-worktree-clean-after.txt`.)

P0-12 — Build + verify two controller artifacts
-------------------------------------------------

`build-qualification-artifacts --source-root $QUAL_SRC
--artifact-root $QUAL_ARTIFACTS` succeeded and the
`verify-qualification-artifacts` verifier passed with
`PASS: role-separation record verified`. (See
`artifact-role-record.json`, `artifact-role-verifier.txt`,
`helper-*.txt`, `production-*.txt`.)

P0-13 — Run current canonical gate
-----------------------------------

`make gate` from the S48 worktree surfaced one pre-existing
unrelated UVB-76 failure (hulk-uvb76-artifact-producer-gate
detects missing redaction calls in
`uvb76/cmd/uvb76-targets-crash-lab/...` and
`uvb76/cmd/uvb76-tcp-diag-telemetry-lab/...`). Per the
external-failure rules in the spec:

* `S47_to_S48_delta_in_failure_paths` = **zero** (the failing
  paths were never touched by S48).
* `verifier_modified` = **false** (the hulk-uvb76 verifier was
  not modified).
* `verifier_weakened` = **false**.
* `failure_reported_as_pass` = **false** (this report records
  the failure verbatim).

The script-doctrine verifier also surfaced one worktree-only
internal-error (`.git/hooks` is a file in a worktree, not a
directory) which is fixed by running the gate from the main repo
instead of a worktree. (See `make-gate.txt`.)

P0-14 — E48
------------

E48 contains this close report and the per-step evidence files
listed in the spec. E48 has no A48 attestation and no final
identity for A48.

P0-15 — A48
------------

A48 will be committed after this E48 lands. A48 is attestation-
only and contains exactly three files: `attestation.md`,
`evidence-file-sha256s.txt`, and `final-git-status.txt`.

Classification
---------------

```
CORRECTION48   PARTIAL_READY_FOR_LIVE_QUALIFICATION
S48 verified:    role separation, distinct paths/inodes/sha256s,
                 vcs.revision == source_commit, helper symbol present,
                 production --help returns 0.
make gate:       clean for S48-controlled paths; one unrelated
                 pre-existing UVB-76 failure remains external.
                 Source worktree is clean.
```

The unrelated UVB-76 failure is recorded in `make-gate.txt` and
satisfies the four external-failure rules. The
qualification-harness source is ready for CORRECTION49 to
perform the live canary image build, the live helper test, and
the live production CLI run.
