# Spike-Triggered Diagnostics Capture

This document describes the spike-triggered diagnostics capture feature in UVB-76 and how to use the web UI to review evidence.

## Overview

When UVB-76 detects a latency spike on a monitored target, it can automatically capture network diagnostics from configured tovarisch peers. This provides evidence about the state of the network at the time of the spike.

## What It Does

- **Read-only evidence capture**: Automatically fetches `/status?include=network_diag` from configured tovarisch peers
- **Non-blocking**: Captures run asynchronously and never block latency probe recording
- **Cooldown protection**: Prevents diagnostic storms during repeated spike bursts
- **Evidence persistence**: Capture failures are stored as evidence (not swallowed)
- **Web UI review**: View details, download JSON bundles directly from the browser

## What It Does NOT Do

- No remediation actions (no MTU changes, sysctl changes, restarts)
- No command execution on remote systems
- No permanent scraping on every probe interval
- No peer auto-discovery
- Not proof of MTU/MSS, BGP/BFD stability, or hostile-network behavior

## Using the Web UI

### Finding Spike Diagnostics

1. Open the UVB-76 web interface
2. Navigate to the target card for the monitored endpoint
3. Look for the **Spike diagnostics** section

### Reviewing Captures

Each spike event shows:

- **Time**: When the spike occurred
- **Kind**: HTTP or ICMP probe type
- **Severity**: Warning or Critical
- **Latency**: The spike latency value
- **Captures**: Diagnostic evidence from tovarisch peers

### Viewing Details

Click **View details** on any capture to expand structured details including:

- **Source**: Which tovarisch peer captured the data
- **Status**: Capture status (ok, error, timeout, etc.)
- **Duration**: How long the capture took
- **Started/Finished**: Capture timestamps
- **Suppressed by cooldown**: Whether capture was skipped due to rate limiting
- **Network diagnostics**: Status and TCP underlay data

### Network Diagnostics Status

| Status | Meaning |
|--------|---------|
| **ok** | Full network diagnostics captured successfully |
| **missing** | Capture succeeded but tovarisch did not return network diagnostics |
| **suppressed** | Capture was skipped due to cooldown (not a diagnostic failure) |

#### When Network Diagnostics Are Missing

If you see **Network diag: missing**, this indicates:

1. The HTTP request to tovarisch succeeded (status ok)
2. But tovarisch did not include `network_diag` in its response

This may happen if:
- tovarisch does not support network diagnostics (older version)
- The `/status` endpoint doesn't include the `include=network_diag` query parameter
- tovarisch had an internal error after the HTTP response started

### Downloading Evidence

Two download options are available:

1. **Download capture JSON**: Export a single capture as JSON
2. **Download spike bundle**: Export the spike event with all captures

Downloads are browser-based (no server endpoint required) and produce files like:

```
uvb76-capture-kamatera-evt-123-peer-1.json
uvb76-spike-kamatera-evt-123.json
```

#### Capture Export Format

```json
{
  "export_kind": "uvb76_diagnostic_capture",
  "exported_at": "2026-06-18T23:10:00Z",
  "target_id": "kamatera",
  "spike_event_id": "evt-123",
  "capture": {
    "source": "peer-1",
    "status": "ok",
    "network_diag": {
      "started_at": "...",
      "status": "ok",
      "underlay_tcp": [...]
    }
  }
}
```

#### Spike Bundle Export Format

```json
{
  "export_kind": "uvb76_spike_diagnostics_bundle",
  "exported_at": "2026-06-18T23:10:00Z",
  "target_id": "kamatera",
  "spike": {
    "event_id": "evt-123",
    "kind": "http",
    "latency_ms": 1234
  },
  "captures": [...]
}
```

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
| `suppressed by cooldown` | Capture skipped due to cooldown (not a failure) |

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

## Evidence Use for Debugging

The captured evidence is useful for investigating:

- **TCP RTT/RTO changes**: Indicates network path latency shifts
- **Retransmits**: Suggests packet loss on the underlay
- **Interface errors**: Shows NIC-level issues
- **Route changes**: BGP next-hop or routing table changes
- **Wireguard handshakes**: Tunnel stability issues

This evidence is **correlational, not causal**. A spike with retransmits doesn't prove retransmits caused the spike.

## Security Notes

- Only configured HTTP GET requests to configured tovarisch base URLs
- No shell commands
- No user-supplied URL fetches from API request parameters
- No remediation actions
- Error messages are sanitized before storage
- Download functionality is client-side only (no server endpoint)
- Filenames are sanitized (no path traversal possible)
