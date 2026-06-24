# Idle Staircase Memory Lab - Architecture

The idle staircase lab runner is **Python-owned**. The `.sh` file is a compatibility wrapper only.

## Shell Wrapper (`lab_tovarisch_idle_memory.sh`)

The shell script is a thin launcher that:
- Sets `set -euo pipefail`
- Resolves `SCRIPT_DIR`
- Executes Python runner: `exec python3 "${SCRIPT_DIR}/lab_tovarisch_idle_memory.py" "$@"`

## Python Runner (`lab_tovarisch_idle_memory.py`)

The Python runner owns all lab logic:

- **Argument parsing**: All CLI flags supported
- **Config generation**: Writes `tovarisch_lab.conf` with `[lab]` section
- **Manifest writing**: Records native flags, git state, platform info
- **Process lifecycle**: `subprocess.Popen` for tovarisch, proper termination
- **`/proc` sampling**: Reads `VmRSS`, `VmHWM`, `VmSize`, `VmData`, `VmSwap` directly in Python
- **Synthetic event timeline**: Shell-side bookkeeping events (cannot prove native attribution)
- **Analyzer invocation**: Subprocess call to `idle_staircase_analyzer_cli.py`
- **Output validation**: Fail-closed on missing `native_event_timeline.tsv` when enabled
- **Artifact summary**: Final verdict and artifact path

## Module Structure

```
scripts/lab_runner/
  __init__.py      - Package exports
  config.py        - LabRunConfig dataclass, parse_args(), LabError
  proc.py          - require_linux_proc(), read_proc_status()
  artifacts.py     - Config/manifest/event writing functions
  tovarisch.py     - Process lifecycle (start, wait, terminate)
  loop.py          - Idle sampling loop, status burst
  analyzer.py      - Analyzer CLI invocation
  validation.py    - Output verification, final summary
  self_tests.py    - Self-test suite
  main.py          - Main entry point
```

## Event Attribution Rules

- **`event_timeline.tsv`**: Shell/Python-side synthetic bookkeeping. **Cannot prove native attribution.**
- **`native_event_timeline.tsv`**: Tovarisch-native events from real runtime paths. **Required for `confirmed_leak`.**

Missing `native_event_timeline.tsv` in enabled mode is a **hard failure**. The Python runner fails with actionable diagnostics.

## Native Event Output Validation

| Mode | `native_event_timeline.tsv` Policy |
|------|-------------------------------------|
| `--native-events` (heartbeat enabled) | **Required**: Must exist and be non-empty with heartbeat events |
| `--native-events --disable-heartbeat` | **Optional**: May be missing/header-only if no native events emitted |
| No native events | Not required |

## Process Lifecycle

1. Start tovarisch with `subprocess.Popen`
2. Wait 2s for startup
3. Verify `process.poll() is None`
4. Verify `/proc/<pid>` exists before sampling
5. Run idle loop with `/proc` sampling
6. Terminate gracefully, kill if still alive after 5s
7. Copy native timeline if needed
8. Run analyzer
9. Verify outputs

## Self-Tests

```bash
python3 scripts/lab_tovarisch_idle_memory.py --self-test
```

Self-tests verify:
- Config generation with/without native_events_path
- Manifest native flag recording
- `/proc` status parser correctness
- Invalid run_id rejection
- Duration/interval validation
