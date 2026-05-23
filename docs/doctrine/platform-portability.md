# Platform Portability Doctrine

`tovarisch` must remain portable by design, even when its primary deployment target is Linux cheap-VM leaf nodes.

## Rule

Platform-specific code paths must be:

1. **Minimal** — Keep platform branches as small as possible
2. **Isolated** — Fully isolated behind narrow interfaces
3. **Compile-gated** — Built in CI for every supported target, with platform selection kept explicit and narrow
4. **Tested** — Covered by pure tests where possible
5. **Documented** — Clearly documented when runtime coverage is not available

## Why

Local development commonly happens on macOS, while production leaf deployment will often happen on Linux.

This creates a dangerous gap: Linux-only branches may compile only in CI and may not receive the same runtime test coverage as portable code. Until Linux runtime coverage is available and trustworthy, platform-specific code must be treated as hazardous material.

## Design Policy

### Portable Code Is the Default

Platform-specific code may exist only when there is a clear operating-system boundary, such as:

- Process telemetry
- Filesystem/procfs/sysfs access
- Socket/listener implementation details
- Service management integration
- Routing/network interface inspection

### Module Organization

Platform-specific implementation must live in focused modules with obvious names, for example:

- `runtime/linux_telemetry.zig`
- `runtime/macos_telemetry.zig`
- `net/linux_sysfs.zig`
- `http/linux_listener.zig`

Portable modules must depend on small interfaces, not scattered `builtin.os.tag` checks.

## Forbidden Pattern

Do **not** spread platform branching across business logic:

```zig
if (builtin.os.tag == .linux) {
    // ... do something linux-specific
} else if (builtin.os.tag == .macos) {
    // ... do something macos-specific
}
```

inside unrelated command, status, HTTP, or health-check code.

## Preferred Pattern

Use a narrow adapter boundary:

```zig
const platform = @import("platform.zig");

const rss = platform.runtime.currentRssKiB();
```

The platform module may dispatch internally, but the caller should not care which OS is running.

## Testing Policy

Every platform-specific module must have at least one of:

1. **Pure parser tests** using fixture input (e.g., parsing `/proc/self/stat` format strings)
2. **Contract tests** against stable public output
3. **Compile-only gate** for the target (proves type/API compatibility)
4. **Explicit accepted uncovered-risk entry** in the coverage ledger

> **Linux-only runtime paths must not be considered fully covered merely because the macOS test suite passes.**

## CI Policy

The gate must include cross-platform compile checks for supported targets.

At minimum:

- Native developer target
- Linux target used for release artifacts (see `scripts/quality_gate.sh` / `make tovarisch-compile-linux`)

Compile-only gates are useful but not sufficient. They prove syntax and type compatibility, not behavioral correctness.

See `docs/tooling/scripts-inventory.md` for cross-platform compile gate tooling.

## Coverage Honesty

Coverage reports must distinguish:

| Category | Definition |
|----------|------------|
| **Portable code** | Covered by unit tests on the local (macOS) developer machine |
| **Platform-specific parser logic** | Covered by pure fixtures that parse OS-native data formats |
| **Platform-specific runtime branches, compile-gated** | Syntax/API verified but runtime untested |
| **Platform-specific runtime branches, uncovered** | Not yet exercised — must appear in accepted uncovered-risk ledger |

Any Linux-only behavior that is not executed in tests must appear in the accepted uncovered-risk ledger until Linux runtime coverage exists.

See `docs/coverage/tovarisch-coverage.md` for the coverage ledger format.

## File-Size Policy

Platform adapters must remain small and boring.

If a platform-specific module grows large, split it by concern before it becomes a hidden second application.

## Related Doctrine

- [Day-0 Code Coverage](./day-0-code-coverage.md) — Coverage philosophy and tooling rules
- [Tiny Leafs](./tiny-leafs.md) — Leaf node constraints
- [LLM-Friendliness](./llm-friendliness.md) — Code readability for agents
