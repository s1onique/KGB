# Epic: UVB-76 Platform and Projection Hardening

**Epic ID**: ACT-UVB76-HULK05
**Status**: ACTIVE
**Started**: 2026-07-10

---

## Executive Summary

The UVB-76 Platform and Projection Hardening epic establishes comprehensive artifact-secret-hygiene contracts to prevent credential leakage through tracked artifacts. This epic creates a repository-wide, deterministic, self-tested system for identifying prohibited secret classes in artifacts and enforcing canonical redaction behavior.

## Motivation

UVB-76 generates and stores diagnostic artifacts (captured packets, memory evidence, lab results) that may contain sensitive infrastructure metadata. Without explicit hygiene contracts:
- Artifacts may accidentally include session tokens, API keys, or passwords
- Committed evidence becomes a credential sprawl risk
- No systematic way to verify artifacts are safe before commit
- Diagnostics may inadvertently expose secrets when used for debugging

## Scope

### In Scope
- Repository-wide artifact surface inventory
- Prohibited secret class definitions
- Pure deterministic redaction boundary (Go package)
- Repository verifier with bounded execution
- Self-test for verifier
- Dedicated gate target (`make hulk-uvb76-artifact-secret-gate`)
- Example config sanitization

### Out of Scope
- Runtime secret injection (separate system)
- Key rotation or management
- Audit log retention
- Network-based secret scanning
- Non-Zig/Go artifact producers

---

## Board Table (HULK05)

| ACT | Focus | Register | Gate | Status |
|-----|-------|----------|------|--------|
| ACT-UVB76-HULK05 | Artifact Secret Hygiene | `uvb76/internal/redact/redact.go` | `hulk-uvb76-artifact-secret-gate` | IN PROGRESS |

---

## Implementation Details

### Phase 1: Artifact Surface Inventory

**File**: `scripts/uvb76_artifact_secret_hygiene/inventory.py`

Artifact surfaces defined:
| ID | Path | Format | Sensitivity | Sanitizer |
|----|------|--------|-------------|-----------|
| config-example | `uvb76/uvb76.example.json` | JSON | HIGH | REDACT_CONFIG |
| capture-netns-lab-artifacts | `uvb76/cmd/uvb76-capture-netns-lab/**/*.json` | JSON | HIGH | REDACT_JSON |
| latency-crash-lab-artifacts | `uvb76/cmd/uvb76-latency-crash-lab/**/*.json` | JSON | HIGH | REDACT_JSON |
| targets-crash-lab-artifacts | `uvb76/cmd/uvb76-targets-crash-lab/**/*.json` | JSON | HIGH | REDACT_JSON |
| memory-lab-artifacts | `uvb76/cmd/uvb76-memory-lab/**/*.json` | JSON | HIGH | REDACT_JSON |
| fuzz-corpus | `uvb76/state/testdata/fuzz/**/*` | FUZZ | HIGH | REDACT_JSON |
| diag-capture-packets | `artifacts/**/*-packet.json` | JSON | HIGH | REDACT_JSON |
| memory-lab-evidence | `artifacts/memory-labs/**/*.json` | JSON | HIGH | REDACT_JSON |

### Phase 2: Secret Classes

**File**: `uvb76/internal/redact/redact.go`

Prohibited secret classes:
| Rule ID | Class | Description |
|---------|-------|-------------|
| UVB76-SECRET-0001 | PrivateKeyBlock | PEM private key block (BEGIN PRIVATE KEY, etc.) |
| UVB76-SECRET-0002 | BearerAuth | Bearer authorization tokens |
| UVB76-SECRET-0003 | BasicAuth | Basic authentication credentials |
| UVB76-SECRET-0004 | SessionCookie | Session cookies/tokens |
| UVB76-SECRET-0005 | PasswordField | Plaintext password fields |
| UVB76-SECRET-0006 | TokenField | API token fields |
| UVB76-SECRET-0007 | CredentialURL | URLs with embedded credentials |
| UVB76-SECRET-0008 | ClientKeyData | Client key data |
| UVB76-SECRET-0009 | SensitiveQuery | Sensitive URL query parameters |
| UVB76-SECRET-0010 | PasswordHash | Password hash values (sha256:) |
| UVB76-SECRET-0011 | ProxyAuth | Proxy authentication |
| UVB76-SECRET-0012 | APIKeyHeader | API key headers |

### Phase 3: Pure Redaction Boundary

**File**: `uvb76/internal/redact/redact.go`

Key exports:
- `Redacted` constant: `[REDACTED]`
- `DetectSecret(input string) string` - Returns rule ID if secret detected
- `RedactHeaders(headers http.Header) http.Header` - Sanitize HTTP headers
- `RedactURL(rawURL string) string` - Sanitize URLs
- `RedactConfigValue(fieldName, value string) string` - Sanitize config fields
- `RedactStructuredJSON(data []byte) ([]byte, error)` - Sanitize JSON
- `RedactArtifactValue(input string) string` - Sanitize for persistence
- `ContainsSecret(input string) bool` - Check for secrets

Properties:
- Pure: No filesystem, network, environment, or clock access
- Deterministic: Same input always produces same output
- Idempotent: `Redact(Redact(value)) == Redact(value)`

### Phase 4: Repository Verifier

**File**: `scripts/verify_uvb76_artifact_secret_hygiene.py`

