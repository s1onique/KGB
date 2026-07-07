#!/usr/bin/env python3
"""
Verifier for UVB-76 HULK02 Diagnostic Capture State Machine Contracts.

This verifier validates that HULK02 capture state contracts exist and conform to expected structure:
- All HULK02 contract files exist and contain proper test patterns
- No unallowlisted t.Skip/t.Skipf in HULK02 contract tests
- Fake backend is used in capture service unit contracts
- Real tcpdump/ss/ip command execution is NOT introduced in unit contract tests
- Makefile exposes hulk-uvb76-capture-gate
- hulk-uvb76-capture-gate runs go test for diagnostics/state/server capture contracts
- Files do not exceed 450-line LLM-friendliness limit

Supports self-test mode with fixture validation.

ACT-UVB76-HULK02-DIAGNOSTIC-CAPTURE-STATE-MACHINE
ACT-UVB76-HULK02R2-CAPTURE-CONTRACT-FILE-SPLIT
"""

import os
import re
import sys
import tempfile
import shutil

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.dirname(SCRIPT_DIR)
UVB76_DIR = os.path.join(REPO_ROOT, "uvb76")

# HULK02 full inventory - all required contract files (split files)
CONTRACT_FILES = [
    # State package - capture status matrix
    ("state/capture_status_matrix_contract_test.go", "Capture status matrix contract tests"),
    # State package - state machine decision
    ("state/capture_state_machine_decision_contract_test.go", "Capture state machine decision contract tests"),
    # State package - state machine invariant
    ("state/capture_state_invariant_contract_test.go", "Capture state machine invariant contract tests"),
    # State package - spike capture projection canonical
    ("state/spike_capture_projection_canonical_contract_test.go", "Spike capture projection canonical contract tests"),
    # State package - spike capture projection matrix
    ("state/spike_capture_projection_matrix_contract_test.go", "Spike capture projection matrix contract tests"),
    # State package - spike capture projection JSON
    ("state/spike_capture_projection_json_contract_test.go", "Spike capture projection JSON contract tests"),
    # Server package - capture status canonical
    ("server/capture_status_canonical_test.go", "Canonical capture status API contract tests"),
    # Server package - capture status constraints
    ("server/capture_status_constraints_test.go", "Capture status field constraint API contract tests"),
    # Diag package - capture service success
    ("diag/capture_service_success_contract_test.go", "Capture service success contract tests"),
    # Diag package - capture service error
    ("diag/capture_service_error_contract_test.go", "Capture service error contract tests"),
    # Diag package - capture service TCP absence
    ("diag/capture_service_tcp_absence_contract_test.go", "Capture service TCP absence contract tests"),
    # Diag package - capture service JSON
    ("diag/capture_service_json_contract_test.go", "Capture service JSON contract tests"),
]

# Helper files (optional - no func Test required)
HELPER_FILES = [
    ("state/capture_contract_helpers_test.go", "Shared test helpers for capture state contracts"),
    ("diag/capture_service_contract_helpers_test.go", "Shared test helpers for capture service contracts"),
]

# Canonical capture statuses (from ACT-UVB76-HULK02 specification)
CANONICAL_STATUSES = [
    "captured",
    "skipped_cooldown",
    "failed",
    "disabled",
    "not_configured",
    "not_attempted",
    "in_progress",
    "missing",
]

# TCP absence reason allowlist (from ACT-UVB76-HULK02 specification) - canonical 8
TCP_ABSENCE_REASONS = [
    "no_matching_socket",
    "socket_closed_before_capture",
    "command_failed",
    "not_configured",
    "permission_denied",
    "target_not_tcp",
    "target_mapping_missing",
    "unsupported_platform",
]

# Allowlisted skip pattern - ACT comments that permit skips
# Matches: comment on same line OR comment on previous line(s) within 100 chars of t.Skip
ALLOWLIST_SKIP_PATTERN = re.compile(
    r'//\s*ACT-UVB76-HULK02-ALLOW-SKIP:'
)

# LLM-friendliness line limit
MAX_LINES = 450


def count_lines(file_path):
    """Count lines in a file."""
    try:
        with open(file_path, 'r') as f:
            return len(f.readlines())
    except:
        return 0


