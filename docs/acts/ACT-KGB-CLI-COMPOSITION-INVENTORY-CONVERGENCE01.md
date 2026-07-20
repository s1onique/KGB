# ACT-KGB-CLI-COMPOSITION-INVENTORY-CONVERGENCE01

## Status: COMPLETE

## Objective

Register every currently unclassified `os/exec` boundary reported by the canonical CLI composition inventory verifier and restore the hygiene gate without adding exclusions, suppressions, broad allowlists, or detector exceptions.

## Reported Missing Surfaces

The hygiene-gate reported four legitimate Go process-composition files missing from `docs/tooling/cli-composition-inventory.csv`:

1. `uvb76/cmd/uvb76-makefile-composition-check/main.go`
   - `os/exec import` at line 30
   - `exec.Command()` at line 325

2. `cmd/ratchet-verifier/main.go`
   - `os/exec import` at line 16
   - `exec.Command()` at line 70

3. `internal/tooling/allocationtrackerimports/selftest.go`
   - `os/exec import` at line 6
   - `exec.Command()` at lines 340, 364

4. `internal/tooling/allocationtrackerimports/scanner.go`
   - `os/exec import` at line 8
   - `exec.Command()` at lines 47, 79

## Investigation

### CSV Format Analysis

The canonical inventory uses `(path, pattern)` as the identity key. Multiple `exec.Command()` calls within the same file share a single inventory entry per pattern.

- **Column count**: 15 columns (confirmed from header: id, path, language, pattern, owner_area, runtime_classification, frequency, allowed, justification, timeout_bounded, output_bounded, redaction_required, replacement_candidate, status, notes)
- **Highest existing identifier**: CLI-0061

### Source Analysis

