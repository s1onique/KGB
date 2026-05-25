# Zig 0.16 Observations

---

## 2026-05-24 — Zig 0.16 threading and sleep APIs (heartbeat thread implementation)

- **Context:** Implementing a detached heartbeat thread for `tovarisch` that emits logs every 30 seconds independently of HTTP request traffic.
- **Symptom:** Multiple API discovery issues during implementation.

**std.Thread does NOT have sleep:**
- `std.Thread.sleep` does not exist in Zig 0.16.x.
- `std.Thread` only has: `setName`, `getName`, `getCurrentId`, `getCpuCount`, `spawn`, `getHandle`, `detach`, `join`, `yield`.

**Working fix: Use std.c.nanosleep:**
```zig
var ts: c.timespec = .{
    .sec = @intCast(30),
    .nsec = 0,
};
_ = c.nanosleep(&ts, null);
```

**Zig 0.16 c.timespec uses .sec/.nsec, not .tv_sec/.tv_nsec:**
- The libc `timespec` struct field names differ from traditional C headers.
- Use `.sec` and `.nsec` in Zig 0.16.x, not the traditional `tv_sec`/`tv_nsec`.
- Example: `c.timespec{ .sec = 30, .nsec = 0 }`

**std.Thread.Mutex does NOT exist:**
- `std.Thread.Mutex` is not available in Zig 0.16.x.
- Working fix: Use `std.c.pthread_mutex_t` for cross-platform mutex.

**std.Thread.spawn expects tuple arguments, not pointer:**
```zig
const t = try std.Thread.spawn(
    .{ .stack_size = 65536 },
    heartbeatThread,
    .{&heartbeat_ctx},  // Tuple, not raw pointer
);
t.detach();
```

**Comptime type check for writer pointer conversion:**
- When storing `anytype` writer in a `*anyopaque` field, simple `@ptrCast` fails for value types.
- Working fix: `std.mem.zeroes(c.pthread_mutex_t)` for mutex initialization.
- For writer pointer: `&out_writer` gives address of any value type.

**Files affected:**
- `tovarisch/src/http/heartbeat.zig` — new module with heartbeat thread logic
- `tovarisch/src/http/server.zig` — uses heartbeat module

**Promote to field manual?** Yes — threading patterns are common for daemon-style services.

---

## 2026-05-24 — Zig 0.16 std.time API limitations for heartbeat implementation

- **Context:** Attempted to implement periodic heartbeat logging with real-time timestamps in `tovarisch`.
- **Symptom:** `std.time.Timestamp` is not available in Zig 0.16.x; `std.time.nanoTimestamp()` does not exist; `std.io.FixedBufferStream` is not available.
- **Failed assumptions:**
  - `std.time.Timestamp` exists as a type (it does not in Zig 0.16.x)
  - `std.time.now()` is available for timestamp generation
  - `std.io.FixedBufferStream` exists as a helper for test writers
- **Working fix:**
  - Used a static placeholder timestamp (`"2026-05-24T00:00:00Z"`) for the heartbeat log `ts` field
  - Implemented custom `TestWriter` struct for unit tests instead of `std.io.FixedBufferStream`
  - Replaced counter-based approximation with detached heartbeat thread using `std.c.nanosleep`
- **Recommendation:** Document these as known limitations in zig-0.16-field-manual.md.
- **Promote to field manual?** Yes — these are common patterns that agents will attempt.

---

## 2026-05-24 — rtnetlink NETLINK_ROUTE socket implementation lessons

- **Context:** Implementing `discoverPrivateAddresses()` in `tovarisch/src/net/linux_addr.zig` using `AF_NETLINK` socket for live interface address discovery.

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

### Attribute parsing: buffer offsets, not pointers
- `rtattr.rta_len` includes header size; attribute data starts at `pos + @sizeOf(rtattr)`.
- Use explicit bounds checking: `if (data_end >= data_start + 4)` for IPv4.
- `@memcpy` with exact length: `@memcpy(address_octets[0..4], buffer[data_start .. data_start + 4])`.

