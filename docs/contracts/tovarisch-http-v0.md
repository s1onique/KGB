# `tovarisch` HTTP Service v0 Contract

## Purpose

This document defines the v0 HTTP service contract for `tovarisch serve`. The HTTP service provides local-first observability endpoints for leaf-node health monitoring by a KGB UVB-76.

## Canonical Runtime

```bash
tovarisch serve
```

Default port: **8317**

## Bind Behavior

### Default: Private Interfaces Only (v0 target)

By default, `tovarisch serve` should bind to all private, non-loopback interface addresses:

```
192.168.x.x:8317
10.x.x.x:8317
172.16.x.x:8317
fd00::/8 (ULA):8317
```

**Do NOT bind to public IPs by default.**
**Do NOT bind to 0.0.0.0 blindly.**

### Current v0 implementation: loopback only

`tovarisch serve` currently binds to `127.0.0.1:8317` by default (loopback interface only).

A follow-up ACT will change default binding to private, non-loopback interface addresses once private interface enumeration exists.

### Explicit Overrides

```bash
# Bind to specific address
tovarisch serve --listen 127.0.0.1:8317

# Explicitly bind to all private interfaces
tovarisch serve --listen-private

# Dangerous: bind to all interfaces (including public)
tovarisch serve --listen-all-public-dangerous
```

## Endpoints

### `GET /healthz`

Simple liveness check.

**Request:** `GET /healthz HTTP/1.1`

**Response:**
```
HTTP/1.1 200 OK
Content-Type: application/json
Content-Length: 17

{"status":"ok"}
```

**Status Codes:**
- `200 OK` - service is running
- `500 Internal Server Error` - service is degraded

**Security:** Safe to expose on private interfaces.

---

### `GET /status.json`

Machine-readable health status (extends `tovarisch-status-v0.md`).

**Request:** `GET /status.json HTTP/1.1`

**Response:**
```
HTTP/1.1 200 OK
Content-Type: application/json

{"service":"tovarisch","version":"0.1.1","node_id":"local-dev","status":"warn","checks":[...]}
```

**Status Codes:**
- `200 OK` - status retrieved
- `500 Internal Server Error` - status retrieval failed

**Security:** Safe to expose on private interfaces. Contains node identity and health checks.

**Additional Check:** Response should include an `http` check showing service is listening.

---

### `GET /metrics.json`

Interface and tunnel metrics payload.

**Request:** `GET /metrics.json HTTP/1.1`

**Response:**
```
HTTP/1.1 200 OK
Content-Type: application/json

{"service":"tovarisch","version":"0.1.1","node_id":"local-dev","captured_at":"...","interfaces":[...],"tunnels":[...]}
```

**Status Codes:**
- `200 OK` - metrics retrieved
- `500 Internal Server Error` - metrics retrieval failed

**Security:** May reveal network topology. Expose only on private interfaces.

---

### Other Paths

All other paths return `404 Not Found`.

```
HTTP/1.1 404 Not Found
Content-Type: application/json

{"error":"not_found"}
```

---

### Non-GET Methods

All non-GET methods return `405 Method Not Allowed`.

```
HTTP/1.1 405 Method Not Allowed
Content-Type: application/json

{"error":"method_not_allowed"}
```

## HTTP Protocol Constraints

For v0, the HTTP implementation is intentionally primitive:

- **HTTP/1.1 only**
- **No keepalive** - close connection after response
- **No chunked transfer encoding**
- **No TLS**
- **No compression**
- **No WebSocket**

This ensures the binary remains tiny and suitable for constrained leaf nodes.

## Security Stance

### v0: Read-Only Only

No mutation endpoints in v0.

### Forbidden by Default

| Endpoint Pattern | Reason |
|-----------------|--------|
| `GET /debug/*` | May expose sensitive internal state |
| `POST /control/*` | Not implemented yet |
| Any public IP binding | Risk of external exposure |

### Debug Endpoints (Future)

If `/debug/*` endpoints are added later:
- They MUST be disabled by default
- They MUST be loopback-only when enabled
- They MUST require explicit flag to enable

## Request Parsing

Minimal parsing requirements:

1. Read the request line (`GET /path HTTP/1.1`)
2. Ignore headers for v0
3. Read optional body (not used in v0)
4. Route by path

## Response Format

All responses are JSON:

```json
{"status":"ok"}
{"error":"not_found"}
{"error":"method_not_allowed"}
```

## Error Handling

Errors return appropriate HTTP status codes:

| Condition | Status Code | Body |
|-----------|-------------|------|
| OK | 200 | JSON payload |
| Not Found | 404 | `{"error":"not_found"}` |
| Method Not Allowed | 405 | `{"error":"method_not_allowed"}` |
| Internal Error | 500 | `{"error":"internal_error"}` |

## Connection Lifecycle

1. Accept incoming TCP connection
2. Read request line
3. Route to handler
4. Write HTTP response
5. Close connection

No keepalive. Each request is a separate connection.

## Privacy Constraints

The HTTP endpoints must NOT expose:
- Browsing history
- Visited domains
- Destination IP flow logs
- Message contents
- Per-user behavioral timelines

**Allowed:** node identity, transport state, interface statistics, health status.

## Contract Version

| Version | Date | Changes |
|---------|------|---------|
| 0.1.0 | 2026-05-22 | Initial HTTP service contract |

## Relationship to Other Contracts

- Base status contract: `tovarisch-status-v0.md`
- Health checks contract: `tovarisch-local-health-v0.md`
- Metrics payload: defined in `tovarisch-metrics-v0.md` (future)

## Future Evolution

1. **TLS support** - future ACT
2. **Bearer token auth** - future ACT
3. **Keepalive** - future ACT
4. **Chunked responses** - future ACT
5. **Debug endpoints** - future ACT (loopback-only)
6. **POST endpoints** - future ACT for control operations
