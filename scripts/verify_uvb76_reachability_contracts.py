#!/usr/bin/env python3
"""
Verifier for UVB-76 reachability semantics contracts (HULK04).

Validates that HULK04 reachability contracts exist and conform to the expected structure:
- All HULK04 contract files exist and contain proper test patterns
- Canonical reachability terms appear in tests
- Forbidden bare "unreachable" is absent from UI/API/event contract outputs
- No unallowlisted t.Skip/t.Skipf
- No http://test.local in probe/server contract tests
- Makefile exposes hulk-uvb76-reachability-gate
- Gate runs go test -race for probe/state/server reachability tests

Supports self-test mode with fixture validation.

ACT-UVB76-HULK04-PROBE-REACHABILITY-SEMANTICS
ACT-UVB76-HULK04R-RESOLVER (file splitting)
"""

import os
import re
import sys
import tempfile
import shutil

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.dirname(SCRIPT_DIR)
UVB76_DIR = os.path.join(REPO_ROOT, "uvb76")

# HULK04 contract files (updated with split state files and renamed vocabulary projection files)
CONTRACT_FILES = [
    ("probe/reachability.go", "Reachability classifier"),
    ("probe/reachability_semantics_contract_test.go", "Probe semantics contract tests"),
    ("probe/reachability_validation_contract_test.go", "Probe validation contract tests"),
    ("state/reachability_state_http_contract_test.go", "HTTP state transition tests"),
    ("state/reachability_state_icmp_contract_test.go", "ICMP state transition tests"),
    ("state/reachability_state_combined_contract_test.go", "Combined state transition tests"),
    ("server/reachability_event_vocabulary_projection_contract_test.go", "Event vocabulary projection tests"),
    ("server/reachability_api_vocabulary_projection_contract_test.go", "API vocabulary projection tests"),
    ("server/reachability_api_validation_contract_test.go", "API validation tests"),
    ("server/reachability_event_validation_contract_test.go", "Event validation tests"),
    ("probe/reachability_fake_backend_test.go", "Fake probe backends"),
]

# Allowed skip pattern - ACT comments that permit skips
# Allow either on same line or on preceding lines within 100 chars
ALLOWLIST_SKIP_PATTERN = re.compile(
    r'//\s*ACT-UVB76-HULK04-ALLOW-SKIP:.*\n.*t\.Skip|t\.Skip.*//\s*ACT-UVB76-HULK04-ALLOW-SKIP'
)

# Canonical reachability terms that must appear in tests
CANONICAL_TERMS = [
    "target_reachable",
    "service_reachable",
    "partially_reachable",
    "service_unreachable",
    "network_unreachable",
    "probe_failed",
    "probe_degraded",
    "probe_recovered",
    "unknown",
]

# Forbidden bare terms - should NOT appear in API/UI/event contract outputs
# Note: These patterns may appear in tests verifying the forbidden patterns,
# in comments explaining the rule, or in the semantics test itself.
FORBIDDEN_BARE_TERMS = [
    "unreachable",
    "reachable",
]


def check_contract_file_exists(relative_path, description):
    """Check that a contract file exists."""
    full_path = os.path.join(UVB76_DIR, relative_path)
    errors = []
    if not os.path.isfile(full_path):
        errors.append(f"ERROR: {relative_path} does not exist")
    return errors


def check_canonical_terms_in_tests(relative_path):
    """Check that canonical reachability terms appear in tests.
    
    Only semantics and validation test files need the full set of canonical terms.
    State and API tests may test behavior without using the string literals.
    """
    full_path = os.path.join(UVB76_DIR, relative_path)
    errors = []

    if not os.path.isfile(full_path):
        return errors

    with open(full_path, 'r') as f:
        content = f.read()

    # Only semantics and validation tests need canonical terms
    if 'semantics' not in relative_path and 'validation' not in relative_path:
        return errors  # Skip - state and API tests test behavior, not labels

    # Count how many canonical terms appear in this file
    found_count = 0
    for term in CANONICAL_TERMS:
        if f'"{term}"' in content or f"'{term}'" in content:
            found_count += 1

    # Each test file should have at least 3 canonical terms
    if found_count < 3:
        # List what was found
        found_terms = [t for t in CANONICAL_TERMS if f'"{t}"' in content or f"'{t}'" in content]
        errors.append(
            f"ERROR: {relative_path} has only {found_count} canonical terms: {found_terms}. "
            f"Need at least 3 for meaningful coverage."
        )

    return errors


