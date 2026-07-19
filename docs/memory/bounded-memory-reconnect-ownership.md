# Bounded-Memory Reconnect Ownership

## Scope

This document is the corrected ownership contract for the bounded-memory
reconnect instrumentation. It supersedes the previous ownership notes
that:

- bound a separate `HandleOracle { probe, ledger }` aggregate at `init`
  (now redundant);
- exposed `ConnectorProbe` and `HandleLedger` as parallel authoritative
  structs callers could rename (no longer reachable from outside the
  `runtime/` package);
- left the oracle's identity implicit by accepting an oracle argument at
  `init(allocator, oracle)` (replaced with a heap-owned self-authoritative
  state);
- recorded only the `ptr` half of `ReconnectHandle` in the state's
  `active_handle`, allowing a forged release_fn to invoke a different
  physical cleanup path (corrected in the FA pass: the state now
  stores the full `{ptr, release_fn}` token and verifies both halves
  on release).

## Authoritative state

```text
tovarisch/src/runtime/
  allocation_tracker.zig                       # public surface (re-exports only)
  allocation_tracker_internal.zig              # opaque state + accessors
                                                #   + trackingAllocator factory
                                                #   + adoptHandle / releaseHandle
                                                #     (full-token identity check)
                                                #   + finishReconnectBoundary
  allocation_tracker_destroy.zig               # DestroyError + complete
                                                #   destroy-time validator
                                                #   (all three lifetimes)
  allocation_tracker_snapshots.zig             # MemorySnapshot /
                                                # ResourceSnapshot /
                                                # HandleSnapshot + read-only
                                                # accessor surface
                                                #   (HandleSnapshot.active_handle
                                                #    is now ?ReconnectHandle)
  allocation_tracker_connector_probe.zig       # ReconnectHandle (full release
                                                #   token: { ptr, release_fn })
                                                #   + reconnect_handle_release_fn
  allocation_tracker_tracking_allocator.zig     # producer-side TrackingAllocator
                                                #   (PRIVATE; not re-exported)
```

External code never sees the concrete state struct, the probe, the
ledger, or the oracle aggregate. The only observable handle-side type is
the full `ReconnectHandle { ptr, release_fn }` token. The state itself
owns:

- `active_handle: ?ReconnectHandle`           (full token, both halves)
- `handles_acquired: u64`
- `handles_released: u64`
- `release_calls: u64`

and is the SOLE authority for these counters. There is no parallel
struct a caller can substitute to shadow the state.

## Public API contract

```text
external code can request a fresh state         (init)
external code can destroy the state            (deinit)
external code cannot obtain StateImpl           (not re-exported)
external code cannot drive handle counters     (must call adoptHandle / releaseHandle)
external code cannot request a classified allocator outside trackingAllocator
```

`ReconnectMemoryStateStorage` is gone. The state is heap-allocated and
`opaque {}`. Connectors obtain a `*ReconnectMemoryState` exclusively
through `init(allocator)`. The layout is sealed.

### Production wiring contract (FA pass)

```zig
// loadConfigAndBgp — every production bundle must install state.
bundle.reconnect_memory_state = try allocation_tracker.init(allocator);

// cleanupBgpBundle — and destroy it LAST, after every handle, timer,
// socket, and classified allocation has been released.
destroyMemoryState(bundle, allocator);
```

The `reconnect_ownership` module centralises the pair as
`installMemoryState` / `destroyMemoryState` so callers cannot forget
either step. The `BgpServeBundle.reconnect_memory_state` field stays
optional only for backward compatibility with hand-constructed test
bundles; every code path in `serve_integration.zig` now installs
state as part of `loadConfigAndBgp`.

## Authoritative handle lifecycle

Connectors produce a `ReconnectHandle { ptr, release_fn }`; they do NOT
mutate any handle-side counter. The orchestrator owns the transition:

```zig
// Phase 1
const handle = try connector.acquire(tcp_config);

// Phase 2 — MUST happen BEFORE the real connect.
try allocation_tracker.adoptHandle(state, handle);

// Phase 3 — physical connect. Failure here is balanced by the
// `errdefer releaseOnErrdefer` registered below the adopt call.
errdefer reconnect_ownership.releaseOnErrdefer(bundle, handle);

const tcp = try connector.finish(handle, tcp_config);

// On success, the orchestrator takes sole ownership of `tcp` and
// remembers the handle for the next closeForReconnect cycle.
bundle.tcp = tcp;
bundle.active_connector_handle = handle;
```

`adoptHandle` rejects a second active handle with `error.HandleAlreadyActive`.

