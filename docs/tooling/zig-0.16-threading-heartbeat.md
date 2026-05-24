# Zig 0.16 Threading and heartbeat lessons

This document captures verified lessons from the heartbeat implementation in `tovarisch/src/http/heartbeat.zig` and its integration in `tovarisch/src/http/server.zig`.

## ✅ CURRENT STATUS: Heartbeat Enabled (Default Stack)

**Threaded heartbeat is ENABLED** with default stack after root cause was identified:
- Explicit `.stack_size = 65536` (64 KiB) caused crashes on Linux/glibc release target
- Default stack (`.{}`) works reliably in both serve-context and standalone tests
- Root cause confirmed via `tovarisch thread-smoke` variant 3

See: [R-009 in docs/security/accepted-risks.md](../security/accepted-risks.md)

---

## Historical design: Local state, no shared context (DISABLED)

The heartbeat thread was designed to own its state locally:

```zig
pub fn heartbeatThread() void {
    var uptime_seconds: u64 = 0;

    while (true) {
        // sleep...
        uptime_seconds += HEARTBEAT_INTERVAL_SECS;
        emitHeartbeatToFd(uptime_seconds);
    }
}

// Spawn with no arguments
std.Thread.spawn(
    .{ .stack_size = 65536 },
    heartbeat.heartbeatThread,
    .{},
) catch |err| { /* non-fatal */ };
```

**No mutex, no shared context, no `@constCast`.** For a decorative daemon-lifetime heartbeat, this was the correct engineering choice.

**PROBLEM**: `std.Thread.spawn` itself crashes on production Linux even with this clean design.

---

## Historical lesson: pthread_mutex_t initialization

The following section is preserved as a historical Zig/POSIX lesson. It describes a trap that was encountered before the simplified design was adopted.

### DO NOT use `std.mem.zeroes()` to initialize `pthread_mutex_t`

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

---

## Daemon thread startup must be non-fatal

**BANNED: `catch unreachable` around daemon runtime thread setup.**

```zig
// WRONG - causes crash loop under systemd if thread fails
const heartbeat_t = try std.Thread.spawn(...);
heartbeat_t.detach();

// CORRECT - heartbeat failures are non-fatal
std.Thread.spawn(...) catch |err| {
    // Log error, continue serving HTTP
    return;
};
```

**Rule:** Heartbeat startup failures must be logged and degraded gracefully. A decorative heartbeat must never crash the leaf daemon.

## Daemon-lifetime heartbeat thread pattern

**Use `std.Thread.spawn(...)` for the heartbeat worker.**

```zig
const thread = try std.Thread.spawn(
    .{ .stack_size = 65536 },
    heartbeat.heartbeatThread,
    .{},  // No context argument - thread owns local state
);
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

**The heartbeat thread owns its state locally.** There is no shared context between the main thread and the heartbeat thread.

```zig
pub fn heartbeatThread() void {
    var uptime_seconds: u64 = 0;  // Local state
    // ...
}
```

This eliminates:
- Stack-owned cross-thread state coupling
- `@constCast` from const context
- Mutex complexity for a decorative feature

**Background decorative threads must not share mutable stack-owned state unless there is a real shutdown lifecycle.** The simplified design is correct for v0.

**Future hazard:** If graceful shutdown appears, the thread still needs to be stopped (join or signal). With local state, there's no shared context to corrupt. The shutdown mechanism would be signal-based thread cancellation, not mutex-protected shared state.

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

// Thread spawn with explicit stack size (no context argument)
// The thread must be detached for daemon-lifetime operation.
if (std.Thread.spawn(
    .{ .stack_size = 65536 },  // 64KB stack for heartbeat worker
    heartbeat.heartbeatThread,
    .{},  // Empty tuple - thread owns local state
)) |thread| {
    // Detach the thread for daemon-lifetime operation.
    // The thread runs until process exit; detach() allows it to continue
    // independently without blocking the main thread on join.
    thread.detach();
} else |err| {
    // Log error, continue serving HTTP
};

// Blocking sleep
var ts: c.timespec = .{ .sec = 30, .nsec = 0 };
_ = c.nanosleep(&ts, null);
