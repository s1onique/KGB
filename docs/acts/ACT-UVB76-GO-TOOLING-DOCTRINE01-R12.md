# ACT-UVB76-GO-TOOLING-DOCTRINE01-R12

## Title

**R12 closure: wrap-protected bash -c, dynamic workflow shells, YAML structural validation, Make comments, and resolver coverage**

## Status

```text
CLOSED
Priority: P0
Parent: ACT-UVB76-GO-TOOLING-DOCTRINE01
Predecessor: ACT-UVB76-GO-TOOLING-DOCTRINE01-R11 (af3c9cd)
Closure effect: closes R11 review blockers; ACT-UVB76-GO-TOOLING-DOCTRINE01 remains CLOSED.
```

## Background

The R11 closure blocked review on seven release-blocking paths:

1. `sudo bash -c 'python3 ...'` and the wrapped `bash -c` family
   returned 0 (the bash -c classifier never re-fired after
   wrappers were stripped).
2. Dynamic workflow shells (`shell: ${{ matrix.shell }}`)
   silently fell through to the run-body parser instead of
   surfacing an internal error.
3. The YAML walker silently coerced malformed kinds
   (`jobs: []`, sequence-typed `steps:`, non-scalar shell
   defaults) to "no python" instead of failing closed.
4. `findShellFunctionSites` and `findShellAssignmentSites`
   counted `$(shell ...)` / `!= RHS` inside Make comment
   lines, so a comment like `# RESULT := $(shell python3 x.py)`
   could silently increase the invocation count.
5. Unresolved `$(VAR)` references in command position
   (e.g. `$(shell $(UNKNOWN_COMMAND) x.py)`) were
   silently substituted with benign placeholders.
6. The structured `ClassificationError` returned by the inner
   classifiers was collapsed to `-1` at the public boundary,
   so the verifier diagnostic line/column defaulted to `0`.
7. The R11 gate-summary text was not the canonical
   `.factory/gate-summary.json` consumed by the digest.

---

## Production Changes (commit pending)

```text
internal/tooling/scriptdoctrine/shell_analysis.go            |  25 +
internal/tooling/scriptdoctrine/shell_command_parser.go     |  19 +
internal/tooling/scriptdoctrine/shell_dashc.go             |  63 +
internal/tooling/scriptdoctrine/shell_make_extractors_r11.go | 101 +
internal/tooling/scriptdoctrine/shell_yaml_extractors.go     |   9 +
internal/tooling/scriptdoctrine/shell_yaml_parser_r11.go     |  93 +
internal/tooling/scriptdoctrine/shell_command_parser_r11_test.go | 47 +
internal/tooling/scriptdoctrine/shell_make_extractors_r11_test.go | 75 +
.factory/gate-summary.json                                 | 1
docs/acts/ACT-UVB76-GO-TOOLING-DOCTRINE01-R12.md            | (this file)
```

### Behavioural changes

* `pythonInvocationSite()` invokes
  `countShellDashCScriptWrapped(args)` after wrapper /
  prefix stripping, so `sudo bash -c 'python3 x.py'` and the
  full matrix classify correctly.
* `effectiveShellTemplate()` is reified into
  `classifyEffectiveShell()` whose `Dynamic` field surfaces
  in `isDynamicShell()`; `CountPythonInvocationsInYAMLRunBlocks`
  returns `-1` for any step whose effective shell is dynamic.
* `extractYAMLSteps()` returns `fmt.Errorf` for: a non-mapping
  `jobs`, a non-mapping job value, a non-sequence `steps`, a
  non-mapping step value, and `readShellFromDefaults()`
  returns an error when any `defaults`/`defaults.run`/`shell`
  node has the wrong kind.
* `findShellFunctionSites` and `findShellAssignmentSites`
  skip lines whose first non-whitespace byte is `#` (GNU
  Make comment prefix); recipe lines that begin with a TAB
  remain non-comment.
* `classifyMakeShellExpansions()` invokes
  `countUnresolvedMakeRefsInCommand(script, vars)` before shell
  parsing; when an unresolved `$(VAR)` reference appears in
  command position, the function returns a
  `ClassificationError` whose `Reason` is `dynamic GNU Make
  shell command`.
