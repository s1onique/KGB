# CORRECTION42 Cline Checkpoint

## Assumptions

- **Interpretation:** finish only the post-attach transport safety gaps listed by CORRECTION42.
- **Minimum change:** add nil/shape validation, one-close authority, context-driven read interruption, a framing-header guard, terminal inspect enforcement, and direct hermetic tests.
- **Verification:** gofmt, focused vet/tests/race/count-100, repository short tests, canonical `make gate`, and Git/evidence topology checks.
- **Out of scope:** caller or qualified-runtime migration, legacy protocol deletion, reachability changes, image rebuild, live Docker, MEMLAB-08C, and MEMLAB-08B/parent closure.

## Preflight

- Branch: `main`
- Initial working tree: clean
- S41: `d467f09298ab0f31cbd99353b29962531753deaf`
- E41: `40ec4d048cae63cc5704a869231289a33f8901ea`
- A41 identity: resolved in `A41-identity-erratum.md`
