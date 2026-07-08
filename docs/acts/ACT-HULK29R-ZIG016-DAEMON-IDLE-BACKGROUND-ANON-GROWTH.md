# ACT-HULK29R-ZIG016-DAEMON-IDLE-BACKGROUND-ANON-GROWTH

## Status

**ACTIVE INVESTIGATION** — 2026-07-08

## Goal

Identify the source of ~480 KiB/hour (~136 bytes/second) private anonymous heap growth in `tovarisch` during idle periods when all external watchers and HTTP clients are stopped.

## Evidence Summary

### From: ACT-HULK29R-ZIG016-STATUS-HTTP-CHECK-DETAIL-OWNERSHIP (superseded)

The HTTP /status endpoint was exonerated:
- 10k /status burst: +24 KiB Anonymous/Private_Dirty (~2.46 bytes/request)
- This rules out request rendering as the leak source

### 30-Minute Idle Window Measurements

With watch/curl stopped, observed:

| Metric | Start | End | Delta |
|--------|-------|-----|-------|
| VmRSS | 6408 KiB | 6640 KiB | +232 KiB |
| RssAnon | 3684 KiB | 3924 KiB | +240 KiB |
| Private_Dirty | 3684 KiB | 3924 KiB | +240 KiB |
| Anonymous | 3684 KiB | 3924 KiB | +240 KiB |
| VmData | 52132 KiB | 52372 KiB | +240 KiB |

### Rate Calculation

```
240 KiB / 1800 seconds = 0.133 KiB/second = ~136 bytes/second
```

### Hypothesis

The ~136 bytes/request observed in 1 Hz polling was an **artifact of 1 Hz polling rate**:
- 1 Hz polling × ~136 bytes/second ≈ ~136 bytes/request

The actual leak is a **background tick** (~1 second interval) unrelated to HTTP requests.

## Investigation Candidates

Instrument/disable candidates one at a time:

### 1. BFD/BGP Health Loop
- [ ] BFD session state machine
- [ ] BGP peer health checks
- [ ] Reconnect/retry logic
- [ ] Keepalive timers

### 2. WireGuard/Tunnel Probe Loop
- [ ] wg show endpoint polling
- [ ] Interface state checks
- [ ] Handshake timing checks

### 3. Prefix Watcher Refresh Path
- [ ] Route table watchers
- [ ] Network namespace checks
- [ ] ARP/NDP table polling

### 4. Metrics/History Ring Buffers
- [ ] Timestamped event buffer
- [ ] Latency histogram allocations
- [ ] Reachability status ring

### 5. Subprocess/Socket Retry Paths
- [ ] CLI runner retry logic
- [ ] Network socket reconnect
- [ ] UDP/TCP health probes

## Investigation Strategy

1. **Instrument candidates one at a time** with tick counters and byte meters
2. **Disable candidates selectively** to isolate the leak
3. **Use memory attribution matrix** to track allocations per subsystem
4. **Profile with valgrind massif** if available on target platform

## Next Steps

- [ ] Read tovarisch main event loop to enumerate tick sources
- [ ] Add per-subsystem memory counters
- [ ] Run with individual subsystems disabled
- [ ] Capture memory attribution matrix at idle baseline and +30min

## References

- Supersedes: `ACT-HULK29R-ZIG016-STATUS-HTTP-CHECK-DETAIL-OWNERSHIP`
- Original evidence: `docs/evidence/memory-lab/`
- Memory tools: `scripts/lab_memory_attribution_matrix.py`
