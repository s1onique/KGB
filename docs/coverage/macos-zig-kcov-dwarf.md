# macOS Zig 0.16 / kcov DWARF Coverage Limitation

> **Purpose**: Document the known macOS-specific limitation where kcov produces coverage numbers from binaries compiled without debug-line information.

## The Problem

On macOS ARM64 (Darwin), `tovarisch`'s Zig test binary compiles without debug-line info, causing kcov to emit coverage numbers from binaries with incomplete DWARF line tables.

### Symptom

```
[DWARF-DIAGNOSTIC] === Binary analysis for source mapping ===
[DWARF-DIAGNOSTIC] file type:
[DWARF-DIAGNOSTIC] /path/to/tovarisch-test: Mach-O 64-bit executable arm64
[DWARF-DIAGNOSTIC] readelf not available — skipping section listing
[DWARF-DIAGNOSTIC] checking for project source paths in DWARF line tables...
[DWARF-DIAGNOSTIC] WARNING: no project source paths found in DWARF line tables
[DWARF-DIAGNOSTIC] This suggests Zig compiled the tests without debug-line info
[DWARF-DIAGNOSTIC] === End binary analysis ===
```

### Effect

- kcov still produces a coverage percentage (e.g., 85.05%)
- But the DWARF diagnostic shows Zig compiled the tests without debug-line info
- The coverage number is **untrustworthy** because kcov cannot map coverage data to source lines
- The threshold comparison becomes meaningless when DWARF is incomplete

### Root Cause

Zig 0.16 on macOS may not emit `DW_AT_name` paths for source files when building with `-Doptimize=Debug`. The DWARF line table is missing project source paths, making kcov unable to perform source-line mapping.

## Detection

The `coverage_gate.py` script detects this condition via DWARF diagnostics:

1. After building the test binary, it runs `print_dwarf_diagnostics()`
2. Checks for project source paths in DWARF line tables using `llvm-dwarfdump --debug-line`
3. If no paths found, emits the warning and sets `dwarf_had_paths = False`

## Honest Signal Policy

When DWARF is incomplete (no source paths in line tables):

1. **kcov coverage numbers are marked untrustworthy**
2. **Test-as-signal becomes the honest fallback**: `make tovarisch-test` passing proves the code exercises the covered behaviors
3. **The gate does NOT silently use the kcov number** — it treats it as suspected-bad

## Resolution Options

### Option A: Accept macOS Coverage Limitation (Recommended)

On macOS, the gate uses test-as-signal as honest coverage when DWARF is incomplete:

- `make tovarisch-test` passing = honest coverage signal
- kcov numbers are reported but not used for threshold comparison
- The accepted risk is documented in `docs/coverage/tovarisch-coverage.md`

This preserves the Day-0 doctrine: coverage must remain honest test-as-signal coverage.

### Option B: Repair DWARF Instrumentation

If the root cause is a Zig build configuration issue:

- Investigate `-Doptimize=Debug` vs debug info flags for Zig 0.16 on macOS
- Verify `build.zig` test target configuration
- Requires Zig toolchain changes (may not be fixable locally)

### Option C: Lower Threshold (FORBIDDEN)

Do NOT silently lower `COVERAGE_THRESHOLD` to make the gate pass. This violates:

- Day-0 code coverage doctrine: "Coverage must remain honest test-as-signal coverage, not fake percentages"
- ACT acceptance criteria: "Do not silently lower COVERAGE_THRESHOLD"

## Platform Classification

| Platform | DWARF Completeness | Coverage Signal |
|----------|-------------------|-----------------|
| Linux (stable Zig) | Usually complete | kcov numbers trustworthy |
| macOS (Zig 0.16) | May be incomplete | Test-as-signal fallback |
| Linux CI | Variable | kcov or test-as-signal |

## References

- [Day-0 Code Coverage Doctrine](../doctrine/day-0-code-coverage.md)
- [Tovarisch Coverage Ledger](./tovarisch-coverage.md)
- [coverage_gate.py](../../scripts/coverage_gate.py)
