# Frontend Test Hygiene

## Problem

Raw Vitest can spawn unbounded worker processes that:
- Over-parallelize on local developer machines (CPU exhaustion)
- Accumulate stale processes after test runs
- Create overlapping test runs from the same checkout
- Make it impossible to clean up after interrupted runs

This is especially problematic in development when tests hang or developers Ctrl-C mid-run.

## Solution

All normal local/quality-gate test entrypoints must route through the bounded wrapper:

```
./scripts/run_frontend_tests.sh
```

## Safe Commands

### For Gate/CI (non-watch, bounded)

```bash
# Via wrapper (recommended)
./scripts/run_frontend_tests.sh

# Via npm scripts (also safe)
npm run test:run    # in uvb76/web/
npm run test:ci     # future: if added
```

### For Development (watch mode, human-only)

```bash
# Explicit escape hatch - NOT used by quality gate
npm run test:watch  # in uvb76/web/
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `UVB76_VITEST_MAX_WORKERS` | 4 | Max parallel workers |
| `UVB76_FRONTEND_TEST_TIMEOUT_SECONDS` | 600 | Outer timeout (10 min) |
| `UVB76_VITEST_TEST_TIMEOUT` | 10000 | Per-test timeout (10 sec) |
| `UVB76_REPO_PATH` | auto | Repo root for process detection |

## Wrapper Options

```
./scripts/run_frontend_tests.sh [OPTIONS]

OPTIONS:
  --profile        Print timing/logging for slow file discovery
  --shard N K      Run shard K of N (deterministic file sharding)
  --kill-stale     Kill stale Vitest/node processes before running
  -h, --help       Show help
```

## Process Hygiene

The wrapper enforces:

1. **Lock file** - Prevents concurrent test runs in the same checkout
2. **Stale process detection** - Detects Vitest/node processes from this repo
3. **Bounded workers** - Default 4 workers max
4. **Outer timeout** - Default 10 minutes
5. **Cleanup on exit** - SIGTERM then SIGKILL for any leftovers

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Tests passed |
| 1 | Tests failed or hygiene error |
| 2 | Lock conflict (another run in progress) |
| 3 | Stale processes detected (use `--kill-stale`) |
| 4 | Timeout or cleanup failure |

## Emergency Cleanup

If tests hang or leave orphaned processes:

```bash
# Option 1: Use the kill-stale flag
./scripts/run_frontend_tests.sh --kill-stale

# Option 2: Manual cleanup (only matching same-repo processes)
pkill -f "node.*vitest.*$(pwd)"
pkill -f "vitest.*$(pwd)"

# Option 3: Remove stale lock
rm -f .tmp/frontend-test-locks/wrapper.lock
```

## Verifier

The hygiene verifier scans gate scripts and CI configs:

```bash
# Check for violations
python3 scripts/verify_frontend_test_hygiene.py

# Self-test
python3 scripts/verify_frontend_test_hygiene.py --self-test
```

It fails if gate/CI paths use:
- Raw `npm test`
- Raw `vitest` (without `run` mode)
- Any unbounded watch-mode in automated paths

## Why Watch Mode is Forbidden in Gate

Watch mode (`vitest --watch`) can:
- Spawn unlimited workers over time
- Hang indefinitely waiting for file changes
- Interfere with concurrent test runs
- Make process cleanup impossible

The explicit `test:watch` script exists for developers who need it, but gate/CI must never use it.

## Troubleshooting

### "Another frontend test run is active"

Remove stale lock:
```bash
rm -f .tmp/frontend-test-locks/wrapper.lock
```

Or kill stale processes:
```bash
./scripts/run_frontend_tests.sh --kill-stale
```

### High CPU from multiple vitest workers

The wrapper caps workers at 4 by default. If you need fewer:
```bash
UVB76_VITEST_MAX_WORKERS=2 ./scripts/run_frontend_tests.sh
```

### Tests timeout

Increase the outer timeout:
```bash
UVB76_FRONTEND_TEST_TIMEOUT_SECONDS=1200 ./scripts/run_frontend_tests.sh
