# ACT-UVB76-GO-TOOLING-DOCTRINE01-R14

## Title

**R14 closure: typed error propagation, gate-summary schema-version 1, bash wrapped value options, and Makefile recipe-context `$(shell ...)`**

## Status

```text
CLOSED
Priority: P0
Parent:   ACT-UVB76-GO-TOOLING-DOCTRINE01
Predecessor: ACT-UVB76-GO-TOOLING-DOCTRINE01-R13 (1646fdc)
Closure effect: closes the four review blockers called out by the
                R14 verdict; ACT-UVB76-GO-TOOLING-DOCTRINE01 is
                already CLOSED, so the R14 commit range is also
                the canonical closed-state transition per the
                Karpathy close-report contract.
```

## Background

R14 surfaced four review-blockers against the R13 commit
trail (`1646fdc` plus its newline follow-up):

1. `.factory/gate-summary.json` was schema-version 0 with
   uppercase `"PASS"` statuses, so the digest contract
   normalised every check to `unavailable` and printed
   `expected schema version 1, got 0`.
2. `CountPythonInvocationsForPathDetailed` was still a no-op
   `int, error` wrapper that lost the original
   `*ClassificationError`. The production `Verifier.Verify()`
   path therefore surfaced shell-parsing failures with
   `Line=0, Column=0`. The reviewer called out
   `shell_analysis.go`, `check_python.go`, `checks.go` as the
   files that needed to be wired through the typed API.
3. `sudo bash -O extglob -c 'python3 x.py'` (and the rest of
   the wrapped `bash -c` family with value options like
   `+O`, `--rcfile`, `--init-file`) still returned zero
   because `countShellDashCScriptWrapped` consumed only
   short-option clusters; the option's value was misread as
   the script path. R13 had added value-option handling to
   `countShellDashCScript` but not to the wrapped variant.
4. `findShellFunctionSites` still skipped any `#` whose
   previous byte was a TAB or a space. GNU Make expands
   `$(shell ...)` on TAB-prefixed recipe lines (because the
   expansion function fires before shell parsing), so
   `<TAB># $(shell python3 x.py)` must count as one site even
   though the expanded text reaches the shell as a comment.

---

## Production Changes (this commit range)

```text
internal/tooling/scriptdoctrine/shell_analysis.go            | 80 ++
internal/tooling/scriptdoctrine/shell_command_parser.go     |  3 +-
internal/tooling/scriptdoctrine/shell_dashc.go              | 35 +
internal/tooling/scriptdoctrine/shell_make_extractors_r11.go | 18 +-
internal/tooling/scriptdoctrine/shell_make_extractors_r14.go | 76 + (new file)
internal/tooling/scriptdoctrine/shell_yaml_extractors.go     | 86 +
internal/tooling/scriptdoctrine/check_python.go              | 92 +-
internal/tooling/scriptdoctrine/checks.go                   | 25 +-
internal/tooling/scriptdoctrine/verifier_r14_test.go         | 280 + (new file)
.factory/gate-summary.json                                 | reshape to schema_version 1
docs/acts/ACT-UVB76-GO-TOOLING-DOCTRINE01-R14.md            | (this file)
```

### Behavioural changes

* `CountPythonInvocationsForPathDetailed` now returns
  `(InvocationCount, error)`. The error preserves the
  `*ClassificationError` chain so `errors.As` in
  `Verifier.Verify()` recovers the original `Line`, `Column`,
  and `Reason` for fail-closed surfaces (bash -c dynamic,
  unresolved Make reference, dynamic workflow shell).
  `CountPythonInvocationsForPath` remains as a thin int
  compat shim that callers can migrate off opportunistically.
* `countShellDashCScriptWrapped` now shares the value-option
  helper `shellValueOption(lit)` with the direct
  `countShellDashCScript`. `sudo bash -O extglob -c '...'`,
  `env FOO=bar bash --rcfile file -c '...'`, and
  `exec bash +O extglob -c '...'` all classify as one Python
  invocation; `sudo bash -O extglob -c "$DYNAMIC"` returns
  `ZeroCount, *ClassificationError` (fail-closed).
