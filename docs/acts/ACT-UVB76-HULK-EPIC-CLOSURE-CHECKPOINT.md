# ACT-UVB76-HULK-EPIC-CLOSURE-CHECKPOINT

**Status**: CLOSED  
**Date**: 2026-07-07  
**Epic**: UVB-76 Hulk Core Hardening

## Summary

Closed the UVB-76 Hulk core hardening epic (HULK01–HULK04) by verifying all
gates, verifiers, self-tests, and Go proofs pass. Created closure document
and deferred list for next epic.

## Closed ACTs

### ACT-UVB76-HULK01: Runtime Contract Gate
- Closed runtime/concurrency contract enforcement
- Race-gated state/server/probe coverage
- Verifier/self-test in `hulk-uvb76-gate`
- 9 contract test files enforced

### ACT-UVB76-HULK02: Capture State Machine
- Closed capture status matrix and projection semantics
- Removed service skip debt
- Normalized DiagCaptureStatus → CaptureStatus layers
- Verifier/self-test in `hulk-uvb76-capture-gate`
- 12 contract test files, 8 canonical statuses, 8 TCP absence reasons

### ACT-UVB76-HULK03: Latency Query Fuzz Boundary
- Closed pure query parser and deterministic query contracts
- Bounded mutation fuzzing enforced with `-fuzztime`
- Verifier rejects fuzz soft-fail and missing fuzz targets
- Verifier self-test uses temp fixtures
- 2 fuzz targets: `FuzzLatencySeriesQueryParams`, `FuzzLatencySeriesWindowStepRange`

### ACT-UVB76-HULK04: Probe Reachability Semantics
- Closed HTTP/ICMP reachability vocabulary
- Pure classifier and matrix tests
- State transition tests
- API/event vocabulary projection contracts with honest scope
- Verifier/self-test in `hulk-uvb76-reachability-gate`
- 11 contract files, no forbidden bare terms

## Verification Commands

```bash
# Global gate
make gate

# Dedicated Hulk gates
make hulk-uvb76-gate
make hulk-uvb76-capture-gate
make hulk-uvb76-latency-gate
make hulk-uvb76-reachability-gate

# Verifiers (all pass)
python3 scripts/verify_uvb76_runtime_contracts.py
python3 scripts/verify_uvb76_runtime_contracts.py --self-test
python3 scripts/verify_uvb76_capture_state_contracts.py
python3 scripts/verify_uvb76_capture_state_contracts.py --self-test
python3 scripts/verify_uvb76_latency_series_contracts.py
python3 scripts/verify_uvb76_latency_series_contracts.py --self-test
python3 scripts/verify_uvb76_reachability_contracts.py
python3 scripts/verify_uvb76_reachability_contracts.py --self-test

# Go proofs with race detection
cd uvb76 && go test -race -v ./state/... ./server/... ./probe/... ./diag/...

# Bounded fuzz tests
cd uvb76 && go test ./server/... -run '^$' -fuzz FuzzLatencySeriesQueryParams -fuzztime=10s
cd uvb76 && go test ./server/... -run '^$' -fuzz FuzzLatencySeriesWindowStepRange -fuzztime=10s
```

## Gate Results

| Gate | Result |
|------|--------|
| `make gate` | PASS |
| `make hulk-uvb76-gate` | PASS |
| `make hulk-uvb76-capture-gate` | PASS |
| `make hulk-uvb76-latency-gate` | PASS |
| `make hulk-uvb76-reachability-gate` | PASS |
| All verifiers | PASS |
| All verifier self-tests | PASS |
| Race-gated Go tests | PASS |
| FuzzLatencySeriesQueryParams | PASS (bounded) |
| FuzzLatencySeriesWindowStepRange | PASS (bounded) |

## Known Deferrals

The following platform hardening work is deferred to the next epic:

| Deferred ACT | Rationale |
|--------------|-----------|
| ACT-UVB76-HULK05-ARTIFACT-SECRET-HYGIENE | Artifact secret hygiene outside current scope |
| ACT-UVB76-HULK06-OTEL-EXPORT-BOUNDARY | OpenTelemetry export boundary not in current goals |
| ACT-UVB76-HULK07-ROUTER-PACKAGING-CONTRACT | Router packaging contract deferred |
| ACT-UVB76-HULK08-NETNS-LAB-GO-PORT | Netns lab Go port deferred |
| ACT-UVB76-HULK09-PRODUCTION-EVENT-BUS-PROJECTION | HULK04 intentionally scoped event coverage as vocabulary projection, not production event bus integration |
| ACT-UVB76-HULK10-UI-REACHABILITY-LABEL-CONTRACT | UI label tests not wired during HULK04 |

## No-Regression Notes

**Before this epic:**
- UVB-76 had runtime crash-class risk around concurrent state reads/writes
- Diagnostic capture status semantics were implicit and drift-prone
- Latency-series query inputs were not fuzz-gated
- HTTP/ICMP reachability wording could imply false network unreachability

**After this epic:**
- Runtime/concurrency contracts are race-gated
- Capture status semantics are executable and verifier-enforced
- Latency query parsing is deterministic and bounded-fuzz-gated
- Reachability vocabulary distinguishes service/network/partial/unknown states
- Dedicated Hulk gates protect each surface

## Non-Goals (Not Started)

- HULK05 (not started)
- OpenTelemetry (not added)
- Router packaging redesign (not changed)
- Netns lab Go port (not started)
- UI graph redesign (not changed)

## Files Changed

- `docs/acts/ACT-UVB76-HULK-EPIC-CLOSURE-CHECKPOINT.md` (this file)

## Commit

Epic closure commit created with this document.
