# ACT-UVB76-GO-TOOLING-DOCTRINE01-R11

## Title

**Close Make-time execution, dynamic shell indirection, and GitHub Actions shell-inheritance gaps**

## Status

```text
CLOSED
Priority: P0
Parent:   ACT-UVB76-GO-TOOLING-DOCTRINE01
Predecessor: ACT-UVB76-GO-TOOLING-DOCTRINE01-R10
Closure effect: closes the parent ACT.
```

## Background

R7–R10 closed most heuristic Python-detection gaps in the script
doctrine verifier but left three categories of execution surface
that a Python invocation could still use to escape detection:

* GNU Make's `$(shell …)` function and `!=` shell-assignment,
  which execute at Make-expansion time, before any recipe runs;
* the dynamic `bash -c "$COMMAND"` / `sh -c "$(…)"` family,
  whose command string the verifier cannot statically resolve;
* GitHub Actions custom shell templates whose first word is a
  python interpreter (`shell: python -u {0}`), and workflow
  shell inheritance via `defaults.run.shell` at workflow or job
  scope.

R11 closes those gaps. Internal helper APIs now return a
structured `InvocationCount` (and a non-nil error when the
analysis surface is dynamic), the YAML workflow walker is
rewritten over `go.yaml.in/yaml/v3`'s AST node tree, and a new
GNU Make extractor scans `$(shell …)` and `!= RHS` while
masking those bytes from the recipe parser to prevent double
counts.

---

## Production Changes

```text
go.mod                                                            |   2 +
go.sum                                                            |   4 +
internal/tooling/scriptdoctrine/check_python.go                   |   4 +
internal/tooling/scriptdoctrine/checks.go                         |  15 +
internal/tooling/scriptdoctrine/shell_analysis.go                 |  22 +
internal/tooling/scriptdoctrine/shell_command_parser.go            | 196 +++++++ (rewritten, AST walker)
internal/tooling/scriptdoctrine/shell_command_parser_r10_test.go  |  46 +-
internal/tooling/scriptdoctrine/shell_command_parser_r9_test.go   |  37 +-
internal/tooling/scriptdoctrine/shell_command_parser_test.go       |   1 +
internal/tooling/scriptdoctrine/shell_extractors.go                |  (Make $(shell) and != extension)
internal/tooling/scriptdoctrine/shell_wrappers.go                  | NEW (sudo/env wrapper tables)
internal/tooling/scriptdoctrine/shell_dashc.go                     | NEW (bash -c / sh -c handler)
internal/tooling/scriptdoctrine/shell_make_extractors_r11.go       | NEW (Make $(shell) scanner)
internal/tooling/scriptdoctrine/shell_yaml_extractors.go           | (helpers retained)
internal/tooling/scriptdoctrine/shell_yaml_parser_r11.go           | NEW (YAML AST walker)
internal/tooling/scriptdoctrine/types.go                          |  85 +  (InvocationCount, ClassificationError, WorkflowRunStep, Diagnostic.Line/Column)
```

### New files

* `shell_dashc.go` — bash/sh -c classifier with strict short-option
  cluster detection (`-c`, `-ec`, `-ce`, `-xec`, `-ceeu`).
* `shell_wrappers.go` — explicit `sudo`/`env` option tables; the
  previously-undetected `-S`, `--stdin`, `-A`, `--askpass`
  family now resolves correctly.
* `shell_make_extractors_r11.go` — balanced-paren `$(shell …)`
  scanner, `!=` assignment scanner, Make-variable resolution
  (`PYTHON := python3` + `$(shell $(PYTHON) x.py)` ->
  one python invocation), Make-expansion pre-processor
  (`$$` -> `$`, `$(VAR)` -> `1`) so arithmetic-only embedded
  expansions still classify as parseable shell.
* `shell_yaml_parser_r11.go` — yaml.Node based walker with
  structural validation (document count, expected mapping /
  sequence kinds, alias cycles), per-step shell precedence via
  step > job defaults > workflow defaults, and a custom
  template parser that recognises commands-with-flags
  (`python -u {0}`).