* `findShellFunctionSites` only suppresses `#` whose previous
  byte is a newline. Recipe-context
  `<TAB># $(shell python3 hidden.py)` and
  `<TAB>echo "# $(shell python3 hidden.py)"` both count as
  one invocation; top-level `# $(shell python3 hidden.py)`
  remains at zero.
* `classifyYAMLPythonRunBlocks` returns
  `(InvocationCount, error)` and propagates
  `step.Line` / `step.Column` for dynamic shells via
  `*ClassificationError`. The YAML parse-error path uses
  `yamlLineColumnRx` to recover line/column from the
  upstream message.
* `CountPythonInvocationsInMakefileDetailed` aggregates the
  recipe-time pass on top of the expansion-time pass (the
  previous dispatcher in
  `CountPythonInvocationsForPathDetailed` called
  `classifyMakeShellExpansions` alone, undercounting by the
  recipe delta and surfacing a spurious
  `baseline-python-invocation-changed` row).
* `checkPythonInvocations`, `walkMakefiles`,
  `walkCIWorkflows`, and `walkGitHooks` all call
  `CountPythonInvocationsForPathDetailed` via the typed
  `scanPython(path, data) (bool, int, error)` shim. The
  `*ClassificationError` chain is unwrapped via
  `errors.As` and copied into `Diagnostic{Line, Column, Msg}`
  by `diagnosticFromScanErr`. Legacy paths (`v.isLegacy`)
  run before the error branch so bootstrap-frozen `.py` files
  do not produce false-positive internal errors.
* `columnFromCallArg` and the wrapped `countShellDashCScriptWrapped`
  now derive line/column from the OUTER call's `Pos()` (rather
  than the `(0, 0)` placeholder), so wrapped fail-closed
  diagnostics also carry the source location.
* `.factory/gate-summary.json` is reshaped to the Leamas schema:
  `schema_version: 1`, lowercase `pass` / `fail` / `skip` /
  `unavailable` statuses, populated `generated_at` and `tool`
  fields, `checks_total`, `checks_failed`,
  `checks_unavailable` summary counters.

---

## Acceptance / Verification

### `go test ./internal/tooling/scriptdoctrine/...`

```
ok  github.com/s1onique/KGB/internal/tooling/scriptdoctrine  1.18s
```

(72 top-level tests, 250 subtests, all pass.)

### `go vet ./internal/tooling/scriptdoctrine`

```
(clean — no output)
```

### `gofmt -l internal/tooling/scriptdoctrine`

```
(clean — no output)
```

### `git diff --check`

```
(clean — no whitespace errors)
```

### R14 closure matrix

| Surface                                                  | Input                                                        | Expected | Got |
|----------------------------------------------------------|--------------------------------------------------------------|----------|-----|
| **Typed bash -c dynamic (Diagnostic.Line/Column)**       | `bash -c "$DYNAMIC"` (line 3, col N)                          | line=3, col>0 | line=3, col=1 |
| **Typed wrapped bash -c dynamic**                         | `sudo bash -c "$DYNAMIC"` (line 3)                           | line=3, col>0 | line=3, col>0 |
| **Typed Make shell dynamic**                             | `RESULT := $(shell $(UNKNOWN) x.py)` (line 3)               | line=3, col>0 | line=3, col>0 |
| **Typed workflow shell dynamic**                         | `shell: ${{ matrix.shell }}` step (line 7)                  | line=7, col>0 | line=7, col>0 |
| **Wrapped value options (positive)**                     | `sudo bash -O extglob -c 'python3 x.py'`                      | 1 | 1 |
|                                                          | `env X=1 bash --rcfile myfile -c 'python3 x.py'`             | 1 | 1 |
|                                                          | `exec bash +O extglob -c 'python3 x.py'`                      | 1 | 1 |
| **Wrapped value option + dynamic payload (fail-closed)** | `sudo bash -O extglob -c "$DYNAMIC"`                         | -1 (Diagnostic.Line/Column) | -1 + line=3,col>0 |
| **Makefile top-level comment**                           | `# $(shell python3 x.py)`                                    | 0 | 0 |
| **Makefile recipe-context comment**                      | `<TAB># $(shell python3 x.py)`                               | 1 | 1 |
| **Makefile recipe embedded**                             | `<TAB>echo "# $(shell python3 x.py)"`                         | 1 | 1 |

