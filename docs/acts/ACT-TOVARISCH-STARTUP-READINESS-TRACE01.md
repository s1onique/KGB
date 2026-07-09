# ACT-TOVARISCH-STARTUP-READINESS-TRACE01

## Status: COMPLETE (R1/R2/R3 passed)

## Objective

Make the `tovarisch` startup path explainable and testable by instrumenting startup phases with timing and emitting structured events for diagnosing gaps between process start and HTTP readiness.

**Production Symptom:** A 68-second gap between BFD start and HTTP readiness that was not diagnosable from existing logs.

## Deliverables

### Core Implementation

- [x] **startup_trace.zig** (172 lines) - Core tracing module with StartupTracer, PhaseGuard, and StartupPhase enum
- [x] **startup_trace_time.zig** (48 lines) - Cross-platform monotonic time helper (Linux: clock_gettime, macOS: mach_absolute_time)
- [x] **startup_trace_unit_tests.zig** (120 lines) - Unit tests for core types
- [x] **startup_trace_integration_tests.zig** (202 lines) - Integration tests for startup tracing
- [x] **http/serve_startup.zig** - Startup-aware HTTP serve loop with proper handleConnection call

### Instrumentation

- [x] **daemon_command.zig** - Instrumented startup phases:
  - `config_load` - Config file loading
  - `bfd_start` - BFD configuration loading
  - `bgp_load` - BGP configuration loading
  - `lab_and_net_diag_config_parse` - Lab and network diag config parsing
- [x] **logging.zig** - Added startup events: startup_phase_started, startup_phase_finished, startup_phase_slow, startup_ready
- [x] **http/serve_startup.zig** - Phase guards that finish BEFORE infinite loop (correct semantics)

### Test Coverage

- [x] Unit tests for PhaseGuard and slow phase detection
- [x] Integration tests for startup sequence and event ordering
- [x] Test module references in test_all.zig and test_suite_http.zig

### Documentation

- [x] **docs/operations/startup-readiness.md** - Operational guidance for interpreting traces

## R1 Fixes Applied

1. **HTTP request handling restored**: serve_startup.zig now calls `server.acceptOneNormal` which calls `handleConnection`
2. **Phase guards finish before infinite loop**: http_accept_loop guard ends before entering while(true) loop
3. **Real bgp_result preserved**: bgp_result is now properly passed to HTTP serve
4. **Additional phases added**: bgp_load, lab_and_net_diag_config_parse for better gap diagnosis
5. **Config read once**: Combined lab and net_diag parsing to avoid reading config file twice

## Verification

```
make gate         - PASS
make tovarisch-build - PASS
make tovarisch-test  - 1688 passed, 31 skipped, 0 failed
make tovarisch-status - Valid JSON output
```

## Startup Phases

| Phase | Description |
|-------|-------------|
| `config_load` | Config file loading and parsing |
| `bfd_start` | BFD configuration loading and loop startup |
| `bgp_load` | BGP configuration loading (reading file, parsing, validating) |
| `bgp_runtime_start` | BGP FSM thread spawn and initial peer handshakes |
| `lab_and_net_diag_config_parse` | Lab and network diagnostics config parsing |
| `http_bind` | HTTP server socket creation and bind |
| `http_accept_loop` | HTTP accept loop startup (emits startup_ready) |

## R2/R3 Fixes Applied

### R2: BGP Runtime Start Phase
1. Added `bgp_runtime_start` phase to distinguish BGP config parsing from FSM thread startup
2. This allows diagnosing gaps between config load and actual peering

### R3: Tracer Propagation for startup_ready Emission
1. `serveForeverAfterBind` now accepts `tracer: ?*startup_trace.StartupTracer` parameter
2. Tracer is passed through to `serveForeverNormalWithTracer`
3. When tracer is provided, `startup_ready` event is emitted after http_accept_loop starts
4. When tracer is null, the function behaves identically but skips event emission
5. This enables the R3 seam test to verify tracer→startup_ready chain

## Files Changed

- tovarisch/src/startup_trace.zig
- tovarisch/src/startup_trace_time.zig
- tovarisch/src/startup_trace_unit_tests.zig
- tovarisch/src/startup_trace_integration_tests.zig
- tovarisch/src/http/serve_startup.zig
- tovarisch/src/http/server.zig
- tovarisch/src/cli/daemon_command.zig
- tovarisch/src/logging.zig
- tovarisch/src/test_all.zig
- tovarisch/src/test_suite_http.zig
- docs/operations/startup-readiness.md
