# ACT-HULK29R-ZIG016-MEMOWN03-NO-DOCONLY-REGRESSION-TESTS

## Status

**CLOSED** ✓

## Summary

Added a hygiene verifier that rejects ACT/regression/leak/memory-style Zig tests when they are documentation-only placeholders such as `try std.testing.expect(true);`. The verifier uses pragmatic source scanning with repo-relative diagnostics and is wired into the quality gate.

## Motivation

After MEMOWN02 (ACT-HULK29R-ZIG016-MEMOWN02-COMMAND-RUNNER-SEAM), the real ownership contract is executable through `WgCommandRunner`, `FakeWgCommandRunner`, and repeated `std.testing.allocator` tests. However, a placeholder-style test was present:

```zig
test "CliBackend CLI-path: requires integration test environment" {
    try std.testing.expect(true);
}
```

This kind of test can be useful as a note but must not be allowed to masquerade as a memory/leak/regression/ACT contract. This ACT adds a hygiene verifier to catch future placeholder tests before they enter `make gate`.

## Implementation

### Files Added

- `scripts/verify_no_doconly_regression_tests.py` — Main verifier
- `tests/test_verify_no_doconly_regression_tests.py` — Unit tests

### Files Modified

- `scripts/quality_gate.sh` — Wired in the verifier

### Detection Logic

The verifier scans Zig test files and fails when a test appears to be a regression/ACT/memory/leak contract but its body is documentation-only.

**Contract-like markers** (case-insensitive):
- `ACT-`
- `regression`
- `leak`
- `memory leak`
- `memory ownership`
- `RSS`
- `ownership contract`

**Trivial body patterns** (fail if only these are present):
- `try std.testing.expect(true);`
- `try std.testing.expectEqual(true, true);`
- `return;`

**Meaningful evidence** (test passes if body contains any):
- `std.testing.allocator`
- `.deinit(`
- `allocator.alloc`, `allocator.dupe`, `allocator.free`
- `Fake`, `asRunner(`
- `cliWireguardStatusWithRunner`, `parseWgDumpOutput`
- `while (`, `for (`

### Example Diagnostics

```
tovarisch/src/net/wg_status_boundary_test.zig:319: documentation-only regression test:
  test "CliBackend CLI-path: requires integration test environment"
  reason: contract-like marker found near test, but body only asserts true
  fix: remove the test, rename it as documentation, or replace it with an executable seam/allocator test
```

## Test Coverage

Unit tests cover:
1. ACT-marked test with only `try std.testing.expect(true);` → fails
2. Regression-marked test with comments plus trivial assertion → fails
3. MEMOWN02-style test using `std.testing.allocator` and `FakeWgCommandRunner` → passes
4. Ordinary smoke test with `expect(true)` when not marked as ACT/regression/leak/memory → passes
5. Parser test that calls `parseWgDumpOutput` and asserts a real field → passes
6. Repo-relative path and line number reporting
7. Multiple test blocks in one file
8. Nested braces inside loops or struct literals

## Acceptance Criteria

- [x] `scripts/verify_no_doconly_regression_tests.py` exists
- [x] Verifier scans Zig test files for ACT/regression/leak/memory contract-like tests
- [x] Verifier rejects documentation-only tests that only assert true
- [x] Verifier allows ordinary non-contract smoke tests
- [x] Verifier allows MEMOWN02 allocator-backed runner seam tests
- [x] Unit tests cover fail/pass cases and repo-relative diagnostics
- [x] `scripts/quality_gate.sh` runs the verifier
- [x] `python tests/test_verify_no_doconly_regression_tests.py -v` passes
- [x] `make gate` passes

## Non-Goals

- Do not build a full Zig parser
- Do not prove every regression test is good
- Do not ban all `expect(true)` usage globally
- Do not change WireGuard status production code
- Do not add runtime RSS canaries yet

## Verification

```bash
# Run the verifier
python3 scripts/verify_no_doconly_regression_tests.py

# Run the verifier's self-test
python3 scripts/verify_no_doconly_regression_tests.py --self-test

# Run unit tests
python3 tests/test_verify_no_doconly_regression_tests.py -v

# Run full quality gate
make gate
```

## Related ACTs

- ACT-HULK29R-ZIG016-STATUS-RSS-REQUEST-LEAK — Fixed RSS leak in CLI path
- ACT-HULK29R-ZIG016-MEMOWN01-OWNED-COMMAND-RESULT — OwnedWgCommandResult deinit
- ACT-HULK29R-ZIG016-MEMOWN02-COMMAND-RUNNER-SEAM — Command runner seam with FakeWgCommandRunner
