# ACT-HULK29R-ZIG016-MEMOWN04-MEMORY-OWNERSHIP-INVENTORY

## Status: Complete

## Motivation

MEMOWN01 introduced `OwnedWgCommandResult.deinit()`, MEMOWN02 added the `WgCommandRunner` seam and allocator-backed tests, and MEMOWN03 added a verifier rejecting doc-only regression tests. This ACT adds a memory ownership inventory and verifier so future allocator-producing functions cannot silently return owned memory without documented ownership, cleanup method, and executable test coverage.

## Changes

### Added Files

**docs/tooling/memory-ownership-inventory.csv**
- Memory ownership inventory with columns: id, path, language, symbol, kind, allocator_boundary, owned_type, owner, cleanup, coverage, request_path, verified, notes
- Initial rows covering:
  - MEMOWN-0001: OwnedWgCommandResult (owned_type)
  - MEMOWN-0002: runWgShowDump (producer)
  - MEMOWN-0003: cliWireguardStatusWithRunner (consumer)
  - MEMOWN-0004: FakeWgCommandRunner.run (test producer)
  - MEMOWN-0005-0009: MEMOWN01/MEMOWN02 allocator-backed tests
  - MEMOWN-0010: verify_no_doconly_regression_tests (verifier)

**scripts/verify_memory_ownership_inventory.py**
- Verifier performing CSV schema checks:
  - Required header validation
  - Stable ID format (MEMOWN-\d{4})
  - Path existence
  - Enum value validation (kind, allocator_boundary, request_path, verified)
  - Duplicate ID detection
- Source-backed ownership checks:
  - owned_type rows with cleanup=deinit must have fn deinit
  - producer rows with allocator_boundary=returns_owned must have errdefer
  - consumer rows with allocator_boundary=consumes_owned must have .deinit or defer
  - test rows with cleanup=std.testing.allocator must have std.testing.allocator in body
  - request_path=yes rows must have verified=yes
- Built-in self-test via --self-test flag

**tests/test_verify_memory_ownership_inventory.py**
- Unit tests for all helper functions
- Test cases covering:
  1. Valid minimal inventory passes
  2. Missing inventory file fails
  3. Wrong header fails
  4. Duplicate IDs fail
  5. Malformed ID fails
  6. Nonexistent path fails
  7. Invalid kind fails
  8. Invalid allocator_boundary fails
  9. owned_type without deinit fails
  10. producer without errdefer fails
  11. consumer without deinit/defer fails
  12. std.testing.allocator test without std.testing.allocator fails
  13. request_path=yes with verified=no fails
  14. Repo-relative diagnostics
  15. Real repository inventory integration test

### Modified Files

**scripts/quality_gate.sh**
- Added new files to required array:
  - docs/tooling/memory-ownership-inventory.csv
  - scripts/verify_memory_ownership_inventory.py
  - tests/test_verify_memory_ownership_inventory.py
- Added gate lines:
  - `[gate] checking memory ownership inventory (HULK29R-ZIG016-MEMOWN04)`
  - `[gate] checking memory ownership inventory self-test (HULK29R-ZIG016-MEMOWN04)`

## Verification

```bash
# Run the verifier
python3 scripts/verify_memory_ownership_inventory.py

# Run the self-test
python3 tests/test_verify_memory_ownership_inventory.py -v

# Run the full gate
make gate
```

## Non-Goals (Not Implemented)

- Full Zig parser (pragmatic source scanning used)
- Memory ownership across Go/Python code
- Runtime RSS canaries
- Valgrind or Linux-only tooling
- Complete repo inventory

## Files Changed

- docs/tooling/memory-ownership-inventory.csv (new)
- scripts/verify_memory_ownership_inventory.py (new)
- tests/test_verify_memory_ownership_inventory.py (new)
- scripts/quality_gate.sh (modified)

## Zig 0.16 Observations

No Zig 0.16-specific issues encountered. This ACT focuses on Python tooling verification.
