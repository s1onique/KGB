# ACT-HULK29R-ZIG016-TEST-BASE-FINGERPRINT-CLI-INVENTORY

## Objective

Register `tests/test_tovarisch_test_base_fingerprint.py` subprocess usage in CLI composition inventory to unblock `make gate`.

## Root Cause

The fingerprint self-test intentionally uses `subprocess.run()` to verify the script's real executable contract. The CLI composition inventory verifier (`scripts/verify_cli_composition_inventory.py`) detected unregistered subprocess usage:
- line 14: `subprocess module` import
- line 41: `subprocess module` import
- line 41: `subprocess.run()` call

This is legitimate external process boundary, not a false positive. The inventory was not updated when the ACT that introduced the test was created.

## Changes

### `docs/tooling/cli-composition-inventory.csv`

Added two narrow inventory entries:

```csv
CLI-0047,tests/test_tovarisch_test_base_fingerprint.py,python,subprocess module,tooling,build_test,once,yes,Self-test imports subprocess to execute the fingerprint script through the Python interpreter and verify the real CLI artifact boundary,yes,yes,no,no,verified,Test-only verifier self-test wrapper (ACT-HULK29R-ZIG016-TEST-BASE-FINGERPRINT-CLI-INVENTORY)
CLI-0048,tests/test_tovarisch_test_base_fingerprint.py,python,subprocess call,tooling,build_test,once,yes,Self-test calls subprocess.run with explicit argv to verify script execution and artifact creation; bounded by timeout=300, argv list uses sys.executable, no shell,yes,yes,no,no,verified,Test-only verifier self-test wrapper (ACT-HULK29R-ZIG016-TEST-BASE-FINGERPRINT-CLI-INVENTORY)
```

## Verification

```bash
python3 scripts/verify_cli_composition_inventory.py
make gate
```

## Expected Result

CLI composition inventory verifier passes; gate proceeds past the current blocker.

## Files Changed

- `docs/tooling/cli-composition-inventory.csv` (added CLI-0047, CLI-0048)
- `docs/acts/ACT-HULK29R-ZIG016-TEST-BASE-FINGERPRINT-CLI-INVENTORY.md` (new)
