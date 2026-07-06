# ACT-HULK29R-ZIG016-WG-PEERS-TIMEOUT-DIAGNOSTIC-SEAM

**Date:** 2026-07-06  
**Author:** Claude Code (Cline/MiniMax)  
**Status:** FOUNDATION COMPLETE — INTEGRATION PENDING

## Problem Statement

The `/status` endpoint's WireGuard peer check (`wg_peers`) returned generic error messages like "wg command timeout" that didn't distinguish between:
- Timeout vs interface missing vs permission denied vs backend missing
- Which interface was selected
- Which backend was in use
- Whether the timeout was actually hit

## Solution

Created a structured diagnostic system for WireGuard peer check failures:

### New Module: `wg_peer_diagnostic.zig`

```zig
pub const WireGuardPeerDiagnostic = struct {
    backend: []const u8,           // "cli" or "netlink"
    selected_interface: []const u8, // e.g., "wg-kgb0"
    command: []const u8,           // e.g., "wg show wg-kgb0 dump"
    timeout_secs: ?u64,            // timeout value if applicable
    exit_code: ?u8,                // exit code from command
    error_kind: []const u8,        // machine-readable error kind
    stderr_len: usize,             // safe value, no slice borrow
    stdout_len: usize,             // safe value, no slice borrow
};
```

### Diagnostic Detail Format

The `formatPeerDiagnosticDetail()` function produces human-readable diagnostic strings:

```
wg timeout: interface=wg-kgb0 backend=cli timeout_secs=5
wg interface_missing: interface=wg-kgb0 backend=cli exit=1
wg permission_denied: interface=wg-kgb0 backend=cli exit=126
wg backend_missing: interface=wg-kgb0 backend=cli exit=127
wg command_failed: interface=wg-kgb0 backend=cli exit=2
wg malformed_output: interface=wg-kgb0 backend=cli exit=0
```

### Memory Safety

- All diagnostic fields use value types (no borrowed slices from command results)
- `stderr_len` and `stdout_len` are `usize` values, not slices
- `exit_code` is `?u8`, not borrowed from command result
- No `OwnedWgCommandResult` data escapes its `deinit()`

## Files Changed

### Created

- `tovarisch/src/net/wg_peer_diagnostic.zig` (100 lines)
  - `WireGuardPeerDiagnostic` struct
  - `DIAGNOSTIC_DETAIL_BUF_SIZE` constant (256)
  - `formatPeerDiagnosticDetail()` function with MemoryCopySafety annotations

### Modified

- `tovarisch/src/net/wg_status_boundary.zig`
  - Re-exports `WireGuardPeerDiagnostic`, `DIAGNOSTIC_DETAIL_BUF_SIZE`, `formatPeerDiagnosticDetail` from `wg_peer_diagnostic.zig`

## Verification

```
$ make tovarisch-test
1582 passed; 29 skipped; 0 failed.

$ python3 scripts/verify_memory_ownership_inventory.py
MEMORY OWNERSHIP INVENTORY VERIFIER: PASS

$ python3 scripts/verify_cli_composition_inventory.py
=== VERIFICATION PASSED ===

$ make gate
[gate] PASS
```

## Integration Points

The diagnostic types are re-exported from `wg_status_boundary.zig` and can be used by:

1. `status_checks.zig` - for `/status` endpoint integration
2. `wg_status_boundary_cli.zig` - for CLI backend diagnostics
3. Any module that needs structured WireGuard error information

## Actuation Status

**Foundation complete; integration pending.**

### Delivered
- `WireGuardPeerDiagnostic` struct with value-type fields
- `formatPeerDiagnosticDetail()` formatter function
- Re-exports from `wg_status_boundary.zig`
- `MemoryCopySafety` annotations on `@memcpy` usage

### Pending
- Wire diagnostic detail into actual `/status` response generation
- Add timeout detection in `runWgShowDump()` that populates `diagnostic.timeout_secs`
- Consider stderr excerpts with explicit memory ownership

## References

- `man wg(8)` - WireGuard userspace tool documentation
- `docs/doctrine/embedded-memory-frugality.md` - memory ownership contracts
- `docs/doctrine/native-owned-critical-paths.md` - anti-NIH clause
