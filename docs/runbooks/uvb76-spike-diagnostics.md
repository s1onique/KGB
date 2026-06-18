# Spike-Triggered Diagnostics Capture

This document describes the spike-triggered diagnostics capture feature in UVB-76.

## Overview

When UVB-76 detects a latency spike on a monitored target, it can automatically capture network diagnostics from configured tovarisch peers. This provides evidence about the state of the network at the time of the spike without requiring manual SSH/debug commands.

## What It Does

- **Read-only evidence capture**: Automatically fetches `/status?include=network_diag` from configured tovarisch peers
- **Non-blocking**: Captures run asynchronously and never block latency probe recording
- **Cooldown protection**: Prevents diagnostic storms during repeated spike bursts
- **Evidence persistence**: Capture failures are stored as evidence (not swallowed)

## What It Does NOT Do

- No remediation actions (no MTU changes, sysctl changes, restarts)
- No command execution on remote systems
- No permanent scraping on every probe interval
- No peer auto-discovery
- Not proof of MTU/MSS, BGP/BFD stability, or hostile-network behavior

## Configuration

Add a `diagnostics` section to your `uvb76.json`:

```json
{
  "diagnostics": {
    "enabled": true,
    "capture_on_spike": true,
    "timeout_ms": 1500,
    "cooldown_seconds": 90,
    "peers": [
      {
        "name": "tovarisch-home",
        "base_url": "http://10.77.0.1:8080",
        "targets": ["kamatera"]
      }
    ]
  }
}
```

### Configuration Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | false | Enable/disable diagnostics capture |
| `capture_on_spike` | bool | true | Capture on spike detection |
| `timeout_ms` | int | 1500 | HTTP request timeout in milliseconds |
| `cooldown_seconds` | int | 90 | Suppress duplicate captures for this duration |
| `peers` | array | [] | List of tovarisch peer configurations |

### Peer Configuration

Each peer maps target IDs to diagnostic endpoints:

| Field | Type | Required | Description |
|-------|------|---------|-------------|
| `name` | string | yes | Unique peer name for identification |
| `base_url` | string | yes | Base URL of tovarisch (must use http:// or https://) |
| `targets` | array | yes | List of target IDs that trigger this peer |

## Capture Status Values

| Status | Meaning |
|--------|---------|
| `ok` | Successful capture with data |
| `unavailable` | tovarisch returned unavailable status |
| `timeout` | HTTP request timed out |
| `error` | HTTP error or JSON parse failure |
| `disabled` | Diagnostics feature disabled in config |
| `no_peer_mapping` | Target has no configured diagnostic peer |

## API Endpoints

### Get Spikes with Captures

```
GET /api/v1/latency/spikes?target_id=<id>&include_captures=true
```

Response includes `captures` array per spike event:

```json
{
  "spikes": [
    {
      "event_id": "...",
      "target_id": "kamatera",
      "captures": [
        {
          "source": "tovarisch-home",
          "base_url": "http://10.77.0.1:8080",
          "capture_started_at": "2026-06-18T12:00:00Z",
          "capture_finished_at": "2026-06-18T12:00:01Z",
          "duration_ms": 123,
          "status": "ok",
          "error": null,
          "network_diag": {
            "started_at": "1718700000000",
            "status": "ok",
            "wireguard": {...},
            "interfaces": [...],
            "routes": [...],
            "underlay_tcp": [...],
            "events": [...]
          }
        }
      ]
    }
  ],
  "count": 1
}
```

## Cooldown Semantics

- First spike for target/peer triggers immediate capture
- Subsequent spikes within `cooldown_seconds` suppress capture
- All spike events are still recorded
- Suppressed captures include `suppressed_by_cooldown: true`

## Security Notes

- Only configured HTTP GET requests to configured tovarisch base URLs
- No shell commands
- No user-supplied URL fetches from API request parameters
- No remediation actions
- Error messages are sanitized before storage
