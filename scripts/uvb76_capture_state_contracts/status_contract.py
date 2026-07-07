"""
Canonical status and reason contract validation for UVB-76 HULK02.

Validates that contract tests reference canonical capture statuses and
structured TCP absence reasons. Does not own CLI parsing.
"""

import os
import re

from .constants import (
    CANONICAL_STATUSES,
    CONTRACT_FILES,
    TCP_ABSENCE_REASONS,
)
from .json_normalize import strip_comments, strip_json_strings


def check_canonical_statuses_defined(relative_path: str, uvb76_dir: str) -> list[str]:
    """
    Check that canonical statuses are defined in tests.

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

    # Count how many canonical statuses are referenced
    statuses_found = sum(
        1 for s in CANONICAL_STATUSES
        if f'CaptureStatus{s.title().replace("_", "")}' in content or f'"{s}"' in content
    )

    # At least 1 canonical status should be referenced (some focused tests only test 1-2)
    if statuses_found < 1:
        errors.append(f"ERROR: {relative_path} references fewer than 1 canonical status")

    return errors


def check_tcp_absence_allowlist(uvb76_dir: str, verbose: bool = True) -> list[str]:
    """
    Check that TCP absence reasons are preserved in contract tests.

    Args:
        uvb76_dir: Absolute path to the uvb76 package directory.
        verbose: If True, print progress messages.

    Returns:
        List of error messages (empty if no errors).
    """
    errors = []

    # Check diag contract files for reason codes
    diag_dir = os.path.join(uvb76_dir, "diag")
    tcp_absence_test = os.path.join(diag_dir, "capture_service_tcp_absence_contract_test.go")

    found_reasons = False
    if os.path.isfile(tcp_absence_test):
        with open(tcp_absence_test, 'r') as f:
            content = f.read()
        reasons_found = sum(1 for r in TCP_ABSENCE_REASONS if r in content)
        if reasons_found >= 5:
            found_reasons = True

    if not found_reasons:
        errors.append("ERROR: TCP absence reasons not found in contract tests")

    return errors


def check_fake_backend_used(relative_path: str, uvb76_dir: str) -> list[str]:
    """
    Check that capture service contract tests use fake backends.

    Args:
        relative_path: Path relative to uvb76_dir.
        uvb76_dir: Absolute path to the uvb76 package directory.

    Returns:
        List of error messages (empty if no errors).
    """
    full_path = os.path.join(uvb76_dir, relative_path)
    errors = []

    # Only check diag files
    if not relative_path.startswith("diag/"):
        return errors

    if not os.path.isfile(full_path):
        return errors  # Already reported by existence check

    with open(full_path, 'r') as f:
        content = f.read()

    # Strip comments to avoid false positives from doctrine comments
    content_no_comments = strip_comments(content)
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
    forbidden_tools = ['tcpdump', 'ss', 'ip']
    for tool in forbidden_tools:
        # Check for the tool as a JSON string value like "ss" or "ip"
        if re.search(rf'":\s*"{re.escape(tool)}"', content_no_comments):
            # Check if this is in a test data context (command_tool field is allowed)
            if re.search(rf'"command_tool":\s*"{re.escape(tool)}"', content_no_comments):
                continue  # This is test data, not real command execution
            errors.append(
                f"ERROR: {relative_path} contains forbidden tool '{tool}' in JSON - "
                f"use fake backends instead"
            )

    return errors


def validate_status_contracts(uvb76_dir: str, verbose: bool = True) -> list[str]:
    """
    Validate canonical status and reason contracts.

    Args:
        uvb76_dir: Absolute path to the uvb76 package directory.
        verbose: If True, print progress messages.

    Returns:
        List of error messages.
    """
    import re
    all_errors = []

    if verbose:
        print("E. Checking canonical statuses defined in contracts...")
    for relative_path, description in CONTRACT_FILES:
        if verbose:
            print(f"  Checking canonical statuses in: {relative_path}")
        errors = check_canonical_statuses_defined(relative_path, uvb76_dir)
        if errors:
            if verbose:
                for e in errors:
                    print(f"    {e}")
            all_errors.extend(errors)
        elif verbose:
            print(f"    OK: References canonical statuses")

    if verbose:
        print("\nF. Checking TCP absence reason allowlist...")
    errors = check_tcp_absence_allowlist(uvb76_dir, verbose)
    if errors:
        if verbose:
            for e in errors:
                print(f"    {e}")
        all_errors.extend(errors)
    elif verbose:
        print(f"    OK: TCP absence reasons preserved")

    if verbose:
        print("\nD. Checking fake backend usage in capture service contracts...")
    for relative_path, description in CONTRACT_FILES:
        if relative_path.startswith("diag/"):
            if verbose:
                print(f"  Checking fake backend in: {relative_path}")
            errors = check_fake_backend_used(relative_path, uvb76_dir)
            if errors:
                if verbose:
                    for e in errors:
                        print(f"    {e}")
                all_errors.extend(errors)
            elif verbose:
                print(f"    OK: Uses fake backends")

    return all_errors


def get_canonical_status_count() -> int:
    """Return the number of canonical capture statuses."""
    return len(CANONICAL_STATUSES)


def get_tcp_absence_reason_count() -> int:
    """Return the number of TCP absence reasons."""
    return len(TCP_ABSENCE_REASONS)