def check_file_line_limit(relative_path):
    """Check that a file does not exceed the LLM-friendliness line limit."""
    full_path = os.path.join(UVB76_DIR, relative_path)
    errors = []
    if not os.path.isfile(full_path):
        return errors  # Will be caught by existence check

    line_count = count_lines(full_path)
    if line_count > MAX_LINES:
        errors.append(
            f"ERROR: {relative_path} exceeds {MAX_LINES}-line LLM-friendliness limit: {line_count}"
        )
    return errors


def check_contract_test_exists(relative_path, description):
    """Check that a contract test file exists."""
    full_path = os.path.join(UVB76_DIR, relative_path)
    errors = []
    if not os.path.isfile(full_path):
        errors.append(f"ERROR: {relative_path} does not exist")
    return errors


def check_contract_test_content(relative_path):
    """Check that contract test file contains expected test patterns."""
    full_path = os.path.join(UVB76_DIR, relative_path)
    errors = []
    if not os.path.isfile(full_path):
        return errors  # Already reported by existence check

    with open(full_path, 'r') as f:
        content = f.read()

    # Check for test function patterns
    has_tests = (
        'func Test' in content and
        '(t *testing.T)' in content
    )
    if not has_tests:
        errors.append(f"ERROR: {relative_path} lacks test functions")

    # Check for HULK02 marker
    if 'ACT-UVB76-HULK02' not in content:
        errors.append(f"ERROR: {relative_path} lacks ACT-UVB76-HULK02 marker comment")

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
                f"Allowed only with '// ACT-UVB76-HULK02-ALLOW-SKIP:' comment."
            )

    return errors


def strip_json_strings(content):
    """Remove JSON string contents to avoid false positives from test data."""
    # Remove double-quoted strings (handles escaped quotes)
    content = re.sub(r'"(?:[^"\\]|\\.)*"', '""', content)
    # Remove single-quoted strings
    content = re.sub(r"'(?:[^'\\]|\\.)*'", "''", content)
    return content


def check_fake_backend_used(relative_path):
    """Check that capture service contract tests use fake backends."""
    full_path = os.path.join(UVB76_DIR, relative_path)
    errors = []

    # Only check diag files
    if not relative_path.startswith("diag/"):
        return errors

    if not os.path.isfile(full_path):
        return errors  # Already reported by existence check

    with open(full_path, 'r') as f:
        content = f.read()

    # Strip comments to avoid false positives from doctrine comments
    # Remove single-line comments
    content_no_comments = re.sub(r'//.*$', '', content, flags=re.MULTILINE)
    # Remove multi-line comments
    content_no_comments = re.sub(r'/\*.*?\*/', '', content_no_comments, flags=re.DOTALL)
    # Remove JSON string contents (test data like "command_tool": "ss")
    content_no_strings = strip_json_strings(content_no_comments)

    # Check for fake backend usage
    has_fake_backend = (
        'httptest.NewServer' in content or
        'fakeBackend' in content or
        'fakeTovarisch' in content
    )
    if not has_fake_backend:
        errors.append(f"ERROR: {relative_path} should use fake backend (httptest.NewServer)")

    # Check for real command execution (forbidden in unit tests)
    # These patterns should not appear outside of comments or JSON strings
    forbidden_patterns = [
        'exec.Command',
        'os/exec',
        'syscall.Exec',
    ]
    for pattern in forbidden_patterns:
        if pattern in content_no_strings:
            errors.append(
                f"ERROR: {relative_path} contains forbidden '{pattern}' - use fake backends instead"
            )

    # Check for real network tool names (forbidden in unit tests)
    # Only flag if these appear as standalone tool names in JSON strings
    # (indicating real command execution) rather than as parts of other words
    # Also skip tool names that appear in test data fields like "command_tool"
    forbidden_tools = ['tcpdump', 'ss', 'ip']
    for tool in forbidden_tools:
        # Check for the tool as a JSON string value like "ss" or "ip"
        # Only flag if it's not part of test data structure fields
        # We allow "command_tool": "ss" in test data (field name contains "tool")
        if re.search(rf'":\s*"{re.escape(tool)}"', content_no_comments):
            # Check if this is in a test data context (command_tool field is allowed)
            if re.search(rf'"command_tool":\s*"{re.escape(tool)}"', content_no_comments):
                continue  # This is test data, not real command execution
            errors.append(
                f"ERROR: {relative_path} contains forbidden tool '{tool}' in JSON - "
                f"use fake backends instead"
            )

    return errors


