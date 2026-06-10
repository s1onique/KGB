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
