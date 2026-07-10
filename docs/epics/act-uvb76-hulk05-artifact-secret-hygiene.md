# Epic: UVB-76 Platform and Projection Hardening

**Epic ID**: ACT-UVB76-HULK05

**Status**: ACTIVE

**Started**: 2026-07-10

**R2**: Rule Registry Convergence (ACT-UVB76-HULK05R2) — COMPLETE

**R3**: Typed Redaction Semantics (ACT-UVB76-HULK05R3) — COMPLETE

**R3R2**: Ownership Contract Mutation Truth (ACT-UVB76-HULK05R3R2) — COMPLETE

---

## Executive Summary

The UVB-76 Platform and Projection Hardening epic establishes comprehensive artifact-secret-hygiene contracts to prevent credential leakage through tracked artifacts. This epic creates a repository-wide, deterministic, self-tested system for identifying prohibited secret classes in artifacts and enforcing canonical redaction behavior.

**R2 adds**: Canonical secret-rule registry with one authoritative vocabulary shared by Python, Go, tests, and documentation.

**R3 adds**: Typed redaction APIs with explicit handling for each data shape (cookies, headers, URLs, structured JSON). Generic APIs are renamed to reflect narrow scope.

---

## Motivation

UVB-76 generates and stores diagnostic artifacts (captured packets, memory evidence, lab results) that may contain sensitive infrastructure metadata. Without explicit hygiene contracts:

- Artifacts may accidentally include session tokens, API keys, or passwords
- Committed evidence becomes a credential sprawl risk
- No systematic way to verify artifacts are safe before commit
- Diagnostics may inadvertently expose secrets when used for debugging

**R2 motivation**: Conflicting rule identities between Python and Go, contextual rules outside registry, and documentation describing different mappings. Goal: one ID has exactly one meaning everywhere.

---

## Scope

### In Scope

- Repository-wide artifact surface inventory
- Prohibited secret class definitions (canonical registry)
- Pure deterministic redaction boundary (Go package)
- Repository verifier with bounded execution
- Self-test for verifier including registry mutation tests
- Dedicated gate target (`make hulk-uvb76-artifact-secret-gate`)
- Example config sanitization
- **R2**: Canonical rule registry with validated projections

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
| ACT-UVB76-HULK05 | Artifact Secret Hygiene | `uvb76/internal/redact/redact.go` | `hulk-uvb76-artifact-secret-gate` | OPEN |

### HULK05R2 Sub-tasks: Registry Convergence

| Sub-task | Focus | Status |
|----------|-------|--------|
| R2-1 | Canonical rule registry | COMPLETE |
| R2-2 | Python projection from registry | COMPLETE |
| R2-3 | Go constants agree with registry | COMPLETE |
| R2-4 | Structured JSON detector coverage | COMPLETE |
| R2-5 | Registry mutation self-tests | COMPLETE |
| R2-6 | Gate integration | COMPLETE |

### HULK05R3 Sub-tasks: Typed Redaction Semantics

| Sub-task | Focus | Status |
|----------|-------|--------|
| R3-1 | Split request Cookie and Set-Cookie APIs | COMPLETE |
| R3-2 | Make header redaction typed | COMPLETE |
| R3-3 | Rename generic APIs to reflect narrow scope | COMPLETE |
| R3-4 | URL redaction bounds and semantics | COMPLETE |
| R3-5 | Structured JSON redaction completeness | COMPLETE |
| R3-6 | Typed Finding result model | COMPLETE |
| R3-7 | Go implementation split (headers.go, cookies.go, findings.go) | COMPLETE |

### HULK05R4 Sub-tasks: Producer Enforcement

| Sub-task | Focus | Status |
|----------|-------|--------|
| R4-1 | Artifact producer validation | NEXT |

---

## Canonical Rule Registry

**File**: `scripts/uvb76_artifact_secret_hygiene/registry.json`

### Registry Schema

Each rule entry contains:

