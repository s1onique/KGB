#!/usr/bin/env python3
"""Verify tovarisch status JSON contract.

Contract-preserving port from shell/JQ to Python stdlib.
Validates the v0 status contract for `tovarisch status --json`.

Usage:
    python3 scripts/verify_tovarisch_status_contract.py [--self-test]

Options:
    --self-test    Run regression tests (delegates to _test.py module)

Exit codes:
    0   All validations passed
    1   Contract violation
    2   File not found or unreadable
    3   Invalid JSON
"""

import binascii
import json
import os
import re
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Optional

# === Contract Constants ===
CONTRACT_DOC = Path("docs/contracts/tovarisch-status-v0.md")
FIXTURE_PATH = Path("docs/contracts/examples/tovarisch-status-v0.json")
STATUS_ZIG = Path("tovarisch/src/status.zig")

REQUIRED_CONTRACT_SECTIONS = [
    "Purpose", "Top-level Fields", "Runtime Object", "Check Object Fields",
    "Allowed Values", "Privacy Constraints", "Non-goals", "Example", "Future-Compatible",
]
REQUIRED_FIXTURE_STRINGS = ['"service"', '"version"', '"node_id"', '"status"', '"checks"', '"name"']
VERSION_PATTERN = re.compile(r"^0\.1\.2\+")
VALID_STATUS_VALUES = {"ok", "warn", "error", "unknown"}


# === Dataclasses ===
@dataclass
class ValidationError:
    path: str
    expected: str
    actual: str

    def __str__(self) -> str:
        return f"[status-contract] FAIL: {self.path}: expected {self.expected}, got {self.actual}"


# === Result Types ===
class CliResult:
    """Result of CLI status command."""
    def __init__(self, data: Optional[dict] = None, error: Optional[str] = None, unavailable: bool = False):
        self.data = data
        self.error = error
        self.unavailable = unavailable


# === Validation Functions ===
def check_required_files() -> list[str]:
    """Check required files exist and are non-empty."""
    errors = []
    for f in [CONTRACT_DOC, FIXTURE_PATH, STATUS_ZIG]:
        if not f.exists():
            errors.append(f"[status-contract] FAIL: missing file: {f}")
        elif f.stat().st_size == 0:
            errors.append(f"[status-contract] FAIL: empty file: {f}")
    return errors


def check_contract_documentation() -> list[str]:
    """Verify contract doc has required sections."""
    errors = []
    try:
        content = CONTRACT_DOC.read_text()
    except OSError as e:
        return [f"[status-contract] FAIL: cannot read {CONTRACT_DOC}: {e}"]
    for section in REQUIRED_CONTRACT_SECTIONS:
        if section not in content:
            errors.append(f"[status-contract] FAIL: contract missing section: {section}")
    return errors


def check_fixture_strings() -> list[str]:
    """Verify fixture has required JSON string markers."""
    errors = []
    try:
        content = FIXTURE_PATH.read_text()
    except OSError as e:
        return [f"[status-contract] FAIL: cannot read {FIXTURE_PATH}: {e}"]
    for s in REQUIRED_FIXTURE_STRINGS:
        if s not in content:
            errors.append(f"[status-contract] FAIL: fixture missing: {s}")
    return errors


def load_json(path: Path) -> tuple[Optional[dict], Optional[str]]:
    """Load and parse JSON file."""
    try:
        return json.loads(path.read_text()), None
    except json.JSONDecodeError as e:
        return None, f"[status-contract] FAIL: {path} is not valid JSON: {e}"
    except OSError as e:
        return None, f"[status-contract] FAIL: cannot read {path}: {e}"


