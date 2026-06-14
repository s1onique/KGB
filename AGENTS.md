# AGENTS.md

Canonical contract for coding agents working in the KGB repository.

## Purpose

This file is the authoritative reference for any agent performing work in this repo.
All agent-facing docs point here; do not duplicate doctrine elsewhere.

## Project Shape

- **KGB** is the whole system — lightweight anti-censorship control plane.
- **station** is the Go control tower, running on trusted/home infrastructure.
- **tovarisch** is the Zig constrained leaf daemon, running on tiny remote machines.
- **kgbctl** is the future operator CLI.
- Do not call `tovarisch` an "agent" except when explaining the role generically.

## Mandatory First Reads

Before any implementation work, read:

- `README.md`
- `docs/doctrine/factory.md`
- `docs/doctrine/ai-native-code-discipline-axioms.md`
- `docs/doctrine/karpathy-agent-guidelines.md`
- `docs/doctrine/kgb.md`
- `docs/doctrine/privacy.md`
- `docs/doctrine/tiny-leafs.md`
- `docs/architecture/overview.md`
- `docs/architecture/components.md`
- `docs/architecture/naming.md`
- `docs/tooling/zig-0.16-field-manual.md`
- `docs/tooling/cline-context.md`
- Current epic doc under `docs/epics/`

## Factory Workflow

Work happens in explicit epics and ACTs.

1. Open an epic or locate the current one.
2. Read the epic before implementation.
3. Keep changes small and reviewable.
4. Add verification before claiming done.

Every ACT must end with:

- **Summary** of what was done
- **Files changed** list
- **Verification output** (exact command output)
- **Assumptions / blockers** (if any)
- **Zig 0.16 observations** (if any)

## Quality Gate

- `make gate` is the acceptance boundary.
- For Zig changes, also run targeted commands:
  - `make tovarisch-build`
  - `make tovarisch-test`
  - `make tovarisch-status`
- Do not claim completion without verification output.

## LLM-Friendliness

- Keep files small, explicit, boring, and testable.
- Prefer focused modules over large files.
- Do not perform opportunistic rewrites.
- Do not do broad renames without explicit instruction.
- Add docs/contracts before expanding machine-readable interfaces.
- Large files are agent-hostile; split by responsibility.

## Privacy and Safety Doctrine

KGB observes infrastructure health, not people.

**Allowed:** node identity, transport state, handshake age, reachability, probe results, config version, clock skew.

**Forbidden:** browsing history, visited domains, destination IP flow logs, message contents, per-user behavioral timelines.

Connectivity facts are allowed. Human behavior surveillance is forbidden.

## Leaf Constraints

Leaf nodes must NOT include:

- Kubernetes
- Containers by default
- Embedded TSDB
- Full observability stack
- Modern web UI requirement
- Unbounded memory/disk growth
- Framework gravity

## Zig Doctrine

KGB targets Zig 0.16.x-style APIs for `tovarisch`.

- Read `docs/tooling/zig-0.16-field-manual.md` before editing Zig.
- Inspect existing working Zig files before inventing APIs.
- Do not downgrade Zig.
- Keep `main.zig` as process boundary only.
- Keep CLI/status logic testable.

## Zig Learning Protocol

If you encounter any Zig 0.16-specific difficulty, stale API assumption, compiler error caused by old examples, build.zig mismatch, stdlib rename, test harness problem, or IO/process API surprise:

1. Continue solving the task.
2. Do not downgrade Zig.
3. Record the observation in your final response under `Zig 0.16 observations`.
4. Include:
   - **symptom**: what went wrong
   - **wrong assumption**: what you tried that failed
   - **working fix**: how you solved it
   - **files affected**: which files you changed
   - **promote to field manual?**: yes/no
5. If the observation is clearly reusable, also patch `docs/tooling/zig-0.16-observations.md`.
6. Do not change the curated field manual unless explicitly asked.

## Forbidden Moves

- Do not add enterprise/OAuth/governance-first complexity.
- Do not add Kubernetes assumptions for leaf nodes.
- Do not turn `tovarisch` into a generic observability agent.
- Do not introduce user-behavior monitoring.
- Do not downgrade Zig.
- Do not bypass the quality gate.
- Do not make large speculative architecture rewrites.
