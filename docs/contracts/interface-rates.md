# Interface Traffic Rates Contract

## Purpose

This document defines the rate calculation behavior for private interface traffic metrics.

## Overview

Rates are derived from two cumulative counter samples. The rate calculation module (`rates.zig`) is intentionally pure: it performs no wall-clock access, no OS/sysfs access, and no HTTP wiring.

## Rate Calculation Behavior

### Data Flow

```
InterfaceCounterSample (previous) + InterfaceCounterSample (current)
                              |
                              v
                    rates.calculateRate()
                              |
                              v
                ?InterfaceRate (null if unavailable)
```

### Return null (rate unavailable) when:

1. **First sample**: No previous sample exists (null previous)
2. **Non-positive elapsed time**: `current.sampled_at_ms <= previous.sampled_at_ms`
3. **Sub-second elapsed time**: `elapsed_ms < 1000` (cannot derive meaningful per-second rate)
4. **Counter reset detected**: Any current counter is less than its previous counter

### Counter Reset Handling

A counter reset occurs when a current value is less than its previous value. This can happen due to:
- Interface restart
- System reboot
- Kernel counter overflow reset
- Interface removal/recreation

When a reset is detected, `calculateRate()` returns `null` because no valid rate can be computed.

### Rate Computation (when available)

- `window_seconds`: Integer elapsed seconds between samples (`(current.sampled_at_ms - previous.sampled_at_ms) / 1000`)
- Deltas: `current.counter - previous.counter` (guaranteed non-negative by reset detection)
- Per-second rates: `delta / window_seconds` (integer division)

### Edge Cases

| Scenario | Behavior |
|----------|----------|
| Elapsed time < 1000ms | `null`; rate unavailable (sub-second samples have no rate) |
| Very small deltas | Per-second rate may be 0 (integer truncation) |
| Large counters | Uses u64 arithmetic; handles values up to 2^64-1 |

## Data Structures

### InterfaceCounterSample

```zig
pub const InterfaceCounterSample = struct {
    name: []const u8,
    rx_bytes: u64,
    tx_bytes: u64,
    rx_packets: u64,
    tx_packets: u64,
    sampled_at_ms: i64,
};
```

### InterfaceRate

```zig
pub const InterfaceRate = struct {
    window_seconds: u64,
    rx_bytes_delta: u64,
    tx_bytes_delta: u64,
    rx_packets_delta: u64,
    tx_packets_delta: u64,
    rx_bytes_per_second: u64,
    tx_bytes_per_second: u64,
    rx_packets_per_second: u64,
    tx_packets_per_second: u64,
};
```

## Scope and Limitations

### This Module Does NOT Include:

- ❌ `/metrics.json` wiring (implemented in `metrics_state.zig`)
- ❌ HTTP server integration (implemented in `http/routes.zig`)
- ❌ Counter collection (belongs in `linux_stats.zig`)

### This Module Includes:

- ✅ Pure rate calculation function
- ✅ Comprehensive unit tests (12 test cases)
- ✅ Counter reset detection
- ✅ Integer-only arithmetic
- ✅ Edge case handling (zero/negative elapsed, divide-by-zero prevention)

## ACT 4: Live Metrics DTO Wiring

**Status**: ✅ **Implemented**

ACT 4 wired the DTO format into the live `/metrics.json` route:
- Live `/metrics.json` now uses the DTO row shape
- Every interface row includes `"rate":null`
- `metrics_version` updated to `"0.2"`

## ACT 5: Persistent Sampler State Wiring

**Status**: ✅ **Implemented**

ACT 5 wired persistent sampler state across `/metrics.json` requests:

### Components

- **MetricsState** (`metrics_state.zig`): Owns InterfaceSampler, persists across requests
- **ServerState** (`http/server.zig`): Owns MetricsState, initialized on server start
- **Route handler** (`http/routes.zig`): Uses state pointer for metrics rendering

### Ownership Model

