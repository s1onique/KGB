# E43/A43 Semantic Erratum

```yaml
S43_commit: 2305b8eb34389090f761b51c2d087b46b2fe7872
S43_tree: 1c784cb3e585e18b188669f994d3db2a8847c36c
S43_parent: 545852cac5aa70fb8784cfae8af7bb5e7c8d0451

E43_commit: 9681e12eddfcd78712e7766d90ef5d4665e5e122
E43_tree: 51b57188d9a9985b307e3e28e05906d67f55229f
E43_parent: 2305b8eb34389090f761b51c2d087b46b2fe7872

A43_commit: ac547e24a8312fda882371c4d230f2de80c0d932
A43_tree: 2a2ee11ef3e91bd1e489892013079efca7f3694b
A43_parent: 9681e12eddfcd78712e7766d90ef5d4665e5e122
A43_changed_files:
  - docs/epics/tovarisch-memory-lab-runtime-qualification/correction43/attestation.md
  - docs/epics/tovarisch-memory-lab-runtime-qualification/correction43/evidence-file-sha256s.txt
  - docs/epics/tovarisch-memory-lab-runtime-qualification/correction43/final-git-status.txt

subject_identities_recorded_E43: c4b9725a1fc6c4fc025709631cb228093aa937f3
subject_identities_recorded_ET43: da47215e9625ee5423bcdce94ceac64e704dc884
subject_identities_A43_placeholder: "<to be filled at A43 commit time>"
subject_identities_AT43_placeholder: "<to be filled at A43 commit time>"
```

S43 source closure remains valid. The E43 `subject-identities.txt` blob contains a stale pre-amend E43 identity and two unresolved A43/AT43 placeholders. Therefore A43 attestation invariant 9, which claims the evidence records the reported E43/ET43 identities consistently, is false. The A43 hash manifest correctly binds that stale evidence blob; hashing a stale blob proves which blob was attested, but cannot make the blob's semantic claims current.

E43 cannot contain its own final commit identity without circularity. A43 is the authority for the final E43/ET43 tuple. CORRECTION44 is the successor authority for the A43/AT43/parent tuple. This erratum supersedes the semantic defect without rewriting S43, E43, or A43.
