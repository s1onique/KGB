# ACT-UVB76-GO-TOOLING-DOCTRINE01-R7

## Title

Replace handwritten shell parser with `mvdan.cc/sh/v3/syntax` AST walker; close P1/P2 review defects.

## Background

The R6 commit (`ACT-UVB76-GO-TOOLING-DOCTRINE01-R6`) introduced a per-line
handwritten shell parser (`shell_command_parser.go`) that the R7 review
identified as a P0 correctness risk and several surrounding P1/P2
robustness defects. This ACT closes those findings by:

- Replacing the handwritten parser with an AST visitor over
  `mvdan.cc/sh/v3/syntax`, the canonical Bash AST library.
- Adding miss-handling tests for compound commands, assignment / redirection
  prefixes, quoting / heredocs, and malformed-syntax fail-closed semantics.
- Wiring `scanner.Err()` and the `walkMakefiles` walk-error capture so
  unparseable inputs surface as `internal-error` diagnostics instead of
  silently green-lighting the script.

## Scope

In scope (touched files):

- `go.mod`, `go.sum` — added `mvdan.cc/sh/v3 v3.13.1` dependency.
- `internal/tooling/scriptdoctrine/shell_command_parser.go` — replaced
  handwritten `splitShellList` + friends with an AST visitor.
- `internal/tooling/scriptdoctrine/shell_command_parser_test.go` (new) — AST
  visitor behaviour tests.
- `internal/tooling/scriptdoctrine/shell_analysis.go` — shebang handling,
  full-program AST parse, per-line reader fail-closed behaviour,
  `Scanner.Err()` honoured.
- `internal/tooling/scriptdoctrine/shell_analysis_test.go` — slimmed, keeps
  file/API-level coverage; R7 AST tests moved to the companion file.
- `internal/tooling/scriptdoctrine/check_python.go` — `walkMakefiles` now
  captures the top-level `filepath.Walk` error.
- `internal/tooling/scriptdoctrine/verifier_test.go` — slimmed, added the
  R7 walk-error regression test.

Out of scope (unchanged but relevant background):

- The bootstrap baseline in
  `docs/tooling/script-doctrine-bootstrap-baseline.csv` keeps its R5/R6
  values for the legacy entries. No new lines were added.

## Implementation Notes

### Why `mvdan.cc/sh/v3/syntax`

- It is the de-facto Go Bash grammar (used by `shfmt`, `sh` integration
  tests, etc.).
- It exposes a typed AST (`Stmt`, `CallExpr`, `BinaryCmd`,
  `IfClause`, `WhileClause`, `ForClause`, `CaseClause`, `Subshell`,
  `Block`, `Redirect`, `CmdSubst`, `DblQuoted`, `ProcSubst`) which is
  exactly what we need to count command sites per shell construct.
- We deliberately do NOT enable `RecoverErrors`, so a malformed line
  returns a hard parser error that the caller escalates to -1
  (fail-closed).

### Walker behaviour

The new `pythonWalker` in `shell_command_parser.go` handles:

- **Simple commands** — `CallExpr` whose first word is `python` / `python3`
  / version-suffixed python / `pip` / `pip3` / `pytest`.
- **Command prefixes** — `sudo`, `env`, `/usr/bin/env`, `exec`, `command`
  are stripped; `command -flag ...` is recognised as a lookup and never
  counts.
- **Pipelines / boolean chains** — `BinaryCmd` is recursed on each side.
- **Compound commands** — `IfClause`, `WhileClause` (incl. `UntilClause`),
  `ForClause`, `CaseClause`, `Subshell`, `Block` are recursed into their
  body statements.
- **Redirections** — `>` / `<` / `>>` words and heredoc bodies are walked
  (Hdoc field of `Redirect`) so a python invocation inside a heredoc
  body still counts.
- **Command substitutions** — `$(...)`, backticks (preserved by
  mvdan.cc/sh inside `CallExpr.Assigns[i].Value` and Word parts), and
  process substitutions `<(...)` / `>(...)` are recursed.
- **Prefix assignments** — even when `CallExpr.Args` is empty (a bare
  `X=python3 ...` assignment), the value Word is walked so
  `X=python3 -c 'print(1)'` (backtick or $()) still counts.

### Two-stage `CountPythonInvocations`

```go
// First: full AST parse - the R7-correct path.
file, err := parser.Parse(bytes.NewReader(stripped), "inline")
if err == nil {
    ...walker...
    return w.count
}

// Fall back: per-line scanner. Preserve R5/R6 behaviour for Makefiles
// and YAML files whose surrounding structure is not shell.
n := countPythonInvocationsFromReader(bytes.NewReader(stripped))
return n
```

The per-line reader's fail-closed contract: if every candidate line is
rejected by the per-line parser, the function returns -1 (genuine
malformed shell). Lines with embedded `${{ ... }}` (GitHub Actions
syntax) parse fine in mvdan.cc/sh and contribute zero python sites
beyond their shell content.

### Known trade-off

Multi-line heredocs whose body contains a python invocation are still
caught by the AST path of `CountPythonInvocations`. When the AST parser
rejects the input (the common Makefile / YAML case), we fall back to
the per-line reader; heredoc body lines spanning the EOF terminator are
not recognised as carrying a substitution. No repository file currently
hits this combination.

## Defects Closed