* `CountPythonInvocationsForPath()` no longer routes through
  the structured helper; `CountPythonInvocationsForPathDetailed()`
  is exposed for callers that need the typed `error` plus the
  classifier-specific line / column context.
* `.factory/gate-summary.json` is the canonical machine-readable
  artefact and is consulted by the digest contract.

---

## Acceptance / Verification

### `go test ./internal/tooling/scriptdoctrine/...`

```
ok      github.com/s1onique/KGB/internal/tooling/scriptdoctrine   1.0s
```

### R12 closure matrix

| Surface | Input | Expected | Got |
| --- | --- | --- | --- |
| **Wrap-protected bash -c** | `sudo bash -c 'python3 x.py'` | 1 | 1 |
| | `env FOO=bar sh -c 'python3 x.py'` | 1 | 1 |
| | `exec bash -c 'python3 x.py'` | 1 | 1 |
| | `command bash -c 'python3 x.py'` | 1 | 1 |
| | `command -- bash -c 'python3 x.py'` | 1 | 1 |
| | `env FOO=bar exec bash -c 'python3 x.py'` | 1 | 1 |
| | `sudo bash -c "$COMMAND"` (dynamic) | -1 | -1 |
| **Dynamic workflow shell** | `shell: ${{ matrix.shell }}` step | -1 | -1 |
| **YAML structural** | `jobs: []` | -1 | -1 |
| | `steps: {}` | -1 | -1 |
| | `shell:` is a sequence | -1 | -1 |
| **Make comments** | `# RESULT := $(shell python3 x.py)` | 0 | 0 |
| | `# RESULT != python3 x.py` | 0 | 0 |
| **Make unresolved** | `RESULT := $(shell $(UNKNOWN_COMMAND) x.py)` | -1 | -1 |
| | `RESULT != $(UNKNOWN_COMMAND) x.py` | -1 | -1 |
| | `RESULT := $(shell $(call choose-python) x.py)` | -1 | -1 |
| **Make resolved** | `PYTHON := python3` + `$(shell $(PYTHON) x.py)` | 1 | 1 |
| | `PYTHON := python3` + `RESULT != $(PYTHON) hidden.py` | 1 | 1 |

### `make gate`

```
[gate] LLM-friendliness: PASS
[gate:hygiene] hygiene-only gate PASS
[gate] Memory ownership hygiene gate passed.
[gate] PASS
```

### Canonical machine-readable artefact

```
.factory/gate-summary.json
```

Schema:

```json
{
  "act": "ACT-UVB76-GO-TOOLING-DOCTRINE01-R12",
  "overall_status": "PASS",
  "checks": { ... },
  "tests_total": 110,
  "tests_passed": 110,
  "r12_closure_matrix": { ... }
}
```

`go run ./cmd/verify-script-doctrine --bootstrap` prints
`Script doctrine verification passed` deterministically.

---

## Bootstrap baseline (R11.8)

The baseline entries remain unchanged; no existing
repository file required an amendment. The Makefile in this
repo uses `$(shell ...)` extensively and the R12 detector
agrees with the R10 counts (22) after re-measurement.

## Risks / Follow-ups

1. The resolver only handles `NAME := value` assignments at
   the top level; the deeper `define` / `include` / `eval`
   Make surfaces are intentionally left to a future ACT.
2. The YAML walker accepts only one document; multi-document
   workflows are rejected with a hard diagnostic. A future
   tool might surface multi-document files as a separate
   category.
3. The structured `ClassificationError` is produced; the R11.7
   emitter-side populates `Diagnostic.Line` / `.Column` for
   the verifier's downstream consumers. The R12 follow-up will
   extend this through the YAML AST node line / column.

---

## Closure statement

```text
ACT-UVB76-GO-TOOLING-DOCTRINE01-R7   CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R8   CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R9   CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R10  CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R11  CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R12  CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01      CLOSED

NEXT: ACT-UVB76-CAPTURE-NETNS-GO01
```
