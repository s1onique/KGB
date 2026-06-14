# AI-Native Code Discipline Axioms — KGB Doctrine

KGB maps the six axioms from The AI-Native Code Discipline — Manifesto to concrete KGB/tovarisch behavior.

## URI Scheme

Stable references use the `kgb://` scheme:

| Reference | Meaning |
|-----------|---------|
| `kgb://doctrine/ai-native-code-discipline-axioms` | This doc |
| `kgb://doctrine/<name>` | Other doctrine docs |
| `kgb://adr/<id>` | Architecture decision records |
| `kgb://act/<epic-id>/<slice>` | ACT slices |
| `kgb://epic/<name>` | Epic documents |

Avoid unstable references: "the doc above", "that previous ACT", "the chat knows".

---

## Axiom 1 — Repo-Local Project Memory

### Manifesto Meaning

Decisions must live in repo docs, not only chat history. Field lessons belong in tooling docs. ACT outcomes must leave enough breadcrumb trail for a future agent.

### KGB/tovarisch Interpretation

| Component | Interpretation |
|-----------|----------------|
| `tovarisch` | Zig field lessons go in `docs/tooling/zig-0.16-field-manual.md`. Zig observations go in `docs/tooling/zig-0.16-observations.md`. |
| `uvb76` | Station decisions go in `docs/doctrine/` or `docs/adr/`. |
| Zig runtime | Runtime ownership patterns documented in `docs/doctrine/runtime-harness-adaptation.md`. |
| Network/protocol | Protocol decisions documented in contracts under `docs/contracts/`. |
| Docs and gates | All required docs listed in `scripts/quality_gate.sh`. |

### Required Repo Behavior

- Every decision that affects future work must be written to a doc in the repo.
- Field lessons from Zig 0.16 work must be recorded in `docs/tooling/zig-0.16-observations.md`.
- ACT close reports must include files changed, verification output, and next exact step.
- Do not rely on "the chat knows" — a future agent must be able to resume from docs.

### Good Examples

- `docs/tooling/zig-0.16-field-manual.md` records working Zig 0.16 patterns.
- `docs/epics/act-5-sysfs-collector.md` has slice-by-slice close reports with next steps.
- `docs/doctrine/runtime-harness-adaptation.md` documents the four-layer harness.
- `docs/contracts/tovarisch-status-v0.md` documents the status contract.

### Rejection Triggers

- A decision is documented only in chat history.
- A Zig 0.16 observation is not recorded after hitting it.
- An ACT close report lacks files changed, verification output, or next step.

---

## Axiom 2 — Cold-Resume Checkpoint

### Manifesto Meaning

Meaningful ACTs need a close report or WAL update. A future agent should resume without asking the user to reconstruct state.

### KGB/tovarisch Interpretation

| Component | Interpretation |
|-----------|----------------|
| `tovarisch` | WAL docs under `docs/epics/*-wal.md` or epic slice docs. |
| `uvb76` | Station WALs follow same pattern. |
| Close report | Must include: summary, files changed, verification commands and results, production path exercised, gate blind spots, doctrine/ADR impact, next exact step. |

### Required Repo Behavior

Every meaningful ACT must produce a close report with:

1. **Summary** — one-paragraph description of what was done.
2. **Files changed** — exact list of files modified.
3. **Verification output** — exact command output from `make gate`, `make tovarisch-test`, etc.
4. **Production path exercised** — which real runtime paths were tested.
5. **Gate blind spots** — which gates were not run and why (e.g., no Linux host).
6. **Doctrine or ADR impact** — any doctrine or ADR updates.
7. **WAL / Cold Resume** — path to relevant doc, next exact step.

### Good Examples

- `docs/epics/act-5-sysfs-collector.md` has detailed slice-by-slice close reports.
- Each slice includes: board status, acceptance checklist, implementation details, files changed, next step.
- `docs/epics/tovarisch-webservice-day0.md` tracks epic-level progress.

