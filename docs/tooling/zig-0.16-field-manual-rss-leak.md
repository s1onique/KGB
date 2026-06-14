# Zig 0.16 Request/Status Rendering and Memory Ownership

This document captures verified lessons from the page_allocator RSS leak fix in `tovarisch/src/bfd/status.zig` and related status rendering paths.

## Historical Lesson: page_allocator RSS Leak

Avoid `std.heap.page_allocator` in request/status rendering paths. A single leaked `std.fmt.allocPrint(std.heap.page_allocator, ...)` can show up as deterministic ~4 KiB RSS growth per call, because the allocation is page-backed.

### The Bug Pattern

The original defect in `tovarisch/src/bfd/status.zig` used `std.fmt.allocPrint()` with `std.heap.page_allocator` to format a small status string like `"X/Y bfd sessions up"`. The allocated string was never freed, and since `page_allocator` returns page-backed memory, each call consumed one OS page (~4 KiB).

Production verification showed:
- Before fix: each `/status` request increased RSS by about 4 KiB.
- Root cause: `tovarisch/src/bfd/status.zig::buildStatusCheck()` used `std.fmt.allocPrint()` with `std.heap.page_allocator`.
- The allocated string was never freed.

### The Fix: Caller-Owned Buffers

For small dynamic status strings, prefer caller-owned scratch buffers and `std.fmt.bufPrint()`:

```zig
var detail_buf: [64]u8 = undefined;
const detail = std.fmt.bufPrint(
    &detail_buf,
    "{d}/{d} bfd sessions up",
    .{ up_count, peer_count },
) catch "bfd partially up";
```

The returned slice points into `detail_buf`, so it must not outlive that buffer. If the rendered status object escapes the function, the buffer must be owned by the render/request context or be a documented static buffer.

### Static Buffers Are Not Thread-Safe

Module-level static buffers are not reentrant or thread-safe:

```zig
// WARNING: This function uses a module-level static buffer for the partial-BFD case.
// It is NOT reentrant and NOT thread-safe. Use buildStatusCheckInto() for safe usage.
var static_detail_buf: [64]u8 = undefined;

pub fn buildStatusCheck(snapshot: StatusSnapshot) StatusCheck {
    return buildStatusCheckInto(snapshot, &static_detail_buf);
}
```

Static buffers must be removed before threaded HTTP serving. Use `buildStatusCheckInto()` with request-owned storage instead.

### Ownership Rules

- Heap-allocating APIs must make ownership explicit in the function name, doc comment, or type shape.
- Hidden allocation inside "pure-looking" status builders is forbidden.
- Production soak tests are useful for confirming allocator fixes: repeated endpoint calls should not grow `VmRSS`, `RssAnon`, or `VmData`.

### Production Verification

```bash
pid=$(pidof tovarisch)
before=$(awk '/VmRSS|VmData|RssAnon/ {print}' /proc/$pid/status)

for i in $(seq 1 1000); do
  curl -s http://127.0.0.1:8317/status >/dev/null
done

echo "before:"
echo "$before"
echo "after:"
awk '/VmRSS|VmData|RssAnon/ {print}' /proc/$pid/status
```

Expected result: no deterministic growth in `VmRSS`, `RssAnon`, or `VmData`.

### Related Code

- `tovarisch/src/bfd/status.zig` — BFD status reporting (fixed variant with `buildStatusCheckInto()`)
- `tovarisch/src/status.zig` — Status payload rendering
- `tovarisch/src/status_tests.zig` — Regression tests for status rendering memory behavior

## Memory Ownership Hygiene Gate

KGB includes an automated gate that scans status/request rendering paths for risky
allocation patterns that can reintroduce RSS leaks.

### Gate Location

```
scripts/check_memory_ownership.sh
```

### Risky Patterns Caught

- `std.heap.page_allocator` — leaks page-backed memory per call
- `std.fmt.allocPrint` — allocation without explicit ownership
- `ArenaAllocator.init` — unbounded growth potential
- `toOwnedSlice` — ownership transfer patterns
- `.dupe(` — heap allocation patterns
- `ArrayList.init(` — dynamic collection initialization

### Unsafe Annotation Rejection

The gate also scans `MemoryOwnership` annotations for phrases that rationalize unbounded leaks. If an annotation contains any of these phrases, the file fails the gate — the annotation itself is treated as broken:

| Rejected phrase | Why |
|---|---|
| `leaked per emit` | Leak is still unbounded |
| `bounded by request` | Request rate is not bounded by the protocol |
| `bounded by request count` | Same as above |
| `daemon-lifetime` | Leaks accumulate for process lifetime |
| `per emit cycle` | Emit cycles are unbounded |
| `per request` | Request rate is unbounded |
| `leaked but acceptable` | Leaks are never acceptable in leaf paths |

Annotations must explain WHY the specific use case is safe (e.g., "all memory released before handler returns"), not justify why a leak is tolerable.

### Allowing Intentional Cases

When a risky pattern is intentional and safe, add a `MemoryOwnership` annotation
near the allocation:

```zig
// MemoryOwnership: page_allocator is intentional for transient per-request
// interface stats collection. Memory is released when request handler returns.
const allocator = std.heap.page_allocator;
```

The gate checks within ±5 lines of the pattern for the annotation.

### Files Scanned

The gate scans:
- `tovarisch/src/status.zig` — Status payload rendering
- `tovarisch/src/status/` — Status rendering paths
- `tovarisch/src/http/` — HTTP request/status paths

Test files (`*_tests.zig`) are exempt. Fixtures (`*/fixtures/*.zig`) are exempt in normal mode, but are scanned in `--self-test` mode (see below).

### Self-Test Mode

The gate supports `--self-test` for validating sentinel fixtures:

```bash
./scripts/check_memory_ownership.sh --self-test
```

In this mode the gate scans `tovarisch/fixtures/` only and validates:
- `bad-memory-ownership-pattern.zig` — MUST fail (proves gate catches bad patterns)
- `good-memory-ownership-pattern.zig` — MUST pass (proves gate accepts annotated safe patterns)

This mode is also run as a sub-step of `make gate` to catch regressions in the gate itself.

### Wire-in

The gate is wired into `make gate`:

```bash
echo "[gate] checking memory ownership hygiene in status/request paths"
./scripts/check_memory_ownership.sh
```

### Fixing Gate Failures

To fix a gate failure:

1. **Add annotation** if the pattern is intentional and safe
2. **Refactor to caller-owned buffer** if the pattern can be avoided

Example refactor from `std.fmt.allocPrint` to `std.fmt.bufPrint`:

```zig
// BEFORE (risky - must be freed or leaks):
const formatted = try std.fmt.allocPrint(allocator, "status: {s}", .{"ok"});
_ = formatted; // Memory leak!

// AFTER (safe - caller owns buffer):
var detail_buf: [64]u8 = undefined;
const detail = std.fmt.bufPrint(&detail_buf, "status: {s}", .{"ok"}) catch "error";
// detail points to detail_buf - no allocation, no leak
```
