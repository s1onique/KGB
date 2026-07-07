#!/usr/bin/env python3
"""
Self-test validation for UVB-76 reachability contracts verifier.

ACT-UVB76-HULK04R3
"""

import os
import re
import sys
import tempfile

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.dirname(SCRIPT_DIR)
UVB76_DIR = os.path.join(REPO_ROOT, "uvb76")

ALLOWLIST_SKIP_PATTERN = re.compile(
    r'//\s*ACT-UVB76-HULK04-ALLOW-SKIP:.*t\.Skip'
)

FORBIDDEN_BARE_TERMS = ["unreachable", "reachable"]
CANONICAL_TERMS = [
    "target_reachable", "service_reachable", "partially_reachable",
    "service_unreachable", "network_unreachable", "probe_failed",
    "probe_degraded", "probe_recovered", "unknown",
]


def check_contract_file_exists(relative_path):
    full_path = os.path.join(UVB76_DIR, relative_path)
    if not os.path.isfile(full_path):
        return [f"ERROR: {relative_path} does not exist"]
    return []


def check_no_forbidden_bare_terms(relative_path):
    full_path = os.path.join(UVB76_DIR, relative_path)
    errors = []

    if not os.path.isfile(full_path):
        return errors

    with open(full_path, 'r') as f:
        content = f.read()

    for term in FORBIDDEN_BARE_TERMS:
        pattern = rf'"{term}"'
        matches = list(re.finditer(pattern, content))

        for match in matches:
            start = content.rfind('\n', 0, match.start()) + 1
            end = content.find('\n', match.end())
            if end == -1:
                end = len(content)
            line = content[start:end]

            stripped = line.strip()
            if '//' in stripped and (stripped.startswith('//') or '//' in stripped[:stripped.find('"')]):
                continue
            if '// Forbidden' in line or '// ACT-' in line:
                continue

            errors.append(f"ERROR: {relative_path} contains forbidden bare term '{term}'")

    return errors


def check_canonical_terms_in_tests(relative_path):
    full_path = os.path.join(UVB76_DIR, relative_path)
    errors = []

    if not os.path.isfile(full_path):
        return errors

    with open(full_path, 'r') as f:
        content = f.read()

    found_count = 0
    for term in CANONICAL_TERMS:
        if f'"{term}"' in content or f"'{term}'" in content:
            found_count += 1

    if found_count < 3:
        errors.append(f"ERROR: {relative_path} has only {found_count} canonical terms")

    return errors


def check_probe_no_dns_dependency(relative_path):
    full_path = os.path.join(UVB76_DIR, relative_path)
    errors = []

    if not relative_path.startswith("probe/"):
        return errors

    if not os.path.isfile(full_path):
        return errors

    with open(full_path, 'r') as f:
        content = f.read()

    dns_pattern = re.compile(r'https?://test(?:[0-9]*)?\.local', re.IGNORECASE)
    matches = dns_pattern.findall(content)

    if matches:
        for match in matches:
            errors.append(f"ERROR: {relative_path} contains DNS-dependent URL '{match}'")

    return errors


def check_makefile_has_hulk_gate():
    makefile_path = os.path.join(REPO_ROOT, "Makefile")
    errors = []

    if not os.path.isfile(makefile_path):
        return ["ERROR: Makefile does not exist"]

    with open(makefile_path, 'r') as f:
        content = f.read()

    if not re.search(r'^hulk-uvb76-reachability-gate\s*:', content, re.MULTILINE):
        return ["ERROR: Makefile lacks 'hulk-uvb76-reachability-gate:' target"]

    hulk_gate_match = re.search(
        r'^hulk-uvb76-reachability-gate\s*:.*?(?=\n\n|\n\.[A-Z]|\Z)',
        content,
        re.MULTILINE | re.DOTALL
    )
    if hulk_gate_match:
        gate_content = hulk_gate_match.group(0)
        if 'go test' not in gate_content:
            errors.append("ERROR: hulk-uvb76-reachability-gate lacks 'go test'")
        if '-race' not in gate_content:
            errors.append("ERROR: hulk-uvb76-reachability-gate lacks '-race' flag")

    return errors