```
Server (serveForever)
  └── ServerState
        └── MetricsState
              └── InterfaceSampler
                    └── previous: StringHashMapUnmanaged(StoredSample)
```

### Clock Source

Timestamps are generated using `std.os.linux.clock_gettime(CLOCK_REALTIME)` — wall-clock seconds and nanoseconds converted to milliseconds.

- Sub-second precision available (nanoseconds in the timestamp)
- Rates become available only when elapsed whole seconds are positive (>= 1000ms)
- On non-Linux platforms, falls back to returning 0 (tests inject explicit timestamps)

Rationale: Wall-clock is sufficient because elapsed time is computed within the sampler (`current.sampled_at_ms - previous.sampled_at_ms`). Monotonic time is not used because it doesn't survive process restarts and doesn't provide useful absolute reference for debugging.

### Behavior by Scenario

| Scenario | Behavior |
|----------|----------|
| First request | `rate: null` for all interfaces (no previous sample) |
| Second request with valid elapsed | `rate: { ... }` for interfaces with previous sample |
| Counter reset | `rate: null` for that interface only |
| New interface | `rate: null` (no previous sample) |
| Reappearing interface | `rate: null` (previous state was cleared) |
| Sub-second elapsed | `rate: null` (cannot derive meaningful rate) |

### First Request Example

```json
{
  "service": "tovarisch",
  "version": "0.1.1",
  "metrics_version": "0.2",
  "private_interfaces": [{
    "name": "eth0",
    "rx_bytes": 123,
    "tx_bytes": 456,
    "rx_packets": 7,
    "tx_packets": 8,
    "rate": null
  }],
  "notes": [
    "rate is null until a previous sample exists",
    "interface counters are cumulative",
    "IPv4 private interfaces only; IPv6 is deferred"
  ]
}
```

### Later Request with Rate Example

```json
{
  "service": "tovarisch",
  "version": "0.1.1",
  "metrics_version": "0.2",
  "private_interfaces": [{
    "name": "eth0",
    "rx_bytes": 3123,
    "tx_bytes": 6456,
    "rx_packets": 37,
    "tx_packets": 48,
    "rate": {
      "window_seconds": 30,
      "rx_bytes_delta": 3000,
      "tx_bytes_delta": 6000,
      "rx_packets_delta": 30,
      "tx_packets_delta": 40,
      "rx_bytes_per_second": 100,
      "tx_bytes_per_second": 200,
      "rx_packets_per_second": 1,
      "tx_packets_per_second": 1
    }
  }],
  "notes": [
    "rate is null until a previous sample exists",
    "interface counters are cumulative",
    "IPv4 private interfaces only; IPv6 is deferred"
  ]
}
```

## Future Work

The following work is deferred to future ACTs:

1. ~~**ACT 1**: Pure rate calculation~~ ✅ **Implemented in `rates.zig`**
2. ~~**ACT 2**: InterfaceSampler state management~~ ✅ **Implemented in `interface_sampler.zig`**
3. ~~**ACT 4**: Wire DTO format into live `/metrics.json` with null rates~~ ✅ **Implemented**
4. ~~**ACT 5**: Wire persistent sampler state across HTTP requests for live rates~~ ✅ **Implemented in `metrics_state.zig`**
5. **IPv6 support**: Extend to handle IPv6 interface counters

## Module Location

- **Pure rate calculation**: `tovarisch/src/net/rates.zig`
- **Sampler state**: `tovarisch/src/net/interface_sampler.zig`
- **Metrics state**: `tovarisch/src/metrics_state.zig`
- **HTTP routes**: `tovarisch/src/http/routes.zig`
- **HTTP server**: `tovarisch/src/http/server.zig`
- **Tests**: Inline in `rates.zig` (12 test cases), `metrics_state_tests.zig` (14 test cases)
- **Test wiring**: `tovarisch/src/test_all.zig`

## References

- [tovarisch-http-v0.md](./tovarisch-http-v0.md) — HTTP contract (rates implemented)
- [ACT 5 Epic](../epics/act-5-sysfs-collector.md) — sysfs collection context
