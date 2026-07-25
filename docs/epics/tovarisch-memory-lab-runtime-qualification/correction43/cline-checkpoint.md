# Cline Checkpoint for CORRECTION43

## Baseline (S42/E42/A42)

- S42: a4c131da8cfe449f4a34a49c4c77dba9a260862b
- ST42: 858c4ee58d81a5eaee767278a136386f728da545
- E42: 762a20495a15aba56c67834db455cc1f38c226b0
- ET42: 3e016df4d423cabc746040accc0c40e7403b047e
- A42: 545852cac5aa70fb8784cfae8af7bb5e7c8d0451
- AT42: 8f9d5c6b259fd74aea349cda6ec392c8875fa221

## Goal

Reject every incomplete or noncanonical Docker multiplex frame before the
control envelope can be accepted. Introduce stable framing errors, reject
partial headers/payloads, validate reserved bytes, narrow stream identifiers,
preserve frame-size and cumulative bounds, and add reader-contract/property
coverage.

## Files changed in S43

- `tovarisch/labs/memory/internal/dockerlab/control_frame_guard.go`
- `tovarisch/labs/memory/internal/dockerlab/control_frame_guard_correction43_test.go` (new)

## Out of scope

- Migrate production CLI callers
- Migrate qualified lifecycle
- Delete legacy Dockerlab protocol
- Modify reachability evidence
- Rebuild canary image
- Require live Docker daemon
- Execute MEMLAB-08C
