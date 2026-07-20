# ACT: KGB Artifact Writer Architecture Baseline - CORRECTION02 Requirements

**Status:** REQUIREMENTS RECORDED — IMPLEMENTATION PENDING

**Parent:** ACT-KGB-ARTIFACT-WRITER-ARCHITECTURE-BASELINE01

**Requires:** CORRECTION02

**Expert Review:** Go static-analysis and security-gate reviewer

## ACT Status

```
ACT-KGB-ARTIFACT-WRITER-ARCHITECTURE-BASELINE01-CORRECTION02:
REQUIREMENTS RECORDED — IMPLEMENTATION PENDING
```

## CORRECTION01 Status: PARTIAL

CORRECTION01 established:
- Real Go scanner exists
- Exact-ID comparator exists
- Hashes are structurally valid (87/87 valid SHA-256)
- Stale/unexpected comparisons fail appropriately

CORRECTION01 missing/incorrect:
- Position-independent identity (line numbers in hash)
- Type-resolved calls (TypesInfo not used)
- Sink provenance (fmt.Fprintf to os.Stderr flagged as bypass)
- Complete operation coverage (claimed but not implemented)
- Fail-closed scanning (errors logged but continue)
- Unknown-surface rejection
- Mutation tests
- Canonical gate wiring
- Patch hygiene

## Core Architectural Issues

### 1. Finding IDs Include Line Numbers

**Problem:** `fset.Position(pos).Line` makes hashes unstable to cosmetic changes.

**Required Fix:** Remove line numbers from hash entirely. Use only structural identity.

**Identity Schema:**

```json
{
  "schema_version": 2,
  "surface_id": "string",
  "module_path": "string",
  "package_path": "string",
  "repository_relative_file": "string",
  "enclosing_symbol": "string",
  "canonical_callee_package": "string",
  "canonical_callee_name": "string",
  "normalized_destination_ast": "string",
  "normalized_containing_statement_ast": "string",
  "persistence_mode": "string",
  "line": "integer (diagnostic metadata only)",
  "column": "integer (diagnostic metadata only)"
}
```

**Note:** Do NOT use `filepath.Base(file)` - it collides for common names like `cmd/foo/main.go` vs `cmd/bar/main.go`. Use the repository-relative path.

### 2. Type Resolution Not Used

**Problem:** `TypesInfo` is loaded but never consulted. Callee identity inferred from identifier spelling.

**Required Fix:** Use `types.Info` to resolve:
- Package path (not identifier name)
- Object identity (not selector spelling)
- Import aliases resolved to canonical path

**Correct Import Alias Resolution:**

```go
func importedPackagePath(info *types.Info, ident *ast.Ident) (string, bool) {
    obj := info.Uses[ident]

    pkgName, ok := obj.(*types.PkgName)
    if !ok || pkgName.Imported() == nil {
        return "", false
    }

    return pkgName.Imported().Path(), true
}
```

**Note:** `types.Info.Uses` maps identifiers to the objects they denote. For package identifiers in qualified expressions, this resolves to `*types.PkgName`, whose `Imported()` method returns the target package.

### 3. fmt.Fprintf to os.Stderr/Stdout Flagged

**Problem:** All `fmt.Fprintf` calls are flagged without checking destination.

**Required Fix:** Bounded provenance analysis with explicit classification:

```text
resolved os.Stdout/os.Stderr            ignore
resolved HTTP response writer           ignore
direct *os.File from persistence API   detect
local variable traced to *os.File      detect
unknown interface provenance            report explicitly as unsupported
```

**Provenance Classification:**

1. Check first argument type via `types.Info.Types[arg].Type`
2. Walk assignment chain to trace variables
3. Classify explicitly - never silently approve unknown sinks

### 4. Incomplete Operation Coverage

**Problem:** Header claims detection of:
- os.Create + Write
- os.OpenFile + Write
- os.CreateTemp
- (*os.File).Write
- io.Copy

**Baseline contains only:**
- os.WriteFile: 46
- fmt.Fprintf: 41

**Required:** Either implement full coverage OR emit honest coverage matrix:

