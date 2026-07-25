# CORRECTION44 Attestation

## Authorities

```yaml
S44: 439e841c3fecc8051ff76e2d44d706c43fc52823
ST44: 7a81967634dbfed762cf2ebf16adb603ca176d07
E44: 68de7775ac92bb48c2f771a2a7077db3db7d7e5c
ET44: d990af1ecb907bb65a338ec385b855d43651dd9e
E44_parent: S44
```

## Statement

A44 attests that the CORRECTION44 evidence closure is a
content-bearing delta: the production control authority was
converged onto the canonical `dockerlab.ControlRunner`, the
legacy duplicate protocol implementation and the permanent
`v2` naming are deleted from production, the qualified
lifecycle now carries an explicit
`QualifiedLifecycleDependencies` record, and the qualified
evidence verifier independently recomputes reachability from
the raw fields. A44 binds the S44 source commit, the E44
evidence commit, and the non-circular SHA-256 digests of S44
production source and configuration blobs.

## Hashes

S44 source-file SHA-256s are listed in `evidence-file-sha256s.txt`.
The hashes are taken from the S44 commit's tree, not from E44,
so E44 cannot retroactively change the S44 source.
