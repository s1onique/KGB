# CORRECTION46 — E45/A45 Semantic Erratum

## Status

CORRECTION45 remains **PARTIAL**. This erratum records defects without rewriting S45, E45, or A45.

## Reconciled identities

```yaml
S45_commit: efa752f0c9a133bac4969bb69cba6680c2b04662
S45_tree: 0e456806eb0a6fb2b78707f3192519b10e19ff79
S45_parent: eefa177dfb54068a8dc6521017376f433a9347a5

E45_commit: ac9e213feba8cff445c0ea4ec4ea63044a941127
E45_tree: d0dbfa4ceb9409642a7baadff0fff48e86663769
E45_parent: efa752f0c9a133bac4969bb69cba6680c2b04662

A45_commit: 131b2553488fb121858ecc65c4750ae7dfc041d1
A45_tree: 02a226001e30d0cf80f6dc9a4319c8619b92475a
A45_parent: ac9e213feba8cff445c0ea4ec4ea63044a941127
A45_changed_files:
  - M docs/epics/tovarisch-memory-lab-runtime-qualification/correction45/A44-evidence-erratum.md
  - A docs/epics/tovarisch-memory-lab-runtime-qualification/correction45/attestation.md
  - A docs/epics/tovarisch-memory-lab-runtime-qualification/correction45/evidence-file-sha256s.txt

E45_A45_diff_check: fail
E45_A45_whitespace_findings:
  - docs/epics/tovarisch-memory-lab-runtime-qualification/correction45/canary-binary-buildinfo.txt:3
  - docs/epics/tovarisch-memory-lab-runtime-qualification/correction45/helper-buildinfo.txt:3
  - docs/epics/tovarisch-memory-lab-runtime-qualification/correction45/production-cli-buildinfo.txt:3
```

## Semantic errata

1. CORRECTION45's PARTIAL classification remains valid.
2. The rejected production artifact lacks all three required ownership classes: workload-owned reachability, lifecycle-owned terminal truth, and CLI-owned controller provenance. The defect is not reachability alone.
3. The evidence producer consumed an incomplete or stale snapshot rather than the finalized lifecycle outcome.
4. E45/A45 patch hygiene failed because captured `go version -m` module lines contain trailing whitespace.
5. `correction45/canary-image-build.json` is stale and binds an earlier source/image rather than the final S45 live image.
6. CORRECTION45 build output, build metadata, and live image inspect record three distinct image identities without explicit identity classes. Docker Engine image IDs, BuildKit manifest digests, and BuildKit index digests must not be conflated.
7. Raw helper output and raw helper qualified-execution evidence were not committed.
8. The production call-graph test count is inconsistent: the checkpoint claims 19 tests while `A44-evidence-erratum.md` lists 21 named tests.
9. CORRECTION46 records A45's tree (`02a226001e30d0cf80f6dc9a4319c8619b92475a`) and parent (`ac9e213feba8cff445c0ea4ec4ea63044a941127`).
10. S45, E45, and A45 are historical authorities and are not rewritten.