```json
{
  "rule_id": "UVB76-SECRET-XXXX",
  "class": "snake_case_class_name",
  "scope": "universal|artifact_context",
  "severity": "critical|high|medium",
  "allowlistable": false,
  "safe_explanation": "human-readable explanation",
  "safe_remediation": "human-readable remediation",
  "detector_kind": "pattern|field_name|header_name|url_component|structured_json"
}
```

### Universal Scope Rules (applied to ALL files)

| Rule ID | Class | Description | Detector |
|---------|-------|-------------|----------|
| UVB76-SECRET-0001 | private_key_pem | Generic private key PEM block | pattern |
| UVB76-SECRET-0002 | encrypted_private_key_pem | Encrypted private key PEM | pattern |
| UVB76-SECRET-0003 | rsa_private_key_pem | RSA private key PEM | pattern |
| UVB76-SECRET-0004 | ec_private_key_pem | EC private key PEM | pattern |
| UVB76-SECRET-0005 | openssh_private_key_pem | OpenSSH private key PEM | pattern |

### Artifact Context Scope Rules

| Rule ID | Class | Description | Detector |
|---------|-------|-------------|----------|
| UVB76-SECRET-0010 | authorization_bearer | Authorization Bearer header | header_name |
| UVB76-SECRET-0011 | authorization_basic | Authorization Basic header | header_name |
| UVB76-SECRET-0012 | proxy_authorization | Proxy-Authorization header | header_name |
| UVB76-SECRET-0013 | api_key_header | X-API-Key header | header_name |
| UVB76-SECRET-0020 | session_token_header | X-Session-Token header | header_name |
| UVB76-SECRET-0030 | cookie_credential | Cookie header with credentials | header_name |
| UVB76-SECRET-0031 | set_cookie_credential | Set-Cookie with credentials | header_name |
| UVB76-SECRET-0032 | uvb76_session_cookie | uvb76_session cookie value | pattern |
| UVB76-SECRET-0040 | password_field | Password field values | field_name |
| UVB76-SECRET-0041 | password_hash_field | Password hash field values | field_name |
| UVB76-SECRET-0050 | generic_token_field | Generic token field values | field_name |
| UVB76-SECRET-0060 | client_key_data | client_key_data field | field_name |
| UVB76-SECRET-0061 | private_key_data | private_key/private_key_data fields | field_name |
| UVB76-SECRET-0070 | credential_bearing_http_url | URL with embedded credentials | url_component |
| UVB76-SECRET-0071 | credential_bearing_database_dsn | Database DSN with credentials | url_component |
| UVB76-SECRET-0072 | sensitive_url_query_parameter | Sensitive query parameter | url_component |
| UVB76-SECRET-0080 | jwt_like_token | JWT-like token (dot-separated base64) | pattern |
| UVB76-SECRET-0081 | bearer_token_literal | Bearer token literal | pattern |

**Total rules**: 23

---

## Registry Projection Mechanism

### Python Projection

**Source**: `scripts/uvb76_artifact_secret_hygiene/rules.py`

Python rules are derived from the registry at module import time:
- `UNIVERSAL_RULES` - all universal scope rules
- `ARTIFACT_CONTEXT_RULES` - all artifact_context scope rules with pattern detectors

### Structured JSON Scanner

**Source**: `scripts/uvb76_artifact_secret_hygiene/structured_scanner.py`

Handles `field_name` and `structured_json` detector kinds:
- Recursive JSON traversal
- Safe value detection ([REDACTED], null, empty)
- Nested object and array handling

### Go Constants

**Source**: `uvb76/internal/redact/redact.go`

Go constants agree with registry via:
- Matching rule ID values (e.g., `RulePrivateKeyPEM = "UVB76-SECRET-0001"`)
- Comments linking to registry class (e.g., `// private_key_pem`)

---

## Canonical Ownership Mapping

**Source**: `scripts/uvb76_artifact_secret_hygiene/ownership.py`