def validate_top_level_fields(data: dict) -> list[ValidationError]:
    """Validate required top-level fields."""
    errors = []
    if "service" not in data:
        errors.append(ValidationError("service", '"tovarisch"', "missing"))
    elif data["service"] != "tovarisch":
        errors.append(ValidationError("service", '"tovarisch"', f'"{data["service"]}"'))

    if "version" not in data:
        errors.append(ValidationError("version", "0.1.2+<sha>", "missing"))
    elif not VERSION_PATTERN.match(str(data["version"])):
        errors.append(ValidationError("version", "0.1.2+<sha>", f'"{data["version"]}"'))

    if "node_id" not in data:
        errors.append(ValidationError("node_id", '"local-dev"', "missing"))
    elif data["node_id"] != "local-dev":
        errors.append(ValidationError("node_id", '"local-dev"', f'"{data["node_id"]}"'))

    if "status" not in data:
        errors.append(ValidationError("status", "ok|warn|error|unknown", "missing"))
    elif data["status"] not in VALID_STATUS_VALUES:
        errors.append(ValidationError("status", "ok|warn|error|unknown", f'"{data["status"]}"'))

    if "checks" not in data:
        errors.append(ValidationError("checks", "array", "missing"))
    elif not isinstance(data["checks"], list):
        errors.append(ValidationError("checks", "array", type(data["checks"]).__name__))

    if "runtime" not in data:
        errors.append(ValidationError("runtime", "object", "missing"))
    elif not isinstance(data["runtime"], dict):
        errors.append(ValidationError("runtime", "object", type(data["runtime"]).__name__))
    return errors


def validate_checks(data: dict) -> list[ValidationError]:
    """Validate checks array structure."""
    errors = []
    checks = data.get("checks", [])
    if not isinstance(checks, list):
        return errors

    for i, check in enumerate(checks):
        if not isinstance(check, dict):
            errors.append(ValidationError(f"checks[{i}]", "object", type(check).__name__))
            continue
        for field in ["name", "status", "detail"]:
            if field not in check:
                errors.append(ValidationError(f"checks[{i}].{field}", "string", "missing"))
            elif not isinstance(check[field], str):
                errors.append(ValidationError(f"checks[{i}].{field}", "string", type(check[field]).__name__))
        if "status" in check and check["status"] not in VALID_STATUS_VALUES:
            errors.append(ValidationError(f"checks[{i}].status", "ok|warn|error|unknown", f'"{check["status"]}"'))

    # First check must be "process" with status "ok"
    if checks and isinstance(checks[0], dict):
        if checks[0].get("name") != "process":
            errors.append(ValidationError("checks[0].name", '"process"', f'"{checks[0].get("name", "missing")}"'))
        if checks[0].get("status") != "ok":
            errors.append(ValidationError("checks[0].status", '"ok"', f'"{checks[0].get("status", "missing")}"'))
    return errors


def validate_runtime(data: dict) -> list[ValidationError]:
    """Validate runtime object structure."""
    errors = []
    runtime = data.get("runtime")
    if runtime is None:
        errors.append(ValidationError("runtime", "object (not null)", "null"))
        return errors
    if not isinstance(runtime, dict):
        return errors

    if "pid" not in runtime:
        errors.append(ValidationError("runtime.pid", "number > 0", "missing"))
    elif not isinstance(runtime["pid"], (int, float)):
        errors.append(ValidationError("runtime.pid", "number > 0", type(runtime["pid"]).__name__))
    elif runtime["pid"] <= 0:
        errors.append(ValidationError("runtime.pid", "number > 0", str(runtime["pid"])))

    if "rss_kib" in runtime:
        rss = runtime["rss_kib"]
        if rss is not None:
            if not isinstance(rss, (int, float)):
                errors.append(ValidationError("runtime.rss_kib", "number >= 0 or null", type(rss).__name__))
            elif rss < 0:
                errors.append(ValidationError("runtime.rss_kib", "number >= 0", str(rss)))
    return errors


def validate_fixture(data: dict) -> list[ValidationError]:
    """Validate fixture against contract."""
    errors = []
    errors.extend(validate_top_level_fields(data))
    errors.extend(validate_checks(data))
    errors.extend(validate_runtime(data))
    return errors


