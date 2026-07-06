#!/usr/bin/env python3
# verify_state_transition_register.py — State transition register and test wiring verifier
#
# ACT-TOVARISCH-ZIG-HULK26: Static verifier for protocol transition coverage
#
# Verifies:
# 1. Transition register exists
# 2. No active DEFERRED transitions in register
# 3. Required transition test files exist (BGP totality, BFD totality, BFD FSM)
# 4. Transition tests are imported by test_all.zig
# 5. Transition tests are imported by split suites
# 6. No documentation-only placeholder expect(true) assertions
#
# Exit codes:
#   0 — all checks pass
#   1 — verification failed

import re
import sys
from pathlib import Path
from typing import List, Tuple

# Repository root relative paths
REPO_ROOT = Path(__file__).parent.parent

REQUIRED_REGISTER = REPO_ROOT / "docs/architecture/tovarisch-state-transition-register.md"

REQUIRED_TRANSITION_TESTS = [
    REPO_ROOT / "tovarisch/src/bgp/transition_totality_tests.zig",
    REPO_ROOT / "tovarisch/src/bfd/transition_totality_tests.zig",
    REPO_ROOT / "tovarisch/src/bfd/transition_fsm_tests.zig",
]

REQUIRED_AGGREGATE_SUITES = [
    REPO_ROOT / "tovarisch/src/test_all.zig",
]

SPLIT_SUITE_PATTERNS = [
    REPO_ROOT / "tovarisch/src/test_suite_base.zig",
    REPO_ROOT / "tovarisch/src/test_suite_bgp.zig",
    REPO_ROOT / "tovarisch/src/test_suite_bfd.zig",
]

# Patterns that indicate placeholder test content
PLACEHOLDER_PATTERNS = [
    re.compile(r'^\s*try\s+std\.testing\.expect\s*\(\s*true\s*\)\s*;?\s*$'),
    re.compile(r'^\s*try\s+testing\.expect\s*\(\s*true\s*\)\s*;?\s*$'),
]

# Patterns that indicate DEFERRED transitions (case-insensitive)
DEFERRED_PATTERN = re.compile(r'\bDEFERRED\b', re.IGNORECASE)


def _relative_path(path: Path) -> str:
    """Return repo-relative path for cleaner diagnostics."""
    try:
        return str(path.relative_to(REPO_ROOT))
    except ValueError:
        return str(path)


def check_required_paths_exist() -> List[Tuple[str, str]]:
    """Check that required files exist. Returns list of (category, message) errors."""
    errors = []
    
    if not REQUIRED_REGISTER.exists():
        errors.append(("[missing]", f"{_relative_path(REQUIRED_REGISTER)} does not exist"))
    
    for test_path in REQUIRED_TRANSITION_TESTS:
        if not test_path.exists():
            errors.append(("[missing]", f"{_relative_path(test_path)} does not exist"))
    
    return errors


def check_no_deferred_transitions() -> List[Tuple[str, str]]:
    """Check register has no active deferred transition entries."""
    errors = []
    
    if not REQUIRED_REGISTER.exists():
        return errors  # Already reported by check_required_paths_exist
    
    try:
        content = REQUIRED_REGISTER.read_text()
        lines = content.split('\n')
        
        for i, line in enumerate(lines, 1):
            # Skip comments and known-safe sections
            stripped = line.strip()
            if stripped.startswith('#') or stripped.startswith('//'):
                continue
            
            # Check for DEFERRED token
            if DEFERRED_PATTERN.search(line):
                # Allow "DEFERRED transitions: 0" or "No Pending Transitions" section
                # Fail if it's not the zero-count declaration
                if 'DEFERRED transitions: 0' in line or 'DEFERRED transitions:0' in line:
                    continue
                errors.append((
                    "[deferred]",
                    f"{_relative_path(REQUIRED_REGISTER)}:{i} contains DEFERRED"
                ))
                
    except Exception as e:
        errors.append(("[error]", f"failed to read {_relative_path(REQUIRED_REGISTER)}: {e}"))
    
    return errors


def check_no_placeholder_expect_true(test_path: Path) -> List[Tuple[str, str]]:
    """Check test file has no placeholder expect(true) assertions."""
    errors = []
    
    if not test_path.exists():
        return errors  # Already reported by check_required_paths_exist
    
    try:
        content = test_path.read_text()
        lines = content.split('\n')
        
        for i, line in enumerate(lines, 1):
            # Skip comments
            stripped = line.strip()
            if stripped.startswith('//'):
                continue
            
            for pattern in PLACEHOLDER_PATTERNS:
                if pattern.search(line):
                    errors.append((
                        "[placeholder]",
                        f"{_relative_path(test_path)}:{i} contains documentation-only expect(true)"
                    ))
                    
    except Exception as e:
        errors.append(("[error]", f"failed to read {_relative_path(test_path)}: {e}"))
    
    return errors


