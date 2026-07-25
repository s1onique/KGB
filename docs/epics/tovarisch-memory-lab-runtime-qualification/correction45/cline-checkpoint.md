# CORRECTION45 Cl/MiniMax Checkpoint

## Status

CORRECTION45 PARTIAL: helper smoke test PASSED; production CLI run
failed evidence rejection.

## Branches explored

- `feat/memory-lab/canary-matrix-qualification01-correction45` (recommended)

## Subject Identities

| Identity | Value |
| --- | --- |
| S45 | efa752f0c9a133bac4969bb69cba6680c2b04662 |
| ST45 | 0e456806eb0a6fb2b78707f3192519b10e19ff79 |
| S45_parent | eefa177dfb54068a8dc6521017376f433a9347a5 (A44) |

## Lines cleared

- S44/E44/A44 read and verified
- A44-evidence-erratum.md produced
- placeholder test removed
- production call-graph seam extracted (control_sequence.go)
- recording DockerExecAPI fixture created
- 19 production call-graph tests added
- static legacy-authority regression test added
- canary HTTP handlers updated to canonical payload format
- live smoke test refactored to use RunCanonicalControlSequence
- runWorkload refactored to use RunCanonicalControlSequence
- S45 source committed (multiple amends)
- canary image built from S45 + vcs.modified=false
- helper binary built from S45 + vcs.modified=false
- CLI binary built from S45 + vcs.modified=false
- helper live smoke test PASSED (all observations)
- production CLI run failed evidence (PARTIAL)

## Lines blocked

- production CLI runWorkload Reachability copy did not reach
  the verification consumer under the current lifecycle ordering
- MEMLAB-08C is BLOCKED until CORRECTION46 resolves the
  CLI runWorkload producer-verifier contradiction

## Stop conditions triggered

- "the CLI run fails" — STOP 21
