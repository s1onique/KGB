# Epic: KGB Security Doctrine and Threat Model

## Goal

Establish security foundation for KGB/tovarisch with documented doctrine, threat model, accepted risks, and review ceremony.

This is documentation-first. No crypto, auth, or protocol implementation in this epic.

## Non-Goals

- No cryptographic implementation
- No authentication mechanism implementation
- No new protocol design
- No penetration testing (deferred)

## ACT 1: Doctrine and Initial Threat Model

**Status**: Complete

Create foundational security documents:
- Security doctrine (posture, principles, non-goals)
- Threat model (components, assets, trust boundaries, abuse cases, controls)
- Accepted risks ledger (deferred risks with owners and review triggers)
- Security review ceremony (when to review, how to review, required output)
- Epic document (this file)

### ACT 1 Board

| ID | Work Item | Status |
|---|---|---|
| sec-001 | Add `docs/security/doctrine.md` | **done** |
| sec-002 | Add `docs/security/threat-model.md` | **done** |
| sec-003 | Add `docs/security/accepted-risks.md` | **done** |
| sec-004 | Add `docs/security/security-review-ceremony.md` | **done** |
| sec-005 | Add this epic doc | **done** |
| sec-006 | Run `make gate` | **done** |

### ACT 1 Acceptance

- [ ] `docs/security/doctrine.md` exists with Day-0 posture, OWASP framework, Orange Book reference, auth rejection statement, secure defaults principle
- [ ] `docs/security/threat-model.md` exists with component inventory, asset inventory, trust boundaries, abuse case table, controls table (now/near-term/deferred), review triggers
- [ ] `docs/security/accepted-risks.md` exists with at least 5 deferred risks (node identity, signed config, release signing, fuzzing, privilege separation), each with owner, reason, expiry/review trigger, mitigation path
- [ ] `docs/security/security-review-ceremony.md` exists with review triggers (8 types), 6 review questions, lightweight process, required outputs (threat model update, accepted risk, or no-change note)
- [ ] This epic doc exists with 6 ACTs marked appropriately
- [ ] `make gate` passes
- [ ] Files are small and LLM-friendly (< 200 lines each)
- [ ] No large generated diagrams

---

## ACT 2: Route/Listener Attack Surface Inventory

**Status**: Not Started

Inventory all network routes and listeners in tovarisch:
- Document each HTTP endpoint (path, method, auth requirements)
- Document each listener (address, port, binding behavior)
- Identify public vs localhost-only exposure
- Identify data flows for each endpoint
- Map to threat model components and assets

### ACT 2 Board

| ID | Work Item | Status |
|---|---|---|
| sec-007 | Document all HTTP routes in tovarisch | **pending** |
| sec-008 | Document listener binding behavior | **pending** |
| sec-009 | Map data flows for each endpoint | **pending** |
| sec-010 | Update threat model with attack surface | **pending** |
| sec-011 | Run `make gate` | **pending** |

### ACT 2 Acceptance

- [ ] All HTTP routes documented with path, method, purpose, data handled
- [ ] All listeners documented with address, port, binding logic
- [ ] Public binds identified and rationale documented
- [ ] Threat model updated with attack surface inventory section
- [ ] `make gate` passes

---

## ACT 3: Secrets/Log Redaction Doctrine and Tests

**Status**: Not Started

Define what data is sensitive and how to redact it:
- Identify sensitive fields in config, status, logs
- Define redaction patterns (e.g., show first/last 4 chars)
- Document log format and fields that must not be logged
- Add tests for redaction logic

### ACT 3 Board

| ID | Work Item | Status |
|---|---|---|
| sec-012 | Define sensitive data types | **pending** |
| sec-013 | Document redaction patterns | **pending** |
| sec-014 | Implement redaction for known sensitive fields | **pending** |
| sec-015 | Add tests for redaction | **pending** |
| sec-016 | Update threat model with redaction controls | **pending** |
| sec-017 | Update accepted risks (R-006 resolved or updated) | **pending** |
| sec-018 | Run `make gate` | **pending** |

### ACT 3 Acceptance

- [ ] Sensitive data types documented (keys, tokens, endpoints, full paths)
- [ ] Redaction patterns defined for each type
- [ ] Redaction implemented for config, status, logs
- [ ] Unit tests for redaction coverage
- [ ] Threat model updated
- [ ] Accepted risks updated
- [ ] `make gate` passes