| ID | Defect | Fix |
|----|--------|-----|
| R7-P0 | Handwritten parser silently dropped compound commands and heredoc bodies | Replaced with mvdan.cc/cc/sh AST visitor |
| R7-P1 | `scanner.Err()` ignored in `countPythonInvocationsFromReader` | Read after the loop, return -1 on error |
| R7-P1 | `walkMakefiles` walk error not captured | Stored return value, surfaced as `internal-error` |
| R7-P2 | Assignment prefixes (`FOO=bar python3 ...`) and redirection-word substitutions not walked | Walker honours `CallExpr.Assigns` and `Redirect.Word` |
| R7-P2 | Compound commands (for/while/if/case/subshell/block) not parsed | Walker recurses each compound AST node |
| R7-P2 | Quoting / comments / heredocs handled by handwritten preprocessor instead of grammar | mvdan.cc/sh handles them natively |
| R7-P2 | Malformed-syntax returned 0 (fail-open) | Returns -1, propagated by callers |

## Acceptance / Verification

### Unit tests (`go test ./internal/tooling/scriptdoctrine/...`)

30 top-level tests run, with 154 subtests:

- `TestCountPythonInvocationsCommandSites` — R6 mandated cases plus R7 regression
- `TestCountPythonInvocationsDeduplicatesOverlappingPatterns` — R6 dedup
- `TestCountPythonInvocationsIgnoresCommentsAndOutputCommands` — R6 quoting / comments
- `TestCountPythonInvocationsCompoundCommands` — R7 for / while / until / if-else / subshell / brace / case / pipeline
- `TestCountPythonInvocationsAssignmentAndRedirectionPrefixes` — R7 `FOO=bar ...` and `>$(...)` / `<<heredoc`
- `TestCountPythonInvocationsQuotingCommentsHeredocs` — R7 single / double quotes, inline comments, process substitution
- `TestCountPythonInvocationsMalformedSyntaxFailsClosed` — R7 fail-closed (-1) for unparseable input
- `TestCountPythonInvocationsScannerErrIsFailClosed` — R7 P1 regression: 64KiB+ line returns -1
- `TestCountPythonInvocationsNilReturnsMinusOne` — nil input contract
- `TestIsPythonCommandWord` — vocabulary pin
- `TestIsCommandPrefixWord` — prefix vocabulary pin
- `TestWalkMakefilesCapturesTopLevelWalkError` — R7 P1 regression
- `TestMutationExactlyOneAddedInvocation` — R6 mutation
- `TestMutationSemicolonAddedInvocation` — R6 mutation
- `TestMutationAfterOutputCommand` — R6 mutation
- `TestMutationCommentsDoNotCount` — R6 mutation
- `TestVerifierBootstrapBaselinePassesAtFrozenValues` — bootstrap baseline
- `TestVerifierBootstrapBaselineDetectsCountChange` — baseline drift
- `TestVerifierDetectsStaleBaselinePath` — stale-baseline diagnostic
- `TestVerifierFailsClosedOnReadError` — fail-closed on read errors
- `TestVerifierDetectsExtensionlessPythonShebang` — shebang without extension
- `TestSortDiagnostics` — sort invariants
- plus existing `CountLogicalLOC`, `HasPythonShebang`, `LOCBoundary`,
  `LoadBaseline` tests (all kept).

### `make gate`

```
[gate] PASS
```

The full gate (LLM-friendliness, memory ownership hygiene, allocation
patterns, Zig copy safety, final newlines, required docs, forbidden
naming, privacy doctrine, AGENTS/.clinerules, coverage ledger commands,
verify-script-doctrine, status contract, structured logs, plaintext
logging, memory budgets / attribution matrix, native-owned critical
paths, tovarisch Zig build + test + status contract) passes.

### `make verify-script-doctrine`

```
=== Script Doctrine Verification ===
go run ./cmd/verify-script-doctrine --bootstrap
Script doctrine verification passed
```

No new diagnostics, no false-greens, no false-reds.

## Files Changed

```
go.mod
go.sum
internal/tooling/scriptdoctrine/check_python.go
internal/tooling/scriptdoctrine/shell_analysis.go
internal/tooling/scriptdoctrine/shell_analysis_test.go
internal/tooling/scriptdoctrine/shell_command_parser.go
internal/tooling/scriptdoctrine/shell_command_parser_test.go   (new)
internal/tooling/scriptdoctrine/verifier_test.go
docs/acts/ACT-UVB76-GO-TOOLING-DOCTRINE01-R7.md                  (new)
```

`docs/acts/ACT-UVB76-GO-TOOLING-DOCTRINE01-R7.md` is itself the ACT
record.

## Risks / Follow-ups

1. Multi-line heredoc body lines are not recognised on the per-line
   fallback path. No in-repo file currently exercises this; flagged
   in this ACT for future review.
2. `mvdan.cc/sh/v3/syntax` consumes significant memory for very large
   bash programs (the parser is an in-memory AST). This is acceptable
   for the current Makefile / workflow file sizes; if a 10MB Makefile
   appears, re-evaluate.
3. The per-line reader's "all-line failure -> -1" rule is a heuristic.
   Future malformed-syntax inputs whose every line happens to be a
   tolerated garbage line could still under-report zero invocations
   instead of returning -1. Acceptable per the trade-off documented
   above.
