# [Closed] Epic: Tovarisch Hulk-Zig / FP-Hardening Close Report

**Epic ID**: ACT-TOVARISCH-ZIG-HULK25
**Status**: CLOSED
**Closed**: 2026-07-07
**Commit**: `fff8c30c9a3241ab4ba42067a0f5b1508d390a1b`

---

## Executive Close Statement

The Tovarisch Hulk-Zig / FP-hardening epic is **CLOSED**. All four foundational pillars have been established, hardened, and verified:

| Pillar | Status |
|--------|--------|
| Rust-like ownership/allocation discipline | ✅ CLOSED |
| Effect-boundary / functional-core discipline | ✅ CLOSED |
| Total parser / no-panic external input discipline | ✅ CLOSED |
| Protocol state-transition totality | ✅ CLOSED |

This maps to Zig's explicitness model (no hidden control flow, no hidden memory allocations) and FP/Haskell-like discipline (referential transparency, pure-ish core logic, explicit effects, total parsers, total transitions).

---

## Final Board Table (HULK01–HULK25)

| HULK | Focus | Register | Gate | Status |
|------|-------|----------|------|--------|
| HULK01 | CLI/process boundary | — | — | CLOSED |
| HULK02 | /status.json budget | — | — | CLOSED |
| HULK03 | BGP snapshot contracts | — | — | CLOSED |
| HULK04 | BFD snapshot contracts | — | — | CLOSED |
| HULK05 | Linux sysfs/procfs boundary | — | — | CLOSED |
| HULK06 | HTTP response budget | — | — | CLOSED |
| HULK07 | WireGuard allocation hygiene | — | — | CLOSED |
| HULK08 | BGP FSM enum | — | — | CLOSED |
| HULK09 | BFD FSM enum | — | — | CLOSED |
| HULK10 | Network diagnostics budget | — | — | CLOSED |
| HULK11 | Safe command pattern | — | — | CLOSED |
| HULK12 | CLI argument parsing | — | — | CLOSED |
| HULK13 | Heartbeat emission | — | — | CLOSED |
| HULK14 | Memory ownership hygiene | — | — | CLOSED |
| HULK15 | Allocation audit pass | — | — | CLOSED |
| HULK16 | Linux sysfs/procfs migration | — | — | CLOSED |
| HULK17 | BFD FSM completeness | — | — | CLOSED |
| HULK18 | RISKY-HIGH gate | `tovarisch-allocation-register.md` | `check_allocation_patterns.sh` | CLOSED |
| HULK19 | Memory epic close | — | — | CLOSED |
| HULK20 | Effect boundary register | `tovarisch-effect-boundary-register.md` | — | CLOSED |
| HULK21 | Effect boundary verifier | — | `verify_effect_boundaries.py` | CLOSED |
| HULK22 | Total parser coverage | `tovarisch-total-parser-register.md` | `verify_total_parsers.py` | CLOSED |
| HULK23 | Second-ring parser coverage | — | `verify_total_parsers.py` | CLOSED |
| HULK24 | State-transition totality | `tovarisch-state-transition-register.md` | — | CLOSED |
| HULK25 | **Final close report** | — | `make gate` | **CLOSED** |

---

## Enforced Gates

| Gate | Command | Exit on Violation |
|------|---------|-------------------|
| Quality gate | `make gate` | Non-zero |
| Allocation patterns | `scripts/check_allocation_patterns.sh` | RISKY-HIGH: 1, RISKY-MEDIUM with `--enforce-medium`: 2 |
| Effect boundary | `scripts/verify_effect_boundaries.py` | 1 (if PURE module uses IO) |
| Total parser | `scripts/verify_total_parsers.py` | 1 (if total parser uses panic/unreachable) |
| Linux read boundary | `scripts/verify_linux_read_boundary_bypass.py` | 1 (if sysfs/procfs read outside boundary) |
| Memory ownership hygiene | `scripts/check_memory_ownership.sh` | Non-zero |
| Zig build | `make tovarisch-build` | Non-zero |
| Zig test | `make tovarisch-test` | Non-zero |

---

## Registers Created

| Register | File | DEFERRED |
|----------|------|----------|
| Allocation register | `docs/architecture/tovarisch-allocation-register.md` | 28 |
| Effect boundary register | `docs/architecture/tovarisch-effect-boundary-register.md` | 75 (report-only) |
| Total parser register | `docs/architecture/tovarisch-total-parser-register.md` | 0 |
| State transition register | `docs/architecture/tovarisch-state-transition-register.md` | 0 |

---

## Remaining WARN/Report-Only Items

### Allocation DEFERRED (28 total — report-only, gate passes)

