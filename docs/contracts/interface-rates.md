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

### This ACT Does NOT Include:

- ❌ `/metrics.json` wiring (future ACT)
- ❌ HTTP server integration (future ACT)
- ❌ Counter collection (belongs in `linux_stats.zig`)
- ❌ Sample storage/state management (future ACT)
- ❌ IPv6 rate support (deferred)

### This ACT Includes:

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
- Notes updated to reflect null rates until sampler state is wired

## Future Work

The following work is deferred to future ACTs:

1. ~~**ACT 2**: Add sampler state that matches interfaces by name and produces `rate: null | InterfaceRate` per current counter row~~ ✅ **Implemented in `interface_sampler.zig`**
2. ~~**ACT 4**: Wire DTO format into live `/metrics.json` with null rates~~ ✅ **Implemented**
3. **ACT 5**: Wire persistent sampler state across HTTP requests for live rates
4. **IPv6 support**: Extend to handle IPv6 interface counters

## Sampler State

A sampler layer (`interface_sampler.zig`) provides stateful rate calculation across multiple update cycles:

### Behavior

| Scenario | Behavior |
|----------|----------|
| First observation for any interface | `rate = null` (no previous sample) |
| Second+ observation with valid elapsed/counters | `rate = InterfaceRate` |
| Newly appearing interface | `rate = null` (treated as first observation) |
| Reappearing interface after disappearance | `rate = null` (previous state was cleared) |
| Disappeared interface | Removed from previous state after update |
| Counter reset for one interface | `rate = null` for that interface only; other interfaces unaffected |
| Output order | Follows current input order |

### Interface Matching

Interfaces are matched by **exact name** comparison. No prefix matching or fuzzy logic.

### Ownership Model

- Sampler owns duplicated map keys (interface names duplicated on insertion)
- `update()` returns `[]SampledInterface` owned by caller with separate caller-owned name duplicates
- Caller passes `[]const rates.InterfaceCounterSample` (does not need to outlive sampler)
- Call `deinit()` to free all sampler-owned map keys

### Data Structures

#### SampledInterface

```zig
pub const SampledInterface = struct {
    sample: rates.InterfaceCounterSample,
    rate: ?rates.InterfaceRate,
};
```

#### InterfaceSampler

```zig
pub const InterfaceSampler = struct {
    pub fn init(allocator: std.mem.Allocator) InterfaceSampler
    pub fn deinit(self: *InterfaceSampler) void
    pub fn update(
        self: *InterfaceSampler,
        current: []const rates.InterfaceCounterSample,
    ) ![]SampledInterface
};
```

### Scope and Limitations

**This ACT (2) Does NOT Include:**

- ❌ `/metrics.json` wiring (future ACT)
- ❌ HTTP server integration (future ACT)
- ❌ Sysfs counter collection (belongs in `linux_stats.zig`)

**This ACT Includes:**

- ✅ Sampler state that matches interfaces by name
- ✅ First/new/reappearing interfaces return null rate
- ✅ Existing interfaces get rates when valid
- ✅ Disappeared interfaces are forgotten
- ✅ Counter reset isolated per interface
- ✅ Output order follows current input order
- ✅ Safe name lifetime handling (sampler duplicates names)

## Module Location

- **Source**: `tovarisch/src/net/rates.zig`
- **Tests**: Inline in `rates.zig` (12 test cases)
- **Test wiring**: `tovarisch/src/test_all.zig`

## References

- [tovarisch-http-v0.md](./tovarisch-http-v0.md) — HTTP contract (rates to be added in future ACT)
- [ACT 5 Epic](../epics/act-5-sysfs-collector.md) — sysfs collection context
