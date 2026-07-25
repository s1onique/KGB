# Cline Checkpoint for CORRECTION44

## Assumptions

- Interpret CORRECTION44 as a hermetic production-path convergence only.
- Preserve exact image/network/no-pull/cleanup/provenance authority while replacing the control protocol path.
- Use the existing shared `internal/canarycontrol` implementation as the sole schema, payload, decoder, semantic-validation, and retry-policy authority.
- Do not rebuild the canary image, contact a live Docker daemon, execute MEMLAB-08C, repair UVB-76 writers, or claim MEMLAB-08B/MEMLAB-08C closure.

## Baseline

- S43: `2305b8eb34389090f761b51c2d087b46b2fe7872`
- ST43: `1c784cb3e585e18b188669f994d3db2a8847c36c`
- E43: `9681e12eddfcd78712e7766d90ef5d4665e5e122`
- ET43: `51b57188d9a9985b307e3e28e05906d67f55229f`
- A43: `ac547e24a8312fda882371c4d230f2de80c0d932`
- AT43: `2a2ee11ef3e91bd1e489892013079efca7f3694b`
- A43 parent: E43

## Minimum intended change

Migrate qualified production lifecycle control calls to one canonical `ControlRunner`, derive four-operation reachability from validated envelopes, harden independent evidence verification, delete Dockerlab's duplicate legacy protocol, remove permanent `v2` names, and add required hermetic ownership/call-graph/mutation proofs.

## Verification boundary

Run the complete CORRECTION44 command list, then `make gate`. Any known unrelated UVB-76 gate failure is recorded verbatim and is not repaired here.