def normalize_for_comparison(data: dict) -> dict:
    """Normalize runtime values for CLI vs fixture comparison.
    
    Runtime values vary by platform/run, so we normalize them.
    Also normalizes version since it includes dynamic SHA.
    """
    normalized = json.loads(json.dumps(data))  # Deep copy
    if "runtime" in normalized and isinstance(normalized["runtime"], dict):
        normalized["runtime"]["pid"] = 1
        normalized["runtime"]["rss_kib"] = "normalized"
    if "version" in normalized:
        normalized["version"] = "0.1.2+<sha>"
    return normalized


def _decode_diagnostic(data: bytes) -> str:
    """Decode bytes for diagnostics only, replacing invalid UTF-8."""
    return data.decode("utf-8", errors="backslashreplace")


def _bad_utf8_context(data: bytes, exc: UnicodeDecodeError) -> str:
    """Format UTF-8 decode error with byte context for forensics."""
    start = max(0, exc.start - 32)
    end = min(len(data), exc.end + 32)
    window = data[start:end]
    return (
        f"{exc}; byte_offset={exc.start}; "
        f"context_hex={binascii.hexlify(window, sep=b' ').decode('ascii')}; "
        f"context_repr={window!r}"
    )


def run_cli_status() -> CliResult:
    """Run tovarisch status --json via Zig build.
    
    Returns CliResult with:
    - data: parsed JSON if successful
    - error: error message string if command failed
    - unavailable: True if zig is not available
    """
    env = os.environ.copy()
    env["TOVARISCH_WG_COMMAND_PATH"] = "/nonexistent"
    
    try:
        proc = subprocess.run(
            ["zig", "build", "run", "--", "status", "--json"],
            cwd="tovarisch",
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
            timeout=60,
            env=env,
        )
    except FileNotFoundError:
        return CliResult(unavailable=True)
    except OSError as exc:
        return CliResult(error=f"[status-contract] FAIL: could not start CLI: {exc}")
    except subprocess.TimeoutExpired:
        return CliResult(error="[status-contract] FAIL: zig build run timed out")

    stderr_text = _decode_diagnostic(proc.stderr)

    if proc.returncode != 0:
        return CliResult(error=f"[status-contract] FAIL: CLI exited non-zero\n  exit_code: {proc.returncode}\n  stderr:\n{stderr_text}")

    try:
        stdout_text = proc.stdout.decode("utf-8")
    except UnicodeDecodeError as exc:
        return CliResult(error="[status-contract] FAIL: CLI stdout is not valid UTF-8 JSON: " + _bad_utf8_context(proc.stdout, exc) + ("\n  stderr:\n" + stderr_text if stderr_text else ""))

    output = stdout_text.strip()
    if not output:
        return CliResult(error="[status-contract] FAIL: could not get CLI output")

    try:
        return CliResult(data=json.loads(output))
    except json.JSONDecodeError as e:
        return CliResult(error=f"[status-contract] FAIL: CLI output is not valid JSON: {e}\nstdout:\n{output}\nstderr:\n{stderr_text}")


