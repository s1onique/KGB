# CORRECTION43 Close Report

## Summary

Finished the strict Docker multiplex framing boundary started by CORRECTION42.
The control frame guard now rejects every incomplete or noncanonical frame
before the control envelope can be accepted. Partial headers (1-7 bytes) and
partial payloads (declared N, delivered <N) are reported as
`ErrIncompleteControlFrameHeader` / `ErrIncompleteControlFramePayload`,
each paired with `io.ErrUnexpectedEOF` so they are discoverable with
`errors.Is`. Reserved header bytes 1, 2, 3 must be zero, and the stream
identifier is narrowed to 1/2/3 (rejecting stdin 0 with
`ErrUnexpectedControlStream` and unknown streams with
`ErrInvalidControlFrame`). Frame-size and cumulative bounds introduced
by CORRECTION42 are retained unchanged.

## Status

```yaml
CORRECTION42: SUPERSEDED_BY_CORRECTION43
CORRECTION43: CLOSED_STRICT_DOCKER_FRAMING_HERMETIC
parent_correction03: PARTIAL
MEMLAB_08B: IN_PROGRESS
MEMLAB_08C: BLOCKED
next: CORRECTION44
```

## Production path exercised

Real `ProductionControlExecRuntime` continue to use the guard in front of
`stdcopy.StdCopy`. The guard validates every frame header and produces
structured errors that propagate through `runExec` to `ControlFailureError`
and the shared `*canarycontrol.ProtocolError` chain. No live Docker
daemon was used; tests use synthetic multiplexed bytes with exact Docker
frame headers and exact reserved-byte layouts.

## Verification

- Focused tests (`go test -count=1 -v ./internal/canarycontrol/...
  ./internal/dockerlab/...`): all PASS.
- Race tests (`go test -race -count=1 ./internal/canarycontrol/...
  ./internal/dockerlab/...`): all PASS.
- Count-100 tests (`go test -count=100 ./internal/canarycontrol/...
  ./internal/dockerlab/...`): all PASS.
- Short tests (`go test -count=1 -short ./...`): all PASS.
- `go vet ./internal/canarycontrol/... ./internal/dockerlab/...`: clean.
- `gofmt -l` on changed packages: clean.

Exact outputs are committed alongside this report as
`focused-tests.txt`, `race-tests.txt`, `count100-tests.txt`,
`short-tests.txt`, and `focused-vet.txt`.

## Gate blind spots / accepted risks

`make gate` continues to fail on the pre-existing
`hulk-uvb76-artifact-producer-gate` (60 UVB-76 writer-bypass findings).
The S42 → S43 delta in every reported failure path is zero; no verifier
was weakened; the gate is not reported as passing. This is the same
external blocker documented in CORRECTION42. The Docker framing code
introduced by CORRECTION43 is fully verified hermetically.

## Doctrine / ADR impact

No doctrine or ADR changes. The four new framing errors are stable,
discoverable with `errors.Is`, never collapsed to plain `io.EOF`, and
always paired with `io.ErrUnexpectedEOF` for structured truncation.
Bounded state and native SDK reuse conform to existing memory-frugality
and native-owned-critical-path doctrine.

## Cold resume / next exact step

CORRECTION44 must migrate the production CLI and qualified lifecycle,
delete legacy Dockerlab protocol authority and permanent v2 naming, and
add operation-level canonical reachability plus strict evidence mutation
matrices. The complete frame guard is the authoritative framing layer;
production callers must move to it before legacy code can be removed.

## Zig 0.16 observations

None. No Zig source or tooling was modified.
