# Idle Staircase Memory Lab

## Live Symptom Summary

Live tovarisch (version `0.1.1-rc51+05902ce`) exhibited stepwise heap/data growth while mostly idle:

- **RSS and VmData grew together** in small steps (~100-124 KiB per step)
- **Growth rate**: ~33 KiB/min (~2 MB/hour)
- **Total growth window**: +672 KiB over ~20.2 minutes
- **VmHWM and VmSwap stayed flat** during sampled window
- **Status burst deprioritized**: 5000 `/status` requests did not produce proportional RSS growth

**Note**: The root cause of the observed staircase pattern is not conclusively attributed.
Owner attribution requires event correlation analysis and targeted testing.

## Investigation Status

### Completed Audits

1. **Heartbeat tunnel-summary ownership**: The `collectTunnelSummary()` function properly frees stats snapshots via `freeInterfaceStatsSnapshots()`. The `emitHeartbeatToFd()` now uses `collectTunnelSummaryWithStats()` with explicit `defer` to ensure deterministic memory cleanup.

2. **WG check path**: The `collectWgDiagnosticsOwned()` API properly owns the stdout buffer and releases it via `deinit()`. Error paths (CommandNotFound, CommandFailed) are handled without memory retention.

3. **Status rendering**: Uses `page_allocator` with proper cleanup via `defer diag.deinit(allocator)` and `defer linux_interface_stats.freeInterfaceStatsSnapshots()`.

### Attribution Framework

The lab provides local attribution infrastructure:

- **Event attribution hooks**: Subsystem event logging (`heartbeat_tick`, `wg_check`, `bgp_maintenance`, `bfd_tick`, etc.)
- **Subsystem toggles**: Control synthetic event emission (NOT actual tovarisch runtime behavior)
- **Correlated event analysis**: Matches memory steps to subsystem events
- **Focused regression tests**: Validates each periodic path doesn't leak on repeated execution

> ⚠️ **Important**: The subsystem toggles (`--heartbeat-only`, `--wg-only`, etc.) only control **synthetic shell-side event logging**. They do NOT disable actual tovarisch periodic paths (heartbeat, WG checks, BGP/BFD). The events logged are on fixed intervals and cannot be used for real attribution. Real attribution requires tovarisch-native event emission.

## Verdict Meanings

| Verdict | Meaning | Required Evidence |
|---------|---------|------------------|
| `confirmed_leak` | Detected staircase steps with **explicit owner attribution** | Non-empty owner, memory steps, correlated events |
| `bounded_warmup_or_allocator_highwater` | Minimal growth, likely bounded | Evidence of plateau or <200 KiB total |
| `inconclusive` | Growth pattern unclear or owner unattributed | Reason explaining unattribution |

### Confirmed Leak Requirements

A `confirmed_leak` verdict requires ALL of:

1. **Non-empty owner** (not "unknown")
2. **Memory step evidence** (`steps_detected` >= 3, `total_growth_kib` > 500)
3. **Event timeline evidence** - owner subsystem events in timeline
4. **Owner evidence text** - description of attribution reasoning
5. **Correlated events** - at least one event from owner subsystem near a memory step (within 30 seconds)
6. **Real tovarisch-native events** - NOT shell-side synthetic events

The verifier rejects `confirmed_leak` with:
- `owner: unknown`
- Only synthetic/shell-side events
- Missing correlated events

## How to Run Local Idle Memory Lab

### Quick Run (10 minutes)
```bash
make lab-tovarisch-idle-memory
```

### Extended Run (30 minutes)
```bash
make lab-tovarisch-idle-memory DURATION=1800
```

### With /status Burst Test
```bash
make lab-tovarisch-idle-memory DURATION=600 STATUS_BURST=true
```

### With Syscall Tracing (Linux only)
```bash
make lab-tovarisch-idle-memory DURATION=600 STRACE=true
```

### Direct Script Usage
```bash
./scripts/lab_tovarisch_idle_memory.sh --duration 600 --interval 5
```

## Targeted Attribution Testing

### Subsystem Toggles (Synthetic Event Labeling)

The subsystem toggles (`--heartbeat-only`, `--wg-only`, `--bgp-bfd-only`, `--no-subsystems`) **only control shell-side synthetic event emission** to the event timeline. They do **NOT** disable actual tovarisch runtime periodic paths.

These toggles are useful for **synthetic event-label experiments** only - not for subsystem isolation.

```bash
# Synthetic-only: emit only heartbeat events
./scripts/lab_tovarisch_idle_memory.sh --heartbeat-only

# Synthetic-only: emit only WG check events  
./scripts/lab_tovarisch_idle_memory.sh --wg-only

# Synthetic-only: emit only BGP/BFD events
./scripts/lab_tovarisch_idle_memory.sh --bgp-bfd-only

# Synthetic-only: suppress all synthetic events
./scripts/lab_tovarisch_idle_memory.sh --no-subsystems
```

> ⚠️ **Important**: `--no-subsystems` does NOT disable actual tovarisch periodic paths. It only suppresses synthetic shell-side event logging. To confirm growth without periodic paths, you would need actual tovarisch-native subsystem toggles (not yet implemented).

### Environment Variables

```bash
# Force WG command not-found path
TOVARISCH_WG_COMMAND_PATH=/nonexistent ./scripts/lab_tovarisch_idle_memory.sh

# Custom port
LAB_TOVARISCH_PORT=8318 ./scripts/lab_tovarisch_idle_memory.sh

# Synthetic event toggles via env
HEARTBEAT_ENABLED=false WG_CHECK_ENABLED=true BGP_BFD_ENABLED=false ./scripts/lab_tovarisch_idle_memory.sh
```

### Attribution Strategy

