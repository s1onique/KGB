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

## Native Event Attribution Infrastructure

### Event Source Types

The lab supports two distinct event sources:

| Source | Description | Attribution Use |
|--------|-------------|-----------------|
| **Shell-side synthetic** | Events emitted by lab shell script on fixed schedule | Bookkeeping only, cannot produce `confirmed_leak` |
| **Tovarisch-native** | Events emitted from real tovarisch runtime paths | Required for `confirmed_leak` attribution |

### Native Event Architecture

Native events are emitted from actual tovarisch runtime code:

- **Bounded ring buffer**: Maximum 256 events to prevent unbounded growth
- **Low overhead**: Event emission is a no-op when disabled
- **Bounded details**: Max 128 byte detail strings (no unbounded command output)
- **Process PID**: Included for multi-process lab scenarios
- **Monotonic time**: Uses elapsed milliseconds, not wall clock

### Native Event Names

Heartbeat:
- `heartbeat_tick_start` - Start of heartbeat tick cycle
- `heartbeat_tick_end` - Successful heartbeat tick completion
- `heartbeat_tick_failed` - Heartbeat tick failure

WireGuard:
- `wg_check_start` - Start of WG check cycle
- `wg_check_end` - Successful WG check completion
- `wg_check_failed` - WG check failure with error class

Health/status:
- `health_collect_start` - Start of health collection
- `health_collect_end` - Successful health collection
- `health_collect_failed` - Health collection failure

BGP:
- `bgp_maintenance_start` - Start of BGP maintenance loop
- `bgp_maintenance_end` - Successful BGP maintenance
- `bgp_maintenance_failed` - BGP maintenance failure
- `bgp_reconnect_start` - BGP reconnect attempt
- `bgp_reconnect_end` - BGP reconnect complete

BFD:
- `bfd_tick_start` - Start of BFD tick
- `bfd_tick_end` - Successful BFD tick
- `bfd_tick_failed` - BFD tick failure

Status (lab/debug only):
- `status_request_start` - Start of status request
- `status_request_end` - Status request complete

## Investigation Status

### Completed Audits

1. **Heartbeat tunnel-summary ownership**: The `collectTunnelSummary()` function properly frees stats snapshots via `freeInterfaceStatsSnapshots()`. The `emitHeartbeatToFd()` now uses `collectTunnelSummaryWithStats()` with explicit `defer` to ensure deterministic memory cleanup.

2. **WG check path**: The `collectWgDiagnosticsOwned()` API properly owns the stdout buffer and releases it via `deinit()`. Error paths (CommandNotFound, CommandFailed) are handled without memory retention.

3. **Status rendering**: Uses `page_allocator` with proper cleanup via `defer diag.deinit(allocator)` and `defer linux_interface_stats.freeInterfaceStatsSnapshots()`.

### Native Event Infrastructure Added

1. **Tovarisch-native event ring buffer** (`tovarisch/src/runtime/lab_events.zig`): Bounded event stream for attribution
2. **Real runtime toggles** (config `[lab]` section): `disable_heartbeat`, `disable_wg_checks`, `disable_bgp`, `disable_bfd`
3. **Native event emission** in heartbeat thread around actual tick cycle
4. **Config file support** for lab settings

## Verdict Meanings

| Verdict | Meaning | Required Evidence |
|---------|---------|-------------------|
| `confirmed_leak` | Detected staircase steps with **native event attribution** | Native events from real runtime, correlated with memory steps |
| `bounded_warmup_or_allocator_highwater` | Minimal growth, likely bounded | Evidence of plateau or <200 KiB total |
| `inconclusive` | Growth pattern unclear or owner unattributed | Reason explaining unattribution |

### Confirmed Leak Requirements

A `confirmed_leak` verdict requires ALL of:

1. **Non-empty owner** (not "unknown")
2. **Memory step evidence** (`steps_detected` >= 3, `total_growth_kib` > 500)
3. **Native events enabled**: `native_events_enabled: true` in manifest
4. **Native event timeline**: `native_event_timeline.tsv` exists with data rows
5. **Correlated native events**: At least one native event from owner subsystem near a memory step
6. **Runtime toggle state**: Manifest shows actual toggles used (`native_disable_*` fields)
7. **No shell-only synthetic attribution**: Shell-side events cannot produce `confirmed_leak`

