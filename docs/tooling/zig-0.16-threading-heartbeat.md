# Zig 0.16 Threading and heartbeat lessons

This document captures verified lessons from the detached HTTP heartbeat implementation in `tovarisch/src/http/heartbeat.zig` and its integration in `tovarisch/src/http/server.zig`.

## CRITICAL: pthread_mutex_t initialization

**DO NOT use `std.mem.zeroes()` to initialize `pthread_mutex_t`.**

This is a **fatal error** that causes `unreachable` panics in Zig 0.16 std/Thread.zig. On Linux, a zeroed `pthread_mutex_t` is invalid and causes undefined behavior when the thread tries to use it.

**Correct pattern - use `PTHREAD_MUTEX_INITIALIZER`:**

```zig
pub fn initHeartbeatContext() HeartbeatContext {
    return HeartbeatContext{
        // PTHREAD_MUTEX_INITIALIZER is a compile-time constant with default attributes.
        // This is the portable POSIX approach - no pthread_mutex_init() runtime call needed.
        .mutex = c.PTHREAD_MUTEX_INITIALIZER,
        .uptime_seconds = 0,
        .done = false,
    };
}
```

**Why `std.mem.zeroes()` fails:**
- `pthread_mutex_t` is typically an opaque struct (on Linux, it's an int representation)
- Zeroed mutex is invalid and causes crashes when `pthread_mutex_lock()` is called
- Zig 0.16's `std.Thread` detects this invalid state and hits `unreachable`

**Why `PTHREAD_MUTEX_INITIALIZER` works:**
- POSIX static initializer is a compile-time constant
- Valid for default mutex attributes
- No runtime initialization required
- Portable across Linux and macOS

**Alternative runtime approach:** You can also use `pthread_mutex_init()` for dynamic initialization, but static initialization with `PTHREAD_MUTEX_INITIALIZER` is simpler and preferred for daemon-lifetime contexts.

## Daemon thread startup must be non-fatal

**BANNED: `catch unreachable` around daemon runtime thread setup.**

```zig
// WRONG - causes crash loop under systemd if thread fails
const heartbeat_t = try std.Thread.spawn(...);
heartbeat_t.detach();

// CORRECT - heartbeat failures are non-fatal
heartbeat.initHeartbeatContext() catch |err| {
    // Log error, continue serving HTTP
    return;
};
std.Thread.spawn(...) catch |err| {
    // Log error, continue serving HTTP
    return;
};
```

**Rule:** Heartbeat startup failures must be logged and degraded gracefully. A decorative heartbeat must never crash the leaf daemon.

## Daemon-lifetime heartbeat thread pattern

**Use `std.Thread.spawn(...)` for the heartbeat worker.**

```zig
const heartbeat_t = try std.Thread.spawn(
    .{ .stack_size = 65536 },
    heartbeat.heartbeatThread,
    .{&heartbeat_ctx},
);
heartbeat_t.detach();
```

**Detach only when the daemon lifetime is intentionally process-bound.** The thread survives until the process exits. There is no graceful shutdown path currently.

**Do not approximate heartbeat with an accept-loop counter.** An accept loop only emits when traffic exists. A real heartbeat proves liveness independent of request arrival.

## Blocking sleep

**Use `std.c.nanosleep` for the heartbeat worker.** Zig 0.16 `std.Thread` does not expose a sleep function. This is the portable blocking sleep API:

```zig
var ts: c.timespec = .{
    .sec = @intCast(HEARTBEAT_INTERVAL_SECS),
    .nsec = 0,
};
_ = c.nanosleep(&ts, null);
```

**Interrupted sleep behavior:** `nanosleep` returns immediately with remaining time in the second argument if interrupted by a signal. The current heartbeat implementation ignores the return value and remaining time. This is acceptable for daemon-lifetime threads. If precise timing recovery is needed, capture the remaining time and re-sleep.

## Context lifetime

The heartbeat context is currently stack-owned inside `serveForever()`:

```zig
var heartbeat_ctx = heartbeat.initHeartbeatContext();
const heartbeat_t = try std.Thread.spawn(
    .{ .stack_size = 65536 },
    heartbeat.heartbeatThread,
    .{&heartbeat_ctx},
);
heartbeat_t.detach();
```

**This is an accepted daemon-lifetime shortcut, not a reusable ownership pattern.** `serveForever()` is an infinite daemon loop. Normal lifecycle does not return. The heartbeat thread outlives the stack frame by design.

**Future hazard:** If graceful shutdown appears, heartbeat state must become owned lifecycle state and the thread must be stopped and joined (or otherwise made safe). A `done` flag exists in `HeartbeatContext` for this purpose, but there is currently no code path that sets it.

## Logging interleaving

The heartbeat thread writes directly to stdout via `c.write(1, bytes.ptr, bytes.len)`, while the main server path uses `writeLogRecord()` through a buffered writer. These two paths can interleave.

**Current acceptance:** Each log record is one complete `c.write()` call of a small NDJSON line. This is atomic-enough in practice for the current output volume.

**Future requirement:** If log records grow or multiple background emitters are added, centralize synchronization. Do not let multiple threads write casually without a shared lock or serialized writer.

## Operator contract

- **Startup log** emits after HTTP listen succeeds.
- **Heartbeat** proves liveness independent of request traffic (every 30 seconds).
- **`--statonly` or future quiet modes** must define whether heartbeat logs are suppressed, compacted, or replaced.

## Verification

Run at minimum:

```bash
make tovarisch-build
make tovarisch-test
make tovarisch-status
make gate
```

There is currently no heartbeat-specific smoke or grep test in the scripts directory. Do not claim coverage for real timing behavior unless there is an actual deterministic test.

## Key imports for threading

```zig
const std = @import("std");
const c = std.c;

// Thread spawn with explicit stack size
const thread = try std.Thread.spawn(
    .{ .stack_size = 65536 },  // 64KB stack for heartbeat worker
    heartbeat.heartbeatThread,
    .{&heartbeat_ctx},
);
thread.detach();

// Mutex for shared state
_ = c.pthread_mutex_lock(&ctx.mutex);
// ... protected access ...
_ = c.pthread_mutex_unlock(&ctx.mutex);

// Blocking sleep
var ts: c.timespec = .{ .sec = 30, .nsec = 0 };
_ = c.nanosleep(&ts, null);
