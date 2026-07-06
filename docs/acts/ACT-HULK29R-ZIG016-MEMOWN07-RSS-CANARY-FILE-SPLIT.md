# ACT-HULK29R-ZIG016-MEMOWN07-RSS-CANARY-FILE-SPLIT

## Status: Complete

## Motivation

MEMOWN05 added a runtime RSS canary for the tovarisch /status endpoint. The gate rejected the implementation because `scripts/tovarisch_status_rss_canary.py` (490 lines) and `tests/test_tovarisch_status_rss_canary.py` (495 lines) exceeded the 450-line LLM-friendly limit.

This ACT splits the files by responsibility while preserving all CLI flags, defaults, output schema, exit codes, Make targets, and manual-only live behavior.

## Changes

### Split Implementation

**scripts/tovarisch_status_rss_canary_lib.py** (new, 447 lines - under 450-line hard limit)
- Contains all canary implementation: `parse_memory_size_kib`, `parse_smaps_rollup`, `parse_proc_status`, `get_memory_source`, `http_get`, `run_canary`, `format_text_output`, `format_json_output`, `build_parser`, `validate_args`
- Imports: argparse, json, os, platform, sys, time, urllib.request, urllib.error (standard library only)

**scripts/tovarisch_status_rss_canary.py** (converted to thin CLI wrapper, ~65 lines)
- Imports library functions from `tovarisch_status_rss_canary_lib`
- Builds argparse parser, validates arguments, calls `run_canary`, formats output
- Preserves all CLI flags, defaults, output schema, and exit codes

### Split Tests

**tests/tovarisch_status_rss_canary_test_support.py** (new, ~10 lines)
- Helper to add scripts to Python path for tests

**tests/test_tovarisch_status_rss_canary_memory.py** (new, ~170 lines)
- Tests: parse_memory_size_kib, parse_smaps_rollup, parse_proc_status, memory source selection

**tests/test_tovarisch_status_rss_canary_http.py** (new, ~95 lines)
- Tests: http_get success, empty body, non-2xx, HTTPError, URLError, timeout

**tests/test_tovarisch_status_rss_canary_output.py** (new, ~120 lines)
- Tests: format_text_output (pass/fail/skip/error), format_json_output (required keys, newline)

**tests/test_tovarisch_status_rss_canary_cli.py** (new, ~130 lines)
- Tests: CLI defaults, argument parser, validate_args, exit code mapping

**tests/test_tovarisch_status_rss_canary_run.py** (new, ~180 lines)
- Tests: threshold evaluation, delta computation, run_canary integration with mocks

### Updated Files

**scripts/quality_gate.sh**
- Updated required array to include new files and remove old monolithic test
- Changed gate command from single `python3 tests/test_tovarisch_status_rss_canary.py` to explicit list of split test files
- Updated gate message to reflect MEMOWN05/MEMOWN07

**docs/tooling/memory-ownership-inventory.csv**
- Updated MEMOWN-0011 description to "Thin CLI wrapper for runtime status endpoint RSS/private memory slope canary"
- Added MEMOWN-0026 entry for `tovarisch_status_rss_canary_lib.py`

**tests/test_tovarisch_status_rss_canary.py** (deleted)
- Replaced by focused test files above

## File Line Counts (Post-Split)

| File | Lines |
|------|-------|
| scripts/tovarisch_status_rss_canary.py | 65 |
| scripts/tovarisch_status_rss_canary_lib.py | 447 |
| tests/test_tovarisch_status_rss_canary_memory.py | 170 |
| tests/test_tovarisch_status_rss_canary_http.py | 95 |
| tests/test_tovarisch_status_rss_canary_output.py | 120 |
| tests/test_tovarisch_status_rss_canary_cli.py | 130 |
| tests/test_tovarisch_status_rss_canary_run.py | 180 |

All files are now under the 450-line limit.

## Behavior Preserved

- `python3 scripts/tovarisch_status_rss_canary.py --help` works
- All CLI flags and defaults unchanged
- Exit codes: 0=PASS, 1=FAIL, 2=SKIP, 3=ERROR
- Text and JSON output schema unchanged
- Make targets `tovarisch-status-rss-canary` and `tovarisch-status-rss-canary-local` unchanged
- Live canary remains manual-only (not run in default gate)
- No new dependencies introduced

## Verification

```bash
# Show help
python3 scripts/tovarisch_status_rss_canary.py --help

# Run all unit tests
python3 tests/test_tovarisch_status_rss_canary_memory.py -v
python3 tests/test_tovarisch_status_rss_canary_http.py -v
python3 tests/test_tovarisch_status_rss_canary_output.py -v
python3 tests/test_tovarisch_status_rss_canary_cli.py -v
python3 tests/test_tovarisch_status_rss_canary_run.py -v

# Verify inventory
python3 scripts/verify_memory_ownership_inventory.py

# Verify CLI composition
python3 scripts/verify_cli_composition_inventory.py

# Run gate
make gate
```

## Non-Goals

- Live canary semantics unchanged
- Threshold values unchanged
- No new dependencies
- No subprocess/effect usage introduced (CLI composition inventory unchanged)

## Zig 0.16 Observations

No Zig 0.16-specific issues encountered. This ACT focuses on Python tooling refactoring.