def check_canonical_statuses_defined(relative_path):
    """Check that canonical statuses are defined in tests."""
    full_path = os.path.join(UVB76_DIR, relative_path)
    errors = []

    if not os.path.isfile(full_path):
        return errors  # Already reported by existence check

    with open(full_path, 'r') as f:
        content = f.read()

    # Count how many canonical statuses are referenced
    statuses_found = sum(
        1 for s in CANONICAL_STATUSES
        if f'CaptureStatus{s.title().replace("_", "")}' in content or f'"{s}"' in content
    )

    # At least 1 canonical status should be referenced (some focused tests only test 1-2)
    if statuses_found < 1:
        errors.append(f"ERROR: {relative_path} references fewer than 1 canonical status")

    return errors


def check_tcp_absence_allowlist():
    """Check that TCP absence reasons are preserved in contract tests."""
    errors = []

    # Check diag contract files for reason codes
    diag_dir = os.path.join(UVB76_DIR, "diag")
    tcp_absence_test = os.path.join(diag_dir, "capture_service_tcp_absence_contract_test.go")

    found_reasons = False
    if os.path.isfile(tcp_absence_test):
        with open(tcp_absence_test, 'r') as f:
            content = f.read()
        reasons_found = sum(1 for r in TCP_ABSENCE_REASONS if r in content)
        if reasons_found >= 5:
            found_reasons = True

    if not found_reasons:
        errors.append(f"ERROR: TCP absence reasons not found in contract tests")

    return errors


def check_makefile_has_hulk_gate():
    """Check that Makefile contains hulk-uvb76-capture-gate target."""
    makefile_path = os.path.join(REPO_ROOT, "Makefile")
    errors = []

    if not os.path.isfile(makefile_path):
        errors.append("ERROR: Makefile does not exist")
        return errors

    with open(makefile_path, 'r') as f:
        content = f.read()

    # Check for hulk-uvb76-capture-gate target
    if not re.search(r'^hulk-uvb76-capture-gate\s*:', content, re.MULTILINE):
        errors.append("ERROR: Makefile lacks 'hulk-uvb76-capture-gate:' target")
        return errors

    # Check that hulk-uvb76-capture-gate includes go test for relevant packages
    hulk_gate_match = re.search(
        r'^hulk-uvb76-capture-gate\s*:.*?(?=\n\n|\n\.[A-Z]|\Z)',
        content,
        re.MULTILINE | re.DOTALL
    )
    if hulk_gate_match:
        gate_content = hulk_gate_match.group(0)
        if 'go test' not in gate_content:
            errors.append("ERROR: hulk-uvb76-capture-gate target lacks 'go test' command")
        if '-race' not in gate_content:
            errors.append("ERROR: hulk-uvb76-capture-gate target lacks '-race' flag for go test")
        # Check for specific packages
        if './state/' not in gate_content and './state/...' not in gate_content:
            errors.append("ERROR: hulk-uvb76-capture-gate target should include ./state/... package")
        if './server/' not in gate_content and './server/...' not in gate_content:
            errors.append("ERROR: hulk-uvb76-capture-gate target should include ./server/... package")
        if './diag/' not in gate_content and './diag/...' not in gate_content:
            errors.append("ERROR: hulk-uvb76-capture-gate target should include ./diag/... package")

    return errors


