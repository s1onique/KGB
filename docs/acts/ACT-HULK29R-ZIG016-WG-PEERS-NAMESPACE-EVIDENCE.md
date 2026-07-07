# ACT-HULK29R-ZIG016-WG-PEERS-NAMESPACE-EVIDENCE

## Status

**COMPLETE**

## Summary

Enhanced the `wrong_namespace_or_unreachable` diagnostic detail to include evidence sources (`os_link_seen`, `wg_cli_seen`) so future failures explain **which evidence source saw what**.

## Problem

The live status readout showed:

```
tunnel   ok   detected tunnel interfaces: wg-kgb0
wg_peers warn  wg wrong_namespace_or_unreachable: namespace mismatch
```

The detail string was accurate but did not include evidence sources. Operators could not see:
- Whether the OS-level link (via sysfs) was visible
- Whether the WireGuard CLI (`wg show`) could see the interface

## Solution

Added `classifierErrorKindToDetailWithEvidence()` that formats namespace mismatch details with evidence flags:

```
wg wrong_namespace_or_unreachable: namespace mismatch os_link_seen=true wg_cli_seen=false
```

This matches the expert's recommended format:

> ```text
> wg wrong_namespace_or_unreachable: namespace mismatch; os_link_seen=true cli_interfaces_seen=false
> ```

## Changes

### `tovarisch/src/status_checks.zig`

- Added `classifierErrorKindToDetailWithEvidence()` function that:
  - For `wrong_namespace_or_unreachable`: outputs `os_link_seen` and `wg_cli_seen` evidence flags
  - For other classes: falls back to standard formatter
- Added `DIAGNOSTIC_DETAIL_BUF_SIZE = 256` constant
- Added `truncatedDiagnosticEvidence()` fallback for buffer overflow
- Updated `getWgPeersCheck()` to use the new evidence-enhanced formatter

### `tovarisch/src/status_wg_evidence_tests.zig` (new)

Split evidence-enhanced tests from `status_checks.zig` to satisfy LLM-friendliness limits:
- `test "classifierErrorKindToDetailWithEvidence: namespace mismatch includes evidence"`
- `test "classifierErrorKindToDetailWithEvidence: namespace mismatch with both visible"`
- `test "classifierErrorKindToDetailWithEvidence: non-namespace classes fall back"`
- `test "classifierErrorKindToDetailWithEvidence: buffer size constant is correct"`

## Files Changed

- `tovarisch/src/status_checks.zig` (modified: 412 lines)
- `tovarisch/src/status_wg_evidence_tests.zig` (new: 98 lines)

## Verification

```bash
$ make tovarisch-build
BUILD SUCCESS

$ make tovarisch-test
TEST SUCCESS

$ make gate
[gate] LLM-friendliness: PASS
[PASS] Memory ownership hygiene gate passed.
```

## Design Notes

### Evidence Sources

The diagnostic now shows:

| Field | Source | Meaning |
|-------|--------|---------|
| `os_link_seen` | sysfs probe | Kernel-level interface presence |
| `wg_cli_seen` | `wg show interfaces` | WireGuard control plane visibility |

### Interpretation

| `os_link_seen` | `wg_cli_seen` | Likely Cause |
|----------------|---------------|--------------|
| `true` | `false` | Namespace mismatch (interface in different netns) |
| `false` | `false` | Interface missing or probe failure |
| `true` | `true` | Other issue (unexpected but still "wrong_namespace" classification) |

### Status Contract

Classification remains `warn` for `wrong_namespace_or_unreachable`. This preserves the deployment invariant: BFD/BGP health indicates the tunnel is functional despite peer diagnostic limitation.

## Follow-up

### Optional Enhancement (Out of Scope)

For future ACT, consider adding network namespace inode evidence:

```text
wg wrong_namespace_or_unreachable: namespace mismatch os_link_seen=true wg_cli_seen=false service_netns=4026532341 probe_netns=4026531840
```

Requires:
- Reading `/proc/self/ns/net` for probe namespace
- Reading `/proc/<pid>/ns/net` for service namespace (requires PID access)
- Careful handling of null PID cases (stateless deployments)

**Not implemented** in this ACT to keep scope narrow.

## Related ACTs

- ACT-HULK29R-ZIG016-WG-STATUS-BACKEND-COHERENCE — Fixed sysfs fallback for OS link detection
- ACT-HULK29R-ZIG016-WG-PEERS-NAMESPACE-DIAGNOSTIC-PROOF — Proved namespace mismatch classification
