# KGB/Tovarisch Secrets and Log Redaction Policy

## Purpose

This policy defines sensitive data classifications and redaction requirements for KGB/tovarisch outputs: logs, status JSON, metrics JSON, CLI stderr, and future UVB-76 reports.

**Goal**: Prevent accidental disclosure of secrets, credentials, or highly sensitive infrastructure details while maintaining useful operational observability.

## Sensitive Data Classes

### Class S1: Critical Secrets (Never Print)

These values must **never** appear in any output (logs, status, metrics, CLI stderr).

| Data Type | Examples | Rationale |
|-----------|----------|-----------|
| Signing keys | Ed25519 private key bytes, seed material | Full compromise of operator intent |
| Bearer tokens | `Authorization: Bearer <token>` header values | Session hijacking |
| Private key material | WireGuard private key, tunnel PSK bytes | Tunnel compromise |
| API credentials | UVB-76 API keys, third-party tokens | Unauthorized UVB-76 access |
| Full config contents | Raw YAML with all values intact | Infrastructure enumeration |

### Class S2: Sensitive Endpoints (Redact Defaults)

These values require redaction when exposed in outputs.

| Data Type | Redaction Rule | Rationale |
|-----------|----------------|------------|
| UVB-76 URLs with credentials | Strip credentials and query strings | Credential disclosure |
| Tunnel endpoint addresses | Allow IP/port only, strip any embedded secrets | Infrastructure enumeration |
| Full filesystem paths | Keep basename or category; redact operator identity | Privacy, operator anonymity |
| Raw error strings from external libs | Truncate or summarize; never dump full chain | Internal state disclosure |

### Class S3: Identifier-Sensitive (Limited Exposure OK)

These values **may** appear in outputs with limitations.

| Data Type | Exposure Rule | Rationale |
|-----------|---------------|-----------|
| `node_id` | Full value allowed in status/metrics; no prefix truncation needed for now | Identifies leaf node, not secret per privacy doctrine |
| Interface names | Full name allowed (`eth0`, `wg0`); counts allowed | Topology hints acceptable in private network context |
| Interface counters | Full values allowed (rx_bytes, tx_bytes, etc.) | Infrastructure health, not personal data |
| Process IDs | Full value allowed | Operational telemetry, not secret |

### Class N: Non-Sensitive (Print Freely)

These values are safe to include in all outputs.

| Data Type | Examples | Rationale |
|-----------|----------|-----------|
| Service names | `tovarisch` | No sensitive content |
| Version strings | `0.1.1` | Non-sensitive metadata |
| Status strings | `ok`, `warn`, `error` | Operationally necessary |
| Port numbers | `8317` | Non-sensitive infrastructure facts |
| Error names (from tovarisch) | `AcceptFailed`, `BindFailed` | Internal but not secret |
| Health check summaries | `running`, `not configured yet` | Non-sensitive status messages |

---

## Output-Specific Policy

### Logs (NDJSON to stdout/stderr)

**Policy**: Structured logs are operator-visible output. All Class S1 and S2 data must be redacted.

Rules:
- Never emit raw config values, tokens, or keys
- Error messages: use tovarisch error names, not external library error chains
- URLs: strip credentials before logging
- Paths: emit only category or basename, never full path
- Marker: use `<redacted>` for unknown sensitive fields

Example:
```
# Bad - exposes credential
{"level":"info","event":"config_loaded","fields":{"uvb76_url":"https://token:secret@uvb76.local/api"}}

# Good - stripped URL
{"level":"info","event":"config_loaded","fields":{"uvb76_url":"https://uvb76.local/api"}}
```

### Status JSON (`/status`, `/status.json`)

**Policy**: Local-only diagnostic endpoint. Class S1 excluded; Class S3 allowed.

Rules:
- `node_id`: allowed (identifier-sensitive, not secret per privacy doctrine)
- Check `detail` strings: avoid including sensitive values; use generic summaries
- Runtime (pid, rss_kib): allowed (operational telemetry)
- Never include tokens, keys, or full config in status

Example status check detail:
```
# Good - generic summary
{"name":"config","status":"warn","detail":"not configured yet"}

# Acceptable - operational fact without secret
{"name":"tunnel","status":"ok","detail":"wg0 endpoint 10.0.0.2:51820"}
```

### Metrics JSON (`/metrics.json`)

**Policy**: Private-interface endpoint. Topology exposure acceptable within private network context.

Rules:
- Interface names: allowed (`eth0`, `wg0`, `tun0`)
- Interface counters: allowed (rx_bytes, tx_bytes, rates)
- `node_id`: allowed (identifier-sensitive, not secret)
- Never include tunnel encryption keys or bearer tokens
- Keep interface counters in human-readable form (bytes, not hex)

### CLI stderr

**Policy**: Operator-visible output. Same redaction as logs.

Rules:
- Error messages: use concise summaries, not full stack traces from external libs
- Config validation: indicate missing fields without showing expected values
- Startup failures: report what failed and why, not internal state dumps

### Future Audit Logs

**Policy**: Long-term records. Strict redaction required.

Rules:
- All Class S1 data: never logged
- Class S2 data: redact by default, log with explicit flag only
- Timestamps: include for forensics
- Actor: log operator identity (from signed desired-state), not raw credentials

### Future UVB-76 Reports

**Policy**: Inter-node communication. Signed, encrypted.

