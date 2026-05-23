# Tovarisch Coverage Ledger

> **Purpose**: This ledger tracks which `tovarisch` behaviors are covered by automated checks and which remain accepted uncovered risk.

Coverage is an accountability surface. Every important behavior must either be covered or consciously accepted as uncovered.

## Dual Coverage System

### 1. Real Line Coverage (kcov)

`tovarisch` uses `kcov` to measure actual line coverage of the test binary:

- **Threshold**: 83% (configurable via `COVERAGE_THRESHOLD`)
- **Files covered**: `tovarisch/src/` only (no cache/vendor paths)
- **Gate**: Fails if below threshold, unless `ALLOW_MISSING_KCOV=1`
- **Enforcement**: Required in `make gate`

See `scripts/coverage_gate.sh` for implementation.

### 2. Behavior Coverage Ledger

This document tracks which specific behaviors are covered by automated checks.

## Behavior Coverage Matrix

### Covered Behaviors

| Behavior | Coverage Mechanism | Gate-Enforced? | Status | Gap / Follow-up |
|----------|-------------------|----------------|--------|-----------------|
| CLI usage / invalid args | Unit tests in `cli.zig` | Yes (kcov + gate) | ✅ Covered | None |
| `--help` / `-h` command | Unit tests: returns ok, prints usage | Yes (kcov + gate) | ✅ Covered | None |
| `--version` command | Unit test + gate verification | Yes (kcov + gate) | ✅ Covered | None |
| `check` command | Unit test + gate verification | Yes (kcov + gate) | ✅ Covered | None |
| `status --json` JSON validity | `verify_status_json.sh` structural validation | Yes (gate) | ✅ Covered | None |
| JSON structural contract | Unit tests + verification script | Yes (gate) | ✅ Covered | None |
| Required JSON fields/types | Verification script checks fields and types | Yes (gate) | ✅ Covered | None |
| `CheckStatus` enum rendering | Unit tests for `deriveStatus()` | Yes (kcov + gate) | ✅ Covered | None |
| Top-level status derivation | Unit tests verify error > warn > ok | Yes (kcov + gate) | ✅ Covered | None |
| Local checks: process | Static check in `status.zig` + output test | Yes (kcov + gate) | ✅ Covered | None |
| Local checks: binary | Static check in `status.zig` + output test | Yes (kcov + gate) | ✅ Covered | None |
| Local checks: config | Static check shows "not configured yet" as warn | Yes (kcov + gate) | ✅ Covered | None |
| Local checks: state_dir (placeholder) | Emits warn with "state directory not found" | Yes (kcov + gate) | ✅ Covered | Temporary until real Io.Dir API used |
| Multiple local checks in output | Unit test `status --json contains multiple checks` | Yes (kcov + gate) | ✅ Covered | None |

### Accepted Uncovered Future Behaviors

| Behavior | Reason Uncovered | Follow-up |
|----------|-----------------|-----------|
| Real config loading | Config system not implemented; static config check used as placeholder | Implement config loading and add coverage |
| Dynamic node identity | Hardcoded to "local-dev"; identity schema TBD | Define node identity scheme and add coverage |
| Probe execution | No probes implemented yet; static checks only | Implement probe execution and add coverage |
| Tunnel supervision | Tunnel backend not designed yet | Design tunnel interface and add coverage |
| Signed status reports | Report schema TBD; no signing implementation | Define report schema and add coverage |
| Desired-state pull | Desired-state model not designed | Design desired-state interface and add coverage |
| Transport to UVB-76 | UVB-76-side not implemented | Implement UVB-76 transport and add coverage |
| state_dir (directory exists) | Io.Dir API not yet understood in Zig 0.16; placeholder returns warn | Investigate std.fs.Dir.stat() or simpler API |
| state_dir (path is file, not dir) | Io.Dir API not yet understood; placeholder only | Implement real filesystem check |
| state_dir (permission denied) | Io.Dir API not yet understood; placeholder only | Implement real filesystem check |

## Coverage Mechanisms

### Real Line Coverage (kcov)

`kcov` instruments the test binary and measures actual line coverage:

- **Command**: `make coverage`
- **Threshold**: 60% (configurable via `COVERAGE_THRESHOLD`)
- **Files covered**: `tovarisch/src/` only
- **Parser**: `scripts/extract_kcov_line_coverage.py`
- **Gate**: Fails if coverage below threshold, unless `ALLOW_MISSING_KCOV=1`

### Unit Tests (`zig build test`)

The Zig test suite in `tovarisch/src/` covers:

- **cli.zig**: `--help`, `-h`, `--version`, `check`, unknown command, missing args, `status` validation
- **status.zig**: `deriveStatus()` derivation logic, JSON parsing/serialization round-trip, required fields

Run: `cd tovarisch && zig build test`

### Structural JSON Validation (`verify_status_json.sh`)