### Confirmed Leak Rejection Criteria

The verifier rejects `confirmed_leak` if:
- `native_events_enabled` is `false` or missing
- `native_event_timeline.tsv` is missing or has no data rows
- Artifact contains shell-side synthetic events with `subsystem_config` marker
- Owner is `unknown` or empty
- Steps detected < 3 or total growth <= 500 KiB
- No native events correlate with memory steps

## How to Run Local Idle Memory Lab

### Quick Run (10 minutes) - Shell Synthetic Only
```bash
make lab-tovarisch-idle-memory
```

### Native Events Enabled (Required for Attribution)
```bash
./scripts/lab_tovarisch_idle_memory.sh --native-events --duration 600
```

### Native Isolation Modes

**Heartbeat only** (disable WG, BGP, BFD):
```bash
./scripts/lab_tovarisch_idle_memory.sh --native-heartbeat-only
```

**WireGuard only** (disable heartbeat, BGP, BFD):
```bash
./scripts/lab_tovarisch_idle_memory.sh --native-wg-only
```

**No periodic paths** (baseline measurement):
```bash
./scripts/lab_tovarisch_idle_memory.sh --native-no-periodic
```

### Individual Native Toggles

```bash
# Enable native events
./scripts/lab_tovarisch_idle_memory.sh --native-events

# Disable specific subsystems
./scripts/lab_tovarisch_idle_memory.sh --native-events --disable-heartbeat
./scripts/lab_tovarisch_idle_memory.sh --native-events --disable-wg-checks
./scripts/lab_tovarisch_idle_memory.sh --native-events --disable-bgp
./scripts/lab_tovarisch_idle_memory.sh --native-events --disable-bfd

# Custom native event output path
./scripts/lab_tovarisch_idle_memory.sh --native-events --native-events-path /tmp/my_events.tsv
```

## Environment Variables

```bash
# Native event emission
NATIVE_EVENTS=true
NATIVE_EVENTS_PATH=/path/to/native_event_timeline.tsv

# Real runtime toggles
DISABLE_HEARTBEAT=true
DISABLE_WG_CHECKS=true
DISABLE_BGP=true
DISABLE_BFD=true

# Force WG command not-found path
TOVARISCH_WG_COMMAND_PATH=/nonexistent ./scripts/lab_tovarisch_idle_memory.sh

# Custom port
LAB_TOVARISCH_PORT=8318 ./scripts/lab_tovarisch_idle_memory.sh
```

## Config File Settings

Native lab settings can be configured in the tovarisch config file:

```ini
[lab]
native_events_enabled = true
native_events_path = "/tmp/native_event_timeline.tsv"
disable_heartbeat = false
disable_wg_checks = true
disable_bgp = true
disable_bfd = true
```

## Artifact Format

Artifacts are written to:
```
artifacts/memory-labs/tovarisch/idle-staircase/<run-id>/
```

### Files

| File | Description |
|------|-------------|
| `manifest.yaml` | Lab configuration, build info, git state, subsystem toggles, native event state |
| `memory_samples.tsv` | RSS/VmData samples with timestamps |
| `event_timeline.tsv` | Shell-side synthetic events (for bookkeeping) |
| `native_event_timeline.tsv` | Tovarisch-native events (for attribution) |
| `verdict.txt` | Verdict with growth analysis and attribution |
| `tovarisch_lab.conf` | Config file used for native settings |
| `strace.log` | (Optional) Syscall trace if STRACE=true |

### Manifest Native Fields

```yaml
# Native event source
native_events_enabled: true
native_events_path: "/path/to/native_event_timeline.tsv"

# Runtime toggle state
native_disable_heartbeat: false
native_disable_wg_checks: true
native_disable_bgp: true
native_disable_bfd: true
```

### Native Event Timeline Format