Rules:
- Class S1 data: never included
- Class S2 data: include only if operationally necessary
- Class S3 data: allowed (node_id for routing)
- Reports must be signed to prevent tampering

---

## Redaction Patterns

### Pattern R1: Never Print Full Secrets

**Rule**: Any field identified as Class S1 must be replaced entirely.

```
# Before
{"token": "sk_live_abc123xyz"}

# After
{"token": "<redacted>"}
```

### Pattern R2: Fixed Markers

**Rule**: Use consistent `<redacted>` marker for unknown sensitive fields.

Use this when uncertain whether a field is sensitive:
- Unknown: `<redacted>`
- Known safe identifier: print as-is
- Known sensitive: `<redacted>`

### Pattern R3: Identifier Prefixes Only When Useful

**Rule**: For node_id and similar identifiers, full value is acceptable.

Rationale: node_id identifies the leaf node but does not reveal operator identity or credentials. Full value helps debugging.

```
# Allowed - node_id in full
{"node_id": "leaf-001"}

# Not needed - prefix truncation adds no security value
{"node_id": "lea..."}
```

### Pattern R4: Path Basename or Category

**Rule**: For filesystem paths, extract useful category without full path.

```
# Full path - problematic
"/home/operator/.config/kgb/tunnels.yaml"

# Basename only - acceptable
{"config_file": "tunnels.yaml"}

# Category only - also acceptable
{"config_dir": ".config/kgb"}
```

### Pattern R5: URL Credential Stripping

**Rule**: Remove credentials and query strings from URLs before logging or status.

```
# Before
https://user:password@uvb76.internal/api?debug=true

# After
https://uvb76.internal/api
```

**Implementation**: Strip scheme, credentials, and query before emitting.

### Pattern R6: Error Chain Truncation

**Rule**: Truncate external library error chains to tovarisch error names.

```
# Before - external library error chain
"error": "BindFailed: failed to bind to 0.0.0.0:8317 - network unavailable - permission denied - EINVAL"

# After - tovarisch error only
"error": "BindFailed"
```

---

## Forbidden Logging Patterns

These patterns must never appear in any KGB/tovarisch output:

### F1: Raw Config Dump

```
# Forbidden
{"event":"config_loaded","config":{"uvb76_token":"sk_live_xxx","wg_private_key":"base64..."}}
```

**Fix**: Log config validation result only, not full contents.

### F2: Environment Variable Dump

```
# Forbidden
{"event":"app_startup","env":{"UVB76_TOKEN":"sk_live_xxx","HOME":"/home/operator"}}
```

**Fix**: Log startup parameters only, not environment.

### F3: Token or Key Values

```
# Forbidden
{"field":"token","value":"sk_live_abc123xyz"}
```

**Fix**: Use `<redacted>` or omit field entirely.

### F4: Private Key Bytes

```
# Forbidden
{"field":"wg_private_key","value":"base64_encoded_private_key_bytes"}
```

**Fix**: Log key existence only, not contents.

### F5: Full Authorization Headers

```
# Forbidden
{"header":"authorization","value":"Bearer sk_live_xxx"}
```

**Fix**: Log header presence only, not value.

### F6: Unbounded Error Chains

```
# Forbidden
{"error":"MultiError{outer{wrapped{deep{external_lib_error}}}}"}
```

**Fix**: Truncate to tovarisch error name; log wrapped error count only if needed.

### F7: Full Filesystem Paths

```
# Forbidden
{"config_path":"/home/operator/.config/kgb/config.yaml"}
```

**Fix**: Use basename or relative category.

---

## Redaction Implementation Status

| Component | Status | Notes |
|-----------|--------|-------|
| Log output | **Requires implementation** | Error names are used; full redaction framework deferred |
| Status JSON | **Adequate for v0** | No sensitive fields currently emitted |
| Metrics JSON | **Adequate for v0** | Topology exposure acceptable; no secrets |
| CLI stderr | **Requires implementation** | Error handling needs redaction review |
| Audit logs | **Deferred** | Future feature |
| UVB-76 reports | **Deferred** | Future feature |

---

## Integration Points

### tovarisch logging.zig

Currently emits structured NDJSON with event types. Error events use `@errorName(err)` which provides tovarisch error names (e.g., `AcceptFailed`, `BindFailed`). This is acceptable as these are internal error names, not external library chains.

Future work:
- Add redaction utility function for URL stripping
- Add config field sanitization before logging

### tovarisch status.zig

Currently emits node_id and check details. No sensitive values in current implementation. Check details are static strings (e.g., `running`, `not configured yet`).

Future work:
- Add redaction review when dynamic config values are added to status

### tovarisch http/server.zig

Server errors currently print errno values to stderr via `std.debug.print`. This is low-sensitivity but could be improved with structured logging.

---

## Review Triggers

This policy must be updated when:

1. New config fields added with sensitive values
2. New HTTP endpoints added with user-controlled input
3. UVB-76 communication implemented
4. Audit logging implemented
5. New output formats added (e.g., Prometheus metrics)

---

## References

- [KGB security doctrine](./doctrine.md)
- [KGB threat model](./threat-model.md)
- [Privacy doctrine](../doctrine/privacy.md)
- [Accepted risks ledger](./accepted-risks.md) — R-006 tracks no-secrets-redaction risk
- [tovarisch-status-v0 contract](../contracts/tovarisch-status-v0.md)
- [tovarisch-http-v0 contract](../contracts/tovarisch-http-v0.md)
