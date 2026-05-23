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
```

## Zig 0.16 JSON Serialization

Do not assume a high-level `std.json.stringify()` helper exists.

For controlled JSON output, use the **streaming API**:

```zig
var jw = std.json.Stringify{ .writer = writer };
try jw.beginObject();
try jw.objectField("field_name");
try jw.write(value);
try jw.beginArray();
try jw.write(array_element);
try jw.endArray();
try jw.endObject();
```

### Key methods

| Method | Purpose |
|--------|---------|
| `beginObject()` | Start an object `{` |
| `endObject()` | End an object `}` |
| `beginArray()` | Start an array `[` |
| `endArray()` | End an array `]` |
| `objectField(name)` | Write a field name key `"name":` |
| `write(value)` | Write a value (string, number, bool, etc.) |

### Example: `status --json`

See `tovarisch/src/status.zig` for a working example of rendering a status payload:

```zig
pub fn renderPayload(writer: anytype) !void {
    var jw = std.json.Stringify{ .writer = writer };
    try jw.beginObject();
    try jw.objectField("service");
    try jw.write(static_status.service);
    // ... more fields
    try jw.endObject();
}
```

This pattern is suitable for stable CLI JSON contracts.

## Reserved Keywords in Enum Variants

If an enum variant name collides with a Zig keyword, escape it with `@"..."`.

Example:

```zig
pub const CheckStatus = enum {
    ok,
    warn,
    @"error",
};
```

Use it as `.@"error"` in comparisons and construction.

This matters because `error` is a reserved keyword in Zig. Without escaping:
- The enum variant cannot be named `error`
- Comparison `.error` fails to parse

With escaping:
- The variant `@"error"` is valid
- Use `.@"error"` to refer to it
- `@tagName()` correctly outputs `"error"` (quotes are part of the identifier)

## Daemon Command Tests

Do not unit-test commands that enter blocking daemon loops by calling the top-level CLI runner.

Extract pure argument parsing and config construction into a separate function, test that, and cover the daemon loop with a manual or integration smoke test.

**Example pattern:**

```zig
pub fn parseServeArgs(args: []const []const u8, stderr: anytype) ServeParseResult {
    // Pure parsing logic
    ...
}

// Test the parser, not the daemon
test "parseServeArgs defaults to loopback" {
    const w = VoidWriter{};
    const parsed = parseServeArgs(&.{}, w);
    try std.testing.expect(parsed == .ok);
}
```

The blocking accept loop is not executed by unit tests. Use manual smoke tests or integration tests for the daemon path.

## HTTP Method Parsing

`std.meta.stringToEnum()` is exact and does not map uppercase protocol strings such as `GET` to lowercase enum tags such as `.get`.

Use an explicit parser for wire-protocol tokens:

```zig
fn parseMethod(method_str: []const u8) Method {
    if (std.mem.eql(u8, method_str, "GET")) return .get;
    if (std.mem.eql(u8, method_str, "POST")) return .post;
    // ... other methods
    return .unknown;
}
```

## Third-Party build.zig Files May Break Under Zig 0.16 Build API Changes

Third-party projects that include their own `build.zig` may use APIs that have changed or been removed in Zig 0.16.

### Observed Example: zig-kcov Build Failure

The `roc-lang/zig-kcov` fork's `build.zig` uses `Run.captureStdOut()` without required options argument:

```
build.zig:108:51: error: member function expected 1 argument(s), found 0
```

**Root cause**: Zig 0.16 requires `Run.captureStdOut()` with explicit options.

**Workaround**: Use CMake to build third-party projects instead of Zig's build system, when available. The CMake path for zig-kcov builds and runs correctly on Linux.

**Rule**: When third-party build.zig fails under Zig 0.16, check if a CMake or autotools path exists before attempting to patch the build.zig.

## Zig 0.16 stdlib Drift

For Zig 0.16-dev, verify standard library APIs against the installed local stdlib before coding from memory. Observed mismatches:

| API | Status | Alternative |
|-----|--------|-------------|
| `std.time.sleep` | Unavailable | `std.Thread.yield()` for non-blocking polling |
| `std.io.fixedBufferStream` | Unavailable | Implement inline fixed buffer writer |
| `std.c.write` | 3 args (fd, [*]const u8, usize) | Cast slices to raw pointers |
| `std.c.fd_set`, `std.c.timespec` | Platform-dependent | Use alternative approaches |

**Always inspect local stdlib source** when encountering API uncertainty. Zig's language reference and stdlib are versioned; local source is the right source of truth for API drift.

## HTTP Server Sockets and libc

When implementing HTTP servers with socket operations, be aware:

### std.posix Limitations in Zig 0.16

The `std.posix` namespace does **NOT** expose socket functions directly:
- `std.posix.socket` — **NOT AVAILABLE**
- `std.posix.bind` — **NOT AVAILABLE**
- `std.posix.listen` — **NOT AVAILABLE**
- `std.posix.accept` — **NOT AVAILABLE**

Socket operations exist in:
- `std.os.linux.socket` (Linux direct syscalls, no libc required)
- `std.c.socket` (libc wrapper, requires explicit libc linking)

### Using std.c.socket in Zig 0.16

To use `std.c.socket`, `std.c.bind`, `std.c.listen`, `std.c.accept` with cross-platform targets, set `.link_libc = true` in the module options.

**Important:** In Zig 0.16 module-style build, there is **no `exe.linkLibC()`** method. Link libc via `Module.CreateOptions`:

```zig
const exe = b.addExecutable(.{
    .name = "tovarisch",
    .root_module = b.createModule(.{
        .root_source_file = b.path("src/main.zig"),
        .target = target,
        .optimize = optimize,
        // Required for std.c.socket on cross-platform builds
        .link_libc = true,
    }),
});
```

This is required for non-native targets (e.g., `arm-linux-musleabihf`).

### Leaf-Service Doctrine Tradeoff

The leaf-service doctrine prefers avoiding libc where practical. However:
- HTTP socket functionality requires explicit libc linking in Zig 0.16
- This is a documented exception until `std.posix` includes socket APIs
- TODO: Investigate `std.os.linux.socket`-based alternative for pure syscall approach

### Critical: `.link_libc` is NOT a project-wide flag

In Zig 0.16 module-style build, `.link_libc` is a **module/artifact-level build option**. It must be applied to **every root module** that imports code using `std.c.*` functions.

**Executables and tests are separate build artifacts with separate root modules:**

```zig
// Executable root module — has .link_libc
const exe = b.addExecutable(.{
    .root_module = b.createModule(.{
        .root_source_file = b.path("src/main.zig"),
        .link_libc = true,  // Required for std.c.*
    }),
});

// Test root module — must ALSO have .link_libc
// It does NOT inherit the executable's module options!
const unit_tests = b.addTest(.{
    .root_module = b.createModule(.{
        .root_source_file = b.path("src/test_all.zig"),
        .link_libc = true,  // Required for std.c.* in test compilation
    }),
});
```

**Key insight:** `b.addExecutable()` and `b.addTest()` each create a separate build artifact with a separate root module. Settings on one artifact's root module do **not** automatically apply to another.

When `test_all.zig` uses `std.testing.refAllDecls`, it forces compilation of all imported declarations, including code that references `std.c.write`, `std.c.socket`, etc. This requires libc linkage at **test compile time** as well.

**Rule:** Apply `.link_libc = true` to every `b.createModule()` that imports code which can reference `std.c.*` functions.
