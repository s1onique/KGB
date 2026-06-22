# Embedded Memory Frugality and Leak Discipline — KGB Doctrine

**URI**: `kgb://doctrine/embedded-memory-frugality`

Memory footprint and allocation behavior are explicit product contracts for KGB embedded daemons. This doctrine establishes the memory discipline framework for both Go (UVB-76) and Zig (tovarisch) components.

---

## 1. Memory Footprint as Product Contract

Memory is not an implementation detail — it is a shipped contract.

| Property | Commitment |
|----------|------------|
| Idle RSS/PSS | Bounded, measured at startup |
| Warm steady-state RSS/PSS | Bounded after warmup |
| Hot-path allocation count | Budgeted per request/probe cycle |
| History cap | Explicit retention limit |
| Max response/artifact size | Bounded, non-negotiable |

**Forbidden**: unbounded growth, unbounded history, unbounded allocation in hot paths.

---

## 2. Allocation Ownership Doctrine

### General Rules

1. **No unowned allocations** — every heap allocation must have a clear owner
2. **No unbounded growth** — arrays, maps, and arenas must have explicit capacity limits
3. **Hot-path budgets** — hot paths (status, probe, diagnostics) have per-call allocation budgets
4. **Ownership transfer** — ownership must be explicit when transferring memory between components

### Go-Specific Rules

- Prefer `sync.Pool` for amortized allocations in hot paths
- Use `runtime.ReadMemStats` for heap profiling
- Bounded channels and slices with explicit capacity
- No unbounded goroutine spawning

### Zig-Specific Rules

- `std.testing.allocator` for test allocation tracking
- Local deinit/free for all heap allocations in non-test code
- Arena lifetime must be explicitly bounded
- Patterns requiring ownership transfer must document the transfer

---

## 3. Suspicious Allocation Patterns

The following patterns require explicit ownership coverage:

| Pattern | Risk | Required Coverage |
|---------|------|-------------------|
| `std.heap.page_allocator` | RSS leak per call | Deinit not possible; avoid in hot paths |
| `std.fmt.allocPrint` | Unbounded heap allocation | Caller-owned buffer or arena lifetime |
| `ArenaAllocator.init` | Unbounded growth potential | Bounded arena lifetime + explicit deinit |
| `.toOwnedSlice()` | Ownership transfer | Document transfer or local free |
| `.dupe()` | Heap allocation | Document ownership or bounded lifetime |
| `ArrayList.init()` | Dynamic growth | Explicit capacity or arena |
| `alloc`, `allocPrint` | Heap allocation | Bounded lifetime or deinit |

### Ownership Coverage Options

Each suspicious allocation must be covered by ONE of:

1. **Local deinit/free** — allocation freed in same scope
2. **Ownership-transfer annotation** — `MemoryOwnership:` comment explaining transfer
3. **Bounded arena lifetime** — arena has explicit deinit within process lifetime
4. **Inventory exception** — documented and approved exception with rationale

### MemoryOwnership Annotation Format

```zig
// MemoryOwnership: [brief ownership description]
// Rationale: [why this allocation is safe for the specific use case]
// Transfer: [if ownership is transferred, document the recipient]
```

**Forbidden justifications**: "bounded by requests", "per emit cycle", "leaked but acceptable", "daemon-lifetime".

---

## 4. Hermetic Memory Labs

Memory labs are controlled experiments that measure RSS/PSS behavior under specific workloads.

### Lab Artifact Schema

Every memory lab produces an artifact with this structure:

```yaml
version: "1.0"
service:
  name: string          # e.g., "tovarisch" or "uvb76"
  version: string        # e.g., "1.2.3+abc1234"
  commit: string         # git commit hash or "unknown"
environment:
  arch: string           # e.g., "linux/arm64", "darwin/amd64"
  kernel: string         # e.g., "5.15.0-generic"
workload:
  type: string           # e.g., "status-json-warmup", "icmp-probe-loop"
  operations: integer   # total operations performed
  errors: integer       # total errors encountered
  duration_ms: integer  # total wall-clock time
  interval_ms: integer  # operation interval
memory:
  first:
    rss_kib: integer    # RSS at start
    pss_kib: integer    # PSS at start (when available)
  max:
    rss_kib: integer    # RSS peak
    pss_kib: integer    # PSS peak
  last:
    rss_kib: integer    # RSS at end
    pss_kib: integer    # PSS at end
  growth:
    rss_kib: integer    # last - first RSS
    pss_kib: integer    # last - first PSS
    rss_percent: float  # growth percentage
runtime:
  goroutines: integer   # peak goroutine count (Go only)
  gc_count: integer     # GC cycles observed (Go only)
  heap_alloc_bytes: integer  # peak heap alloc (Go only)
  gc_pause_ns: integer # peak GC pause (Go only)
decision:
  pass: boolean         # true if within budget
  reason: string        # explanation of pass/fail
```

### Tovarisch Memory Labs

**Idle Warmup Lab**
- Workload: Start tovarisch, wait 60s, capture RSS/PSS
- Purpose: Establish baseline idle memory footprint

**Status JSON Warmup Lab**
- Workload: Repeated `/status`, `/status.json`, `/status.json?include=network_diag`
- Purpose: Prove no RSS growth under repeated status rendering

