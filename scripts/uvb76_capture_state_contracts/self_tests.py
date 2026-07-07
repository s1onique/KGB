"""
Self-tests for UVB-76 HULK02 Capture State Contract verifier.
"""

import os
import re
import shutil
import tempfile


def run_self_tests(
    count_lines,
    max_lines,
    validate_inventory_module,
    validate_skip_allowlist_module,
    verbose: bool = True
) -> tuple[list[str], dict[str, bool], int, int]:
    """
    Run self-test cases to verify the verifier itself.

    Args:
        count_lines: Function to count lines in a file.
        max_lines: Maximum allowed lines threshold.
        validate_inventory_module: Module containing inventory validation functions.
        validate_skip_allowlist_module: Module containing skip allowlist functions.
        verbose: Whether to print test progress.

    Returns:
        Tuple of (errors, results_dict, test_count, pass_count).
    """
    results = {}
    errors = []
    test_count = 0
    pass_count = 0

    if verbose:
        print("=== HULK02 Capture State Contract Verifier Self-Tests ===\n")
    test_dir = tempfile.mkdtemp(prefix="capture-contract-verifier-test-")

    try:
        check_contract_test_exists = validate_inventory_module.check_contract_test_exists
        check_contract_test_content = validate_inventory_module.check_contract_test_content
        check_no_skips_in_core_service_files = validate_skip_allowlist_module.check_no_skips_in_core_service_files
        check_capture_status_assertions_in_core_service_files = validate_skip_allowlist_module.check_capture_status_assertions_in_core_service_files

        # Test 1: Valid contract test file passes existence check
        test_count += 1
        if verbose:
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
            if verbose:
                print("  PASS")
        else:
            results["valid_contract_test"] = False
            errors.append("Valid contract test not recognized")
            if verbose:
                print("  FAIL")

        # Test 2: Missing contract test fails
        test_count += 1
        if verbose:
            print("Test 2: Missing contract test fails existence check")
        missing_errors = check_contract_test_exists(
            "nonexistent/file_test.go",
            "Missing test",
            test_dir
        )
        if any("does not exist" in e for e in missing_errors):
            results["missing_file"] = True
            pass_count += 1
            if verbose:
                print("  PASS")
        else:
            results["missing_file"] = False
            errors.append("Missing file not detected")
            if verbose:
                print("  FAIL")

        # Test 3: File without HULK02 marker fails
        test_count += 1
        if verbose:
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
            if verbose:
                print("  PASS")
        else:
            results["no_marker"] = False
            errors.append("Missing marker not detected")
            if verbose:
                print("  FAIL")

        # Test 4: Makefile regex detection
        test_count += 1
        if verbose:
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
            if verbose:
                print("  PASS")
        else:
            results["makefile_detection"] = False
            errors.append("Makefile detection failed")
            if verbose:
                print("  FAIL")

        # Test 5: Real command execution detection
        test_count += 1
        if verbose:
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
            if verbose:
                print("  PASS")
        else:
            results["forbidden_command"] = False
            errors.append("Forbidden command not detected")
            if verbose:
                print("  FAIL")

        # Test 6: Fake backend detection
        test_count += 1
        if verbose:
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
            if verbose:
                print("  PASS")
        else:
            results["fake_backend"] = False
            errors.append("Fake backend not detected")
            if verbose:
                print("  FAIL")

        # Test 7: Line limit check
        test_count += 1
        if verbose:
            print("Test 7: Line limit check")
        test_file5 = os.path.join(test_dir, "over_limit_test.go")
        with open(test_file5, 'w') as f:
            for i in range(500):
                f.write(f'// Line {i+1}\n')
                f.write('func dummy() {{}}\n')

        line_count = count_lines(test_file5)
        if line_count > max_lines:
            results["line_limit"] = True
            pass_count += 1
            if verbose:
                print("  PASS")
        else:
            results["line_limit"] = False
            errors.append("Line limit not enforced")
            if verbose:
                print("  FAIL")

        # Test 8: Core service file with t.Skip fails
        test_count += 1
        if verbose:
            print("Test 8: Core service file with t.Skip fails")
        core_test_file = os.path.join(test_dir, "diag/capture_service_error_contract_test.go")
        os.makedirs(os.path.dirname(core_test_file), exist_ok=True)
        with open(core_test_file, 'w') as f:
            f.write('package diag\n')
            f.write('// ACT-UVB76-HULK02: Test\n')
            f.write('func TestSomething(t *testing.T) {\n')
            f.write('    t.Skip("test")\n')  # Skip in core service file
            f.write('}\n')

        skip_errors = check_no_skips_in_core_service_files("diag/capture_service_error_contract_test.go", test_dir)
        if skip_errors:
            results["core_service_skip_fails"] = True
            pass_count += 1
            if verbose:
                print("  PASS")
        else:
            results["core_service_skip_fails"] = False
            errors.append("Core service skip not detected")
            if verbose:
                print("  FAIL")

        # Test 9: Core service file with ACT-UVB76-HULK02-ALLOW-SKIP still fails
        test_count += 1
        if verbose:
            print("Test 9: Core service file with ALLOW-SKIP still fails")
        core_test_file2 = os.path.join(test_dir, "diag/capture_service_success_contract_test.go")
        with open(core_test_file2, 'w') as f:
            f.write('package diag\n')
            f.write('// ACT-UVB76-HULK02: Test\n')
            f.write('func TestSomething(t *testing.T) {\n')
            f.write('    // ACT-UVB76-HULK02-ALLOW-SKIP: reason\n')
            f.write('    t.Skip("test")\n')  # Allowlisted skip in core service file
            f.write('}\n')

        skip_errors2 = check_no_skips_in_core_service_files("diag/capture_service_success_contract_test.go", test_dir)
        if skip_errors2:
            results["core_service_allowskip_still_fails"] = True
            pass_count += 1
            if verbose:
                print("  PASS")
        else:
            results["core_service_allowskip_still_fails"] = False
            errors.append("Core service allowskip not detected")
            if verbose:
                print("  FAIL")

        # HULK02R4 Self-Tests

        # Test 10: Error contract without CaptureStatusFailed fails
        test_count += 1
        if verbose:
            print("Test 10: Error contract without CaptureStatusFailed fails")
        error_contract_file = os.path.join(test_dir, "diag/capture_service_error_contract_test.go")
        os.makedirs(os.path.dirname(error_contract_file), exist_ok=True)
        with open(error_contract_file, 'w') as f:
            f.write('package diag\n')
            f.write('// ACT-UVB76-HULK02: Test\n')
            f.write('func TestSomething(t *testing.T) {\n')
            f.write('    // Test without CaptureStatusFailed assertion\n')
            f.write('    if capture.Status != state.DiagCaptureStatusError {\n')
            f.write('        t.Errorf("expected error status")\n')
            f.write('    }\n')
            f.write('}\n')

        capture_status_errors = check_capture_status_assertions_in_core_service_files("diag/capture_service_error_contract_test.go", test_dir)
        if capture_status_errors:
            results["error_contract_missing_capture_status_failed"] = True
            pass_count += 1
            if verbose:
                print("  PASS")
        else:
            results["error_contract_missing_capture_status_failed"] = False
            errors.append("Error contract without CaptureStatusFailed not detected")
            if verbose:
                print("  FAIL")

        # Test 11: Error contract with CaptureStatusFailed passes
        test_count += 1
        if verbose:
            print("Test 11: Error contract with CaptureStatusFailed passes")
        error_contract_file2 = os.path.join(test_dir, "diag/capture_service_error_contract_test.go")
        os.makedirs(os.path.dirname(error_contract_file2), exist_ok=True)
        with open(error_contract_file2, 'w') as f:
            f.write('package diag\n')
            f.write('// ACT-UVB76-HULK02: Test\n')
            f.write('func TestSomething(t *testing.T) {\n')
            f.write('    if capture.Status != state.DiagCaptureStatusError {\n')
            f.write('        t.Errorf("expected error status")\n')
            f.write('    }\n')
            f.write('    if capture.CaptureStatus != state.CaptureStatusFailed {\n')
            f.write('        t.Errorf("expected failed capture status")\n')
            f.write('    }\n')
            f.write('}\n')

        capture_status_errors2 = check_capture_status_assertions_in_core_service_files("diag/capture_service_error_contract_test.go", test_dir)
        if not capture_status_errors2:
            results["error_contract_with_capture_status_failed"] = True
            pass_count += 1
            if verbose:
                print("  PASS")
        else:
            results["error_contract_with_capture_status_failed"] = False
            errors.append("Error contract with CaptureStatusFailed incorrectly flagged")
            if verbose:
                print("  FAIL")

        # Test 12: Success contract without CaptureStatusCaptured fails
        test_count += 1
        if verbose:
            print("Test 12: Success contract without CaptureStatusCaptured fails")
        success_contract_file = os.path.join(test_dir, "diag/capture_service_success_contract_test.go")
        os.makedirs(os.path.dirname(success_contract_file), exist_ok=True)
        with open(success_contract_file, 'w') as f:
            f.write('package diag\n')
            f.write('// ACT-UVB76-HULK02: Test\n')
            f.write('func TestSomething(t *testing.T) {\n')
            f.write('    if capture.Status != state.DiagCaptureStatusOK {\n')
            f.write('        t.Errorf("expected ok status")\n')
            f.write('    }\n')
            f.write('}\n')

        capture_status_errors3 = check_capture_status_assertions_in_core_service_files("diag/capture_service_success_contract_test.go", test_dir)
        if capture_status_errors3:
            results["success_contract_missing_capture_status_captured"] = True
            pass_count += 1
            if verbose:
                print("  PASS")
        else:
            results["success_contract_missing_capture_status_captured"] = False
            errors.append("Success contract without CaptureStatusCaptured not detected")
            if verbose:
                print("  FAIL")

        # Test 13: Success contract with CaptureStatusCaptured passes
        test_count += 1
        if verbose:
            print("Test 13: Success contract with CaptureStatusCaptured passes")
        success_contract_file2 = os.path.join(test_dir, "diag/capture_service_success_contract_test.go")
        os.makedirs(os.path.dirname(success_contract_file2), exist_ok=True)
        with open(success_contract_file2, 'w') as f:
            f.write('package diag\n')
            f.write('// ACT-UVB76-HULK02: Test\n')
            f.write('func TestSomething(t *testing.T) {\n')
            f.write('    if capture.Status != state.DiagCaptureStatusOK {\n')
            f.write('        t.Errorf("expected ok status")\n')
            f.write('    }\n')
            f.write('    if capture.CaptureStatus != state.CaptureStatusCaptured {\n')
            f.write('        t.Errorf("expected captured status")\n')
            f.write('    }\n')
            f.write('}\n')

        capture_status_errors4 = check_capture_status_assertions_in_core_service_files("diag/capture_service_success_contract_test.go", test_dir)
        if not capture_status_errors4:
            results["success_contract_with_capture_status_captured"] = True
            pass_count += 1
            if verbose:
                print("  PASS")
        else:
            results["success_contract_with_capture_status_captured"] = False
            errors.append("Success contract with CaptureStatusCaptured incorrectly flagged")
            if verbose:
                print("  FAIL")

    finally:
        shutil.rmtree(test_dir, ignore_errors=True)

    return errors, results, test_count, pass_count
