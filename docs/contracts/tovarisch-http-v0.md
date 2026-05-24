# `tovarisch` HTTP Service v0 Contract

## Purpose

This document defines the v0 HTTP service contract for `tovarisch serve`. The HTTP service provides local-first observability endpoints for leaf-node health monitoring by a KGB UVB-76.

## Canonical Runtime

```bash
tovarisch serve
```

Default port: **8317**

## Runtime State

`tovarisch serve` maintains process-local sampler state for rate calculation:

- **ServerState**: Owned by the server, initialized on startup, deinitialized on exit
- **MetricsState**: Contains InterfaceSampler that persists previous samples across requests
- **Sampler state**: Tracks interface counters across requests; enables rate calculation

### First Request Behavior

First `/metrics.json` request in a process returns `rate: null` for all interfaces (no previous sample).

### Subsequent Request Behavior

Later `/metrics.json` request with valid elapsed time and increasing counters returns populated rate object.

### Counter Reset

Counter reset (any counter value less than previous) returns `rate: null` for that interface.

### New/Reappearing Interfaces

New interfaces or reappearing interfaces return `rate: null` (treated as first sample).

### Clock Source

Timestamps are generated using `std.os.linux.clock_gettime(CLOCK_REALTIME)` — wall-clock seconds and nanoseconds converted to milliseconds.

- Sub-second precision available (nanoseconds in the timestamp)
- Rates become available only when elapsed whole seconds are positive (>= 1000ms)
- On non-Linux platforms, falls back to returning 0 (rates will be null until tests inject timestamps)

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

Private interface traffic metrics payload (IPv4 only, with process-local sampler state for live rates).

**Request:** `GET /metrics.json HTTP/1.1`

**Response (success, v0.2 — first request: rate field null):**
```
HTTP/1.1 200 OK
Content-Type: application/json

{"service":"tovarisch","version":"0.1.1","metrics_version":"0.2","private_interfaces":[{"name":"eth0","rx_bytes":123,"tx_bytes":456,"rx_packets":7,"tx_packets":8,"rate":null}],"notes":["rate is null until a previous sample exists","interface counters are cumulative","IPv4 private interfaces only; IPv6 is deferred"]}
```

**Response (success, v0.2 — later request with rate):**
```
HTTP/1.1 200 OK
Content-Type: application/json

{"service":"tovarisch","version":"0.1.1","metrics_version":"0.2","private_interfaces":[{"name":"eth0","rx_bytes":3123,"tx_bytes":6456,"rx_packets":37,"tx_packets":48,"rate":{"window_seconds":30,"rx_bytes_delta":3000,"tx_bytes_delta":6000,"rx_packets_delta":30,"tx_packets_delta":40,"rx_bytes_per_second":100,"tx_bytes_per_second":200,"rx_packets_per_second":1,"tx_packets_per_second":1}}],"notes":["rate is null until a previous sample exists","interface counters are cumulative","IPv4 private interfaces only; IPv6 is deferred"]}
```

**Response (live collection failure):**
```
HTTP/1.1 200 OK
Content-Type: application/json

{"service":"tovarisch","version":"0.1.1","metrics_version":"0.2","status":"warn","private_interfaces":[],"error":"metrics_unavailable","detail":"private interface stats unavailable","notes":["rate is null until a previous sample exists","interface counters are cumulative","IPv4 private interfaces only; IPv6 is deferred"]}
```

**Status Codes:**
- `200 OK` - metrics retrieved (or fallback warning payload)
- Note: Even when live collection fails, HTTP 200 is returned with `"status":"warn"` to provide a valid, actionable payload.

**Field Descriptions:**
| Field | Type | Description |
|-------|------|-------------|
| `service` | string | Always `"tovarisch"` |
| `version` | string | Tovarisch binary version (e.g., `"0.1.1"`) |
| `metrics_version` | string | Metrics schema version (`"0.2"` with rate field) |
| `status` | string | `"warn"` only on fallback; absent on success |
| `private_interfaces` | array | List of private interface stats (empty on fallback) |
| `error` | string | `"metrics_unavailable"` only on fallback |
| `detail` | string | Human-readable detail (fallback only) |
| `notes` | array | Array of three strings: rate null note, cumulative note, IPv4-only note |

**Interface Object Fields:**
| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Interface name (e.g., `"eth0"`, `"wg0"`) |
| `rx_bytes` | integer | Cumulative receive bytes (not rate) |
| `tx_bytes` | integer | Cumulative transmit bytes (not rate) |
| `rx_packets` | integer | Cumulative receive packets (not rate) |
| `tx_packets` | integer | Cumulative transmit packets (not rate) |
| `rate` | null or object | `null` on first sample or after counter reset; populated object when previous sample exists and counters are increasing |

**Rate Object Fields** (when `rate` is not null):
| Field | Type | Description |
|-------|------|-------------|
| `window_seconds` | integer | Elapsed time between samples in seconds |
| `rx_bytes_delta` | integer | Bytes received since previous sample |
| `tx_bytes_delta` | integer | Bytes transmitted since previous sample |
| `rx_packets_delta` | integer | Packets received since previous sample |
| `tx_packets_delta` | integer | Packets transmitted since previous sample |
| `rx_bytes_per_second` | integer | Receive bandwidth in bytes/second |
| `tx_bytes_per_second` | integer | Transmit bandwidth in bytes/second |
| `rx_packets_per_second` | integer | Receive packet rate in packets/second |
| `tx_packets_per_second` | integer | Transmit packet rate in packets/second |

**Important Notes:**
- All counters are **cumulative**, not instantaneous bandwidth/rates.
- Only **IPv4 private interfaces** are included (RFC1918: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16).
- **IPv6 is deferred** to a future ACT.
- Counters are collected from Linux `/sys/class/net/<iface>/statistics/*`.
- Interface addresses are discovered via rtnetlink.
- Public interfaces, loopback (without private IPv4), and IPv6-only interfaces are excluded.
- **Sampler state** persists across requests within the same process; enables rate calculation.

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
| 0.2 | 2026-05-24 | Add rate field, sampler state wiring (ACT 5) |

## Relationship to Other Contracts

- Base status contract: `tovarisch-status-v0.md`
- Health checks contract: `tovarisch-local-health-v0.md`
- Metrics payload: `interface-rates.md` (for rate calculation)
- Rate contract: `interface-rates.md`

## Future Evolution

1. **TLS support** - future ACT
2. **Bearer token auth** - future ACT
3. **Keepalive** - future ACT
4. **Chunked responses** - future ACT
5. **Debug endpoints** - future ACT (loopback-only)
6. **POST endpoints** - future ACT for control operations