The ownership mapping provides executable proof that every registry rule has an owner.

### Supported Ownership Kinds (Closed Vocabulary)

| Ownership Kind | Description |
|---------------|-------------|
| `private_key_marker_redactor` | Detects and redacts private key PEM markers |
| `typed_header_redactor` | Handles Authorization, X-API-Key, X-Session-Token headers |
| `request_cookie_redactor` | Handles Cookie header with credentials |
| `set_cookie_redactor` | Handles Set-Cookie header with credentials |
| `config_redactor` | Handles sensitive configuration field values |
| `structured_json_redactor` | Handles nested JSON structures |
| `url_redactor` | Handles URLs with embedded credentials |
| `repository_detection_only` | Detected but not actively redacted in artifacts |

### Ownership Projection

| Rule ID | Class | Ownership |
|---------|-------|-----------|
| UVB76-SECRET-0001 | private_key_pem | private_key_marker_redactor |
| UVB76-SECRET-0002 | encrypted_private_key_pem | private_key_marker_redactor |
| UVB76-SECRET-0003 | rsa_private_key_pem | private_key_marker_redactor |
| UVB76-SECRET-0004 | ec_private_key_pem | private_key_marker_redactor |
| UVB76-SECRET-0005 | openssh_private_key_pem | private_key_marker_redactor |
| UVB76-SECRET-0010 | authorization_bearer | typed_header_redactor |
| UVB76-SECRET-0011 | authorization_basic | typed_header_redactor |
| UVB76-SECRET-0012 | proxy_authorization | typed_header_redactor |
| UVB76-SECRET-0013 | api_key_header | typed_header_redactor |
| UVB76-SECRET-0020 | session_token_header | typed_header_redactor |
| UVB76-SECRET-0030 | cookie_credential | request_cookie_redactor |
| UVB76-SECRET-0031 | set_cookie_credential | set_cookie_redactor |
| UVB76-SECRET-0032 | uvb76_session_cookie | request_cookie_redactor |
| UVB76-SECRET-0040 | password_field | structured_json_redactor, config_redactor |
| UVB76-SECRET-0041 | password_hash_field | structured_json_redactor, config_redactor |
| UVB76-SECRET-0050 | generic_token_field | structured_json_redactor, config_redactor |
| UVB76-SECRET-0060 | client_key_data | structured_json_redactor |
| UVB76-SECRET-0061 | private_key_data | structured_json_redactor |
| UVB76-SECRET-0070 | credential_bearing_http_url | url_redactor |
| UVB76-SECRET-0071 | credential_bearing_database_dsn | url_redactor |
| UVB76-SECRET-0072 | sensitive_url_query_parameter | url_redactor |
| UVB76-SECRET-0080 | jwt_like_token | repository_detection_only |
| UVB76-SECRET-0081 | bearer_token_literal | repository_detection_only |

**Total rules**: 23

**Total ownership assignments**: 26

---

## Drift Detection

The self-test suite includes mutation tests that verify:

- Duplicate rule IDs are rejected
- Unknown scanner IDs are rejected
- Missing explanation/remediation is rejected
- Invalid scope values are rejected
- Invalid detector_kind values are rejected
- Python rules must derive from registry
- Go constants must have matching registry entries
- Ownership mapping must have all registry rules

---

## Split Test Files

