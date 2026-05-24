# KGB Security Review Ceremony

This ceremony defines when security reviews are required, how to conduct them, and what output is expected.

## When a Security Review is Required

Trigger a security review before implementing or merging changes that:

1. **New listener or route**: adding HTTP endpoints, network binds, or API routes
2. **New config input**: accepting new YAML keys, environment variables, or file inputs
3. **New file parser**: adding parsers for config, protocols, or data formats
4. **Route/tunnel/kernel interaction**: changes to how traffic flows or system calls
5. **Public bind behavior**: binding to non-localhost addresses
6. **New dependency**: adding new libraries or external services
7. **Auth/identity/signing changes**: modifying how identity or authentication works
8. **Packaging/release changes**: modifying how artifacts are built or distributed

## Review Questions

For each triggering change, answer these questions:

### 1. What is the change?

Describe the technical change in plain terms.

### 2. What assets are affected?

List:
- Data assets (config, keys, logs)
- Trust boundaries crossed
- New network exposure

### 3. What can go wrong?

Consider:
- Confidentiality: can unauthorized parties read data?
- Integrity: can data be modified without detection?
- Availability: can the system be made unavailable?
- Abuse cases: can the feature be used for unintended purposes?

### 4. What controls exist?

List:
- Existing controls that still apply
- New controls needed
- Deferred controls that should be tracked

### 5. Is this risk acceptable?

Consider:
- Who owns the risk?
- What is the mitigation path?
- When should this be revisited?

### 6. What documentation needs updating?

List:
- Threat model updates
- Accepted risks updates
- Doctrine updates
- Contract updates

## Lightweight Review Process (Solo Maintainer)

For small changes, a lightweight review is sufficient:

1. **Read the questions** (above)
2. **Write 1-3 sentences** answering each question
3. **Make a decision**: proceed, modify, or escalate
4. **Update relevant docs** or note "no change to threat model"

For significant changes (new component, new trust boundary), consider:

1. **Write a short security note** (1-2 pages)
2. **Get external review** if possible
3. **Update threat model and accepted risks**
4. **Set review reminder**

## Required Output

Every security review must produce one of:

### Option A: Threat Model Update

If the change affects components, assets, trust boundaries, or controls:
- Update `threat-model.md` with changes
- Add new abuse cases if needed
- Mark controls as deferred or implemented

### Option B: Accepted Risk Update

If the change introduces a new risk:
- Add to `accepted-risks.md` with owner, reason, expiry, mitigation
- Do not hide risks

### Option C: Explicit No-Change Note

If the change has no security implications:
```
Security review: no change required
Reason: [brief explanation]
Reviewed: YYYY-MM-DD
```

Examples of no-change:
- Typo fixes in documentation
- Test coverage additions
- CI script updates with no new assets

## Review Storage

Store security review notes in the relevant epic or as a comment in the PR.

Example:
```
Security review for ACT X.X:
- Change: added YAML config parser for probe targets
- Assets affected: config file, probe endpoints
- Risk: config injection via malicious YAML
- Controls: input validation, minimal permissions
- Decision: proceed with mitigation
- Docs updated: threat model (new abuse case AC-XX)
```

## Escalation Criteria

Escalate to more formal review if:

- New trust boundary created
- Authentication mechanism changed
- Cryptographic primitives added or modified
- Release pipeline modified
- Compliance requirements may apply

Escalation path: document concerns, add to epic scope, plan formal review.

## Review Frequency

- **Continuous**: each PR triggers review if criteria met
- **Monthly**: review accepted risks ledger for stale entries
- **Quarterly**: full threat model review
- **On significant change**: full review of affected components

## Anti-Patterns to Avoid

1. **Security theater**: reviews that don't affect decisions
2. **Hidden risks**: accepting risks without documentation
3. **Scope creep**: blocking work for minor security concerns
4. **Analysis paralysis**: over-thinking low-risk changes
5. **Deferred forever**: marking controls as deferred but never implementing

## Quick Reference

| Change Type | Review Depth | Documentation |
|---|---|---|
| New listener | Required | Threat model update |
| New config input | Required | Accepted risk or note |
| New parser | Required | Threat model update |
| Public bind | Required | Threat model + accepted risk |
| New dependency | Required | Note or threat model |
| Auth/signing changes | Required | Full review |
| Release changes | Required | Threat model + accepted risk |
| Test/doc fixes | Lightweight | No-change note |

## Ceremony Summary

1. Check if change triggers review
2. Answer six questions
3. Decide: proceed/modify/escalate
4. Update docs: threat model, accepted risks, or no-change note
5. Store review notes in epic/PR
6. Set review reminders for deferred controls