| Bucket | Count | Rationale |
|--------|-------|-----------|
| `cli/*` | 16 | One-shot CLI operations; process exits immediately after |
| `wg/peer.zig` | 5 | Test-only functions with proper cleanup |
| `net/linux_read.zig:realloc` | 2 | Bounded file reads with max_bytes cap |
| `bgp/serve_integration.zig` | 1 | One-time config parse at serve init |
| `bfd/transport.zig:create` | 1 | Test helper; never called in production |
| `net/iptables.zig` | 1 | One-shot command execution |
| `runtime/telemetry.zig` | 2 | Bounded telemetry collection for diagnostics |

### Effect Boundary DEFERRED/UNKNOWN (75 total — report-only)

All 75 UNKNOWN modules are report-only in the current gate. These represent CLI commands, runtime telemetry, and protocol-specific modules that are expected to use IO. The PURE modules have been verified.

### RISKY-MEDIUM (29 total — accepted patterns)

These are accepted patterns with documented ownership and bounded allocations. Examples include:
- `allocator.dupe()` for string ownership transfer with paired `allocator.free()` in errdefer
- `allocator.alloc()` for bounded buffer operations
- `ArrayListUnmanaged` for zero-initialization patterns

### RISKY-LOW (4 total — accepted patterns)

Zero-initialization `ArrayListUnmanaged` fields in fake/test implementations.

---

## DEFERRED Counts

| Category | DEFERRED | Notes |
|----------|----------|-------|
| Allocation patterns | 28 | Report-only; gate passes |
| Total parser | 0 | **All parsers total** |
| State transition | 0 | **All transitions documented** |

---

## Final State Summary

| Property | Value |
|----------|-------|
| Memory ownership hardening | **CLOSED** |
| Effect-boundary hardening | **CLOSED** |
| Total parser hardening | **CLOSED** |
| Second-ring parser coverage | **CLOSED** |
| Transition totality | **CLOSED** |
| Total parser DEFERRED | **0** |
| State transition DEFERRED | **0** |
| Allocation RISKY-HIGH | **0** |

---

## Verification Transcript

### python3 scripts/verify_effect_boundaries.py

```
[verifier] Scanning /Volumes/UserData/Users/chistyakov/Projects/SPbNIX/KGB/tovarisch/src...

[INFO] DEFERRED/UNKNOWN modules (report only):
  ... (75 modules listed) ...

[PASS] Effect boundary verification passed
```

Exit code: 0

### python3 scripts/verify_effect_boundaries.py --self-test

```
[self-test] Running effect boundary verifier self-test...
[self-test] Test 1: Clean PURE module should pass... [PASS]
[self-test] Test 2: PURE module with std.fs.cwd() should fail... [PASS]
[self-test] Test 3: PURE module with std.process should fail... [PASS]
[self-test] Test 4: BOUNDARY module with std.fs.cwd() should pass... [PASS]
[self-test] Test 5: Production import of *_tests.zig should fail... [PASS]
[self-test] Test 6: Comments with forbidden pattern should not trigger... [PASS]

[self-test] All self-tests passed!
```

Exit code: 0

### python3 scripts/verify_total_parsers.py

```
Verifying total parser discipline in tovarisch...
Source root: tovarisch/src

============================================================
Total Parser Verification Summary
============================================================
Modules scanned: 25
Total lines: 6406
Findings: 52 (0 failures, 12 warnings)

By classification:
  TOTAL: 11 ok
  BOUNDARY_TOTAL: 11 ok
  STATEFUL_ADAPTER: 3 ok

Verifier code lines: 2110 across 9 files

[OK] All modules pass total parser verification
```

Exit code: 0

### python3 scripts/verify_total_parsers.py --self-test

```
Running total parser verifier self-tests...
Test cases: 8

[OK] All self-tests passed
```

Exit code: 0

### bash scripts/check_allocation_patterns.sh

```
=== Tovarisch Risky Allocation Pattern Report ===

Files scanned: 240

Risky patterns found:
  HIGH:   0
  MEDIUM: 29
  LOW:    4
  DEFERRED: 28

Accepted patterns: 116
Exempt patterns: 49

[REPORT] DEFERRED patterns found. Report-only; gate passes.
```

Exit code: 0

### bash scripts/check_allocation_patterns.sh --self-test

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

### python3 scripts/verify_linux_read_boundary_bypass.py

```
============================================================
Linux sysfs/procfs boundary verification
============================================================
Checked: 240 .zig files in tovarisch/src
Legacy files documented: 0
Violations found: 0

RESULT: PASS - No unauthorized sysfs/procfs reads found
All legacy files have been migrated to linux_read.zig boundary

Boundary helper: tovarisch/src/net/linux_read.zig
Canonical API: linuxRead(allocator, path, root, config) -> LinuxReadResult
```