### Rejection Triggers

- Close report lacks verification output.
- Close report lacks next exact step.
- A future agent cannot resume without asking the user to reconstruct state.

---

## Axiom 3 — Periodic Health Audit

### Manifesto Meaning

Normal gates prove current invariants, not strategic health. A lightweight periodic review is needed.

### KGB/tovarisch Interpretation

| Component | Interpretation |
|-----------|----------------|
| Normal gates | `make gate` proves: required docs exist, no forbidden naming, privacy doctrine present, Zig package valid, Zig 0.16 field manual content verified, Zig build/test/status when Zig available. |
| Periodic audit | A separate doctrine section defining lightweight periodic review items. |

### Required Repo Behavior

Periodic health audit checklist (run manually, not in CI):

- [ ] Stale docs — docs that reference outdated behavior or missing files.
- [ ] Stale TODOs — TODOs that are no longer relevant or have been done.
- [ ] Unclosed ACTs — ACTs in epics that are marked done but lack close reports.
- [ ] Gate blind spots — gates that skip platform-specific paths (e.g., Linux-only on macOS).
- [ ] Generated artifact drift — generated files that no longer match their sources.
- [ ] LLM-friendliness pressure — files growing too large or too complex.
- [ ] Runtime ownership risks — new global mutable state or hidden runtime ownership.

### Good Examples

- `docs/doctrine/llm-friendliness.md` defines file size and complexity limits.
- `docs/doctrine/runtime-harness-adaptation.md` documents runtime ownership patterns.
- `scripts/verify_split_test_inventory.sh` checks test inventory drift.

### Rejection Triggers

- Claiming that normal gates prove periodic health.
- No distinction between normal quality gates and strategic health review.

---

## Axiom 4 — Production-Path Parity

### Manifesto Meaning

Prefer tests that touch the real path where feasible. Fake-only tests are allowed for deterministic seams, but must not be the only proof for Linux/runtime-specific behavior.

### KGB/tovarisch Interpretation

| Component | Interpretation |
|-----------|----------------|
| `tovarisch` | Linux-only code paths must have Linux-only smoke tests. |
| Zig runtime | Runtime ownership should be proven through real bundle/session paths when feasible. |
| Network/protocol | BGP/BFD tests should exercise real protocol paths. |
| Status contract | Status JSON should be structurally validated through actual command output. |

### Required Repo Behavior

- Prefer tests that touch the real runtime path where feasible.
- Fake-only tests are allowed for deterministic seams (e.g., config parsing with fixtures).
- Linux-only code paths must have Linux-only smoke tests (use `error.SkipZigTest` on non-Linux).
- Platform-specific API branches must be verified on the target platform.

### Accepted Exceptions

- Unsafe paths (e.g., corrupting state).
- Flaky paths (e.g., timing-dependent behavior).
- External-network-dependent paths (e.g., real BGP peers).
- Hardware-only paths (e.g., specific NIC features).

### Good Examples

- `tovarisch/src/net/linux_interface_stats_tests.zig` has Linux smoke test alongside fixture tests.
- `tovarisch/src/bfd/runtime_tests.zig` exercises real BFD session state machine.
- `scripts/verify_tovarisch_status_contract.sh` validates actual `tovarisch status --json` output.
- `scripts/verify_split_test_inventory.sh` ensures test inventory matches build.zig.

### Rejection Triggers

- Linux/runtime-specific behavior proven only through fake-only tests.
- No Linux smoke test for Linux-only code paths.
- Platform-specific API drift not caught by cross-platform compile gate.

---

## Axiom 5 — Managed Agent Blocks

### Manifesto Meaning

Agent-generated or regenerated sections must be explicit. Any generated block should declare owner, source of truth, regeneration command, and whether manual edits are allowed.

### KGB/tovarisch Interpretation

