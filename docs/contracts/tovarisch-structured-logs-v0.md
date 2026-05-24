# Tovarisch Structured Logs Contract v0

> **Status**: Active — All runtime logs must use this format.

## Overview

`tovarisch` emits structured JSON logs for all runtime operational messages. Logs are newline-delimited JSON records, one per line.

**Key distinction**: Logs are separate from command/API payloads.

| Output | Type | Format |
|--------|------|--------|
| `tovarisch serve` stdout | Runtime logs | NDJSON |
| `tovarisch status --json` stdout | Status payload | JSON |
| `/status` HTTP response | Status payload | JSON |
| `--help` stdout | Help text | Prose (allowed) |
| `--version` stdout | Version string | Text (allowed) |

## Log Record Contract

Every log record contains:

```json
{
  "ts": "2026-05-23T13:00:00+02:00",
  "level": "info",
  "event": "event_name",
  "service": "tovarisch",
  "version": "0.1.1",
  "fields": {}
}
```

### Required Fields

| Field | Type | Description |
|-------|------|-------------|
| `level` | string | One of: `debug`, `info`, `warn`, `error` |
| `event` | string | Stable snake_case event identifier |
| `service` | string | Always `tovarisch` |
| `version` | string | Current version constant (e.g., `0.1.1`) |
| `fields` | object | Key-value pairs; may be empty `{}` |

### Optional Fields

| Field | Type | Description |
|-------|------|-------------|
| `ts` | string | ISO-8601/RFC3339 timestamp when available |
| | | Until clock abstraction exists, omit or use placeholder |

## Event Catalog

### HTTP Server Events

| Event | Level | Fields | Description |
|-------|-------|--------|-------------|
| `http_server_listening` | info | `bind_address`, `port` | Server bound and listening |
| `http_accept_loop_started` | info | — | Accept loop entered |
| `http_accept_loop_error` | error | `error` | Error in accept loop iteration |

### Tunnel Events

| Event | Level | Fields | Description |
|-------|-------|--------|-------------|
| `tunnel_stats` | info | `tunnels_total`, `tunnels_up`, `tunnels_down` | Periodic tunnel status |
| `tunnel_stats` | info | `tunnels_total`, `detail` | Placeholder when inventory not implemented |

### Application Events

| Event | Level | Fields | Description |
|-------|-------|--------|-------------|
| `app_startup` | info | — | Application starting |
| `app_shutdown` | info | — | Application shutting down |
| `server_error` | error | `error` | Server-level startup/runtime error surfaced by CLI |
| `uvb76_signal_ready` | info | `signal`, `message` | Operator-facing signal that the HTTP service is ready |

### Heartbeat Events

| Event | Level | Fields | Description |
|-------|-------|--------|-------------|
| `heartbeat` | info | `ts`, `uptime_seconds`, `status`, `checks_count`, `tunnels_count`, `rx_bytes`, `tx_bytes` | Periodic heartbeat log; flat JSON record (not using `fields` wrapper) |

Note: The heartbeat event uses a flat JSON format at the top level (not wrapped in `fields`). This is intentional for operational simplicity. Tunnel counters (`tunnels_count`, `rx_bytes`, `tx_bytes`) are placeholders until the tunnel subsystem exists.

## Field Value Types

Fields support these value types:

| Type | Example | JSON Representation |
|------|---------|---------------------|
| string | `"127.0.0.1"` | `"value"` |
| integer | `8317` | `8317` |
| boolean | `true` | `true` |
| null | — | `null` |

## Escaping Rules

String values must be JSON-escaped:

- `"` → `\"`
- `\` → `\\`
- `\n` → `\n`
- `\r` → `\r`
- `\t` → `\t`
- Control chars (< 0x20) → `\u00XX`

## Examples

### HTTP Server Listening

```json
{"level":"info","event":"http_server_listening","service":"tovarisch","version":"0.1.1","fields":{"bind_address":"127.0.0.1","port":8317}}
```

### Accept Loop Started

```json
{"level":"info","event":"http_accept_loop_started","service":"tovarisch","version":"0.1.1","fields":{}}
```

### Tunnel Stats (Placeholder)

```json
{"level":"info","event":"tunnel_stats","service":"tovarisch","version":"0.1.1","fields":{"tunnels_total":0,"detail":"tunnel inventory not implemented yet"}}
```

### Tunnel Stats (Full)

```json
{"level":"info","event":"tunnel_stats","service":"tovarisch","version":"0.1.1","fields":{"tunnels_total":5,"tunnels_up":3,"tunnels_down":2}}
```

### Accept Loop Error

```json
{"level":"error","event":"http_accept_loop_error","service":"tovarisch","version":"0.1.1","fields":{"error":"ConnectionRefused"}}
```

### Heartbeat Log

```json
{"ts":"2026-05-24T00:00:00Z","level":"info","event":"heartbeat","service":"tovarisch","uptime_seconds":30,"status":"warn","checks_count":5,"tunnels_count":0,"rx_bytes":0,"tx_bytes":0}
```

Note: Heartbeat uses flat JSON format (not wrapped in `fields`). Timestamp is a placeholder until proper time API is available. Tunnel counters are zero until tunnel subsystem exists.

## Emoji Policy

Emoji are allowed in structured log field string values.

Emoji are forbidden in:
- `event`
- `level`
- `service`
- field names

Example:

```json
{"level":"info","event":"uvb76_signal_ready","service":"tovarisch","version":"0.1.1","fields":{"signal":"🚩📻","message":"Listen to UVB-76 signals..."}}
```

## Forbidden Patterns

These patterns are forbidden in runtime code:

```zig
// FORBIDDEN: prose runtime logs
std.debug.print("Listening on {s}:{d}\n", .{...});
std.debug.print("Entering accept loop\n", .{});
stdout.print("Starting tovarisch HTTP service...\n", .{});

// FORBIDDEN: emoji in event/level/service/field names
// (emoji in field STRING VALUES is allowed)

// ALLOWED: command output (not logs)
stdout.print("tovarisch {s}\n", .{version});  // --version
stdout.writeAll("tovarisch check: ok\n");     // check
```

## Adding New Events

When adding a new runtime event:

1. Add the event to `logging.zig` `Event` enum with snake_case name
2. Emit the event via `logging.emit()` or `logging.info()` / `logging.error()`
3. Add test coverage for the log record format
4. Document the event in this contract

## Implementation

See `tovarisch/src/logging.zig` for the implementation.

Verification: `make verify-structured-logs`