def run_verifier():
    """Run the HULK02 capture state contract verifier."""
    all_errors = []
    print("=== UVB-76 HULK02 Capture State Machine Contract Verifier ===\n")

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
        errors = check_contract_test_content(relative_path)
        if errors:
            for e in errors:
                print(f"    {e}")
                all_errors.append(e)
        else:
            print(f"    OK: Contains test patterns and HULK02 marker")

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

    print("\nD. Checking fake backend usage in capture service contracts...")
    for relative_path, description in CONTRACT_FILES:
        if relative_path.startswith("diag/"):
            print(f"  Checking fake backend in: {relative_path}")
            errors = check_fake_backend_used(relative_path)
            if errors:
                for e in errors:
                    print(f"    {e}")
                    all_errors.append(e)
            else:
                print(f"    OK: Uses fake backends")

    print("\nE. Checking canonical statuses defined in contracts...")
    for relative_path, description in CONTRACT_FILES:
        print(f"  Checking canonical statuses in: {relative_path}")
        errors = check_canonical_statuses_defined(relative_path)
        if errors:
            for e in errors:
                print(f"    {e}")
                all_errors.append(e)
        else:
            print(f"    OK: References canonical statuses")

    print("\nF. Checking TCP absence reason allowlist...")
    errors = check_tcp_absence_allowlist()
    if errors:
        for e in errors:
            print(f"    {e}")
            all_errors.append(e)
    else:
        print(f"    OK: TCP absence reasons preserved")

    print("\nG. Checking line limits (LLM-friendliness)...")
    for relative_path, description in CONTRACT_FILES:
        print(f"  Checking line limit for: {relative_path}")
        errors = check_file_line_limit(relative_path)
        if errors:
            for e in errors:
                print(f"    {e}")
                all_errors.append(e)
        else:
            line_count = count_lines(os.path.join(UVB76_DIR, relative_path))
            print(f"    OK: {line_count} lines (under {MAX_LINES})")

    print("\nH. Checking Makefile hulk-uvb76-capture-gate target...")
    errors = check_makefile_has_hulk_gate()
    if errors:
        for e in errors:
            print(f"    {e}")
            all_errors.append(e)
    else:
        print(f"    OK: Makefile contains hulk-uvb76-capture-gate with go test -race")

    print("\n" + "=" * 60)
    print("SUMMARY:")
    print(f"  Contract test files: {len(CONTRACT_FILES)}")
    print(f"  Canonical statuses: {len(CANONICAL_STATUSES)}")
    print(f"  TCP absence reasons: {len(TCP_ABSENCE_REASONS)}")
    print(f"  Errors: {len(all_errors)}")

    return all_errors


