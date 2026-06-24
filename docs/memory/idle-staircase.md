# Idle Staircase Memory Lab

## Live Symptom Summary

Live tovarisch (version `0.1.1-rc51+05902ce`) exhibited stepwise heap/data growth while mostly idle:

- **RSS and VmData grew together** in small steps (~100-124 KiB per step)
- **Growth rate**: ~33 KiB/min (~2 MB/hour)
- **Total growth window**: +672 KiB over ~20.2 minutes
- **VmHWM and VmSwap stayed flat** during sampled window
- **Status burst deprioritized**: 5000 `/status` requests did not produce proportional RSS growth

## Investigation Notes

The heartbeat path in `http/heartbeat.zig` was audited for memory management issues.
The `collectTunnelSummary()` function properly frees stats snapshots via `freeInterfaceStatsSnapshots()`.
However, `emitHeartbeatToFd()` now uses `collectTunnelSummaryWithStats()` with explicit `defer`
to ensure deterministic memory cleanup.

**Note**: The root cause of the observed staircase pattern is not conclusively attributed to
heartbeat alone. The lab infrastructure allows for systematic investigation of periodic
background paths that could contribute to memory growth.

## Why /status Was Deprioritized

The `/status` rendering path (`status.zig`, `status_checks.zig`) uses `page_allocator` but properly frees allocations via:
- `defer diag.deinit(allocator)` in `getWgPeersCheck()`
- `defer linux_interface_stats.freeInterfaceStatsSnapshots()` in tunnel check

The status path is not the likely owner based on the deprioritization evidence.

## How to Run Local Idle Memory Lab

### Prerequisites
- Linux with `/proc` filesystem (required for memory sampling)
- tovarisch binary built: `make tovarisch-build`

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

## Understanding Staircase vs Linear Growth

### Staircase Growth (Leak Pattern)
- **Shape**: Step-wise increases separated by flat periods
- **Implication**: Periodic/background allocation without proper deallocation
- **Owner detection**: Correlate steps with periodic intervals (heartbeat=30s, BGP keepalive=60s)

### Linear Growth (Warmup Pattern)
- **Shape**: Gradual upward slope
- **Implication**: Normal allocator warmup or GC high-water mark settling
- **Acceptable**: Under ~200 KiB total growth in first 10 minutes

### Verdict Meanings

| Verdict | Meaning | Action |
|---------|---------|--------|
| `confirmed_leak` | Detected staircase steps with owner attribution | Fix the attributed path |
| `bounded_warmup_or_allocator_highwater` | Minimal growth, likely bounded | Monitor, no immediate action |
| `inconclusive` | Growth pattern unclear | Collect longer traces |

## Known Limitations

### Without Live Production Access
- **Cannot reproduce exact production process**: No access to `london2`, `10.149.149.1`, real WireGuard, real BIRD
- **Local fakes are sufficient**: The fake WireGuard path and stubbed health checks are deterministic
- **High-cardinality BGP fixture**: ~15k prefixes to match production scale

### Platform Constraints
- Lab requires **Linux with /proc** for memory sampling
- On non-Linux, lab prints SKIP and exits cleanly before starting processes
- strace mode is Linux-only and optional

## Artifact Format

Artifacts are written to:
```
artifacts/memory-labs/tovarisch/idle-staircase/<run-id>/
```

### Files

| File | Description |
|------|-------------|
| `manifest.yaml` | Lab configuration, build info, git state |
| `memory_samples.tsv` | RSS/VmData samples with timestamps |
| `event_timeline.tsv` | Timestamped events (heartbeat ticks, etc.) |
| `verdict.txt` | Verdict with growth analysis |

### Verdict Format
```
verdict: confirmed_leak
owner: heartbeat
reason: Detected N staircase steps with X KiB total growth (Y KiB/min)
steps_detected: N
total_growth_kib: X
growth_rate_kib_per_min: Y
samples_count: M
```

## Regression Gates

### Unit Tests
```bash
make tovarisch-test
# Runs: heartbeat_idle_memory_regression_tests.zig
```

### Artifact Verifier
```bash
make verify-idle-staircase-artifact
# Runs: verify_idle_staircase_artifact.py --self-test
```

## Related Documentation

- [Memory Budgets](./memory-budgets.md)
- [Memory Lab Infrastructure](../labs/memory-lab.md)
- [Embedded Memory Frugality](../doctrine/embedded-memory-frugality.md)
