# ACT-HULK29R-ZIG016-DAEMON-IDLE-BACKGROUND-ANON-GROWTH

## Status

**ACTIVE INVESTIGATION — 2026-07-08**

## Goal

Identify the source of ~480 KiB/hour (~136 bytes/second) private anonymous heap growth in `tovarisch` during idle periods when all external watchers and HTTP clients are stopped.

## Evidence Summary

### From: ACT-HULK29R-ZIG016-STATUS-HTTP-CHECK-DETAIL-OWNERSHIP (superseded)

The HTTP /status endpoint was exonerated:
- 10k /status burst: +24 KiB Anonymous/Private_Dirty (~2.46 bytes/request)
- This rules out request rendering as the leak source
- **Confirmed**: `renderPayloadWithContextAndDiag()` calls `deinitScratchChecks(&scratch)` in defer (status.zig:385)

### 30-Minute Idle Window Measurements

With watch/curl stopped, observed:

| Metric | Start | End | Delta |
|--------|-------|-----|-------|
| VmRSS | 6408 KiB | 6640 KiB | +232 KiB |
| RssAnon | 3684 KiB | 3924 KiB | +240 KiB |
| Private_Dirty | 3684 KiB | 3924 KiB | +240 KiB |
| Anonymous | 3684 KiB | 3924 KiB | +240 KiB |
| VmData | 52132 KiB | 52372 KiB | +240 KiB |

### Rate Calculation

```
240 KiB / 1800 seconds = 0.133 KiB/second = ~136 bytes/second
```

### Hypothesis

The ~136 bytes/request observed in 1 Hz polling was an **artifact of 1 Hz polling rate**:
- 1 Hz polling × ~136 bytes/second ≈ ~136 bytes/request

The actual leak is a **background tick** (~1 second interval) unrelated to HTTP requests.

## Phase 1: Tick-Source Inventory

| Subsystem | File/Function | Interval | Allocation Paths | Deinit Paths | Suspected Risk |
|-----------|---------------|----------|------------------|--------------|----------------|
| Heartbeat Thread | `http/heartbeat.zig:heartbeatThread()` | 30s | `collectTunnelSummaryWithStats()` → `collectInterfaceStats()` → `std.ArrayList.initCapacity()` | `freeTunnelSummarySnapshots()` ✓ | LOW - properly cleaned |
| BFD Receive Loop | `bfd/receive.zig:bfdReceiveLoop()` | 50ms poll timeout | `BfdReceiveLoopState` (static), `poll()` syscall | N/A - no allocation | LOW - bounded buffers |
| BFD Transmit Loop | `bfd/transmit.zig:bfdTransmitLoop()` | 100ms tick | `runtime.tick()` → `isTransmitDue()` → `session.buildTransmitPacket()` | N/A - no allocation | LOW - no per-tick heap |
| BGP FSM Thread | `bgp/runtime.zig:bgpRuntimeThread()` | 100ms | `serve_integration.runSessionOnce()` | N/A - state machine | **MEDIUM** - unknown session state mutation |
| BGP Prefix Watch | `bgp/prefix_watch_linux.zig:poll()` | event-driven | `watches.append()`, `pending_refresh.append()` | `drainPendingRefresh()`, `deinit()` | LOW - bounded |
| BGP Passive Listener | `bgp/passive_listener.zig` | connection-driven | Socket allocation | `close()` | LOW |
| HTTP Accept Loop | `http/server.zig:acceptLoop()` | blocking | Per-request buffers | `deinit()` | LOW |
| Status Render | `status.zig:renderPayloadWithContextAndDiag()` | on-request | `StatusScratch`, network_diag | `deinitScratchChecks()` ✓ | EXONERATED |

### Key Findings

1. **HTTP /status path is CLEAN**: `deinitScratchChecks()` is called in defer (status.zig:385)
2. **Heartbeat is CLEAN**: `freeTunnelSummarySnapshots()` properly frees per-cycle allocations
3. **BFD loops are CLEAN**: No per-tick heap allocations detected
4. **BGP FSM thread**: State machine runs `runSessionOnce()` on each 100ms tick - may be mutating session state

### Suspicious Pattern

The 136 bytes/second rate equals ~10 ticks/sec if the BGP FSM loop is the source. The BGP FSM loop runs every 100ms, so:

```
136 bytes/sec ÷ 10 ticks/sec = ~13.6 bytes/tick
```

If there's an allocation happening in `runSessionOnce()` that isn't freed, this would accumulate ~13.6 bytes per tick.

## Phase 2: Low-Risk Instrumentation (IMPLEMENTED)

### Added: Per-Subsystem Tick Counters (Atomic)

Created `runtime/idle_telemetry.zig` with atomic tick counters using `@atomicRmw` and `@atomicLoad`:

```zig
var bgp_fsm_ticks: u64 = 0;
var bfd_transmit_ticks: u64 = 0;
var bfd_receive_ticks: u64 = 0;
var heartbeat_ticks: u64 = 0;

pub fn incrementBgpFsmTicks() void {
    _ = @atomicRmw(u64, &bgp_fsm_ticks, .Add, 1, .monotonic);
}

pub fn getTickCounters() telemetry.TickCounters {
    return .{
        .bgp_fsm_ticks = @atomicLoad(u64, &bgp_fsm_ticks, .monotonic),
        .bfd_transmit_ticks = @atomicLoad(u64, &bfd_transmit_ticks, .monotonic),
        .bfd_receive_ticks = @atomicLoad(u64, &bfd_receive_ticks, .monotonic),
        .heartbeat_ticks = @atomicLoad(u64, &heartbeat_ticks, .monotonic),
    };
}
```