#### uvb76/cmd/uvb76-makefile-composition-check/main.go
- **Executable**: `make` (hardcoded)
- **argv construction**: Static `["make", "-n", "-f", "Makefile", target]`
- **User input**: Target name from function parameter (not user-controlled CLI input)
- **Shell**: No shell invoked (Go's os/exec does not implicitly invoke shell)
- **Timeout**: None explicit (dry-run mode only)
- **Output bound**: No explicit limit
- **Classification**: `tooling` / `diagnostic_runtime` / `rare`
- **Usage**: Verifies artifact gate ordering in Makefile

#### cmd/ratchet-verifier/main.go
- **Executable**: Path from `--scanner` flag (validated as file)
- **argv construction**: `[*scannerBin, "--format=findings"]`
- **User input**: Scanner path from CLI flag
- **Shell**: No shell invoked
- **Timeout**: None explicit
- **Output bound**: No explicit limit
- **Classification**: `tooling` / `diagnostic_runtime` / `rare`
- **Usage**: Invokes artifact-writer-scanner for baseline comparison

#### internal/tooling/allocationtrackerimports/selftest.go
- **Executables**: `git` and `zig` (from PATH)
- **argv construction**:
  - Line 340: `["git", args...]` where args are controlled
  - Line 364: `["zig", "test", importer, ...]` static except importer path
- **User input**: None
- **Shell**: No shell invoked
- **Timeout**: None explicit
- **Output bound**: No explicit limit (zig test output unbounded)
- **Classification**: `tooling` / `build_test` / `once`
- **Usage**: Self-test only (runs during test execution)

#### internal/tooling/allocationtrackerimports/scanner.go
- **Executable**: `git` (from PATH)
- **argv construction**:
  - Line 47: `["git", "rev-parse", "--show-toplevel"]` static
  - Line 79: `["git", "ls-files", "-z", ...userArgs]` includes user-supplied args
- **User input**: Arguments passed to gitList function
- **Shell**: No shell invoked
- **Timeout**: None explicit
- **Output bound**: No explicit limit (git ls-files output proportional to repo size)
- **Classification**: `tooling` / `diagnostic_runtime` / `rare`
- **Usage**: Repository root detection and file listing

## Changes Made

Added 8 new rows to `docs/tooling/cli-composition-inventory.csv`:

| ID | Path | Pattern | Classification | timeout_bounded | output_bounded |
|----|------|---------|----------------|-----------------|----------------|
| CLI-0062 | uvb76/cmd/uvb76-makefile-composition-check/main.go | os/exec import | tooling/diagnostic_runtime/rare | no | no |
| CLI-0063 | uvb76/cmd/uvb76-makefile-composition-check/main.go | exec.Command() | tooling/diagnostic_runtime/rare | no | no |
| CLI-0064 | cmd/ratchet-verifier/main.go | os/exec import | tooling/diagnostic_runtime/rare | no | no |
| CLI-0065 | cmd/ratchet-verifier/main.go | exec.Command() | tooling/diagnostic_runtime/rare | no | no |
| CLI-0066 | internal/tooling/allocationtrackerimports/selftest.go | os/exec import | tooling/build_test/once | no | no |
| CLI-0067 | internal/tooling/allocationtrackerimports/selftest.go | exec.Command() | tooling/build_test/once | no | no |
| CLI-0068 | internal/tooling/allocationtrackerimports/scanner.go | os/exec import | tooling/diagnostic_runtime/rare | no | no |
| CLI-0069 | internal/tooling/allocationtrackerimports/scanner.go | exec.Command() | tooling/diagnostic_runtime/rare | no | no |

## Corrections

### Bounding Metadata Correction (Commit 840da53)

**Issue**: Initial rows set `timeout_bounded=yes` and `output_bounded=yes`, contradicting the source analysis which documented "Timeout: None explicit".

**Original values**: `timeout_bounded=yes, output_bounded=yes`

**Corrected values**: `timeout_bounded=no, output_bounded=no`

**Rationale**:
- Plain `exec.Command(...).Run()` waits for process completion without timeout
- `CombinedOutput()` collects output without byte limit
- `git ls-files -z` output is proportional to repository size
- `zig test` output is not inherently capped

## Verification

### Narrow Verifier
```
$ python3 scripts/verify_cli_composition_inventory.py
Loading inventory from /home/kgb/Projects/KGB/docs/tooling/cli-composition-inventory.csv
Loaded 65 inventory entries
Scanning codebase for CLI patterns...
Detected 682 CLI usage sites
=== VERIFICATION PASSED ===
```

### Git Diff Check
```
$ git diff --check
git diff --check passed
```

### Hygiene Gate
```
$ make hygiene-gate
[gate:hygiene] hygiene-only gate PASS
```

### Go Build Tests
```
$ cd cmd/ratchet-verifier && go build .
ratchet-verifier build OK
$ cd uvb76/cmd/uvb76-makefile-composition-check && go build .
makefile-composition-check build OK
```

### Negative Proof
Removed CLI-0062 (os/exec import for makefile-composition-check):
- With CLI-0062 present: 65 entries, VERIFICATION PASSED
- Without CLI-0062: 64 entries, VERIFICATION FAILED with error:
  ```
  DETECTED CLI usage at uvb76/cmd/uvb76-makefile-composition-check/main.go:30 pattern='os/exec import' but no inventory entry exists
  ```

Restored CLI-0062 and verified VERIFICATION PASSED with 65 entries.

Note: Only CLI-0062 was mutation-tested as a representative case. The verifier correctly detects missing `(path, pattern)` identities.

### Gate Status

**Pre-existing gate failure unrelated to this change:**

The `make gate` fails at `hulk-uvb76-artifact-producer-gate` due to artifact hygiene issues with file-write operations in uvb76 lab commands. These are pre-existing issues in:
- uvb76-latency-crash-lab
- uvb76-memleak-pprof-lab
- uvb76-memory-lab
- uvb76-targets-crash-lab
- uvb76-tcp-diag-telemetry-lab

The four files registered in this ACT are not mentioned in the artifact hygiene error output.

## Commits

1. **5615f7cf** - docs(tooling): register missing Go CLI composition boundaries
2. **840da53** - docs(tooling): correct CLI boundary bounding metadata

## Files Changed

- `docs/tooling/cli-composition-inventory.csv` (+8 rows, +1 correction)
- `docs/acts/ACT-KGB-CLI-COMPOSITION-INVENTORY-CONVERGENCE01.md` (new)

## Integrity Requirements

- [x] No exclusions added to scanning directories
- [x] No suppressions added to detector patterns
- [x] Verifier remains fail-closed
- [x] No shell claims made without code evidence
- [x] CSV parsing preserved (15 columns, LF line endings, unique IDs)
- [x] No unrelated source refactoring
- [x] Bounding metadata corrected to match source analysis
- [x] ACT document created

## Notes

- Go 1.25.12 is available at `/usr/local/go/bin/go`
- Go builds for affected commands pass
- The `make gate` failure is a pre-existing artifact hygiene issue, not related to this CLI composition inventory change
