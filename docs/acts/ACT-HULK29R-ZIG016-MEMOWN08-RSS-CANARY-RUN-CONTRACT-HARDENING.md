# ACT-HULK29R-ZIG016-MEMOWN08-RSS-CANARY-RUN-CONTRACT-HARDENING

## Status: Complete

## Motivation

MEMOWN05/MEMOWN07 added a runtime RSS canary for the tovarisch /status endpoint. Existing tests verify parsing, HTTP helper behavior, output formatting, CLI defaults, and basic pass/fail outcomes. However, threshold-only tests do not prove phase behavior.

This ACT hardens `run_canary()` by adding executable phase contract tests that verify the exact runtime sequence:

```text
preflight HTTP request
warmup HTTP requests
baseline memory sample
sample HTTP requests
final memory sample
delta calculation
threshold evaluation
```

This makes the RSS canary harder to accidentally weaken while refactoring.

## Changes

### New Test File

**tests/test_tovarisch_status_rss_canary_run_contract.py** (new, ~370 lines)
- Phase contract tests for `run_canary()` execution sequence
- Uses `unittest.mock` for all mocking (no subprocess, no live network, no real /proc)
- Includes `_run_with_mocks()` helper method for consistent test setup
- All 10 required tests implemented (see below)

### Updated Files

**scripts/quality_gate.sh**
- Added `tests/test_tovarisch_status_rss_canary_run_contract.py` to required files array
- Added test execution command to RSS canary test block

## Phase Contract Tests Added

### 1. test_run_canary_success_uses_expected_http_request_count
Verifies exact HTTP request count: preflight + warmup + sample.
- warmup_requests=3, sample_requests=5 → expected 9 total calls
- Asserts all calls use configured URL and timeout

### 2. test_run_canary_success_samples_memory_before_and_after_sample_phase
Verifies exactly two memory samples: baseline then final.
- memory_samples = [(Rss=1000), (Rss=1100)]
- Asserts: rss_kib_delta=100, private_kib_delta=50

### 3. test_run_canary_success_preserves_phase_order
Verifies exact phase order with event tracking.
- warmup_requests=2, sample_requests=3, interval_seconds=0.0
- Expected: http, http, http, sleep:0.1, memory, http, http, http, sleep:0.1, memory

### 4. test_run_canary_interval_sleep_count_matches_request_phases
Verifies interval sleeps between warmup and sample requests.
- warmup_requests=2, sample_requests=3, interval_seconds=0.25
- Expected 7 sleep calls: 2 warmup intervals, 1 baseline pause, 3 sample intervals, 1 final pause

### 5. test_run_canary_preflight_failure_short_circuits
Preflight HTTP failure skips warmup and memory sampling.
- Asserts: status=fail, reason starts with endpoint_unreachable
- http_get called once, get_memory_source not called

### 6. test_run_canary_warmup_failure_short_circuits_before_baseline
Warmup HTTP failure skips baseline memory sampling.
- warmup_requests=3, fails on 4th call (warmup #3)
- Asserts: status=fail, reason starts with warmup_request_failed
- get_memory_source not called, no memory fields populated

### 7. test_run_canary_baseline_memory_failure_skips_before_sample_phase
Baseline memory failure skips sample phase.
- First get_memory_source returns None metrics
- Asserts: status=skip, reason=proc_files_missing
- Sample HTTP requests not sent

### 8. test_run_canary_sample_failure_short_circuits_before_final_memory
Sample HTTP failure skips final memory sampling.
- Fails on sample #2 (with 3 sample requests)
- Asserts: status=fail, reason starts with sample_request_failed
- get_memory_source called once (baseline only)

### 9. test_run_canary_final_memory_failure_skips_after_sample_phase
Final memory failure skips threshold evaluation.
- All sample requests sent successfully
- Final get_memory_source returns None metrics
- Asserts: status=skip, reason=proc_files_missing
- rss_kib_after, rss_kib_delta, private_kib_delta all None

### 10. test_run_canary_rss_threshold_reason_wins_when_both_exceed
RSS threshold reason takes precedence when both RSS and private exceed.
- Both RSS delta (5000) and private delta (1900) exceed thresholds
- Asserts: reason=rss_kib_delta_exceeded (RSS checked first)

## File Line Counts

| File | Lines |
|------|-------|
| tests/test_tovarisch_status_rss_canary_run_contract.py | ~370 |

All test files remain under the 450-line hard limit.

## Behavior Preserved

- Live canary semantics unchanged
- All CLI flags, defaults, output schema, exit codes unchanged
- No new dependencies introduced
- No subprocess usage in tests
- No live network, real /proc, or real time.sleep in tests

## Verification

```bash
# Show help
python3 scripts/tovarisch_status_rss_canary.py --help

# Run all RSS canary unit tests
python3 tests/test_tovarisch_status_rss_canary_memory.py -v
python3 tests/test_tovarisch_status_rss_canary_http.py -v
python3 tests/test_tovarisch_status_rss_canary_output.py -v
python3 tests/test_tovarisch_status_rss_canary_cli.py -v
python3 tests/test_tovarisch_status_rss_canary_run.py -v
python3 tests/test_tovarisch_status_rss_canary_run_contract.py -v

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
- No subprocess/effect usage introduced
- Implementation library not modified (no production code changes)

## Zig 0.16 Observations

No Zig 0.16-specific issues encountered. This ACT focuses on Python test hardening for the RSS canary.