def extract_imports(file_path: Path) -> List[str]:
    """Extract @import(...) paths from a Zig file."""
    imports = []
    
    if not file_path.exists():
        return imports
    
    try:
        content = file_path.read_text()
        # Match @import("...") patterns
        pattern = re.compile(r'@import\("([^"]+)"\)')
        for match in pattern.finditer(content):
            imports.append(match.group(1))
    except Exception:
        pass
    
    return imports


def check_required_imports(suite_path: Path, required_imports: List[str]) -> List[Tuple[str, str]]:
    """Check that suite imports all required transition tests."""
    errors = []
    
    if not suite_path.exists():
        errors.append(("[missing]", f"{suite_path} does not exist"))
        return errors
    
    imports = extract_imports(suite_path)
    
    for required in required_imports:
        if required not in imports:
            errors.append((
                "[unwired]",
                f"{suite_path} does not import {required}"
            ))
    
    return errors


def discover_split_suites() -> List[Path]:
    """Discover split suite files using narrow pattern matching."""
    src_dir = REPO_ROOT / "tovarisch/src"
    suites = []
    
    if src_dir.exists():
        for pattern in ["test_suite_*.zig"]:
            suites.extend(src_dir.glob(pattern))
    
    return sorted(suites)


def _get_zig_import_path(test_path: Path) -> str:
    """Get the relative import path for a test file from tovarisch/src/."""
    src_dir = REPO_ROOT / "tovarisch/src"
    try:
        return str(test_path.relative_to(src_dir))
    except ValueError:
        return test_path.name


def main() -> int:
    """Run all state transition register checks."""
    all_errors = []
    
    # Check 1: Required paths exist
    all_errors.extend(check_required_paths_exist())
    
    # Check 2: No deferred transitions
    all_errors.extend(check_no_deferred_transitions())
    
    # Check 3-5: Required transition test files exist (already checked in #1)
    
    # Check 6: Transition tests are wired into test_all.zig
    for suite in REQUIRED_AGGREGATE_SUITES:
        # Get the import path for each test (relative to tovarisch/src/)
        test_imports = [_get_zig_import_path(p) for p in REQUIRED_TRANSITION_TESTS]
        suite_imports = extract_imports(suite)
        for test_import in test_imports:
            if test_import not in suite_imports:
                all_errors.append((
                    "[unwired]",
                    f"{suite} does not import {test_import}"
                ))
    
    # Check 7: Transition tests are wired into split suites
    split_suites = discover_split_suites()
    
    # Map transition tests to the split suites that should import them
    # BGP tests -> test_suite_bgp.zig only (not sub-suites)
    # BFD tests -> test_suite_bfd.zig only
    # Note: test_suite_bgp_*.zig (protocol, session, tcp, integration) are sub-suites
    #       that inherit from test_suite_bgp.zig
    
    bgp_test = "bgp/transition_totality_tests.zig"
    bfd_totality_test = "bfd/transition_totality_tests.zig"
    bfd_fsm_test = "bfd/transition_fsm_tests.zig"
    
    for suite in split_suites:
        suite_name = suite.name
        imports = extract_imports(suite)
        
        # Skip sub-suites that are children of the main BGP/BFD suites
        # Only the top-level suites (test_suite_bgp.zig, test_suite_bfd.zig) need imports
        if suite_name.startswith("test_suite_bgp_") and suite_name != "test_suite_bgp.zig":
            continue
        if suite_name.startswith("test_suite_http"):
            continue
        if suite_name.startswith("test_suite_cli"):
            continue
        
        # BFD suite should have all BFD transition tests
        if suite_name == "test_suite_bfd.zig":
            for test in [bfd_totality_test, bfd_fsm_test]:
                if test not in imports:
                    all_errors.append((
                        "[unwired]",
                        f"{suite} does not import {test}"
                    ))
        
        # BGP suite should have BGP transition tests
        if suite_name == "test_suite_bgp.zig":
            if bgp_test not in imports:
                all_errors.append((
                    "[unwired]",
                    f"{suite} does not import {bgp_test}"
                ))
    
    # Check 8: No placeholder expect(true) in transition tests
    for test_path in REQUIRED_TRANSITION_TESTS:
        all_errors.extend(check_no_placeholder_expect_true(test_path))
    
    # Output results
    if all_errors:
        print("STATE TRANSITION REGISTER VERIFIER: FAIL")
        # Sort for deterministic output
        for category, message in sorted(all_errors):
            print(f"{category} {message}")
        return 1
    
    print("STATE TRANSITION REGISTER VERIFIER: PASS")
    print(f"checked_register={_relative_path(REQUIRED_REGISTER)}")
    print(f"deferred_transitions=0")
    print(f"checked_transition_tests={len(REQUIRED_TRANSITION_TESTS)}")
    print(f"checked_suite_files={len(split_suites)}")
    
    return 0


if __name__ == "__main__":
    sys.exit(main())
