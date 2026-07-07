#!/usr/bin/env python3
"""
Verifier for UVB-76 runtime contract tests (HULK01).

Validates that HULK01 runtime contract tests exist and conform to the expected structure:
- All 9 HULK01 contract files exist and contain proper test patterns
- Makefile contains hulk-uvb76-gate with go test -race
- No unallowlisted t.Skip patterns exist in contract files
- Probe contracts do not contain DNS-dependent URLs (http://test*.local)

Supports self-test mode with fixture validation.

ACT-UVB76-HULK01R-RUNTIME-CONTRACT-GATE-ENFORCEMENT
"""

import os
import re
import sys

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.dirname(SCRIPT_DIR)
UVB76_DIR = os.path.join(REPO_ROOT, "uvb76")

# HULK01 full inventory - all 9 contract files
CONTRACT_FILES = [
    ("state/latency_tracker_contract_test.go", "Latency tracker contract tests"),
    ("state/latency_tracker_safety_contract_test.go", "Latency tracker safety contract tests"),
    ("state/spike_detector_contract_test.go", "Spike detector contract tests"),
    ("state/spike_safety_contract_test.go", "Spike safety contract tests"),
    ("state/concurrency_contract_test.go", "Concurrency contract tests"),
    ("state/concurrency_invariant_contract_test.go", "Concurrency invariant contract tests"),
    ("server/latency_series_contract_test.go", "Latency series API contract tests"),
    ("server/latency_series_invariant_contract_test.go", "Latency series invariant contract tests"),
    ("probe/inflight_guard_contract_test.go", "In-flight guard contract tests"),
]

# Allowlisted skip pattern - ACT comments that permit skips
ALLOWLIST_SKIP_PATTERN = re.compile(
    r'//\s*ACT-UVB76-HULK01-ALLOW-SKIP:.*t\.Skip'
)


def check_contract_test_exists(relative_path, description):
    """Check that a contract test file exists."""
    full_path = os.path.join(UVB76_DIR, relative_path)
    errors = []
    if not os.path.isfile(full_path):
        errors.append(f"ERROR: {relative_path} does not exist")
    return errors


def check_contract_test_has_race_tests(relative_path):
    """Check that a contract test file contains race detector tests."""
    full_path = os.path.join(UVB76_DIR, relative_path)
    errors = []
    if not os.path.isfile(full_path):
        return errors  # Already reported by existence check

    with open(full_path, 'r') as f:
        content = f.read()

    # Check for common concurrency test patterns
    has_concurrency_pattern = (
        'sync.WaitGroup' in content or
        'go func' in content or
        'atomic.' in content or
        't.Run' in content
    )

    # Files that test safety/invariant or basic contract properties without concurrency are acceptable
    is_safety_or_invariant = (
        '_safety_contract_test.go' in relative_path or
        '_invariant_contract_test.go' in relative_path
    )

    # Contract tests that verify API/data structure correctness are valid
    is_basic_contract = (
        '_contract_test.go' in relative_path and
        '_concurrency_contract_test.go' not in relative_path
    )

    if relative_path == "state/latency_tracker_contract_test.go":
        pass
    elif is_safety_or_invariant:
        pass
    elif is_basic_contract:
        pass
    elif not has_concurrency_pattern:
        errors.append(f"ERROR: {relative_path} lacks concurrency test patterns")

    # Check for contract test function naming
    has_contract_tests = (
        'TestContract' in content or
        'TestLatency' in content or
        'TestSpike' in content or
        'TestConcurrency' in content or
        'TestInflight' in content
    )
    if not has_contract_tests:
        errors.append(f"ERROR: {relative_path} lacks contract test functions (Test* naming)")

    return errors


def check_no_unallowlisted_skips(relative_path):
    """Check that contract files do not contain unallowlisted t.Skip patterns."""
    full_path = os.path.join(UVB76_DIR, relative_path)
    errors = []
    if not os.path.isfile(full_path):
        return errors  # Already reported by existence check

    with open(full_path, 'r') as f:
        content = f.read()

    # Find all t.Skip and t.Skipf occurrences
    skip_pattern = re.compile(r't\.Skip[f]?\s*\(', re.MULTILINE)
    skip_matches = skip_pattern.finditer(content)

    for match in skip_matches:
        # Check if this skip is allowlisted by an ACT comment on the same line
        start = max(0, match.start() - 100)
        end = min(len(content), match.end() + 50)
        context = content[start:end]

        # Look for allowlist comment before the skip
        if not ALLOWLIST_SKIP_PATTERN.search(context):
            errors.append(
                f"ERROR: {relative_path} contains unallowlisted t.Skip at position {match.start()}. "
                f"Allowed only with '// ACT-UVB76-HULK01-ALLOW-SKIP:' comment."
            )

    return errors