```json
{
  "coverage": {
    "os.WriteFile": "supported",
    "ioutil.WriteFile": "supported",
    "os.Create": "unsupported",
    "os.OpenFile": "unsupported",
    "os.CreateTemp": "unsupported",
    "(*os.File).Write": "unsupported",
    "io.Copy": "unsupported",
    "fmt.Fprintf": "partial (requires destination analysis)"
  }
}
```

### 5. Fail-Open Error Handling

**Problem:** Parse/scan errors logged but continue.

**Required:** Fail-closed on:
- Parse errors
- Package-specific errors (pkg.Errors)
- Walk/read errors
- Empty package expansion where files expected
- Ill-typed packages
- Missing scan roots

### 6. Unknown Surfaces Accepted

**Problem:** `DetectSurfaceID` returns "unknown-surface" which is accepted.

**Required:**
```
unbound_findings > 0 → FAIL
unknown surface → FAIL
catalog load error → FAIL
writer file absent → FAIL
```

### 7. Verifier Wired to Existing Gate

**Problem:** No Makefile target, hard-coded path.

**Required:** Replace or extend the existing `hulk-uvb76-artifact-producer-gate` target so it invokes the Go scanner and exact-ID verifier. Do not add a second identically purposed gate.

Resolve repository root via `git rev-parse --show-toplevel` or accept as explicit flag.

### 8. AST Normalization Pinned

**Required:** Record Go version in baseline:

```json
{
  "generator": {
    "name": "artifact-writer-scanner",
    "schema_version": 2,
    "go_version": "go1.25.12"
  }
}
```

Use repository-pinned Go version for baseline generation and verification.

## CORRECTION02 Acceptance Checkpoints

### Identity Stability Tests (Expected after CORRECTION02)
- Line/comment movement → same finding_id
- Import alias change → same finding_id

### Type Resolution Tests (Expected after CORRECTION02)
- Local variable named `os` → NOT flagged as `os.WriteFile`
- Alias import of os → correctly identified

### Sink Provenance Tests (Expected after CORRECTION02)
- `fmt.Fprintf(os.Stderr, ...)` → ignored
- `fmt.Fprintf(os.Stdout, ...)` → ignored
- `fmt.Fprintf(file-backed sink, ...)` → detected
- Unknown sink → reported explicitly as unsupported

### Mutation Tests (Expected after CORRECTION02)
- New call in same legacy file → unexpected=1, FAIL
- Changed destination → stale=1, unexpected=1, FAIL
- Removed call → stale=1, FAIL
- Unknown surface → unbound=1, FAIL
- Parse/package error → FAIL

### Repeatability (Expected after CORRECTION02)
- Scanner output repeated twice → byte-identical

### CI/CD Integration (Expected after CORRECTION02)
```
go test ./cmd/artifact-writer-scanner/... -count=1  → PASS
go test -race ./cmd/artifact-writer-scanner/...      → PASS
go test ./cmd/ratchet-verifier/... -count=1          → PASS
go test -race ./cmd/ratchet-verifier/...            → PASS
make hulk-uvb76-artifact-producer-gate               → PASS
git diff --check                                     → PASS
```

## Required Endpoint

```
new bypass in legacy file       → FAIL unexpected=1
cosmetic source movement        → PASS same finding_id
aliased os.WriteFile            → detected
shadowed local os.WriteFile     → ignored
fmt.Fprintf(os.Stderr, ...)    → ignored
file-backed fmt.Fprintf         → detected
unknown sink                    → fail/report explicitly
unknown surface                 → FAIL unbound=1
parse/type/package error        → FAIL
stale baseline                  → FAIL
repeated scanner output         → byte-identical
git diff --check                → PASS
artifact producer gate          → PASS baseline-equivalent
```

## Implementation Order

Proceed with these commits:

1. `docs`: correct CORRECTION02 requirements and patch hygiene
2. `test`: add identity, type-resolution, provenance and ratchet mutations
3. `feat`: implement position-independent typed scanner
4. `feat`: implement fail-closed catalog-bound verifier
5. `build`: wire verifier into existing artifact-producer gate
6. `chore`: regenerate authoritative baseline
7. `docs`: close CORRECTION02 with fresh evidence

The implementation should be test-first. The baseline must be regenerated only after the scanner's identity, operation coverage and false-positive behavior are fixed.

## Blocker

**Do not start ACT-UVB76-RETAINED-ARTIFACT-MIGRATION-WAVE01 until CORRECTION02 is green.**