```
timestamp	elapsed_millis	event	subsystem	detail	pid
2026-01-01T00:00:30.000	30000	heartbeat_tick_start	heartbeat		1234
2026-01-01T00:00:30.000	30000	heartbeat_tick_end	heartbeat		1234
2026-01-01T00:01:00.000	60000	heartbeat_tick_start	heartbeat		1234
2026-01-01T00:01:00.000	60000	heartbeat_tick_end	heartbeat		1234
```

## Verdict Interpretation

### With Native Events: `confirmed_leak`

```
verdict: confirmed_leak
owner: heartbeat
reason: Native events from heartbeat subsystem correlate with 3 memory steps.
steps_detected: 3
total_growth_kib: 600
native_events_enabled: true
native_event_count: 6
```

**Interpretation**: Native events from heartbeat correlate with memory steps. Owner is attributed.

### With Native Events: `inconclusive`

```
verdict: inconclusive
owner: 
reason: Native events present but none correlate with memory steps.
native_events_enabled: true
```

**Interpretation**: Native events exist but don't correlate with growth. Owner remains unknown.

### Without Native Events: `inconclusive`

```
verdict: inconclusive
owner: 
reason: Shell-side synthetic events cannot produce confirmed_leak. 
        Need tovarisch-native event emission to identify the periodic background owner.
native_events_enabled: false
```

**Interpretation**: Shell-only artifacts cannot produce `confirmed_leak`. Native events required.

### All Periodic Paths Disabled: `inconclusive`

```
verdict: inconclusive
owner: 
reason: Growth persists with all periodic paths disabled. May be allocator warmup.
native_events_enabled: true
native_disable_heartbeat: true
native_disable_wg_checks: true
native_disable_bgp: true
native_disable_bfd: true
```

**Interpretation**: Growth continues even when all known periodic paths are disabled. Suggests allocator warmup or unknown source.

## Attribution Strategy

1. **Run with all periodic paths enabled** (baseline):
   ```bash
   ./scripts/lab_tovarisch_idle_memory.sh --native-events
   ```
   Expected: Staircase growth with native events from active subsystems.

2. **Run with one subsystem at a time**:
   ```bash
   # Heartbeat only
   ./scripts/lab_tovarisch_idle_memory.sh --native-heartbeat-only
   
   # No periodic paths
   ./scripts/lab_tovarisch_idle_memory.sh --native-no-periodic
   ```
   Expected: If growth disappears when heartbeat disabled and appears when enabled, heartbeat is the owner.

3. **Analyze verdict**:
   - `confirmed_leak` with native events: Owner attributed
   - `inconclusive` with all disabled: Allocator warmup or unknown source
   - `bounded_warmup_or_allocator_highwater`: Normal behavior

## Memory Attribution Matrix

The **Memory Attribution Matrix** runs multiple lab variants in sequence to systematically attribute idle memory growth.

### Matrix Variants

| Variant | Heartbeat | WG Checks | BGP | BFD |
|---------|-----------|-----------|-----|-----|
| `all_enabled` | ✓ | ✓ | ✓ | ✓ |
| `heartbeat_disabled` | ✗ | ✓ | ✓ | ✓ |
| `wg_disabled` | ✓ | ✗ | ✓ | ✓ |
| `bgp_disabled` | ✓ | ✓ | ✗ | ✓ |
| `bfd_disabled` | ✓ | ✓ | ✓ | ✗ |
| `bgp_bfd_disabled` | ✓ | ✓ | ✗ | ✗ |
| `no_periodic` | ✗ | ✗ | ✗ | ✗ |

### Matrix Verdicts

| Verdict | Meaning |
|---------|---------|
| `no_growth` | No significant memory growth detected in any variant |
| `bounded_warmup_or_allocator_highwater` | Growth present but bounded. Consistent with allocator settling behavior. |
| `subsystem_correlated_growth` | Growth correlates with specific subsystem(s). Evidence points to periodic background paths. |
| `inconclusive` | Cannot determine attribution. More data needed. |

### Evidence Contract