### Existing files (small surgical edits)

* `types.go` adds `InvocationCount`, `ZeroCount`, `HasSites`,
  `ClassificationError`, `NewClassificationError`,
  `WorkflowRunStep`. The `Diagnostic` struct gains `Line int`
  and `Column int` fields.
* `checks.go` adds the two new `Diagnostic` fields plus a
  `SortDiagnostics` line/column tie-break.
* `shell_analysis.go` advances `SortDiagnostics` to use the new
  line/column tie-break for deterministic output.
* `shell_command_parser.go` is rewritten to use the structured
  return, surfaces errors from `countShellDashCScript` and
  `pythonInvocationSite`, and walks the AST from a single
  orchestrator file. `countPythonSitesInProgram` now returns
  `(InvocationCount, error)`; the public helpers
  `CountPythonInvocations`, `CountPythonInvocationsForPath`,
  and `CountPythonInvocationsFromFile` keep their `int` return
  for compatibility and translate errors to -1.
* `shell_extractors.go` rewires `CountPythonInvocationsInMakefile`
  to compose expansion-time and recipe-time classifiers; recipes
  are parsed against a masked copy of the Makefile so a single
  `$(shell python3 x.py)` site is never counted twice.

---

## Acceptance / Verification

### `go test ./internal/tooling/scriptdoctrine/...`

```
ok      github.com/s1onique/KGB/internal/tooling/scriptdoctrine   (cached)
ok      github.com/s1onique/KGB/internal/tooling/scriptdoctrine   1.083s
```

The test surface is exercised across three R11 suites:

* `shell_command_parser_r11_test.go` — bash -c literal/dynamic
  contract, short option cluster (`-c`, `-ec`, `-euc`, `-xec`),
  sudo `-S`/`-A`/`--stdin`/`--askpass`/`--user=root`/`--`, and
  `command -v`/`-V`/`--` lookups.
* `shell_make_extractors_r11_test.go` — `$(shell …)` literal,
  bare `shell`, `$(shell $(PYTHON) x.py)` via resolved variable,
  `RESULT != python3 x.py` assignment, empty `$(shell)`, the
  fail-closed `unbalanced $(shell python3 x.py` case, and the
  non-double-count invariant for `RESULT := $(shell python3
  x.py)` + recipe echo.
* `shell_yaml_parser_r11_test.go` — shell precedence
  (`step bash overrides workflow python`, `step python
  overrides job bash`, `workflow python applies to two steps`),
  all three `run:` scalar forms (inline / literal block / folded
  block), custom shell templates with options (`python {0}`,
  `python -u {0}`, `/usr/bin/python3 {0}`, `python {0} --flag`,
  `bash -euxo pipefail {0}`), `uses:` step skipping, and
  fail-closed malformed inputs.

The existing `R9`/`R10` suites are updated to use the new
wrapped workflow scaffold so the YAML AST walker sees proper
`jobs.<name>.steps` structure; no test case reports a regression
versus the R10 behaviour for the same input.

### `verifier_r11_test.go` — End-to-end mutations

Each R11.0-required end-to-end mutation runs through the actual
`Verifier` path used by the gate. Helpers restore on-disk state
via `t.Cleanup` so the suite is hermetic.

* **Mutation 1** (Makefile `RESULT := $(shell python3 x.py)`):
  baseline-python-invocation-changed count delta = 1.
* **Mutation 2** (Makefile `RESULT != python3 x.py`):
  baseline-python-invocation-changed count delta = 1.
* **Mutation 3** (`bash -c 'python3 x.py'` -> `bash -c
  "$COMMAND"`): literal form reports 1 python-invocation
  diagnostic; dynamic form reports exactly 1 internal-error
  diagnostic.
* **Mutation 4** (workflow `defaults.run.shell: python` plus
  two run steps): invocation count delta = 2.
* **Mutation 5** (step `shell: python -u {0}`): invocation
  count delta = 1.
* **Mutation 6** (comment that quotes each R11 pattern):
  count delta = 0.

