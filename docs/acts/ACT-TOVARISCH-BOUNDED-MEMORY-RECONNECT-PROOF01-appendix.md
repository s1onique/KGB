# ACT-TOVARISCH-BOUNDED-MEMORY-RECONNECT-PROOF01 Appendix

Appendix to `ACT-TOVARISCH-BOUNDED-MEMORY-RECONNECT-PROOF01.md`.
The main ACT is the closure report; this file carries source-derived snippets,
tables, and the complete publication workset.

## Runtime package: one facade plus five private siblings

```text
tovarisch/src/runtime/
  allocation_tracker.zig                       public facade
  allocation_tracker_internal.zig              private state/mutation core
  allocation_tracker_destroy.zig               private destroy validator
  allocation_tracker_tracking_allocator.zig    private producer allocator
  allocation_tracker_snapshots.zig             private read-side projection
  allocation_tracker_connector_probe.zig       private handle token
```

The public facade imports those five siblings and re-exports only bounded
operations and projections. External code cannot import a private sibling or
use a non-literal import expression without the static gate failing.

## Destroy validator (source-derived)

```zig
pub const DestroyError = error{
    HandleStillAdopted,
    HandleCountImbalance,
    SocketStillOpen,
    TimerStillActive,
    ReconnectGenerationLeak,
    OperationLeak,
    PermanentLeak,
};

pub fn validateForDestroy(
    state: *const ReconnectMemoryState,
) DestroyError!void {
    const impl: *const StateImpl = @ptrCast(@alignCast(state));
    if (impl.active_handle != null) return error.HandleStillAdopted;
    if (impl.handles_acquired != impl.handles_released) {
        return error.HandleCountImbalance;
    }
    if (impl.resources.active_sockets != 0) return error.SocketStillOpen;
    if (impl.resources.active_timers != 0) return error.TimerStillActive;
    if (hasLiveLifetime(&impl.allocations, .reconnect_generation)) {
        return error.ReconnectGenerationLeak;
    }
    if (hasLiveLifetime(&impl.allocations, .operation)) {
        return error.OperationLeak;
    }
    if (hasLiveLifetime(&impl.allocations, .permanent)) {
        return error.PermanentLeak;
    }
}
```

The dedicated validator file contains eight behavioral contract tests plus one
compile anchor. The behaviors are: fresh state, socket, timer,
reconnect-generation allocation, operation allocation, permanent allocation,
active handle, and fully clean state.

## Constructor ownership and FD oracle

The constructor consumes `raw`, `tcp`, and `prefixes`. Its common failure
helper performs the following release order:

```zig
if (bundle.socket_owned) {
    bundle.tcp.close();
    bundle.socket_owned = false;
}
bundle.export_state.deinit();
if (bundle.prefixes.len > 0) allocator.free(bundle.prefixes);
bundle.raw.deinit(std.heap.page_allocator);
std.heap.page_allocator.destroy(bundle);
```

The kernel descriptor oracle is symbolic rather than a hard-coded errno:

```zig
const F_GETFD: c_int = 1;
const EBADF: c_int = @intFromEnum(std.c.E.BADF);

const FdProbeOutcome = enum { open, closed, other_error };

fn probeFdState(fd: std.c.fd_t) FdProbeOutcome {
    std.c._errno().* = 0;
    const result = std.c.fcntl(fd, F_GETFD, @as(c_int, 0));
    if (result >= 0) return .open;
    const err = std.c._errno().*;
    if (err == EBADF) return .closed;
    return .other_error;
}
```

## Import recognizer policy

```go
var approvedLiteralImport = regexp.MustCompile(
    `^@import\s*\(\s*"([^"\\\r\n]+)"\s*,?\s*\)`,
)
```

The regex is no longer the discovery mechanism. `codeImportOffsets` first
lexically finds every real `@import` token after masking comments and literal
text. `FindSubmatch(contents[offset:])` then approves the one supported
literal shape. No match is itself a policy finding. A match is path-resolved
and compared with the canonical runtime paths for these five basenames:

```go
var privateSiblingBasenames = []string{
    "allocation_tracker_internal.zig",
    "allocation_tracker_destroy.zig",
    "allocation_tracker_tracking_allocator.zig",
    "allocation_tracker_snapshots.zig",
    "allocation_tracker_connector_probe.zig",
}
```

The source pathspec is:

```go
const SourceScopePathspec = ":(glob)tovarisch/src/**/*.zig"
```

Explicit `glob` semantics are required so `**/` includes zero directory
levels; the old unadorned pathspec omitted Zig files directly in
`tovarisch/src/`.

## Self-test matrix

| # | Fixture/mutation | Expected |
| ---: | --- | --- |
| 1 | tracked external import of internal | reject |
| 2 | distinct untracked external import of internal | reject |
| 3 | public facade | allow |
| 4 | tracked runtime sibling | filtered/allow |
| 5 | untracked runtime sibling | filtered/allow |
| 6 | destroy sibling | reject |
| 7 | tracking-allocator sibling | reject |
| 8 | snapshots sibling | reject |
| 9 | connector-probe sibling | reject |
| 10 | normalized `./runtime/...` path | reject |
| 11 | multiline direct literal | reject |
| 12 | existing `tools/` same-basename decoy | compile and allow |
| 13 | literal concatenation argument | reject non-literal |
| 14 | identifier argument | reject non-literal |
| 15 | comments/ordinary/multiline strings | mask/allow |
| 16 | cached file deleted after inventory | scanner error/fail closed |
| 17 | inventory entry is a directory | scanner error/fail closed |

The suite additionally asserts that the tracked and untracked paths occupy
only their respective Git inventories, that runtime controls disappear from
both filtered inventories, and that the root-inclusive source pathspec sees
both top-level controls.

## Dedicated proof composition

```text
tovarisch/src/reconnect_proof_tests.zig
  bgp/reconnect_proof_harness.zig                       shared harness
  bgp/reconnect_proof_tests.zig                         10 tests
  bgp/reconnect_proof_regression.zig                     3 tests
  bgp/reconnect_proof_production_init_tests.zig          1 test
  bgp/reconnect_proof_validate_destroy_tests.zig         8 + 1 compile anchor
  bgp/reconnect_proof_constructor_failure_tests.zig      2 tests