The matrix proves or disproves:

1. **Bounded allocator/warmup**: If `no_periodic` variant shows bounded growth (~<500 KiB, ~<5 steps) while `all_enabled` shows none, the growth is likely allocator settling.

2. **Subsystem attribution**: If disabling a specific subsystem eliminates growth while other variants show growth, that subsystem is the likely owner.

3. **Global leak**: This matrix does NOT claim "no leak". It only classifies what the evidence shows.

### Running the Matrix

```bash
# Full matrix with 10-minute variants (default)
./scripts/lab_memory_attribution_matrix.sh

# Longer observation (30 minutes)
./scripts/lab_memory_attribution_matrix.sh --duration 1800

# Custom run ID
./scripts/lab_memory_attribution_matrix.sh --run-id my-test

# Specific variants only
./scripts/lab_memory_attribution_matrix.sh --variants all_enabled no_periodic
```

### Matrix Artifacts

```
artifacts/memory-labs/tovarisch/idle-matrix/<run-id>/
├── matrix-summary.md          # Consolidated matrix results
├── all_enabled/               # Variant artifact
│   ├── manifest.yaml
│   ├── memory_samples.tsv
│   ├── native_event_timeline.tsv
│   └── verdict.txt
├── heartbeat_disabled/
│   └── ...
└── no_periodic/
    └── ...
```

### Matrix Interpretation Guide

#### Scenario 1: `no_growth` verdict
```
Growth: None
All variants: <100 KiB growth, <3 steps
```
**Conclusion**: No idle memory growth detected. System is stable.

#### Scenario 2: `bounded_warmup_or_allocator_highwater` verdict
```
all_enabled growth: 800 KiB, 8 steps
no_periodic growth: 400 KiB, 3 steps
Other variants: Similar to all_enabled
```
**Conclusion**: Growth is bounded even with all periodic paths disabled. Likely allocator warmup settling. Not caused by specific subsystem.

#### Scenario 3: `subsystem_correlated_growth` verdict
```
all_enabled growth: 1200 KiB, 10 steps
heartbeat_disabled growth: 100 KiB, 1 step
no_periodic growth: 50 KiB, 0 steps
```
**Conclusion**: Heartbeat is the likely owner. When heartbeat is disabled, growth largely disappears.

#### Scenario 4: `inconclusive` verdict
```
Mixed results across variants
Unable to attribute growth to specific subsystem
```
**Conclusion**: More data needed. Run longer duration or investigate allocator behavior.

### Fail Conditions

The matrix **fails closed** if:

1. **Native event capture configured but missing**: `native_event_timeline.tsv` not created when native events enabled
2. **Disabled subsystem emits events**: Heartbeat/WG/BGP/BFD events present when that subsystem is disabled
3. **Sample count too low**: Fewer than `duration/60` samples (should be at least 1 per minute)
4. **Duration below minimum**: Duration < 300 seconds (5 minutes)
5. **Analyzer cannot parse run**: Verdict.txt missing or invalid format

### GitHub Actions Matrix Workflow

The **Tovarisch Idle Memory Attribution Matrix** workflow runs the matrix on Linux:

1. **Manual trigger only** (`workflow_dispatch`)
2. **Configurable duration** per variant (default: 600s)
3. **Artifact upload** with `if: always()` for post-mortem analysis
4. **Timeout**: 120 minutes (allows 7 x ~15 min variants with overhead)

```bash
# Trigger via GitHub CLI
gh workflow run tovarisch-idle-memory-attribution-matrix.yml \
  --field duration=600 \
  --field interval=5
```

## Artifact Verification

### Self-Test
```bash
python3 scripts/verify_idle_staircase_artifact.py --self-test
```

### Verify Artifact
```bash
python3 scripts/verify_idle_staircase_artifact.py artifacts/memory-labs/tovarisch/idle-staircase/<run-id>
```

### Verify Matrix
```bash
python3 scripts/verify_memory_attribution_matrix.py artifacts/memory-labs/tovarisch/idle-matrix/<run-id>
python3 scripts/verify_memory_attribution_matrix.py --self-test
```

