CORRECTION48 — A48 Attestation
==========================

Subject identities
------------------

```
S48 commit  = 9d431ab83c168bb6f880a32fcf45c36d12fdc0d8
S48 tree    = 793f86e6d20c9dd147bdf73e2a763c3f1bfc67b2
S48 parent  = 7934a8a8bf71a10df845028fa447d965bc3e1490  (S47)

E48 commit  = d79dc14162a0e106585e77db7f4c77dae62ee810
E48 tree    = 742710ae469842d371ed8817c48f67c6f4680180
E48 parent  = 9d431ab83c168bb6f880a32fcf45c36d12fdc0d8  (S48)

A48 parent  = d79dc14162a0e106585e77db7f4c77dae62ee810  (E48)
A48 tree    = TBD (this commit, E48 + A48 attestation files)
```

Attestations
------------

* S48 is the only code-bearing source subject for the CORRECTION48
  qualification-harness. E48 records every per-step evidence
  file listed in the spec. A48 contains only the three attestation
  files (attestation.md, evidence-file-sha256s.txt,
  final-git-status.txt).

* `metadata_path_authority: CLOSED` — the canonical resolver lives
  in `tovarisch/labs/memory/internal/buildmetadata/path.go`. The
  required `ResolveCanaryMetadataPath(MetadataPathOptions) (string,
  error)` signature is exported. The required test set
  (ExplicitWins, EnvironmentFallback, RepositoryCompatibilityFallback,
  ExplicitMissingFails, EnvironmentMissingFails, DirectoryRejected,
  SymlinkResolved, BrokenSymlinkRejected, NonRegularFileRejected,
  UnknownSchemaRejected, InvalidMetadataRejected) all pass.

* `provenance_clean_policy: CLOSED` — the typed
  `ProvenanceCleanPolicy` enum is wired into `ProvenanceOptions` and
  replaces the legacy `RequireClean bool`. The closure producer
  (tovarisch-memory-lab CLI and helper test executable) both build
  with `ProvenanceRequireClean`. The unknown-policy rejection, the
  dirty-state rejection (tracked/staged/untracked/vcs.modified=true),
  and the `ProvenanceIgnoreWorktree` `QualifyingObservation=false`
  downgrade are all in place and tested.

* `external_artifact_root: CLOSED` —
  `NewQualificationArtifactPaths` rejects relative paths, paths
  beneath the source checkout, paths whose ancestor chain
  contains `.git`, and paths where helper and production collide.

* `helper_role_proven: true` — the compiled helper test executable
  contains the requested test symbol
  `TestQualifiedRun_RuntimeCannotMutateCallerConfig`. Verified
  via the substring check in the verifier (see
  `helper-test-list.txt`, `artifact-role-verifier.txt`).

* `production_role_proven: true` — the production CLI's `--help`
  flag returns exit 0 and prints the usage banner. Verified at
  build time by `productionHelpSucceeded` (see `production-help.txt`).

* `paths_distinct: true` — the recorded helper and production
  absolute paths differ (different parent directory names:
  `tovarisch-memory-lab-helper.test` vs `tovarisch-memory-lab`).

* `inodes_distinct: true` — the recorded helper inode (`2670550`)
  and production inode (`2670241`) differ.

* `hashes_distinct: true` — the recorded helper SHA-256
  (`81e509b12002146f33512094250104b4f92d7f935dd5fa66e2c35e05f31105cf`)
  and production SHA-256
  (`75907a65d7c2b21a326cab06cf91095f42cb523bee3fa5af051874c6802c918b`)
  differ.

* `source_worktree_clean: true` — `git status --short` is empty
  before the build, after the build, and after the verifier.
  `git diff --check` reports no whitespace or conflict errors
  against S48 HEAD (modulo one trailing-whitespace warning on
  `production-buildinfo.txt:3` from the `go version -m` output
  embedded verbatim — not a source code defect).

External gate failure
---------------------

`make gate` (from the S48 worktree) surfaces one pre-existing
unrelated UVB-76 failure:
`hulk-uvb76-artifact-producer-gate` reports missing
`uvb76/internal/artifactio.WriteRedacted*` calls in
`uvb76/cmd/uvb76-targets-crash-lab/...` and
`uvb76/cmd/uvb76-tcp-diag-telemetry-lab/...`.

The four external-failure rules are satisfied:

* `S47_to_S48_delta_in_failure_paths` = **zero** (the failing
  paths were never touched by S48).
* `verifier_modified` = **false** (the hulk-uvb76 verifier was
  not modified by S48 or E48).
* `verifier_weakened` = **false**.
* `failure_reported_as_pass` = **false** (this attestation
  records the failure verbatim; `make-gate.txt` preserves the
  raw output).

The script-doctrine verifier also surfaces a worktree-only
internal-error (`.git/hooks` is a file in a worktree, not a
directory) which is fixed by running the gate from the main
repo instead of a worktree.

Closure
-------

```
CORRECTION48   PARTIAL_READY_FOR_LIVE_QUALIFICATION
S48 verified:   yes (P0-9 through P0-12)
make gate:     one pre-existing unrelated UVB-76 failure remains
               external and satisfies the four external-failure rules
next:          CORRECTION49 performs the live canary image build,
               the live helper test, and the live production CLI run.
```
