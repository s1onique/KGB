# ACT-HULK29R-ZIG016-MEMOWN05-STATUS-RSS-CANARY

## Status: Complete

## Motivation

MEMOWN01 introduced `OwnedWgCommandResult.deinit()`, MEMOWN02 added the `WgCommandRunner` seam and allocator-backed tests, MEMOWN03 added a verifier rejecting doc-only regression tests, and MEMOWN04 added a memory ownership inventory. This ACT adds a practical runtime smoke/canary check for memory behavior that unit tests cannot fully prove, including allocator retention, libc/syscall behavior, request-level caches, accidental global retention, and endpoint composition leaks.

## Purpose

The `/status` endpoint memory canary complements `std.testing.allocator` unit tests by measuring real process memory under repeated HTTP requests. While unit tests verify allocator bookkeeping, they cannot detect:
- Allocator retention behavior across process lifetime
- libc/syscall memory management quirks
- Request-level caching that survives request boundaries
- Accidental global variable retention
- Endpoint composition leaks from shared state

## Memory Source Selection

The canary prefers `/proc/<pid>/smaps_rollup` when available, falling back to `/proc/<pid>/status`:

- **smaps_rollup fields**: Rss, Pss, Private_Clean, Private_Dirty, Shared_Clean, Shared_Dirty
- **Computed**: `private_kib = Private_Clean + Private_Dirty`
- **status fallback fields**: VmRSS, RssAnon, RssFile, RssShmem, VmData
- **Fallback private**: Uses RssAnon when available; falls back to VmRSS as upper bound

Linux kernel docs describe `smaps_rollup` as a single rollup containing sums of the corresponding `smaps` fields for the whole process, and the `proc_pid_status(5)` manpage documents `VmRSS` as resident set size.

## Changes

### Added Files

**scripts/tovarisch_status_rss_canary.py**
- Python standard library only (argparse, json, os, platform, sys, time, urllib.request, urllib.error)
- Required args: `--url`, `--pid`
- Optional args: `--warmup-requests` (25), `--sample-requests` (200), `--interval-seconds` (0.0), `--timeout-seconds` (2.0), `--max-rss-kib-growth` (4096), `--max-private-kib-growth` (1024)
- Output modes: `--output text|json` (default: text)
- Flags: `--allow-missing-smaps-rollup`, `--verbose`
- Exit codes: 0=PASS, 1=FAIL, 2=SKIP, 3=ERROR

**tests/test_tovarisch_status_rss_canary.py**
- Unit tests covering:
  1. Parses smaps_rollup fields and computes private_kib
  2. Parses /proc/PID/status fallback fields
  3. Chooses smaps_rollup when available
  4. Falls back to status when smaps_rollup missing and fallback allowed
  5. Returns SKIP when proc files missing
  6. Computes deltas correctly
  7. Passes when rss/private deltas are below thresholds
  8. Fails when rss delta exceeds threshold
  9. Fails when private delta exceeds threshold
  10. JSON output contains required keys
  11. HTTP request helper treats 2xx non-empty body as success
  12. HTTP request helper rejects empty body
  13. HTTP request helper rejects non-2xx
  14. CLI argument parser defaults are correct

**docs/acts/ACT-HULK29R-ZIG016-MEMOWN05-STATUS-RSS-CANARY.md**
- This documentation file

### Modified Files

**Makefile**
- Added `tovarisch-status-rss-canary` target (manual, requires TOVARISCH_STATUS_URL and TOVARISCH_PID)
- Added `tovarisch-status-rss-canary-local` target (manual, uses default localhost URL)

**scripts/quality_gate.sh**
- Added new files to required array:
  - scripts/tovarisch_status_rss_canary.py
  - tests/test_tovarisch_status_rss_canary.py
- Added gate line: `[gate] checking tovarisch status RSS canary self-test (HULK29R-ZIG016-MEMOWN05)`

(No CLI composition inventory row added - test uses unittest.mock.patch, not subprocess)

**docs/tooling/memory-ownership-inventory.csv**
- Added MEMOWN-0011: tovarisch_status_rss_canary.py verifier entry

## Threshold Rationale

Default thresholds are intentionally generous:
- `max_rss_kib_growth = 4096` (4 MiB RSS growth)
- `max_private_kib_growth = 1024` (1 MiB private memory growth)

These thresholds accommodate:
- Allocator warmup behavior (initial allocations for thread arenas, mmap regions)
- Page mapping overhead during early request processing
- Platform/build-mode variations (debug vs release, glibc vs musl)

Thresholds are configurable to allow tightening for specific environments or relaxing when testing on constrained systems.

## Non-Goals

- Does NOT require a live tovarisch in default gate
- Does NOT run Linux-only live checks in CI by default
- Does NOT require curl, jq, ps, pmap, or valgrind
- Does NOT modify production service code
- Does NOT prove exact leak source
- Does NOT make thresholds non-configurable

## Measurement Model

```
1. Endpoint preflight: one request
2. Warmup phase: warmup_requests
3. Sleep 0.1s
4. Baseline memory sample
5. Sample phase: sample_requests
6. Sleep 0.1s
7. Final memory sample
8. Compare deltas
```

## Manual Usage Examples

```bash
# Basic usage with explicit URL and PID
python3 scripts/tovarisch_status_rss_canary.py \
  --url http://10.149.149.1:8317/status \
  --pid 2174927

# Using Make target with environment variables
TOVARISCH_STATUS_URL=http://10.149.149.1:8317/status TOVARISCH_PID=2174927 make tovarisch-status-rss-canary

# Local canary
TOVARISCH_PID=2174927 make tovarisch-status-rss-canary-local

# JSON output for automation
python3 scripts/tovarisch_status_rss_canary.py \
  --url http://127.0.0.1:8317/status \
  --pid 12345 \
  --output json \
  --verbose
```

## Verification

```bash
# Show help
python3 scripts/tovarisch_status_rss_canary.py --help

# Run unit tests
python3 tests/test_tovarisch_status_rss_canary.py -v

# Run gate
make gate
```

## Files Changed

- scripts/tovarisch_status_rss_canary.py (new)
- scripts/tovarisch_status_rss_canary_lib.py (new, implementation library)
- tests/tovarisch_status_rss_canary_test_support.py (new, test helper)
- tests/test_tovarisch_status_rss_canary_memory.py (new)
- tests/test_tovarisch_status_rss_canary_http.py (new)
- tests/test_tovarisch_status_rss_canary_output.py (new)
- tests/test_tovarisch_status_rss_canary_cli.py (new)
- tests/test_tovarisch_status_rss_canary_run.py (new)
- docs/acts/ACT-HULK29R-ZIG016-MEMOWN05-STATUS-RSS-CANARY.md (new)
- Makefile (modified)
- scripts/quality_gate.sh (modified)
- docs/tooling/memory-ownership-inventory.csv (modified)

Note: MEMOWN07 later split the monolithic files for LLM-friendliness.
See ACT-HULK29R-ZIG016-MEMOWN07-RSS-CANARY-FILE-SPLIT.md for details.

## Zig 0.16 Observations

No Zig 0.16-specific issues encountered. This ACT focuses on Python tooling for runtime verification.