Exit code: 0

### make tovarisch-build

```
cd tovarisch && zig build
```

Exit code: 0

### make tovarisch-test

```
cd tovarisch && zig build test

test
+- run test w
[BFD] bfd_control_packet_sent to=10.149.149.10
[BFD] bfd_control_packet_sent to=10.149.149.10
[BFD] bfd_control_packet_sent to=10.0.0.2
[BFD] bfd_control_packet_sent to=10.0.0.2
```

Exit code: 0

### make tovarisch-status

```
cd tovarisch && zig build run -- status --json
{"service":"tovarisch","version":"0.1.2+fff8c30","node_id":"local-dev","status":"warn",...}
```

Exit code: 0

### make gate

```
[gate] checking LLM-friendliness
... (warnings only, not failures)

[gate] checking memory allocation ownership hygiene
=== Memory Ownership Hygiene Gate ===

Scanned 11 source files.
[PASS] Memory ownership hygiene gate passed.

[gate] PASS
```

Exit code: 0

---

## Known Non-Goals

The following are explicitly **NOT** part of this epic:

- **Kubernetes support**: Leaf nodes must NOT include Kubernetes assumptions
- **Container runtime**: Containers by default are forbidden for leaf nodes
- **Embedded TSDB**: Unbounded time-series storage is forbidden
- **Full observability stack**: Leafs must be constrained, not comprehensive
- **Modern web UI**: No web UI requirement for tovarisch
- **Unbounded memory/disk growth**: Hard budget constraints enforced
- **User-behavior monitoring**: KGB observes infrastructure health, not people
- **RISKY-MEDIUM enforcement**: Currently accepted (report-only); could be tightened in future

---

## Doctrine for Future Code

### Adding New Allocation Surfaces

1. Register the surface in `docs/architecture/tovarisch-allocation-register.md`
2. Use one of the accepted patterns: `allocator.dupe()` + errdefer, bounded arena, page_allocator for one-shot CLI
3. If using `allocator.alloc()`:
   - Pair with `allocator.free()` in errdefer
   - Ensure bounded by protocol constants or config
   - Add test coverage
4. Run `scripts/check_allocation_patterns.sh` before committing

### Adding New Parsers

1. Register the parser in `docs/architecture/tovarisch-total-parser-register.md`
2. Use explicit error returns, not `@panic`, `unreachable`, or `.?` unwrap
3. Handle all input cases explicitly
4. Run `scripts/verify_total_parsers.py` before committing

### Adding New Protocol State Transitions

1. Document the transition in `docs/architecture/tovarisch-state-transition-register.md`
2. Ensure exhaustive switch coverage
3. Add structured failure handling
4. Run `make tovarisch-test` to verify

### Effect Boundary Discipline

1. Keep core logic PURE when possible
2. Move IO to BOUNDARY modules
3. Do not use PURE classification for modules with IO dependencies
4. Run `scripts/verify_effect_boundaries.py` before committing

---

## Acceptance Criteria

- [x] Final close report exists
- [x] Board table lists HULK01–HULK25
- [x] All gate names and outputs listed
- [x] All remaining WARN/report-only items explicitly bucketed
- [x] Total parser DEFERRED count: 0
- [x] State transition DEFERRED count: 0
- [x] Allocation RISKY-HIGH count: 0
- [x] `make gate` passes
- [x] Close report is staged

---

## Board Status

**Tovarisch Hulk-Zig / FP-hardening epic: CLOSED**

After HULK25, the project returns to product work. All four foundational pillars are established and enforced via gates.

---

## See Also

- `docs/architecture/tovarisch-allocation-register.md` — Complete allocation inventory
- `docs/architecture/tovarisch-effect-boundary-register.md` — Effect boundary classification
- `docs/architecture/tovarisch-total-parser-register.md` — Total parser inventory
- `docs/architecture/tovarisch-state-transition-register.md` — Protocol transition table
- `docs/doctrine/embedded-memory-frugality.md` — Memory footprint doctrine
- `docs/doctrine/native-owned-critical-paths.md` — Native code preference
- `docs/tooling/zig-0.16-field-manual.md` — Zig 0.16 patterns
- `scripts/check_allocation_patterns.sh` — Allocation gate
- `scripts/verify_effect_boundaries.py` — Effect boundary verifier
- `scripts/verify_total_parsers.py` — Total parser verifier
- `scripts/verify_linux_read_boundary_bypass.py` — Linux read boundary verifier
