# Karpathy Agent Guidelines

Canonical discipline for coding agents working in the KGB Factory.

## Purpose

These guidelines prevent the classic LLM failure modes:

- "helpful" rewrites that change behavior beyond the request
- accidental architecture that introduces unnecessary complexity
- over-broad cleanup that removes useful context
- unverified "done" claims without evidence

## 1. Pre-flight

Before changing code, state:

- **Assumed task interpretation** — what problem are we solving?
- **Minimum intended change** — what is the smallest safe fix?
- **Verification commands** — how do we prove the change works?
- **Out of scope** — what is explicitly NOT being addressed?

For trivial one-line fixes, this may be compressed to one line each.

## 2. Simplicity

Do not introduce abstraction, configurability, or generality unless the ACT explicitly requires it.

Prefer:

- direct functions over frameworks
- local helpers over cross-cutting modules
- boring code over clever code
- explicit tests over "trust me" reasoning

## 3. Surgical Diff Discipline

Touch only files needed for the ACT.

### Allowed changes

- implementation required by the ACT
- tests proving the ACT
- docs/contracts directly affected by the ACT
- cleanup of unused code created by the ACT

### Forbidden changes

- unrelated formatting
- opportunistic refactors
- deleting pre-existing dead code
- renaming adjacent concepts
- changing behavior without an acceptance criterion

### Review invariant

> Every changed line must be explainable by: requested behavior, required test, required gate, or cleanup caused by this change.

This allows removing newly-unused imports or updating docs while forbidding drive-by gardening.

## 4. When to Stop and Ask

For human pairing: always stop when unclear.

For autonomous ACT execution:

- If unclear **and** the ambiguity blocks correctness → stop and ask.
- If unclear but a safe narrow interpretation exists → state the assumption and proceed surgically.

Agents should not over-ask; they should make progress with stated assumptions.

## 5. Error Handling

Do not add speculative error handling for paths outside the requested behavior.

Do preserve existing safety boundaries and add minimal error handling for newly introduced real failure modes.

## 6. Verification Loop

Every ACT must define close criteria before or during implementation.

A valid ACT closure includes:

- **changed files summary**
- **behavioral summary** — what changed and why
- **tests/gates run** — exact command output
- **known accepted risks** — what we consciously decided not to handle
- **follow-up work** — explicitly separated into a new ACT

## 7. Reviewer Contract

A reviewer may reject an ACT if:

1. The diff is broader than the request.
2. Tests do not reproduce or protect the behavior.
3. New abstractions are unjustified.
4. Success criteria are vague.
5. Unrelated cleanup is mixed into the patch.
6. The close report lacks verification output.

## 8. Example ACT Close Report

```markdown
**Summary**: Fixed nil pointer dereference in interface sampler when no interfaces exist.

**Files changed**:
- tovarisch/src/net/interface_sampler.zig
- tovarisch/src/net/interface_sampler_tests.zig

**Verification**:
$ make tovarisch-test
Test 1/3: interface_sampler... OK
Test 2/3: interface_sampler_with_empty... OK
Test 3/3: interface_sampler_with_real... OK

**Assumptions**:
- Empty interface list is a valid state (no tunnel configured yet).
- No logging change needed; the nil was already silently handled upstream.

**Zig 0.16 observations**: None.

**Follow-up**:
- Consider adding a health check for "no interfaces configured" state (separate ACT).
```

## Factory Integration

These guidelines are enforced by:

- `.clinerules/30-karpathy.md` — compact checklist for Cline/MiniMax agents
- Reviewer prompts (when implemented)
- Quality gate checks (when implemented)

See also: `docs/doctrine/factory.md`