### IFA_ADDRESS vs IFA_LOCAL fallback
- IPv4 addresses come in `IFA_ADDRESS` (peer/destination) and `IFA_LOCAL` (local interface).
- Parse `IFA_ADDRESS` first, then fall back to `IFA_LOCAL` if not found.

### IFA_LABEL for interface names
- Interface names (eth0, wg0, lo) appear in `IFA_LABEL` attribute.
- `parseLabel()` extracts null-terminated strings from netlink buffer.

### rtnetlink constants bug
- `RTM_GETADDR` = 22 (not 20), `NLM_F_DUMP` = 0x300 (not 0x003).

**Files affected:** `tovarisch/src/net/linux_addr.zig`, `tovarisch/src/net/linux_addr_parse.zig`
**Promote to field manual:** Yes.

---

## 2026-05-24 — Per-Recv Timeout: Bounded Loop Alone Is Insufficient

- **Context:** GitHub CI hangs in `zig build test` — individual `std.c.recv()` blocks forever.
- **Root cause:** Bounded outer loop doesn't prevent individual recv() from blocking.

**Working fix — per-recv timeout using std.c.pollfd:**
```zig
fn waitReadable(sock: c_int) AddrError!void {
    var fds = [_]std.c.pollfd{.{ .fd = sock, .events = 0x001, .revents = 0 }};
    const rc = std.c.poll(&fds, 1, 250); // 250ms timeout
    if (rc <= 0) return error.RecvFailed;
}

try waitReadable(sock);
const recv_result = std.c.recv(sock, @ptrCast(&buffer), buffer.len, 0);
```

**Defense-in-depth:** Per-recv timeout + bounded loop + fallback JSON.

**Files affected:** `tovarisch/src/net/linux_addr.zig`

---

## 2026-05-24 — Raw u8 buffers must NOT be @alignCast into extern structs

- **Symptom:** `panic: incorrect alignment` when casting raw `[]u8` to `nlmsghdr`.
- **Working fix:** Copy bytes into aligned local structs:
```zig
fn readStruct(comptime T: type, bytes: []const u8) AddrError!T {
    if (bytes.len < @sizeOf(T)) return error.InvalidMessage;
    var value: T = undefined;
    @memcpy(std.mem.asBytes(&value), bytes[0..@sizeOf(T)]);
    return value;
}
```

---

## 2026-05-24 — `std.os.linux.timespec`: `.sec`/`.nsec`, not `.tv_sec`/`.tv_nsec`

- **Doctrine:** Do not assume libc field names for Zig stdlib OS structs.
- **Files affected:** `tovarisch/src/metrics_state.zig`

---

## 2026-05-23 — c.sockaddr.in is nested inside c.sockaddr, not a direct c member

- **Symptom:** `error: root source file struct 'c' has no member named 'sockaddr_in'`
- **Working fix:** Use `c.sockaddr.in` which is platform-adaptive:
```zig
var addr: c.sockaddr.in = std.mem.zeroes(c.sockaddr.in);
addr.family = c.AF.INET;
addr.port = std.mem.nativeToBig(u16, self.config.port);
```
- **Files affected:** `tovarisch/src/http/server.zig`

---

## 2026-05-23 — `@memcpy arguments alias` panic in chained `std.fmt.bufPrint()` calls

- **Symptom:** `panic: @memcpy arguments alias` when same buffer used as destination and format arg.
- **Working fix:** Use distinct buffers:
```zig
var parent_buf: [4096]u8 = undefined;
var child_buf: [4096]u8 = undefined;
const parent = try std.fmt.bufPrint(&parent_buf, "{s}/{s}", .{ base, iface });
const child = try std.fmt.bufPrint(&child_buf, "{s}/statistics", .{parent});
```
- **Files affected:** `tovarisch/src/net/linux_interface_stats_tests.zig`

---

## 2026-05-23 — `@hasDecl` with pointer dereference and writer type compatibility

- **Symptom:** `error: cannot dereference non-pointer type 'VoidWriter'` — `@hasDecl` fails for value types.
- **Working fix:** Comptime type check:
```zig
if (comptime @TypeOf(out_writer) == *logging.BufferedWriter) {
    // No-op: BufferedWriter doesn't have flush
} else {
    out_writer.flush() catch {};
}
```
- **Files affected:** `tovarisch/src/http/server.zig`, `tovarisch/src/cli/commands.zig`

