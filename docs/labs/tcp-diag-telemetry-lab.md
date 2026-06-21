# UVB-76 TCP Diagnostic Telemetry Lab

## Purpose

This lab proves that TCP telemetry is collected in diagnostic packets by exercising the hermetic diagnostic capture path.

**Primary invariant**: `tcp_telemetry_exercised=true` only when parsed diagnostic packet evidence contains at least one typed TCP telemetry record in `network_diag.underlay_tcp`.

## Anti-False-Green Invariants

The lab MUST NOT set `tcp_telemetry_exercised=true` because:

- The diagnostic peer was configured
- HTTP status was 200
- Raw JSON contains the string "tcp"
- A summary file claimed success

The lab MUST set `tcp_telemetry_exercised=true` only when:

- Capture request used `/status.json?include=network_diag`
- At least one diagnostic packet artifact was written
- The packet contains `network_diag`
- `network_diag` contains `underlay_tcp` with at least one record
- The verifier parses JSON structurally (not via grep/string matching)

## Artifact Contract

The lab produces these artifact files:

| File | Purpose |
|------|---------|
| `lab-result.json` | Final lab summary with `tcp_telemetry_exercised` boolean |
| `capture-request.json` | HTTP request details (method, URL path) |
| `diagnostic-peer-response.json` | Raw response from diagnostic peer |
| `captured-diagnostic-packet.json` | Parsed capture packet with network_diag |
| `verifier-result.json` | Verifier output confirming TCP telemetry evidence |

### lab-result.json Schema

```json
{
  "ok": true,
  "mode": "hermetic-diagnostic-peer",
  "requested_path": "/status.json?include=network_diag",
  "capture_packet_count": 1,
  "tcp_telemetry_packet_count": 1,
  "tcp_record_count": 1,
  "tcp_event_count": 0,
  "tcp_telemetry_exercised": true,
  "artifact_dir": "/tmp/kgb-uvb76-tcp-diag-telemetry-xxx"
}
```

Note: `tcp_telemetry_exercised` is `true` only when `tcp_record_count > 0` from structurally valid `network_diag.underlay_tcp` records. `tcp_event_count` is auxiliary evidence and does not satisfy the lab invariant by itself.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Lab Runner (Go)                        │
├─────────────────────────────────────────────────────────────┤
│  1. Start Hermetic Diagnostic Peer (port 18317)             │
│     └─> Serves /status.json?include=network_diag            │
│     └─> Returns realistic payload with underlay_tcp         │
│                                                             │
│  2. Perform Diagnostic Capture                              │
│     └─> HTTP GET to peer                                    │
│     └─> Parse response JSON                                 │
│     └─> Write artifacts                                     │
│                                                             │
│  3. Run Verifier                                            │
│     └─> Structural JSON parsing                             │
│     └─> Verify underlay_tcp exists and has records          │
│     └─> Derive tcp_telemetry_exercised from evidence        │
└─────────────────────────────────────────────────────────────┘
```

## Local Run

```bash
# Build and run
make lab-uvb76-tcp-diag-telemetry

# Or via bash launcher
./scripts/lab_uvb76_tcp_diag_telemetry.sh

# Run verifier tests only
cd uvb76/cmd/uvb76-tcp-diag-telemetry-lab
go test -v ./...
```

## CI Execution

1. Navigate to GitHub Actions
2. Select "UVB-76 TCP Diagnostic Telemetry Lab"
3. Click "Run workflow"
4. Artifacts are uploaded automatically

## File Layout

```
uvb76/cmd/uvb76-tcp-diag-telemetry-lab/
├── main.go              # Entry point
├── go.mod               # Module definition
└── internal/
    ├── artifact/        # JSON artifact helpers
    │   └── artifact.go
    ├── diagpeer/        # Hermetic diagnostic peer server
    │   └── server.go
    ├── runner/          # Lab orchestration
    │   └── runner.go
    └── verifier/        # Structural verification
        ├── verifier.go
        ├── verifier_test.go
        └── testdata/
            ├── pass_tcp_telemetry/
            ├── fail_no_tcp_telemetry/
            ├── fail_wrong_location/
            ├── fail_wrong_path/
            ├── fail_summary_lies/
            ├── fail_malformed_json/
            └── fail_empty/
```

## Verifier Test Fixtures

| Fixture | Description | Expected |
|---------|-------------|----------|
| `pass_tcp_telemetry` | Valid underlay_tcp record | OK=true |
| `fail_no_tcp_telemetry` | Empty underlay_tcp array | OK=false |
| `fail_underlay_tcp_empty_object` | underlay_tcp contains empty object `{}` | OK=false |
| `fail_wrong_location` | TCP in events, not underlay_tcp | OK=false |
| `fail_wrong_path` | Request to /status instead of /status.json?include=network_diag | OK=false |
| `fail_summary_lies` | Summary claims TCP but packet has none | OK=false |
| `fail_malformed_json` | Invalid JSON in packet | OK=false |
| `fail_empty` | Empty artifact directory | OK=false |

