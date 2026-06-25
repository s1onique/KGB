#!/usr/bin/env python3
# verify_memory_matrix_workflow_shape.py — Workflow-shape verifier for memory attribution matrix
#
# Verifies that the long-running memory attribution matrix workflow is correctly gated
# as manual-only and does not accidentally get wired into push/PR/schedule triggers.
#
# This is NOT part of the long matrix lab itself — it's a cheap shape verification
# that runs in the normal quality gate.

import sys
import re
from pathlib import Path


def verify_workflow_shape(workflow_path: Path) -> tuple[bool, str]:
    """
    Verify memory attribution matrix workflow has correct shape.
    
    Returns: (valid, error_message)
    """
    if not workflow_path.exists():
        return False, f"Workflow not found: {workflow_path}"
    
    content = workflow_path.read_text()
    
    errors = []
    warnings = []
    
    # Extract the "on:" block to check triggers
    on_match = re.search(r'^on:\s*\n((?:.*\n)*?)(?:jobs:|$)', content, re.MULTILINE)
    on_block = on_match.group(1) if on_match else ""
    
    # Check: workflow has workflow_dispatch
    has_workflow_dispatch = "workflow_dispatch" in on_block
    if not has_workflow_dispatch:
        errors.append("Missing workflow_dispatch trigger")
    
    # Check: workflow does NOT have push trigger
    if re.search(r'^\s*push\s*:', on_block, re.MULTILINE):
        errors.append("workflow has push trigger (should be workflow_dispatch only)")
    
    # Check: workflow does NOT have pull_request trigger
    if re.search(r'^\s*pull_request\s*:', on_block, re.MULTILINE):
        errors.append("workflow has pull_request trigger (should be workflow_dispatch only)")
    
    # Check: workflow does NOT have schedule trigger
    if re.search(r'^\s*schedule\s*:', on_block, re.MULTILINE):
        errors.append("workflow has schedule trigger (should be workflow_dispatch only)")
    
    # Check: job runs on Linux
    if not re.search(r'runs-on:.*ubuntu', content):
        errors.append("Job does not run on ubuntu (Linux required for /proc)")
    
    # Check: timeout is bounded
    timeout_match = re.search(r'timeout-minutes:\s*(\d+)', content)
    if not timeout_match:
        errors.append("Missing timeout-minutes")
    else:
        timeout = int(timeout_match.group(1))
        if timeout < 60:
            errors.append(f"Timeout too short: {timeout} minutes (need at least 60)")
        if timeout > 180:
            warnings.append(f"Timeout is {timeout} minutes (long-running lab)")
    
    # Check: preflight runs syntax/compile checks
    if not re.search(r'bash -n|py_compile', content):
        warnings.append("No preflight syntax checks found")
    
    # Check: verifier self-test step exists
    if not re.search(r'verify_memory_attribution_matrix.*--self-test', content):
        errors.append("Missing verifier self-test step")
    
    # Check: command execution uses Bash array, not eval (best practice)
    if re.search(r'eval.*\$\{?cmd', content):
        errors.append("Uses eval with cmd variable (should use array expansion)")
    
    # Check: artifact upload uses if: always()
    upload_match = re.search(r'actions/upload-artifact', content)
    if upload_match:
        # Find the upload step context
        upload_pos = upload_match.start()
        context = content[max(0, upload_pos-500):upload_pos]
        if 'if: always()' not in context and 'always()' not in content[upload_pos:upload_pos+100]:
            # Check if there's an if condition near the upload
            if 'if:' in content[upload_pos:upload_pos+200] and 'always' not in content[upload_pos:upload_pos+200]:
                warnings.append("Artifact upload may not use if: always()")
    
    # Check: artifact upload has bounded retention-days
    retention_match = re.search(r'retention-days:\s*(\d+)', content)
    if retention_match:
        retention = int(retention_match.group(1))
        if retention > 90:
            warnings.append(f"Artifact retention is {retention} days (consider limiting to 30)")
    else:
        warnings.append("No retention-days specified for artifact upload")
    
    # Build result
    if errors:
        return False, "; ".join(errors)
    
    if warnings:
        return True, "; ".join(warnings)
    
    return True, ""