| File | Purpose |
|------|---------|
| `scripts/uvb76_artifact_secret_hygiene/__init__.py` | Module entry point |
| `scripts/uvb76_artifact_secret_hygiene/registry.json` | Canonical rule registry |
| `scripts/uvb76_artifact_secret_hygiene/ownership.py` | Canonical ownership mapping |
| `scripts/uvb76_artifact_secret_hygiene/registry_loader.py` | Registry loading/validation |
| `scripts/uvb76_artifact_secret_hygiene/rules.py` | Registry-derived rules |
| `scripts/uvb76_artifact_secret_hygiene/structured_scanner.py` | Structured JSON scanning |
| `scripts/uvb76_artifact_secret_hygiene/scanner.py` | File scanning |
| `scripts/uvb76_artifact_secret_hygiene/inventory.py` | Artifact surface inventory |
| `scripts/uvb76_artifact_secret_hygiene/main.py` | Main verifier |
| `scripts/uvb76_artifact_secret_hygiene/tests/` | Self-test suites |
| `scripts/uvb76_artifact_secret_hygiene/tests/go_parser_tests.py` | Go AST parser tests |
| `scripts/uvb76_artifact_secret_hygiene/tests/positive_tests.py` | Positive behavior tests |
| `scripts/uvb76_artifact_secret_hygiene/tests/registry_tests.py` | Registry consistency tests |
| `scripts/uvb76_artifact_secret_hygiene/tests/ownership_tests.py` | Ownership mapping tests |
| `uvb76/internal/redact/redact.go` | Go redaction boundary |
| `uvb76/internal/redact/headers.go` | Typed header redaction |
| `uvb76/internal/redact/cookies.go` | Typed Cookie/Set-Cookie redaction |
| `uvb76/internal/redact/findings.go` | Typed Finding result model |
| `uvb76/internal/redact/*_test.go` | Go tests |
| `docs/epics/act-uvb76-hulk05-artifact-secret-hygiene.md` | This epic |

---

## Typed Redaction APIs (R3)

### Cookie Redaction

| API | Purpose |
|-----|---------|
| `RedactRequestCookieHeader(cookie)` | Redacts all name=value pairs in Cookie header |
| `RedactSetCookieHeader(cookie)` | Redacts cookie value, preserves attributes |

**Request Cookie semantics**: All name=value pairs are credentials. Names like "path", "domain" are NOT treated as attributes.

**Set-Cookie semantics**: First name=value is the cookie value (redacted). Recognized attributes are preserved.

### Header Redaction

| API | Purpose |
|-----|---------|
| `RedactHeaders(headers)` | Main entry point, dispatches to typed functions |
| `RedactAuthorizationHeader(value)` | Redacts Authorization header |
| `RedactProxyAuthorizationHeader(value)` | Redacts Proxy-Authorization header |
| `RedactAPIKeyHeader(value)` | Redacts X-API-Key header |
| `RedactSessionTokenHeader(value)` | Redacts X-Session-Token header |

### Generic Artifact Redaction (Narrow-Named)

| API | Coverage |
|-----|----------|
| `DetectPrivateKeyMarker(input)` | Private key PEM markers only |
| `ContainsPrivateKeyMarker(input)` | Private key PEM markers only |
| `RedactPrivateKeyMarkers(input)` | Private key PEM markers only |

**Note**: These functions are intentionally narrow. Use typed redactors for headers, URLs, cookies, and structured data.

### URL Redaction

| Property | Value |
|----------|-------|
| Userinfo | Redacted (username and password) |
| Safe query params | Preserved |
| Sensitive query params | Redacted |
| Bounds | maxURLLength=65536, maxQueryParams=100 |

### Structured JSON Redaction

| Field Class | Status |
|-------------|--------|
| password_field | Supported |
| password_hash_field | Supported |
| generic_token_field | Supported |
| client_key_data | Supported |
| private_key_data | Supported |
| private_key_pem (in strings) | Supported |

---

## Verification

### Run self-test

```bash
python3 scripts/verify_uvb76_artifact_secret_hygiene.py --self-test
```

### Run gate

```bash
make hulk-uvb76-artifact-secret-gate
```

### Run all gates

```bash
make hulk-uvb76-gate
make hulk-uvb76-capture-gate
make hulk-uvb76-latency-gate
make hulk-uvb76-reachability-gate
make gate
```

---

## Acceptance Criteria (R2)

