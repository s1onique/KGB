# ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION03-P0-CONVERGENCE01

## Summary

Convergence successor for ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION03. Handles the ten open P0 findings as one unified implementation rather than fragmented patches.

## Status

**IN_PROGRESS**

## Predecessor: ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION03

### From CORRECTION03 - Open P0 Issues Being Addressed

| P0 | Issue | Implementation Step |
|----|-------|---------------------|
| P0-1 | matrix-cleanup.json not in checksum contract | 1. Integrity authority |
| P0-2 | Production observer bypasses RunBoundedCommand | 2. Command lifecycle authority |
| P0-3 | WaitDelayExpired discarded at Docker boundary | 2. Command lifecycle authority |
| P0-4 | No real WaitDelay test | 3. Real lifecycle proof |
| P0-5 | Command tests don't invoke actual CLI | 6. Terminal command proof |
| P0-6 | Equal-invalid test is not equal-invalid | 4. Artifact fixture correctness |
| P0-7 | Child checksums not regenerated after mutation | 4. Artifact fixture correctness |
| P0-8 | Fixture checksum silently skips missing files | 4. Artifact fixture correctness |
| P0-9 | Network schema not verified | 5. Network reader convergence |
| P0-10 | Docker smoke violates no-pull contract | 7. Docker smoke |

## Implementation Order

### Step 1: Integrity Authority (P0-1)

Create a single canonical matrix artifact inventory containing:
- matrix-manifest.json
- matrix-cleanup.json
- matrix-verdict.json

Require exactly those three entries in matrix-checksums.txt, rejecting missing, extra and duplicate entries.

Until this is done, cleanup semantic mutations are not integrity-bound and the terminal fixture cannot be authoritative.

### Step 2: Command Lifecycle Authority (P0-2, P0-3)

Make:
- DefaultDockerRunner → RunDockerCommand → RunBoundedCommand

the only production execution path.

Project WaitDelayExpired into DockerCommandResult and classify timeout, overflow or WaitDelay expiration as unavailable. Go documents ErrWaitDelay as the successful-process case where output pipes remain open past Cmd.WaitDelay; dropping it at the Docker projection boundary loses material execution truth.

### Step 3: Real Lifecycle Proof (P0-4)

Replace the unused pseudo-helper with an actual helper-process test that:
1. Starts a descendant inheriting stdout
2. Parent exits zero
3. Descendant retains the descriptor
4. RunBoundedCommand returns after WaitDelay
5. WaitDelayExpired == true
6. Exact descendant is cleaned up

Also migrate stdout, stderr, overflow and timeout tests away from sh, dd, sleep, true and false to the same Go helper process.

### Step 4: Artifact Fixture Correctness (P0-6, P0-7, P0-8)

Move matrix_fixture.go to matrix_fixture_test.go unless synthetic fixture generation is a real product feature.

The fixture must:
- Derive inventories from production authorities
- Fail immediately when any required artifact cannot be read
- Regenerate child checksums after child mutations
- Update the manifest's declared child checksum digest when required
- Regenerate matrix checksums only after child evidence is coherent

### Step 5: Network Reader Convergence (P0-9)

Replace the anonymous decoder in verifyChildRunBundle with:
```go
networkID, networkName, err := ReadNetworkIdentity(runDir)
if err != nil {
    return nil, fmt.Errorf("verify network identity: %w", err)
}
```

This eliminates the current duplicate decoding authority and prevents strict decoding failures from being silently converted into an empty identity. ReadNetworkIdentity already validates schema version, required ID, unknown fields and trailing JSON.

### Step 6: Terminal Command Proof (P0-5)

Invoke the real verify-matrix command path with captured stdout, stderr and exit status.

Required cases:
- valid fixture → zero, exactly one PASS
- equal-invalid fixture → nonzero, no PASS
- container exists → nonzero, no PASS
- container unavailable → nonzero, no PASS
- network exists → nonzero, no PASS
- network unavailable → nonzero, no PASS
- process still_alive → nonzero, no PASS
- process unavailable → nonzero, no PASS
- child checksum mismatch → nonzero, no PASS
- matrix checksum mismatch → nonzero, no PASS
- unknown JSON field → nonzero, no PASS
- trailing JSON document → nonzero, no PASS

A genuine equal-invalid fixture must mutate authoritative evidence, reconstruct an invalid verdict, store that exact invalid verdict, refresh valid checksums and still be rejected.

### Step 7: Docker Smoke (P0-10)

The smoke must use an already-present local image verified with docker image inspect; it must never run docker pull.

Create and capture exact object IDs, then remove by those IDs:
```bash
docker container rm -f <captured-container-id>
docker network rm <captured-network-id>
```

When the qualification environment variable is explicitly enabled, unavailable Docker or a missing required local image must fail with an actionable diagnostic rather than skip.

## Documentation Correction

The ACT record currently says "22 semantic mutation tests," while the reviewed table accounted for 20 distinct mutations. Keep the count evidence-derived: either add and name the missing two cases or correct the document to 20.

## Transition Rule

```yaml
P0_CONVERGENCE01_complete:
  CORRECTION03: CLOSED
  MEMLAB-08A: DONE
  MEMLAB-08B: READY

any_acceptance_failure:
  CORRECTION03: IN_PROGRESS
  MEMLAB-08A: OPEN
  MEMLAB-08B: BLOCKED
```

## Files Changed

```
tovarisch/labs/memory/cmd/tovarisch-memory-lab/
├── bounded_command.go           # WaitDelayExpired projection
├── bounded_command_test.go      # Real helper-process WaitDelay test
├── cleanup_observation.go      # Use RunDockerCommand
├── matrix.go                    # P0-1: Add cleanup to checksum contract
├── matrix_fixture.go            # P0-4: Move to _test.go
├── matrix_verify.go             # P0-5: Use ReadNetworkIdentity
├── matrix_verify_terminal_test.go # P0-3: Terminal command tests
└── [new] docker_smoke_test.go   # P0-7: Docker smoke with ID removal
```

## Verification

```bash
cd tovarisch/labs/memory/cmd/tovarisch-memory-lab
go test ./...          # Must pass
go test -race ./...    # Must pass
make gate             # Must pass
```

## Zig 0.16 Observations

Not applicable - this is a Go implementation.
