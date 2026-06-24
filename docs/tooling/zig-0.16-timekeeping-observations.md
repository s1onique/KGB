# Zig 0.16 Timekeeping Observations

---

## 2026-06-24 — std.time.monoTime() not available in Zig 0.16.0

- **Symptom:** `std.time.monoTime()` is not available in Zig 0.16.0's `std.time` namespace.
- **Wrong assumption:** Attempted to use `std.time.monoTime()` for real monotonic time in lab event timestamps.
- **Working fix:** Use caller-supplied elapsed milliseconds.
  - Heartbeat thread passes `uptime_seconds * 1000` to `emit()`.
  - This gives real elapsed time without depending on `std.time.monoTime()`.
  - Count-based derivation was rejected: it creates false correlations between memory steps and event timing.
  - Design constraint: elapsed time must come from a clock seam, not from event count.
- **Files affected:** `tovarisch/src/runtime/lab_events.zig`, `tovarisch/src/http/heartbeat.zig`
- **Promote to field manual?** Yes — timing patterns are common, and the false-correlation risk from count-based timing should be documented.

---

## Key Rule for Native Lab Event Timing

Native lab event elapsed time must come from a caller-supplied clock seam, not from event count.

Why count-based timing is rejected:
- Memory steps or other periodic events may batch or coalesce
- Event count does not map linearly to wall-clock time
- Count-derived timing creates false correlations between memory steps and event timing
- A clock seam (e.g., caller-supplied uptime_millis) provides the ground truth

Preferred pattern:
```zig
// Heartbeat thread knows the real elapsed time
const elapsed_millis = @as(u32, @intCast(uptime_seconds * 1000));

// Pass to emitter for accurate event timestamps
emitter_ptr.emitHeartbeatStart(elapsed_millis);
```

Recording field notes from Zig 0.16 experiments. Confidence varies; do not promote to field manual until verified with a minimal reproducer.