def run_self_tests():
    """Run self-test validation."""
    errors = []
    results = {}

    print("\n=== Self-Test Mode ===\n")

    # Test 1: Missing required file fails
    print("Test 1: Missing required file fails...")
    with tempfile.TemporaryDirectory() as tmpdir:
        fake_uvb76 = os.path.join(tmpdir, "uvb76")
        os.makedirs(fake_uvb76)

        original_dir = UVB76_DIR
        globals()['UVB76_DIR'] = fake_uvb76

        test_errors = check_contract_file_exists("probe/reachability.go")
        if test_errors:
            results["missing_file_fails"] = True
            print("  PASS: Missing file correctly fails")
        else:
            results["missing_file_fails"] = False
            errors.append("Missing file should have failed")
            print("  FAIL: Missing file should have failed")

        globals()['UVB76_DIR'] = original_dir

    # Test 2: Bare unreachable label fails
    print("Test 2: Bare unreachable label fails...")
    with tempfile.TemporaryDirectory() as tmpdir:
        fake_uvb76 = os.path.join(tmpdir, "uvb76")
        os.makedirs(fake_uvb76, exist_ok=True)

        test_file = os.path.join(fake_uvb76, "probe", "reachability_test.go")
        os.makedirs(os.path.dirname(test_file), exist_ok=True)
        with open(test_file, 'w') as f:
            f.write('func TestBad() { label := "unreachable" }')

        original_dir = UVB76_DIR
        globals()['UVB76_DIR'] = fake_uvb76

        test_errors = check_no_forbidden_bare_terms("probe/reachability_test.go")
        if test_errors:
            results["bare_unreachable_fails"] = True
            print("  PASS: Bare unreachable correctly fails")
        else:
            results["bare_unreachable_fails"] = False
            errors.append("Bare unreachable should have failed")
            print("  FAIL: Bare unreachable should have failed")

        globals()['UVB76_DIR'] = original_dir

    # Test 3: http://test.local fixture fails
    print("Test 3: DNS fixture http://test.local fails...")
    with tempfile.TemporaryDirectory() as tmpdir:
        fake_uvb76 = os.path.join(tmpdir, "uvb76")
        os.makedirs(fake_uvb76, exist_ok=True)

        test_file = os.path.join(fake_uvb76, "probe", "dns_test.go")
        os.makedirs(os.path.dirname(test_file), exist_ok=True)
        with open(test_file, 'w') as f:
            f.write('url := "http://test.local/status"')

        original_dir = UVB76_DIR
        globals()['UVB76_DIR'] = fake_uvb76

        test_errors = check_probe_no_dns_dependency("probe/dns_test.go")
        if test_errors:
            results["dns_fixture_fails"] = True
            print("  PASS: DNS fixture correctly fails")
        else:
            results["dns_fixture_fails"] = False
            errors.append("DNS fixture should have failed")
            print("  FAIL: DNS fixture should have failed")

        globals()['UVB76_DIR'] = original_dir

    # Test 4: Valid fixture passes
    print("Test 4: Valid fixture passes...")
    with tempfile.TemporaryDirectory() as tmpdir:
        fake_uvb76 = os.path.join(tmpdir, "uvb76")
        os.makedirs(fake_uvb76, exist_ok=True)

        test_file = os.path.join(fake_uvb76, "probe", "valid_test.go")
        os.makedirs(os.path.dirname(test_file), exist_ok=True)
        with open(test_file, 'w') as f:
            f.write('label := "target_reachable"\n')
            f.write('label2 := "service_unreachable"\n')
            f.write('label3 := "partially_reachable"')

        original_dir = UVB76_DIR
        globals()['UVB76_DIR'] = fake_uvb76

        term_errors = check_canonical_terms_in_tests("probe/valid_test.go")
        forbidden_errors = check_no_forbidden_bare_terms("probe/valid_test.go")

        if not term_errors and not forbidden_errors:
            results["valid_fixture_passes"] = True
            print("  PASS: Valid fixture correctly passes")
        else:
            results["valid_fixture_passes"] = False
            errors.append("Valid fixture should have passed")
            print("  FAIL: Valid fixture should have passed")

        globals()['UVB76_DIR'] = original_dir

    # Test 5: Makefile -race check
    print("Test 5: Makefile missing -race fails...")
    with tempfile.TemporaryDirectory() as tmpdir:
        fake_repo = tmpdir
        makefile_path = os.path.join(fake_repo, "Makefile")

        with open(makefile_path, 'w') as f:
            f.write("""
hulk-uvb76-reachability-gate:
\t@cd uvb76 && go test -v ./probe/... ./state/...
\t@python3 scripts/verify_uvb76_reachability_contracts.py
""")

        original_root = REPO_ROOT
        globals()['REPO_ROOT'] = fake_repo
        globals()['UVB76_DIR'] = os.path.join(fake_repo, "uvb76")

        test_errors = check_makefile_has_hulk_gate()
        if test_errors and any('-race' in e for e in test_errors):
            results["makefile_race_check"] = True
            print("  PASS: Makefile missing -race correctly fails")
        else:
            results["makefile_race_check"] = False
            errors.append("Makefile missing -race should have failed")
            print("  FAIL: Makefile missing -race should have failed")

        globals()['REPO_ROOT'] = original_root
        globals()['UVB76_DIR'] = os.path.join(original_root, "uvb76")

    # Test 6: unallowlisted t.Skip fails
    print("Test 6: Unallowlisted t.Skip fails...")
    with tempfile.TemporaryDirectory() as tmpdir:
        fake_uvb76 = os.path.join(tmpdir, "uvb76")
        os.makedirs(fake_uvb76, exist_ok=True)

        test_file = os.path.join(fake_uvb76, "test_skip_test.go")
        with open(test_file, 'w') as f:
            f.write('func TestSkip() { t.Skip("reason") }')

        original_dir = UVB76_DIR
        globals()['UVB76_DIR'] = fake_uvb76

        # Check for t.Skip pattern
        skip_pattern = re.compile(r't\.Skip[f]?\s*\(', re.MULTILINE)
        with open(test_file, 'r') as f:
            content = f.read()
        skip_matches = list(skip_pattern.finditer(content))

        if skip_matches and not ALLOWLIST_SKIP_PATTERN.search(content):
            results["unallowlisted_skip_fails"] = True
            print("  PASS: Unallowlisted skip correctly fails")
        else:
            results["unallowlisted_skip_fails"] = False
            errors.append("Unallowlisted skip should have failed")
            print("  FAIL: Unallowlisted skip should have failed")

        globals()['UVB76_DIR'] = original_dir

    return errors, results


def main():
    errors, results = run_self_tests()
    test_count = len(results)
    pass_count = sum(1 for v in results.values() if v)

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


if __name__ == "__main__":
    main()
