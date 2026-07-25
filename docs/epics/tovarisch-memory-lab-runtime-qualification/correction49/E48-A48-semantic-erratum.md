# E48/A48 Semantic Erratum

This immutable erratum supersedes the contradicted CORRECTION48 evidence and
attestation claims without rewriting S48, E48, or A48.

```yaml
S48: 9d431ab83c168bb6f880a32fcf45c36d12fdc0d8
E48: d79dc14162a0e106585e77db7f4c77dae62ee810
A48: 4a320a7e2759b9c350025265b4f9c417c17f67cc
A48_tree: 7672b01f3c59a2772a9115106468a3c14155ebf1
A48_parent: d79dc14162a0e106585e77db7f4c77dae62ee810

E48_patch_hygiene: FAIL
E48_artifact_role_tests: FAIL
E48_artifact_role_mutation_tests: FAIL
E48_qualification_build_tests: FAIL
E48_make_gate:
  result: NO_RULE
  UVB76_failure_observed: false

A48_contains_TBD_tree: true
tracked_generated_binaries:
  - build-qualification-artifacts
  - cmd/verify-qualification-artifacts/verify-qualification-artifacts

binary_buildinfo:
  helper_vcs_revision_present: false
  helper_vcs_modified_present: false
  production_vcs_revision_present: false
  production_vcs_modified_present: false

role_record_VCS_values:
  authority: SYNTHESIZED_BY_FALLBACK
```
