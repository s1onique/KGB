# ACT-HULK29R-ZIG016-MEMOWN09-MEMORY-INVENTORY-SELFTEST-FIX

## Status

**COMPLETE** — 2026-07-06

## Goal

Restore `make gate` by fixing pre-existing memory ownership inventory verifier self-test failures.

## Root Cause

MEMOWN06 added coverage-reference validation requiring `verified=yes` rows with non-`n/a` coverage to have the coverage string present in:
- The same source file
- A Zig test file under `tovarisch/src`
- A Python test file under `tests/`

Several self-test fixtures used synthetic inventory rows with `verified=yes` and fake coverage values, but the temporary repositories didn't include matching source/test content.

## Changes

### `tests/test_verify_memory_ownership_inventory.py`

1. **`test_finds_allocator_free`**: Fixed fixture content to include `allocator.free(` pattern directly instead of relying on method call delegation.

2. **`test_consumer_without_deinit_defer`**: Changed `coverage=test` to `coverage=n/a` since this test is not about coverage validation.

3. **`test_allocation_free_row_passes_with_review_note`**: Changed `coverage=status tests` to `coverage=n/a` since this test is about allocation-free review notes.

4. **`test_allocation_free_row_passes_with_value_only_note`**: Changed `coverage=status tests` to `coverage=n/a` since this test is about value-only notes.

5. **`test_consumer_with_allocator_free_passes`**: Changed `coverage=tests` to `coverage=n/a` since this test is about `allocator.free` cleanup detection.

6. Updated assertion in `test_consumer_without_deinit_defer` to match expanded error message pattern `"lacks \`.deinit(\`` (verifier now includes `allocator.free` in the message).

### `scripts/verify_memory_ownership_inventory.py`

1. **Case 1 (valid minimal inventory)**: Changed `coverage=test_deinit` to `coverage=n/a` for both MEMOWN-0001 and MEMOWN-0002 rows.

2. **Case 11 (consumer without deinit/defer)**: Changed `coverage=test` to `coverage=n/a` to match the test assertion pattern.

3. Updated self-test error pattern check from `"lacks \`.deinit(\` or \`defer\`"` to `"lacks \`.deinit(\`" to match the expanded error message.

## Verification

```bash
# Self-test passes
python3 tests/test_verify_memory_ownership_inventory.py -v
# Result: OK (37 tests)

# Real inventory verifier passes
python3 scripts/verify_memory_ownership_inventory.py
# Result: MEMORY OWNERSHIP INVENTORY VERIFIER: PASS

# CLI composition verifier passes
python3 scripts/verify_cli_composition_inventory.py
# Result: === VERIFICATION PASSED ===

# RSS canary works
python3 scripts/tovarisch_status_rss_canary.py --help
# Result: usage: tovarisch_status_rss_canary.py ...

# Full gate passes
make gate
# Result: [gate] PASS
```

## Constraints Honored

- ✅ Did not weaken real repository inventory validation
- ✅ Did not remove coverage-reference validation
- ✅ Did not special-case MEMOWN-0001 or MEMOWN-0002
- ✅ Did not bypass source-backed checks for tests
- ✅ All changed files under 450-line hard limit
- ✅ No RSS canary runtime behavior changes
- ✅ No CLI flag/schema changes
- ✅ No new dependencies added

## Files Changed

- `tests/test_verify_memory_ownership_inventory.py`
- `scripts/verify_memory_ownership_inventory.py`