Two-tier scanning:
1. **Universal rules**: Apply to ALL artifacts regardless of context
2. **Artifact-context rules**: Apply based on artifact type/sensitivity

Bounded execution:
- `MAX_FILE_SIZE`: 1MB per file
- `MAX_FILES_SCANNED`: 10,000 files
- `MAX_DIAGNOSTICS`: 100 findings

Diagnostic safety: Never expose detected secret values in output.

### Phase 5: Gate Integration

**Target**: `make hulk-uvb76-artifact-secret-gate`

```bash
make hulk-uvb76-artifact-secret-gate
```

---

## Key Files Created

| File | Purpose |
|------|---------|
| `uvb76/internal/redact/redact.go` | Pure redaction boundary (351 lines) |
| `uvb76/internal/redact/redact_diagnostic_test.go` | Secret detection and diagnostic tests |
| `uvb76/internal/redact/redact_json_test.go` | JSON redaction tests |
| `scripts/uvb76_artifact_secret_hygiene/__init__.py` | Module entry point |
| `scripts/uvb76_artifact_secret_hygiene/main.py` | Main verifier entry point |
| `scripts/uvb76_artifact_secret_hygiene/inventory.py` | Artifact surface inventory (17 surfaces) |
| `scripts/uvb76_artifact_secret_hygiene/scanner.py` | Secret scanner |
| `scripts/uvb76_artifact_secret_hygiene/tests.py` | Self-test suite |
| `docs/epics/act-uvb76-hulk05-artifact-secret-hygiene.md` | This epic document |

---

## Verification

### Run the gate

```bash
make hulk-uvb76-artifact-secret-gate
```

### Run self-test

```bash
python3 scripts/verify_uvb76_artifact_secret_hygiene.py --self-test
```

### Expected output

```
=== UVB-76 Hulk Gate: Artifact Secret Hygiene ===
=== Verifier Self-Test ===
Running self-test mode...
  ✓ Inventory validation passed
  ✓ N universal rules compiled
  ✓ N context rules compiled
  ✓ Pattern detection tests passed (N cases)
  ✓ Idempotence verification logic present
  ✓ Verifier completed without errors in self-test mode

✓ All self-tests passed
=== Scanning Artifact Surfaces ===
✓ Artifact secret hygiene: PASSED
  Files scanned: N
=== UVB-76 Hulk Gate: Artifact Secret Hygiene PASSED ===
```

---

## Doctrine

### Adding New Artifact Surfaces

1. Add entry to `ARTIFACT_INVENTORY` in `scripts/uvb76_artifact_secret_hygiene/inventory.py`
2. Specify: path, format, sensitivity, sanitizer, rule_set
3. Run `make hulk-uvb76-artifact-secret-gate` to verify

### Artifact Producer Requirements

Artifact-producing code must:
1. Use `redact.RedactArtifactValue()` before persistence
2. Use `redact.RedactHeaders()` for HTTP header artifacts
3. Use `redact.RedactURL()` for URL artifacts
4. Use `redact.RedactStructuredJSON()` for JSON artifacts

### Diagnostic Safety

Diagnostics must NEVER expose detected values:
- Print rule IDs only: `UVB76-SECRET-0005`
- Print field names: `password_hash`
- Never print: actual token, password, or key values

---

## Acceptance Criteria

- [x] Artifact surface inventory created and validated
- [x] Canonical secret classes defined (UVB76-SECRET-0001 through 0012)
- [x] Pure redaction boundary implemented in Go
- [x] Redaction contract tests exist and pass
- [x] Repository verifier with bounded execution
- [x] Verifier self-test mode
- [x] Example config sanitized (`password_hash: "[REDACTED]"`)
- [x] Gate target added to Makefile
- [x] Gate runs successfully
- [x] Epic document created

---

## Verification Results

### make hulk-uvb76-artifact-secret-gate

```
=== UVB-76 Hulk Gate: Artifact Secret Hygiene ===
=== Verifier Self-Test ===
SELF-TEST SUMMARY: 12/12 passed
All self-tests passed!
=== Go Redaction Tests ===
PASS ok github.com/s1onique/KGB/uvb76/internal/redact
=== Scanning Artifact Surfaces ===
=== UVB-76 Artifact Secret Hygiene Verifier (HULK05) ===
OK: Inventory valid (17 surfaces)
Found 1196 candidate files
Scanned 1194 files
Matched 46 artifact surface files
OK: No prohibited secrets detected
VERIFICATION PASSED
=== UVB-76 Hulk Gate: Artifact Secret Hygiene PASSED ===
```

Exit code: 0

### make gate

```
[gate] checking memory attribution matrix workflow shape self-test
Results: 9 passed, 0 failed
[gate] checking memory attribution matrix workflow shape
Valid: True
[gate] checking memory allocation ownership hygiene
[PASS] Memory ownership hygiene gate passed.
[gate] PASS
```

Exit code: 0

---

## See Also

- `uvb76/internal/redact/redact.go` — Pure redaction boundary
- `uvb76/internal/redact/redact_diagnostic_test.go` — Secret detection tests
- `uvb76/internal/redact/redact_json_test.go` — JSON redaction tests
- `scripts/uvb76_artifact_secret_hygiene/inventory.py` — Artifact inventory
- `scripts/uvb76_artifact_secret_hygiene/main.py` — Verifier entry point
- `docs/doctrine/privacy.md` — Privacy doctrine
- `docs/doctrine/kgb.md` — KGB architecture doctrine
