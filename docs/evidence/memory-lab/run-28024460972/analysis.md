# UVB-76 Memory Attribution Analysis

**Run ID**: 28024460972  
**Generated**: 2026-06-24  
**Verdict**: `inconclusive` (low confidence)

## ⚠️ SLOPE-ONLY PRE-ATTRIBUTION NOTE

This analysis is **NOT a definitive verdict**. It provides slope context only. True verdict requires attribution lab artifacts (start/midpoint/end heap profiles, memstats checkpoints, heap object deltas).

The midpoint math argues AGAINST plateau:
- Start→Midpoint: 369.4 KiB/min
- Midpoint→End: 352.4 KiB/min  
- Ratio: **0.954** (continued growth at 95.4% of first-half rate)

This is **continued growth**, not a plateau.

## Evidence Summary

### Short-Window Baseline (workflow 28018628934)

| Metric | Value |
|--------|-------|
| Duration | 10 seconds |
| Signal Quality | `warmup_sensitive` |
| UVB-76 RSS Slope | 770.9 KiB/min |
| RSS Growth | 128 KiB |
| **Note** | Short window, warmup-dominated. NOT definitive evidence. |

### Long-Window Evidence (workflow 28024460972)

| Metric | UVB-76 | Tovarisch |
|--------|--------|-----------|
| Duration | 906.2 seconds | 908.8 seconds |
| Signal Quality | `long_window` | `long_window` |
| RSS First (KiB) | 11,768 | 8,088 |
| RSS Last (KiB) | 17,216 | 9,004 |
| RSS Max (KiB) | 17,344 | 9,004 |
| RSS Growth (KiB) | 5,448 | 916 |
| RSS Growth (%) | 46.3% | 11.3% |
| RSS Slope (KiB/min) | 360.72 | 60.47 |
| PSS Slope (KiB/min) | 360.72 | 60.47 |

## Slope Analysis

### Start → Midpoint (T+0 to T+453s)

| Metric | Value |
|--------|-------|
| Elapsed | 453 seconds |
| RSS Growth | 2,788 KiB |
| Slope | 369.4 KiB/min |
| Description | First half - rapid warmup/heap expansion |

### Midpoint → End (T+453 to T+906s)

| Metric | Value |
|--------|-------|
| Elapsed | 453 seconds |
| RSS Growth | 2,660 KiB |
| Slope | 352.4 KiB/min |
| Description | Second half - continued growth at slightly lower rate |

### Full Run Analysis

| Metric | Value |
|--------|-------|
| Slope Ratio (midpoint-end / start-midpoint) | 0.954 |
| Plateau Verdict | **No plateau evidence.** Midpoint→End slope remains 95.4% of Start→Midpoint slope. This is continued growth, not a plateau. |

### Comparison: Short vs Long Window

| Metric | Value |
|--------|-------|
| Short Window Slope | 770.9 KiB/min |
| Long Window Slope | 360.72 KiB/min |
| Ratio | 0.468 |

**Interpretation**: Long-window slope is 46.8% of short-window slope. This only proves the 10s window was warmup-sensitive; it does not establish bounded growth or plateau in the 906s run.

## Heap Analysis

| Metric | Value |
|--------|-------|
| Heap Alloc Bytes | 4,194,304 |
| Heap Objects Delta | **Unknown** - not captured in leak-slope artifact |
| Top Allocators | **Unknown** - attribution artifacts not available |

**Note**: True attribution analysis requires heap profiles at start/midpoint/end, compared using `go tool pprof`.

## Goroutine Analysis

| Metric | Value |
|--------|-------|
| Count | 25 (stable) |
| Start Count | 25 |
| End Count | 25 |
| Verdict | **Stable throughout run.** Goroutine count is NOT a leak contributor. |

## Allowed Verdicts

| Verdict | Description |
|---------|-------------|
| `confirmed_leak` | Unbounded growth with identified allocation owner |
| `bounded_warmup_plateau` | Growth that flattens, indicating cache/runtime warmup |
| `inconclusive` | Requires longer soak or additional instrumentation |

**This analysis produces**: `inconclusive`

**Reason**: This run has no attribution artifacts. True verdict requires 30-60 min attribution lab with start/midpoint/end heap profiles.

## Limitations

1. **No attribution lab artifacts** (heap profiles, memstats checkpoints) available in this run
2. **Heap profile delta analysis** not possible without start/midpoint/end pprof files
3. **Allocation owner identification** not possible without heap profile comparison
4. Verdict based on slope analysis only, not on detailed runtime stats
5. Attribution lab with forced-GC checkpoints would provide definitive evidence

## Recommended Next Steps

1. **Run attribution lab** (30-60 min soak) to generate heap profiles at start/midpoint/end
2. **Compare heap profiles** using `go tool pprof` to identify top retained allocations
3. **Analyze heap objects delta** (`heap_objects_start` vs `heap_objects_end`)
4. If attribution artifacts show **plateau in heap_sys** or stable `heap_alloc` after midpoint, confirm bounded warmup
5. If attribution artifacts show **continuous heap growth** without plateau, investigate specific allocation paths

## Privacy Compliance

✅ Analysis uses only infrastructure metrics: RSS, PSS, heap stats, goroutine count.  
✅ No browsing history, destination flows, or user-behavior data.  
✅ Compliant with KGB privacy doctrine.

## Source Artifacts

- `uvb76-leak-slope-28024460972.json`
- `tovarisch-leak-slope-28024460972.json`
- `manifest.yaml`
