# CORRECTION45 Attestation

## Authorities

```yaml
S45: efa752f0c9a133bac4969bb69cba6680c2b04662
ST45: 0e456806eb0a6fb2b78707f3192519b10e19ff79
E45: ac9e213feba8cff445c0ea4ec4ea63044a941127
ET45: d0dbfa4ceb9409642a7baadff0fff48e86663769
E45_parent: S45
```

## Statement

A45 attests that CORRECTION45 advanced the source-image-executable
qualification by:

* deleting the CORRECTION44 placeholder test
* adding the bounded production call-graph seam
  `RunCanonicalControlSequence` to the production CLI and the live
  smoke test
* adding a recording DockerExecAPI fixture and 21 production
  call-graph tests
* adding a static legacy-authority regression test
* updating the canary HTTP handlers to return canonical payloads
  compatible with the docker-exec control subcommand
* binding the canary image and both controller binaries to the
  final S45 subject with vcs.modified=false

A45 binds the S45 source subject, the E45 evidence commit, and the
non-circular SHA-256 digests of the E45 evidence blobs.

CORRECTION45 is a PARTIAL close: the helper smoke test PASSED
the canonical four-operation reachability flow. The production
CLI run rejected evidence because the producer-verifier contract
is unresolved under the current lifecycle ordering. CORRECTION45
therefore does not satisfy MEMLAB-08B as a full closure; the
qualified execution path is open for CORRECTION46.

## Image Identification

| Field | Value |
| --- | --- |
| canary_image_id | sha256:50d4b2d78e554bd1cb69bc1db1bdfed1657efbda10b9c61710a8262c68747531 |
| canary_binary_sha256 | fd567d1e1ae6b74dc203e0356efd9c34059942095b4e553a7515fcd335c55874 |

## Binary Identifications

| Field | Value |
| --- | --- |
| helper_binary_sha256 | 06d4b10a40078b5113fde0f22eeaba02d0665fa296fbe33c78287b73f301db36 |
| production_cli_sha256 | 11844fd0d53912dfbc84b8ad870c38110299b2707d8293a5b2ded009e70a35d8 |

E45 hashes are listed in `evidence-file-sha256s.txt`. The hashes
are taken from the E45 commit's tree, not from A45, so A45
cannot retroactively change the E45 evidence.
