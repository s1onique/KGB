# Zig 0.16 Observations

Recording field notes from Zig 0.16 experiments. Confidence varies; do not promote to field manual until verified with a minimal reproducer.

---

## 2026-05-22 — ArrayList initialization uncertainty in Zig 0.16

- **Context:** Attempted to use `std.ArrayList(u8)` in tests/status construction.
- **Symptom:** Expected older examples around `ArrayList.init(allocator)` did not work as-is.
- **Failed assumption:** Online examples for `std.ArrayList` are reliable for Zig 0.16.
- **Working fix:** Avoided allocation for the static v0 status payload and used a compile-time constant string.
- **Candidate doctrine:** Do not introduce dynamic allocation for status JSON until we have a local Zig 0.16-compatible buffer/list pattern.
- **Confidence:** medium; needs a minimal reproducer before promotion to the field manual.

---

## 2026-05-22 — Process entry point confirmed working

- `pub fn main(init: std.process.Init) !void` works as documented.
- `init.arena.allocator()` provides the arena allocator.
- `init.minimal.args.toSlice(arena)` works for argument extraction.
- `Io.File.Writer.init(.stdout(), init.io, buf[0..])` works for stdout/stderr writers.
- Flush stdout before `std.process.exit(...)`.

**Confidence:** high; these patterns are proven in `tovarisch/src/main.zig`.

---

## 2026-05-22 — Build system patterns confirmed

- `.root_module = b.createModule(...)` is the correct pattern in Zig 0.16.
- Module options like `.test_files` are NOT valid in `b.createModule()` options for `addTest`.
- Tests are discovered from all source files in the module; explicit test files not required.

---

## 2026-05-23 — `std.c.dirent.d_name` field not exposed in Zig 0.16/Linux

- **Context:** Linux CI fails in `tovarisch/src/net/linux_stats_tests.zig` with `error: no field named 'd_name' in struct 'c.dirent__struct_...'`
- **Symptom:** `@offsetOf(std.c.dirent, "d_name")` fails because `std.c.dirent` in Zig 0.16/Linux does not expose the `d_name` field.
- **Failed assumption:** The libc `dirent` struct layout was assumed to be reliably introspectable via `@offsetOf`. This depends on platform-specific libc implementation details.
- **Root cause:** The test used a brittle offset-hack to extract interface names from directory entries:
  ```zig
  const name_ptr = @as([*:0]const u8, @ptrFromInt(@intFromPtr(entry) + @offsetOf(std.c.dirent, "d_name")));
  ```
- **Working fix:** Replace dirent iteration with bounded candidate probing:
  ```zig
  const candidates = [_][]const u8{ "lo", "eth0", "ens3", "enp0s1", "enp0s3" };
  for (candidates) |iface| {
      readInterfaceStats(allocator, sysfs_root, iface) catch continue;
      exercised = true;
      break;
  }
  ```
- **Why this is better:** Removes libc layout dependency, directly exercises the Zig reader, fewer moving parts.
- **Files affected:** `tovarisch/src/net/linux_stats_tests.zig`
- **Promote to field manual:** Yes — added "Avoiding libc `dirent` Layout Fragility" section.

**Confidence:** high; verified by replacing brittle dirent code with candidate probing.

---

## 2026-05-23 — `std.c.opendir()` returns optional C pointer

- **Context:** Linux CI fails in `tovarisch/src/net/linux_stats_tests.zig` on `std.c.opendir()` result passed to `std.c.readdir()`.
- **Symptom:** `error: expected type '*c.DIR', found '?*c.DIR'`
- **Failed assumption:** The optional DIR pointer from `opendir()` could be passed directly to `readdir()`. Zig 0.16 does not implicitly unwrap optional C pointers.
- **Working fix:** Unwrap `opendir()` result once with `orelse`, pass non-optional to both `readdir()` and `closedir()`:
  ```zig
  const dir_optional = std.c.opendir(sysfs_root);
  const dir = dir_optional orelse return error.SkipZigTest;
  defer _ = std.c.closedir(dir);

  while (true) {
      const entry = std.c.readdir(dir) orelse break;
      // ... process entry ...
  }
  ```
- **Why this escaped:** macOS local tests skip the Linux-only smoke test, so the type mismatch only surfaces on Linux CI.
- **Files affected:** `tovarisch/src/net/linux_stats_tests.zig`
- **Promote to field manual:** Yes — added "libc Directory APIs with Optional C Pointers" section.

**Confidence:** high; same class as the `.WRONLY` ACCMODE issue — platform-specific branch not validated until Linux CI runs.

---

## 2026-05-23 — Linux open flags: ACCMODE, not boolean `.WRONLY`

- **Context:** Linux CI fails in `tovarisch/src/net/linux_stats.zig` on open flags.
- **Symptom:** `error: no field named 'WRONLY' in struct 'os.linux.O__struct_...'`
- **Failed assumption:** `std.os.linux.O` has boolean fields like `.WRONLY = true`. It does NOT.
- **Root cause:** On Linux, access mode is represented through `.ACCMODE` enum field with values like `.WRONLY` / `.RDWR`.
- **Working fix:** Use `.ACCMODE = std.posix.ACCMODE.WRONLY` instead:
  ```zig
  const flags = std.os.linux.O{
      .ACCMODE = std.posix.ACCMODE.WRONLY,
      .CREAT = true,
      .TRUNC = true,  // Truncate for clean fixture writes
  };
  ```
- **Why this escaped:** Linux-only branch inside `openForWrite()` helper used by tests. macOS local tests exercised the fallback path, not the Linux packed `O` struct.
- **Files affected:** `tovarisch/src/net/linux_stats.zig`
- **Promote to field manual:** Yes — added "Writing Files on Linux" section with the correct pattern.

