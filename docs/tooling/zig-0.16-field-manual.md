# Zig 0.16.x Field Manual

This document captures the current Zig API patterns used in KGB's `tovarisch` service.

**Target:** Zig 0.16.x — Do not downgrade Zig to match stale examples.

## Known-Good Patterns

### Main Entry Point

```zig
pub fn main(init: std.process.Init) !void {
    const arena = init.arena.allocator();
    // ... use arena
}
```

### Stdout/Stderr Writers

```zig
var stdout_buf: [1024]u8 = undefined;
var stdout_file = Io.File.Writer.init(.stdout(), init.io, stdout_buf[0..]);
const stdout = &stdout_file.interface;
```

### Argument Parsing

```zig
const args = try init.minimal.args.toSlice(arena);
```

### Flush Before Exit

```zig
try stdout.flush();
std.process.exit(@intFromEnum(exit_code));
```

### Build System (build.zig)

```zig
const exe = b.addExecutable(.{
    .name = "tovarisch",
    .root_module = b.createModule(.{
        .root_source_file = b.path("src/main.zig"),
        .target = target,
        .optimize = optimize,
    }),
});
```

## Known Traps

- **Stale Zig 0.11–0.14 examples** — Many online examples use old API patterns.
- **Old stdout patterns** — Do NOT use `std.io.getStdOut().writer()`.
- **Old allocator patterns** — Do NOT use `std.heap.GeneralPurposeAllocator` for trivial CLI main.
- **Downgrading Zig** — Never downgrade Zig to satisfy stale examples.

## Agent Instructions

1. **Read this file before editing Zig code.**
2. **Inspect existing working files** (`tovarisch/src/main.zig`, `tovarisch/build.zig`) before inventing APIs.
3. Keep files small and testable.
4. Prefer boring, explicit code over clever abstraction.

## Key Imports

```zig
const std = @import("std");
const Io = std.Io;