---

## ACT 4: Release Artifact Trust Doctrine

**Status**: Not Started

Define how to establish and maintain release artifact trust:
- Document release pipeline (build, sign, distribute)
- Define signing strategy (cosign, sigstore, manual)
- Define artifact verification process
- Document SBOM generation
- Add release signing to gate

### ACT 4 Board

| ID | Work Item | Status |
|---|---|---|
| sec-019 | Document release pipeline | **pending** |
| sec-020 | Define signing strategy | **pending** |
| sec-021 | Implement release signing | **pending** |
| sec-022 | Add SBOM generation | **pending** |
| sec-023 | Add verification to gate | **pending** |
| sec-024 | Update threat model with release trust controls | **pending** |
| sec-025 | Update accepted risks (R-003 resolved or updated) | **pending** |
| sec-026 | Run `make gate` | **pending** |

### ACT 4 Acceptance

- [ ] Release pipeline documented with trust chain
- [ ] Signing strategy defined (cosign recommended)
- [ ] Release signing implemented in CI/CD
- [ ] SBOM generated for releases
- [ ] Verification step added to gate
- [ ] Threat model updated
- [ ] Accepted risks updated (R-003)
- [ ] `make gate` passes

---

## ACT 5: Config Integrity Design Sketch

**Status**: Not Started

Design config integrity without full implementation:
- Define config format and schema
- Define signing mechanism (Ed25519 for tovarisch)
- Define bootstrap ceremony
- Define config update flow
- Sketch migration path from unsigned config

### ACT 5 Board

| ID | Work Item | Status |
|---|---|---|
| sec-027 | Define config schema v1 | **pending** |
| sec-028 | Design signing mechanism | **pending** |
| sec-029 | Design bootstrap ceremony | **pending** |
| sec-030 | Design config update flow | **pending** |
| sec-031 | Sketch migration path | **pending** |
| sec-032 | Document design in contract | **pending** |
| sec-033 | Update threat model with config integrity design | **pending** |
| sec-034 | Update accepted risks (R-001, R-002 updated) | **pending** |
| sec-035 | Run `make gate` | **pending** |

### ACT 5 Acceptance

- [ ] Config schema v1 defined
- [ ] Signing mechanism designed (Ed25519)
- [ ] Bootstrap ceremony designed (key generation, distribution)
- [ ] Config update flow designed (pull, verify, apply, rollback)
- [ ] Migration path from unsigned config sketched
- [ ] Design documented in `docs/contracts/config-v0.md`
- [ ] Threat model updated
- [ ] Accepted risks updated (R-001, R-002)
- [ ] `make gate` passes

---

## ACT 6: Security Gate Integration

**Status**: Not Started

Integrate security checks into the quality gate:
- Add security doc existence checks
- Add accepted risks review trigger to PR checks
- Add security review ceremony checklist
- Add forbidden patterns (hardcoded secrets, default creds)

### ACT 6 Board

| ID | Work Item | Status |
|---|---|---|
| sec-036 | Add security docs existence check to gate | **pending** |
| sec-037 | Add accepted risks review trigger | **pending** |
| sec-038 | Add security review ceremony checklist | **pending** |
| sec-039 | Add forbidden patterns check | **pending** |
| sec-040 | Run `make gate` and verify | **pending** |

### ACT 6 Acceptance

- [ ] Gate checks for `docs/security/` directory
- [ ] Gate checks for all 4 security docs
- [ ] Gate checks for accepted risks with required fields
- [ ] Gate checks for security review ceremony
- [ ] Gate checks for forbidden patterns (secrets, creds)
- [ ] `make gate` passes with security checks

---

## Dependencies

- ACT 2: requires tovarisch routes defined (existing)
- ACT 3: requires tovarisch status/log output (existing)
- ACT 4: requires release pipeline (existing)
- ACT 5: requires node identity design
- ACT 6: requires all prior ACTs

## Future Work (Deferred)

- Implement node identity (requires ACT 5 design)
- Implement signed config bundles (requires ACT 5 design)
- Implement release signing (requires ACT 4 design)
- Fuzzing infrastructure
- Privilege separation
- Penetration testing

## Closure Criteria

Epic is complete when:
- All 6 ACTs are done
- Threat model is comprehensive and maintained
- Accepted risks ledger is current
- Security review ceremony is in use
- Gate passes all security checks