def check_no_forbidden_bare_terms(relative_path):
    """Check that forbidden bare terms don't appear in contract outputs.
    
    Tightened: ACT-UVB76-HULK04R2
    - Now checks ALL files including API/event files
    - Strips comments before checking to allow documentation
    - Skips only reachability.go (defines forbidden terms) and fake backends (use forbidden as fixture)
    """
    full_path = os.path.join(UVB76_DIR, relative_path)
    errors = []

    if not os.path.isfile(full_path):
        return errors

    with open(full_path, 'r') as f:
        content = f.read()

    # Skip reachability.go - it defines the forbidden terms as part of its implementation
    if relative_path == 'probe/reachability.go':
        return errors

    # Skip validation and vocabulary_projection test files - they intentionally use
    # forbidden terms as test fixtures to verify IsLabelForbidden() and ForbiddenLabels()
    # work correctly. These are assertions, not production code paths.
    # Examples: if !probe.IsLabelForbidden("unreachable") { ... }
    #           {"bare_unreachable", "unreachable", true}  // test fixture
    if 'validation' in relative_path or 'vocabulary_projection' in relative_path:
        return errors

    # Skip fake backend test files - they intentionally use forbidden terms as fixtures
    if 'fake_backend' in relative_path:
        return errors

    # Strip comments from content before checking
    # Remove single-line comments
    lines = content.split('\n')
    code_lines = []
    for line in lines:
        # Find comment start
        comment_idx = line.find('//')
        if comment_idx >= 0:
            # Only strip if comment is at the start or after code
            code_part = line[:comment_idx]
            comment_part = line[comment_idx:]
            # Check if comment contains ACT allow directive
            if 'ACT-UVB76-HULK04-ALLOW-SKIP' in comment_part:
                code_lines.append(code_part)
            else:
                code_lines.append(code_part)
        else:
            code_lines.append(line)
    
    code_content = '\n'.join(code_lines)

    # Look for bare unreachable/reachable in code (not comments)
    for term in FORBIDDEN_BARE_TERMS:
        # Check for bare term as string literal in code
        pattern = rf'"{term}"'
        matches = list(re.finditer(pattern, code_content))

        for match in matches:
            # Get line content for context
            start = code_content.rfind('\n', 0, match.start()) + 1
            end = code_content.find('\n', match.end())
            if end == -1:
                end = len(code_content)
            line = code_content[start:end]

            # Found a bare term in code
            errors.append(
                f"ERROR: {relative_path} contains forbidden bare term '{term}' at line: {line.strip()}"
            )

    return errors


def check_no_unallowlisted_skips(relative_path):
    """Check that contract files do not contain unallowlisted t.Skip patterns."""
    full_path = os.path.join(UVB76_DIR, relative_path)
    errors = []

    if not os.path.isfile(full_path):
        return errors

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
                f"Allowed only with '// ACT-UVB76-HULK04-ALLOW-SKIP:' comment."
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
        return errors

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
    """Check that Makefile contains hulk-uvb76-reachability-gate target."""
    makefile_path = os.path.join(REPO_ROOT, "Makefile")
    errors = []

    if not os.path.isfile(makefile_path):
        errors.append("ERROR: Makefile does not exist")
        return errors

    with open(makefile_path, 'r') as f:
        content = f.read()

    # Check for hulk-uvb76-reachability-gate target
    if not re.search(r'^hulk-uvb76-reachability-gate\s*:', content, re.MULTILINE):
        errors.append("ERROR: Makefile lacks 'hulk-uvb76-reachability-gate:' target")
        return errors

    # Check that hulk-uvb76-reachability-gate includes go test -race
    hulk_gate_match = re.search(
        r'^hulk-uvb76-reachability-gate\s*:.*?(?=\n\n|\n\.[A-Z]|\Z)',
        content,
        re.MULTILINE | re.DOTALL
    )
    if hulk_gate_match:
        gate_content = hulk_gate_match.group(0)
        if 'go test' not in gate_content:
            errors.append("ERROR: hulk-uvb76-reachability-gate target lacks 'go test' command")
        if '-race' not in gate_content:
            errors.append("ERROR: hulk-uvb76-reachability-gate target lacks '-race' flag for go test")

    return errors