- [x] One authoritative registry contains all supported classes
- [x] Every rule ID has one meaning
- [x] Python universal and contextual rules derive from or validate against it
- [x] Go rule constants agree with it
- [x] Documentation agrees with it
- [x] Unknown or duplicated rules fail verification
- [x] Missing detector projections fail verification
- [x] Contextual structured-secret classes are covered
- [x] Diagnostics cannot disclose matched values
- [x] Mutation self-tests prove registry-drift rejection
- [x] Dedicated HULK05 gate executes the registry checks
- [x] Existing HULK gates remain green
- [x] `make gate` passes

## Acceptance Criteria (R3)

- [x] Request Cookie and response Set-Cookie use different parsers
- [x] Request cookies never preserve attribute-like cookie names
- [x] Set-Cookie preserves recognized attributes
- [x] Header redaction is case-insensitive and non-mutating
- [x] Generic API names accurately match their coverage
- [x] URL redaction covers userinfo and sensitive queries
- [x] Structured JSON redaction covers nested and array values
- [x] All redactors are deterministic and idempotent
- [x] Diagnostics never contain secret values
- [x] Every registry class has documented ownership
- [x] LLM-friendly exemptions are explicit, reviewed, and documented
- [x] Dedicated HULK05 gate passes
- [x] Aggregate gate passes

## Ownership Contract (R3)

| Metric | Value |
|--------|-------|
| Registry rules | 23 |
| Owned rules | 23 |
| Ownership assignments | 26 |

### Ownership Contract Mutation Truth (R3R2)

**Source**: `scripts/uvb76_artifact_secret_hygiene/tests/ownership_mutation_tests.py`

The ownership validator is parameterized to accept explicit inputs:

```python
def validate_ownership(
    entries: tuple[OwnershipEntry, ...] = OWNERSHIP_ENTRIES,
    registry: dict[str, object] | None = None,
) -> list[str]:
    # Iterates entries directly, NOT RULE_OWNERSHIP
    ...
```

**Key invariants enforced**:
- duplicate rule ID in entries
- empty rule ID
- empty ownership tuple
- duplicate ownership kind within one entry
- unsupported ownership kind
- unknown registry rule ID
- missing registry rule ID

**Mutation test coverage** (6 tests):
1. `test_duplicate_rule_id_detected` - Add duplicate rule ID, require rejection
2. `test_missing_ownership_detected` - Remove one entry, require missing-ownership error
3. `test_unknown_rule_detected` - Append unknown rule ID, require unknown-rule error
4. `test_unsupported_kind_detected` - Use unknown_redactor, require unsupported-kind error
5. `test_empty_ownership_detected` - Use empty tuple, require empty-ownership error
6. `test_duplicate_kind_detected` - Use ("url_redactor", "url_redactor"), require duplicate-kind error

**Projection tests** (8 tests):
- `test_valid_entries_projection_succeeds` - Valid entries return dict
- `test_duplicate_rule_id_raises_error` - Duplicate raises OwnershipValidationError
- `test_empty_ownership_raises_error` - Empty raises OwnershipValidationError
- `test_unknown_rule_raises_error` - Unknown raises OwnershipValidationError
- `test_unsupported_kind_raises_error` - Unsupported raises OwnershipValidationError
- `test_exact_assignment_count` - count_ownership_assignments() == 26
- `test_exact_unique_rule_count` - count_unique_rules() == 23
- `test_no_silent_overwrite` - Validation before projection confirmed

---

## See Also

- `scripts/uvb76_artifact_secret_hygiene/registry.json` — Canonical rule registry
- `scripts/uvb76_artifact_secret_hygiene/ownership.py` — Canonical ownership mapping
- `scripts/uvb76_artifact_secret_hygiene/registry_loader.py` — Registry validation
- `uvb76/internal/redact/redact.go` — Go redaction boundary
- `docs/doctrine/privacy.md` — Privacy doctrine
- `docs/doctrine/kgb.md` — KGB architecture doctrine
