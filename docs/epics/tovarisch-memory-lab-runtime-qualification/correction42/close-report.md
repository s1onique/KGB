# CORRECTION42 Close Report

## Summary

Finished the post-attach Docker exec transport: nil runtimes and malformed hijacked responses fail closed; acquired attachments have one-close authority; context completion interrupts blocked reads and all watcher goroutines are joined; a constant-state framing guard validates declared Docker frame sizes before `stdcopy.StdCopy`; independent cumulative writers remain bounded; and `Running=true` inspect results are rejected before envelope decoding.

## Status

```yaml
CORRECTION41: SUPERSEDED_BY_CORRECTION42
CORRECTION42: CLOSED_DOCKER_TRANSPORT_HERMETIC
parent_correction03: PARTIAL
MEMLAB_08B: IN_PROGRESS
MEMLAB_08C: BLOCKED
next: CORRECTION43
```

## Production path exercised

The real Docker v25 attach response is shape-validated, the real connection is closed idempotently, and valid framed bytes still pass unchanged to Docker's canonical `stdcopy.StdCopy`. Tests use exact Docker headers, fragmented reads, blocked connection-style readers, and deterministic close synchronization. No live daemon was used.

## Verification

Focused vet, verbose tests, race tests, count-100 tests, and repository-wide short tests passed. Exact outputs are committed here. `make gate` failed on the same 60 pre-existing UVB-76 artifact-writer bypass findings and is not represented as a pass. The S41→S42 delta in every reported failure directory is zero; no verifier was weakened.

## Gate blind spots / accepted risks

Production callers, qualified lifecycle, legacy protocol deletion, reachability evidence, image rebuilding, live Docker, and MEMLAB-08C remain out of scope. The frame guard accepts Docker stream identifiers 0–3 and applies stdout limits to stdin/stdout and stderr limits to stderr/system-error as required.

## Doctrine / ADR impact

No doctrine or ADR changes. Bounded state and native SDK reuse conform to existing memory-frugality and native-owned-critical-path doctrine.

## Cold resume / next exact step

CORRECTION43 must migrate the production CLI and qualified lifecycle, delete legacy Dockerlab protocol authority and permanent v2 naming, and add operation-level canonical reachability plus strict evidence mutation matrices.

## Zig 0.16 observations

None. No Zig source or tooling was modified.
