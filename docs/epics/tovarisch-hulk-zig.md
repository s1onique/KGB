# [Closed] Epic: Tovarisch Hulk-Zig Memory Hardening

**Epic ID**: ACT-TOVARISCH-ZIG-HULK19
**Status**: CLOSED
**Closed**: 2026-07-05
**Commit**: b370d7212aec999f252c4b7cae21374bbfcc310f

## Goal

Harden Tovarisch Zig daemon surfaces against memory leaks, unbounded growth, and RSS inflation through incremental HULK iterations culminating in enforceable gates.

---

## Summary

Tovarisch Hulk-Zig epic is **CLOSED**. All production surfaces have been audited, hardened or documented, and gates are enforcing RISKY-HIGH patterns.

**Final state**: 0 RISKY-HIGH, 30 RISKY-MEDIUM report-only, 4 RISKY-LOW report-only, 28 DEFERRED report-only; all gates pass.

---

## 1. Hardened Surfaces

### 1.1 /status.json Request/Response Budgeted Rendering

**Classification**: PRODUCTION — HOT PATH

| Property | Value |
|----------|-------|
| Owner | `http/routes.zig`, `http/response.zig` |
| Pattern | Caller-owned allocator via route-policy FixedBufferAllocator |
| Status | **HARDENED** |
| Test | `http/status_response_budget_tests.zig` |

Response allocations use bounded arena lifetime. String duplications for JSON field values use `allocator.dupe()` with paired `allocator.free()` in errdefer.

### 1.2 Capacity-Aware OwnedResponse

**Classification**: PRODUCTION — HOT PATH

| Property | Value |
|----------|-------|
| Owner | `http/response.zig` |
| Pattern | OwnedResponse struct with explicit capacity tracking |
| Status | **HARDENED** |
| Test | `http/status_response_budget_tests.zig` |

### 1.3 Canonical HTTP 500 Failure Path

**Classification**: PRODUCTION — HOT PATH

| Property | Value |
|----------|-------|
| Owner | `http/routes.zig` |
| Pattern | `routes.zig` returns HTTP 500 with error JSON on failure |
| Status | **HARDENED** |
| Test | `http/routes_tests.zig` |

Returns HTTP 500 with error JSON; caller must not leak. No panic on allocation failure.

### 1.4 CLI/Process Boundary

**Classification**: PRODUCTION — STARTUP

| Property | Value |
|----------|-------|
| Owner | `main.zig`, `cli.zig` |
| Pattern | `init.arena.allocator()` — single arena for entire process |
| Status | **HARDENED** |
| Test | `cli/commands_test_helpers.zig` |

`main.zig` uses pre-allocated 1024-byte stdout/stderr buffers. Argument parsing uses `toSlice(arena)` for owned copy of args.

### 1.5 Linux sysfs/procfs Boundary (HULK16)

**Classification**: PRODUCTION — PERIODIC

| Property | Value |
|----------|-------|
| Owner | `net/linux_read.zig` |
| Pattern | Canonical boundary with path validation |
| Status | **HARDENED (HULK16)** |
| Test | `net/linux_read_fixture_tests.zig` |

Boundary provides:
- Path validation (only allowed roots: `/sys/class/net`, `/proc/self`, `/tmp/kgb_fixture`)
- Max-byte caps to prevent unbounded reads
- Structured error handling (no panics on malformed input)

### 1.6 BGP/BFD Bounded Snapshot Contracts

**Classification**: PRODUCTION — PERIODIC

| Property | Value |
|----------|-------|
| Owner | `bgp/snapshot.zig`, `bfd/snapshot.zig` |
| Pattern | Caller-provided allocator; bounded by protocol constants |
| Status | **HARDENED** |
| Test | `bgp/snapshot_tests.zig`, `bfd/snapshot_tests.zig` |

BGP path limit and BFD session count bound allocations.

### 1.7 BGP Production FSM Enum and u32 ASN Support

**Classification**: PRODUCTION — RUNTIME

| Property | Value |
|----------|-------|
| Owner | `bgp/session.zig`, `bgp/fsm.zig` |
| Pattern | Explicit FSM states; u32 ASN with proper serialization |
| Status | **HARDENED** |
| Test | `status_bgp_fsm_tests.zig`, `status_bgp_state_tests.zig` |

### 1.8 Allocation Register and RISKY-HIGH Gate (HULK18)

**Classification**: ENFORCEMENT

| Property | Value |
|----------|-------|
| Owner | `docs/architecture/tovarisch-allocation-register.md` |
| Pattern | Inventory of all heap allocation surfaces |
| Status | **ENFORCING** |
| Gate | `scripts/check_allocation_patterns.sh` |

RISKY-HIGH patterns always fail gate (exit 1). DEFERRED patterns are report-only (exit 0).

---

## 2. Deferred Legacy Surfaces

**Current count**: 28 DEFERRED patterns (report-only; gate passes)

These are known legacy surfaces documented in the allocation register. Gate passes with DEFERRED present (report-only semantics).

### 2.1 DEFERRED Buckets

