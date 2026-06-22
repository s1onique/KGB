# Native-Owned Critical Paths Doctrine

**Principle:** Project-critical runtime paths should prefer small, native-owned code when that improves reliability, observability, portability, debuggability, or product fit.

## Purpose

Recent engineering work has confirmed a useful lesson: when a runtime path is project-critical and executed frequently, native-owned code (Go, Zig) provides advantages that external CLI composition cannot match:

- **Reliability**: No SIGSEGV from os/exec fork/exec on constrained ARM64 routers (UVB-76 ICMP crash investigation).
- **Observability**: Structured telemetry, bounded memory, predictable resource usage.
- **Portability**: Consistent behavior across embedded/constrained hosts without shell dependency.
- **Debuggability**: Stack traces, memory profiling, crash dumps work on native paths.
- **Product fit**: Fine-grained timeout, cancellation, retry control without shell wrapper complexity.

CLI composition is appropriate when it is a thin wrapper, not when it is the primary execution engine for project-critical paths.

## Core Rule

**Native-owned implementation is preferred for critical-path runtime behavior.**

CLI composition is allowed for:
- Prototypes and one-shot diagnostics
- Operator glue (manual orchestration)
- Stable low-risk wrappers around mature external tools
- Build/test/dev tooling

CLI composition is NOT appropriate for:
- High-frequency per-second execution
- Embedded/constrained host primary paths
- Structured telemetry generation
- Precise timeout/cancellation requirements

## Anti-NIH Clause

**Reject "rewrite everything because we can."**

Native-owned does not mean:
- "Invent crypto" — use boring, proven crypto libraries
- "Invent parsers for complex standards without need" — use mature parsers when available
- "Fork mature infrastructure casually" — reuse stable implementations when ownership burden would exceed benefit

**Reuse is favored when:**
- Library is stable, well-scoped, and inspectable
- Library is not an operational black box
- Library ownership burden would exceed native implementation cost
- Library provides correct semantics without heroic engineering

**Examples of appropriate reuse:**
- Go standard library crypto/net packages
- Zig standard library networking
- Mature protocol implementations (WireGuard, ICMP RFC-compliant libraries)
- Established parsers for stable formats (JSON, CSV, line-based outputs from stable commands)

## Decision Matrix

| Scenario | Preferred | Notes |
|---------|-----------|-------|
| Critical runtime path | Native code | Reliability, observability, crash debugging |
| Per-second or high-frequency execution | Native code | CLI fork/exec overhead is expensive on constrained hosts |
| Embedded/constrained host | Native code | Shell dependencies add fragility |
| Needs structured telemetry | Native code | CLI output parsing is brittle |
| Needs precise timeout/cancellation | Native code | SIGKILL semantics differ from graceful cancellation |
| Needs cross-platform stability | Native code | Shell behavior varies by POSIX compliance |
| External CLI output is unstable | Native code | Human-output parsers break on locale, version changes |
| External CLI is mature, stable, low-frequency | CLI/Library OK | `ss -tin`, `wg show` for low-frequency diagnostics |
| Prototype/operator script | CLI OK | Rapid iteration, manual control |
| Build/test/developer workflow | CLI OK | Not on critical path, bounded by user action |

## Examples

### UVB-76 ICMP: Native ICMP Backend

**Context:** Per-second ICMP probes to constrained ARM64 routers (ASUS RT-AX88U) caused SIGSEGV when implemented via `os/exec ping`.

**Resolution:** Native Go ICMP backend using raw sockets — `uvb76/probe/native_icmp.go`.

**Why native is correct here:**
- Per-second execution makes fork/exec overhead measurable
- Constrained RAM/CPU amplifies subprocess overhead
- Crash debugging requires native stack traces
- Future latency histograms need native telemetry hooks

**Legacy fallback:** `uvb76/probe/icmp_parse.go` still uses `exec.CommandContext("ping")` as OS-ping fallback only; not on critical path by default.

### Tovarisch Diagnostics: Native Parsers

**Context:** Project-owned status surfaces (`tovarisch/src/status.zig`) use native parsers for structured output.

**Why native is correct here:**
- Status surface is observable surface — must be testable and stable
- CLI output parsing is brittle across Zig version differences
- Native parsing enables deterministic behavior coverage

### Shell Remains Acceptable for Thin Wrappers

**Examples:**
- `scripts/coverage_gate.sh` — invokes `zig`, parses output
- `scripts/verify_opkg_package.sh` — invokes binary, checks exit code
- `scripts/check_final_newlines_regression.sh` — file walk, exit code check

**These are appropriate because:**
- Not on critical runtime path
- Bounded by developer action, not per-request execution
- Output is simple (exit codes, file checks)

### External Protocol Reuse Remains Acceptable

**Examples:**
- WireGuard `wg show` — external CLI, low-frequency diagnostics
- `ss -tin` for socket state — stable output format, bounded frequency
- iptables interaction — constrained to configuration, not probing

**These are acceptable when:**
- Tool is mature and stable
- Output format is predictable and documented
- Frequency is low (not per-second)
- Error recovery is simple (fallback or skip)

## Rejection Triggers

Any reviewer or agent should flag these patterns:

| Trigger | Reason |
|---------|--------|
| New `os/exec`, `subprocess`, `child_process`, shell pipeline, or Zig process spawn in runtime code without inventory entry | Untracked drift; runtime path risk |
| Parsing human CLI output in runtime path when native API/procfs/netlink/library path is available | Fragile, locale/version-dependent |
| Silent fallback from native implementation to CLI path in production | Degrades reliability silently |
| CLI path without bounded timeout, output bound, redaction, and structured error classification | Unbounded resource risk |
| New native rewrite without anti-NIH justification | Unnecessary complexity |

## Required Close-Report Evidence

### Adding a CLI/Process Execution Path

Any ACT adding CLI/process execution must report:

```
## CLI Composition Justification

- **why CLI is acceptable**: <brief reason>
- **timeout behavior**: <bounded/unbounded, timeout value>
- **output bounds**: <max bytes, truncation strategy>
- **error classification**: <structured error types>
- **redaction posture**: <sensitive data handling>
- **inventory row**: <CLI-XXXX reference>
- **tests**: <test coverage added>
```

### Replacing CLI with Native Implementation

Any ACT replacing CLI with native must report:

```
## Native Implementation Evidence

- **native interface boundary**: <what calls what>
- **dependency choice**: <library or custom implementation>
- **telemetry added**: <structured output, metrics>
- **fallback behavior**: <what happens if native path fails>
- **migration/soak status**: <how long tested, what deployment targets>
```

## Doctrine Index Entry

This document is part of KGB core doctrine. See also:

- [factory.md](./factory.md) — Factory workflow
- [kgb.md](./kgb.md) — KGB architecture split
- [shell-containment.md](./shell-containment.md) — Shell wrapper policy
- [privacy.md](./privacy.md) — Data handling principles
- [tiny-leafs.md](./tiny-leafs.md) — Leaf constraints