`releaseHandle` verifies the supplied `handle` matches the state's
recorded token on BOTH `ptr` AND `release_fn`. A forged handle that
shares the legitimate `ptr` but supplies a different `release_fn`
returns `error.WrongHandle` without invoking either physical callback,
so a mis-wired caller cannot accidentally force a different cleanup
path. After identity verification, `releaseHandle` invokes
`handle.release_fn(handle.ptr)` exactly once, and only then increments
the release counter and clears the active record. A wrong handle
returns `error.WrongHandle` without firing the physical callback (so a
stale token cannot double-close).

## Atomic boundary coordinator

`finishReconnectBoundary(state, baseline_sockets)` is the single entry
point for "generation complete". It checks the state's own counters
(`active_handle == null` AND `handles_acquired == handles_released`)
BEFORE committing the generation. `HandleLeak` covers both an
outstanding active handle and a mismatched acquire/release total.

## Cleanup-path ownership (FA pass)

The previous helper silently swallowed `releaseHandle` errors on the
grounds that the original connection error is what the caller cares
about. The errdefer runs precisely because control is leaving
through an error path, so the cleanup is the LAST chance the state
has to stay consistent before the caller observes the original
error. Silent masking would let the caller see a "successful"
cleanup while the active-handle record is corrupted.

The current `releaseOnErrdefer` (in `reconnect_ownership.zig`) is
fail-loud:

```zig
fn releaseOnErrdefer(bundle: anytype, handle: allocation_tracker.ReconnectHandle) void {
    const state = bundle.reconnect_memory_state orelse {
        // No state installed. Hard error for any reconnect-capable
        // bundle — every production bundle MUST install state.
        handle.release_fn(handle.ptr);
        @panic("reconnect cleanup reached bundle without a memory state; production MUST call installMemoryState");
    };
    allocation_tracker.releaseHandle(state, handle) catch |err| switch (err) {
        error.NoActiveHandle, error.WrongHandle, error.HandleAlreadyActive =>
            @panic("reconnect error cleanup disagreed with handle state"),
    };
}
```

A releaseHandle failure during in-flight cleanup is evidence of state
corruption, not noise. The daemon panics instead of continuing with
a corrupted active-handle record.

## Test coverage

The 10,000-generation proof asserts absolute counts so deleting every
acquire/release pair cannot return to a vacuous `0 == 0`. See the
companion ACT document for the full matrix; highlights:

- `production connector: failed real attempt is observed by the state oracle` —
  single failed reconnect balances the counters and clears `acquire_inflight`.

- `production connector: two consecutive real failed reconnects both reach realFinish` —
  back-to-back failed reconnects both reach `realFinish`; counter
  pairs balance to 2=2; `acquire_inflight` returns to `false` between
  attempts. Without `installMemoryState`, the second `realAcquire`
  would refuse at the gate with `HandleAlreadyActive`.

- `state oracle: releaseHandle rejects a forged release_fn on the same pointer` —
  a forged handle with the legitimate `ptr` but a different
  `release_fn` is rejected with `error.WrongHandle`, neither callback
  runs, `handles_released` stays at 0, and the state's recorded token
  still matches the legitimate handle.

- `state oracle: releaseHandle rejects a wrong handle without invoking release_fn` —
  pointer-only mismatch is also caught (pre-existing test, retained
  for invariant coverage of the simpler case).

## Static gate

```text
cmd/verify-allocation-tracker-imports/main.go                  CLI boundary
internal/tooling/allocationtrackerimports/scanner.go           inventory + scan
internal/tooling/allocationtrackerimports/selftest.go          fixture suite
```

The native Go gate builds distinct NUL-delimited cached and untracked inventories
with the explicit root-inclusive Git glob
`:(glob)tovarisch/src/**/*.zig`, then filters the trusted
`tovarisch/src/runtime/` package. A narrow lexical pass masks Zig line
comments, ordinary strings, character literals, and multiline-string
lines before locating every real `@import` token. Outside the trusted
package, only one plain quoted literal (plus optional whitespace and a
trailing comma) is approved. Concatenation, identifiers, escaped
literals, and every other unparsed argument shape fail closed. Approved
literals are resolved against the importer and compared with all FIVE
canonical private paths.

The hermetic self-test has 15 fixture classes plus two I/O mutations. It
proves distinct cached/untracked membership, all five private targets,
root-level source coverage, multiline literals, path normalisation,
concatenation and identifier rejection, lexical masking, and both
tracked/untracked runtime controls. Its same-basename control creates
`tools/allocation_tracker_internal.zig` plus `tools/decoy_importer.zig`,
runs `zig test tools/decoy_importer.zig`, and verifies the scanner allows
the existing, compilable decoy. Missing, unreadable, directory, or
repository-escaping importer entries fail closed.

`quality_gate.sh` is 447 physical lines and stays below the 450-line hard
limit; its logical shell count remains at the doctrine baseline ceiling.
Every focused ACT source/tool file also remains below its hard limit.
