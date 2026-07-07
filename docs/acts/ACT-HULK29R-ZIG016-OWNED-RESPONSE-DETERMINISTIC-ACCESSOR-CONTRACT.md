# ACT-HULK29R-ZIG016-OWNED-RESPONSE-DETERMINISTIC-ACCESSOR-CONTRACT

**Classification:** Test-Only / Production-Code-Unchanged

**Date:** 2026-07-07

**ACT Type:** Deterministic test hardening for OwnedResponse contract

---

## Context

ACT-HULK29R-ZIG016-TEST-BASE-NONREPRO-FORENSICS classified the prior CI failure as **non-reproducible** and found no production `OwnedResponse` defect.

The forensic ACT added diagnostics and a new poison-tail contract test. However, that test had an early return:

```zig
if (response.len == response.allocation.len) {
    return;  // <-- Skip when allocator returns exact-fit allocation
}
```

This made the test **allocator-dependent** — it only proved the contract when the allocator happened to return spare capacity.

---

## Problem Statement

The contract test was not deterministic:

1. It relied on `OwnedResponse.init()` returning an allocation with spare capacity
2. If `response.len == response.allocation.len`, the test returned early
3. This left the trailing-capacity exclusion contract unproven in that run

The test needed to always prove the contract, regardless of allocator behavior.

---

## Solution

Replace the allocator-dependent test with a **deterministic fixture**:

1. Build `OwnedResponse` directly from controlled fields (struct fields are public)
2. Allocation is intentionally larger than len (guaranteed, not allocator-dependent)
3. Bytes after len are poisoned with `0xaa`
4. body() must return only `[0..len]`, never the poisoned tail

### Before (allocator-dependent)

```zig
test "OwnedResponse body accessor contract: never exposes trailing capacity" {
    var response = try OwnedResponse.init(allocator, inputs, query);
    defer response.deinit(allocator);
    
    // Early return if no trailing capacity!
    if (response.len == response.allocation.len) {
        return;  // <-- Did not prove contract
    }
    
    @memset(response.allocation[response.len..], 0xaa);
    const body = response.body();
    // ... assertions
}
```

### After (deterministic)

```zig
test "OwnedResponse body accessor contract: never exposes trailing capacity" {
    const logical_json = "{\"service\":\"tovarisch\",\"status\":\"ok\"}";
    const logical_len: usize = logical_json.len;
    const extra_capacity: usize = 64;
    
    // Allocate with explicit trailing capacity
    const full_alloc = try allocator.alloc(u8, logical_len + extra_capacity);
    defer allocator.free(full_alloc);
    
    @memcpy(full_alloc[0..logical_len], logical_json);
    @memset(full_alloc[logical_len..], 0xaa);
    
    // Build OwnedResponse directly
    var response = OwnedResponse{
        .allocation = full_alloc,
        .len = logical_len,
    };
    
    // Always assert the exact contract
    try std.testing.expect(response.len < response.allocation.len);
    try std.testing.expect(response.body().len == response.len);
    try std.testing.expect(response.body().ptr == response.allocation.ptr);
    // ... comprehensive assertions
}
```

---

## Diagnostic Wording Refinement

The `expectBodyBytesInAsciiRange()` helper was updated to clearly distinguish failure modes:

**Before:**
- Mixed `in_trailing` vs `in_body` classification that could confuse the reader

**After:**
- `BODY LENGTH CORRUPTION DETECTED` — when `body.len != logical_len`
  - `body.len > logical_len` would indicate accessor exposing trailing capacity
  - `body.len < logical_len` would indicate truncated write
- `NON-ASCII BYTE IN LOGICAL BODY` — when byte >= 128 in `[0..logical_len]`
  - Explicitly notes this indicates **data corruption**, NOT trailing exposure
  - Trailing exposure would first show as `body.len > logical_len`

---

## Files Changed

| File | Change |
|------|--------|
| `tovarisch/src/status_response_test.zig` | Refactored deterministic test fixture; refined diagnostic wording |

### Changes to `status_response_test.zig`

1. **Refactored `expectBodyBytesInAsciiRange()`** (+~15 lines)
   - Clearer failure message separation
   - Explicit note that trailing exposure manifests as length mismatch

2. **Replaced contract test** (~60 lines)
   - Deterministic fixture construction
   - 7 explicit contract assertions
   - No early return

---

## Verification Results

### Test Output (default seed)

```
zig build test-base --summary all
Build Summary: 4/4 steps succeeded; 727/751 tests passed (24 skipped)
test-base success
+- run test 727 pass, 24 skip (751 total) 1s MaxRSS:14M
```

### Test Output (seed 0x9b3fc132)

```
zig build test-base --summary all --seed 0x9b3fc132
Build Summary: 4/4 steps succeeded; 727/751 tests passed (24 skipped)
test-base success
+- run test 727 pass, 24 skip (751 total) 584ms MaxRSS:14M
```

### Gate Status

- **Zig-specific gate:** ✓ Pass (`zig build test-base`)
- **make gate:** ✗ Pre-existing failure in `uvb76/` files (LLM-friendliness check)
  - This failure is **unrelated** to this ACT
  - `tovarisch/` code is unaffected

---

## Production Code

**No production code was changed.**

The `OwnedResponse` type in `status_response.zig` was not modified. The test now:
- Builds `OwnedResponse` directly using its public fields
- Does not depend on allocator behavior
- Always proves the trailing-capacity exclusion contract

---

## Contract Proven

The test now deterministically proves:

1. `response.len < response.allocation.len` — Precondition (always has trailing capacity)
2. `response.body().len == response.len` — Accessor returns correct length
3. `response.body().ptr == response.allocation.ptr` — Accessor returns correct start
4. `response.body().len < response.allocation.len` — Accessor excludes tail
5. `body` contains only ASCII bytes — Valid data
6. `body` equals expected JSON — Exact content match
7. `allocation[response.len..]` still contains `0xaa` — Accessor never scanned past len

---

## Root Cause Classification

**Test quality issue, not production defect.**

- The original CI failure was allocator-dependent (confirmed by forensics ACT)
- The prior test skipped when allocator returned exact-fit allocation
- The new test eliminates the skip condition entirely

---

## Zig 0.16 Observations

**None.** Changes used standard Zig 0.16 APIs:
- `@memset()` for poisoning
- `@memcpy()` for copying
- `std.testing.expectEqualStrings()` for string comparison
- Public struct field initialization

---

## Conclusion

This ACT is **test-only / production-code-unchanged**.

The `OwnedResponse.body()` trailing-capacity contract is now **deterministic and allocator-independent**. Future test failures will clearly distinguish:

- **Body length exposure** — `body.len > logical_len` (accessor bug)
- **Truncated write** — `body.len < logical_len` (writer bug)
- **Data corruption** — non-ASCII byte in logical body (data source bug)