| File Pattern | Rationale | Count |
|--------------|-----------|-------|
| `*/bgp/serve_integration.zig` | page_allocator for one-time config parse at serve init | 1 |
| `*/bfd/transport.zig:create(TransportContext)` | Test helper; never called in production | 1 |
| `*/cli/*` | page_allocator for one-shot CLI operations | 16 |
| `*/runtime/telemetry.zig` | page_allocator for bounded telemetry collection | 2 |
| `*/wg/peer.zig` | page_allocator only in test functions with proper cleanup | 5 |
| `*/net/linux_read.zig:realloc` | Bounded file reads with max_bytes cap | 2 |
| `*/net/iptables.zig` | page_allocator for one-shot command execution | 1 |

**Total**: 28 (matches script output)

### 2.2 Deferred Surface Details

**BGP serve one-time config parse**: `bgp/serve_integration.zig` uses `page_allocator` for one-time serve initialization. Acceptable because it occurs only at serve startup, not on hot path.

**CLI one-shot operations**: `cli/*` files use `page_allocator` for CLI commands that exit immediately after completion.

**Telemetry page_allocator compatibility**: `runtime/telemetry.zig` uses `page_allocator` for bounded telemetry collection. Acceptable for diagnostic commands.

**iptables one-shot command path**: `net/iptables.zig` uses `page_allocator` for one-shot command execution.

**Test-only deferred cases**: `bfd/transport.zig:create(TransportContext)` and `wg/peer.zig` patterns only occur in test functions with proper cleanup.

---

## 3. Verification Transcript

### 3.1 make tovarisch-build

```
cd tovarisch && zig build
```

Exit code: 0

### 3.2 make tovarisch-test

```
cd tovarisch && zig build test
```

Exit code: 0

### 3.3 make tovarisch-status

```
cd tovarisch && zig build run -- status --json
{"service":"tovarisch","version":"0.1.2+b370d72","node_id":"local-dev","status":"warn","checks":[...],"runtime":{"pid":94886,"rss_kib":null}}
```

Exit code: 0

### 3.4 scripts/check_allocation_patterns.sh

```
=== Tovarisch Risky Allocation Pattern Report ===

Scan root: tovarisch/src
Date: 2026-07-05T04:20:03Z

Files scanned: 236

Risky patterns found:
  HIGH:   0
  MEDIUM: 30
  LOW:    4
  DEFERRED: 28

Accepted patterns: 115
Exempt patterns: 49

[REPORT] DEFERRED patterns found. Report-only; gate passes.
```

Exit code: 0

### 3.5 scripts/check_allocation_patterns.sh --self-test

```
=== Self-Test Mode ===

[PASS] Clean tree passes (exit 0)
[PASS] Synthetic HIGH (unregistered page_allocator) fails with exit 1 (exit 1)
[PASS] Accepted registered pattern passes (exit 0) (exit 0)
[PASS] Synthetic DEFERRED pattern passes (exit 0, report-only) (exit 0)
[PASS] RISKY-MEDIUM with --enforce-medium fails with exit 2 (exit 2)

=== Self-Test Summary ===
Passed: 5
Failed: 0

[SELF-TEST PASSED] All 5 test(s) passed
```

Exit code: 0

### 3.6 python scripts/verify_linux_read_boundary_bypass.py

```
============================================================
Linux sysfs/procfs boundary verification
============================================================
Checked: 236 .zig files in tovarisch/src
Legacy files documented: 0
Violations found: 0

RESULT: PASS - No unauthorized sysfs/procfs reads found
All legacy files have been migrated to linux_read.zig boundary

Boundary helper: tovarisch/src/net/linux_read.zig
Canonical API: linuxRead(allocator, path, root, config) -> LinuxReadResult
```

Exit code: 0

### 3.7 make gate

```
[gate] checking LLM-friendliness
...
[gate] checking memory allocation ownership hygiene
=== Memory Ownership Hygiene Gate ===

Scanned 11 source files.
[PASS] Memory ownership hygiene gate passed.

[gate] PASS
```

Exit code: 0

---

## 4. Acceptance Criteria

- [x] Final close report exists
- [x] Report lists hardened boundaries
- [x] Report lists deferred buckets
- [x] Report includes verification transcript
- [x] make gate passes
- [x] No new code added (documentation only)

---

## 5. Board Status

**Tovarisch Hulk-Zig epic: CLOSED**

| HULK Iteration | Focus | Status |
|----------------|-------|--------|
| HULK16 | Linux sysfs/procfs boundary migration | CLOSED |
| HULK18 | RISKY-HIGH gate enforcement | CLOSED |
| HULK19 | Final close report | CLOSED |

---

## 6. See Also

- `docs/architecture/tovarisch-allocation-register.md` — Complete allocation inventory
- `docs/doctrine/embedded-memory-frugality.md` — Memory footprint doctrine
- `scripts/check_allocation_patterns.sh` — Risky pattern gate
- `scripts/check_memory_ownership.sh` — Memory ownership hygiene gate
- `scripts/verify_linux_read_boundary_bypass.py` — Linux read boundary verifier
- `docs/tooling/zig-0.16-field-manual.md` — Zig 0.16 patterns
