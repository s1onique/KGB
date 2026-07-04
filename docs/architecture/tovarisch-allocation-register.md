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

### 1.3 sysfs/procfs Read Allocations

**Classification**: PRODUCTION — PERIODIC

| Property | Value |
|----------|-------|
| Owner | `net/linux_interfaces.zig`, `net/interface_sampler.zig`, `tunnel_check.zig` |
| Allocator Source | Caller-provided; pattern uses `std.heap.page_allocator` in tunnel_check |
| Maximum Size | Bounded by `/sys/class/net` enumeration and `/proc/net/dev` |
| Deinit/Free Path | `linux_interfaces.freeInterfaceList()` for page_allocator; deferred for caller-owned |
| Failure Mode | Returns empty list; status check reports `.warn` |
| External-Input Influence | Kernel interface list (bounded by system NIC count) |
| Test Coverage | `net/interface_filter_tests.zig`, `net/interface_sampler_tests.zig` |

**Notes**: `tunnel_check.zig` uses `std.heap.page_allocator` with paired free.
This is a deferred legacy surface — see Section 3.

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

**Notes**: See Section 3 for deferred surfaces using page_allocator in serve_integration.

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

---

## 4. Deferred Legacy Surfaces

The following surfaces use `std.heap.page_allocator` and require future hardening:

| Surface | File | Risk | Remediation |
|---------|------|------|-------------|
| Tunnel check | `tunnel_check.zig` | RSS leak per check | Migrate to caller-provided allocator |
| BGP serve | `bgp/serve_integration.zig` | page_allocator for config parse | Acceptable for serve init (one-time) |
| BGP prefix watch | `bgp/prefix_watch_fake.zig` | Test-only | N/A — test fixture |
| WireGuard generate | `wg/generate_tests.zig` | Test-only | N/A — test fixture |

### 4.1 tunnel_check.zig Specifics

```zig
// MemoryOwnership: tunnel_check uses page_allocator for sysfs enumeration
// Rationale: One-shot check during status render; OS-backed memory released on free
// MaxSize: Bounded by kernel interface count
// Deinit: linux_interfaces.freeInterfaceList(std.heap.page_allocator, ifaces)
// FailureMode: Returns null; status check reports warn
// ExternalInputInfluence: Kernel interface list
// TestCoverage: tunnel_check tested in integration tests
```

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
| Risky pattern report | `scripts/check_allocation_patterns.sh` | ADVISORY | Report-only; does not fail gate yet; future enforcement planned |
| Memory ownership hygiene | `scripts/check_memory_ownership.sh` | ENFORCING | Fails gate on uncovered patterns in status/http paths |
| Memory budgets | `scripts/verify_memory_budgets.py` | ENFORCING | Fails gate on budget schema violations |
| Zig build/test | `make tovarisch-build`, `make tovarisch-test` | ENFORCING | Fails gate on build/test failures |

**Note**: `check_allocation_patterns.sh` is currently advisory (report-only). It classifies
patterns but exits 0 regardless of findings. After the report stabilizes, it may be upgraded
to enforce HIGH risk failures in `make gate`.

---

## 7. See Also

- `docs/doctrine/embedded-memory-frugality.md` — Memory footprint contracts
- `scripts/check_memory_ownership.sh` — Memory ownership hygiene gate
- `scripts/check_allocation_patterns.sh` — Risky pattern reporter
- `docs/tooling/zig-0.16-field-manual.md` — Zig 0.16 patterns
