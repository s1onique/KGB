attestation.md (CORRECTION43)

I, the A43 author, attest that the following invariants hold for
CORRECTION43:

1. S43 source-only commit introduces the strict Docker framing
   boundary in `control_frame_guard.go` and the comprehensive
   named-test coverage in
   `control_frame_guard_correction43_test.go`. No other source
   files are modified in S43.

2. E43 evidence commit contains the close report, per-test
   artifacts, and per-file identity record. No source files are
   introduced or modified in E43; the E43 source-file SHAs are
   identical to the S43 source-file SHAs.

3. The A43 attestation commit contains exactly three files:
   - attestation.md (this file)
   - evidence-file-sha256s.txt
   - final-git-status.txt
   No other files are added or modified in A43.

4. The A43 commit's parent is E43 (no merge, no fast-forward
   through A43, no rebase of A43 onto anything other than E43).

5. The make-gate.txt records `make gate` exiting non-zero on the
   pre-existing 60 UVB-76 writer-bypass findings under
   `hulk-uvb76-artifact-producer-gate`. The S42 -> S43 delta in
   every reported failure path is zero. No verifier was weakened.
   The gate is NOT reported as passing.

6. The focused vet, race, count-100, and short test runs all pass
   with exit code 0. The focused tests pass for the entire
   `./internal/canarycontrol/...` and `./internal/dockerlab/...`
   test trees.

7. No live Docker daemon was required. All Docker framing tests
   use synthetic multiplexed bytes with exact Docker frame
   headers and exact reserved-byte layouts.

8. The frame guard retains only one 8-byte header buffer and
   never allocates a payload-sized buffer. The
   `TestFrameGuard_NoLargeAllocationOnOversizedHeader` style
   check is preserved: allocation count is bounded
   (`TestFrameGuard_PropertyRetainedStateBounded`).

9. The close report and per-test artifacts record no
   placeholder text. Every required named test is present and
   its outcome is recorded.

10. The final working tree (after A43) is clean: no dirty
    untracked files outside the A43 attestation.