| Component | Interpretation |
|-----------|----------------|
| Generated blocks | Any section that is regenerated by a script or agent must be clearly marked. |
| Owner declaration | Each generated block must declare: owner/generator, source of truth, regeneration command, manual edit policy. |

### Required Repo Behavior

Any generated block must include:

1. **Owner/Generator** — who or what generates this block.
2. **Source of Truth** — where the authoritative content lives.
3. **Regeneration Command** — how to regenerate the block.
4. **Manual Edit Policy** — whether manual edits are allowed (typically: no, or yes with regeneration).

### Good Examples

- `docs/coverage/tovarisch-coverage.md` has a coverage ledger with explicit entries.
- `docs/epics/act-5-sysfs-collector.md` has explicit board tables with status.
- `scripts/quality_gate.sh` checks required docs exist and are non-empty.

### Rejection Triggers

- A generated block without owner/source/regeneration semantics.
- Large generated prose blobs in hand-maintained docs.
- Generated blocks that cannot be regenerated.

---

## Axiom 6 — Stable Doctrine/ADR Links

### Manifesto Meaning

Important doctrine and ADRs need stable IDs or anchors. Avoid references like "the doc above" or "that previous ACT".

### KGB/tovarisch Interpretation

| Component | Interpretation |
|-----------|----------------|
| URI scheme | Use `kgb://doctrine/<name>`, `kgb://adr/<id>`, `kgb://act/<epic>/<slice>`. |
| Doctrine | `docs/doctrine/` docs use their filename as the anchor. |
| ADRs | Architecture decision records use sequential IDs (e.g., `adr/001-*.md`). |
| ACTs | ACTs reference their epic and slice (e.g., `act/tovarisch-webservice-day0/5h`). |

### Required Repo Behavior

- Use stable URI-style references in docs.
- Do not use unstable references: "the doc above", "that previous ACT", "the chat knows".
- When referencing doctrine, use the canonical URI.
- When referencing ADRs, use the ADR ID.
- When referencing ACTs, use the epic ID and slice ID.

### Good Examples

- This doc uses `kgb://doctrine/ai-native-code-discipline-axioms`.
- `docs/doctrine/README.md` indexes doctrine docs by filename.
- `docs/epics/act-5-sysfs-collector.md` references slices by ID (ACT 5a, 5b, etc.).
- `docs/doctrine/karpathy-agent-guidelines.md` references `.clinerules/` by path.

### Rejection Triggers

- Unstable references: "the doc above", "that previous ACT", "the chat knows".
- References that cannot be resolved without chat history.

---

## Close Report Expectations

Every ACT close report must include:

| Field | Required |
|-------|----------|
| Summary | Yes |
| Files changed | Yes |
| Verification commands and results | Yes |
| Production path exercised | Yes |
| Gate blind spots | Yes |
| Doctrine or ADR impact | Yes |
| WAL / Cold Resume — path to doctrine doc | Yes |
| WAL / Cold Resume — next exact step | Yes |
| Zig observations | Only if Zig files or tooling were touched |

---

## Reviewer Checklist

Reject the patch if:

- It only writes generic manifesto prose without KGB/tovarisch interpretation.
- It duplicates large chunks between docs.
- `.clinerules` becomes long.
- It creates process machinery instead of doctrine.
- It claims gates prove periodic health.
- It relies on chat memory rather than repo-local memory.
- It weakens production-path parity into fake-only testing.
- It adds broad generated/agent block rules without owner/source/regeneration semantics.
- It makes unstable references to "previous chat", "above", or unnamed ACTs.
- It touches unrelated runtime code.
- `make gate` is not run or failure is hand-waved.

---

## See Also

- `kgb://doctrine/factory` — Factory workflow
- `kgb://doctrine/karpathy-agent-guidelines` — Agent discipline
- `kgb://doctrine/verification-ladder` — Verification tiers
- `kgb://doctrine/llm-friendliness` — LLM-friendly files
- `kgb://doctrine/runtime-harness-adaptation` — Runtime ownership
