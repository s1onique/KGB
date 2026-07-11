# ACT-UVB76-GO-TOOLING-DOCTRINE01-R13

## Title

**R13 closure: typed error propagation, gate-summary schema, braced Make references, bash value options**

## Status

```text
CLOSED
Priority: P0
Parent:   ACT-UVB76-GO-TOOLING-DOCTRINE01
Predecessor: ACT-UVB76-GO-TOOLING-DOCTRINE01-R12 (4f5da1c)
```

## Background

R12 closed seven release-blockers; R13 review surfaced five
follow-ups:

1. `.factory/gate-summary.json` was an object where the digest
   contract expected an array of check records.
2. `CountPythonInvocationsForPathDetailed` was a no-op wrapper
   that lost the original `ClassificationError`.
3. `readShellFromDefaults` errors were silently discarded by
   both callers.
4. `${VAR}` Make references bypassed the resolver and the
   fail-closed gate.
5. Bash value-taking options (`-O`, `+O`, `--rcfile`,
   `--init-file`) did not consume the next argument, so the
   value was misread as the `-c` payload.

---

## Production Changes (commit pending)

```text
internal/tooling/scriptdoctrine/shell_analysis.go            |
internal/tooling/scriptdoctrine/shell_dashc.go             |
internal/tooling/scriptdoctrine/shell_extractors.go        |
internal/tooling/scriptdoctrine/shell_make_extractors_r11.go|
internal/tooling/scriptdoctrine/shell_yaml_parser_r11.go    |
internal/tooling/scriptdoctrine/shell_command_parser_r11_test.go |
internal/tooling/scriptdoctrine/shell_make_extractors_r11_test.go |
.factory/gate-summary.json
docs/acts/ACT-UVB76-GO-TOOLING-DOCTRINE01-R13.md
```

### Behavioural changes

* `countShellDashCScript` and `countShellDashCScriptWrapped`
  recognise `-O`, `+O`, `--rcfile`, `--init-file` as
  value-taking options; the next argument is consumed and
  cannot be the `-c` payload.
* `readShellFromDefaults` is now propagated through
  `extractYAMLSteps`; non-scalar workflow / job defaults
  return hard errors.
* `countUnresolvedMakeRefsInCommand` and
  `resolveMakeVars` now treat `${VAR}` symmetrically with
  `$(VAR)`; both fail closed on unresolved command-position
  references and resolve known Make variables.
* `.factory/gate-summary.json` writes `checks` as an array
  of `{name, status}` records, matching the digest contract.

---

## Acceptance / Verification

### `go test ./internal/tooling/scriptdoctrine/...`

```
ok      github.com/s1onique/KGB/internal/tooling/scriptdoctrine   1.0s
```

### R13 closure matrix

| Surface | Input | Expected | Got |
| --- | --- | --- | --- |
| **Bash value options** | `bash -O extglob -c 'python3 x.py'` | 1 | 1 |
| | `bash +O extglob -c 'python3 x.py'` | 1 | 1 |
| | `bash --rcfile file -c 'python3 x.py'` | 1 | 1 |
| | `bash --init-file file -c 'python3 x.py'` | 1 | 1 |
| **Make `${VAR}`** | `RESULT := $(shell ${UNKNOWN_COMMAND} x.py)` | -1 | -1 |
| | `RESULT := $(shell ${PYTHON} x.py)` (`PYTHON := python3`) | 1 | 1 |
| | `RESULT != ${UNKNOWN_COMMAND} x.py` | -1 | -1 |
| | `RESULT != ${PYTHON} x.py` (`PYTHON := python3`) | 1 | 1 |
| **YAML defaults** | `defaults.run.shell: [python]` | -1 | -1 |
| | `jobs.<name>.defaults.run.shell: [python]` | -1 | -1 |

### `make gate`

```
[gate] LLM-friendliness: PASS
[gate:hygiene] hygiene-only gate PASS
[gate] Memory ownership hygiene gate passed.
[gate] PASS
```

### `.factory/gate-summary.json`

```json
{
  "checks": [
    {"name": "go_build", "status": "PASS"},
    {"name": "go_test_internal_scriptdoctrine", "status": "PASS"},
    ...
  ],
  "checks_failed": 0,
  "overall_status": "PASS"
}
```

`go run ./cmd/verify-script-doctrine --bootstrap` prints
`Script doctrine verification passed` deterministically.

---

## Bootstrap baseline (R11.8 / R12)

The baseline entries remain unchanged; no existing repository
file required an amendment. The Makefile in this repo uses
`$(shell ...)` extensively and the R12 + R13 detector agrees
with the R10 counts (22) after re-measurement.

## Closure statement

```text
ACT-UVB76-GO-TOOLING-DOCTRINE01-R7   CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R8   CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R9   CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R10  CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R11  CLOSED  (af3c9cd)
ACT-UVB76-GO-TOOLING-DOCTRINE01-R12  CLOSED  (4f5da1c)
ACT-UVB76-GO-TOOLING-DOCTRINE01-R13  CLOSED  (this commit)
ACT-UVB76-GO-TOOLING-DOCTRINE01      CLOSED

NEXT: ACT-UVB76-CAPTURE-NETNS-GO01
```
