# Karpathy Discipline for Cline/MiniMax

Compact operational checklist for coding agents.

## Before Editing

- State assumptions (interpretation, minimum change, verification commands, out of scope).
- Choose the smallest safe interpretation.
- If ambiguous but safe, state assumption and proceed; if ambiguous and correctness-blocking, ask.

## While Editing

- Make surgical changes only.
- Do not refactor adjacent code.
- Do not add speculative flexibility.
- Remove only unused code introduced by your change.
- Every changed line must be explainable by: requested behavior, required test, required gate, or cleanup caused by this change.

## Before Closing

- Run the narrowest relevant tests first.
- Run `make gate` if feasible.
- Report: changed files, verification output, accepted risks.
- Split unrelated follow-ups into a new ACT.

## Review Rejection Triggers

A reviewer will reject or request patching if:

1. The implementation solves more than the ACT asked for.
2. The diff touches unrelated files or formatting.
3. New abstractions are introduced for single-use behavior.
4. The tests do not prove the requested behavior.
5. The close report lacks verification output.
6. The ACT hides assumptions or unresolved ambiguity.
7. Follow-up work is silently bundled instead of split.

## Full Doctrine

See `docs/doctrine/karpathy-agent-guidelines.md` for the complete guidelines and example ACT close reports.
