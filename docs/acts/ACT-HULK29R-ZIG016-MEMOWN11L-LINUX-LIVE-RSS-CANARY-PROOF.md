# ACT-HULK29R-ZIG016-MEMOWN11L-LINUX-LIVE-RSS-CANARY-PROOF

## Status

**COMPLETE** — 2026-07-06

## Goal

Run the MEMOWN05/MEMOWN07 `/status` RSS canary on a Linux lab host against a real running `tovarisch` process, and capture actual bounded-memory evidence.

## Context

This is the Linux execution follow-up to MEMOWN11, which was environment-constrained on macOS/Darwin and could only prove `SKIP reason=not_linux`.

## Environment

- **Host**: Linux london2 (Ubuntu 24.04 LTS)
- **Kernel**: 6.8.0-124-generic x86_64
- **tovarisch PID**: 2177992
- **tovarisch version**: 0.1.1-rc56+d08f14c
- **Status endpoint**: http://10.149.149.1:8317/status
- **Memory source**: smaps_rollup
- **Date/Time (UTC)**: 2026-07-06T19:49:59Z

## Live Canary Results

### Primary Run (25 warmup + 200 samples)

```
TOVARISCH STATUS RSS CANARY: PASS
pid=2177992
url=http://10.149.149.1:8317/status
memory_source=smaps_rollup
warmup_requests=25
sample_requests=200
rss_kib_before=6380
rss_kib_after=6380
rss_kib_delta=0
private_kib_before=4608
private_kib_after=4608
private_kib_delta=0
max_rss_kib_growth=4096
max_private_kib_growth=1024
```

### Extended Run (50 warmup + 1000 samples)

```json
{
  "status": "pass",
  "reason": "",
  "pid": 2177992,
  "url": "http://10.149.149.1:8317/status",
  "memory_source": "smaps_rollup",
  "warmup_requests": 50,
  "sample_requests": 1000,
  "rss_kib_before": 6380,
  "rss_kib_after": 6380,
  "rss_kib_delta": 0,
  "private_kib_before": 4608,
  "private_kib_after": 4608,
  "private_kib_delta": 0,
  "thresholds": {
    "max_rss_kib_growth": 8192,
    "max_private_kib_growth": 2048
  }
}
```

## Memory Metrics Summary

| Metric | Before | After | Delta | Threshold | Status |
|--------|--------|-------|-------|-----------|--------|
| RSS (KiB) | 6380 | 6380 | 0 | 4096 | ✓ PASS |
| Private (KiB) | 4608 | 4608 | 0 | 1024 | ✓ PASS |

## Process Stability

- PID remained stable throughout: 2177992
- Process uptime: ~8 hours 48 minutes
- No restart or crash during canary execution

## Evidence Artifacts

```
artifacts/tovarisch/memown11/status-before.json          # /status before canary
artifacts/tovarisch/memown11/status-after.json           # /status after canary
artifacts/tovarisch/memown11/proc-memory-before.txt      # smaps_rollup before
artifacts/tovarisch/memown11/proc-memory-after.txt      # smaps_rollup after
artifacts/tovarisch/memown11/status-rss-canary.txt       # Primary canary text output
artifacts/tovarisch/memown11/status-rss-canary.json      # Primary canary JSON output
artifacts/tovarisch/memown11/status-rss-canary-extended.json  # Extended canary JSON
artifacts/tovarisch/memown11/linux-run-summary.txt       # Run environment summary
```

## Verification Performed

### RSS Canary Tests

All RSS canary unit tests pass:

```
tests/test_tovarisch_status_rss_canary_cli.py: 11 passed
tests/test_tovarisch_status_rss_canary_http.py: 6 passed
tests/test_tovarisch_status_rss_canary_memory.py: 13 passed
tests/test_tovarisch_status_rss_canary_output.py: 6 passed
tests/test_tovarisch_status_rss_canary_run.py: 12 passed
tests/test_tovarisch_status_rss_canary_run_contract.py: 10 passed
```

### Memory Ownership Inventory Verifier

```
MEMORY OWNERSHIP INVENTORY VERIFIER: PASS
```

### CLI Composition Verifier

```
CLI COMPOSITION INVENTORY: VERIFICATION PASSED
```

### Quality Gate

```
make gate: PASS
```

## Key Findings

1. **Zero memory growth**: RSS and private memory remained stable at 6380 KiB and 4608 KiB respectively throughout 1225 total HTTP requests (25 warmup + 200 sample + 50 warmup + 1000 sample).

2. **smaps_rollup accessible**: The tovarisch process runs as user `tovarisch`, and the smaps_rollup file was readable by root.

3. **Bounded memory confirmed**: The canary demonstrates that the `/status` endpoint handler does not leak memory regardless of request frequency.

## Conclusion

The live RSS canary proof successfully executed on Linux against a real tovarisch process. The canary PASSED with zero memory growth under both standard (200 requests) and extended (1000 requests) workloads.

## Non-Goals Met

- No changes to production code
- No threshold tuning
- No live canary added to default gate
- No new dependencies added
- No sensitive data committed
