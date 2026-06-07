# Reviewer Prompts for KGB Factory ACTs

Canonical reviewer checklist for validating KGB Factory work.

## Reviewer Close Report Checklist

When reviewing an ACT close report, verify:

### 1. Surgical Diff Discipline

Every changed line must be explainable by one of:
- **Requested behavior** — the ACT explicitly asked for this change
- **Required test** — a test proving the ACT works
- **Required gate** — changes needed for `make gate` to pass
- **Cleanup caused by this change** — removal of code made unused by this ACT

### 2. Seven Rejection Triggers

Reject or request patching if the ACT has:

1. **Over-implementation** — the diff solves more than the ACT asked for
2. **Scope creep** — touches unrelated files or formatting
3. **Speculative abstraction** — new abstractions introduced for single-use behavior
4. **Unproven behavior** — tests do not reproduce or protect the requested behavior
5. **Missing verification** — close report lacks verification output (exact command output)
6. **Hidden assumptions** — ACT hides assumptions or unresolved ambiguity
7. **Bundled follow-ups** — unrelated follow-up work silently mixed into the patch instead of split into a new ACT

### 3. Explicit Rejection Cases

Reject if the ACT contains:

- **Unrequested abstraction** — introducing generalization, configurability, or new modules without explicit ACT requirement
- **Unrelated cleanup** — formatting changes, opportunistic refactors, or deletion of pre-existing dead code that was not created by this ACT
- **Vague verification** — claims of test/gate passing without exact command output or with hand-wavy "tests passed" language

### 4. Completeness Check

A valid close report must include:

- [ ] **Summary** — one-paragraph behavioral summary of what was done
- [ ] **Files changed** — exact list of modified files
- [ ] **Verification output** — exact command output (not paraphrased)
- [ ] **Assumptions/blockers** — if any, stated explicitly
- [ ] **Zig 0.16 observations** — if any Zig 0.16-specific issues were encountered
- [ ] **Follow-up** — explicitly separated into new ACTs if any

## Reviewer Prompt Template

When reviewing an ACT, use this prompt structure:

```
Review the following ACT close report for KGB Factory compliance.

CHECK 1: Surgical diff discipline
- Does every changed line map to: requested behavior, required test, required gate, or cleanup caused by this ACT?
- Are there unrelated formatting, opportunistic refactors, or pre-existing dead code deletions?

CHECK 2: Seven rejection triggers
1. Over-implementation? [yes/no]
2. Unrelated file/formatting touches? [yes/no]
3. Single-use abstractions? [yes/no]
4. Tests proving behavior? [yes/no]
5. Verification output present? [yes/no]
6. Hidden assumptions? [yes/no]
7. Bundled follow-ups? [yes/no]

CHECK 3: Explicit rejection cases
- Unrequested abstraction present? [yes/no]
- Unrelated cleanup present? [yes/no]
- Vague/missing verification? [yes/no]

CHECK 4: Completeness
- Summary present? [yes/no]
- Files changed listed? [yes/no]
- Exact command output provided? [yes/no]

VERDICT: [approve/request-changes]
```

## Integration

- This doc complements `karpathy-agent-guidelines.md` — which defines the agent contract
- Agent close reports should self-check against this list before submission
- Reviewers should use the template above or equivalent checks

## Factory Integration

See also:
- `karpathy-agent-guidelines.md` — canonical agent contract
- `.clinerules/30-karpathy.md` — compact agent checklist
- `docs/doctrine/factory.md` — Factory workflow rules
- `docs/doctrine/agent-failure-taxonomy.md` — failure classification for episode review