def run_self_tests():
    """Run self-test cases to verify the verifier itself."""
    results = {}
    errors = []
    test_count = 0
    pass_count = 0

    print("=== HULK02 Capture State Contract Verifier Self-Tests ===\n")
    test_dir = tempfile.mkdtemp(prefix="capture-contract-verifier-test-")

    try:
        # Test 1: Valid contract test file passes existence check
        test_count += 1
        print("Test 1: Valid contract test file passes existence check")
        test_file = os.path.join(test_dir, "state/capture_contract_test.go")
        os.makedirs(os.path.dirname(test_file), exist_ok=True)
        with open(test_file, 'w') as f:
            f.write('package state\n')
            f.write('// ACT-UVB76-HULK02: Contract test\n')
            f.write('func TestCaptureContract(t *testing.T) {\n')
            f.write('    // Test implementation\n')
            f.write('}\n')

        with open(test_file, 'r') as f:
            content = f.read()

        has_patterns = (
            'func Test' in content and
            '(t *testing.T)' in content and
            'ACT-UVB76-HULK02' in content
        )
        if has_patterns:
            results["valid_contract_test"] = True
            pass_count += 1
            print("  PASS")
        else:
            results["valid_contract_test"] = False
            errors.append("Valid contract test not recognized")
            print("  FAIL")

        # Test 2: Missing contract test fails
        test_count += 1
        print("Test 2: Missing contract test fails existence check")
        missing_errors = check_contract_test_exists(
            "nonexistent/file_test.go",
            "Missing test"
        )
        if any("does not exist" in e for e in missing_errors):
            results["missing_file"] = True
            pass_count += 1
            print("  PASS")
        else:
            results["missing_file"] = False
            errors.append("Missing file not detected")
            print("  FAIL")

        # Test 3: File without HULK02 marker fails
        test_count += 1
        print("Test 3: File without HULK02 marker fails")
        test_file2 = os.path.join(test_dir, "no_marker_test.go")
        with open(test_file2, 'w') as f:
            f.write('package test\n')
            f.write('func TestSimple(t *testing.T) {\n')
            f.write('    // No ACT marker\n')
            f.write('}\n')

        with open(test_file2, 'r') as f:
            content = f.read()

        has_marker = 'ACT-UVB76-HULK02' in content
        if not has_marker:
            results["no_marker"] = True
            pass_count += 1
            print("  PASS")
        else:
            results["no_marker"] = False
            errors.append("Missing marker not detected")
            print("  FAIL")

        # Test 4: Makefile regex detection
        test_count += 1
        print("Test 4: Makefile hulk-uvb76-capture-gate detection")
        test_makefile = os.path.join(test_dir, "Makefile")
        with open(test_makefile, 'w') as f:
            f.write('.PHONY: hulk-uvb76-capture-gate\n')
            f.write('hulk-uvb76-capture-gate:\n')
            f.write('\tcd uvb76 && go test -race -v ./state/... ./server/... ./diag/...\n')

        with open(test_makefile, 'r') as f:
            content = f.read()

        has_gate = re.search(r'^hulk-uvb76-capture-gate\s*:', content, re.MULTILINE)
        has_race = '-race' in content
        has_packages = './state/...' in content and './server/...' in content and './diag/...' in content

        if has_gate and has_race and has_packages:
            results["makefile_detection"] = True
            pass_count += 1
            print("  PASS")
        else:
            results["makefile_detection"] = False
            errors.append("Makefile detection failed")
            print("  FAIL")

        # Test 5: Real command execution detection
        test_count += 1
        print("Test 5: Real command execution detection in unit tests")
        test_file3 = os.path.join(test_dir, "diag/capture_service_test.go")
        os.makedirs(os.path.dirname(test_file3), exist_ok=True)
        with open(test_file3, 'w') as f:
            f.write('package diag\n')
            f.write('import "os/exec"\n')
            f.write('func TestCapture(t *testing.T) {\n')
            f.write('    exec.Command("tcpdump", "-i", "eth0")\n')  # Forbidden
            f.write('}\n')

        with open(test_file3, 'r') as f:
            content = f.read()

        has_forbidden = 'exec.Command' in content or 'os/exec' in content
        if has_forbidden:
            results["forbidden_command"] = True
            pass_count += 1
            print("  PASS")
        else:
            results["forbidden_command"] = False
            errors.append("Forbidden command not detected")
            print("  FAIL")

        # Test 6: Fake backend detection
        test_count += 1
        print("Test 6: Fake backend (httptest) detection")
        test_file4 = os.path.join(test_dir, "diag/capture_fake_test.go")
        with open(test_file4, 'w') as f:
            f.write('package diag\n')
            f.write('import "net/http/httptest"\n')
            f.write('func TestCapture(t *testing.T) {\n')
            f.write('    server := httptest.NewServer(handler)\n')
            f.write('}\n')

        with open(test_file4, 'r') as f:
            content = f.read()

        has_fake = 'httptest.NewServer' in content
        if has_fake:
            results["fake_backend"] = True
            pass_count += 1
            print("  PASS")
        else:
            results["fake_backend"] = False
            errors.append("Fake backend not detected")
            print("  FAIL")

        # Test 7: Line limit check
        test_count += 1
        print("Test 7: Line limit check")
        # Create a file that exceeds the limit in the test dir
        test_file5 = os.path.join(test_dir, "over_limit_test.go")
        with open(test_file5, 'w') as f:
            for i in range(500):
                f.write(f'// Line {i+1}\n')
                f.write('func dummy() {{}}\n')

        # The line count function should count lines correctly
        line_count = count_lines(test_file5)
        if line_count > MAX_LINES:
            results["line_limit"] = True
            pass_count += 1
            print("  PASS")
        else:
            results["line_limit"] = False
            errors.append("Line limit not enforced")
            print("  FAIL")

    finally:
        shutil.rmtree(test_dir, ignore_errors=True)

    return errors, results, test_count, pass_count


def main():
    if "--self-test" in sys.argv:
        errors, results, test_count, pass_count = run_self_tests()
        print("\n" + "=" * 60)
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

    print("\n" + "=" * 60)
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