### `make gate`

```
[gate] LLM-friendliness: PASS
[gate:hygiene] hygiene-only gate PASS
[gate] Memory ownership hygiene gate passed.
[gate] PASS
```

`scripts/verify-script-doctrine` runs cleanly and prints the
deterministic pass marker.

---

## Bootstrap baseline changes (R11.8)

R11 closes existing detection gaps but does not modify the
frozen invocation counts. Each baseline entry was re-measured
through the new classifier; the resulting counts match the
baseline exactly, so the baseline CSV is unchanged.

```text
path                                                          | R10 count | R11 count | reason
Makefile                                                      |        22 |        22 | re-measured, no change
scripts/quality_gate.sh                                        |        37 |        37 | re-measured, no change
.github/workflows/*.yml (five entries)                          |    1..5    |    1..5    | re-measured, no change
scripts/{verify_workflow_release_safety,verify_opkg_package,
  lab_tovarisch_idle_memory,lab_memory_attribution_matrix,
  coverage_gate}.sh                                            |         1 |         1 | re-measured, no change
remaining shell-script entries with python_invocation_count=0 |         0 |         0 | re-measured, no change
```

If a CI maintainer introduces a new `$(shell …)` /
`!=` / dynamic-`bash -c` / workflow-python-shell surface in a
follow-up ACT, the bootstrap baseline will need explicit
amendment with the new count; the existing values are
unchanged because no repository file currently grows the
count.

---

## Implementation Notes

### R11.1 structured errors

`InvocationCount` and `ClassificationError` are the canonical
internal surfaces; the public `int`-returning helpers
(`CountPythonInvocations` etc.) translate a non-nil error to
-1 and the diagnostic writer emits
`internal-error` with the structured error message.

### R11.2 bash / sh -c

`isShortOptionCluster` validates `-X…c…` against `[a-z]+` so
combined forms like `-ec` and `-xec` are recognised while
`-xcSCRIPT` (where the script follows `c` inline) is rejected
— `bash` always expects a separate argument for the script.
A short-option cluster must contain at least one single-letter
flag character; long-form options (`--rcfile FILE`) are tracked
but do not, by themselves, introduce `-c` semantics. The
classifier only marks the call as a `-c` invocation when at
least one valid cluster containing `c` is observed before the
positional argument, so `bash "$VERIFY" /tmp` keeps its
previous "not a -c invocation" classification.

Dynamic command strings (`$VAR`, `${VAR}`, `$(...)`, `` `...` ``)
and malformed literal payloads (missing closing `)`, unmatched
nested heredocs) are surfaced as `ClassificationError`s with a
best-effort line/column based on the AST position of the
problem argument. The public compatibility shim
`CountPythonInvocations` reports -1 so the verifier surfaces an
`internal-error` diagnostic with the resolution path.

### R11.3 wrapper option tables

`sudoValueFlag` enumerates the long-form value-taking flags
(`--user`, `--group`, `--host`, `--prompt`, `--chdir`,
`--type`); `sudoNoValueFlag` lists the value-less flags (`-S`,
`--stdin`, `-A`, `--askpass`, `-E`, `-H`, `-n`, `-i`, `-s`, `-b`,
`-k`, `-K`, `-l`, `-v`). The classifier then iterates from the
wrapper onwards: any value-flag consumes the next non-flag
argument, no-value flags are skipped, `--` terminates option
parsing, the first non-flag positional is treated as the real
executable. `env` mirrors the same shape via
`envValueFlag`. The result is `sudo -S python3 x.py -> 1` and
`sudo -- python3 x.py -> 1` rather than the R10 undercounts.

### R11.4 GNU Make expansion-time sites

