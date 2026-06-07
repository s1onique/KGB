# Agent Failure Taxonomy

Canonical classification for Factory episode review.

## Purpose

This taxonomy helps reviewers classify where an episode broke so future harnesses, prompts, gates, or reviewer policies can improve. It is not for blame — it is for diagnosis and prevention.

## Classification Priority

Use this priority order before attributing failures to "bad reasoning":

1. **Contract-mismatch** — the agent optimized against the wrong target
2. **Action-realization** — the agent understood the goal but executed it incorrectly
3. **Trajectory-degeneration** — the episode started correctly but drifted over time
4. **Reasoning** — the agent had the right contract, executed faithfully, stayed on trajectory, but made an incorrect inference or decision

Rationale: Earlier stages in this list are more likely to be fixable by improving ACTs, gates, or reviewer prompts. Reasoning failures often require model-level intervention.

## Failure Classes

### Contract-Mismatch Failure

**Definition:** The agent optimizes against the wrong contract, missing acceptance criteria, implicit constraints, repository doctrine, or reviewer expectations.

**Software ACT examples:**
- Implementing a broad cleanup when the ACT requested one narrow bug fix
- Adding a feature but missing a required test or required documentation link
- Treating a placeholder scaffold as production data despite doctrine saying placeholders must be explicit
- Updating component CSS when theme ownership doctrine requires theme-specific overrides

**Reviewer signs:**
- The diff may be technically competent but does not satisfy the requested behavior
- Close report claims success against a different target than the ACT
- The agent asks for review while acceptance criteria remain unaddressed

### Action-Realization Failure

**Definition:** The agent appears to understand the goal but fails to correctly realize it in files, commands, patches, or verification.

**Software ACT examples:**
- Says it linked a doctrine page but edits the wrong index
- Adds tests but they do not actually cover the regression path
- Moves a React hook conceptually correctly but leaves a second hook below an early return
- Claims a command passed but uses stale or unrelated output

**Reviewer signs:**
- Plan and explanation are reasonable; patch does not match
- Evidence is stale, missing, or from the wrong command
- The requested file exists but contains incomplete or misplaced content

### Trajectory-Degeneration Failure

**Definition:** The episode starts correctly but degrades over time through drift, over-expansion, repeated local fixes, forgotten constraints, or accumulating unsound assumptions.

**Software ACT examples:**
- A small CSS fix turns into a theme refactor
- A failing test fix expands into unrelated snapshots, fixture rewrites, or broad formatting
- The agent chases secondary failures without preserving the original acceptance criteria
- The close report omits the original goal after many intermediate changes

**Reviewer signs:**
- Later changes are hard to explain from the ACT
- The diff contains unrelated cleanup
- Verification improves one area while silently breaking or ignoring another
- The episode needs a reset, smaller digest, or narrower ACT

### Reasoning Failure

**Definition:** The agent had the right contract, performed actions faithfully, stayed on trajectory, but made an incorrect inference, design choice, diagnosis, or prioritization.

**Software ACT examples:**
- Misdiagnosing a UI state bug as a backend data problem after reading the correct files
- Choosing an unsafe synchronization architecture despite explicit security goals
- Concluding a route is unused because one test does not hit it
- Designing a retry loop that violates idempotency expectations

**Reviewer signs:**
- The patch matches the stated plan, but the plan is wrong
- The agent uses available evidence incorrectly
- The failure is conceptual rather than mechanical or contractual

## Reviewer Usage

Tag the primary failure class first, then optional secondary classes.

**Example tagging:**

```
Primary: contract-mismatch
Secondary: action-realization
Reason: The agent understood the code path but ignored the acceptance criterion requiring reviewer prompt guidance to be linked.
```

## Close-Report Guidance for Failed or Corrected ACTs

Failed or corrected ACTs must include in their close report:

- **Primary failure class**
- **Secondary class** (if useful for prevention)
- **One-sentence cause** — what went wrong
- **Prevention hook** — doctrine, gate, prompt, fixture, or reviewer checklist item that could prevent recurrence

## Related Documents

- `docs/doctrine/reviewer-prompts.md` — reviewer close report checklist
- `docs/doctrine/karpathy-agent-guidelines.md` — agent contract
- `docs/doctrine/factory.md` — Factory workflow rules
