# ACT-UVB76-HULK02R3-CAPTURE-STATE-VERIFIER-LINE-LIMIT-SPLIT

## Status

Complete

## Root Cause

`scripts/verify_uvb76_capture_state_contracts.py` had grown to 660 lines, exceeding the 450-line LLM-friendly limit and blocking gate.

## Fix

Split the monolithic verifier into a thin CLI wrapper and focused implementation modules under `scripts/uvb76_capture_state_contracts/`.

Preserved capture-state contract semantics, canonical status checks, JSON normalization, skip allowlist behavior, and line-limit validation.

## Files Changed

- `scripts/verify_uvb76_capture_state_contracts.py` - Thin CLI wrapper (24 lines)
- `scripts/uvb76_capture_state_contracts/__init__.py` - Package marker (3 lines)
- `scripts/uvb76_capture_state_contracts/constants.py` - Canonical statuses, reasons, file inventory (75 lines)
- `scripts/uvb76_capture_state_contracts/inventory.py` - Split-file inventory validation (124 lines)
- `scripts/uvb76_capture_state_contracts/json_normalize.py` - JSON string stripping/normalization (71 lines)
- `scripts/uvb76_capture_state_contracts/status_contract.py` - Canonical status/reason validation (215 lines)
- `scripts/uvb76_capture_state_contracts/skip_allowlist.py` - Skip allowlist handling (83 lines)
- `scripts/uvb76_capture_state_contracts/line_limits.py` - Line-count checking (152 lines)
- `scripts/uvb76_capture_state_contracts/runner.py` - Orchestration logic (369 lines)

## Verification

```
$ python3 scripts/verify_uvb76_capture_state_contracts.py: PASS
$ python3 scripts/verify_uvb76_capture_state_contracts.py --self-test: PASS (7/7 tests)
$ python3 scripts/verify_uvb76_runtime_contracts_selftest.py: PASS
$ make hulk-uvb76-capture-gate: PASS
$ make llm-friendliness: PASS
```

## Line Counts

| File | Lines | Limit |
|------|-------|-------|
| scripts/verify_uvb76_capture_state_contracts.py | 24 | 450 |
| scripts/uvb76_capture_state_contracts/__init__.py | 3 | 450 |
| scripts/uvb76_capture_state_contracts/constants.py | 75 | 450 |
| scripts/uvb76_capture_state_contracts/inventory.py | 124 | 450 |
| scripts/uvb76_capture_state_contracts/json_normalize.py | 71 | 450 |
| scripts/uvb76_capture_state_contracts/line_limits.py | 152 | 450 |
| scripts/uvb76_capture_state_contracts/runner.py | 369 | 450 |
| scripts/uvb76_capture_state_contracts/skip_allowlist.py | 83 | 450 |
| scripts/uvb76_capture_state_contracts/status_contract.py | 215 | 450 |

## Pre-existing Blocker

- `make hygiene-gate` / `make gate` fails on pre-existing issue:
  - `external-analysis/tovarisch-test-base-fingerprint.json` missing final newline
  - This is unrelated to this ACT; follows the Tovarisch fingerprint ACT
  - Per task instructions: do not touch the completed Tovarisch fingerprint ACT

## Assumptions

- The LLM-friendly checker uses a 300-line soft limit for scripts/
- Package modules inherit the same limits as top-level scripts
- The thin CLI wrapper correctly resolves paths via sys.argv[0] rather than __file__

## Non-Goals Met

- Did not change capture-state contract semantics
- Did not relax canonical status checks
- Did not expand skip allowlists
- Did not change diagnostic capture status names
- Did not rewrite unrelated UVB-76 capture logic
- Did not touch the completed Tovarisch fingerprint ACT
- Did not fix deferred diagnostic capture service skip debt