### Make Targets
```bash
make verify-idle-staircase-artifact
make verify-memory-attribution-matrix
```

## Native Heartbeat Smoke Verification

### Manual GitHub Actions Workflow

A manual GitHub Actions workflow, **"Tovarisch Idle Native Heartbeat Smoke"**, runs the Linux-only native heartbeat smoke proof and uploads enabled/disabled artifacts.

Trigger: `workflow_dispatch` only — not scheduled or PR-gated.

```bash
# Or run locally on Linux:
make tovarisch-compile-linux
./scripts/lab_tovarisch_idle_memory.sh --native-events --duration 95 --run-id heartbeat-native-enabled-smoke
./scripts/lab_tovarisch_idle_memory.sh --native-events --disable-heartbeat --duration 65 --run-id heartbeat-native-disabled-smoke
python3 scripts/verify_idle_staircase_native_heartbeat_smoke.py \
  --enabled artifacts/.../heartbeat-native-enabled-smoke \
  --disabled artifacts/.../heartbeat-native-disabled-smoke
```

**Enabled**: `native_event_timeline.tsv` has `heartbeat_tick_start/end` (elapsed >= 30000ms).
**Disabled**: manifest shows `native_disable_heartbeat: true`, no heartbeat native events.
**Platform**: Lab requires Linux with `/proc`; macOS prints SKIP and exits.

## Current Status

**Verdict**: `inconclusive` (without native events)

**Native infrastructure**: Implemented and ready for use

**Matrix runner**: Implemented for systematic attribution

**Next steps**:
1. Run memory attribution matrix to identify subsystem owner
2. If growth persists with all periodic paths disabled, investigate allocator behavior
3. Use matrix verdict to guide further investigation

## File Structure

| File | Purpose |
|------|---------|
| `scripts/lab_tovarisch_idle_memory.sh` | Thin shell wrapper (delegates to Python) |
| `scripts/lab_tovarisch_idle_memory.py` | Entry point (delegates to lab_runner package) |
| `scripts/lab_runner/` | **Python-owned lab runner package** |
| `scripts/lab_runner/config.py` | LabRunConfig dataclass, parse_args(), LabError |
| `scripts/lab_runner/proc.py` | require_linux_proc(), read_proc_status() |
| `scripts/lab_runner/artifacts.py` | Config/manifest/event writing |
| `scripts/lab_runner/tovarisch.py` | Process lifecycle (start, wait, terminate) |
| `scripts/lab_runner/loop.py` | Idle sampling loop, status burst |
| `scripts/lab_runner/analyzer.py` | Analyzer CLI invocation |
| `scripts/lab_runner/validation.py` | Output verification, final summary |
| `scripts/lab_runner/self_tests.py` | Self-test suite |
| `scripts/lab_runner/main.py` | Main entry point |
| `scripts/lab_memory_attribution_matrix.py` | Matrix runner for systematic attribution |
| `scripts/lab_memory_attribution_matrix.sh` | Thin shell wrapper for matrix runner |
| `scripts/verify_memory_attribution_matrix.py` | Matrix artifact verifier |
| `scripts/idle_staircase_analyzer.py` | Verdict analysis logic (Python) |
| `scripts/idle_staircase_analyzer_cli.py` | CLI wrapper for analyzer |
| `scripts/verify_idle_staircase_artifact.py` | CLI wrapper for artifact verification |
| `scripts/idle_staircase_verifier/` | Verifier package |
| `tovarisch/src/runtime/lab_events.zig` | Native event ring buffer |
| `tovarisch/src/config.zig` | Lab config parsing with native settings |

## Lab Runner Architecture

See [idle-staircase-architecture.md](./idle-staircase-architecture.md) for detailed architecture documentation.

## Related Documentation

- [Memory Budgets](./memory-budgets.md)
- [Memory Lab Infrastructure](../labs/memory-lab.md)
- [Lab Architecture](./idle-staircase-architecture.md)
- [Embedded Memory Frugality](../doctrine/embedded-memory-frugality.md)
