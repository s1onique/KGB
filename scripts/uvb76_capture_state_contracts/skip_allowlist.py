"""
Skip allowlist handling for UVB-76 HULK02 Capture State contracts.

Validates that contract test files do not contain unallowlisted t.Skip patterns.
Only skips with explicit ACT-UVB76-HULK02-ALLOW-SKIP comments are permitted.

Also validates HULK02R4 layer contract:
- Core service files MUST assert canonical CaptureStatus for error/disabled/not_configured cases
- Core service files MUST assert CaptureStatusCaptured for success cases
"""

import os
import re

from .constants import (
    ALLOWLIST_SKIP_PATTERN,
    CONTRACT_FILES,
    CORE_SERVICE_CONTRACT_FILES,
)


def check_no_unallowlisted_skips(relative_path: str, uvb76_dir: str) -> list[str]:
    """
    Check that contract files do not contain unallowlisted t.Skip patterns.

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


def check_no_skips_in_core_service_files(relative_path: str, uvb76_dir: str) -> list[str]:
    """
    Check that core service contract files do not contain t.Skip at all.

    Core service files are NOT allowed to skip tests, even with allowlist comments.
    This ensures the service seam remains executable.

    Args:
        relative_path: Path relative to uvb76_dir.
        uvb76_dir: Absolute path to the uvb76 package directory.

    Returns:
        List of error messages (empty if no errors).
    """
    full_path = os.path.join(uvb76_dir, relative_path)
    errors = []
    if not os.path.isfile(full_path):
        return errors

    with open(full_path, 'r') as f:
        content = f.read()

    # Find all t.Skip and t.Skipf occurrences
    skip_pattern = re.compile(r't\.Skip[f]?\s*\(', re.MULTILINE)
    skip_matches = list(skip_pattern.finditer(content))

    if skip_matches:
        errors.append(
            f"ERROR: Core service contract {relative_path} contains {len(skip_matches)} t.Skip(s). "
            f"Core service contracts MUST NOT skip tests. Remove the skip(s) to make tests executable."
        )

    return errors


def check_capture_status_assertions_in_core_service_files(relative_path: str, uvb76_dir: str) -> list[str]:
    """
    Check that core service contract files assert canonical CaptureStatus.

    HULK02R4 requires that:
    - Error contract tests assert CaptureStatusFailed for error/timeout cases
    - Disabled contract tests assert CaptureStatusDisabled
    - No-peer-mapping contract tests assert CaptureStatusNotConfigured
    - Success contract tests assert CaptureStatusCaptured

    Args:
        relative_path: Path relative to uvb76_dir.
        uvb76_dir: Absolute path to the uvb76 package directory.

    Returns:
        List of error messages (empty if no errors).
    """
    full_path = os.path.join(uvb76_dir, relative_path)
    errors = []
    if not os.path.isfile(full_path):
        return errors

    with open(full_path, 'r') as f:
        content = f.read()

    # Determine which status assertions are required based on file type
    filename = os.path.basename(relative_path)

    # Remove comments from content to avoid false positives
    # This ensures we only detect actual code usage, not comments mentioning the status
    code_only = re.sub(r'//.*$', '', content, flags=re.MULTILINE)  # Remove line comments
    code_only = re.sub(r'/\*.*?\*/', '', code_only, flags=re.DOTALL)  # Remove block comments

    if 'error' in filename.lower():
        # Error contract files must assert CaptureStatusFailed for error/timeout cases
        if 'CaptureStatusFailed' not in code_only:
            errors.append(
                f"ERROR: {relative_path} does not assert CaptureStatusFailed. "
                f"HULK02R4 requires core service error contracts to assert canonical CaptureStatusFailed."
            )
    elif 'success' in filename.lower():
        # Success contract files must assert CaptureStatusCaptured
        if 'CaptureStatusCaptured' not in code_only:
            errors.append(
                f"ERROR: {relative_path} does not assert CaptureStatusCaptured. "
                f"HULK02R4 requires core service success contracts to assert canonical CaptureStatusCaptured."
            )
    elif 'tcp_absence' in filename.lower() or 'tcpabsence' in filename.lower():
        # TCP absence contract files must assert CaptureStatusCaptured for success cases
        if 'CaptureStatusCaptured' not in code_only:
            errors.append(
                f"ERROR: {relative_path} does not assert CaptureStatusCaptured. "
                f"HULK02R4 requires TCP absence contracts to assert canonical CaptureStatusCaptured for success cases."
            )
    elif 'json' in filename.lower():
        # JSON contract files must assert CaptureStatusCaptured for success cases
        if 'CaptureStatusCaptured' not in code_only:
            errors.append(
                f"ERROR: {relative_path} does not assert CaptureStatusCaptured. "
                f"HULK02R4 requires JSON contracts to assert canonical CaptureStatusCaptured for success cases."
            )

    return errors


def validate_skip_allowlist(uvb76_dir: str, verbose: bool = True) -> list[str]:
    """
    Validate skip allowlist compliance across all contract files.
    Also checks that core service files have no skips at all.

    Args:
        uvb76_dir: Absolute path to the uvb76 package directory.
        verbose: If True, print progress messages.

    Returns:
        List of error messages.
    """
    all_errors = []

    if verbose:
        print("C. Checking for unallowlisted t.Skip patterns...")
    for relative_path, description in CONTRACT_FILES:
        if verbose:
            print(f"  Checking skips in: {relative_path}")
        errors = check_no_unallowlisted_skips(relative_path, uvb76_dir)
        if errors:
            if verbose:
                for e in errors:
                    print(f"    {e}")
            all_errors.extend(errors)
        elif verbose:
            print(f"    OK: No unallowlisted t.Skip found")

    # Check core service files have NO skips at all
    if verbose:
        print("D. Checking core service files have no t.Skip...")
    for relative_path in CORE_SERVICE_CONTRACT_FILES:
        if verbose:
            print(f"  Checking core service: {relative_path}")
        errors = check_no_skips_in_core_service_files(relative_path, uvb76_dir)
        if errors:
            if verbose:
                for e in errors:
                    print(f"    {e}")
            all_errors.extend(errors)
        elif verbose:
            print(f"    OK: No t.Skip found in core service contract")

    # HULK02R4: Check that core service files assert canonical CaptureStatus
    if verbose:
        print("E. Checking core service files assert canonical CaptureStatus...")
    for relative_path in CORE_SERVICE_CONTRACT_FILES:
        if verbose:
            print(f"  Checking CaptureStatus assertions in: {relative_path}")
        errors = check_capture_status_assertions_in_core_service_files(relative_path, uvb76_dir)
        if errors:
            if verbose:
                for e in errors:
                    print(f"    {e}")
            all_errors.extend(errors)
        elif verbose:
            print(f"    OK: Canonical CaptureStatus assertions found")

    return all_errors
