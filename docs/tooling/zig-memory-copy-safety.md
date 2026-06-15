# Zig Memory Copy Safety

## Why This Gate Exists

Zig 0.16 panics when `@memcpy` arguments alias. This gate prevents reintroducing that class of crash.

The trap: streaming protocol parsers (BGP, BFD, HTTP, etc.) do recv-buffer compaction. Data arrives, a frame is decoded, remaining bytes slide to the front. If you use `@memcpy` for this, Zig 0.16 explodes with:

```
thread 12345 panic: @memcpy arguments alias
```

The fix is `std.mem.copyForwards` or `std.mem.copyBackwards`, which handle overlap correctly.

## Terminology

### Raw `@memcpy`
The builtin `std.builtin.CopyOptions`-less `@memcpy(dst, src)`.
Forbidden in daemon paths without annotation.

### Same-buffer copy
Both `dst` and `src` slices share the same backing array, even if different offsets.
Example: `buf[0..n]` ← `buf[10..10+n]`  
Always forbidden, even with annotation. No exceptions.

### Safe alternatives

| Pattern | Use when | Call |
|---------|----------|------|
| Forward copy | `src >= dst` (same direction) | `std.mem.copyForwards(u8, dst, src)` |
| Backward copy | `src < dst` (reverse direction) | `std.mem.copyBackwards(u8, dst, src)` |
| Byte-by-byte | Tiny fixed-size structs | `for (src, 0..) \|b, i\| dst[i] = b` |

### Buffer compaction
Shifting received bytes to the front of a recv buffer after decoding a frame.
Always use `copyForwards` (source is ahead of destination, non-overlapping direction).

## Annotation Rules

Raw `@memcpy` is allowed **only** when:

1. A `MemoryCopySafety:` comment is present within **5 lines** of the call.
2. The annotation explains WHY source and destination cannot overlap.
3. **Same-buffer `@memcpy` is always forbidden**, annotation or not.

### Good annotation examples

```zig
// MemoryCopySafety: dst is a fixed-size [256]u8 path buffer, src is
// a caller-provided []const u8 string. No aliasing possible.
@memcpy(buf[0..path.len], path);
buf[path.len] = 0;
```

```zig
// MemoryCopySafety: self.poll_events is a fixed-size temp array copied
// from self.events items. Arrays are distinct allocations.
@memcpy(self.poll_events[0..self.poll_count], self.events.items[0..self.poll_count]);
```

### Bad annotation examples (will be rejected)

```zig
// MemoryCopySafety: bounded by buffer size — THIS IS WRONG
@memcpy(buf[pos..][0..name.len], name);
```

```zig
// MemoryCopySafety: same buffer but different slices — STILL FORBIDDEN
// Same-buffer @memcpy is always rejected regardless of annotation.
@memcpy(sess.recv_buf[0..n], sess.recv_buf[10..10+n]);
```

## Scanned Paths

The gate scans protocol/runtime paths where `@memcpy` hygiene matters most:

- `tovarisch/src/bgp/**/*.zig`
- `tovarisch/src/bfd/**/*.zig`
- `tovarisch/src/runtime/**/*.zig`
- `tovarisch/src/http/**/*.zig`

Excluding:

- `*/fixtures/*.zig` in normal mode (fixtures are only scanned in `--self-test`)
- `_tests.zig` files (test files have their own rules)

## Fixtures

### `tovarisch/fixtures/bad-memory-copy-pattern.zig`
Contains forbidden same-buffer `@memcpy`. Gate must **FAIL** on this file.

### `tovarisch/fixtures/good-memory-copy-pattern.zig`
Contains `@memcpy` with valid `MemoryCopySafety` annotation. Gate must **PASS** on this file.

## Hostile BGP Stream-Framing Tests

See `tovarisch/src/bgp/session_buffer_compaction_tests.zig` for tests that exercise:

1. Split frames (frame arrives in multiple `recv()` calls)
2. Coalesced frames (multiple frames arrive in one `recv()` call)
3. Overlapping copy scenarios during buffer compaction

These tests verify that the codebase handles the exact scenario that caused the original panic.

## Verifier

See `scripts/check_zig_memory_copy_safety.py` for implementation.

## References

- Zig 0.16 field manual: `docs/tooling/zig-0.16-field-manual.md`
- Original ACT: `tovarisch/src/bgp/session_buffer_compaction_tests.zig`
