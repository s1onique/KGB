# KGB Accepted Risks Ledger

This ledger tracks known, accepted risks with explicit ownership, reasoning, and review triggers.

## Ledger Format

Each risk entry contains:
- **ID**: unique identifier (R-XXX)
- **Description**: what the risk is
- **Owner**: who is responsible for monitoring
- **Reason**: why this risk is accepted
- **Expiry/Review Trigger**: when to revisit
- **Mitigation**: current mitigations or planned fixes

---

## R-001: No Node Identity Yet

**Description**: tovarisch has no formal cryptographic identity. Reports cannot be verified as coming from a specific node.

**Owner**: maintainer

**Reason**: Identity bootstrapping requires design work before implementation. Current threat model assumes physical control of tovarisch machines.

**Expiry/Review Trigger**: 
- Implement node identity before any multi-node deployment
- Revisit when UVB-76 federation is designed
- Minimum: before ACT 5 (config integrity design)

**Mitigation**:
- Physical control of machines assumed
- Network segmentation limits exposure
- HTTPS provides transport auth
- Planned: Ed25519 key per tovarisch instance (ACT 5)

---

## R-002: No Signed Config Bundles Yet

**Description**: Config files are plain YAML. Attacker with file system access can modify tunnel endpoints, probe targets, UVB-76 URLs.

**Owner**: maintainer

**Reason**: Signed config bundles require node identity first. Plain text config is acceptable for single-user self-hosted deployments.

**Expiry/Review Trigger**: 
- Implement before any shared or hostile-environment deployment
- Revisit when node identity is established (R-001 resolved)
- Minimum: before ACT 5 (config integrity design sketch)

**Mitigation**:
- Config file ownership by operator
- Minimal permissions on config file (0600)
- Local-only config by default
- Planned: signed config bundles with Ed25519 (ACT 5)

---

## R-003: No Release Signing Yet

**Description**: Release .deb packages are downloaded over HTTPS but not signed. Attacker could replace release with malicious version.

**Owner**: maintainer

**Reason**: Release signing requires key infrastructure (sigstore/cosign). Manual verification by operator is current mitigation.

**Expiry/Review Trigger**:
- Implement before any non-manual deployment automation
- Revisit when release pipeline is established
- Minimum: before ACT 4 (release artifact trust doctrine)

**Mitigation**:
- HTTPS for release downloads
- Operator verification via SHA256 checksums (manual)
- Planned: cosign signatures for all release artifacts (ACT 4)

---

## R-004: No Fuzzing Yet

**Description**: File parsers (config YAML, JSON status, network data) have not been fuzz-tested. Unknown bugs may exist.

**Owner**: maintainer

**Reason**: Fuzzing requires infrastructure setup (fuzzing corpus, CI integration). Current unit tests provide basic coverage.

**Expiry/Review Trigger**:
- Before any user-facing release
- When any new file parser is added
- Minimum: future security epic

**Mitigation**:
- Unit tests for critical paths
- Structured input validation
- Minimal external input surface
- Planned: libFuzzer integration (future epic)

---

## R-005: No Privilege Separation Yet

**Description**: tovarisch runs as the same user that started it (typically root or a privileged user). Could expand blast radius if compromised.

**Owner**: maintainer

**Reason**: Full privilege separation requires careful capability analysis. Default install should work without complex SELinux/AppArmor profiles.

**Expiry/Review Trigger**:
- Before any hostile-environment deployment
- When tovarisch gains network listeners
- Minimum: future security epic

**Mitigation**:
- Systemd unit with MemoryMax, CPUWeight limits
- No setuid binaries
- Minimal filesystem access
- Planned: capabilities-based sandbox (future epic)

---

## R-006: No Secrets Redaction in Logs

**Description**: Structured logs may contain sensitive config values, tunnel endpoints, or error messages with full paths.

**Owner**: maintainer

**Reason**: Redaction requires knowing what is sensitive. Current logs are local-only by default (localhost binding).

**Expiry/Review Trigger**: 
- Before any log aggregation setup
- When logs leave the local machine
- Re-evaluate when config parsing is implemented

**Mitigation (Partially Mitigated)**:
- Logs on localhost by default
- No log forwarding in default config
- **ACT 3 complete**: Redaction doctrine established with sensitive data classes and patterns
  - [redaction-policy.md](./redaction-policy.md) defines Class S1 (critical secrets), Class S2 (sensitive endpoints), Class S3 (identifier-sensitive), Class N (non-sensitive)
  - Patterns R1-R6 provide concrete redaction guidance
  - Forbidden patterns F1-F7 explicitly enumerate what must not appear in logs
- **Implementation status**: Status and metrics JSON are adequate for v0; log output and CLI stderr require future implementation when config parsing is added
- **Planned**: Full redaction implementation when config fields are added (see redaction-policy.md integration points)

---

## R-007: No HTTPS Certificate Pinning

**Description**: tovarisch trusts any certificate presented by UVB-76. Attacker could run a fake UVB-76 with a valid certificate.

**Owner**: maintainer

**Reason**: Certificate pinning requires known-good certificate or SPKI. Current design assumes UVB-76 runs on trusted infrastructure.

**Expiry/Review Trigger**:
- Before any public-facing deployment
- When UVB-76 DNS is not under operator control
- Minimum: future security epic

**Mitigation**:
- UVB-76 on trusted infrastructure
- HTTPS provides transport security
- Planned: SPKI pin or trust-on-first-use (future epic)

---

## R-008: No Audit Log on tovarisch

**Description**: tovarisch does not maintain an audit log of operator actions, config changes, or tunnel events. Post-incident forensics limited.

**Owner**: maintainer

**Reason**: Audit logging adds complexity and storage. Current structured logs provide some traceability.

**Expiry/Review Trigger**:
- Before any compliance-required deployment
- When multi-operator environment is designed
- Minimum: future security epic

**Mitigation**:
- Structured logs to stdout (JSON)
- Operator can redirect to file/log aggregator
- systemd journal integration
- Planned: audit log with rotation (future epic)

---

## Risk Review Process

1. Before any ACT completion, review accepted risks
2. If new risk identified, add to ledger with owner and review trigger
3. If risk resolved, mark as "Resolved" with date and evidence
4. If risk becomes unacceptable, escalate to epic scope

## Risk Acceptance Criteria

A risk may be accepted when:
- Clear owner assigned
- Reason documented
- Review trigger defined
- Mitigation path identified
- Risk is explicit, not hidden

Do not accept risks by accident.
