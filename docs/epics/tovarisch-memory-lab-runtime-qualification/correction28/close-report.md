# CORRECTION28 Close Report

## Identity

| Field | Value |
|-------|-------|
| correction | CORRECTION28 |
| title | Tool-Independent Control Plane — Eliminating shell/curl/wget |
| subject_commit (S28) | (this commit — working tree with CORRECTION28) |
| subject_tree (ST28) | (this commit tree) |
| evidence_commit (E28) | (this commit) |
| parent_commit (S27) | 6380816554371d250afcfbf395c70698f72d8788 |
| parent_tree (ST27) | be707d281924a0ae99ccf881907a43009e028f75 |
| closed_at | 2026-07-24T23:43:00+03:00 |

## Baseline CORRECTION27 Assessment

**CORRECTION27: COMPLETE** — Docker exec-based reachability is correct and complete. CORRECTION28 builds on CORRECTION27 by eliminating shell/curl dependency inside containers.

## CORRECTION28 Changes

### P0-1: Add Attempted/Completed fields to CanaryControlExecResult

The `CanaryControlExecResult` struct was missing fields needed to capture workload operation results from the `operate` subcommand.

**File:** `tovarisch/labs/memory/internal/dockerlab/client.go`

```go
type CanaryControlExecResult struct {
    ExitCode      int
    Stdout        string
    Stderr        string
    HealthValid   bool
    StateValid    bool
    WorkloadValid bool
    State         *CanaryStateFromExec
    Attempted     int  // NEW
    Completed     int  // NEW
    Error         error
}
```

### P0-2: Tool-Independent Control Subcommand

Created `cmd/canary/control.go` implementing the `control` subcommand with three commands:
- `health` — Check canary health
- `state` — Get canary state
- `operate` — Perform N operations

The control subcommand uses Go's `net/http` directly with no shell, no curl, no wget.

**Key features:**
- Exact argv execution: `/app/canary control <command>`
- Proper timeout handling via `context.WithTimeout`
- Response body size limit (64KB) to prevent memory issues
- Proper error handling with diagnostic output

### P0-3: Updated Docker Client to Use canary-control

Updated `CanaryHealthCheckViaExec()`, `CanaryStateViaExec()`, and `CanaryOperateViaExec()` to use `/app/canary control <command>` instead of shell/curl.

**Before (CORRECTION27):**
```go
exitCode, _, err := c.ContainerExec(ctx, containerID, []string{
    "sh", "-c",
    fmt.Sprintf("curl -sf http://localhost:%d/health || ...", port),
})
```

**After (CORRECTION28):**
```go
result := c.CanaryControlExec(ctx, containerID, []string{
    "/app/canary", "control", "health",
    "--port", fmt.Sprintf("%d", port),
    "--timeout", "5s",
})
```

### P0-4: Canary Binary Registration

Updated `cmd/canary/main.go` to register the `control` subcommand in the command dispatch table.

## Binary Metadata

### Canary Binary

| Field | Value |
|-------|-------|
| SHA-256 | cdbd93f51176e6c211c42ddaafe22a900bd092e423cdc94d329b2326501023d2 |
| Path | /tmp/canary |
| Built from | S28 working tree |

### Production CLI Binary

| Field | Value |
|-------|-------|
| SHA-256 | 5e631fcf4bbacc674f652282aee7936262c680546abc70b3f96698e1fda3d12a |
| Path | /tmp/tovarisch-memory-lab |
| Built from | S28 working tree |

## Verification

### Go Tests

```
ok  	github.com/s1onique/KGB/tovarisch/labs/memory	(cached)
ok  	github.com/s1onique/KGB/tovarisch/labs/memory/cmd/canary	0.051s
ok  	github.com/s1onique/KGB/tovarisch/labs/memory/cmd/tovarisch-memory-lab	32.897s
ok  	github.com/s1onique/KGB/tovarisch/labs/memory/internal/dockerlab	0.019s
ok  	github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence	0.014s
```

### Zig Build and Test

```
cd tovarisch && zig build
cd tovarisch && zig build test
test
+- run test w
RECONNECT_PROOF completed_generations=10000 ...
```

### Memory Gate

```
=== memory-gate passed ===
```

## Files Changed

- `tovarisch/labs/memory/cmd/canary/control.go` (new)
- `tovarisch/labs/memory/cmd/canary/main.go` (modified)
- `tovarisch/labs/memory/cmd/tovarisch-memory-lab/main.go` (modified)
- `tovarisch/labs/memory/internal/dockerlab/client.go` (modified)

## Command/Exit-Code Matrix

| Command | Exit Code |
|---------|-----------|
| go test ./... | 0 |
| go build ./cmd/canary | 0 |
| go build ./cmd/tovarisch-memory-lab | 0 |
| zig build | 0 |
| zig build test | 0 |
| make memory-gate | 0 |
| make tovarisch-build | 0 |
| make tovarisch-test | 0 |

## Doctrine Compliance

### Shell Containment

CORRECTION28 eliminates shell/curl dependency as required by:
- `docs/doctrine/shell-containment.md`
- `docs/doctrine/ai-native-code-discipline-axioms.md`

### Native-Owned Critical Paths

The canary-control binary is owned by the image, not the host, ensuring:
- No host-side tool dependency
- Consistent behavior across environments
- Tool-independent transport layer

### Embedded Memory Frugality

- Fixed response body limit (64KB) prevents unbounded memory growth
- Proper timeout handling prevents resource exhaustion
- No external process spawning (shell-free)
