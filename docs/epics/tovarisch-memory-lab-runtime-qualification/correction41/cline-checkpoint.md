# CORRECTION41 Cline Checkpoint

## Assumptions

- **Interpretation:** implement only the hermetic Docker exec adapter and v2 streaming transport correctness requested by CORRECTION41.
- **Minimum change:** retain the v2 public controller shape where possible; replace the attachment/source model, wire the exact Docker v25 SDK seam, add bounded demultiplex writers, correct lifecycle/error causes, and move fake-only support into tests.
- **Verification:** gofmt check; focused `go vet`; focused verbose, race, count-100, and short tests; repository `make gate`; Git topology/diff/cleanliness checks.
- **Out of scope:** production caller migration, legacy protocol deletion, reachability evidence changes, canary image rebuild, live Docker, MEMLAB-08C, and parent/MEMLAB-08B closure.

## Preflight

- Branch: `main`
- Initial working tree: clean
- S40: `e0d0aecb2c65227f5c4b63bbdda5f1954a8176be`
- ST40: `d000905f4bcee19ca67207144386a2873eb71e16`
- E40/A40 identities: resolved in `E40-A40-erratum.md`
