# ACT-HULK29R-ZIG016-STATUS-CONTRACT-FIXTURE-WG-TOOL-MISSING

## Status
**COMPLETE** — Status contract gate fixed.

## Root Cause

The WireGuard status classifier (ACT-HULK29R-ZIG016-WG-STATUS-CLASSIFICATION-FIX) introduced canonical `WgDiagnosticClass.wg_tool_missing` with user-facing detail `"wg wg_tool_missing: wg command not installed"`.

However, two artifacts retained the legacy format:
1. `docs/contracts/examples/tovarisch-status-v0.json` fixture expected `"wg backend_missing: interface=wg-kgb0 backend=cli"`
2. Test in `tovarisch/src/net/wg_peer_diagnostic.zig` expected the legacy format from `formatPeerDiagnosticDetail()` with raw `error_kind="backend_missing"`

The production path uses `classifierErrorKindToDetail()` (status_checks.zig) which outputs the canonical format. The `formatPeerDiagnosticDetail()` function is tested in isolation with hardcoded error_kind values and is NOT used by the production status path.

## Changes

### 1. Updated status contract fixture
**File:** `docs/contracts/examples/tovarisch-status-v0.json`

```diff
-      "detail": "wg backend_missing: interface=wg-kgb0 backend=cli"
+      "detail": "wg wg_tool_missing: wg command not installed"
```

### 2. Added clarifying comment to diagnostic formatter test
**File:** `tovarisch/src/net/wg_peer_diagnostic.zig`

Added documentation noting that the raw `formatPeerDiagnosticDetail()` function is tested with arbitrary error_kind values, while production uses the canonical `classifierErrorKindToDetail()`.

## Verification

```bash
python3 scripts/verify_tovarisch_status_contract.py
make tovarisch-status
make tovarisch-test
make gate
```

## Acceptance Criteria Met

- [x] Status contract fixture expects `wg wg_tool_missing: wg command not installed`
- [x] No legacy `wg backend_missing: interface=wg-kgb0 backend=cli` in active status contract expectations
- [x] `make gate` passes or reaches new unrelated blocker
- [x] CLI composition inventory remains passing
- [x] No production classifier downgrade
- [x] No broad refactor