**Confidence:** high; verified with `make tovarisch-test` passing.

---

## 2026-05-23 — `@intCast` target inference

- **Context:** Using integer type coercion in Zig 0.16.
- **Symptom:** Unclear which API to use for integer casting.
- **Working fix:** `@intCast(value)` takes one argument and infers the target integer type from assignment/context:
  ```zig
  octets[octet_idx] = @intCast(value);
  ```
  For explicit type coercion where inference is not enough, use `@as(T, value)`.
- **Files affected:** Any Zig code doing integer casts.
- **Promote to field manual:** Yes — this is a general Zig 0.16 pattern.

**Confidence:** high; confirmed by reading Zig 0.16 stdlib behavior.

---

## 2026-05-23 — c.sockaddr.in is nested inside c.sockaddr, not a direct c member

- **Context:** Fixing `bind failed errno=97` (EAFNOSUPPORT) on Linux. Attempted to use `c.sockaddr_in` directly.
- **Symptom:** `error: root source file struct 'c' has no member named 'sockaddr_in'`
- **Failed assumption:** `c.sockaddr_in` exists as a direct member of `std.c`. The correct path is `c.sockaddr.in` (nested inside the `sockaddr` struct).
- **Root cause of original bug:** The custom `SockaddrIn` struct had a `sin_len` field (macOS style) but Linux's `sockaddr` struct does NOT have a length prefix. This caused `EAFNOSUPPORT` because Linux couldn't parse the malformed sockaddr.
- **Working fix:** Use `c.sockaddr.in` which is the correct cross-platform sockaddr_in definition for both Linux and macOS:
  ```zig
  var addr: c.sockaddr.in = std.mem.zeroes(c.sockaddr.in);
  addr.family = c.AF.INET;
  addr.port = std.mem.nativeToBig(u16, self.config.port);
  addr.addr = parseIpAddress(self.config.address);
  ```
- **Key difference:** Linux's `c.sockaddr.in` has fields `family`, `port`, `addr`, `zero` (no length prefix). macOS's has `len`, `family`, `port`, `addr`, `zero`. The `c.sockaddr.in` struct is platform-adaptive.
- **Files affected:** `tovarisch/src/http/server.zig`
- **Promote to field manual:** No — this is specific to the sockaddr fix. The field manual section on libc socket operations should mention the correct pattern.

**Confidence:** high; verified with successful `make tovarisch-build`, `make tovarisch-test`, `make tovarisch-status`, `make gate`.

---

## 2026-05-23 — CI Zig dev build lacks `std.process.Init`

- **Context:** GitHub CI fails during Zig build because CI uses `0.16.0-dev.732+2f3234c76`.
- **Symptom:** CI compiler does not have `std.process.Init`, causing `pub fn main(init: std.process.Init) !void` to fail.
- **Local verification:** Stable Zig 0.16.0 (macOS) HAS `std.process.Init` and the original code compiles and works.
- **Failed assumption:** The task suggested CI lacks `Init` and recommended a fallback pattern using `std.process.argsAlloc` and `GeneralPurposeAllocator`.
- **Working fix:** Confirmed the original `std.process.Init` entrypoint works on local stable Zig 0.16.0. The CI failure is specific to that dev build.
- **Alternative patterns attempted (failed on stable Zig 0.16.0):**
  1. `std.heap.GeneralPurposeAllocator` — does not exist in stable 0.16.0
  2. `std.process.argsAlloc(allocator)` — does not exist in stable 0.16.0  
  3. `std.process.args.iterator()` — `std.process.args` does not exist in stable 0.16.0
  4. `std.process.args.toSlice(...)` — `std.process.args` does not exist in stable 0.16.0
- **Files affected:** `tovarisch/src/main.zig` (no changes needed — original works on stable)
- **Promote to field manual:** Yes — documented the `Init`-based entrypoint as the stable pattern.
- **Epic status:** Keep Debian package release ACT open until CI Zig drift is resolved.

**Confidence:** high; verified locally with `make gate`, `make tovarisch-build`, `make tovarisch-test`, `make tovarisch-status`.

---

## 2026-05-22 — Io.Dir API not directly usable in status.zig

- **Context:** Attempted to use `std.fs.cwd()` and `std.Io.Dir.cwd()` to check directory existence.
- **Symptom:** `std.fs.cwd()` does not exist in Zig 0.16; `std.Io.Dir.cwd()` exists but requires an `Io` context for operations.
- **Failed assumptions:**
  1. `std.fs.cwd()` was assumed to be available (old API pattern)
  2. `std.process.cwd()` was assumed to exist (doesn't exist in Zig 0.16)
  3. `std.Io.Dir.openDir(path, io, options)` requires 3 arguments including `Io` context
  4. `std.options.debug_io` doesn't exist as a simple field access
  5. Static global initialization in `const x = fn()` causes segfault at runtime for certain function calls
- **Working fix:** For v0 ACT 2, implemented `state_dir` check as a placeholder that always returns `warn`. The actual directory checking will be implemented once the `Io.Dir` API is fully understood.
- **Files affected:** `tovarisch/src/status.zig`
- **Promote to field manual:** No — this is a known limitation, not a recommended pattern. The placeholder approach is temporary.
- **Recommendation:** Investigate `std.fs.Dir.stat()` or another simpler API for directory existence checking. The old `std.fs.cwd().stat(path)` pattern is invalid in Zig 0.16.

**Confidence:** high; verified with compiler errors and successful builds.
