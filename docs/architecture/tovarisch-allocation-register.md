# Tovarisch Allocation Register

**URI**: `kgb://architecture/tovarisch-allocation-register`

Canonical inventory of heap allocation surfaces in production tovarisch code.
Each class documents owner, allocator source, bounds, deinit path, failure mode,
external-input influence, and test coverage.

---

## 1. Allocation Classes

### 1.1 Request/Response Allocations

**Classification**: PRODUCTION — HOT PATH

| Property | Value |
|----------|-------|
| Owner | `http/routes.zig`, `http/response.zig` |
| Allocator Source | Request-scoped allocator; currently backed by route-policy FixedBufferAllocator in `http/routes.zig`; no global allocator. |
| Maximum Size | Bounded by HTTP response budget (see `http/status_response_budget_tests.zig`) |
| Deinit/Free Path | `response.deinit()` called after JSON serialization completes |
| Failure Mode | Returns HTTP 500 with error JSON; caller must not leak |
| External-Input Influence | Bounded by protocol constants; response budget enforced |
| Test Coverage | `http/routes_tests.zig`, `http/status_response_budget_tests.zig` |

**Notes**: Response allocations use bounded arena lifetime. String duplications for
JSON field values use `allocator.dupe()` with paired `allocator.free()` in errdefer.

### 1.2 CLI/Process Boundary Allocations

**Classification**: PRODUCTION — STARTUP

| Property | Value |
|----------|-------|
| Owner | `main.zig`, `cli.zig` |
| Allocator Source | `init.arena.allocator()` — single arena for entire process |
| Maximum Size | Arena grows within OS process limits; bounded by init args |
| Deinit/Free Path | OS releases arena on process exit |
| Failure Mode | `std.process.Init` returns error; process exits non-zero |
| External-Input Influence | CLI args length bounded by OS limits |
| Test Coverage | `cli/commands_test_helpers.zig` |

**Notes**: `main.zig` uses pre-allocated 1024-byte stdout/stderr buffers.
Argument parsing uses `toSlice(arena)` for owned copy of args.

### 1.3 sysfs/procfs Read Allocations (HARDENED)

**Classification**: PRODUCTION — PERIODIC

| Property | Value |
|----------|-------|
| Owner | `net/linux_read.zig` boundary, consumed by `net/linux_stats.zig`, `net/extended_interface_stats.zig`, `runtime/telemetry.zig`, `tunnel_check.zig` |
| Allocator Source | Caller-provided; via canonical `linux_read.zig` boundary |
| Maximum Size | Bounded by `/sys/class/net` enumeration and `/proc/self/status` (max 8KB for status) |
| Deinit/Free Path | `allocator.free()` on LinuxReadResult.value; paired in callers |
| Failure Mode | Structured `LinuxReadResult` variants (missing, permission_denied, too_large, malformed, io_error) |
| External-Input Influence | Kernel interface list (bounded by system NIC count) |
| Test Coverage | `net/linux_read_fixture_tests.zig` |

**Notes (HULK16)**: All production Linux runtime-file reads now go through the canonical
`linux_read.zig` boundary. This includes:
- `/proc/self/status` via `telemetry.zig`
- Sysfs counters via `linux_stats.zig` and `extended_interface_stats.zig`
- Interface enumeration via `linux_interfaces.zig` and `tunnel_check.zig`

The boundary provides:
- Path validation (only allowed roots: `/sys/class/net`, `/proc/self`, `/tmp/kgb_fixture`)
- Max-byte caps to prevent unbounded reads
- Structured error handling (no panics on malformed input)

### 1.4 BGP/BFD Snapshot Allocations

**Classification**: PRODUCTION — PERIODIC

| Property | Value |
|----------|-------|
| Owner | `bgp/snapshot.zig`, `bfd/snapshot.zig` |
| Allocator Source | Caller-provided `std.mem.Allocator` |
| Maximum Size | Bounded by protocol constants (BGP path limit, BFD session count) |
| Deinit/Free Path | Caller owns snapshots; `allocator.free()` after use |
| Failure Mode | Returns error; session state machine logs and continues |
| External-Input Influence | Peer count bounded by config; path count bounded by protocol |
| Test Coverage | `bgp/snapshot_tests.zig`, `bfd/snapshot_tests.zig` |