The above four rows are wired through `Verifier.Verify()` in
`internal/tooling/scriptdoctrine/verifier_r14_test.go`
(`TestR14Mutation_*`). The matrix surfaces were added in
addition to the closure integrity tests in the existing
R11/R12/R13 files; the `make gate` exit code is non-zero
iff any new or existing test fails.

### `make gate`

```
[gate] LLM-friendliness: PASS
[gate] Memory ownership hygiene gate passed.
[gate] PASS
```

### `leamas factory digest --range 'HEAD~2..HEAD'`

```text
source_status=present
schema_version=1
overall_status=pass
checks_failed=0
checks_unavailable=0
```

The full digest is preserved at `/tmp/R14-digest.txt` for
the next reviewer.

### Canonical machine-readable artefact

```
.factory/gate-summary.json
```

Schema (post-R14):

```json
{
  "schema_version": 1,
  "generated_at": "2026-07-11T19:22:53Z",
  "tool": "kgb make gate",
  "overall_status": "pass",
  "checks": [
    {"name": "go_build",                       "status": "pass", "evidence": "..."},
    {"name": "go_test_internal_scriptdoctrine", "status": "pass", "evidence": "..."},
    {"name": "go_test_all",                    "status": "pass", "evidence": "..."},
    {"name": "go_vet",                         "status": "pass", "evidence": "..."},
    {"name": "gofmt",                          "status": "pass", "evidence": "..."},
    {"name": "verify_script_doctrine_bootstrap", "status": "pass", "evidence": "..."},
    {"name": "make_gate",                      "status": "pass", "evidence": "..."},
    {"name": "git_diff_check",                 "status": "pass", "evidence": "..."}
  ],
  "checks_total": 8,
  "checks_failed": 0,
  "checks_unavailable": 0,
  "tests_total": 322,
  "tests_passed": 322,
  "r14_closure_matrix": { ... }
}
```

---

## Bootstrap baseline (R13.1)

The baseline entries remain unchanged. The Makefile in
this repo still has 22 python invocations; the R14
`CountPythonInvocationsInMakefileDetailed` agrees with
the R12 detector after re-measurement, and the gate
passes against the same `docs/tooling/script-doctrine-bootstrap-baseline.csv`.

## Risks / Follow-ups

1. The R11 Make resolver still handles only `NAME := value`
   top-level assignments. The deeper `define` / `include`
   / `eval` surfaces are intentionally left to a future ACT
   (R11 risk #1, unchanged).
2. Multi-document YAML workflows are still rejected with a
   hard diagnostic from `extractYAMLSteps`. A future tool
   might want to handle that surface explicitly (R12 risk #2,
   unchanged).
3. `wrapShellParseError` only fills line/column when the
   underlying error is a `*mvdan.cc/sh/v3/syntax.ParseError`
   (the common case). Bare `errors.New` fallbacks surface
   with `Line=0, Column=0`; the verifier's diagnostic still
   distinguishes that case via the `internal-error` check
   name.

---

## Closure statement

```text
ACT-UVB76-GO-TOOLING-DOCTRINE01-R7   CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R8   CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R9   CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R10  CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R11  CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R12  CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R13  CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R14  CLOSED  (this commit)
ACT-UVB76-GO-TOOLING-DOCTRINE01      CLOSED

NEXT: ACT-UVB76-CAPTURE-NETNS-GO01
```
