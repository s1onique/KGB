# CORRECTION29 Close Report

## Identity

| Field | Value |
|-------|-------|
| correction | CORRECTION29 |
| title | Canonical Canary-Control Protocol |
| subject_commit (S29) | ca190e6b4bce93e41b933c8aafb922ac1188979e |
| subject_tree (ST29) | 005eedf047fa3516da834d2c67ef476964aa1d39 |
| evidence_commit (E29) | (this commit) |
| parent_commit (S28) | 2c801ef1f7cac9ac00f66cf2cadd214c1e388d83 |
| parent_tree (ST28) | 56310d512cee44fc0259e549f77f435f22335d21 |
| closed_at | 2026-07-25T00:00:30+03:00 |

## Baseline CORRECTION28 Assessment

**CORRECTION28: SUPERSEDED** — CORRECTION29 completes the tool-independent control plane by defining the canonical protocol.

## CORRECTION29 Changes

### P0-1: Canonical ControlEnvelope Protocol

Defined the canonical protocol envelope with typed payloads:

```go
type ControlEnvelope struct {
    SchemaVersion string           `json:"schema_version"`
    Operation     string           `json:"operation"`
    Success       bool             `json:"success"`
    HTTPStatus   int              `json:"http_status"`
    Health       *HealthResult   `json:"health,omitempty"`
    State        *StateResult    `json:"state,omitempty"`
    Workload     *WorkloadResult `json:"workload,omitempty"`
    ErrorClass   ErrorClass      `json:"error_class,omitempty"`
}
```

Schema version: `canary-control/v1`

### P0-2: Typed Successful Results

Each operation emits a complete typed envelope:

**Health:**
```json
{
  "schema_version": "canary-control/v1",
  "operation": "health",
  "success": true,
  "http_status": 200,
  "health": {
    "ready": true,
    "mode": "bounded"
  }
}
```

**State:**
```json
{
  "schema_version": "canary-control/v1",
  "operation": "state",
  "success": true,
  "http_status": 200,
  "state": {
    "mode": "bounded",
    "retained_blocks": 0,
    "retained_bytes": 0,
    "operation_count": 0,
    "fd_count": 0,
    "ready": true
  }
}
```

**Operate:**
```json
{
  "schema_version": "canary-control/v1",
  "operation": "operate",
  "success": true,
  "http_status": 200,
  "workload": {
    "requested": 100,
    "attempted": 100,
    "completed": 100
  }
}
```

### P0-3: Typed Failures

Stable error classes for classification:

- `invalid_arguments`
- `request_create_failed`
- `connection_failed`
- `request_timeout`
- `response_too_large`
- `unexpected_http_status`
- `malformed_json`
- `unknown_json_field`
- `missing_required_field`
- `trailing_json`
- `health_not_ready`
- `state_invalid`
- `workload_count_mismatch`

### P0-4: Response-Size Ceiling

Enforced 64KB response body limit with `io.LimitReader`.

### P0-5: Strict JSON Decoding

- `json.Decoder` with `DisallowUnknownFields`
- Trailing data detection and rejection
- Required field presence validation

### P0-6: Strict Envelope Parsing

Docker client now strictly parses envelopes:
- Schema version validation
- Operation/argv consistency
- Success/exit-code consistency
- Operation-specific payload validation

### P0-7: Remove Marker-Based Parsing

Removed:
- `strings.Contains(stdout, "OK")`
- `strings.Contains(stdout, "STATE_VALID")`
- `strings.Contains(stdout, "WORKLOAD_VALID")`

### P0-8: Exact Exec Argv

Commands use exact argv:
```bash
/app/canary control health --port 8080 --timeout 5s
/app/canary control state --port 8080 --timeout 5s
/app/canary control operate --port 8080 --count 100 --timeout 30s
```

## Binary Metadata

### Canary Binary

| Field | Value |
|-------|-------|
| SHA-256 | aed67318f30257eeea52d9efbd7ab6341edd8ce840416336d9e1ea17ba4db11f |
| Path | /tmp/canary |
| Built from | S29 |

### Production CLI Binary

| Field | Value |
|-------|-------|
| SHA-256 | 122bf92659fcbde2f98f23c82cd47c59e5d8072e8fb6d65301becdcbff9a883d |
| Path | /tmp/tovarisch-memory-lab-cli |
| Built from | S29 |

## Verification

### Go Tests

```
ok  github.com/s1onique/KGB/tovarisch/labs/memory
ok  github.com/s1onique/KGB/tovarisch/labs/memory/cmd/canary
ok  github.com/s1onique/KGB/tovarisch/labs/memory/cmd/tovarisch-memory-lab
ok  github.com/s1onique/KGB/tovarisch/labs/memory/internal/dockerlab
ok  github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence
```

### Memory Gate

```
=== memory-gate passed ===
```

## Files Changed

- `tovarisch/labs/memory/cmd/canary/control.go`
- `tovarisch/labs/memory/internal/dockerlab/client.go`

## Command/Exit-Code Matrix

| Command | Exit Code |
|---------|-----------|
| go test ./... | 0 |
| go build ./cmd/canary | 0 |
| go build ./cmd/tovarisch-memory-lab | 0 |
| make memory-gate | 0 |
| make tovarisch-build | 0 |
| make tovarisch-test | 0 |

## Doctrine Compliance

### Shell Containment

CORRECTION29 maintains shell-free control plane with:
- Exact argv execution
- No shell, no curl, no wget
- Typed protocol with strict validation

### Native-Owned Critical Paths

The canary-control binary is owned by the image, ensuring tool-independent transport.

### Embedded Memory Frugality

- Fixed 64KB response body limit
- Proper timeout handling
- No unbounded memory growth
