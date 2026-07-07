# ACT-HULK29R-ZIG016-WG-STATUS-BACKEND-COHERENCE

## Status

**COMPLETE** ✅

## Objective

Normalize WireGuard status evidence so that `/status` does not present an unexplained contradiction between `tunnel ok` and `wg_peers wireguard_interface_missing`.

## Context

Previous ACT (ACT-HULK29R-ZIG016-WG-PEERS-NAMESPACE-DIAGNOSTIC-PROOF) proved that:

```json
{
  "name": "tunnel",
  "status": "ok",
  "detail": "detected tunnel interfaces: wg-kgb0"
}
```

coexists with:

```json
{
  "name": "wg_peers",
  "status": "warn",
  "detail": "wg wrong_namespace_or_unreachable: namespace mismatch"
}
```

This ACT addresses the root cause: when `ip link show` fails due to namespace isolation, the evidence collector was setting `os_link_seen = false`, causing the classifier to return `wireguard_interface_missing` instead of `wrong_namespace_or_unreachable`.

## Root Cause

The `tunnel` and `wg_peers` checks use different observation backends:

| Check | Backend | Visibility |
|-------|---------|------------|
| `tunnel` | sysfs (`/sys/class/net`) | Kernel-level interface presence |
| `wg_peers` | CLI (`wg show`) | WireGuard control plane |

When WireGuard interface is in a different network namespace than tovarisch's running context:
- `ip link show wg-kgb0` fails (namespace isolation)
- Evidence collection sets `os_link_seen = false`
- Classifier returns `wireguard_interface_missing`

But `tunnel` check via sysfs sees the interface, creating the contradiction.

## Solution

Added **sysfs fallback** to `collectOsLinkEvidence()`. When `ip link show` fails (exit code != 0), the function now falls back to checking `/sys/class/net/<interface>` directly.

This provides an independent backend-visible link-presence signal that helps distinguish backend/namespace visibility mismatch from true interface absence.

## Changes Made

### 1. Added sysfs fallback in `wg_cli_facts.zig`

When `ip link show` fails (exit code != 0), the function now falls back to checking `/sys/class/net/<interface>` directly.

The sysfs fallback returns `os_link_kind = .unknown` because we cannot verify WireGuard link kind from sysfs alone. The classifier handles `.unknown` + `os_link_seen = true` + CLI invisible as `wrong_namespace_or_unreachable`.

### 2. Added backend coherence tests in `wg_cli_facts_tests.zig`

Two tests proving the classification rules:

- `test "classifier: sysfs-visible CLI-invisible => wrong_namespace_or_unreachable"` with `.unknown` os_link_kind
- `test "classifier: true interface missing => wireguard_interface_missing"` with `.missing` os_link_kind

### 3. Updated contract documentation in `tovarisch-status-v0.md`

Added backend split explanation and classification rules section:

```markdown
**Backend Split Explanation:**

The `tunnel` and `wg_peers` checks use different observation backends:

| Check | Backend | Visibility |
|-------|---------|------------|
| `tunnel` | sysfs (`/sys/class/net`) | Kernel-level interface presence |
| `wg_peers` | CLI (`wg show`) | WireGuard control plane |

**Classification rules (ACT-HULK29R-ZIG016-WG-STATUS-BACKEND-COHERENCE):**

When `ip link show` fails (e.g., namespace isolation), sysfs provides an independent backend-visible link-presence signal. If sysfs sees the selected interface while wg CLI cannot inspect it, this indicates a backend/namespace visibility mismatch rather than true interface absence.

1. **True interface absence** (neither sysfs nor wg CLI sees it):
   - Detail: `wg wireguard_interface_missing: interface not found`

2. **Backend/namespace visibility mismatch** (sysfs sees it, wg CLI doesn't):
   - Detail: `wg wrong_namespace_or_unreachable: namespace mismatch`
```

## Files Changed

| File | Change |
|------|--------|
| `tovarisch/src/net/wg_cli_facts.zig` | Added sysfs fallback to `collectOsLinkEvidence()` |
| `tovarisch/src/net/wg_cli_facts_tests.zig` | Added 2 backend coherence tests |
| `docs/contracts/tovarisch-status-v0.md` | Added backend split documentation |

## Verification

### All Focused Commands Pass

```bash
$ make tovarisch-build
cd tovarisch && zig build
✅ PASS

$ make tovarisch-test
1656 passed; 29 skipped; 0 failed.
✅ PASS

$ python3 scripts/verify_tovarisch_status_contract.py
[status-contract] PASS
✅ PASS

$ python3 scripts/verify_cli_composition_inventory.py
=== VERIFICATION PASSED ===
✅ PASS

$ bash scripts/verify_split_test_inventory.sh
[split-test-inventory] PASS: split test inventory is consistent
✅ PASS
```

### Gate Status

`make gate` shows pre-existing LLM-friendliness warnings in unrelated uvb76 files (state.go files exceeding 300 line soft limit). These are not caused by this ACT's changes.

### Contract Proof

After this ACT, `/status` behavior:

| State | tunnel | wg_peers detail |
|-------|--------|------------------|
| True interface missing | warn | `wg wireguard_interface_missing: interface not found` |
| Sysfs-visible but CLI-invisible | ok | `wg wrong_namespace_or_unreachable: namespace mismatch` |

The unexplained contradiction:
```
tunnel ok: detected tunnel interfaces: wg-kgb0
wg_peers warn: wg wireguard_interface_missing: interface not found
```

...is now resolved. When sysfs sees the interface, `wg_peers` will correctly report `wrong_namespace_or_unreachable`.

## Non-Goals (Preserved)

- ❌ Did NOT fix `state_dir` warning (out of scope)
- ❌ Did NOT make `wg_peers` green (suppressing warning would hide real state)
- ❌ Did NOT add namespace auto-remediation
- ❌ Did NOT change BFD/BGP behavior
- ❌ Did NOT add new CLI execution sites (sysfs uses existing `linux_read` boundary)

## Zig 0.16 Observations

No Zig 0.16-specific issues encountered in this ACT.

## Summary

This ACT normalized WireGuard status evidence by adding sysfs fallback to the OS link detection. When `ip link show` fails due to namespace isolation, sysfs provides an independent backend-visible link-presence signal. This ensures that:

1. **True interface absence** → `wireguard_interface_missing`
2. **Sysfs-visible but CLI-invisible** → `wrong_namespace_or_unreachable`

The `/status` contract is now coherent: `tunnel ok` will never coexist with `wg_peers wireguard_interface_missing`.
