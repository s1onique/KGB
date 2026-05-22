# KGB Metrics Doctrine

KGB metrics are not observability for servers.

They are vital signs for escape routes.

## Metric classes

### Node survival

- uptime
- restart count
- RSS
- disk free
- clock skew
- state write success

### Control-plane reachability

- station reachable
- last successful pull
- last successful report
- desired-state version
- last-known-good version

### Tunnel liveness

- transport up
- interface present
- last handshake age
- rx/tx counters
- stale handshake detection

### Censorship/degradation signals

- UDP viability
- TCP viability
- TLS handshake viability
- DNS success/mismatch
- packet loss
- latency
- suspected MTU breakage

### Fallback readiness

- fallback configured
- fallback last tested
- fallback test result
- fallback switch count

### Route correctness

- default route via tunnel
- DNS route via tunnel
- public IP matches expected exit
- route leak detected

## Doctrine

Generic server monitoring is insufficient.

`tovarisch` must report purpose-built, privacy-safe, bounded metrics that indicate whether the escape route still works.
