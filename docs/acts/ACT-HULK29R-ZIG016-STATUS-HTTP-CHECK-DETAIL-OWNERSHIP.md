# ACT-HULK29R-ZIG016-STATUS-HTTP-CHECK-DETAIL-OWNERSHIP

## Status

**SUPERSEDED** — 2026-07-08

This ACT is **exonerated** for HTTP /status request-rendering. The live canary disproves the request-leak hypothesis.

**Superseded by:** `ACT-HULK29R-ZIG016-DAEMON-IDLE-BACKGROUND-ANON-GROWTH`

## Original Investigation

Investigated the reported ~136 bytes/request RSS growth under HTTP /status polling. Static code analysis confirmed:

1. **Memory ownership is correctly implemented** in all status rendering paths
2. **`getWgPeersCheck()`** properly sets `owns_detail=true` and `deinitScratchChecks()` frees it
3. **`cliWireguardStatusDiagnosticAttemptWithRunner()`** uses `defer cmd_result.deinit()` on all exit paths
4. **`status_network_diag.zig`** properly handles deinit for all array lists on all paths

## Live Canary Results

### 10k /status Burst Test

```
10k /status burst added only +24 KiB Anonymous/Private_Dirty
≈ 2.46 bytes/request
```

**Conclusion:** Status rendering does NOT leak at request granularity.

### 30-Minute Idle Window (watch/curl stopped)

```
VmRSS     6408 -> 6640 KiB (+232 KiB)
RssAnon   3684 -> 3924 KiB (+240 KiB)
Private_Dirty 3684 -> 3924 KiB (+240 KiB)
Anonymous 3684 -> 3924 KiB (+240 KiB)
VmData    52132 -> 52372 KiB (+240 KiB)
```

**Conclusion:** The growth is **real private anonymous heap growth**, but NOT status-render per-request leakage.

## Rate Analysis

- Growth rate: **~480 KiB/hour**, **~136 bytes/second**
- Earlier ~136 bytes/request was an **artifact of 1 Hz polling** (1 Hz × 136 bytes/second ≈ 136 bytes/request)
- The ~136 bytes/second rate is independent of HTTP request frequency

## Files Changed

- `tovarisch/src/net/wg_cli_facts.zig` - Comment-only clarification of Zig defer semantics
- `tovarisch/src/status_checks.zig` - Comment-only clarification of memory ownership
- `docs/acts/ACT-HULK29R-ZIG016-STATUS-HTTP-CHECK-DETAIL-OWNERSHIP.md` - Superseded by new investigation

## Verification Results

```bash
$ make gate
[gate] PASS

$ make tovarisch-test
(test output with no failures)

$ make tovarisch-build tovarisch-status
{"service":"tovarisch","version":"0.1.2+fe2dc36.dirty",...}
```

## Next Investigation

**See:** `ACT-HULK29R-ZIG016-DAEMON-IDLE-BACKGROUND-ANON-GROWTH`

Target candidates for 1-second background loop leaking ~136 bytes/tick:
- BFD/BGP health/reconnect loop
- WireGuard/tunnel probe loop
- prefix watcher refresh path
- metrics/history ring buffers
- subprocess/socket retry paths