---

## 2026-05-23 — `std.c.opendir()` returns optional C pointer

- **Symptom:** `error: expected type '*c.DIR', found '?*c.DIR'`
- **Working fix:** Unwrap with `orelse`:
```zig
const dir = std.c.opendir(sysfs_root) orelse return error.SkipZigTest;
defer _ = std.c.closedir(dir);
```
- **Files affected:** `tovarisch/src/net/linux_stats_tests.zig`

---

## 2026-05-23 — Linux open flags: ACCMODE, not boolean `.WRONLY`

- **Symptom:** `error: no field named 'WRONLY' in struct 'os.linux.O__struct_...'`
- **Working fix:** Use `.ACCMODE = std.posix.ACCMODE.WRONLY`.
- **Files affected:** `tovarisch/src/net/linux_stats.zig`

---

## 2026-05-22 — Process entry point confirmed working

- `pub fn main(init: std.process.Init) !void` works as documented.
- `init.arena.allocator()` provides the arena allocator.
- `init.minimal.args.toSlice(arena)` works for argument extraction.
- `Io.File.Writer.init(.stdout(), init.io, buf[0..])` works for stdout/stderr writers.
- **Files affected:** `tovarisch/src/main.zig`

---

## 2026-05-22 — Build system patterns confirmed

- `.root_module = b.createModule(...)` is the correct pattern in Zig 0.16.
- Module options like `.test_files` are NOT valid in `b.createModule()` options for `addTest`.
- Tests are discovered from all source files in the module; explicit test files not required.

---

## 2026-05-23 — CI Zig dev build lacks `std.process.Init`

- **Symptom:** CI compiler does not have `std.process.Init` (uses `0.16.0-dev.732+2f3234c76`).
- **Local verification:** Stable Zig 0.16.0 (macOS) HAS `std.process.Init`.
- **Files affected:** `tovarisch/src/main.zig` (no changes needed — original works on stable)

---

## 2026-05-22 — Io.Dir API not directly usable in status.zig

- **Symptom:** `std.fs.cwd()` does not exist in Zig 0.16; `std.Io.Dir.cwd()` requires an `Io` context.
- **Working fix:** `state_dir` check is a placeholder returning `warn` until `Io.Dir` API is fully understood.
- **Files affected:** `tovarisch/src/status.zig`

---

---

## 2026-05-24 — Zig 0.16 environment variable reading

**symptom:** Attempting `std.os.getenv`, `std.process.getenv`, `std.posix.getenv` all fail with "has no member named 'getenv'"

**wrong assumption:** Standard Zig API patterns for environment variable access would be available under common namespaces

**working fix:** Use `std.c.getenv()` to read environment variables:
```zig
if (c.getenv("TOVARISCH_ENABLE_HEARTBEAT_THREAD_UNSAFE") != null) {
    // diagnostic code path
}
```

**files affected:**
- `tovarisch/src/http/server.zig`

**promote to field manual?** No — this is a diagnostic-only pattern; heartbeat env var is temporary

---

## 2026-05-24 — Zig 0.16 opendir() for filesystem checks in status.zig

- **Context:** Implementing `getStateDirCheckForPath()` in `tovarisch/src/status.zig` to replace the placeholder `state_dir` check.
- **Symptom:** `std.c.stat` is not available in Zig 0.16's `std.c` namespace; compilation error.
- **Working fix:** Use `std.c.opendir()` instead of `stat()`:
  ```zig
  const dir = std.c.opendir(c_path);
  if (dir) |d| {
      // Path is a directory - ok
      _ = std.c.closedir(d);
      return Check{ .name = "state_dir", .status = .ok, .detail = "state directory ready" };
  }
  // Check errno for ENOENT/ENOTDIR vs other errors
  const errno = std.c._errno().*;
  const e_noent = @intFromEnum(std.c.E.NOENT);
  const e_notdir = @intFromEnum(std.c.E.NOTDIR);
  if (errno == e_noent or errno == e_notdir) {
      return Check{ .name = "state_dir", .status = .warn, .detail = "state directory not found" };
  }
  return Check{ .name = "state_dir", .status = .unknown, .detail = "state directory inaccessible" };
  ```

