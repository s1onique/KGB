#!/usr/bin/env python3
"""Verifies the tovarisch-idle-native-heartbeat-smoke workflow follows required shape."""

import argparse
import re
import sys
import tempfile
from pathlib import Path
from typing import Optional


# Workflow-specific expectations
WORKFLOW_FILE = "tovarisch-idle-native-heartbeat-smoke.yml"
REQUIRED_RUN_IDS = ["heartbeat-native-enabled-smoke", "heartbeat-native-disabled-smoke"]


class WorkflowShapeError:
    def __init__(self, msg: str):
        self.message = msg

    def __str__(self) -> str:
        return f"SHAPE-ERROR: {self.message}"


def parse_workflow(content: str) -> dict:
    """Parse key workflow properties."""
    lines = content.split("\n")
    props = {
        "has_workflow_dispatch": False,
        "has_push": False,
        "has_pull_request": False,
        "runs_on_linux": False,
        "timeout_minutes": None,
        "runs_lab_script": False,
        "runs_disable_heartbeat": False,
        "runs_verifier": False,
        "uploads_with_always": False,
        "has_bounded_timeout": False,
    }
    in_job = False
    has_lab_script = False
    for i, line in enumerate(lines):
        stripped = line.strip()
        # Check for workflow_dispatch
        if stripped == "workflow_dispatch:":
            props["has_workflow_dispatch"] = True
        # Check for push trigger
        if re.match(r"^\s*push\s*:", stripped):
            props["has_push"] = True
        # Check for pull_request trigger
        if re.match(r"^\s*pull_request\s*:", stripped):
            props["has_pull_request"] = True
        # Check for Linux runner
        if "runs-on:" in stripped and "ubuntu" in stripped:
            props["runs_on_linux"] = True
        # Check for timeout-minutes
        if m := re.search(r"timeout-minutes:\s*(\d+)", stripped):
            props["timeout_minutes"] = int(m.group(1))
            props["has_bounded_timeout"] = True
        # Check for lab script execution (allow multiline)
        if "lab_tovarisch_idle_memory.sh" in stripped:
            has_lab_script = True
            # Look ahead for --native-events in next few lines
            for j in range(i, min(i + 10, len(lines))):
                if "--native-events" in lines[j]:
                    props["runs_lab_script"] = True
                    break
        # Check for disable-heartbeat
        if "--disable-heartbeat" in stripped:
            props["runs_disable_heartbeat"] = True
        # Check for smoke verifier
        if "verify_idle_staircase_native_heartbeat_smoke.py" in stripped:
            props["runs_verifier"] = True
        # Check for upload with always (allow multiline)
        # Accepts: if: always() or if: always() && ...
        # Check if this line or adjacent lines contain the pattern
        if "actions/upload-artifact" in stripped:
            found = False
            # Look before and after for if: always()
            for j in range(max(0, i - 5), min(i + 10, len(lines))):
                if re.search(r"if:\s*always\(\)", lines[j]):
                    props["uploads_with_always"] = True
                    found = True
                    break
    return props


def check_workflow_shape(workflow_path: Path) -> list[WorkflowShapeError]:
    """Check that the workflow follows the required shape."""
    errors = []
    if not workflow_path.exists():
        errors.append(WorkflowShapeError(f"Workflow file not found: {workflow_path}"))
        return errors

    content = workflow_path.read_text()
    props = parse_workflow(content)

    if not props["has_workflow_dispatch"]:
        errors.append(WorkflowShapeError("Workflow must have workflow_dispatch trigger"))
    if props["has_push"]:
        errors.append(WorkflowShapeError("Workflow must NOT have push trigger"))
    if props["has_pull_request"]:
        errors.append(WorkflowShapeError("Workflow must NOT have pull_request trigger"))
    if not props["runs_on_linux"]:
        errors.append(WorkflowShapeError("Workflow must run on Linux runner (ubuntu)"))
    if not props["runs_lab_script"]:
        errors.append(WorkflowShapeError("Workflow must run lab_tovarisch_idle_memory.sh with --native-events"))
    if not props["runs_disable_heartbeat"]:
        errors.append(WorkflowShapeError("Workflow must run --disable-heartbeat variant"))
    if not props["runs_verifier"]:
        errors.append(WorkflowShapeError("Workflow must run verify_idle_staircase_native_heartbeat_smoke.py"))
    if not props["uploads_with_always"]:
        errors.append(WorkflowShapeError("Workflow must upload artifacts with if: always()"))
    if not props["has_bounded_timeout"]:
        errors.append(WorkflowShapeError("Workflow must have bounded timeout-minutes"))

    # Check for deterministic run IDs in content
    for run_id in REQUIRED_RUN_IDS:
        if run_id not in content:
            errors.append(WorkflowShapeError(f"Workflow must use deterministic run-id: {run_id}"))

    return errors


