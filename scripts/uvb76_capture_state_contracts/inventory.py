"""
Split inventory validation for UVB-76 HULK02 Capture State contracts.

Validates that required contract test files exist and contain expected patterns.
Does not own line-limit checks (those belong in line_limits.py).
"""

import os

from .constants import CONTRACT_FILES, HELPER_FILES


def check_contract_test_exists(relative_path: str, description: str, uvb76_dir: str) -> list[str]:
    """
    Check that a contract test file exists.

    Args:
        relative_path: Path relative to uvb76_dir.
        description: Human-readable description for error messages.
        uvb76_dir: Absolute path to the uvb76 package directory.

    Returns:
        List of error messages (empty if no errors).
    """
    full_path = os.path.join(uvb76_dir, relative_path)
    errors = []
    if not os.path.isfile(full_path):
        errors.append(f"ERROR: {relative_path} does not exist")
    return errors


def check_contract_test_content(relative_path: str, uvb76_dir: str) -> list[str]:
    """
    Check that contract test file contains expected test patterns.

    Validates:
    - Contains test functions (func Test... (t *testing.T))
    - Contains ACT-UVB76-HULK02 marker comment

    Args:
        relative_path: Path relative to uvb76_dir.
        uvb76_dir: Absolute path to the uvb76 package directory.

    Returns:
        List of error messages (empty if no errors).
    """
    full_path = os.path.join(uvb76_dir, relative_path)
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


def validate_inventory(uvb76_dir: str, verbose: bool = True) -> tuple[list[str], int]:
    """
    Validate the split inventory of contract test files.

    Checks existence and content patterns for all CONTRACT_FILES and HELPER_FILES.

    Args:
        uvb76_dir: Absolute path to the uvb76 package directory.
        verbose: If True, print progress messages.

    Returns:
        Tuple of (error_messages, total_files_checked).
    """
    all_errors = []

    if verbose:
        print("A. Checking contract test files exist...")
    for relative_path, description in CONTRACT_FILES:
        if verbose:
            print(f"  Checking: {relative_path}")
        errors = check_contract_test_exists(relative_path, description, uvb76_dir)
        if errors:
            if verbose:
                for e in errors:
                    print(f"    {e}")
            all_errors.extend(errors)
        elif verbose:
            print(f"    OK: File exists")

    if verbose:
        print("\nB. Checking contract test content patterns...")
    for relative_path, description in CONTRACT_FILES:
        if verbose:
            print(f"  Checking patterns in: {relative_path}")
        errors = check_contract_test_content(relative_path, uvb76_dir)
        if errors:
            if verbose:
                for e in errors:
                    print(f"    {e}")
            all_errors.extend(errors)
        elif verbose:
            print(f"    OK: Contains test patterns and HULK02 marker")

    total_files = len(CONTRACT_FILES) + len(HELPER_FILES)
    return all_errors, total_files


def get_contract_file_count() -> int:
    """Return the number of required contract files."""
    return len(CONTRACT_FILES)


def get_helper_file_count() -> int:
    """Return the number of optional helper files."""
    return len(HELPER_FILES)
