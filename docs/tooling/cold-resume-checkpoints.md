# Cold-Resume Checkpoint Contract

Canonical reference for ACT close reports and cold-resume checkpoints in KGB.

## Purpose

A future engineer or LLM agent should be able to resume an ACT from repo-local state without relying on chat history. Closed or in-progress work must leave enough structured information to answer:

- What changed?
- What was verified?
- What remains risky or incomplete?
- What exact next step should happen?
- What is the honest close state?

## Required Structural Markers

Every closed or completed ACT **must** include markers for:

| Marker | Required | Flexible Variants |
|--------|----------|-------------------|
| Status | Yes | `[Closed]`, `[Closed pending commit hygiene]`, `[Open]`, `ACT status`, `**Status:**`, `**Status**: Complete` |
| Summary | Yes | `Summary`, `### Summary`, `## Closure Summary`, `Completion summary` |
| Files changed | Yes | `Files Changed`, `Files changed`, `### Files Changed`, `Changed files` |
| Verification | Yes | `Verification`, `Verification Commands`, `make gate`, `### Verification`, `Verification commands and results` |
| Caveats | Yes | `Caveats`, `Known caveats`, `Blockers`, `Remaining`, `### Caveats` |
| Next step | Yes | `Next`, `Next exact step`, `### Next`, `Recommended next ACT`, `## Next Step` |

## Recommended (Not Initially Mandatory)

| Marker | Purpose |
|--------|---------|
| Behavior changed / preserved | Documents side effects |
| Gate blind spots | Which gates were not run and why |
| Commit hygiene status | Unstaged files or pending work |
| Production/manual verification status | Manual steps that could not be automated |

## Checkpoint Block Patterns

The verifier accepts checkpoint blocks that start with these markers:

- `# ACT`
- `## ACT`
- `### ACT`
- `ACT Closed`
- `[Closed] ACT:`
- `[Open] ACT:`
- `[Closed pending commit hygiene] ACT:`
- `Completion summary`
- `Close report`

## Closed vs Open Checkpoints

**Closed/Completed checkpoints** require all 6 required markers.

**Open/In-progress checkpoints** require at minimum:
- Status marker
- Next step marker (or current state description)
- Caveats/blockers if any exist

## Historical Debt

Some epics were created before this verifier existed. These are tracked in:

```
docs/reference_allowlists/cold_resume_checkpoint_legacy_allowlist.csv
```

This allowlist is explicit and small. It documents:
- Which historical files have incomplete checkpoints
- Why they are allowlisted
- Planned resolution or rationale for gaps

New ACTs should not add to this debt.

## See Also

- `kgb://doctrine/ai-native-code-discipline-axioms` — AXIOM-2 definition
- `kgb://doctrine/karpathy-agent-guidelines` — Agent discipline
- `scripts/verify_cold_resume_checkpoints.py` — Mechanical verifier
