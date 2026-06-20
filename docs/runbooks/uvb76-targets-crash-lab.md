# UVB-76 Targets Crash Lab

## Purpose

This lab proves the UVB-76 admin `/api/v1/targets` surface does not crash under HTTPS admin handler churn, including the router-relevant HTTP/2 path that exposed the previous SIGSEGV crash.

## Crash Class Protected

The lab protects against:
- **SIGSEGV in request handlers**: The original crash surfaced through `config.DiagPeerStatusURL()` → `net/url.Parse` → Go runtime allocation → stack growth → SIGSEGV
- **Handler churn regressions**: Concurrent authenticated requests to `/api/v1/targets` over HTTPS/HTTP2
- **TLS/HTTPS path issues**: Runtime TLS cert handling in the HTTPS server path

## How to Run Locally

### Prerequisites
- Go 1.21+
- `make` and bash
- Network access to localhost (for lab port binding)

### Default Run (60 seconds)
```bash
make lab-uvb76-targets-crash
```

### Short Run (10 seconds, 4 workers)
```bash
make lab-uvb76-targets-crash
# Or pass --short to the launcher
./scripts/lab_uvb76_targets_crash.sh --short
```

### Custom Duration/Workers
```bash
UVB76_TARGETS_CRASH_LAB_DURATION=30 \
UVB76_TARGETS_CRASH_LAB_WORKERS=16 \
make lab-uvb76-targets-crash
```

### HTTP/2 Disabled Mode
```bash
UVB76_TARGETS_CRASH_LAB_HTTP2_DISABLED=1 \
UVB76_TARGETS_CRASH_LAB_DURATION=30 \
make lab-uvb76-targets-crash
```

## Artifacts

The lab writes artifacts to a temporary directory under `/tmp/kgb-uvb76-targets-crash-*`:

| File | Description |
|------|-------------|
| `summary.json` | Lab outcome (status, counts, mode) |
| `config.json` | Hermetic UVB-76 config |
| `cert.pem` | Runtime-generated TLS certificate |
| `key.pem` | Runtime-generated TLS key |
| `uvb76.stdout.log` | Daemon stdout |
| `uvb76.stderr.log` | Daemon stderr |
| `targets-response-sample.json` | Sample `/api/v1/targets` response |
| `workload-summary.json` | Per-worker request stats |
| `worker-stats.json` | Detailed worker statistics |

## Running the Verifier Manually

After the lab runs, you can verify the artifacts independently:

```bash
# Find the artifact directory
ls -lt /tmp/kgb-uvb76-targets-crash-*

# Run the verifier
./uvb76/uvb76-targets-crash-verify /tmp/kgb-uvb76-targets-crash-<timestamp>
```

## Why Runtime Cert Generation?

**No inline blobs**: TLS certificates and keys are never embedded in source code.

**Security**: Generated certs are short-lived (24-hour validity) and use fresh key material per lab run.

**Reproducibility**: Deterministic salt in config generation ensures reproducible lab configs while TLS certs are regenerated each run.

**Portability**: No embedded secrets means the lab code can be safely version-controlled and shared.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `UVB76_TARGETS_CRASH_LAB_DURATION` | 60 | Lab duration in seconds |
| `UVB76_TARGETS_CRASH_LAB_WORKERS` | 8 | Number of concurrent request workers |
| `UVB76_TARGETS_CRASH_LAB_HTTP2_DISABLED` | (unset) | Set to "1" to disable HTTP/2 |
| `UVB76_BINARY` | auto-detect | Path to UVB-76 binary |

## Fail-Closed Conditions

The lab fails if any of these occur:

- UVB-76 process exits before the lab ends
- UVB-76 logs `SIGSEGV`, `panic:`, or `fatal error:`
- Any request gets a connection error after readiness
- Any request returns non-200 status
- Response JSON cannot be decoded
- Response has fewer than 2 targets
- Expected target IDs are missing
- Expected diagnostic fields are missing
- `effective_capture_url` is empty for the diagnostic target
- Request success count is zero

## CI Execution

Primary execution is via GitHub Actions `workflow_dispatch` or local `make lab-uvb76-targets-crash`.

Not part of `make gate` (soak duration makes it unsuitable for fast CI gates).

## Related Documentation

- [UVB-76 Runtime Architecture](../architecture/uvb76-runtime.md)
- [UVB-76 Latency Crash Lab](./uvb76-latency-crash-lab.md) (sibling lab for LatencyTracker)
- [UVB-76 Capture URL Lab](./uvb76-capture-url-lab.md)