### 1.5 Long-Lived Runtime State

**Classification**: PRODUCTION — PROCESS LIFETIME

| Property | Value |
|----------|-------|
| Owner | `bgp/session.zig`, `bfd/session.zig`, `bgp/runtime.zig`, `bfd/runtime.zig` |
| Allocator Source | Initialized at session start; caller-provided |
| Maximum Size | Bounded by peer count from config |
| Deinit/Free Path | `runtime.deinit()` on serve shutdown; explicit destroy calls |
| Failure Mode | Serve returns error; clean shutdown |
| External-Input Influence | Config-derived peer count |
| Test Coverage | `bgp/session_tests.zig`, `bfd/session_tests.zig`, `bgp/runtime_tests.zig` |

### 1.6 Parser Scratch Allocations

**Classification**: PRODUCTION — PARSING

| Property | Value |
|----------|-------|
| Owner | `config.zig`, `bgp/config_parse.zig`, `bfd/config_parse.zig`, `wg/config.zig` |
| Allocator Source | Caller-provided; `std.heap.page_allocator` in serve_integration.zig |
| Maximum Size | Bounded by config file size (limited by OS) |
| Deinit/Free Path | `raw.deinit(std.heap.page_allocator)` after config parse completes |
| Failure Mode | Parse error returned; clean shutdown |
| External-Input Influence | Config file content (size bounded by OS) |
| Test Coverage | `bgp/passive_listener_config_tests.zig`, `wg/peer_tests.zig` |

### 1.7 Test/Lab-Only Allocations

**Classification**: TEST ONLY — NOT PRODUCTION

| Property | Value |
|----------|-------|
| Owner | `*_tests.zig` files, `fixtures/` |
| Allocator Source | `std.testing.allocator`, `std.heap.page_allocator` |
| Maximum Size | Test fixture size; bounded by test harness |
| Deinit/Free Path | `defer std.testing.allocator.free()`, `defer std.heap.page_allocator.free()` |
| Failure Mode | Test fails; no production impact |
| External-Input Influence | None (test fixtures are synthetic) |
| Test Coverage | All test files |

**Notes**: Test files are EXEMPT from memory ownership gate per `check_memory_ownership.sh`.

---

## 2. New-Code Doctrine

Every production allocation must define the following properties:

### 2.1 Required Properties

```zig
// MemoryOwnership: <brief ownership description>
// Rationale: <why this allocation is safe for the specific use case>
// MaxSize: <maximum size in bytes or item count>
// Deinit: <function or mechanism that frees this allocation>
// FailureMode: <what happens on allocation failure>
// ExternalInputInfluence: <yes/no and brief description>
// TestCoverage: <test file or function that validates this allocation>
```

### 2.2 MemoryOwnership Annotation Format

```zig
// MemoryOwnership: <brief ownership description>
// Rationale: This allocation is safe because [reason].
// MaxSize: <number> bytes/items maximum
// Deinit: <deinit|free|destroy> called in <location>
// FailureMode: <error propagation, graceful degradation, etc.>
// ExternalInputInfluence: <yes/no>
// TestCoverage: <test file and function>
```

### 2.3 Forbidden Justifications

The following phrases indicate UNSAFE annotations and must be rejected:

- "leaked per emit"
- "bounded by request"
- "bounded by request count"
- "leaked but acceptable"
- "leaked but bounded"
- "daemon-lifetime"
- "per emit cycle"
- "per request"

### 2.4 Ownership Coverage Options

Each suspicious allocation must be covered by ONE of:

1. **Local deinit/free** — allocation freed in same scope via `errdefer` or `defer`
2. **Ownership-transfer annotation** — `MemoryOwnership:` comment explaining transfer
3. **Bounded arena lifetime** — arena has explicit deinit within process lifetime
4. **Inventory exception** — documented and approved exception with rationale

---

## 3. Hardened Boundaries (Current)

