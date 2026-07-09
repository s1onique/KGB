# Startup Readiness Diagnostics

## Overview

This document describes the startup phase instrumentation in `tovarisch` for diagnosing gaps between process start and HTTP readiness.

## Problem Statement

Systemd's "Started" journal line uses process-start semantics, not application-readiness semantics. For Type=simple services (the default), systemd considers startup complete when the process forks/execs, regardless of when the HTTP accept loop begins accepting connections.

A production symptom showed a 68-second gap between BFD start and HTTP readiness that was not diagnosable from existing logs.

## Solution: Startup Phase Tracing

The `startup_trace.zig` module provides structured startup phase instrumentation:

- **PhaseGuard**: RAII-style timer for a single startup phase
- **StartupTracer**: Tracks total startup duration from process entry to readiness

### Startup Phases

| Phase | Description |
|-------|-------------|
| `config_load` | Config file loading and parsing (TOML [server].listen override) |
| `bfd_start` | BFD configuration loading and loop startup |
| `bgp_load` | BGP configuration loading, parsing, and validation |
| `bgp_runtime_start` | BGP FSM thread spawn and initial peer handshakes |
| `lab_and_net_diag_config_parse` | Lab and network diagnostics config parsing |
| `http_bind` | HTTP server socket creation and bind |
| `http_accept_loop` | HTTP accept loop startup (emits startup_ready) |

## Events

### startup_phase_started
Emitted at the beginning of each phase.

```json
{"level":"info","event":"startup_phase_started","service":"tovarisch","version":"...","fields":{"phase":"bfd_start"}}
```

### startup_phase_finished
Emitted when a phase completes within the threshold (default 5 seconds).

```json
{"level":"info","event":"startup_phase_finished","service":"tovarisch","version":"...","fields":{"phase":"bfd_start","duration_ms":125}}
```

### startup_phase_slow
Emitted when a phase exceeds the slow threshold. Logged at `error` level to draw attention.

```json
{"level":"error","event":"startup_phase_slow","service":"tovarisch","version":"...","fields":{"phase":"bfd_start","duration_ms":68000,"threshold_ms":5000}}
```

### startup_ready
Canonical application readiness event, emitted after HTTP accept loop starts.

```json
{"level":"info","event":"startup_ready","service":"tovarisch","version":"...","fields":{"ready_kind":"http_accept_loop","startup_duration_ms":1342,"bind_address":"10.149.149.1","port":8317}}
```

## Diagnosing Startup Gaps

### Quick Diagnosis

```bash
journalctl -u tovarisch.service -n 200 -o short-iso | \
  grep -E 'startup_phase|startup_ready|http_server_listening'
```

### Full Timeline

```bash
journalctl -u tovarisch.service -o short-iso | \
  grep -E 'startup_phase|startup_ready|bfd_load_result|bgp_load_result'
```

### Slow Phase Detection

Slow phases (>5s) are logged at `error` level:

```bash
journalctl -u tovarisch.service -p err -o short-iso | \
  grep 'startup_phase_slow'
```

## Systemd Integration

### Type=notify (Recommended for Precise Readiness)

For applications that send `READY=1` via sd_notify(), systemd waits for that signal. This provides exact readiness semantics.

`tovarisch` does NOT currently send `sd_notify(READY=1)`. Adding this is tracked in:
- **ACT-TOVARISCH-SYSTEMD-NOTIFY-READY01**

### Type=simple (Current Default)

For Type=simple units, systemd considers the service started when the main process forks/execs. Use `startup_ready` as the canonical readiness marker for monitoring.

Example systemd service file snippet:
```ini
[Service]
Type=simple
# Restart on crash
Restart=on-failure
# Give up to 5 minutes for startup
TimeoutStartSec=300
```

### Health Check Pattern

Create a systemd unit that polls the HTTP endpoint:

```ini
[Unit]
Requires=tovarisch.service
After=tovarisch.service

[Service]
Type=oneshot
ExecStart=/bin/sh -c 'until curl -sf http://127.0.0.1:8317/status; do sleep 1; done'
TimeoutSec=300
```

## Slow Threshold Configuration

The default slow threshold is 5000ms. This is defined in `startup_trace.zig`:

```zig
pub const DEFAULT_SLOW_THRESHOLD_MS: u64 = 5000;
```

To adjust for your deployment, modify this constant or add a configuration option.

## Alerting

### Prometheus Alert Rule

```yaml
groups:
  - name: tovarisch
    rules:
      - alert: TovarischSlowStartup
        expr: |
          sum by (instance) (
            rate(tovarisch_startup_phase_slow_total[5m])
          ) > 0
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "Tovarisch startup phase exceeding threshold"
          description: "Instance {{ $labels.instance }} has slow startup phases"
```

### Log-based Alert (journald)

```bash
# Alert when any startup_phase_slow is emitted
journalctl -f -u tovarisch.service -p err | grep startup_phase_slow
```

## Testing

Unit tests for startup tracing are in `startup_trace.zig` and integration tests in `startup_trace_integration_tests.zig`.

Run tests:
```bash
make tovarisch-test
```

## Files

| File | Purpose |
|------|---------|
| `tovarisch/src/startup_trace.zig` | Core tracing module |
| `tovarisch/src/startup_trace_integration_tests.zig` | Integration tests |
| `tovarisch/src/cli/daemon_command.zig` | Instrumented startup sequence |
| `tovarisch/src/http/server.zig` | HTTP serve with tracer passthrough |
| `tovarisch/src/logging.zig` | Event enum additions |

## See Also

- [systemd.service(5)](https://man7.org/linux/man-pages/man5/systemd.service.5.html) — Type=notify semantics
- [sd_notify(3)](https://man7.org/linux/man-pages/man3/sd_notify.3.html) — Readiness notification