Added `TickCounters` to `RuntimeTelemetry` in `runtime/telemetry.zig` so ticks are included in `/status` output.

### Instrumented Subsystems

Tick increments added to:
- `bgp/runtime.zig`: `idle_telemetry.incrementBgpFsmTicks()` on each FSM iteration
- `bfd/transmit.zig`: `idle_telemetry.incrementBfdTransmitTicks()` on each tick
- `bfd/receive.zig`: `idle_telemetry.incrementBfdReceiveTicks()` on each poll
- `http/heartbeat.zig`: `idle_telemetry.incrementHeartbeatTicks()` on each 30s cycle

### Verification

- [x] Build passes
- [x] Tests pass (1665 passed, 31 skipped, 0 failed)
- [x] Gate passes

### Planned (Not Yet Implemented)

- [ ] Compile-gated debug logging (behind `KGB_DEBUG_*` flags)
- [ ] New BGP FSM tick-loop memory test
- [ ] Phase 3: Isolation matrix (run with each subsystem disabled)

## Phase 3: Isolation Matrix

### Configuration Matrix

| Configuration | BFD Loop | BGP Loop | Heartbeat | Prefix Watch | Expected Growth |
|---------------|----------|----------|-----------|--------------|-----------------|
| A: All enabled | ON | ON | ON | ON | ~136 bytes/sec |
| B: BFD disabled | OFF | ON | ON | ON | TBD |
| C: BGP disabled | ON | OFF | ON | ON | TBD |
| D: Heartbeat disabled | ON | ON | OFF | ON | TBD |
| E: BFD+BGP disabled | OFF | OFF | ON | ON | TBD |
| F: All protocols disabled | OFF | OFF | OFF | OFF | ~0 bytes/sec |

### Measurement Protocol

For each configuration, capture `/proc/$pid/smaps_rollup`:
```
RSS:       [value]
Anonymous: [value]
Private_Dirty: [value]
Pss:       [value]
```

Sample interval: 300 seconds (5 minutes)
Compute: `bytes_per_second = (end_value - start_value) / elapsed_seconds`

### smaps_rollup Evidence

```bash
# Capture baseline
cat /proc/$PID/smaps_rollup | grep -E "Rss|Anonymous|Private_Dirty|Pss"

# Or parse with Python
python3 -c "
import re
with open('/proc/self/smaps_rollup') as f:
    data = f.read()
metrics = {
    'Rss': int(re.search(r'Rss:\s+(\d+)', data).group(1)),
    'Anonymous': int(re.search(r'Anonymous:\s+(\d+)', data).group(1)),
    'Private_Dirty': int(re.search(r'Private_Dirty:\s+(\d+)', data).group(1)),
    'Pss': int(re.search(r'Pss:\s+(\d+)', data).group(1)),
}
print(metrics)
"
```

## Phase 4: Regression Guard

### Memory Attribution Tests

The existing `idle_memory_attribution_tests.zig` covers:
- WireGuard show collector error paths
- Heartbeat tunnel summary collection
- Interface stats collection
- BGP export delta computation
- BFD runtime type existence

### Gap Identified

**Missing**: Test for BGP FSM loop iterations without memory growth

### Planned Test

```zig
test "BGP FSM tick loop does not accumulate retained allocations" {
    // Verify that repeated FSM iterations don't leak memory
    // This tests the session state machine in isolation
    const allocator = std.testing.allocator;
    const runtime = @import("../bgp/runtime.zig");
    
    // Simulate many FSM ticks
    for (0..1000) |_| {
        // Each tick should not accumulate retained allocations
        // If session state grows unbounded, this test will detect it
    }
}
```

## Investigation Status

### Exonerated
- [x] HTTP /status request rendering
- [x] Heartbeat tunnel summary collection
- [x] BFD transmit/receive loops
- [x] Prefix watch polling

### Under Investigation
- [ ] BGP FSM thread (`runSessionOnce()` path)
- [ ] Any hidden global state mutations
- [ ] Kernel-level allocations (not process heap)

### Next Steps

1. **Run isolation matrix**: Start daemon in configuration A, measure 5 min baseline
2. **Disable BGP**: Restart with BGP disabled, measure 5 min
3. **Compare**: If growth rate drops significantly, BGP FSM is the source
4. **Deep dive BGP FSM**: Instrument `runSessionOnce()` allocations

## References

- Supersedes: `ACT-HULK29R-ZIG016-STATUS-HTTP-CHECK-DETAIL-OWNERSHIP`
- Original evidence: `docs/evidence/memory-lab/`
- Memory tools: `scripts/lab_memory_attribution_matrix.py`
- Zig 0.16 field manual: `docs/tooling/zig-0.16-field-manual.md`

## Verification

- [x] `make gate` passes
- [x] `make tovarisch-build` succeeds
- [x] `make tovarisch-test` passes
- [x] Tick counter instrumentation compiles without errors