Shell-side synthetic events can **enrich inconclusive artifacts** but **cannot produce `confirmed_leak` verdicts**. Real attribution requires:

1. **Tovariisch-native event emission** - events emitted from within tovarisch code, not shell-side
2. **Correlation with memory steps** - events near memory step timestamps
3. **Verdict enforcement** - the verifier rejects any `confirmed_leak` artifact with shell-side synthetic events

## Understanding Staircase vs Linear Growth

### Staircase Growth (Leak Pattern)
- **Shape**: Step-wise increases separated by flat periods
- **Implication**: Periodic/background allocation without proper deallocation
- **Owner detection**: Correlate steps with periodic intervals (heartbeat=30s, BGP keepalive=60s)

### Linear Growth (Warmup Pattern)
- **Shape**: Gradual upward slope
- **Implication**: Normal allocator warmup or GC high-water mark settling
- **Acceptable**: Under ~200 KiB total growth in first 10 minutes

## Artifact Format

Artifacts are written to:
```
artifacts/memory-labs/tovarisch/idle-staircase/<run-id>/
```

### Files

| File | Description |
|------|-------------|
| `manifest.yaml` | Lab configuration, build info, git state, subsystem toggles |
| `memory_samples.tsv` | RSS/VmData samples with timestamps |
| `event_timeline.tsv` | Timestamped events (heartbeat ticks, WG checks, etc.) |
| `verdict.txt` | Verdict with growth analysis and attribution |
| `strace.log` | (Optional) Syscall trace if STRACE=true |

### Event Types in Timeline

The lab logs these event types for attribution:

| Event | Subsystem | Trigger |
|-------|-----------|---------|
| `heartbeat_tick` | heartbeat | Every 30 seconds if heartbeat enabled |
| `wg_check` | wireguard | Every 60 seconds if WG checks enabled |
| `bgp_maintenance` | bgp | Every 10 seconds if BGP enabled |
| `bfd_tick` | bfd | Every 10 seconds if BFD enabled |
| `status_burst_start/complete` | status | During /status burst test |

### Enhanced Verdict Fields

Verdicts include these attribution fields:

```yaml
suspected_owner: heartbeat       # Identified owner (or empty)
owner_evidence: "Dominant subsystem: heartbeat (20 events)..."  # Attribution reasoning
correlated_events: heartbeat=20,wg=10,bgp=0,bfd=0  # Event counts by subsystem
enabled_subsystems: heartbeat=true,wg=true,bgp_bfd=false  # Subsystems enabled
disabled_subsystems: bgp_bfd  # Subsystems disabled for this run
```

## Regression Tests

### Focused Allocator Tests

Run targeted tests for periodic paths:

```bash
# Test WG check error paths
cd tovarisch && zig build test -- test_name="repeated failed WG"

# Test heartbeat tunnel summary
cd tovarisch && zig build test -- test_name="repeated heartbeat tunnel summary"

# Test BGP export delta
cd tovarisch && zig build test -- test_name="BGP export delta"

# Run all attribution tests
cd tovarisch && zig build test -- test_name="memory attribution"
```

### Test Coverage

The attribution test suite (`idle_memory_attribution_tests.zig`) covers:

- `repeated failed WG check does not leak memory` - WG command-not-found path
- `repeated heartbeat tunnel summary collection does not leak` - Heartbeat cycle
- `repeated interface stats collection does not leak` - Health collection
- `BGP export delta computation does not leak` - BGP export rebuild
- `repeated status check render does not leak` - Status rendering (negative control)

## Artifact Verification

### Self-Test
```bash
python3 scripts/verify_idle_staircase_artifact.py --self-test
```

### Verify Artifact
```bash
python3 scripts/verify_idle_staircase_artifact.py artifacts/memory-labs/tovarisch/idle-staircase/<run-id>
```

### Make Target
```bash
make verify-idle-staircase-artifact
```

## Known Limitations

### Without Live Production Access
- **Cannot reproduce exact production process**: No access to `london2`, `10.149.149.1`, real WireGuard, real BIRD
- **Local testing uses stubbed paths**: The lab runs tovarisch with minimal configuration
- **Event correlation is probabilistic**: Memory steps may not perfectly align with events due to sampling interval

### Platform Constraints
- Lab requires **Linux with /proc** for memory sampling
- On non-Linux, lab prints SKIP and exits cleanly before starting processes
- strace mode is Linux-only and optional

## Current Verdict

**Verdict**: `inconclusive`

**Reason**: Staircase growth detected (N steps, X KiB total) but owner is unattributed. Shell-side synthetic events cannot be used for attribution. Real attribution requires tovarisch-native event emission.

**Next Steps (Future Work)**:
1. **Add tovarisch-native event emission** - emit events from within tovarisch code (heartbeat, WG checks, BGP/BFD ticks)
2. **Add actual runtime subsystem toggles** - allow disabling real periodic paths (not just shell-side synthetic events)
3. **Re-run attribution with real events** - correlate tovarisch-native events with memory steps
4. **If growth persists with real subsystems disabled**, investigate allocator warmup behavior

> ⚠️ Current shell-side toggles (`--heartbeat-only`, `--no-subsystems`, etc.) do NOT disable actual tovarisch runtime paths. They only suppress synthetic shell-side event logging.

## Related Documentation

- [Memory Budgets](./memory-budgets.md)
- [Memory Lab Infrastructure](../labs/memory-lab.md)
- [Embedded Memory Frugality](../doctrine/embedded-memory-frugality.md)
- [Heartbeat Module](../../tovarisch/src/http/heartbeat.zig)
- [WireGuard Collector](../../tovarisch/src/net/wg_show_collector.zig)
- [Attribution Tests](../../tovarisch/src/http/idle_memory_attribution_tests.zig)