**BGP/BFD Steady-State Lab**
- Workload: Continuous BGP/BFD status with real peers
- Purpose: Prove no memory growth under protocol steady-state

### UVB-76 Memory Labs

**Idle Warmup Lab**
- Workload: Start uvb76, wait 120s, capture RSS/PSS
- Purpose: Establish baseline idle memory footprint

**Status API Polling Lab**
- Workload: Repeated `/api/v1/status` polling
- Purpose: Prove no RSS growth under API polling

**ICMP Probe Loop Lab**
- Workload: Continuous ICMP probe execution
- Purpose: Prove no RSS growth under probe workload

**Diagnostic Capture Loop Lab**
- Workload: Continuous diagnostic capture with network_diag
- Purpose: Prove no RSS growth under diagnostic workload

---

## 5. Automated Memory Optimization Loop

When a memory lab detects growth:

```
┌─────────────────────────────────────────────────────────────────┐
│  1. DETECT GROWTH                                                │
│     └─ Memory lab reports RSS growth > threshold                 │
├─────────────────────────────────────────────────────────────────┤
│  2. CLASSIFY SIGNAL                                              │
│     └─ Is it a leak? (linear growth over time)                   │
│     └─ Is it expected? (warmup curve flattening)                │
│     └─ Is it a spike? (single elevated reading)                 │
├─────────────────────────────────────────────────────────────────┤
│  3. CAPTURE PROFILE/EVIDENCE                                     │
│     └─ Go: pprof heap profile                                    │
│     └─ Zig: allocation site analysis                            │
│     └─ Record: artifact, timeline, workload                       │
├─────────────────────────────────────────────────────────────────┤
│  4. IDENTIFY ALLOCATION OWNER                                    │
│     └─ Trace allocation to source location                       │
│     └─ Determine ownership boundary                              │
│     └─ Check for missing deinit/free                             │
├─────────────────────────────────────────────────────────────────┤
│  5. PATCH                                                        │
│     └─ Add MemoryOwnership annotation if transfer is safe        │
│     └─ Add explicit deinit/free if local                         │
│     └─ Bound arena lifetime if arena growth                      │
│     └─ Document exception if legitimately unbounded              │
├─────────────────────────────────────────────────────────────────┤
│  6. RERUN LAB                                                    │
│     └─ Execute same workload                                      │
│     └─ Capture new artifact                                       │
├─────────────────────────────────────────────────────────────────┤
│  7. COMPARE ARTIFACTS                                             │
│     └─ Compare: first/max/last RSS, growth, peak heap            │
│     └─ Verify: growth is eliminated or reduced                  │
├─────────────────────────────────────────────────────────────────┤
│  8. TIGHTEN BUDGET                                                │
│     └─ Update memory budget YAML with new measured baseline       │
│     └─ Document: change rationale, evidence artifact             │
└─────────────────────────────────────────────────────────────────┘
```

---

## 6. Release and Deployment Memory Evidence

Before releasing embedded deployments:

1. **Run memory labs** for each target platform
2. **Capture artifacts** — upload lab artifacts to release artifacts
3. **Verify budgets** — compare RSS/PSS against budget YAML
4. **Document baseline** — record measured values in release notes

**Forbidden**: releasing embedded images without memory evidence.

---

## 7. Gate Wiring

### Fast Local Gate (deterministic, fast)

| Check | Tool | Fail on |
|-------|------|---------|
| Doctrine exists | `quality_gate.sh` | Missing `embedded-memory-frugality.md` |
| Budget files valid | `verify_memory_budgets.py` | Invalid YAML or schema |
| Zig allocation ownership | `check_memory_ownership.sh` | Uncovered suspicious pattern |
| Zig allocator leak tests | `zig build test` | `std.testing.allocator` failures |
| Go hot-path budgets | `verify_go_memory_budgets.py` | Unbudgeted allocation regression |

### Manual/Scheduled Gate (slow, comprehensive)

| Check | Tool | Frequency |
|-------|------|-----------|
| Hermetic memory labs | `lab-*-memory.sh` | On release |
| Long soaks | 24h+ soak tests | Monthly |
| Profile capture | pprof/heapanalyzer | On growth detection |
| Artifact comparison | `compare_memory_artifacts.py` | On each lab run |

---

## 8. Memory Budget Files

Repository-controlled memory budgets live in `docs/memory/budgets/`:

- `tovarisch-memory-budget.yaml` — tovarisch RSS/PSS budgets
- `uvb76-memory-budget.yaml` — uvb76 RSS/PSS budgets

Budget values marked `baseline_required` must be measured before being set.

---

## 9. See Also

- `kgb://doctrine/tiny-leafs` — Leaf node constraints
- `kgb://doctrine/ai-native-code-discipline-axioms` — AXIOM-3 (production-path parity)
- `kgb://doctrine/runtime-harness-adaptation` — Runtime ownership patterns
- `scripts/check_memory_ownership.sh` — Memory ownership hygiene gate
- `scripts/verify_memory_budgets.py` — Budget schema verifier
- `docs/memory/budgets/` — Memory budget YAML files
- `docs/labs/` — Memory lab artifacts and documentation
