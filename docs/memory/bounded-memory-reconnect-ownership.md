# Bounded Memory Reconnect - Ownership Inventory (CURRENT)

## ACT Scope

This ACT (`ACT-TOVARISCH-BOUNDED-MEMORY-INSTRUMENTATION01`) covers ONLY:

- `tovarisch/src/runtime/allocation_tracker.zig` (instrumentation core)

Deferred to successor ACTs:

- `ACT-TOVARISCH-BOUNDED-MEMORY-STATUS01`: JSON renderer + integration into /status output
- `ACT-TOVARISCH-BOUNDED-MEMORY-RECONNECT-PROOF01`: 10,000-generation production reconnect execution
- `ACT-TOVARISCH-BOUNDED-MEMORY-CONCURRENCY01`: locking/snapshot handoff model

Test helpers in `tovarisch/src/bgp/reconnect_stress_tests.zig` are exercised by
the aggregate test graph but contain no production lifecycle integration.

## Structural Owners (AllocationOwner)

1. **process** — Daemon-wide singleton data (config, bundle, runtime)
2. **bgp_subsystem** — Long-lived BGP subsystem state (BgpServeBundle, allocator)
3. **bgp_session** — One BGP session (peers + per-session caches)

## Temporal Lifetimes (AllocationLifetime)

1. **permanent** — Process-lifetime (never freed except at shutdown)
2. **reconnect_generation** — One BGP reconnect generation
3. **operation** — One runOnce or status render

## Types and Authority

| Type                     | Purpose                                          | Authority         |
|--------------------------|--------------------------------------------------|-------------------|
| AllocationOwner          | 3-variant enum (compile-time contiguous)        | single            |
| AllocationLifetime       | 3-variant enum (compile-time contiguous)        | single            |
| OwnerMetrics             | Per-cell live/peak/total counters               | value type        |
| AllocationMetrics        | Per-cell × per-lifetime table                    | value type        |
| TrackingAllocator        | Real std.mem.Allocator vtable wrapper          | wrapper           |
| BoundedResourceCounters  | Socket/timer/collection counters               | value type        |
| ReconnectMemoryState     | Single generation authority + finalize         | SINGLE generation |
| MemorySnapshot           | JSON-safe view with signed baseline delta       | projection        |
| ResourceSnapshot         | JSON-safe resource view                          | projection        |

`ReconnectMemoryState.generation` is the single authority for the reconnect
generation counter. `AllocationMetrics` does NOT store a generation mirror —
the snapshot and accounting paths read from `state.generation` directly.

## Resource Counters (BoundedResourceCounters)

| Resource           | Capacity | Operation     | Invariant                                     |
|--------------------|---------:|---------------|-----------------------------------------------|
| active_sockets     |     0..N | Open/Close    | Open + Close pair (programmer errors panic)   |
| active_timers      |     0..N | Start/Stop    | Start + Stop pair (programmer errors panic)   |
| error_history      |       16 | Reserve/Release | reject-newest, count maintained             |
| retry_collection   |       64 | Reserve/Release | reject-newest, count maintained             |

NOTE: Current implementation is occupancy-based (counter + Reserve/Release),
NOT a ring with overwrite-oldest semantics.

## Generation Transitions (Failure-Atomic)

```text
finishGeneration(baseline_sockets)
  -> step 1: compute next_generation
  -> step 2: validateForGeneration()      (allocations per-gen empty)
  -> step 3: validateBaseline()           (total_live_bytes == captured baseline)
  -> step 4: validateGenerationComplete() (resource baseline)
  -> step 5: ONLY if all pass, commit:
              allocations.commitGeneration(next_generation) (baseline + reset per-gen)

              state.generation = next_generation
```

If any validation fails, `state.generation` MUST stay unchanged.
Tests:
- `failure-atomic on resource leak`: SocketLeak → gen preserved
- `BaselineDrift detected on permanent-classification growth`: gen preserved
- `finishGeneration rejects unfreed allocation (failure-atomic)`: gen preserved

## Overflow Policy

All counter increments use `std.math.add(...) catch @panic("...overflow")`.
No silent clamping. Underflow panics (`recordResizeShrink`, `recordFree`).

## Snapshot Field Set (when integrated to /status)

```
live_bytes                       u64
peak_bytes                       u64
reconnect_allocation_count       u64
reconnect_free_count             u64
reconnect_live_bytes             u64
reconnect_generation             u64   (from state.generation)
baseline_live_bytes              ?u64
baseline_delta_bytes             i128 (signed; growth +, reduction -, exact 0)
active_sockets                   u32
peak_sockets                     u32
active_timers                    u32
peak_timers                      u32
error_history_count              u32
error_history_capacity           u32
retry_collection_count           u32
retry_collection_capacity        u32
```

## Zig 0.16 Observations (Promoted to Field Manual)

### Symptom: `inline for (... fields) |f, i|` fails with "extra capture"
- Wrong assumption: 0.16 supports two captures in inline for over `.fields`
- Working fix: runtime loops `for (0..N) |i|`, single-capture inline for
- Files affected: `allocation_tracker.zig`

### Symptom: `std.io.fixedBufferStream` not available
- Wrong assumption: 0.14 import path still valid
- Working fix: `std.ArrayList(T).initCapacity(allocator, n)` for tests

### Symptom: documentation comments on `comptime` blocks rejected
- Wrong assumption: doc comments work above comptime blocks
- Working fix: move doc comment above the comptime keyword or omit

### Symptom: `try` on std.math.add inside non-error function
- Wrong assumption: error unions propagate freely
- Working fix: use `catch @panic(...)` instead
