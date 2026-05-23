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

---

## 2026-05-23 — `@memcpy arguments alias` panic in chained `std.fmt.bufPrint()` calls

- **Context:** Fixture tests in `tovarisch/src/net/linux_interface_stats_tests.zig` building paths like `{base}/{iface}/statistics/{file}`.
- **Symptom:** `panic: @memcpy arguments alias` when the same buffer was used as both bufPrint destination and as a format argument.
- **Failed assumption:** Reusing a single buffer for sequential path construction and number formatting was safe.

**Working fix — use distinct buffers:**

```zig
var parent_buf: [4096]u8 = undefined;
var child_buf: [4096]u8 = undefined;
const parent = try std.fmt.bufPrint(&parent_buf, "{s}/{s}", .{ base, iface });
const child = try std.fmt.bufPrint(&child_buf, "{s}/statistics", .{parent});
```

- **Files affected:** `tovarisch/src/net/linux_interface_stats_tests.zig`
- **Promote to field manual:** Yes.

**Confidence:** high; verified with `make tovarisch-test` passing.

---

## 2026-05-23 — `@hasDecl` with pointer dereference and writer type compatibility

- **Context:** Implementing `writeLogRecord()` helper in `server.zig` to flush NDJSON records immediately.
- **Symptom:** Two compilation errors:
  1. `error: cannot dereference non-pointer type 'cli.commands.VoidWriter'` — `@hasDecl(@TypeOf(out_writer.*), "flush")` fails when writer is passed by value.
  2. `error: no field or member function named 'flush' in 'cli.commands.CaptureWriter'` — writer without `flush` method doesn't compile.
- **Failed assumptions:**
  1. `@hasDecl(@TypeOf(out_writer.*), "flush")` would work for all writer types (pointer or value).
  2. `std.meta.trait(.Pointer)` could be used to detect pointer types (does not exist in this Zig 0.16).
  3. A catch-all `out_writer.flush() catch {}` would work for writers without flush (Zig requires method to exist at compile time).
- **Root cause:** Zig's compile-time type checking with `anytype` requires explicit handling for both pointer and value writer types. The test writers (`VoidWriter`, `CaptureWriter`) have different shapes than the real CLI writer.
- **Working fix:**
  1. Use comptime type check to skip flush for `logging.BufferedWriter` (no flush method):
     ```zig
     if (comptime @TypeOf(out_writer) == *logging.BufferedWriter) {
         // No-op: BufferedWriter doesn't have flush
     } else {
         out_writer.flush() catch {};
     }
     ```
  2. Add no-op `flush` method to test writers for compatibility:
     ```zig
     pub fn flush(_: *Self) error{}!void {}
     ```
- **Files affected:** `tovarisch/src/http/server.zig`, `tovarisch/src/cli/commands.zig`
- **Promote to field manual:** No — specific to the `writeLogRecord` pattern. This is a working fix pattern.

**Confidence:** high; verified with `make tovarisch-build`, `make tovarisch-test`, `make gate` all passing.

---

## 2026-05-24 — Per-Recv Timeout: Bounded Loop Alone Is Insufficient

- **Context:** GitHub CI still hangs in `zig build test` after adding bounded recv loop. The screenshot shows Ubuntu amd64 deb job stuck in `cd tovarisch && zig build test` after hygiene/build succeeded.
- **Symptom:** CI test binary is still running, not failing. The remaining blocker is a live kernel path: individual `std.c.recv()` blocking forever before loop counter advances.
- **Root cause:** Bounded outer loop limits the number of recv() iterations, but each individual `std.c.recv()` can block indefinitely if the kernel never responds.
- **Failed assumption:** Adding loop bounds prevents hangs. It does NOT — each syscall can still block forever.

**Working fix — per-recv timeout using std.c.pollfd:**

