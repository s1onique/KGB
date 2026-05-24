# Tovarisch Attack Surface Inventory

## Overview

This document provides a concrete attack-surface inventory for `tovarisch serve`, covering HTTP routes, listener behavior, bind flags, exposed data, and security posture.

**Answer without reading Zig**: What can a network peer hit, what data comes back, where is it allowed to bind, and what can go wrong?

---

## HTTP Routes

### `GET /healthz`

| Field | Value |
|-------|-------|
| Method | GET |
| Path | `/healthz` |
| Purpose | Liveness probe for orchestration systems |
| Data exposed | Trivial liveness state only (minimal JSON) |
| Sensitivity | **Low** |
| Expected bind scope | Any (local operator, private LAN, VPN peer) |
| Abuse cases | None significant - read-only liveness check |
| Existing controls | None required for low-sensitivity endpoint |
| Deferred controls | None |

### `GET /status` / `GET /status.json`

| Field | Value |
|-------|-------|
| Method | GET |
| Path | `/status` and `/status.json` (aliases) |
| Purpose | Node identity and runtime state snapshot |
| Data exposed | Node identifier, runtime/process state, health checks, version info |
| Sensitivity | **Medium** |
| Expected bind scope | Loopback or private interface only |
| Abuse cases | Network peer enumerates running services and node identity without authentication |
| Existing controls | Default bind to 127.0.0.1; CLI validation blocks public IPs without flag |
| Deferred controls | Rate limiting, IP allowlist for non-loopback binds |

### `GET /metrics.json`

| Field | Value |
|-------|-------|
| Method | GET |
| Path | `/metrics.json` |
| Purpose | Infrastructure health metrics for station |
| Data exposed | Topology hints, interface counters, rate samples, node identifiers |
| Sensitivity | **Medium/High** |
| Expected bind scope | Loopback or private interface only |
| Abuse cases | Network peer learns network topology, interface names, traffic patterns, node identity |
| Existing controls | Default bind to 127.0.0.1; CLI validation blocks public IPs without flag |
| Deferred controls | Authentication, TLS, metrics scrubbing (interface names, node IDs) |

### Unknown Paths (404)

| Field | Value |
|-------|-------|
| Method | Any path not matched |
| Path | Any unrecognized path |
| Purpose | Graceful non-disclosure |
| Data exposed | Minimal (no internal state) |
| Sensitivity | **Low** |
| Expected bind scope | Any |
| Abuse cases | Probe for valid routes - response must not leak internals |
| Existing controls | `handleNotFound` returns generic 404, no stack traces or paths |
| Deferred controls | None |

### Non-GET Methods (405)

| Field | Value |
|-------|-------|
| Method | POST, PUT, DELETE, PATCH, HEAD, OPTIONS |
| Path | Any |
| Purpose | Reject write operations (v0 is read-only) |
| Data exposed | Minimal (method not allowed indicator) |
| Sensitivity | **Low** |
| Expected bind scope | Any |
| Abuse cases | None - read-only service correctly rejects mutations |
| Existing controls | `handleMethodNotAllowed` returns 405 for non-GET methods |
| Deferred controls | None |

---

## Listener Behavior

### Default Configuration

| Field | Value |
|-------|-------|
| Default address | `127.0.0.1` (loopback only) |
| Default port | `8317` |
| Socket option | `SO_REUSEADDR` enabled |

### `--listen` Behavior

- Accepts explicit IPv4 address and port
- Validation via `classifyServeBindHost` 
- Rejects public IPs without `--listen-all-public-dangerous` flag
- Safe for private interface binding (e.g., `10.0.0.5:8317`)

### `--listen-all-public-dangerous` Behavior

- Explicit dangerous flag required to bind to public IPs
- Required for wildcards (`0.0.0.0`) and public CIDR
- Flag name signals danger; no silent public exposure
- **Warning**: Exposing station or tovarisch HTTP ports to public internet is forbidden by doctrine

### Historical Note

The `--listen-all` flag (0.0.0.0 without dangerous marker) was rejected. Only `--listen-all-public-dangerous` is supported for explicit acknowledgment of the risk.

---

## Bind Address Classification

| Classification | Examples | Requires Flag |
|----------------|----------|---------------|
| Loopback | `127.0.0.1` | No |
| Private | `10.x.x.x`, `192.168.x.x`, `172.16-31.x.x` | No |
| Link-local | `169.254.x.x` | No |
| Public | Anything else | Yes (`--listen-all-public-dangerous`) |

---

## Trust Boundaries

### Local Operator → Tovarisch

- **Trust level**: High (same host, same operator)
- **Access**: Full access to all routes via loopback
- **Controls**: None required by default; operator controls local firewall

### Private LAN/VPN Peer → Tovarisch

- **Trust level**: Medium (private network, but not same host)
- **Access**: Status and metrics routes if bound to private interface
- **Controls**: Bind validation, explicit dangerous flag for public binds

### Public Internet → Tovarisch

- **Trust level**: None (forbidden by doctrine)
- **Access**: Must not occur without explicit dangerous flag acknowledgment
- **Controls**: CLI validation blocks public binds without `--listen-all-public-dangerous`

---

## Route Security Classification Summary

| Route | Sensitivity | Bind Scope | Key Risk |
|-------|-------------|------------|----------|
| `/healthz` | Low | Any | None |
| `/status`, `/status.json` | Medium | Private only | Node identity disclosure |
| `/metrics.json` | Medium/High | Private only | Topology and counter disclosure |
| Unknown paths | Low | Any | Internal leak via error messages |
| Non-GET methods | Low | Any | None (correctly rejected) |

---

## Abuse Cases Matrix

| Abuse Case | Affected Routes | Impact | Existing Controls | Deferred Controls |
|------------|-----------------|--------|-------------------|-------------------|
| Node identity enumeration | `/status`, `/status.json` | Reconnaissance | Loopback default, bind validation | Auth, TLS |
| Network topology disclosure | `/metrics.json` | Reconnaissance | Loopback default, bind validation | Scrub interface names |
| Traffic pattern analysis | `/metrics.json` | Behavioral inference | Loopback default | Rate limiting |
| Public bind misconfiguration | All routes | Unauthorized exposure | `classifyServeBindHost` blocks without flag | None (flag is sufficient) |
| Error message information leak | Unknown paths | Internal disclosure | `handleNotFound` is silent | None needed |

---

## Current Security Posture

**State**: Loopback-only by default, private bind supported, public bind requires explicit dangerous flag.

**Acceptable for**: Private LAN, VPN, local operator access.

**Forbidden for**: Direct public internet exposure without explicit operator acknowledgment.

---

## Deferred Security Work

- Rate limiting on `/status` and `/metrics.json`
- TLS support for authenticated station-to-tovarisch communication
- Metrics scrubbing (remove interface names, node identifiers for external export)
- IP allowlist for non-loopback binds
- Audit logging for failed bind attempts

---

## References

- [tovarisch HTTP v0 contract](../contracts/tovarisch-http-v0.md)
- [KGB security doctrine](./doctrine.md)
- [KGB threat model](./threat-model.md)