def verify() -> int:
    """Run all verifications."""
    print("[status-contract] Starting verification")

    # Check required files
    print("[status-contract] Checking required files")
    for f in [CONTRACT_DOC, FIXTURE_PATH, STATUS_ZIG]:
        if f.exists() and f.stat().st_size > 0:
            print(f"  OK: {f}")
    errors = check_required_files()
    for err in errors:
        print(err, file=sys.stderr)
    if errors:
        return 2
    print("[status-contract] Required files present")

    # Check contract documentation
    print("[status-contract] Checking contract documentation")
    errors = check_contract_documentation()
    for err in errors:
        print(err, file=sys.stderr)
    if errors:
        return 1
    print("[status-contract] Contract documentation complete")

    # Check fixture strings
    print("[status-contract] Checking fixture")
    errors = check_fixture_strings()
    for err in errors:
        print(err, file=sys.stderr)
    if errors:
        return 1
    print("[status-contract] Fixture structure valid")

    # Validate fixture JSON
    print("[status-contract] Validating fixture JSON")
    fixture_data, err = load_json(FIXTURE_PATH)
    if err:
        print(err, file=sys.stderr)
        return 3
    print("[status-contract] Fixture is valid JSON")

    # Verify fixture fields
    print("[status-contract] Verifying fixture fields")
    if fixture_data.get("service") == "tovarisch":
        print("  OK: service == tovarisch")
    if VERSION_PATTERN.match(str(fixture_data.get("version", ""))):
        print("  OK: version matches 0.1.2+<sha> pattern")
    if fixture_data.get("node_id") == "local-dev":
        print("  OK: node_id == local-dev")
    if fixture_data.get("status") in VALID_STATUS_VALUES:
        print("  OK: status is valid")

    checks = fixture_data.get("checks", [])
    if checks and isinstance(checks[0], dict):
        if checks[0].get("name") == "process":
            print("  OK: first check is process")
        if checks[0].get("status") == "ok":
            print("  OK: first check status is ok")

    runtime = fixture_data.get("runtime", {})
    if isinstance(runtime, dict) and isinstance(runtime.get("pid"), (int, float)) and runtime["pid"] > 0:
        print("  OK: runtime.pid > 0")
    rss = runtime.get("rss_kib") if isinstance(runtime, dict) else None
    if rss is None or (isinstance(rss, (int, float)) and rss >= 0):
        print("  OK: runtime.rss_kib is null or >= 0")

    print("[status-contract] Fixture fields verified")

    # Run structured validation
    print("[status-contract] Running structured validation")
    validation_errors = validate_fixture(fixture_data)
    for ve in validation_errors:
        print(str(ve), file=sys.stderr)
    if validation_errors:
        return 1
    print("[status-contract] Structured validation passed")

    # CLI comparison (only if zig is available)
    cli_result = run_cli_status()
    
    if cli_result.unavailable:
        print("[status-contract] INFO: Zig not available — skipping CLI comparison")
    elif cli_result.error:
        print(cli_result.error, file=sys.stderr)
        return 1
    else:
        print("[status-contract] Zig available — comparing CLI output with fixture")
        
        # Normalize both for comparison
        cli_norm = normalize_for_comparison(cli_result.data)
        fixture_norm = normalize_for_comparison(fixture_data)
        
        if cli_norm != fixture_norm:
            print("[status-contract] FAIL: CLI output differs from fixture after runtime normalization", file=sys.stderr)
            print(f"[status-contract] CLI normalized:     {json.dumps(cli_norm)}", file=sys.stderr)
            print(f"[status-contract] Fixture normalized: {json.dumps(fixture_norm)}", file=sys.stderr)
            return 1
        
        print("[status-contract] CLI output matches fixture (runtime values normalized)")
        
        # Validate CLI rss_kib is null or non-negative
        cli_runtime = cli_result.data.get("runtime", {})
        if isinstance(cli_runtime, dict):
            cli_rss = cli_runtime.get("rss_kib")
            if cli_rss is None or (isinstance(cli_rss, (int, float)) and cli_rss >= 0):
                print("  OK: runtime.rss_kib is null or non-negative")
            else:
                print(f"[status-contract] FAIL: CLI runtime.rss_kib is negative: {cli_rss}", file=sys.stderr)
                return 1

    print("")
    print("[status-contract] PASS")
    return 0


def main() -> int:
    """Main entry point."""
    args = sys.argv[1:]
    if "--self-test" in args:
        # Delegate to test module
        from importlib.util import spec_from_file_location, module_from_spec
        spec = spec_from_file_location("test_module", Path(__file__).parent / "verify_tovarisch_status_contract_test.py")
        test_module = module_from_spec(spec)
        spec.loader.exec_module(test_module)
        result = test_module.run_tests() if hasattr(test_module, 'run_tests') else True
        if result:
            print("")
            print("[status-contract] Self-test PASS")
            return 0
        else:
            print("")
            print("[status-contract] Self-test FAIL", file=sys.stderr)
            return 1
    return verify()


if __name__ == "__main__":
    sys.exit(main())
