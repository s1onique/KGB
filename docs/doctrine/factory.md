# Factory Doctrine

KGB follows the Factory style of development.

## Rules

1. Work is organized as explicit epics.
2. Epics have boards with small, reviewable work items.
3. Every meaningful change must be verifiable.
4. Quality gates are part of the product, not ceremony.
5. Documentation is a design surface.
6. The repo should be friendly to humans and LLMs.
7. Files should stay small, readable, and purposeful.
8. Prefer boring correctness over clever abstraction.
9. Do not add enterprise ceremony unless the project actually needs it.
10. If a system property matters, encode it in docs and gates.

## Default workflow

1. Open an epic.
2. Define doctrine/contract before implementation.
3. Make the smallest useful change.
4. Add verification.
5. Update docs.
6. Close only when the repo proves the claim.

## Agent Discipline

See `karpathy-agent-guidelines.md` for the canonical agent contract:

- Pre-flight: state assumptions, minimum change, verification commands, out of scope.
- Simplicity: no abstraction unless required.
- Surgical diff: only touch files needed for the ACT.
- Verification loop: define close criteria before claiming done.
- Review invariant: every changed line must be explainable.