def run_self_tests():
    """Run self-tests for the workflow shape verifier."""
    print("=== Memory Matrix Workflow Shape Verifier Self-Tests ===\n")
    
    import tempfile
    tests_passed = 0
    tests_failed = 0
    
    with tempfile.TemporaryDirectory() as tmpdir:
        tmppath = Path(tmpdir)
        
        # Test 1: Valid workflow passes
        print("Test 1: Valid workflow passes")
        valid_workflow = """
name: Test Matrix
on:
  workflow_dispatch:
jobs:
  test:
    runs-on: ubuntu-latest
    timeout-minutes: 120
    steps:
      - name: Preflight
        run: bash -n script.sh && python3 -m py_compile script.py
      - name: Self-test
        run: python3 scripts/verify_memory_attribution_matrix.py --self-test
      - name: Upload
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: test
          path: artifacts/**
          retention-days: 30
"""
        wf_path = tmppath / "valid.yml"
        wf_path.write_text(valid_workflow)
        valid, error = verify_workflow_shape(wf_path)
        if valid:
            print("  PASS")
            tests_passed += 1
        else:
            print(f"  FAIL: {error}")
            tests_failed += 1
        
        # Test 2: Missing workflow_dispatch fails
        print("Test 2: Missing workflow_dispatch fails")
        bad_workflow = """
name: Test Matrix
on:
  push:
jobs:
  test:
    runs-on: ubuntu-latest
    timeout-minutes: 120
    steps:
      - name: Preflight
        run: bash -n script.sh && python3 -m py_compile script.py
      - name: Self-test
        run: python3 scripts/verify_memory_attribution_matrix.py --self-test
      - name: Upload
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: test
          path: artifacts/**
          retention-days: 30
"""
        wf_path = tmppath / "no_dispatch.yml"
        wf_path.write_text(bad_workflow)
        valid, error = verify_workflow_shape(wf_path)
        if not valid and "workflow_dispatch" in error:
            print("  PASS (correctly rejected)")
            tests_passed += 1
        else:
            print(f"  FAIL: expected rejection, got valid={valid}, error={error}")
            tests_failed += 1
        
        # Test 3: Has push trigger fails
        print("Test 3: Has push trigger fails")
        push_workflow = """
name: Test Matrix
on:
  workflow_dispatch:
  push:
jobs:
  test:
    runs-on: ubuntu-latest
    timeout-minutes: 120
    steps:
      - name: Preflight
        run: bash -n script.sh && python3 -m py_compile script.py
      - name: Self-test
        run: python3 scripts/verify_memory_attribution_matrix.py --self-test
      - name: Upload
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: test
          path: artifacts/**
          retention-days: 30
"""
        wf_path = tmppath / "has_push.yml"
        wf_path.write_text(push_workflow)
        valid, error = verify_workflow_shape(wf_path)
        if not valid and "push" in error:
            print("  PASS (correctly rejected)")
            tests_passed += 1
        else:
            print(f"  FAIL: expected rejection, got valid={valid}, error={error}")
            tests_failed += 1
        
        # Test 4: Missing self-test step fails
        print("Test 4: Missing self-test step fails")
        no_selftest = """
name: Test Matrix
on:
  workflow_dispatch:
jobs:
  test:
    runs-on: ubuntu-latest
    timeout-minutes: 120
    steps:
      - run: echo hello
      - uses: actions/upload-artifact@v4
        with:
          name: test
          path: artifacts/**
          retention-days: 30
"""
        wf_path = tmppath / "no_selftest.yml"
        wf_path.write_text(no_selftest)
        valid, error = verify_workflow_shape(wf_path)
        if not valid and "self-test" in error:
            print("  PASS (correctly rejected)")
            tests_passed += 1
        else:
            print(f"  FAIL: expected rejection, got valid={valid}, error={error}")
            tests_failed += 1
        
        # Test 5: Missing retention-days generates warning
        print("Test 5: Missing retention-days generates warning")
        no_retention = """
name: Test Matrix
on:
  workflow_dispatch:
jobs:
  test:
    runs-on: ubuntu-latest
    timeout-minutes: 120
    steps:
      - name: Preflight
        run: bash -n script.sh && python3 -m py_compile script.py
      - name: Self-test
        run: python3 scripts/verify_memory_attribution_matrix.py --self-test
      - uses: actions/upload-artifact@v4
        with:
          name: test
          path: artifacts/**
"""
        wf_path = tmppath / "no_retention.yml"
        wf_path.write_text(no_retention)
        valid, error = verify_workflow_shape(wf_path)
        if valid and "retention" in error.lower():
            print("  PASS (warning generated)")
            tests_passed += 1
        else:
            print(f"  FAIL: expected warning about retention, got valid={valid}, error={error}")
            tests_failed += 1
    
    print(f"\nResults: {tests_passed} passed, {tests_failed} failed")
    return tests_failed == 0


def main():
    import argparse
    
    parser = argparse.ArgumentParser(
        description="Verify memory attribution matrix workflow shape"
    )
    parser.add_argument(
        "--workflow",
        default=".github/workflows/tovarisch-idle-memory-attribution-matrix.yml",
        help="Path to workflow file (default: standard location)"
    )
    parser.add_argument(
        "--self-test",
        action="store_true",
        help="Run self-tests"
    )
    
    args = parser.parse_args()
    
    if args.self_test:
        success = run_self_tests()
        sys.exit(0 if success else 1)
    
    workflow_path = Path(args.workflow)
    valid, error = verify_workflow_shape(workflow_path)
    
    print(f"Workflow: {workflow_path}")
    print(f"Valid: {valid}")
    
    if error:
        if valid:
            print(f"Warnings: {error}")
        else:
            print(f"Errors: {error}")
    
    sys.exit(0 if valid else 1)


if __name__ == "__main__":
    main()