```

Transitive tests from imported production modules bring the dedicated artifact
to 102 tests. The aggregate artifact reports 1761 total tests.

## Final focused-file line inventory

Measured with `wc -l` after the correction:

```text
   43 cmd/verify-allocation-tracker-imports/main.go
  282 internal/tooling/allocationtrackerimports/scanner.go
  392 internal/tooling/allocationtrackerimports/selftest.go
  150 tovarisch/src/runtime/allocation_tracker.zig
  138 tovarisch/src/runtime/allocation_tracker_destroy.zig
  441 tovarisch/src/runtime/allocation_tracker_internal.zig
  200 tovarisch/src/runtime/allocation_tracker_snapshots.zig
  139 tovarisch/src/runtime/allocation_tracker_tracking_allocator.zig
   64 tovarisch/src/runtime/allocation_tracker_connector_probe.zig
  449 tovarisch/src/bgp/serve_integration.zig
  174 tovarisch/src/bgp/serve_bundle_constructor.zig
  445 tovarisch/src/bgp/reconnect_ownership.zig
  215 tovarisch/src/bgp/reconnect_proof_harness.zig
  367 tovarisch/src/bgp/reconnect_proof_tests.zig
  274 tovarisch/src/bgp/reconnect_proof_regression.zig
  170 tovarisch/src/bgp/reconnect_proof_production_init_tests.zig
  285 tovarisch/src/bgp/reconnect_proof_validate_destroy_tests.zig
  371 tovarisch/src/bgp/reconnect_proof_constructor_failure_tests.zig
   22 tovarisch/src/reconnect_proof_tests.zig
```

Every focused ACT source/tool file is below the 450-line hard limit.
`scripts/quality_gate.sh` is 447 physical lines and remains at its script-
doctrine logical-LOC ceiling. `docs/tooling/zig-0.16-observations.md` remains at its
400-line hard limit; the new focused import observation is split into
`zig-0.16-import-observations.md`.

## Complete publication workset (34 files)

### Modified tracked files (14)

- `.factory/gate-summary.json`
- `Makefile`
- `docs/memory/bounded-memory-reconnect-ownership.md`
- `docs/tooling/zig-0.16-observations.md`
- `scripts/quality_gate.sh`
- `tovarisch/build.zig`
- `tovarisch/src/bgp/bgp_reconnect_regression_tests.zig`
- `tovarisch/src/bgp/passive_listener_integration_tests.zig`
- `tovarisch/src/bgp/reconnect_stress_tests.zig`
- `tovarisch/src/bgp/runtime.zig`
- `tovarisch/src/bgp/serve_integration.zig`
- `tovarisch/src/runtime/allocation_tracker.zig`
- `tovarisch/src/test_all.zig`
- `tovarisch/src/test_suite_bgp_integration.zig`

### Added files (20)

- `docs/acts/ACT-TOVARISCH-BOUNDED-MEMORY-RECONNECT-PROOF01.md`
- `docs/acts/ACT-TOVARISCH-BOUNDED-MEMORY-RECONNECT-PROOF01-appendix.md`
- `docs/tooling/zig-0.16-import-observations.md`
- `cmd/verify-allocation-tracker-imports/main.go`
- `internal/tooling/allocationtrackerimports/scanner.go`
- `internal/tooling/allocationtrackerimports/selftest.go`
- `tovarisch/src/bgp/reconnect_ownership.zig`
- `tovarisch/src/bgp/reconnect_proof_constructor_failure_tests.zig`
- `tovarisch/src/bgp/reconnect_proof_harness.zig`
- `tovarisch/src/bgp/reconnect_proof_production_init_tests.zig`
- `tovarisch/src/bgp/reconnect_proof_regression.zig`
- `tovarisch/src/bgp/reconnect_proof_tests.zig`
- `tovarisch/src/bgp/reconnect_proof_validate_destroy_tests.zig`
- `tovarisch/src/bgp/serve_bundle_constructor.zig`
- `tovarisch/src/reconnect_proof_tests.zig`
- `tovarisch/src/runtime/allocation_tracker_connector_probe.zig`
- `tovarisch/src/runtime/allocation_tracker_destroy.zig`
- `tovarisch/src/runtime/allocation_tracker_internal.zig`
- `tovarisch/src/runtime/allocation_tracker_snapshots.zig`
- `tovarisch/src/runtime/allocation_tracker_tracking_allocator.zig`
