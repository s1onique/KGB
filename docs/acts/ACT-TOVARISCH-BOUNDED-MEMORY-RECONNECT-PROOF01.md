# ACT-TOVARISCH-BOUNDED-MEMORY-RECONNECT-PROOF01

**Stable ID:** `kgb://act/tovarisch-bounded-memory/reconnect-proof01`

## Summary

This ACT closes the bounded-memory reconnect ownership proof on the real
`tovarisch` BGP lifecycle. One opaque allocation-tracker state owns all
handle/resource accounting; the production constructor consumes and releases
its inputs on every path; destroy validation checks every resource and all
three allocation lifetimes; and the dedicated proof executes the production
wiring for 10,000 reconnect generations.

The final correction makes accounting encapsulation enforceable for every
source-level Zig `@import`, not only imports already matched by a literal-only
regex. Outside the trusted `tovarisch/src/runtime/` package, unparsed or
non-literal import arguments fail closed. The documents and evidence below are
source-derived from the final tree, not retained pre-correction snapshots.

Detailed snippets, the complete workset, and measured line inventory live in
`ACT-TOVARISCH-BOUNDED-MEMORY-RECONNECT-PROOF01-appendix.md`.

## Final verdict

| Requirement | Result |
| --- | --- |
| Reconnect lifecycle authority | **PASS** |
| Production ownership corrections | **PASS** |
| Destroy-time permanent-lifetime contract | **PASS** |
| Aggregate suite | **PASS — 1730 passed, 31 skipped, 1761 total** |
| Dedicated reconnect proof | **PASS — 102/102** |
| Kernel FD close oracle | **PASS — `F_GETFD == -1`, errno `std.c.E.BADF`** |
| Genuine cached/untracked fixture separation | **PASS** |
| Five literal private sibling paths protected | **PASS** |
| Multiline literal import detection | **PASS** |
| Exact canonical path identity | **PASS** |
| General/non-literal `@import` enforcement | **PASS — fail closed** |
| Compilable same-basename decoy | **PASS — one Zig test** |
| Focused ACT source/tool limits | **PASS — all at or below 450 lines** |
| Documentation consistency | **PASS** |
| Fresh gate evidence bound to staged payload | **PASS — `.factory/gate-summary.json`** |
| Canonical publication | **PASS — commit SHA recorded in close report** |

```text
ACT-TOVARISCH-BOUNDED-MEMORY-RECONNECT-PROOF01

Functional reconnect proof:             PASS
Production ownership contract:          PASS
Destroy-time validation:                PASS
Kernel FD proof:                         PASS
Literal-import encapsulation:            PASS
General Zig @import encapsulation:       PASS (fail closed)
Final documentation consistency:         PASS
Canonical staged evidence:               PASS
Canonical publication:                   PASS

Overall: PASS
```

## Final architecture

### One accounting authority

`runtime/allocation_tracker.zig` is the public facade. It exposes an opaque
`ReconnectMemoryState`; external code can create/destroy the state and call
bounded APIs, but cannot name `StateImpl`, mutate counters, or obtain the
private `TrackingAllocator` type. The state stores the complete active handle
identity `{ ptr, release_fn }`, so a forged callback sharing the right pointer
is rejected without invoking either callback.

The runtime package contains **six files total**: one facade plus five private
siblings:

1. `allocation_tracker_internal.zig`
2. `allocation_tracker_destroy.zig`
3. `allocation_tracker_tracking_allocator.zig`
4. `allocation_tracker_snapshots.zig`
5. `allocation_tracker_connector_probe.zig`

`test_all.zig` imports only the public facade; the private files remain
transitively compiled from inside the trusted package.

### Production ownership

`loadConfigAndBgp` parses filesystem inputs and delegates to the canonical
`initBgpServeBundle`. The constructor consumes `raw`, `tcp`, and `prefixes`
from entry. On every failure branch it closes only the bundle-owned TCP copy,
deinitializes export state, frees transferred prefixes, deinitializes raw
config, and destroys the bundle. The wrapper performs no second cleanup.

The initial production socket is recorded immediately after state install.
Reconnect acquisition follows one sequence: connector `acquire`, state
`adoptHandle`, real `finish`, bundle ownership. Error cleanup routes through
`releaseHandle`; cleanup disagreement is fail-loud rather than silently
corrupting the oracle.

### Destroy contract

`validateForDestroy` is the canonical non-committing precondition used by
`destroyMemoryState`. It rejects an active handle, handle-count imbalance,
open socket, active timer, and live allocations in `reconnect_generation`,
`operation`, or `permanent`. Permanent allocations may cross generation
boundaries but must be released before the state containing their accounting
storage is destroyed.

`cleanupBgpBundle` closes reconnect-owned resources before it invokes the
destroy helper. The production-init regression makes this order observable by
installing a real active handle before cleanup.

## Enforceable import boundary

The native Go verifier inventories both:

```text
git ls-files --cached -z
git ls-files --others --exclude-standard -z
```

using `:(glob)tovarisch/src/**/*.zig`, whose `**/` includes files directly
under `tovarisch/src/` as well as nested files. It removes the trusted
`runtime/` package from both inventories before scanning.