- **Limitation:** `opendir()` cannot distinguish between "file exists" (ENOTDIR) and "path doesn't exist" (ENOENT). Without `stat()`, we cannot return `error` for "path is not a directory". The honest behavior is to return `warn` for both cases.

- **Files affected:**
  - `tovarisch/src/status.zig` — new `getStateDirCheckForPath()` function
  - `tovarisch/src/status_tests.zig` — new test file with isolated filesystem tests

- **Promote to field manual?** Yes — filesystem observation patterns are common.

## 2026-05-24 — std.c._errno() returns c_int, not enum

- **Symptom:** `error: incompatible types: 'c_int' and 'c.darwin.E'` when comparing errno directly.
- **Working fix:** Use `@intFromEnum()` to convert enum to integer:
  ```zig
  const errno = std.c._errno().*;
  const e_noent = @intFromEnum(std.c.E.NOENT);
  if (errno == e_noent) { ... }
  ```

- **Files affected:** `tovarisch/src/status.zig`

- **Promote to field manual?** Yes — errno handling is a common pattern.

## 2026-05-24 — std.c.open() mode parameter must be cast to c_uint

- **Symptom:** `error: integer and float literals passed to variadic function must be casted to a fixed-size number type`
- **Working fix:** Cast the mode argument:
  ```zig
  const fd = std.c.open(c_path, std.c.O{ .ACCMODE = std.posix.ACCMODE.WRONLY, .CREAT = true }, @as(c_uint, 0o644));
  ```

- **Files affected:** `tovarisch/src/status_tests.zig`

- **Promote to field manual?** Yes — variadic C function call patterns are common.

---

---

## 2026-05-24 — Zig 0.16 test declarations and doc comments

**symptom:** `error: documentation comments cannot be attached to tests` when using `///` before test declarations.

**wrong assumption:** `/// doc comments` could be used before `test` blocks.

**working fix:** Use `//` regular comments before `test`:
```zig
// Contract test: tunnel_count field exists in output.
test "tunnel contract: tunnel_count field exists" {
    ...
}
```

**files affected:**
- `tovarisch/src/metrics_tunnel_contract_tests.zig`

**promote to field manual?** Yes — test documentation patterns are common.

---

## 2026-05-24 — Zig 0.16 integer overflow on unsigned subtraction

**symptom:** `panic: integer overflow` when decrementing unsigned `usize` counters (e.g., brace-depth) without guards.

**wrong assumption:** Unchecked `brace_depth -= 1` would work for unsigned integers.

**working fix:** Guard before subtracting:
```zig
if (brace_depth > 0) brace_depth -= 1;
```

**files affected:**
- `tovarisch/src/metrics_tunnel_contract_tests.zig`

**promote to field manual?** Yes — unsigned arithmetic guard patterns are common.

---

## 2026-05-24 — Zig 0.16 std.mem.indexOf returns optional usize, not index

**symptom:** Integer arithmetic errors when treating `std.mem.indexOf` result as direct offset without unwrapping.

**wrong assumption:** `std.mem.indexOf` returns the index directly; could use position + 20 directly.

**working fix:** Use `marker.len` pattern with `orelse`:
```zig
const marker = "\"tunnel_interfaces\":[";
const tunnel_start = std.mem.indexOf(u8, slice, marker) orelse return error.MissingField;
const after_bracket = slice[tunnel_start + marker.len..];  // Add marker.len to position
```

**files affected:**
- `tovarisch/src/metrics_tunnel_contract_tests.zig`

**promote to field manual?** Yes — string search offset patterns are common.

---

Recording field notes from Zig 0.16 experiments. Confidence varies; do not promote to field manual until verified with a minimal reproducer.

Old entries have been promoted to `zig-0.16-field-manual.md`. This file tracks experimental observations.


---


