# ACT-HULK29R-ZIG016-STATUS-HTTP-CHECK-DETAIL-OWNERSHIP

## Status
**INVESTIGATION COMPLETE - Pending Live Canary Proof**

## Summary
Investigated the reported ~136 bytes/request RSS growth under HTTP /status polling. Static code analysis confirms:

1. **Memory ownership is correctly implemented** in all status rendering paths
2. **`getWgPeersCheck()`** properly sets `owns_detail=true` and `deinitScratchChecks()` frees it
3. **`cliWireguardStatusDiagnosticAttemptWithRunner()`** uses `defer cmd_result.deinit()` on all exit paths
4. **`status_network_diag.zig`** properly handles deinit for all array lists on all paths
5. **Gate passes**, **tests pass**, **build passes**

**NOTE:** Static code review alone cannot disprove the observed live RSS growth. Live canary proof is required.

## Files Changed
- `tovarisch/src/net/wg_cli_facts.zig` - Comment-only clarification of Zig defer semantics
- `tovarisch/src/status_checks.zig` - Comment-only clarification of memory ownership
- `docs/acts/ACT-HULK29R-ZIG016-STATUS-HTTP-CHECK-DETAIL-OWNERSHIP.md` - This document

## Investigation Findings

### Corrected Zig Defer Interpretation
Per Zig language reference: `defer` "executes an expression unconditionally at scope exit." Registered defers run before returning from the scope. This means:
- When `runIpLinkShow` fails (catch block), `result` is never assigned, so no cleanup needed
- When CLI probe succeeds but exit_code triggers sysfs fallback, `defer` runs before return ✓

### Evidence Collection (wg_cli_facts.zig)
The `collectOsLinkEvidence()` and `collectWgInterfacesEvidence()` functions use correct patterns:
- `defer result.deinit(allocator)` runs on all exit paths after assignment
- Error paths in catch blocks don't need cleanup (result not assigned)
- CLI exit_code=1 fallback to sysfs correctly triggers defer before return

### WireGuard Diagnostic (wg_status_boundary_cli.zig)
The `cliWireguardStatusDiagnosticAttemptWithRunner()` function:
- Uses `defer cmd_result.deinit(allocator)` at the start after successful runner.run()
- All error return paths (exit_code 127, 126, timeout, non-zero, truncated, parse error) properly trigger the defer

### Check Detail Ownership (status.zig, status_checks.zig)
- `getWgPeersCheck()` allocates owned detail only in error cases (lines 106-122)
- `Check` struct has `owns_detail: bool` field tracking ownership
- `Check.deinit()` safely frees owned detail
- `deinitScratchChecks()` properly iterates and frees all owned checks

### Network Diag (status_network_diag.zig)
- All array lists have `defer .deinit(allocator)` statements
- Early return paths properly convert arrays to owned slices before returning
- No memory leaks in any error path

## Verification Results

```bash
$ make gate
[PASS] Memory ownership hygiene gate passed.
[gate] PASS

$ make tovarisch-test
(test output with no failures)

$ make tovarisch-build tovarisch-status
{"service":"tovarisch","version":"0.1.2+fe2dc36.dirty",...}
```

## Required Next Step: Live Canary

The original evidence was a **live daemon slope**:
```
5576 KiB → 6188 KiB
+612 KiB over ~4594 /status polls
≈ 136 bytes/request
```

To conclusively close this ACT, run the RSS canary against the same live daemon:

```bash
# Start canary against running tovarisch
make tovarisch-status-rss-canary \
  TOVARISCH_STATUS_URL=http://10.149.149.1:8317/status \
  TOVARISCH_PID=<pid>

# Or capture smaps manually
PID=$(curl -fsS "$URL" | jq -r '.runtime.pid')
snap() {
  echo "---- $(date -Is) ----"
  awk '/VmRSS|RssAnon|RssFile|RssShmem|VmData/ {print}' /proc/$PID/status
  if [ -r /proc/$PID/smaps_rollup ]; then
    awk '/^Rss:|^Pss:|^Anonymous:|^Private_Dirty:/ {print}' /proc/$PID/smaps_rollup
  fi
}
snap
for i in $(seq 1 10000); do curl -fsS "$URL" >/dev/null; done
snap
```

**If slope is flat after warmup:** Mark ACT as COMPLETE investigation-only with live evidence.
**If slope remains:** Continue investigation outside the already-reviewed ownership paths.