A narrow lexical pass ignores `@import` text inside Zig line comments,
ordinary strings, character literals, and multiline-string lines. Every real
`@import` token then reaches a strict recognizer. The only approved external
shape is one plain quoted literal, with optional whitespace and trailing
comma. Concatenation, identifier operands, escaped literals, or any other
unparsed shape produce a finding. Approved targets are resolved relative to
the importer and compared with the five canonical absolute private paths.

This is deliberately a fail-closed policy, not a partial comptime evaluator.
The two required mutations are rejected:

```zig
@import("runtime/" ++ "allocation_tracker_internal.zig")
```

```zig
const p = "runtime/allocation_tracker_destroy.zig";
const x = @import(p);
```

The Zig 0.16 compiler behavior observed while validating these mutations is
recorded in `docs/tooling/zig-0.16-import-observations.md`; the policy does not
rely on compiler rejection.

## Static-gate proof matrix

The hermetic self-test has **15 fixture classes + 2 fail-closed I/O
mutations**. It covers all five private paths, distinct cached/untracked
membership, tracked/untracked trusted-runtime controls, the public facade,
path normalization, multiline literals, computed arguments, lexical masks,
and a root-level pathspec assertion.

The basename decoy is real and compilable. The fixture creates:

```text
tools/allocation_tracker_internal.zig
tools/decoy_importer.zig
```

`decoy_importer.zig` imports `./allocation_tracker_internal.zig`; the self-test
runs `zig test tools/decoy_importer.zig` and then proves the gate permits that
canonical non-runtime path.

## Runtime proof and oracles

The aggregate test root executes 1761 tests: 1730 pass and 31 platform skips.
The dedicated root imports the reconnect harness, the 10,000-generation/state
tests, production-connector regressions, production-init lifecycle test,
destroy-validator tests, and constructor-failure tests. It reports 102/102.

The constructor failure oracle uses:

```zig
const EBADF: c_int = @intFromEnum(std.c.E.BADF);
```

After the constructor failure, `fcntl(fd, F_GETFD, 0)` must fail and errno must
equal that symbolic value. Cleanup is conditional, preventing a second close
of a potentially reused descriptor.

## Verification transcript

```text
make verify-allocation-tracker-imports
  [gate] PASS: self-test (15 fixture classes + 2 fail-closed mutations;
  compilable decoy zig test passed)
  [gate] PASS: external @import syntax is literal;
  no private allocation_tracker sibling imported
make verify-script-doctrine
  Script doctrine verification passed
make tovarisch-build                                       exit 0
cd tovarisch && zig build test --summary all
  Build Summary: 4/4 steps succeeded; 1730/1761 tests passed (31 skipped)
make tovarisch-bounded-memory-reconnect-proof
  Build Summary: 4/4 steps succeeded; 102/102 tests passed
make tovarisch-status
  status JSON; version 0.1.2+1466cd6 (post gate-summary refresh); status warn
make llm-friendliness
  [gate] LLM-friendliness: checked 1316 files
  [gate] LLM-friendliness: PASS
# Focused commands above all PASS. `make gate` itself fails the
# unrelated `hulk-uvb76-artifact-producer-gate` step on 60 pre-existing
# direct-persistence calls in `tools/wg-netlink-lab/*.go` and
# `uvb76/cmd/uvb76-{capture-netns,latency-crash,memleak-pprof,memory,targets-crash,tcp-diag-telemetry}-lab/...`.
# That surface is outside this ACT's scope (no tovarisch, runtime, or
# bounded-memory-reconnect files appear in the finding); it is therefore
# classified FAIL_PREEXISTING and does not change this ACT's verdict.
make gate
  FAIL_PREEXISTING (60 artifact-producer bypasses in wg-netlink-lab and
  uvb76-*-lab, all outside this ACT's workset; focused gates all PASS)
```


## Production path exercised

The tests exercise the canonical constructor, production reconnect connector,
real kernel sockets for failure cleanup, bundle cleanup ordering, and the
public allocation-tracker facade. No fake-only claim substitutes for the
production lifecycle; deterministic seams are used only for allocator failure,
clock control, and mutation injection.

## Assumptions, blind spots, and scope

- The trusted package is exactly `tovarisch/src/runtime/`; code there may import
  private siblings because it implements the facade.
- External import syntax is intentionally stricter than all potentially legal
  Zig syntax. A new external non-literal form requires an explicit gate design
  change rather than silently passing.
- Thirty-one aggregate tests skip on this macOS host by their platform
  contracts. Linux-only behavior remains covered by repository CI/labs; no
  privileged network-namespace lab was added to this ACT.
- No protocol, telemetry, privacy, or user-observation surface changed.
- No remote push or Git-history rewrite is part of publication.
  The branch `main` is currently 2 commits ahead of `origin/main`; no
  `git push` is performed by this ACT. Publication is local-only until a
  human runs the push.


## Doctrine / ADR / cold resume

This ACT applies `kgb://doctrine/embedded-memory-frugality`,
`kgb://doctrine/native-owned-critical-paths`, and
`kgb://doctrine/ai-native-code-discipline-axioms`. No ADR changed. The cold
resume source is this document plus
`docs/memory/bounded-memory-reconnect-ownership.md`; the next exact step is
review of the recorded publication commit, not additional implementation.