def run_self_test() -> bool:
    """Run self-test on the workflow shape verifier."""
    print("[verify-tovarisch-idle-heartbeat-smoke-workflow] SELF-TEST MODE\n")

    # Valid workflow fixture
    valid_fixture = """name: Test Smoke Workflow
on:
  workflow_dispatch:
jobs:
  smoke:
    runs-on: ubuntu-latest
    timeout-minutes: 20
    steps:
      - run: ./scripts/lab_tovarisch_idle_memory.sh --native-events --run-id heartbeat-native-enabled-smoke
      - run: ./scripts/lab_tovarisch_idle_memory.sh --native-events --disable-heartbeat --run-id heartbeat-native-disabled-smoke
      - run: python3 scripts/verify_idle_staircase_native_heartbeat_smoke.py --enabled X --disabled Y
      - uses: actions/upload-artifact@v4
        if: always()
"""

    # Invalid: has push trigger
    invalid_push_fixture = """name: Test Smoke Workflow
on:
  workflow_dispatch:
  push:
jobs:
  smoke:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/upload-artifact@v4
        if: always()
"""

    # Invalid: no workflow_dispatch
    invalid_no_dispatch_fixture = """name: Test Smoke Workflow
on:
  push:
jobs:
  smoke:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/upload-artifact@v4
        if: always()
"""

    tests_passed = 0
    tests_failed = 0

    with tempfile.TemporaryDirectory() as tmpdir:
        tmppath = Path(tmpdir)

        # Test 1: Valid workflow passes
        print("Test 1: Valid workflow passes shape checks")
        valid_wf = tmppath / "valid.yml"
        valid_wf.write_text(valid_fixture)
        errors = check_workflow_shape(valid_wf)
        if not errors:
            print("  PASS")
            tests_passed += 1
        else:
            print(f"  FAIL: {errors}")
            tests_failed += 1

        # Test 2: Workflow with push trigger fails
        print("Test 2: Workflow with push trigger fails")
        invalid_wf = tmppath / "invalid_push.yml"
        invalid_wf.write_text(invalid_push_fixture)
        errors = check_workflow_shape(invalid_wf)
        if errors and any("push" in str(e) for e in errors):
            print("  PASS (correctly rejected)")
            tests_passed += 1
        else:
            print(f"  FAIL: expected push rejection, got {errors}")
            tests_failed += 1

        # Test 3: Workflow without workflow_dispatch fails
        print("Test 3: Workflow without workflow_dispatch fails")
        invalid_wf2 = tmppath / "invalid_no_dispatch.yml"
        invalid_wf2.write_text(invalid_no_dispatch_fixture)
        errors = check_workflow_shape(invalid_wf2)
        if errors and any("workflow_dispatch" in str(e) for e in errors):
            print("  PASS (correctly rejected)")
            tests_passed += 1
        else:
            print(f"  FAIL: expected workflow_dispatch rejection, got {errors}")
            tests_failed += 1

        # Test 4: Missing file fails
        print("Test 4: Missing workflow file fails")
        missing_wf = tmppath / "missing.yml"
        errors = check_workflow_shape(missing_wf)
        if errors and "not found" in str(errors[0]):
            print("  PASS (correctly rejected)")
            tests_passed += 1
        else:
            print(f"  FAIL: expected not-found rejection, got {errors}")
            tests_failed += 1

    print()
    print(f"Results: {tests_passed} passed, {tests_failed} failed")
    return tests_failed == 0


def main():
    parser = argparse.ArgumentParser(
        description="Verify tovarisch idle heartbeat smoke workflow shape"
    )
    parser.add_argument("--self-test", action="store_true", help="Run self-test")
    args = parser.parse_args()

    if args.self_test:
        success = run_self_test()
        sys.exit(0 if success else 1)

    script_dir = Path(__file__).parent
    workflows_dir = script_dir.parent / ".github" / "workflows"
    workflow_path = workflows_dir / WORKFLOW_FILE

    print(f"[verify-tovarisch-idle-heartbeat-smoke-workflow] checking {WORKFLOW_FILE}\n")

    errors = check_workflow_shape(workflow_path)
    if errors:
        for err in errors:
            print(f"  {err}", file=sys.stderr)
        print(f"\n[verify-tovarisch-idle-heartbeat-smoke-workflow] FAIL: {len(errors)} shape error(s)", file=sys.stderr)
        sys.exit(1)

    print(f"[verify-tovarisch-idle-heartbeat-smoke-workflow] PASS: Workflow shape valid")
    sys.exit(0)


if __name__ == "__main__":
    main()