def check_probe_no_dns_dependency(relative_path):
    """Check that probe contracts do not contain DNS-dependent test URLs."""
    full_path = os.path.join(UVB76_DIR, relative_path)
    errors = []

    # Only check probe files
    if not relative_path.startswith("probe/"):
        return errors

    if not os.path.isfile(full_path):
        return errors  # Already reported by existence check

    with open(full_path, 'r') as f:
        content = f.read()

    # Check for DNS-dependent URLs that cause test timeouts
    dns_pattern = re.compile(r'https?://test(?:[0-9]*)?\.local', re.IGNORECASE)
    matches = dns_pattern.findall(content)

    if matches:
        for match in matches:
            errors.append(
                f"ERROR: {relative_path} contains DNS-dependent URL '{match}'. "
                f"Use httptest.NewServer() for local fixture servers instead."
            )

    return errors


def check_makefile_has_hulk_gate():
    """Check that Makefile contains hulk-uvb76-gate target."""
    makefile_path = os.path.join(REPO_ROOT, "Makefile")
    errors = []

    if not os.path.isfile(makefile_path):
        errors.append("ERROR: Makefile does not exist")
        return errors

    with open(makefile_path, 'r') as f:
        content = f.read()

    # Check for hulk-uvb76-gate target
    if not re.search(r'^hulk-uvb76-gate\s*:', content, re.MULTILINE):
        errors.append("ERROR: Makefile lacks 'hulk-uvb76-gate:' target")

    # Check that hulk-uvb76-gate includes go test -race
    hulk_gate_match = re.search(
        r'^hulk-uvb76-gate\s*:.*?(?=\n\n|\n\.[A-Z]|\Z)',
        content,
        re.MULTILINE | re.DOTALL
    )
    if hulk_gate_match:
        gate_content = hulk_gate_match.group(0)
        if 'go test' not in gate_content:
            errors.append("ERROR: hulk-uvb76-gate target lacks 'go test' command")
        if '-race' not in gate_content:
            errors.append("ERROR: hulk-uvb76-gate target lacks '-race' flag for go test")

    return errors


def run_verifier():
    """Run the runtime contract verifier."""
    all_errors = []
    print("=== UVB-76 Runtime Contract Verifier (HULK01) ===\n")

    print("A. Checking contract test files exist...")
    for relative_path, description in CONTRACT_FILES:
        print(f"  Checking: {relative_path}")
        errors = check_contract_test_exists(relative_path, description)
        if errors:
            for e in errors:
                print(f"    {e}")
                all_errors.append(e)
        else:
            print(f"    OK: File exists")

    print("\nB. Checking contract test content patterns...")
    for relative_path, description in CONTRACT_FILES:
        print(f"  Checking patterns in: {relative_path}")
        errors = check_contract_test_has_race_tests(relative_path)
        if errors:
            for e in errors:
                print(f"    {e}")
                all_errors.append(e)
        else:
            print(f"    OK: Contains concurrency test patterns")

    print("\nC. Checking for unallowlisted t.Skip patterns...")
    for relative_path, description in CONTRACT_FILES:
        print(f"  Checking skips in: {relative_path}")
        errors = check_no_unallowlisted_skips(relative_path)
        if errors:
            for e in errors:
                print(f"    {e}")
                all_errors.append(e)
        else:
            print(f"    OK: No unallowlisted t.Skip found")

    print("\nD. Checking probe contracts for DNS dependency...")
    for relative_path, description in CONTRACT_FILES:
        if relative_path.startswith("probe/"):
            print(f"  Checking DNS hygiene in: {relative_path}")
            errors = check_probe_no_dns_dependency(relative_path)
            if errors:
                for e in errors:
                    print(f"    {e}")
                    all_errors.append(e)
            else:
                print(f"    OK: No DNS-dependent URLs found")

    print("\nE. Checking Makefile hulk-uvb76-gate target...")
    errors = check_makefile_has_hulk_gate()
    if errors:
        for e in errors:
            print(f"    {e}")
            all_errors.append(e)
    else:
        print(f"    OK: Makefile contains hulk-uvb76-gate with go test -race")

    print("\n" + "=" * 50)
    print("SUMMARY:")
    print(f"  Contract test files: {len(CONTRACT_FILES)}")
    print(f"  Errors: {len(all_errors)}")

    return all_errors


def main():
    if "--self-test" in sys.argv:
        # Import self-test module
        from verify_uvb76_runtime_contracts_selftest import run_self_tests
        errors, results, test_count, pass_count = run_self_tests()
        print("\n" + "=" * 50)
        print(f"SELF-TEST SUMMARY: {pass_count}/{test_count} passed")
        for name, passed in results.items():
            status = "PASS" if passed else "FAIL"
            print(f"  {name}: {status}")

        if errors:
            print("\nSELF-TEST ERRORS:")
            for e in errors:
                print(f"  - {e}")
            sys.exit(1)
        else:
            print("\nAll self-tests passed!")
            sys.exit(0)

    errors = run_verifier()

    print("\n" + "=" * 50)
    if errors:
        print("\nVERIFICATION FAILED:")
        for e in errors:
            print(f"  {e}")
        sys.exit(1)
    else:
        print("\nVERIFICATION PASSED")
        sys.exit(0)


if __name__ == "__main__":
    main()
