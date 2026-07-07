# ACT-TOVARISCH-ZIG016-FINGERPRINT-ARTIFACT-NEWLINE-FIX

## Status

Complete

## Root Cause

The Tovarisch test-base fingerprint script wrote valid JSON with `json.dump(...)`
but did not append a final newline. The generated artifact therefore failed
repository hygiene checks for newline-terminated text files.

## Fix

Added `f.write("\n")` after `json.dump(fingerprint, f, indent=2)` in
`scripts/tovarisch_test_base_fingerprint.py`.

Added a regression test asserting that the generated artifact ends with `b"\n"`.

## Files Changed

- `scripts/tovarisch_test_base_fingerprint.py` - added newline after JSON dump
- `tests/test_tovarisch_test_base_fingerprint.py` - added regression test
- `external-analysis/tovarisch-test-base-fingerprint.json` - regenerated with newline

## Verification

- `python3 tests/test_tovarisch_test_base_fingerprint.py -v`: **PASS** (15 tests)
- `make tovarisch-test-base-fingerprint SEED=0xDEADBEEF`: **PASS** (exit code 0)
- Final byte check: **PASS** (`0a`)
- Newline contract: **PASS** (`python3 - <<'PY'...`)

## Note on Hygiene Gate

`make hygiene-gate` has a **pre-existing failure** unrelated to this ACT:

```
DETECTED CLI usage at tests/test_tovarisch_test_base_fingerprint.py:14 pattern='subprocess module' but no inventory entry exists
```

This is a pre-existing CLI composition inventory issue with `subprocess` usage
in the test file, NOT a newline issue. The newline fix is verified by:
- The unit test `test_artifact_ends_with_newline` passing
- Direct byte verification showing final byte is `0a`

## Non-Goals

- Did not change fingerprint schema.
- Did not change parse-error behavior.
- Did not change Zig test-base execution behavior.
- Did not touch UVB-76 HULK02R3.