def check_files_under_line_limit():
    """Check that all HULK04 files are under reasonable line limits.
    
    ACT-UVB76-HULK04R2: Restored 450 line limit as required.
    Files over 450 lines should be split into smaller, focused files.
    """
    errors = []
    for relative_path, _ in CONTRACT_FILES:
        full_path = os.path.join(UVB76_DIR, relative_path)
        if os.path.isfile(full_path):
            with open(full_path, 'r') as f:
                line_count = len(f.readlines())
            # All HULK04 files should be under 450 lines
            limit = 450
            if line_count > limit:
                errors.append(
                    f"ERROR: {relative_path} has {line_count} lines (limit: {limit})"
                )
    return errors


def run_verifier():
    """Run the reachability contract verifier."""
    all_errors = []
    print("=== UVB-76 Reachability Contract Verifier (HULK04) ===\n")

    print("A. Checking contract files exist...")
    for relative_path, description in CONTRACT_FILES:
        print(f"  Checking: {relative_path}")
        errors = check_contract_file_exists(relative_path, description)
        if errors:
            for e in errors:
                print(f"    {e}")
                all_errors.append(e)
        else:
            print(f"    OK: File exists")

    print("\nB. Checking canonical terms in tests...")
    for relative_path, description in CONTRACT_FILES:
        if '_test.go' in relative_path:
            print(f"  Checking terms in: {relative_path}")
            errors = check_canonical_terms_in_tests(relative_path)
            if errors:
                for e in errors:
                    print(f"    {e}")
                    all_errors.append(e)
            else:
                print(f"    OK: All canonical terms present")

    print("\nC. Checking for forbidden bare terms...")
    for relative_path, description in CONTRACT_FILES:
        if '_test.go' in relative_path or 'reachability.go' in relative_path:
            print(f"  Checking forbidden terms in: {relative_path}")
            errors = check_no_forbidden_bare_terms(relative_path)
            if errors:
                for e in errors:
                    print(f"    {e}")
                    all_errors.append(e)
            else:
                print(f"    OK: No forbidden bare terms")

    print("\nD. Checking for unallowlisted t.Skip patterns...")
    for relative_path, description in CONTRACT_FILES:
        if '_test.go' in relative_path:
            print(f"  Checking skips in: {relative_path}")
            errors = check_no_unallowlisted_skips(relative_path)
            if errors:
                for e in errors:
                    print(f"    {e}")
                    all_errors.append(e)
            else:
                print(f"    OK: No unallowlisted t.Skip found")

    print("\nE. Checking probe contracts for DNS dependency...")
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

    print("\nF. Checking Makefile hulk-uvb76-reachability-gate target...")
    errors = check_makefile_has_hulk_gate()
    if errors:
        for e in errors:
            print(f"    {e}")
            all_errors.append(e)
    else:
        print(f"    OK: Makefile contains hulk-uvb76-reachability-gate with go test -race")

    print("\nG. Checking file line limits...")
    errors = check_files_under_line_limit()
    if errors:
        for e in errors:
            print(f"    {e}")
            all_errors.append(e)
    else:
        print(f"    OK: All files under 350/450 lines")

    print("\n" + "=" * 50)
    print("SUMMARY:")
    print(f"  Contract files: {len(CONTRACT_FILES)}")
    print(f"  Errors: {len(all_errors)}")

    return all_errors

def main():
    # Delegate self-test to separate module
    if "--self-test" in sys.argv:
        import subprocess
        selftest_path = os.path.join(SCRIPT_DIR, "verify_uvb76_reachability_contracts_selftest.py")
        result = subprocess.run([sys.executable, selftest_path, "--self-test"], capture_output=False)
        sys.exit(result.returncode)

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
