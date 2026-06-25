# Idle Memory Attribution Matrix

The **Memory Attribution Matrix** runs multiple lab variants in sequence to systematically attribute idle memory growth to specific subsystems.

## Live Symptom Summary

Live tovarisch exhibited stepwise heap/data growth while mostly idle:

- **RSS and VmData grew together** in small steps (~100-124 KiB per step)
- **Growth rate**: ~33 KiB/min (~2 MB/hour)
- **Total growth window**: +672 KiB over ~20.2 minutes
- **VmHWM and VmSwap stayed flat** during sampled window

**Note**: Root cause requires attribution via the matrix.

## Matrix Variants

| Variant | Heartbeat | WG Checks | BGP | BFD |
|---------|-----------|-----------|-----|-----|
| `all_enabled` | ✓ | ✓ | ✓ | ✓ |
| `heartbeat_disabled` | ✗ | ✓ | ✓ | ✓ |
| `wg_disabled` | ✓ | ✗ | ✓ | ✓ |
| `bgp_disabled` | ✓ | ✓ | ✗ | ✓ |
| `bfd_disabled` | ✓ | ✓ | ✓ | ✗ |
| `bgp_bfd_disabled` | ✓ | ✓ | ✗ | ✗ |
| `no_periodic` | ✗ | ✗ | ✗ | ✗ |

## Matrix Verdicts

| Verdict | Meaning |
|---------|---------|
| `no_growth` | No significant memory growth detected in any variant |
| `bounded_warmup_or_allocator_highwater` | Growth present but bounded. Consistent with allocator settling behavior. |
| `subsystem_correlated_growth` | Growth correlates with specific subsystem(s). Evidence points to periodic background paths. |
| `inconclusive` | Cannot determine attribution. More data needed. |

## Evidence Contract

The matrix proves or disproves:

1. **Bounded allocator/warmup**: If `no_periodic` variant shows bounded growth (~<500 KiB, ~<5 steps) while `all_enabled` shows none, the growth is likely allocator settling.

2. **Subsystem attribution**: If disabling a specific subsystem eliminates growth while other variants show growth, that subsystem is the likely owner.

3. **Global leak**: This matrix does NOT claim "no leak". It only classifies what the evidence shows.

## Running the Matrix

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

## Matrix Artifacts

```
artifacts/memory-labs/tovarisch/idle-matrix/<run-id>/
├── matrix-summary.md          # Consolidated matrix results
├── matrix-manifest.yaml       # Matrix configuration
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

## Matrix Interpretation Guide

### Scenario 1: `no_growth` verdict

```
Growth: None
All variants: <100 KiB growth, <3 steps
```

**Conclusion**: No idle memory growth detected. System is stable.

### Scenario 2: `bounded_warmup_or_allocator_highwater` verdict

```
all_enabled growth: 800 KiB, 8 steps
no_periodic growth: 400 KiB, 3 steps
Other variants: Similar to all_enabled
```

**Conclusion**: Growth is bounded even with all periodic paths disabled. Likely allocator warmup settling.

### Scenario 3: `subsystem_correlated_growth` verdict

```
all_enabled growth: 1200 KiB, 10 steps
heartbeat_disabled growth: 100 KiB, 1 step
no_periodic growth: 50 KiB, 0 steps
```

**Conclusion**: Heartbeat is the likely owner. When heartbeat is disabled, growth largely disappears.

### Scenario 4: `inconclusive` verdict

```
Mixed results across variants
Unable to attribute growth to specific subsystem
```

**Conclusion**: More data needed. Run longer duration or investigate allocator behavior.

## Fail Conditions

The matrix **fails closed** if:

1. **Native event capture configured but missing**: `native_event_timeline.tsv` not created when native events enabled
2. **Disabled subsystem emits events**: Heartbeat/WG/BGP/BFD events present when that subsystem is disabled
3. **Sample count too low**: Fewer than `duration/60` samples (should be at least 1 per minute)
4. **Duration below minimum**: Duration < 300 seconds (5 minutes)
5. **Analyzer cannot parse run**: Verdict.txt missing or invalid format

## Disabled Subsystem Leak Semantics

When a subsystem is disabled, it must not emit any native events. This is a **fail-closed** condition:

| Variant | Disabled | Expected Events |
|---------|----------|-----------------|
| `heartbeat_disabled` | heartbeat | No heartbeat_* events |
| `wg_disabled` | WG checks | No wg_* events |
| `bgp_disabled` | BGP | No bgp_* events |
| `bfd_disabled` | BFD | No bfd_* events |
| `bgp_bfd_disabled` | BGP+BFD | No bgp_* or bfd_* events |
| `no_periodic` | all | No heartbeat_*, wg_*, bgp_*, or bfd_* events |

If any disabled subsystem emits events, the matrix run fails immediately.

## Matrix Manifest Contract

The `matrix-manifest.yaml` records:

```yaml
run_id: matrix-20260101-120000
duration_seconds: 600
sample_interval_seconds: 5
variants:
  - all_enabled
  - heartbeat_disabled
  - wg_disabled
  - bgp_disabled
  - bfd_disabled
  - bgp_bfd_disabled
  - no_periodic
```

## Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `MATRIX_DURATION` | Duration per variant (seconds) | 600 |
| `MATRIX_INTERVAL` | Sample interval (seconds) | 5 |
| `MATRIX_RUN_ID` | Custom run identifier | `matrix-{timestamp}` |

## GitHub Actions Matrix Workflow

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

## Verifier

Verify matrix artifacts:

```bash
python3 scripts/verify_memory_attribution_matrix.py artifacts/memory-labs/tovarisch/idle-matrix/<run-id>
python3 scripts/verify_memory_attribution_matrix.py --self-test
```

## Make Targets

```bash
make lab-memory-attribution-matrix
make verify-memory-attribution-matrix
```

## Related Documentation

- [Idle Staircase Memory Lab](./idle-staircase.md)
- [Memory Budgets](./memory-budgets.md)
- [Memory Lab Infrastructure](../labs/memory-lab.md)
- [Embedded Memory Frugality](../doctrine/embedded-memory-frugality.md)
