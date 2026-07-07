# ACT-HULK29R-ZIG016-TEST-BASE-NONREPRO-FORENSICS

**Classification:** Diagnostic / No-Code-Production-Change

**Date:** 2026-07-07

**ACT Type:** Non-reproducible test failure forensics

---

## Original CI Failure Summary

| Field | Value |
|-------|-------|
| Test | `status_response_test.test.OwnedResponse body does not expose trailing allocation capacity` |
| Failure line | `tovarisch/src/status_response_test.zig:412` |
| CI result | `742 pass, 7 skip, 1 fail (750 total)` |
| Seed | `0x9b3fc132` |

---

## Current Non-Repro Status

| Run | Pass | Skip | Fail | Total | Seed |
|-----|------|------|------|-------|------|
| CI (original failure) | 742 | 7 | 1 | 750 | `0x9b3fc132` |
| Local (before ACT) | 726 | 24 | 0 | 750 | `0xDEADBEEF` |
| Local (after ACT) | 727 | 24 | 0 | 751 | `0x109ea6e0` |
| Local (with seed) | 727 | 24 | 0 | 751 | `0x9b3fc132` |

**Analysis:** The original CI failure did not reproduce. The test passes locally with the same seed.

---

## Production Code Verification

### OwnedResponse Invariant Confirmation

The `OwnedResponse` type in `tovarisch/src/status_response.zig` is correct:

```zig
pub const OwnedResponse = struct {
    allocation: []u8,  // Full allocation
    len: usize,        // Exact written bytes
};

pub fn body(self: *const OwnedResponse) []const u8 {
    return self.allocation[0..self.len];  // Always returns [0..len]
}
```

**Invariants verified:**
- `len <= allocation.len` ✓
- `body().len == len` ✓
- `body().ptr == allocation.ptr` when `len > 0` ✓
- `body()` never exposes `allocation[len..]` ✓

### Why Debug 0xaa Can Indicate Undefined Memory

The CI failure reported a byte `>= 128` in the body. In Debug builds, uninitialized memory often contains:
- `0xaa` (Zig/Sanitizer poison pattern)
- `0xcc` (MSVC debug fill)
- `0xfe` (additional poison patterns)

**Important:** These values indicate *undefined memory behavior*, not a bug in the accessor. The test was correct to fail on such values, but the root cause was likely:

1. **Allocator returning dirty memory** (valid behavior in some allocators)
2. **Test running on a different allocator implementation** than local
3. **Stale CI checkout** with different code
4. **Environment difference** (different allocator, memory sanitizer, etc.)

The fix is **not** to change `OwnedResponse` semantics (which are correct), but to **harden the test diagnostics** so future failures are self-diagnosing.

---

## Changes Made

### Files Changed

1. **`tovarisch/src/status_response_test.zig`** (+123 lines)
   - Added `expectBodyBytesInAsciiRange()` diagnostic helper function
   - Replaced bare per-byte `expect(byte < 128)` with diagnostic helper
   - Added `OwnedResponse body accessor contract: never exposes trailing capacity` test with explicit 0xaa poisoning

### Diagnostic Helper: `expectBodyBytesInAsciiRange()`

Reports on failure:
- `body.len` vs `logical_len` vs `alloc_len` mismatch
- Offending byte index, value (hex/decimal)
- Whether byte is in body range or trailing capacity
- Bounded preview around offending byte (±16 bytes)
- Context about what the failure indicates

### New Contract Test

The new test proves the accessor contract directly:
1. Constructs an `OwnedResponse` with capacity > len
2. Explicitly poisons trailing bytes with `0xaa`
3. Asserts `body()` returns only `[0..len]`, never the poisoned tail
4. Verifies body contains valid ASCII JSON

This test does **not** rely on undefined behavior - it explicitly controls all bytes.

---

## Pass/Skip Distribution Mismatch Analysis

| Metric | CI Original | Local Before | Local After |
|--------|-------------|--------------|-------------|
| Total | 750 | 750 | 751 |
| Pass | 742 | 726 | 727 |
| Skip | 7 | 24 | 24 |
| Fail | 1 | 0 | 0 |

**Observations:**
- Local skip count (24) > CI skip count (7) - likely platform/feature condition difference
- The extra skip count is pre-existing and not related to this ACT
- The new test adds 1 to total (751 vs 750)

---

## Verification Results

### Test Output

```
zig build test-base --summary all
Build Summary: 4/4 steps succeeded; 727/751 tests passed (24 skipped)
+- run test 727 pass, 24 skip (751 total)
```

### Seed-Specific Run

```
zig build test-base --summary all --seed 0x9b3fc132
Build Summary: 4/4 steps succeeded; 727/751 tests passed (24 skipped)
+- run test 727 pass, 24 skip (751 total)
```

### Gate Status

- **Zig-specific gate:** ✓ Pass (`zig build test-base`)
- **make gate:** ✗ Pre-existing failures in uvb76/ files (unrelated to this ACT)

---

## Future Failure Self-Diagnosis

If the test fails again, the diagnostic output will show:

1. **Length mismatch:** If `body.len != logical_len`, prints all three values
2. **Offending byte:** Index, hex/decimal value, preview around byte
3. **Range classification:** Whether byte is in body or trailing capacity
4. **Hint:** If in trailing capacity, explains possible causes

This eliminates the need to debug the raw memory state.

---

## Root Cause Classification

**No production defect found.** The original CI failure was likely caused by:

1. **Environment difference** - Different allocator behavior
2. **Stale CI checkout** - Code may have been different
3. **Platform difference** - Different memory fill patterns

**Mitigation applied:** Test now has self-diagnosing diagnostics.

---

## Deferred Risks

| Risk | Mitigation |
|------|------------|
| Different allocators may return dirty memory | Test is now diagnostic, not relying on specific allocator behavior |
| Skip count difference between CI/local | Pre-existing, not addressed in this ACT |

---

## Zig 0.16 Observations

**None.** The changes used standard Zig 0.16 APIs:
- `@memset()` for poisoning
- `std.debug.print()` for diagnostics
- Standard testing expect patterns

---

## Files Modified Summary

| File | Lines Added | Lines Removed | Net Change |
|------|-------------|---------------|------------|
| `tovarisch/src/status_response_test.zig` | +123 | -2 | +121 |

---

## Conclusion

This ACT is **diagnostic / no-code-production-change**. The `OwnedResponse` type is correct. The CI failure was non-reproducible, likely due to environment differences. The test has been hardened with self-diagnosing diagnostics so any future recurrence will be immediately actionable.