```zig
const POLLIN: c_short = 0x001;

fn waitReadable(sock: c_int) AddrError!void {
    var fds = [_]std.c.pollfd{
        .{ .fd = sock, .events = POLLIN, .revents = 0 },
    };
    const rc = std.c.poll(&fds, 1, 250); // 250ms timeout
    if (rc <= 0) return error.RecvFailed;
    if ((fds[0].revents & POLLIN) == 0) return error.RecvFailed;
}

// Before every recv():
try waitReadable(sock);
const recv_result = std.c.recv(sock, @ptrCast(&buffer), buffer.len, 0);
```

**Final defense-in-depth doctrine:**

1. **Per-recv timeout** — poll() before recv(); fail fast on timeout
2. **Bounded recv loop** — max iterations (e.g., 16); prevent runaway loops
3. **Progress protection** — ensure parsing advances offset; invalid message on stall
4. **Fallback JSON** — `/metrics.json` renders warning on live collection failure

**Key insight:** All three layers are needed. Per-recv timeout is the critical fix — without it, the first blocking recv() hangs CI before any loop bound is ever checked.

- **Files affected:** `tovarisch/src/net/linux_addr.zig`
- **Promote to field manual:** Yes — update "Linux rtnetlink Socket Pattern" section with per-recv timeout pattern.

---

## 2026-05-24 — rtnetlink NETLINK_ROUTE socket implementation lessons

- **Context:** Implementing `discoverPrivateAddresses()` in `tovarisch/src/net/linux_addr.zig` using `AF_NETLINK` socket for live interface address discovery.
- **Key lessons learned:**

### AF_NETLINK = 16, not a named constant
- `AF_NETLINK` is not exposed as a named constant in `std.c`; use literal `16`.
- `NETLINK_ROUTE` protocol is `0`.
- `SOCK_RAW` socket type is `3`.

### Message alignment with `align4()`
- Netlink messages and attributes must be aligned to 4-byte boundaries.
- Use `(len + 3) & ~@as(usize, 3)` for alignment; advance offsets by aligned lengths.
- Buffer offset-based iteration avoids pointer arithmetic pitfalls.

### Multipart response handling via `NLMSG_DONE`
- Kernel may return multipart messages; outer loop continues until `nlhdr.nlmsg_type == NLMSG_DONE`.
- Use a `done` flag: `if (nlhdr_ptr.nlmsg_type == NLMSG_DONE) { done = true; break; }`
- `RTM_NEWADDR` responses contain interface address attributes.

### Attribute parsing: buffer offsets, not pointers
- `rtattr.rta_len` includes header size; attribute data starts at `pos + @sizeOf(rtattr)`.
- Use explicit bounds checking: `if (data_end >= data_start + 4)` for IPv4.
- `@memcpy` with exact length: `@memcpy(address_octets[0..4], buffer[data_start .. data_start + 4])`.

### IFA_ADDRESS vs IFA_LOCAL fallback
- IPv4 addresses come in `IFA_ADDRESS` (peer/destination) and `IFA_LOCAL` (local interface).
- For point-to-point links, `IFA_LOCAL` may be the only address present.
- Parse `IFA_ADDRESS` first, then fall back to `IFA_LOCAL` if not found.

### IFA_LABEL for interface names
- Interface names (eth0, wg0, lo) appear in `IFA_LABEL` attribute.
- `parseLabel()` extracts null-terminated strings from netlink buffer: handles both null-terminated and length-bounded strings.
- Labels may contain null bytes in the middle; only return up to first null.

### AF_INET filter for IPv4-only
- Check `ifa_family == AF_INET` (2) to filter out IPv6 addresses.
- Set `ifa_family = 0` (AF_UNSPEC) in request to get all families, filter in response.

- **Files affected:** `tovarisch/src/net/linux_addr.zig`, `tovarisch/src/net/linux_addr_parse.zig`
- **Promote to field manual:** Yes — add "Linux rtnetlink Socket Pattern" section.

---

## 2026-05-24 — Raw u8 buffers must NOT be @alignCast into extern structs

