# Project Context Model

> **Optional but recommended** before major reviews. Fill this in per-project or per-review-bundle to constrain findings, scoring, and recommendations. When absent, reviewers proceed with generic review but must not fabricate a context model.

---

## Trusted Inputs

Inputs that reviewers should treat as **safe by default** — no need to assume attacker control unless the finding explicitly justifies otherwise.

Examples:
- Repo-owned source files under version control
- CI/CD logs from trusted runners
- Generated artifacts with provenance chain (build logs, signed binaries)
- Human-supplied scope notes in the review bundle
- Internal API contracts documented in `docs/contracts/`

---

## Untrusted Inputs

Inputs that reviewers should **not trust by default** — require evidence of provenance or authenticity before treating as safe.

Examples:
- User-supplied content (config files, uploaded digests, pasted code)
- External logs from third-party services
- Model output in review bundles
- Third-party artifacts without verified checksums
- Web content or scraped data
- Any input marked "external" or "unverified" in the review bundle

---

## Out-of-Scope Concerns

Items reviewers should **not spend finding budget on** for this project or review.

Examples:
- Findings against internal/test infrastructure unless explicitly in scope
- Third-party library vulnerabilities with no known exploit path
- Design choices already ratified in accepted-risks.md
- Performance concerns for non-production deployments
- Concerns about tooling that operators can already disable

---

## Project-Specific Bug Bar

What severity or quality level **justifies a flagged finding** for this project.

Examples:
- `high` severity: data loss, auth bypass, credential exposure
- `medium` severity: information disclosure, denial of service under normal use
- `low` severity: cosmetic issues, missing docs, non-blocking code style
- Any finding must have a concrete reproduction path or failing test

---

## Evidence Required for Findings

What evidence **must accompany** any finding to avoid being dismissed as advisory-only.

Exact requirements:
- **File and line number** or equivalent identifier
- **Command output** or test failure demonstrating the issue
- **Reproduction path**: exact steps to trigger the finding
- **Failing test**: when fixable, a test that would fail without the fix
- **Concrete violated contract**: docstring, interface contract, or explicit assertion

Generic advisory language ("consider using X instead of Y") without the above is **not a finding**.

---

## Known False-Positive Classes

Recurring patterns reviewers should **suppress** unless stronger evidence exists.

Examples:
- Flagging TODOs as unfixed bugs without checking issue tracker
- Reporting missing docs for internal-only interfaces
- Flagging third-party dependencies without known CVE
- Reporting config drift in development environments
- Flagging deprecated APIs without known exploitation path

> Suppression means: do not raise as a finding unless the finding includes stronger-than-usual evidence that overrides the false-positive class.

---

## Review Bundle Usage

When this file is present in the review bundle:

1. Before starting the review, read and internalize this model.
2. Constrain all findings, scoring, and recommendations to this model.
3. If a finding **contradicts** the context model, explicitly justify why the model is wrong or incomplete.
4. Evidence requirements above override generic reviewer preferences when stricter.

When this file is absent:

- Proceed with existing generic review flow.
- Do not fabricate a context model.
- Apply standard reviewer judgment without project-specific constraints.
