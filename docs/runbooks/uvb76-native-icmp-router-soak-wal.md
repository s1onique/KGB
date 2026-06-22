# WAL: UVB-76 Native ICMP Router Soak

**Date:** 2026-06-22
**Status:** Native ICMP implemented; pending router soak

## Summary

This WAL documents the native ICMP backend implementation for UVB-76 and the router soak procedure to verify it avoids the OS ping SIGSEGV crash.

## Prior OS Ping Crash Signature

The confirmed crash path was:

```text
ICMPClient.probeTarget
  -> OSPingBackend.Ping
  -> PingOSWithRunner
  -> BoundedCommandRunner.Run
  -> BoundedCommandRunner.copyBoundedWithBuf
  -> runtime.makeslice
  -> runtime.memclrNoHeapPointers
  -> fatal SIGSEGV
```

This occurred on ASUS RT-AX88U / linux arm64 under continuous per-second ICMP probing.

## Native ICMP Backend Configuration

### Example Configuration

```json
{
  "latency": {
    "icmp": {
      "enabled": true,
      "backend": "native",
      "interval_seconds": 1,
      "timeout_seconds": 3
    }
  }
}
```

### Explicit OS Ping Fallback

If native ICMP socket fails (e.g., permission denied without CAP_NET_RAW), explicitly configure OS ping:

```json
{
  "latency": {
    "icmp": {
      "enabled": true,
      "backend": "os_ping",
      "interval_seconds": 1,
      "timeout_seconds": 3
    }
  }
}
```

**Important:** Native backend does NOT silently fall back to OS ping. If native fails and `backend` is not explicitly set to `os_ping`, the daemon will fail at startup with an actionable error message.

## Telemetry Fields

Verify native ICMP telemetry via the status endpoint:

```bash
curl -sk https://127.0.0.1:8443/api/v1/status | jq '.icmp_native'
```

Expected output:

```json
{
  "backend": "native",
  "sent": 123,
  "received": 123,
  "timeouts": 0,
  "socket_open_errors": 0,
  "permission_errors": 0,
  "parse_errors": 0,
  "unmatched_replies": 0,
  "last_rtt_ms": 12,
  "last_error_class": ""
}
```

## Router Soak Runbook

### Prerequisites

1. SSH access to router (ASUS RT-AX88U or similar ARM64 router)
2. UVB-76 binary with native ICMP support installed
3. Configuration with `backend: "native"` (or no backend specified, defaults to native on Linux)
4. Access to router logs (via `logread` or `/tmp/uvb76.log`)

### Soak Procedure

#### Pre-flight Check

```bash
# Verify binary has native ICMP support
/opt/bin/uvb76 --help 2>&1 | head -5

# Verify config uses native backend
cat /opt/etc/uvb76/uvb76.json | jq '.latency.icmp.backend'

# Check initial status
curl -sk https://127.0.0.1:8443/api/v1/status | jq '.icmp_native'
```

Expected pre-flight:
- `backend` should be `"native"` or null (defaults to native on Linux)
- `sent`, `received` should start at 0

#### 30-Minute Soak Checkpoint

```bash
# Check ICMP telemetry
curl -sk https://127.0.0.1:8443/api/v1/status | jq '.icmp_native'

# Check for any errors
curl -sk https://127.0.0.1:8443/api/v1/status | jq '.icmp_native | select(.permission_errors > 0)'

# Check logs for crashes
logread | grep -i "SIGSEGV\|signal\|crash\|panic" || echo "No crash detected"

# Verify no os/exec ping processes
ps | grep -c "[p]ing" || echo "No ping processes"
```

Pass criteria at 30 minutes:
- [ ] `sent` > 0 (probes are being sent)
- [ ] `permission_errors` == 0 (socket opened successfully)
- [ ] No SIGSEGV in logs
- [ ] No restart count increase
- [ ] `ps` shows no ping processes (proves native backend is used)

#### 60-Minute Soak Checkpoint

Repeat 30-minute checks plus:
- [ ] `sent` >= 3600 (1 probe/second for 60 minutes)
- [ ] `received` + `timeouts` approximately equals `sent`
- [ ] Memory stable (check via `ps` RSS column)

#### 4-Hour Soak Checkpoint

Repeat 60-minute checks plus:
- [ ] Service uptime > 4 hours
- [ ] No accumulated errors
- [ ] Latency samples continue arriving

### Pass/Fail Criteria

**Pass:**
- Service stays alive
- ICMP samples continue arriving
- No `os/exec ping` activity in logs or `ps` output
- Native telemetry shows sent/received or sent/timeouts
- No new SIGSEGV
- No restart count increase
- `backend` field in telemetry is `"native"`

**Fail:**
- Any SIGSEGV or unexpected restart
- `permission_errors` > 0 (socket not opening)
- Silent fallback to OS ping detected (`ps | grep ping` shows activity)
- Memory leak suspected (RSS growth > 50%)

### Evidence Collection

On pass, capture:

```bash
# Final telemetry snapshot
curl -sk https://127.0.0.1:8443/api/v1/status > /tmp/uvb76-final-status.json

# Uptime
curl -sk https://127.0.0.1:8443/api/v1/status | jq '.started_at'

# Process info
ps | grep uvb76

# Log excerpt (last 100 lines)
logread | tail -100 > /tmp/uvb76-log-excerpt.txt
```

## Files Changed

- `uvb76/probe/native_icmp.go` - NativeICMPBackend implementation
- `uvb76/probe/native_icmp_test.go` - Unit tests
- `uvb76/config/latency.go` - Backend selection config
- `uvb76/probe/icmp.go` - Backend selection logic in ICMPClient
- `uvb76/main.go` - ICMP client initialization with error handling
- `uvb76/server/server.go` - Native ICMP telemetry in status API

## Verification Commands

```bash
# Build
cd uvb76 && go build -o uvb76 .

# Run tests
go test ./probe/... -v

# Race detector tests
go test -race ./probe/... -count=3

# Quality gate
make gate
```

## Status

```
[Native ICMP implemented / pending router soak]
```

Router soak evidence is required before claiming the crash is fixed.

## Future Work

- [ ] Run router soak on ASUS RT-AX88U
- [ ] Document soak evidence in this WAL
- [ ] Update status to `[Closed with native ICMP router soak evidence]`