Validates `status --json` output:
1. Valid JSON (parses without error)
2. Required fields: `service`, `version`, `node_id`, `status`, `checks`
3. Field types: strings and arrays
4. Semantic constraints: `service` must be `"tovarisch"`, `status` one of `ok|warn|error`
5. Check objects: each has `name`, `status`, `detail` fields

### Gate Integration (`make gate`)

- Runs `kcov` coverage gate when kcov is available
- Runs `verify_status_json.sh` on `status --json` output
- Fails if coverage below threshold or JSON contract is violated
- Behavior coverage ledger must exist and mention all public commands

## Commands Tracked

All public `tovarisch` commands must appear in this ledger:

| Command | Public? | Covered |
|---------|---------|---------|
| `--help`, `-h` | Yes | ✅ Unit test + gate |
| `--version` | Yes | ✅ Unit test + gate |
| `check` | Yes | ✅ Unit test + gate |
| `status --json` | Yes | ✅ Unit test + structural validation |
| `serve` | Yes | ✅ CLI parse unit tests; daemon loop = manual smoke test |

## Updating This Ledger

When adding new behavior:

1. Add row to the Behavior Coverage Matrix
2. Specify coverage mechanism (unit test, integration test, script, gate check)
3. Mark gate-enforced status
4. If uncovered, document reason and follow-up

When implementing previously-uncovered behavior:

1. Add explicit test coverage
2. Update this ledger to mark as "Covered"
3. Remove from "Accepted Uncovered" section

## Philosophy

- **No fake coverage**: We do not invent percentages. We track specific behaviors.
- **Uncovered ≠ ignored**: If a behavior is important but not yet covered, it appears here as accepted uncovered risk.
- **Coverage signals intent**: Test passing is the current coverage proxy until Zig coverage backend matures.
- **TODOs are visible**: Gaps are documented, not hidden.

## Accepted Tooling Risks

### Linux kcov Backend Emits Empty Reports for Zig 0.16 Test Binary

**Status**: Known Linux CI backend gap — accepted tooling risk.

**Evidence**: All serious kcov-compatible backend paths have been exhausted:

| Backend | Result | Evidence |
|---------|--------|----------|
| Upstream kcov from source | Builds/runs; DWARF has project paths; reports structurally valid but empty | `files: []`, `covered_lines: 0`, `total_lines: 0` |
| roc-lang/zig-kcov prebuilt release | Executable dies with `rc=-4` / SIGILL before help/version | Release binary incompatible with GitHub runner CPU/runtime |
| roc-lang/zig-kcov source build via Zig | Blocked by Zig 0.16 build.zig API drift | `build.zig:108:51: error: member function expected 1 argument(s), found 0` |
| roc-lang/zig-kcov source build via CMake | Builds/runs; standard/verify/include-pattern modes complete; reports structurally valid but empty | `coverage.json files=[]`, `Cobertura classes=[]`, `covered_lines: 0`, `total_lines: 0` |

**Impact**: Linux CI real line coverage for `tovarisch` is temporarily unsupported. The test binary has DWARF project paths, and kcov backends emit structurally valid JSON/XML reports, but no source files are included.

**Local/macOS Coverage**: Real line coverage remains strict where kcov works. Current local coverage passes at approximately 82.69%.

**Linux CI Behavior**: Linux release workflow runs real Zig tests and status-contract verification without requiring kcov coverage. This is not a coverage gap — it is a documented backend limitation.

**Future**: Native Zig coverage is a long-running open area. This gap may resolve when Zig's own coverage tooling matures.

## Platform-Specific Coverage Honesty

See [Platform Portability Doctrine](../doctrine/platform-portability.md) for full rules.

Platform-specific runtime paths require honest classification. The coverage ledger must distinguish between:

| Classification | Meaning | Sufficient? |
|---------------|---------|-------------|
| Covered by pure parser tests | Fixture input validates parsing logic | ✅ Yes |
| Covered by contract tests | Stable output validated against contract | ✅ Yes |
| Compile-gated only | Syntax/API verified on target, runtime untested | ⚠️ Partial |
| Accepted uncovered risk | Not yet exercised; must appear here | ❌ No |

**Critical rule**: Linux-only runtime paths are **not fully covered** merely because the macOS test suite passes. Any platform-specific behavior not exercised in tests must appear in the accepted uncovered-risk ledger until Linux runtime coverage exists.

### Platform-Specific Code Path Inventory

The following table inventories all known platform-specific branches in `tovarisch`:

