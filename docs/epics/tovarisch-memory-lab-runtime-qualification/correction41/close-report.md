# CORRECTION41 Close Report

## Summary

Implemented the real Docker v25 SDK exec adapter and corrected the v2 transport around one hijacked multiplexed reader. The controller demultiplexes exactly once with Docker `stdcopy.StdCopy` into independent bounded writers, closes every acquired attachment, joins close failures, rejects blank identities before subsequent Engine operations, distinguishes parent cancellation/deadline from the attempt timeout cause, and exposes protocol plus transport causes through `ControlFailureError.Unwrap() []error`.

## Scope and status

```yaml
CORRECTION40: SUPERSEDED_BY_CORRECTION41
CORRECTION41: CLOSED_DOCKER_ADAPTER_HERMETIC
parent_correction03: PARTIAL
MEMLAB_08B: IN_PROGRESS
MEMLAB_08C: BLOCKED
next: CORRECTION42
```

No production caller migration, legacy protocol deletion, reachability evidence change, image rebuild, live Docker execution, or MEMLAB-08C matrix was performed.

## Production path exercised

The exact Docker SDK method signatures compile against `*client.Client`; adapter unit tests record exact create/attach/inspect arguments and use a real `types.HijackedResponse`. Hermetic controller tests use actual Docker multiplex frames and Docker's canonical demultiplexer. No live daemon was required.

## Verification

Focused vet, verbose tests, race tests, count-100 tests, and repository-wide short tests passed; exact outputs are committed alongside this report. `make gate` failed at the pre-existing UVB-76 artifact-writer bypass gate with 60 findings. The reported paths are outside S40→S41; `git diff` over every reported directory is empty. This is recorded as a blocker, not a pass.

## Gate blind spots and accepted risks

A live Docker daemon and production callers are intentionally outside this ACT. Docker v25's `types.HijackedResponse.Close()` discards `net.Conn.Close()` errors, so the adapter closes `response.Conn` directly to satisfy close-error propagation. The SDK seam necessarily cannot pass container ID to attach/inspect because Docker v25 APIs identify those operations by exec ID; container authority is retained by the runtime method and inspect response identity is checked when Docker supplies it.

## Doctrine / ADR impact

No doctrine or ADR changes. The implementation reuses the Docker SDK and canonical demultiplexer in accordance with native-owned critical-path anti-NIH doctrine and keeps diagnostics bounded per embedded-memory doctrine.

## Cold resume / next exact step

CORRECTION42 must migrate production callers, remove the legacy dockerlab protocol authority, implement canonical operation-level reachability, and harden evidence mutation checks. Do not rebuild the image or claim MEMLAB-08B closure from this report.

## Zig 0.16 observations

None. No Zig source or tooling was modified.
