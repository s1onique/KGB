# Zig 0.16 Observations — 2026-05-25 additions

These entries were too large to fit in the main `zig-0.16-observations.md` file.

---

## execve argv shape

**symptom:** `std.c.execve` expects `argv: [*:null]const ?[*:0]const u8`. Plain pointer array causes type errors.

**wrong assumption:** `[_][*:0]const u8{ ... }` compatible with execve.

**working fix:** Cast array with `@ptrCast`:
```zig
const argv = [_][*:0]const u8{ WG_COMMAND, "show" };
const argv_null: [*:null]const ?[*:0]const u8 = @ptrCast(&argv);
_ = std.c.execve(WG_COMMAND, argv_null, EMPTY_ENV);
```

**files affected:** `tovarisch/src/net/wg_show_collector.zig`

**promote to field manual?** Yes — process execution patterns are common.

---

## std.c.read pointer arithmetic

**symptom:** `std.c.read` expects `[*]u8`. Slice syntax causes type errors.

**wrong assumption:** `read_ptr[bytes_read..remaining]` works for bounded read.

**working fix:** Cast buffer and use pointer arithmetic:
```zig
const read_ptr: [*]u8 = @ptrCast(&buffer);
const n = std.c.read(fd, read_ptr + bytes_read, remaining);
```

**files affected:** `tovarisch/src/net/wg_show_collector.zig`

**promote to field manual?** Yes — libc-style APIs require pointer types, not slices.
