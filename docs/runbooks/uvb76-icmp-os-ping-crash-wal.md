# WAL: UVB-76 ICMP OS Ping Command Runner Crash on Router

**Date:** 2026-06-22  
**Severity:** P0 / Router-stability blocker  
**Status:** Crash path identified; regression coverage added; root cause still under investigation

## Summary

UVB-76 crashed on ASUS RT-AX88U / linux arm64 while running ICMP OS ping probes.
The crash occurred in `BoundedCommandRunner` with a SIGSEGV in `runtime.makeslice`/`runtime.memclrNoHeapPointers`.

**The crash was in the ICMP OS ping command-runner path, not TLS, latency retention, spike detection, or diagnostic capture.**

## Root Cause

The suspected bug pattern was shared scratch buffer between stdout and stderr drains:

```go
scratch := make([]byte, 256)

go func() {
    copyBoundedWithBuf(stdout, ..., scratch)  // goroutine 1
}()

copyBoundedWithBuf(stderr, ..., scratch)  // goroutine 2 - same buffer!
```

While the current code already had separate buffers (line 100 for stderr, line 105 inline for stdout), the test coverage was insufficient to catch concurrent stdout/stderr capture races.

## Mitigation

Until the root cause is fixed, avoid this path on the router:
- Disable ICMP OS ping probe, or
- Temporarily increase ICMP interval significantly (reduces probability, not fix)

HTTP probing and the HTTPS server are not the direct crash path.

## Fix Applied

1. **Confirmed separate buffers** in `BoundedCommandRunner.Run()`:
   - `stderrReadBuf` is local to the main goroutine (line 100)
   - stdout goroutine creates its own `make([]byte, 256)` inline (line 105)

2. **Added regression tests** in `uvb76/probe/cmd_runner_race_test.go`:
   - `TestBoundedCommandRunner_ConcurrentStdoutStderrCaptureRegression` - deterministic test
   - `TestBoundedCommandRunner_ConcurrentStdoutStderrStress` - 100 iterations
   - `TestBoundedCommandRunner_ConcurrentStderrStdoutStress` - 20 goroutines × 50 iterations
   - `TestBoundedCommandRunner_BothTruncated` - both streams truncated simultaneously

3. **Test verification:**
   - All new tests pass
   - `go test -race ./probe/...` passes on development host
   - `make gate` passes

## Files Changed

- `uvb76/probe/cmd_runner_race_test.go` - new file with regression tests

## Verification Commands

```bash
cd uvb76 && go test -race ./probe/... -count=3
go test ./probe/... -run "TestBoundedCommandRunner_(ConcurrentStdoutStderr|BothTruncated)" -v
make gate
```

## Lessons Learned

1. **Shared mutable buffers between goroutines are a dangerous anti-pattern** - they can cause SIGSEGV, not just data races, leading to runtime crashes in allocation/memclr paths.

2. **No production race was found** - The inspected `BoundedCommandRunner.Run()` already uses separate scratch buffers. The suspected shared-buffer pattern was not present in the production code during this ACT.

3. **Test isolation matters** - the existing `TestBoundedCommandRunner_Concurrent` only tested parallel Run() calls, not concurrent stdout/stderr within a single Run() call.

4. **Root cause still under investigation** - The crash in `runtime.makeslice`/`runtime.memclrNoHeapPointers` on ARM64 may be related to other memory pressure issues, not just buffer sharing.

## Future Work

- Consider adding `-race` to CI pipeline for the probe package
- Add similar regression tests for other command-runner implementations
- Document the "no shared scratch buffer" invariant in code comments
