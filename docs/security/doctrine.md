# KGB Security Doctrine

Security is a Day-0 architectural requirement, not a retrofit.

## Security Posture

KGB operates with these postures:

- **Local-first**: trust local resources before remote ones
- **Paranoid-by-default**: assume hostile conditions unless proven otherwise
- **Least-privilege**: minimum required permissions for each component
- **Auditable**: all state changes leave traceable records
- **Observable**: infrastructure health, not human behavior
- **Hostile-network aware**: assume network degradation, interception, manipulation

## Threat Modeling Framework

KGB uses OWASP-style threat modeling:

1. **What are we building?** Component inventory, data flows, trust boundaries
2. **What can go wrong?** Abuse cases, attack surface, failure modes
3. **What are we doing about it?** Controls, mitigations, compensating factors
4. **Did we do a good job?** Reviews, tests, assurance evidence

## Historical Reference: Orange Book Lessons

The Trusted Computer System Evaluation Criteria (Orange Book) provides historical inspiration for:

- **Identification/authentication**: nodes must prove identity before trust
- **Audit/accountability**: actions must be traceable to authenticated principals
- **Access control**: least privilege enforced at system boundaries
- **Trusted path**: operator intent must reach the system without tampering
- **Assurance**: claims must be backed by evidence, not trust alone

These principles inform KGB's design without requiring formal evaluation.

## Auth/Identity Rejection

KGB explicitly rejects enterprise-first OAuth/OIDC complexity for v0 unless:

- The operator's development machine becomes shared infrastructure
- Multiple distinct human operators require fine-grained role separation
- Regulatory requirements mandate centralized identity providers

Until then, KGB uses:

- Local key material for signing
- Manual identity bootstrapping
- Simple operator authentication

## Secure Defaults Principle

Secure defaults matter more than convenience.

- Default deny on all network listeners
- No listening on 0.0.0.0 unless explicitly configured
- No plaintext secrets in config or logs
- No interactive auth that can be shoulder-surfed
- No default credentials

## Data Classification

| Category | Examples | Handling |
|---|---|---|
| Allowed | node identity, tunnel state, handshake age, probe results | record, sign |
| Forbidden | browsing history, destination IPs, user behavior | never collect |
| Deferred | config signatures, release signatures | future ACT |

## Component Security Posture

### tovarisch (Zig leaf daemon)

- Runs with minimum OS privileges
- No embedded database
- No dynamic code loading
- Bounded memory and disk usage
- Local fallback config survives bad desired-state
- Signed reports are the target state

### UVB-76 (Go control tower)

- Runs on trusted infrastructure
- Database with audit log
- Signed desired-state generation
- May use conventional auth for operators
- No user-behavior surveillance

### kgbctl (operator CLI)

- Runs on operator machine
- Holds signing key material
- Never transmits raw secrets over network
- Explicit operator confirmation for dangerous operations

## Security Non-Goals

KGB does not aim to:

- Replace enterprise-grade IAM
- Provide compliance audit trails for regulators
- Detect sophisticated nation-state adversaries
- Offer zero-trust network perimeters
- Implement military-grade cryptography

KGB aims to keep escape routes alive with reasonable security, not maximum security.