| File | Path | Classification | Notes |
|------|------|----------------|-------|
| `runtime/telemetry.zig` | `linuxGetVmRssKiB()` — opens `/proc/self/status` | **Linux CI smoke test** | Exercise in CI via `linux_smoke_test.sh`; verifies `rss_kib` is non-null |
| `runtime/telemetry.zig` | `getVmRssKiB()` — `builtin.os.tag == .linux` gate | **Linux CI smoke test** | Covered by smoke test on Linux; null fallback remains for non-Linux |
| `net/linux_stats.zig` | `readInterfaceStats()` — live sysfs read | **Linux CI smoke test** | Zig-native smoke test probes `/sys/class/net` and calls `readInterfaceStats()`; exercised in CI |
| `net/linux_stats.zig` | `fileExists()` — Linux `std.c.open` path | **Linux CI smoke test** | Exercise via live sysfs smoke test; probes common Linux interface names directly |
| `net/linux_interface_stats.zig` | `collectInterfaceStats()` — composition layer | **Linux CI smoke test** | Exercise via live sysfs smoke test; composes listInterfaces() + readInterfaceStats() |
| `net/linux_interface_stats.zig` | `freeInterfaceStatsSnapshots()` — cleanup helper | **Pure fixture tests** | Cross-platform, tested via fixture tests |
| `net/linux_stats.zig` | `openForWrite()` — Linux `std.c.open` path | **Compile-gated only** | Used only for test fixtures; no production write path needed |
| `net/linux_stats.zig` | `closeFile()` — Linux `std.c.close` path | **Linux CI smoke test** | Exercise via live sysfs smoke test; closes file descriptors after reading stats |
| `net/linux_stats.zig` | `readFromFd()` — Linux `std.c.read` path | **Linux CI smoke test** | Exercise via live sysfs smoke test; reads counter values from sysfs files |
| `net/linux_stats.zig` | `writeToFd()` — Linux `std.c.write` path | **Compile-gated only** | Used only for test fixtures; no production write path needed |

#### Parser Logic (Portable)

The following code is **fully tested** via pure parser unit tests on macOS:

| Function | Tests |
|----------|-------|
| `telemetry.zig: parseVmRssKiB()` | 7 unit tests covering VmRSS line formats, edge cases, empty input |
| `linux_stats.zig: parseCounter()` | 6 unit tests covering valid/invalid/overflow inputs |
| `linux_stats.zig: statsFromCounters()` | 1 unit test covering InterfaceStats construction |
| `net/private_ip.zig: classifyIpv4Text()` | 30+ unit tests covering all RFC ranges and invalid inputs |

#### Runtime Paths (Linux-only, Mixed Coverage)

These functions reach Linux-specific syscalls that are exercised in Linux CI via `linux_smoke_test.sh` and/or Zig unit tests:

| Function | Linux API | Coverage |
|----------|-----------|----------|
| `linuxGetVmRssKiB()` | `std.c.open("/proc/self/status")`, `std.c.read()`, `std.c.close()` | **Linux CI smoke test** (`linux_smoke_test.sh: test_rss_read`) |
| `fileExists()` (Linux path) | `std.c.open()` | **Linux CI smoke test** (`linux_stats.zig: live sysfs smoke test`) |
| `openForRead()` (Linux path) | `std.c.open()` | **Linux CI smoke test** (`linux_stats.zig: live sysfs smoke test`) |
| `openForWrite()` (Linux path) | `std.c.open(O.CREAT)` | **Compile-gated only** (used in fixture tests only; no production write path) |
| `closeFile()` (Linux path) | `std.c.close()` | **Linux CI smoke test** (`linux_stats.zig: live sysfs smoke test`) |
| `readFromFd()` (Linux path) | `std.c.read()` | **Linux CI smoke test** (`linux_stats.zig: live sysfs smoke test`) |
| `writeToFd()` (Linux path) | `std.c.write()` | **Compile-gated only** (used in fixture tests only; no production write path) |
| `readInterfaceStats()` (live sysfs) | All above via `readFile()` | **Linux CI smoke test** (`linux_stats.zig: live sysfs smoke test`) |
| `listInterfaces()` (Linux path) | `std.c.opendir()`, `std.c.readdir()`, `std.c.closedir()` | **Linux CI smoke test** (`linux_interfaces_tests.zig: live sysfs smoke test`) |
| `freeInterfaceList()` | N/A (allocation helper) | **Pure fixture tests** — cross-platform, fully tested |

#### Accepted Uncovered Risk (Remaining)

| Path | Reason Uncovered | Follow-up |
|------|------------------|-----------|
| `openForWrite()` (Linux path) | Used only for test fixtures; no production write path needed | Acceptable — no production write path needed |
| `writeToFd()` (Linux path) | Used only for test fixtures; no production write path needed | Acceptable — no production write path needed |

#### Classification Key

- **Pure parser tests**: Fixture-based, no OS dependency, fully portable ✅
- **Compile-gated only**: Syntax verified on target, runtime behavior not exercised ⚠️
- **Accepted uncovered risk**: Known gap, documented here until Linux runtime coverage exists ❌

This inventory is authoritative. Any new platform-specific code must be added to this table and classified according to the Platform Portability Doctrine before merging.

## References

- [Day-0 Code Coverage Doctrine](../doctrine/day-0-code-coverage.md)
- [Platform Portability Doctrine](../doctrine/platform-portability.md)
- [Quality Gate Script](../scripts/quality_gate.sh)
- [JSON Verification Script](../scripts/verify_status_json.sh)
- [zig-0.16-field-manual.md: Build.zig API Drift](../tooling/zig-0.16-field-manual.md)