The following surfaces have been audited and are considered safe:

| Surface | File | Pattern | Status |
|---------|------|---------|--------|
| HTTP response | `http/response.zig` | Caller-owned allocator | HARDENED |
| Status checks | `status_checks.zig` | Render-owned/reentrant | HARDENED |
| Network diag | `status_network_diag.zig` | `ArrayList.deinit()` | HARDENED |
| TCP diag | `status_network_diag_tcp.zig` | Paired `dupe`/`free` | HARDENED |
| Config parse | `config.zig` | Caller-provided allocator | HARDENED |
| Metrics state | `metrics_state.zig` | `allocator.dupe()` with free | HARDENED |
| **Linux sysfs/procfs** | `net/linux_read.zig` | Canonical boundary with path validation | **HARDENED (HULK16)** |
| Telemetry | `runtime/telemetry.zig` | Uses linux_read boundary | **HARDENED (HULK16)** |
| Linux stats | `net/linux_stats.zig` | Uses linux_read boundary | **HARDENED (HULK16)** |
| Extended stats | `net/extended_interface_stats.zig` | Uses linux_read boundary | **HARDENED (HULK16)** |
| Tunnel check | `tunnel_check.zig` | Caller-provided allocator | **HARDENED (HULK16)** |

### 3.1 linux_read.zig Boundary Contract

```zig
// MemoryOwnership: linux_read.zig returns owned content on .value variant
// Rationale: Allocator-backed read with bounded size; caller must free on .value
// MaxSize: Configurable via ReadConfig.max_bytes (default 4KB, max 64KB)
// Deinit: allocator.free() on LinuxReadResult.value
// FailureMode: Structured LinuxReadResult variants (no panic)
// ExternalInputInfluence: Kernel files (bounded by max_bytes)
// TestCoverage: net/linux_read_fixture_tests.zig
```

---

## 4. Deferred Legacy Surfaces (Post-HULK16)

After HULK16, all deferred surfaces have been addressed:

| Surface | File | Status | Notes |
|---------|------|--------|-------|
| ~~Tunnel check~~ | ~~`tunnel_check.zig`~~ | **MIGRATED (HULK16)** | Now uses caller-provided allocator |
| ~~Telemetry~~ | ~~`runtime/telemetry.zig`~~ | **MIGRATED (HULK16)** | Now uses linux_read.zig boundary |
| ~~Linux stats~~ | ~~`net/linux_stats.zig`~~ | **MIGRATED (HULK16)** | Now uses linux_read.zig boundary |
| ~~Extended stats~~ | ~~`net/extended_interface_stats.zig`~~ | **MIGRATED (HULK16)** | Now uses linux_read.zig boundary |
| BGP serve | `bgp/serve_integration.zig` | ACCEPTABLE | page_allocator for one-time serve init |
| BGP prefix watch | `bgp/prefix_watch_fake.zig` | N/A | Test-only fixture |
| WireGuard generate | `wg/generate_tests.zig` | N/A | Test-only fixture |

### 4.1 Remaining Acceptable Surfaces

#### BGP serve_integration.zig

```zig
// MemoryOwnership: page_allocator for config parse at serve init
// Rationale: One-time allocation during serve startup; not on hot path
// MaxSize: Bounded by config file size
// Deinit: raw.deinit() after parse completes
// FailureMode: Parse error returned; clean shutdown
// ExternalInputInfluence: Config file
// TestCoverage: bgp/passive_listener_config_tests.zig
```

**Assessment**: This surface is acceptable because it occurs only at serve initialization,
not on the hot path. It is bounded by config file size and freed immediately after use.

---

## 5. Risky Pattern Report

See `scripts/check_allocation_patterns.sh` for automated verification.

### 5.1 Patterns to Detect

