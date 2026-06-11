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
- **CI Zig dev builds** — CI uses `0.16.0-dev.732+2f3234c76` which may lack `std.process.Init`. This is a CI configuration issue, not a code issue.

## Zig 0.16 entrypoint compatibility

The `std.process.Init` entrypoint pattern is the correct approach for Zig 0.16.x:

```zig
pub fn main(init: std.process.Init) !void {
    const arena = init.arena.allocator();
    const args = try init.minimal.args.toSlice(arena);
    // ... use arena and args
}
```

This works on stable Zig 0.16.0. CI failures may be due to CI using an incompatible dev build.

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

## C Path APIs Require Null-Terminated Strings

`std.fmt.bufPrint()` returns a `[]u8` slice and does **not** append a null terminator.

C filesystem APIs (`open`, `fopen`, `access`, `mkdir`, `rmdir`) require null-terminated paths. Always copy the slice into a stack buffer, append `0`, and pass `[*:0]const u8`.

**Centralized helper pattern:**

```zig
/// Converts a Zig slice to a null-terminated C string.
/// std.fmt.bufPrint() returns a slice without null-terminator,
/// but C filesystem APIs require null-terminated paths.
fn toCString(path: []const u8, buf: *[4096]u8) error{PathTooLong}![*:0]const u8 {
    if (path.len >= buf.len) return error.PathTooLong;
    @memcpy(buf[0..path.len], path);
    buf[path.len] = 0;
    return @as([*:0]const u8, @ptrCast(buf));
}

// Usage:
var path_buf: [4096]u8 = undefined;
const c_path = try toCString(path, &path_buf);
const fd = std.c.open(c_path, flags, mode);
```

**Why this matters:**
- `[]u8` is a length-carrying Zig slice (no implicit null terminator)
- `path.ptr` from a slice is type `[*]const u8` (pointer without null termination)
- C path APIs expect `[*:0]const u8` (pointer with null terminator)
- String literals in Zig are null-terminated, but `bufPrint()` output is not

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

## Cross-Platform Semantic Analysis

### Platform-Specific Code Analysis Gap

A branch guarded by `@import("builtin").os.tag` may hide platform-specific API drift on non-target hosts. Linux-only code that compiles cleanly on macOS local gate may not be validated because the Linux branch is not semantically analyzed. On Linux CI, that branch becomes live, exposing any API mismatches.

**Example:** `std.fs.cwd().openFile()` was rejected in Linux CI for the `/proc/self/status` RSS reader, while macOS local gate stayed green because the Linux branch was not active.

**Mitigation:** Use cross-platform compile targets to verify:

```bash
zig build -Dtarget=x86_64-linux-gnu
```

**Rule:** Do not add a cross-target test target unless the build graph is confirmed to compile tests without executing them, or the target runs on native Linux.

### General KGB/tovarisch Doctrine

Any platform-specific branch gets a matching compile target before it is trusted:

```make
tovarisch-compile-linux:
	cd tovarisch && zig build -Dtarget=x86_64-linux-gnu

tovarisch-compile-macos:
	cd tovarisch && zig build -Dtarget=x86_64-macos
```

Also consider native architecture variants (e.g., `aarch64-macos` for Apple Silicon).

## Linux-Specific Patterns

For Linux-specific patterns (file operations, directory APIs, cross-platform compilation, etc.), see [`zig-0.16-linux-patterns.md`](./zig-0.16-linux-patterns.md).

## ArrayList allocator-passing pattern

In Zig 0.16-era `std.ArrayList`, initialize with `.empty` and pass the allocator to mutating/ownership methods:

```zig
var names = std.ArrayList([]const u8).empty;
defer names.deinit(allocator);

try names.append(allocator, item);
const owned = try names.toOwnedSlice(allocator);
```

Use the same allocator throughout the list lifetime.

## dirent name field

In this Zig 0.16 environment, `std.c.dirent` exposes the directory-entry name as `name`, not `d_name`.

## bufPrint aliasing with format arguments

> **See also:** `zig-0.16-observations.md` — this section documents a known Zig 0.16 `@memcpy arguments alias` panic in chained `bufPrint()` calls. Full details and solution pattern are documented there.

## Threading and heartbeat lessons

For threading patterns, detached heartbeat implementation, blocking sleep, and context lifetime ownership, see [`zig-0.16-threading-heartbeat.md`](./zig-0.16-threading-heartbeat.md).

## Request/Status Rendering and Memory Ownership

For page_allocator leak lessons, request/status rendering memory ownership patterns, and production soak test guidance, see [`zig-0.16-field-manual-rss-leak.md`](./zig-0.16-field-manual-rss-leak.md).

## TCP Socket Testing in Zig Tests

For TCP socket testing patterns that prevent CI hangs, see [`zig-0.16-tcp-socket-tests.md`](./zig-0.16-tcp-socket-tests.md). This companion doc covers: no raw blocking `accept()`/`recv()`, bounded `poll()` before socket receive, and compile/run split for CI observability.