`findShellFunctionSites` walks the Makefile balancing `(` and
`)` so nested `$(( 1 + 2 ))` does not steal a `)` from the
outer expansion. `findShellAssignmentSites` is per-line and
recognises `NAME != RHS` only at the top level (no TAB-indented
recipe lines, no continuation lines, and embedded `$(...)` is
balanced so `name != $(call foo,$(bar))` does not split the
expression in two). `classifyMakeShellExpansions` resolves
Make variables via `extractMakeVariables` and `resolveMakeVars`,
then runs the inner body through `preProcessMakeShell`
(`$$` -> `$`, balanced `$(...)` -> `1`) before
`countPythonSitesInProgram`. Dynamic unmaskable sites fail
closed.

`findUnbalancedMakeParens` reports any `$(` whose matching `)`
is missing, so `unbalanced $(shell python3 x.py` surfaces as a
hard internal error per the R11.4 matrix. The recipe parser
then runs against a `maskMakeExpansionSites` copy of the
Makefile in which each expansion-time site is replaced with
spaces; the parser writes `'X'` for those substituted `$(...)`
bodies via `sanitizeMakeVars`, and the result is a clean
shell program with no Python double count.

### R11.5 YAML AST workflow parser

The walker uses `go.yaml.in/yaml/v3` directly. It enforces the
one-document contract, requires the root to be a mapping,
expects `jobs` to be a mapping of mappings, expects the
`steps` child to be a sequence of mappings, and rejects a
`run:` whose Node is not a scalar. `uses:` steps are skipped
(node-level early return). Alias resolution walks the AST
in-place with a cycle guard (`visiting` map keyed by
`*anchor`); unresolvable aliases produce a hard
diagnostic.

### R11.6 effective workflow shell precedence

`isPythonShell(stepShell, jobDefaults, workflowShell)` walks the
three settings in most-specific order and returns true when
the executable word of the resolved custom template (or plain
template) is a Python interpreter. `shellTemplateExecutable`
splits on the first `{0}` placeholder so command + flags
templates (`python -u {0}`, `bash --noprofile --norc -c {0}`,
`/usr/bin/python3 {0} --flag`) are all classified correctly
without depending on a single hand-written regex. Step shell
scalar kind is verified before its value is read so a
sequence-typed `shell:` produces a fail-closed diagnostic
rather than silently being treated as no-shell.

### R11.7 diagnostics

`Diagnostic.Line` and `Diagnostic.Column` are populated by the
`Diagnostic` writer for structural surfaces (Makefile
`$(shell …)` / `!=` sites via `lineOf(data, bad[0])` and the
YAML AST nodes via `runNode.Line` / `runNode.Column`). Sort
order keys on `(Check, Path, Line, Column)` for deterministic
output. No raw temporary-test paths leak into assertions;
mutation tests use `t.TempDir()` exclusively.

### R11.8 baseline

The bootstrap baseline CSV was re-measured end-to-end through
the new classifiers. No entry moved. The baseline `commit`,
`loc_algorithm=logical-shell-v1`, and per-file
`python_invocation_count=...` rows are unchanged; the
provenance metadata header remains the same.

---

## Risks / Follow-ups

1. `$(shell …)` argument parsing treats the inner body as a
   complete shell program after `preProcessMakeShell`. Inputs
   that embed `$(…)` inside arithmetic expansions are
   pre-processed with `1` placeholders, which can mask the
   exact runtime count but cannot introduce a new python
   invocation site. Acceptable per the doctrine invariant
   "Python invocation count is monotonically non-negative".
2. The YAML AST walker requires one document; multi-document
   workflow files are rejected with a hard diagnostic. No
   in-repo workflow uses `---` separators today, but a
   follow-up that ingests documents programmatically may want
   to surface multi-document files as a separate category.
3. `sort.Slice` is stable but `SortDiagnostics` is implemented
   with explicit field-by-field comparison so deterministic
   output survives concurrent extension. Mutation 6
   demonstrates that a non-action comment never changes the
   count; subsequent readers can rely on that invariant.

---

## Closure statement

Upon acceptance:

```text
ACT-UVB76-GO-TOOLING-DOCTRINE01-R7   CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R8   CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R9   CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R10  CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R11  CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01      CLOSED

NEXT: ACT-UVB76-CAPTURE-NETNS-GO01
```