| Pattern | Risk Level | Required Coverage |
|---------|------------|-------------------|
| `std.heap.page_allocator` | HIGH | MemoryOwnership annotation + free |
| `std.heap.c_allocator` | HIGH | MemoryOwnership annotation + free |
| `GeneralPurposeAllocator` | MEDIUM | Test-only; forbidden in production |
| `allocator.alloc` | MEDIUM | Paired free in same scope |
| `allocator.dupe` | MEDIUM | Paired free via errdefer |
| `allocator.realloc` | HIGH | Bounded growth; paired free |
| `ArrayList.init` | MEDIUM | `.deinit()` called |
| `ArrayListUnmanaged` | LOW | Caller manages lifetime |

### 5.2 Exemptions

The following are EXEMPT from risky pattern detection:

- Files ending in `_tests.zig` (test files)
- Files in `*/fixtures/*` (test fixtures)
- Lines containing `MemoryOwnership:` annotation
- Lines containing `defer` for deallocation

---

## 6. Gate Wiring

| Check | Tool | Gate | Notes |
|-------|------|------|-------|
| Risky pattern report | `scripts/check_allocation_patterns.sh` | **ENFORCING for RISKY-HIGH; DEFERRED remains report-only** | **HULK18: Now enforcing; fails gate on RISKY-HIGH patterns; DEFERRED patterns are report-only** |
| Memory ownership hygiene | `scripts/check_memory_ownership.sh` | ENFORCING | Fails gate on uncovered patterns in status/http paths |
| Memory budgets | `scripts/verify_memory_budgets.py` | ENFORCING | Fails gate on budget schema violations |
| Linux read boundary | `scripts/verify_linux_read_boundary_bypass.py` | ENFORCING | Fails gate on direct sysfs/procfs bypass |
| Zig build/test | `make tovarisch-build`, `make tovarisch-test` | ENFORCING | Fails gate on build/test failures |

### 6.1 check_allocation_patterns.sh Enforcement Semantics (HULK18)

**Enforcement thresholds:**
- **RISKY-HIGH**: Always fails gate (exit 1)
- **DEFERRED**: Report-only (exit 0) — known legacy surfaces documented in register
- **RISKY-MEDIUM**: Report-only by default; optionally enforcing with `--enforce-medium` flag (exit 2)
- **RISKY-LOW**: Report-only (exit 0)
- **ACCEPTED/EXEMPT**: Pass (exit 0)

**Exit codes:**
- 0: Gate pass (clean, LOW, DEFERRED-only, or only ACCEPTED/EXEMPT patterns)
- 1: Gate fail — RISKY-HIGH pattern found
- 2: Gate fail — RISKY-MEDIUM pattern with `--enforce-medium` flag

**Self-test:**
- Run `scripts/check_allocation_patterns.sh --self-test` to verify synthetic failure proofs
- Verifies: clean tree passes, unregistered HIGH fails (exit 1), accepted patterns pass, DEFERRED passes (report-only)

**Documentation of current state:**
- Current tree: 0 RISKY-HIGH, 0 RISKY-MEDIUM, 0 RISKY-LOW, 28 DEFERRED
- DEFERRED patterns are documented legacy surfaces across CLI layer, telemetry, and test harnesses
- Gate passes with DEFERRED present (report-only semantics)

**DEFERRED buckets (HULK18):**
| File Pattern | Rationale |
|--------------|-----------|
| `*/bgp/serve_integration.zig` | page_allocator for one-time config parse at serve init |
| `*/bfd/transport.zig:create(TransportContext)` | Test helper; never called in production |
| `*/cli/*` | page_allocator for one-shot CLI operations |
| `*/runtime/telemetry.zig` | page_allocator for bounded telemetry collection |
| `*/wg/peer.zig` | page_allocator only in test functions with proper cleanup |
| `*/net/linux_read.zig:realloc` | Bounded file reads with max_bytes cap |
| `*/net/iptables.zig` | page_allocator for one-shot command execution |

---

## 7. See Also

- `docs/doctrine/embedded-memory-frugality.md` — Memory footprint contracts
- `scripts/check_memory_ownership.sh` — Memory ownership hygiene gate
- `scripts/check_allocation_patterns.sh` — Risky pattern reporter
- `scripts/verify_linux_read_boundary_bypass.py` — Linux read boundary verifier
- `docs/tooling/zig-0.16-field-manual.md` — Zig 0.16 patterns