- **Context:** Linux live tests crash with `panic: incorrect alignment` in `discoverPrivateAddresses()` on `tovarisch/src/net/linux_addr.zig:201`.
- **Symptom:** `@ptrCast(@alignCast(&buffer[offset]))` panics when the byte offset is not aligned for `nlmsghdr` (extern struct requires proper alignment).
- **Root cause:** Raw `[]u8` network/kernel buffers arrive at arbitrary byte offsets. Zig correctly rejects casting unaligned byte pointers to extern structs.
- **Wrong assumption:** `@alignCast` was assumed to safely convert any pointer. It does NOT — it panics when the source is not actually aligned.

**Working fix — reading response structs:**

```zig
fn readStruct(comptime T: type, bytes: []const u8) AddrError!T {
    if (bytes.len < @sizeOf(T)) return error.InvalidMessage;
    var value: T = undefined;
    @memcpy(std.mem.asBytes(&value), bytes[0..@sizeOf(T)]);
    return value;
}

// Usage:
const nlhdr = try readStruct(nlmsghdr, buffer[offset..msg_len]);
const response_len = @as(usize, @intCast(nlhdr.nlmsg_len));
```

**Working fix — building request structs:**

```zig
// Build local aligned structs and copy into request buffer
var hdr = nlmsghdr{
    .nlmsg_len = @intCast(nlmsg_len),
    .nlmsg_type = @intCast(RTM_GETADDR),
    // ...
};
var msg = ifaddrmsg{ ... };

@memcpy(request[0..@sizeOf(nlmsghdr)], std.mem.asBytes(&hdr));
@memcpy(request[@sizeOf(nlmsghdr)..][0..@sizeOf(ifaddrmsg)], std.mem.asBytes(&msg));
```

**Doctrine:** Raw `[]u8` network/kernel buffers are byte-aligned. Do NOT `@alignCast` them into extern structs. Copy bytes into aligned local structs instead.

- **Files affected:** `tovarisch/src/net/linux_addr.zig`
- **Promote to field manual:** Yes — refine the alignment section to cover network buffers.

---

## 2026-05-24 — rtnetlink Constants Bug: RTM_GETADDR=20, NLM_F_DUMP=0x003

- **Context:** Live strace showed `nlmsg_type=0x14` (20) with `NLM_F_REQUEST|NLM_F_MULTI` and kernel returning `NLMSG_ERROR -EOPNOTSUPP`.
- **Symptom:** `/metrics.json` falls back to warning; rtnetlink address discovery fails.
- **Root cause:** Two wrong constants:
  1. `RTM_GETADDR` was 20 (actually `RTM_NEWADDR`). Correct: `RTM_GETADDR = 22`.
  2. `NLM_F_DUMP` was 0x003 (actually `REQUEST|MULTI`). Correct: `NLM_F_DUMP = 0x300` (`NLM_F_ROOT|NLM_F_MATCH`).

- **Working fix — corrected constants:**
  ```zig
  const RTM_NEWADDR: c_uint = 20;
  const RTM_DELADDR: c_uint = 21;
  const RTM_GETADDR: c_uint = 22;
  const NLMSG_ERROR: c_uint = 2;
  const NLM_F_REQUEST: c_uint = 0x001;
  const NLM_F_ROOT: c_uint = 0x100;
  const NLM_F_MATCH: c_uint = 0x200;
  const NLM_F_DUMP: c_uint = NLM_F_ROOT | NLM_F_MATCH; // 0x300
  ```

- **Also fix: set `ifa_family = @intCast(AF_INET)` and handle `NLMSG_ERROR`:**
  ```zig
  if (nlhdr.nlmsg_type == NLMSG_ERROR) return error.InvalidMessage;
  ```

- **Files affected:** `tovarisch/src/net/linux_addr.zig`, `tovarisch/src/net/linux_addr_tests.zig`
- **Promote to field manual:** Yes.
