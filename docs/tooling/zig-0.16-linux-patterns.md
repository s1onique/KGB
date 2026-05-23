# Zig 0.16 Linux-Specific Patterns

This document captures Linux-specific patterns for KGB's `tovarisch` service.

**Target:** Zig 0.16.x on Linux — Do not downgrade Zig to match stale examples.

## Linux File Operations

Use `std.c.open` with `std.os.linux.O` packed struct. **Never use boolean fields** like `.WRONLY = true` — use `.ACCMODE` enum:

```zig
// Reading: RDONLY
const flags = std.os.linux.O{ .ACCMODE = std.posix.ACCMODE.RDONLY };
const fd = std.c.open(c_path, flags, @as(c_uint, 0));

// Writing: WRONLY with CREAT/TRUNC
const flags = std.os.linux.O{
    .ACCMODE = std.posix.ACCMODE.WRONLY,
    .CREAT = true,
    .TRUNC = true,
};
const fd = std.c.open(c_path, flags, @as(c_uint, 0o644));
```

**Key insight:** `.ACCMODE` values: `.RDONLY`, `.WRONLY`, `.RDWR`. Cast perm to `c_uint`. Link libc via module options for cross-platform targets.

## libc Directory APIs with Optional C Pointers

`std.c.opendir()` returns `?*c.DIR` (optional). `std.c.readdir()` and `std.c.closedir()` expect `*c.DIR` (non-optional). Zig 0.16 does not implicitly unwrap.

```zig
const dir_optional = std.c.opendir("/sys/class/net");
const dir = dir_optional orelse return error.SkipZigTest;
defer _ = std.c.closedir(dir);

while (true) {
    const entry = std.c.readdir(dir) orelse break;  // also optional
    // ...
}
```

**Linux-only code path** — macOS tests skip this path; type errors surface only on Linux CI.

## Avoiding libc `dirent` Layout Fragility

**Prefer candidate probing over `readdir()` for smoke tests.**

The `std.c.dirent` struct layout (specifically `d_name` field offset) is libc-platform-dependent and not reliably exposed via `@offsetOf(std.c.dirent, "d_name")` in Zig 0.16/Linux. This makes directory iteration via libc brittle.

For smoke tests that only need to exercise file reading on Linux, use bounded candidate probing:

```zig
// Candidate interfaces ordered by likelihood on GitHub Actions Linux.
// The loopback interface 'lo' almost certainly exists and has statistics.
const candidates = [_][]const u8{
    "lo",
    "eth0",
    "ens3",
    "enp0s1",
    "enp0s3",
};

var exercised = false;

for (candidates) |iface| {
    // Try to read stats from a known interface name.
    readInterfaceStats(allocator, sysfs_root, iface) catch continue;

    // Successful read is the smoke assertion.
    exercised = true;
    break;
}

if (!exercised) return error.SkipZigTest;
```

**Why this is better:**
- Removes libc layout dependency
- Directly exercises the actual reader code under test
- Fewer moving parts = better smoke signal
- `lo` almost always exists on Linux CI

**Alternative:** If directory iteration is truly needed, prefer `std.fs.Dir.iterate()` when available.

## Cross-Platform Semantic Analysis

### Platform-Specific Code Analysis Gap

A branch guarded by `@import("builtin").os.tag` may hide platform-specific API drift on non-target hosts. Linux-only code that compiles cleanly on macOS local gate may not be validated because the Linux branch is not semantically analyzed. On Linux CI, that branch becomes live, exposing any API mismatches.

**Example:** A Linux-only file API mismatch was rejected in Linux CI while macOS local gate stayed green because the Linux branch was not semantically analyzed.

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

