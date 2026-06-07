# Runtime Harness Adaptation

Inspired by the runtime-harness idea, Factory improves agent reliability by adapting the harness around the model, not by assuming the model is intrinsically reliable.

The model may propose actions, but the harness decides whether they are valid.

## The Four Factory Harness Layers

### 1. Contract Layer

Defines what the agent is allowed to do.

**What it controls:** Intent boundaries, acceptance criteria, forbidden behavior.

**Factory artifacts:**
- ACT title and scope
- Explicit non-goals (e.g., "docs-only scope")
- Acceptance criteria checklist
- Forbidden files or behavior constraints
- "No implementation changes" constraints
- Reviewer rejection triggers (see `reviewer-prompts.md`)

**Failure prevented:** Scope creep, over-implementation, unintended behavior changes.

---

### 2. Skill Layer

Defines reusable ways of doing work.

**What it controls:** Procedural knowledge, known patterns, repair playbooks.

**Factory artifacts:**
- `AGENTS.md` — canonical agent contract
- `.clinerules/` — agent discipline rules (bootstrap, doctrine, Zig, Karpathy, verification)
- `docs/doctrine/` — doctrine docs (factory, kgb, privacy, tiny-leafs, etc.)
- Prior ACT close reports — known gate-failure patterns and repair patterns
- `docs/tooling/cline-context.md` — tooling integration patterns

**Failure prevented:** Reinventing procedures, missing context, unknown failure modes.

---

### 3. Action Layer

Defines observable execution.

**What it controls:** Evidence of what was done, not what was intended.

**Factory artifacts:**
- File edits (tracked via `git diff`)
- Shell commands and their output
- `make gate` — quality gate acceptance
- `scripts/quality_gate.sh` — doc/lint checks
- Targeted digests — handoff artifacts
- Test output (`make tovarisch-test`)
- Build output (`make tovarisch-build`)

**Failure prevented:** "Trust me" claims without evidence, unverifiable completion.

---

### 4. Trajectory Layer

Defines progress control across steps.

**What it controls:** Multi-step flow, drift detection, stopping conditions.

**Factory artifacts:**
- WAL/cold-resume — progress persistence (when used)
- Targeted digest — structured handoff between steps
- Reviewer loop — human check before close
- Close report — final evidence package
- Unresolved-risk ledger — explicit risk tracking
- "What changed since previous review" — drift detection

**Failure prevented:** Uncontrolled drift, silent scope expansion, premature closure.

---

## Mapping Table

| Harness Layer | Factory Artifact | What It Controls | Failure Prevented |
|---------------|------------------|------------------|-------------------|
| Contract | ACT prompt, acceptance criteria | Intent boundaries | Scope creep, over-implementation |
| Contract | Reviewer rejection triggers | Forbidden behavior | Unintended changes |
| Skill | AGENTS.md, .clinerules | Procedural knowledge | Missing context, reinventing |
| Skill | Doctrine docs | Reusable patterns | Unknown failure modes |
| Action | `make gate`, scripts | Observable evidence | Unverified claims |
| Action | `git diff`, test output | Concrete changes | "Trust me" reasoning |
| Trajectory | Targeted digest, close report | Progress control | Drift, premature closure |
| Trajectory | Reviewer loop | Human validation | Silent scope expansion |

---

## Example: CI/Reviewer Loop

A typical docs-only ACT demonstrating all four layers:

**Contract:**
- ACT says "docs-only scope"
- Acceptance criteria: add `docs/doctrine/runtime-harness-adaptation.md`, link from index
- Explicit non-goal: no implementation changes

**Skill:**
- `AGENTS.md` tells the agent to read doctrine index before adding docs
- `docs/doctrine/factory.md` defines doc-first workflow
- `.clinerules/30-verification.md` tells the agent to verify no implementation files changed

**Action:**
- Agent edits one markdown file
- Updates `docs/doctrine/README.md` (doctrine index)
- Runs `git diff --stat` to confirm only docs touched
- Documents verification output in close report

**Trajectory:**
- Targeted digest captures: files changed, what was added, scope verification
- Reviewer checks: contract respected (docs-only), action evidence (git diff), no drift
- Reviewer rejects if implementation files were modified, or if scope creep occurred
- Close report includes exact verification output for future reference

---

## Operating Rule

> The model may propose actions, but the harness decides whether they are valid.

- **Contract** constrains intent — what the ACT is allowed to do.
- **Skill** provides reusable procedure — how similar work was done before.
- **Action** creates evidence — what actually changed and how.
- **Trajectory** keeps the work from drifting — progress control and review.

See also:
- `factory.md` — Factory workflow rules
- `karpathy-agent-guidelines.md` — agent discipline contract
- `reviewer-prompts.md` — reviewer rejection triggers
- `.clinerules/` — agent harness rules
